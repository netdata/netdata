// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"github.com/stretchr/testify/require"
)

func TestDynCfgAddCommitsOrDisposesOneGraphTransaction(t *testing.T) {
	tests := map[string]struct {
		apply     bool
		wantGraph bool
	}{
		"apply response before notification": {
			apply: true, wantGraph: true,
		},
		"dispose preserves empty graph": {},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			controller, graph, supervisor, output, state := newDynCfgJobTestHarness(t)
			scope := lifecycle.ResourceTransactionScope{ID: "module_job"}
			request := DynCfgJobRequest{
				Args:         []string{"go.d:collector:module", "add", "job"},
				Payload:      []byte(`{"option":"value"}`),
				ContentType:  "application/json",
				CallerSource: "user=test",
				HasPayload:   true,
			}
			plan, err := lifecycle.NewResourceTransactionTaskPlan(
				lifecycle.SourceFunction,
				time.Time{},
				lifecycle.TransactionTaskPhases,
				nil,
				scope,
				func(
					ctx context.Context,
					current lifecycle.ReadyResource,
					taskScope lifecycle.ResourceTransactionScope,
					permit lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					return controller.Prepare(
						ctx,
						request,
						current,
						taskScope,
						permit,
					)
				},
			)
			require.NoError(t, err)
			ref := startDynCfgJobTestTask(t, supervisor, plan)
			first := <-supervisor.CompletionCh()
			require.False(t, first.Ref != ref ||
				first.Sequence != 1 ||
				first.Kind != lifecycle.TaskOutcomePreparedResourceTransaction ||
				first.Err != nil)

			if test.apply {
				require.NoError(t, supervisor.SendAction(
					lifecycle.TaskAction{
						Ref: ref, Sequence: 2,
						Kind: lifecycle.TaskActionApplyResourceTransaction,
					},
				),
				)

				second := <-supervisor.CompletionCh()
				require.False(t, second.Ref != ref ||
					second.Sequence != 2 ||
					second.Kind != lifecycle.TaskOutcomeAppliedResourceTransaction ||
					second.Err != nil)
				disposition, current, err :=
					supervisor.TakeAppliedResourceTransaction(ref, 2, scope)
				require.NoError(t, err)
				require.False(t, disposition != lifecycle.ResourceTransactionUnchanged || current != nil)

				_, preflightResultErr := supervisor.PreflightResult(ref, "add", 1)
				require.NoError(t, preflightResultErr)

				sendDynCfgJobTestAction(
					t,
					supervisor,
					lifecycle.TaskAction{
						Ref: ref, Sequence: 3,
						Kind: lifecycle.TaskActionEncodeWrite,
						UID:  "add", Expiry: 1,
					},
				)
				sendDynCfgJobTestAction(
					t,
					supervisor,
					lifecycle.TaskAction{
						Ref: ref, Sequence: 4,
						Kind: lifecycle.TaskActionCleanup,
					},
				)
				sendDynCfgJobTestAction(
					t,
					supervisor,
					lifecycle.TaskAction{
						Ref: ref, Sequence: 5,
						Kind: lifecycle.TaskActionTerminate,
					},
				)
			} else {
				sendDynCfgJobTestAction(
					t,
					supervisor,
					lifecycle.TaskAction{
						Ref: ref, Sequence: 2,
						Kind: lifecycle.TaskActionDispose,
					},
				)
				current, err := supervisor.TakeDisposedResourceTransaction(ref, 2, scope)
				require.NoError(t, err)
				require.Nil(t, current)
				sendDynCfgJobTestAction(
					t,
					supervisor,
					lifecycle.TaskAction{
						Ref: ref, Sequence: 3,
						Kind: lifecycle.TaskActionTerminate,
					},
				)
			}

			require.NoError(t, supervisor.Release(ref))

			record, exists := graph.Lookup("module_job")
			require.EqualValues(t, test.wantGraph, exists)
			require.False(t, exists && record.Status != dyncfg.StatusAccepted.String())
			require.EqualValues(t, 1, state.collectorCleanup)
			wire := output.String()
			if !test.apply {
				require.EqualValues(t, "", wire)
				return
			}
			resultAt := strings.Index(wire, "FUNCTION_RESULT_BEGIN add 202 application/json 1\n")
			notificationAt := strings.Index(wire, "CONFIG go.d:collector:module:job create accepted job")
			require.False(t, resultAt < 0 || notificationAt < 0 || resultAt >= notificationAt)
		})
	}
}

