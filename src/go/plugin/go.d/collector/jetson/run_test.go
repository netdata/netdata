// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package jetson

import (
	"bytes"
	"context"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

func TestCollectionTransitions(t *testing.T) {
	c, fake := newCollectorWithFake(t)
	startRun(t, c)
	conn := fake.accept(t)
	_, err := collecttest.CollectScalarSeries(c)
	require.ErrorContains(t, err, "no fresh")

	sendRecord(t, conn, "GR3D_FREQ 25%@900 EMC_FREQ 30%@1600")
	waitSample(t, c, func(s sample) bool { return s.GPUFrequency != nil })
	want := map[string]metrix.SampleValue{
		"gpu_utilization": 25,
		"gpu_frequency":   900,
		"emc_utilization": 30,
		"emc_frequency":   1600,
	}
	assertCollected(t, c, want)
	assertCollected(t, c, want) // publication can reuse a fresh record

	sendRecord(t, conn, "GR3D_FREQ @[300,off,600] EMC_FREQ 0%")
	waitSample(t, c, func(s sample) bool { return len(s.GPCFrequencies) == 3 })
	assertCollected(t, c, map[string]metrix.SampleValue{
		`gpu_gpc_frequency{gpc="0"}`: 300,
		`gpu_gpc_frequency{gpc="2"}`: 600,
		"emc_utilization":            0,
	}) // no retained scalar or utilization from the previous record
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})

	sendRecord(t, conn, "TEMP@42C")
	waitSample(t, c, func(s sample) bool { return !s.hasReadings() })
	_, err = collecttest.CollectScalarSeries(c)
	require.ErrorContains(t, err, "no supported")

	sendRecord(t, conn, "GR3D_FREQ 0%@0 EMC_FREQ 0%@0")
	waitSample(t, c, func(s sample) bool { return s.GPUFrequency != nil })
	assertCollected(t, c, map[string]metrix.SampleValue{
		"gpu_utilization": 0,
		"gpu_frequency":   0,
		"emc_utilization": 0,
		"emc_frequency":   0,
	})
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
}

func TestSourceRecovery(t *testing.T) {
	for name, tc := range map[string]struct {
		fail    func(*testing.T, net.Conn)
		silence bool
	}{
		"nonzero exit": {fail: func(t *testing.T, conn net.Conn) {
			code := 7
			sendCommand(t, conn, fakeCommand{
				Exit: &code,
			})
		}},
		"output EOF": {fail: func(t *testing.T, conn net.Conn) { require.NoError(t, conn.Close()) }},
		"silence":    {silence: true},
		"unrecognized output": {silence: true, fail: func(t *testing.T, conn net.Conn) {
			sendCommand(t, conn, fakeCommand{
				Line: "warning GR3D_FREQ 90%",
			})
		}},
	} {
		t.Run(name, func(t *testing.T) {
			c, fake := newCollectorWithFake(t)
			c.timing = sourceTiming{
				maxSampleAge:    100 * time.Millisecond,
				stallTimeout:    300 * time.Millisecond,
				restartDelayMin: 100 * time.Millisecond,
				restartDelayMax: 200 * time.Millisecond,
			}
			startRun(t, c)
			conn := fake.accept(t)
			sendRecord(t, conn, "GR3D_FREQ 50%")
			waitSample(t, c, func(s sample) bool { return s.GPUUtilization != nil })
			_, err := collecttest.CollectScalarSeries(c)
			require.NoError(t, err)

			if tc.fail != nil {
				tc.fail(t, conn)
			}
			if tc.silence {
				require.Eventually(t, func() bool {
					_, err := collecttest.CollectScalarSeries(c)
					return err != nil
				}, time.Second, 5*time.Millisecond)
			} else {
				require.Eventually(t, func() bool { return c.latest.Load() == nil }, time.Second, 5*time.Millisecond)
			}

			replacement := fake.accept(t)
			sendRecord(t, replacement, "EMC_FREQ 12%@800")
			waitSample(t, c, func(s sample) bool { return s.EMCFrequency != nil })
			assertCollected(t, c, map[string]metrix.SampleValue{
				"emc_utilization": 12,
				"emc_frequency":   800,
			})
		})
	}
}

