// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

func TestVNodeDependencyNotificationObservesCommittedState(t *testing.T) {
	jobs := testRunJobServices(t)
	secretConfig := testRunSecrets(t)
	acquirer := controlledAcquirer{attempts: make(chan acquisitionAttempt, 4)}
	jobs.SNMPVnodeAcquirer = acquirer
	output := newProcessSynchronizedBuffer()
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Secrets:    secretConfig,
		Generation: 1, ShutdownTimeout: time.Second, UIDs: lifecycle.NewUIDLedger(), Frames: frames,
		Modules: collectorapi.Registry{}, Jobs: jobs, Discovery: testRunDiscoveryServices(t),
	})
	require.NoError(t, err)
	notifications := make(chan jobruntime.VnodeSnapshot, 8)
	forward := generation.vnodes.dependencyChanged
	generation.vnodes.dependencyChanged = func(name string) {
		snapshot, ok := generation.vnodeConfig.Lookup(name)
		if !ok {
			t.Error("notification preceded vnode commit")
		}
		select {
		case notifications <- snapshot:
		default:
			t.Error("unexpected duplicate notifications")
		}
		forward(name)
	}
	require.NoError(t, generation.start(context.Background()))
	t.Cleanup(func() {
		generation.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		require.NoError(t, generation.Wait(ctx))
	})
	next := func() jobruntime.VnodeSnapshot {
		t.Helper()
		select {
		case snapshot := <-notifications:
			return snapshot
		case <-time.After(3 * time.Second):
			t.Fatal("committed vnode did not notify dependents")
			return jobruntime.VnodeSnapshot{}
		}
	}
	submit := func(uid string, args []string, payload string) {
		t.Helper()
		require.NoError(t, generation.kernel.Submit(t.Context(), jobmgr.Request{
			UID: uid, Route: "config", Source: lifecycle.SourceFunction, Args: args,
			HasPayload: true, Payload: []byte(payload), ContentType: "application/json", CallerSource: "user=test",
		}))
		output.waitContains(t, "FUNCTION_RESULT_BEGIN "+uid+" ")
	}
	submit("vnode-add", []string{"go.d:vnode", "add", "router"},
		`{"mode":"snmp","mode_snmp":{"address":"device","credentials":{"community":"fixture"}}}`)
	require.Nil(t, next().Vnode, "new SNMP vnode is committed before acquisition")
	attempt := nextAcquisition(t, acquirer)
	attempt.reply <- acquisitionReply{metadata: &vnodes.Metadata{Hostname: "acquired"}}
	require.Equal(t, "acquired", next().Vnode.Hostname)
	submit("vnode-update", []string{"go.d:vnode:router", "update"},
		`{"mode":"snmp","mode_snmp":{"address":"device","credentials":{"community":"fixture"}},"labels":{"site":"new"}}`)
	require.Equal(t, "new", next().Vnode.Labels["site"])
	submit("vnode-rejected", []string{"go.d:vnode:router", "update"},
		`{"mode":"snmp","mode_snmp":{"address":"different","credentials":{"community":"fixture"}}}`)
	require.Contains(t, output.String(), "FUNCTION_RESULT_BEGIN vnode-rejected 400 application/json")
	require.Empty(t, notifications, "rejected vnode proposal must not notify dependents")
}
