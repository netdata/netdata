// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type refreshOutputFrame struct {
	payload   string
	committed bool
}

type refreshOutputSink struct {
	frames chan refreshOutputFrame
	done   chan struct{}
	reject atomic.Bool
}

func (*refreshOutputSink) Write([]byte) (int, error) { panic("transactional output expected") }
func (s *refreshOutputSink) CommitJobOutput(payload []byte, state jobruntime.OutputStateTransaction) error {
	var outputErr error
	if s.reject.Swap(false) {
		_ = state.Abort()
		outputErr = errors.New("synthetic frame rejection")
	} else if err := state.Commit(); err != nil {
		return err
	}
	select {
	case s.frames <- refreshOutputFrame{
		payload:   string(payload),
		committed: outputErr == nil,
	}:
	case <-s.done:
	}
	return outputErr
}

type refreshOutputTransaction struct {
	settled chan bool
}

func (s refreshOutputTransaction) Commit() error { s.settled <- true; return nil }
func (s refreshOutputTransaction) Abort() error  { s.settled <- false; return nil }

func TestRefreshOutputCleanupReleasesSenders(t *testing.T) {
	for name, reject := range map[string]bool{"accepted": false, "rejected": true} {
		t.Run(name, func(t *testing.T) {
			c, _, _ := newRefreshTestCollector(t)
			sink, _, cleanup := startRefreshOutputJob(t, c)
			// Exercise a pending send, then another send after cleanup.
			for range 2 {
				sink.reject.Store(reject)
				state := refreshOutputTransaction{settled: make(chan bool)}
				done := make(chan error, 1)
				go func() { done <- sink.CommitJobOutput(nil, state) }()
				assert.Equal(t, !reject, <-state.settled)
				cleanup()
				select {
				case err := <-done:
					if reject {
						assert.EqualError(t, err, "synthetic frame rejection")
					} else {
						assert.NoError(t, err)
					}
				case <-time.After(time.Second):
					// Release the sender before reporting a regression.
					<-sink.frames
					<-done
					t.Fatal("cleanup left the output sender blocked")
				}
			}
		})
	}
}

func TestCollectorRuntimeSystemChangeNativeOutputRetries(t *testing.T) {
	st := newSystemChangeTest(t)
	c, device, now := st.c, st.device, st.now
	sink, collect, cleanup := startRefreshOutputJob(t, c)
	first := collect(1)
	require.True(t, first.committed)
	guid := c.VirtualNode().GUID
	assert.Contains(t, first.payload, "HOST_DEFINE '"+guid+"' 'before'")
	assert.Contains(t, first.payload, "BEGIN 'snmp_refresh."+refreshTestTableKey("ifTraffic", "1")+"'")
	assert.Contains(t, first.payload, "SET 'ifTraffic' = 42")

	device.setText(snmputils.OidSysName, "after")
	*now = now.Add(time.Minute)
	sink.reject.Store(true)
	rejected := collect(2)
	assert.False(t, rejected.committed)
	assert.Contains(t, rejected.payload, "HOST_DEFINE '"+guid+"' 'after'")
	retried := collect(3)
	require.True(t, retried.committed)
	assert.Contains(t, retried.payload, "HOST_DEFINE '"+guid+"' 'after'")
	assert.Contains(t, retried.payload, "CLABEL 'sysName' 'after'")
	assert.Contains(t, retried.payload, "CLABEL 'snmp_host' 'after'")
	assert.Contains(t, retried.payload, "SET 'ifTraffic' = 42")
	assert.NotContains(t, retried.payload, "obsolete", "charts reported again are redefined, not retired")
	unchanged := collect(4)
	assert.True(t, unchanged.committed)
	assert.NotContains(t, unchanged.payload, "HOST_DEFINE")
	assert.NotContains(t, unchanged.payload, "CHART '")

	client, _ := newReplacementTestClient(t, device, nil)
	t.Cleanup(cleanup) // Close the installed client before its mock controller settles.
	c.newSnmpClient = func() gosnmp.Handler { return client }
	c.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector { return newReplacementTestDD(t, cfg) }
	device.values[snmputils.OidSysObject] = gosnmp.SnmpPDU{
		Name:  snmputils.OidSysObject,
		Type:  gosnmp.ObjectIdentifier,
		Value: replacementTestObjectID,
	}
	device.setText(snmputils.OidSysName, "replacement")
	*now = now.Add(time.Minute)
	sink.reject.Store(true)
	assert.False(t, collect(5).committed)
	replaced := collect(6)
	assert.True(t, replaced.committed)
	assert.Contains(t, replaced.payload, "HOST_DEFINE '"+guid+"' 'replacement'")
	assert.Contains(t, replaced.payload, "SET 'replacementLoad' = 7")
	assert.Contains(t, replaced.payload, "DIMENSION 'ifTraffic' 'ifTraffic' 'incremental' '1' '1' 'obsolete'")
	assert.Nil(t, c.Charts().Get("snmp_device_prof_refresh_stats_timings"), "accepted output prunes retired charts")
	require.Len(
		t,
		c.Charts().Get(refreshTestTableKey("ifTraffic", "1")).Dims,
		2,
		"accepted output prunes the retired dimension",
	)
}

