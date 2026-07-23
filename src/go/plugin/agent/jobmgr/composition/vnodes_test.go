// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/discovery"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/stretchr/testify/require"
)

const testVNodeGUID = "11111111-1111-1111-1111-111111111111"

func TestVNodeBindingPreparesUpdates(t *testing.T) {
	tests := map[string]struct {
		args         []string
		payload      string
		wantMutation bool
		wantHostname string
	}{
		"invalid config ID": {
			args:    []string{"other:vnode:db", string(dyncfg.CommandUpdate)},
			payload: `{"hostname":"changed","guid":"` + testVNodeGUID + `"}`,
		},
		"unknown vnode": {
			args:    []string{"go.d:vnode:missing", string(dyncfg.CommandUpdate)},
			payload: `{"hostname":"changed","guid":"` + testVNodeGUID + `"}`,
		},
		"invalid payload": {args: []string{"go.d:vnode:db", string(dyncfg.CommandUpdate)}, payload: `{`},
		"unchanged vnode": {
			args:    []string{"go.d:vnode:db", string(dyncfg.CommandUpdate)},
			payload: `{"hostname":"db","guid":"` + testVNodeGUID + `"}`,
		},
		"updated vnode": {
			args:         []string{"go.d:vnode:db", string(dyncfg.CommandUpdate)},
			payload:      `{"hostname":"changed","guid":"` + testVNodeGUID + `"}`,
			wantMutation: true,
			wantHostname: "changed",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			binding, configured := newTestVNodeBinding(t, confgroup.TypeDyncfg, nil)
			target := "db"
			if test.args[0] == "go.d:vnode:missing" {
				target = "missing"
			}
			transaction, err := binding.prepare(
				context.Background(),
				functionadapter.HandlerInput{
					Args:         test.args,
					Payload:      []byte(test.payload),
					ContentType:  "application/json",
					CallerSource: "user=test",
					HasPayload:   true,
				},
				nil,
				lifecycle.ResourceTransactionScope{
					ID: "vnode:" + target,
				},
				lifecycle.LongLivedPermit{},
			)
			require.NoError(t, err)

			if test.wantMutation {
				require.IsType(t, &preparedVNodeTransaction{}, transaction)
			} else {
				require.IsType(t, &joboutput.PreparedNoopResourceTransaction{}, transaction)
			}
			_, err = transaction.Apply(context.Background())
			require.NoError(t, err)

			snapshot, ok := configured.Lookup("db")
			require.True(t, ok)
			if test.wantMutation {
				require.Equal(t, test.wantHostname, snapshot.Vnode.Hostname)
				require.EqualValues(t, 2, snapshot.Revision)
			} else {
				require.Equal(t, "db", snapshot.Vnode.Hostname)
				require.EqualValues(t, 1, snapshot.Revision)
			}
		})
	}
}

func TestVNodeBindingPreparesRemovals(t *testing.T) {
	tests := map[string]struct {
		args         []string
		sourceType   string
		graphConfigs []dyncfg.GraphConfig
		wantRemoval  bool
	}{
		"invalid config ID": {
			args:       []string{"other:vnode:db", string(dyncfg.CommandRemove)},
			sourceType: confgroup.TypeDyncfg,
		},
		"unknown vnode": {
			args:       []string{"go.d:vnode:missing", string(dyncfg.CommandRemove)},
			sourceType: confgroup.TypeDyncfg,
		},
		"non-dyncfg vnode": {
			args:       []string{"go.d:vnode:db", string(dyncfg.CommandRemove)},
			sourceType: confgroup.TypeUser,
		},
		"referenced vnode": {
			args:       []string{"go.d:vnode:db", string(dyncfg.CommandRemove)},
			sourceType: confgroup.TypeDyncfg,
			graphConfigs: []dyncfg.GraphConfig{{
				ID: "module_job", Module: "module", Name: "job",
				Payload: []byte("module: module\nname: job\nvnode: db\n"),
			}},
		},
		"removed vnode": {
			args:        []string{"go.d:vnode:db", string(dyncfg.CommandRemove)},
			sourceType:  confgroup.TypeDyncfg,
			wantRemoval: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			binding, configured := newTestVNodeBinding(t, test.sourceType, test.graphConfigs)
			target := "db"
			if test.args[0] == "go.d:vnode:missing" {
				target = "missing"
			}
			transaction, err := binding.prepare(
				context.Background(),
				functionadapter.HandlerInput{
					Args: test.args,
				},
				nil,
				lifecycle.ResourceTransactionScope{
					ID: "vnode:" + target,
				},
				lifecycle.LongLivedPermit{},
			)
			require.NoError(t, err)

			if test.wantRemoval {
				require.IsType(t, &preparedVNodeTransaction{}, transaction)
			} else {
				require.IsType(t, &joboutput.PreparedNoopResourceTransaction{}, transaction)
			}
			_, err = transaction.Apply(context.Background())
			require.NoError(t, err)

			_, exists := configured.Lookup("db")
			require.Equal(t, !test.wantRemoval, exists)
		})
	}
}

