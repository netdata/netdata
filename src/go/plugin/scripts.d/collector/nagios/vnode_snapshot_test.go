// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"context"
	"maps"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNamedVnodeSnapshotMatchesRunningCheck(t *testing.T) {
	initial := &vnodes.VirtualNode{
		Name:     "router",
		Hostname: "old",
		GUID:     "11111111-1111-1111-1111-111111111111",
		Labels:   map[string]string{"_address": "192.0.2.1", "_alias": "old-alias", "site": "old", "removed": "value"},
	}
	configuration, err := discovery.NewVNodeConfigurationWithInitial(map[string]*vnodes.Config{"router": &vnodes.Config{VirtualNode: *initial}})
	require.NoError(t, err)
	publisher := hostoutput.New()
	publisher.Bind(configuration.Definition)
	out := &lockedBuffer{}
	frames, err := lifecycle.NewFrameOwner(out)
	require.NoError(t, err)
	var now atomic.Int64
	now.Store(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix())
	runner := &snapshotCheckRunner{
		requests: make(chan map[string]string, 4),
		release:  make(chan struct{}, 4),
	}
	coll := newTestCollector()
	coll.runner = runner
	coll.now = func() time.Time { return time.Unix(now.Load(), 0) }
	coll.UpdateEvery = 1
	coll.JobConfig = JobConfig{
		Name:          "check",
		Vnode:         "router",
		Plugin:        writeTestPluginFile(t, "true"),
		CheckInterval: confDuration(time.Second),
	}
	job := jobruntime.NewJobV2(
		jobruntime.JobV2Config{
			PluginName: "scripts.d",
			Name:       "check",
			ModuleName: "nagios",
			FullName:   "nagios_check",
			Module:     coll,
			Out: joboutput.FrameWriter{
				Owner: frames,
			},
			Publication:           publisher,
			Vnode:                 *initial,
			VnodeName:             "router",
			VnodeRevision:         1,
			VnodeMetadataRevision: 1,
			VnodeLookup:           configuration.Lookup,
			UpdateEvery:           1,
		},
	)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	ready, done := make(chan struct{}), make(chan struct{})
	go func() { defer close(done); job.StartManaged(ready) }()
	<-ready
	t.Cleanup(func() { job.Stop(); <-done; job.Cleanup() })
	tickJobUntil(t, job, func() bool { return len(runner.requests) > 0 }, "first check did not start")
	first := receiveSnapshotMacros(t, runner.requests)
	updated := initial.Copy()
	updated.GUID = "22222222-2222-2222-2222-222222222222"
	updated.Hostname = "new"
	updated.Labels = map[string]string{"_address": "192.0.2.2", "_alias": "new-alias", "site": "new", "added": "value"}
	prepared, err := configuration.PrepareUpsert("router", 1, &vnodes.Config{VirtualNode: *updated})
	require.NoError(t, err)
	_, err = prepared.Commit()
	require.NoError(t, err)
	runner.release <- struct{}{}
	require.Eventually(t, func() bool { return out.Len() > 0 }, 2*time.Second, time.Millisecond)
	assert.Contains(t, out.String(), "HOST '"+initial.GUID+"'")
	assert.NotContains(t, out.String(), "HOST '"+updated.GUID+"'")
	now.Add(2)
	tickJobUntil(t, job, func() bool { return len(runner.requests) > 0 }, "second check did not start")
	second := receiveSnapshotMacros(t, runner.requests)
	runner.release <- struct{}{}
	require.Eventually(
		t,
		func() bool { return strings.Contains(out.String(), "HOST '"+updated.GUID+"'") },
		2*time.Second,
		time.Millisecond,
	)
	assert.Equal(
		t,
		map[string]string{
			"NAGIOS_HOSTNAME":           "router",
			"NAGIOS_HOSTADDRESS":        "192.0.2.1",
			"NAGIOS_HOSTALIAS":          "old-alias",
			"NAGIOS__HOSTLABEL_SITE":    "old",
			"NAGIOS__HOSTLABEL_REMOVED": "value",
		},
		first,
	)
	assert.Equal(
		t,
		map[string]string{
			"NAGIOS_HOSTNAME":         "router",
			"NAGIOS_HOSTADDRESS":      "192.0.2.2",
			"NAGIOS_HOSTALIAS":        "new-alias",
			"NAGIOS__HOSTLABEL_SITE":  "new",
			"NAGIOS__HOSTLABEL_ADDED": "value",
		},
		second,
	)
}

type snapshotCheckRunner struct {
	requests chan map[string]string
	release  chan struct{}
}

func (r *snapshotCheckRunner) Run(ctx context.Context, request checkRunRequest) (checkRunResult, error) {
	macros := buildMacroSet(request.Job, request.Vnode, request.MacroState, request.Now)
	selected := maps.Clone(macros.Env)
	for key := range selected {
		switch key {
		case "NAGIOS_HOSTNAME",
			"NAGIOS_HOSTADDRESS",
			"NAGIOS_HOSTALIAS",
			"NAGIOS__HOSTLABEL_SITE",
			"NAGIOS__HOSTLABEL_REMOVED",
			"NAGIOS__HOSTLABEL_ADDED":
		default:
			delete(selected, key)
		}
	}
	select {
	case r.requests <- selected:
	case <-ctx.Done():
		return checkRunResult{}, ctx.Err()
	}
	select {
	case <-r.release:
	case <-ctx.Done():
		return checkRunResult{}, ctx.Err()
	}
	return checkRunResult{
		ServiceState: "OK",
		JobState:     "OK",
		Parsed: output.ParsedOutput{
			Perfdata: []output.PerfDatum{{Label: "requests", Value: 1}},
		},
	}, nil
}

func receiveSnapshotMacros(t *testing.T, ch <-chan map[string]string) map[string]string {
	t.Helper()
	select {
	case snapshot := <-ch:
		return snapshot
	case <-time.After(2 * time.Second):
		t.Fatal("check did not start")
		return nil
	}
}