func startRefreshOutputJob(t *testing.T, c *Collector) (*refreshOutputSink, func(int) refreshOutputFrame, func()) {
	t.Helper()
	sink := &refreshOutputSink{
		frames: make(chan refreshOutputFrame),
		done:   make(chan struct{}),
	}
	job := jobruntime.NewJob(jobruntime.JobConfig{
		PluginName:  "go.d",
		ModuleName:  "snmp",
		Name:        "refresh",
		FullName:    "snmp_refresh",
		Module:      c,
		Out:         sink,
		CleanupOut:  io.Discard,
		UpdateEvery: 1,
	})
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	job.Mute()
	run := jobruntime.NewManagedRun(context.Background(), nil)
	done := make(chan struct{})
	go func() { defer close(done); job.StartManaged(run) }()
	<-run.StartupDone()
	var stopped sync.Once
	cleanup := func() {
		stopped.Do(func() {
			close(sink.done) // Release output even when the test stopped receiving frames.
			run.Stop(nil)
			job.Stop()
			<-done
			job.Cleanup()
		})
	}
	t.Cleanup(cleanup)
	collect := func(tick int) refreshOutputFrame {
		t.Helper()
		var frame refreshOutputFrame
		require.Eventually(t, func() bool {
			job.Tick(tick)
			select {
			case frame = <-sink.frames:
				return true
			default:
				return false
			}
		}, 5*time.Second, time.Millisecond)
		return frame
	}
	return sink, collect, cleanup
}

func TestCollectorRuntimeReplacementDimensionsOutputRetries(t *testing.T) {
	c, device, now := newRefreshTestCollector(t)
	profile, key, outOID := compositeRefreshTestProfile(device, false)
	c.snmpProfiles = []*ddsnmp.Profile{profile}
	sink, collect, cleanup := startRefreshOutputJob(t, c)
	require.True(t, collect(1).committed)
	client, _ := newReplacementTestClient(t, device, nil)
	t.Cleanup(cleanup)
	c.newSnmpClient = func() gosnmp.Handler { return client }
	c.newDdSnmpColl = func(cfg ddsnmpcollector.Config) ddCollector {
		cfg.Profiles = []*ddsnmp.Profile{profile}
		return ddsnmpcollector.New(cfg)
	}
	device.omittedOID = outOID
	device.values[snmputils.OidSysObject] = gosnmp.SnmpPDU{
		Name:  snmputils.OidSysObject,
		Type:  gosnmp.ObjectIdentifier,
		Value: replacementTestObjectID,
	}
	*now = now.Add(time.Minute)
	sink.reject.Store(true)
	assert.False(t, collect(2).committed)
	replaced := collect(3)
	require.True(t, replaced.committed)
	assert.Contains(t, replaced.payload, "DIMENSION 'out' 'out' 'incremental' '1' '1' 'obsolete'")
	chart := c.Charts().Get(chartIDFromName(key))
	require.NotNil(t, chart)
	assert.Nil(
		t,
		chart.GetDim(metricIDFromName(key, "out")),
		"accepted replacement output prunes the incumbent dimension",
	)
	device.omittedOID = ""
	recovered := collect(4)
	require.True(t, recovered.committed)
	assert.Contains(t, recovered.payload, "DIMENSION 'out' 'out' 'incremental' '1' '1' ''")
	assert.Contains(t, recovered.payload, "SET 'out' = 34")
	assert.NotContains(t, recovered.payload, "obsolete")
	unchanged := collect(5)
	assert.True(t, unchanged.committed)
	assert.NotContains(t, unchanged.payload, "CHART '")
}