func TestResourceOnlyTransactionReplacesWithoutGraphMutation(t *testing.T) {
	events := []string{}
	currentIdentity := lifecycle.ResourceIdentity{
		ID: "job", Generation: 1,
	}
	successorIdentity := lifecycle.ResourceIdentity{
		ID: "job", Generation: 2,
	}
	current := &transactionTestReadyResource{
		identity: currentIdentity, prefix: "current", events: &events,
	}
	successorReady := &transactionTestReadyResource{
		identity: successorIdentity, prefix: "successor", events: &events,
	}
	successor := &transactionTestPreparedResource{
		identity: successorIdentity, ready: successorReady, events: &events,
	}
	result, err := lifecycle.NewSealedResult(200, "application/json", []byte(`{"status":200,"message":""}`))
	require.NoError(t, err)
	transaction, err := PrepareResourceTransaction(
		ResourceTransactionSpec{
			Scope: lifecycle.ResourceTransactionScope{
				ID: "job", Current: currentIdentity,
				Successor: successorIdentity,
			},
			Disposition: lifecycle.ResourceTransactionReplaced,
			Current:     current, Successor: successor,
			Result: result, Cleanup: func() error { return nil },
		},
	)
	require.NoError(t, err)

	_, applyErr := transaction.Apply(context.Background())
	require.NoError(t, applyErr)

	want := []string{
		"current-stop",
		"current-finalize",
		"successor-accept",
		"successor-publish",
	}
	require.EqualValues(t, strings.Join(want, ","), strings.Join(events, ","))
}

func newDynCfgJobTestHarness(
	t *testing.T,
) (
	*DynCfgJobController,
	*dyncfg.Graph,
	*lifecycle.TaskSupervisor,
	*bytes.Buffer,
	*factoryTestState,
) {
	t.Helper()
	output := &bytes.Buffer{}
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	supervisor, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	state := &factoryTestState{}
	modules := collectorapi.Registry{
		"module": {
			Create: func() collectorapi.CollectorV1 {
				return state.module(nil, false)
			},
			Config: func() any {
				return &collectorapi.MockConfiguration{}
			},
			JobConfigSchema: collectorapi.MockConfigSchema,
		},
	}
	resolver, err := secretresolver.NewAtomicResolver(nil)
	require.NoError(t, err)
	factory, err := NewFactory(
		FactoryConfig{
			PluginName: "go.d", Modules: modules,
			Tasks: supervisor, Frames: frames,
			Resolver: resolver, StoreScope: unavailableStoreScope,
			Vnodes:    vnoderegistry.New(),
			Scheduler: newTestScheduler(t),
		},
	)
	require.NoError(t, err)
	configModules, err := NewConfigModuleFactory(
		ConfigModuleFactoryConfig{
			Modules: modules, Resolver: resolver,
			StoreScope: unavailableStoreScope,
		},
	)
	require.NoError(t, err)
	graph, err := dyncfg.NewGraph(nil)
	require.NoError(t, err)
	controller, err := NewDynCfgJobController(
		DynCfgJobControllerConfig{
			PluginName: "go.d", Modules: modules,
			Defaults: confgroup.Registry{
				"module": {
					UpdateEvery: 1,
				},
			},
			Factory: factory, ConfigModules: configModules,
			Graph: graph, Frames: frames,
		},
	)
	require.NoError(t, err)
	return controller, graph, supervisor, output, state
}

func startDynCfgJobTestTask(
	t *testing.T,
	supervisor *lifecycle.TaskSupervisor,
	plan lifecycle.TaskPlan,
) lifecycle.TaskRef {
	t.Helper()
	request, err := supervisor.Enqueue(lifecycle.TaskClassFrameworkControl, plan)
	require.NoError(t, err)
	var starts [lifecycle.TaskStartServiceQuantum]lifecycle.TaskStart
	count, _, err := supervisor.Dispatch(context.Background(), 1, &starts)
	require.NoError(t, err)
	require.False(t, count != 1 || starts[0].Request != request || starts[0].Err != nil)
	return starts[0].Task
}

func sendDynCfgJobTestAction(
	t *testing.T,
	supervisor *lifecycle.TaskSupervisor,
	action lifecycle.TaskAction,
) {
	t.Helper()

	require.NoError(t, supervisor.SendAction(action))

	ack := <-supervisor.AcknowledgementCh()
	require.False(t, ack.Ref != action.Ref || ack.Sequence != action.Sequence || ack.Kind != action.Kind || ack.Err != nil)
}