func TestCancelTerminatesDescendants(t *testing.T) {
	c, fake := newCollectorWithFake(t)
	stop := startRun(t, c)
	conn := fake.accept(t)
	sendCommand(t, conn, fakeCommand{
		Spawn: true,
	})
	child := fake.accept(t)
	stop()
	for _, connection := range []net.Conn{conn, child} {
		require.NoError(t, connection.SetReadDeadline(time.Now().Add(5*time.Second)))
		_, err := connection.Read(make([]byte, 1))
		require.ErrorIs(t, err, io.EOF) // every owned process closed its socket
	}
}

func TestManagedRuntime(t *testing.T) {
	c, fake := newCollectorWithFake(t)
	out := &lockedBuffer{}
	job := jobruntime.NewJobV2(jobruntime.JobV2Config{
		PluginName:  "go.d",
		Name:        "localhost",
		ModuleName:  "jetson",
		FullName:    "jetson_localhost",
		Module:      c,
		Out:         out,
		UpdateEvery: 1,
	})
	require.NoError(t, job.AutoDetectionManaged(t.Context()))
	run := jobruntime.NewManagedRun(t.Context(), nil)
	done := make(chan struct{})
	go func() { defer close(done); job.StartManaged(run) }()
	t.Cleanup(func() {
		run.Stop(nil)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("managed job failed to stop")
		}
	})
	select {
	case <-run.StartupDone():
	case <-time.After(5 * time.Second):
		t.Fatal("managed startup blocked")
	}
	require.NoError(t, run.StartupErr())
	require.True(t, run.Running())

	conn := fake.accept(t)
	sendRecord(t, conn, "GR3D_FREQ 25%@900 EMC_FREQ 30%@1600")
	waitSample(t, c, func(s sample) bool { return s.GPUFrequency != nil })
	require.Eventually(t, func() bool {
		job.Tick(1)
		return strings.Contains(out.String(), "jetson.gpu_utilization")
	}, 3*time.Second, 20*time.Millisecond)
	assert.Contains(t, out.String(), "SET 'utilization' = 25")

	run.Stop(nil)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("managed Run not joined")
	}
	require.Nil(t, c.latest.Load())
	assert.False(t, run.Running())
}

func TestStartupAndLateExecFailure(t *testing.T) {
	for name, tc := range map[string]struct{ missingWrapper bool }{
		"helper acquisition fails":              {missingWrapper: true},
		"target exec fails after helper starts": {},
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := newCollectorWithFake(t)
			c.findTegrastats = func() (string, error) { return filepath.Join(t.TempDir(), "missing-tegrastats"), nil }
			require.NoError(t, c.Check(t.Context()))
			if tc.missingWrapper {
				t.Cleanup(ndexec.SetRunnerPathsForTests(filepath.Join(t.TempDir(), "missing-wrapper"), ""))
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			ready := make(chan struct{})
			done := make(chan error, 1)
			go func() { done <- c.Run(ctx, func() { close(ready) }) }()

			if tc.missingWrapper {
				select {
				case err := <-done:
					require.ErrorContains(t, err, "start tegrastats")
				case <-time.After(3 * time.Second):
					t.Fatal("startup failure not returned")
				}
				select {
				case <-ready:
					t.Fatal("ready before acquisition")
				default:
				}
				return
			}

			select {
			case <-ready:
			case <-time.After(3 * time.Second):
				t.Fatal("helper did not signal readiness")
			}
			_, err := collecttest.CollectScalarSeries(c)
			require.Error(t, err)
			select {
			case err := <-done:
				t.Fatalf("late exec failure ended Run: %v", err)
			case <-time.After(150 * time.Millisecond):
			}
			cancel()
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(3 * time.Second):
				t.Fatal("retry did not cancel")
			}
		})
	}
}

func assertCollected(t *testing.T, c *Collector, want map[string]metrix.SampleValue) {
	t.Helper()
	got, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
