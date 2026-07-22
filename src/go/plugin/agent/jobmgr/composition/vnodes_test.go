// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"io"
	"testing"

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

func newTestVNodeBinding(
	t *testing.T,
	sourceType string,
	graphConfigs []dyncfg.GraphConfig,
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
	binding, err := newVNodeBinding(1, "go.d", frames, configured, graph)
	require.NoError(t, err)
	return binding, configured
}