func TestVNodeBindingRejectsOwnedTransactionScope(t *testing.T) {
	binding, _ := newTestVNodeBinding(t, confgroup.TypeDyncfg, nil)
	_, err := binding.prepare(
		context.Background(),
		functionadapter.HandlerInput{
			Args: []string{"go.d:vnode:db", string(dyncfg.CommandUpdate)},
		},
		nil,
		lifecycle.ResourceTransactionScope{
			ID: "vnode:db",
			Current: lifecycle.ResourceIdentity{
				ID:         "vnode:db",
				Generation: 1,
			},
		},
		lifecycle.LongLivedPermit{},
	)
	require.EqualError(t, err, "jobmgr composition: invalid vnode transaction scope")
}

func TestVNodeDiagnosticsFollowAppliedCommandWithoutPayload(t *testing.T) {
	const payloadSentinel = "vnode-payload-must-not-appear"
	diagnostics := &recordingCompositionDiagnosticObserver{}
	binding, _ := newTestVNodeBindingWithDiagnostics(t, confgroup.TypeDyncfg, nil, diagnostics)
	transaction, err := binding.prepare(
		context.Background(),
		functionadapter.HandlerInput{
			Args:         []string{"go.d:vnode:db", string(dyncfg.CommandUpdate)},
			Payload:      []byte(`{"hostname":"` + payloadSentinel + `","guid":"` + testVNodeGUID + `"}`),
			ContentType:  "application/json",
			CallerSource: "user=test",
			HasPayload:   true,
		},
		nil,
		lifecycle.ResourceTransactionScope{
			ID: "vnode:db",
		},
		lifecycle.LongLivedPermit{},
	)
	require.NoError(t, err)
	_, err = transaction.Apply(context.Background())
	require.NoError(t, err)

	events := diagnostics.snapshot()
	var completed *jobmgr.DiagnosticEvent
	for _, event := range events {
		if event.Name == "vnode configuration command completed" {
			completed = &event
			break
		}
	}
	require.NotNil(t, completed)
	require.Equal(t, jobmgr.DiagnosticInfo, completed.Level)
	require.Equal(t, "vnode:db", completed.Resource)
	require.Equal(t, string(dyncfg.CommandUpdate), completed.Command)
	require.Equal(t, "configured", completed.State)
	require.EqualValues(t, 1, completed.Generation)
	require.Equal(t, 202, completed.ResultStatus)
	require.NotContains(t, fmt.Sprintf("%+v", events), payloadSentinel)
}

func newTestVNodeBinding(
	t *testing.T,
	sourceType string,
	graphConfigs []dyncfg.GraphConfig,
) (*vnodeBinding, *agentdiscovery.VNodeConfiguration) {
	return newTestVNodeBindingWithDiagnostics(t, sourceType, graphConfigs, nil)
}

func newTestVNodeBindingWithDiagnostics(
	t *testing.T,
	sourceType string,
	graphConfigs []dyncfg.GraphConfig,
	diagnostics jobmgr.DiagnosticObserver,
) (*vnodeBinding, *agentdiscovery.VNodeConfiguration) {
	t.Helper()
	source := "user=test"
	if sourceType != confgroup.TypeDyncfg {
		source = "file=test"
	}
	configured, err := agentdiscovery.NewVNodeConfigurationWithInitial(
		map[string]*vnodes.VirtualNode{
			"db": {Name: "db", Hostname: "db", GUID: testVNodeGUID, Source: source, SourceType: sourceType},
		},
	)
	require.NoError(t, err)
	graph, err := dyncfg.NewGraph(graphConfigs)
	require.NoError(t, err)
	frames, err := lifecycle.NewFrameOwner(io.Discard)
	require.NoError(t, err)
	binding, err := newVNodeBinding(1, "go.d", frames, configured, graph, diagnostics)
	require.NoError(t, err)
	return binding, configured
}
