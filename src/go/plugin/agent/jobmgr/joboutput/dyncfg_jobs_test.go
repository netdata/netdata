// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
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
			if err != nil {
				t.Fatal(err)
			}
			ref := startDynCfgJobTestTask(t, supervisor, plan)
			first := <-supervisor.CompletionCh()
			if first.Ref != ref ||
				first.Sequence != 1 ||
				first.Kind != lifecycle.TaskOutcomePreparedResourceTransaction ||
				first.Err != nil {
				t.Fatalf("initial completion=%+v", first)
			}

			if test.apply {
				if err := supervisor.SendAction(
					lifecycle.TaskAction{
						Ref: ref, Sequence: 2,
						Kind: lifecycle.TaskActionApplyResourceTransaction,
					},
				); err != nil {
					t.Fatal(err)
				}
				second := <-supervisor.CompletionCh()
				if second.Ref != ref ||
					second.Sequence != 2 ||
					second.Kind != lifecycle.TaskOutcomeAppliedResourceTransaction ||
					second.Err != nil {
					t.Fatalf("apply completion=%+v", second)
				}
				disposition, current, err :=
					supervisor.TakeAppliedResourceTransaction(
						ref,
						2,
						scope,
					)
				if err != nil {
					t.Fatal(err)
				}
				if disposition != lifecycle.ResourceTransactionUnchanged ||
					current != nil {
					t.Fatalf(
						"disposition=%v current=%T",
						disposition,
						current,
					)
				}
				if _, err := supervisor.PreflightResult(
					ref,
					"add",
					1,
				); err != nil {
					t.Fatal(err)
				}
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
				current, err := supervisor.TakeDisposedResourceTransaction(
					ref,
					2,
					scope,
				)
				if err != nil {
					t.Fatal(err)
				}
				if current != nil {
					t.Fatalf("disposed graph-only transaction returned %T", current)
				}
				sendDynCfgJobTestAction(
					t,
					supervisor,
					lifecycle.TaskAction{
						Ref: ref, Sequence: 3,
						Kind: lifecycle.TaskActionTerminate,
					},
				)
			}
			if err := supervisor.Release(ref); err != nil {
				t.Fatal(err)
			}

			record, exists := graph.Lookup("module_job")
			if exists != test.wantGraph {
				t.Fatalf(
					"graph record exists=%v want=%v record=%+v",
					exists,
					test.wantGraph,
					record,
				)
			}
			if exists &&
				record.Status != dyncfg.StatusAccepted.String() {
				t.Fatalf("graph status=%q", record.Status)
			}
			if state.collectorCleanup != 1 {
				t.Fatalf(
					"validation cleanup calls=%d want=1",
					state.collectorCleanup,
				)
			}
			wire := output.String()
			if !test.apply {
				if wire != "" {
					t.Fatalf("disposed transaction emitted %q", wire)
				}
				return
			}
			resultAt := strings.Index(
				wire,
				"FUNCTION_RESULT_BEGIN add 202 application/json 1\n",
			)
			notificationAt := strings.Index(
				wire,
				"CONFIG go.d:collector:module:job create accepted job",
			)
			if resultAt < 0 ||
				notificationAt < 0 ||
				resultAt >= notificationAt {
				t.Fatalf(
					"response/notification order is wrong: %q",
					wire,
				)
			}
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
	result, err := lifecycle.NewSealedResult(
		200,
		"application/json",
		[]byte(`{"status":200,"message":""}`),
	)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"current-stop",
		"current-finalize",
		"successor-accept",
		"successor-publish",
	}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("events=%v want=%v", events, want)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := lifecycle.NewTaskSupervisor(frames)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	stores := secretstore.NewService()
	factory, err := NewFactory(
		FactoryConfig{
			PluginName: "go.d", Modules: modules,
			Tasks: supervisor, Frames: frames,
			Resolver: resolver, Stores: stores,
			Vnodes:    vnoderegistry.New(),
			Scheduler: newTestScheduler(t),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	configModules, err := NewConfigModuleFactory(
		ConfigModuleFactoryConfig{
			Modules: modules, Resolver: resolver, Stores: stores,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := dyncfg.NewGraph(nil)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	return controller, graph, supervisor, output, state
}

func startDynCfgJobTestTask(
	t *testing.T,
	supervisor *lifecycle.TaskSupervisor,
	plan lifecycle.TaskPlan,
) lifecycle.TaskRef {
	t.Helper()
	request, err := supervisor.Enqueue(
		lifecycle.TaskClassFrameworkControl,
		plan,
	)
	if err != nil {
		t.Fatal(err)
	}
	var starts [lifecycle.TransientTaskSlots]lifecycle.TaskStart
	count, _, err := supervisor.Dispatch(
		context.Background(),
		1,
		&starts,
	)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 ||
		starts[0].Request != request ||
		starts[0].Err != nil {
		t.Fatalf(
			"task start count=%d start=%+v",
			count,
			starts[0],
		)
	}
	return starts[0].Task
}

func sendDynCfgJobTestAction(
	t *testing.T,
	supervisor *lifecycle.TaskSupervisor,
	action lifecycle.TaskAction,
) {
	t.Helper()
	if err := supervisor.SendAction(action); err != nil {
		t.Fatal(err)
	}
	ack := <-supervisor.AcknowledgementCh()
	if ack.Ref != action.Ref ||
		ack.Sequence != action.Sequence ||
		ack.Kind != action.Kind ||
		ack.Err != nil {
		t.Fatalf("action acknowledgement=%+v", ack)
	}
}
