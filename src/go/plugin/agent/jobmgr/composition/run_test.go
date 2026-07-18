// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnoderegistry"
	"gopkg.in/yaml.v2"
)

func TestRunGenerationPreservesFullJobCapacityWithDiscoveryPipeline(
	t *testing.T,
) {
	tests := map[string]struct {
		jobs int
	}{
		"one pipeline plus the full job population": {
			jobs: lifecycle.MaximumActiveJobs,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var cleanupCalls atomic.Int32
			modules := collectorapi.Registry{
				"module": {
					Create: func() collectorapi.CollectorV1 {
						return &collectorapi.MockCollectorV1{
							ChartsFunc: func() *collectorapi.Charts {
								return &collectorapi.Charts{
									&collectorapi.Chart{
										ID: "chart", Title: "chart",
										Units: "value",
										Dims: collectorapi.Dims{
											&collectorapi.Dim{ID: "value"},
										},
									},
								}
							},
							CollectFunc: func(context.Context) map[string]int64 {
								return map[string]int64{"value": 1}
							},
							CleanupFunc: func(context.Context) {
								cleanupCalls.Add(1)
							},
						}
					},
					Config: func() any {
						return &collectorapi.MockConfiguration{}
					},
				},
			}
			jobs := testRunJobServices(t)
			jobs.Defaults = confgroup.Registry{
				"module": {UpdateEvery: 1},
			}
			jobs.Graph = fullCapacityInitialJobs(t, test.jobs)

			frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
			if err != nil {
				t.Fatal(err)
			}
			admission := lifecycle.NewAdmissionLedger()
			uids := lifecycle.NewUIDLedger()
			generation, err := newRunGeneration(runGenerationConfig{
				Generation: 1, ShutdownTimeout: 10 * time.Second,
				Clock: lifecycle.RealClock{}, Admission: admission, UIDs: uids,
				Frames: frames, Modules: modules, Jobs: jobs,
				Discovery: testRunDiscoveryServices(t),
				Planner: func(
					runPlannerCapabilities,
				) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
					return runRejectingPlanner{},
						jobmgr.RunFinalizerFunc(
							func(context.Context, uint64) error { return nil },
						),
						nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := generation.Start(context.Background()); err != nil {
				waitErr := generation.Wait(context.Background())
				t.Fatalf("full-capacity generation start: %v; shutdown: %v", err, waitErr)
			}
			if got := generation.scheduler.Census(); got != test.jobs {
				t.Fatalf("scheduler jobs=%d want=%d", got, test.jobs)
			}
			if got := len(generation.graph.IDs()); got != test.jobs {
				t.Fatalf("graph jobs=%d want=%d", got, test.jobs)
			}
			if got := generation.tasks.LongLivedCensus().Active; got != test.jobs+1 {
				t.Fatalf("long-lived resources=%d want=%d", got, test.jobs+1)
			}

			generation.Stop()
			if err := generation.Wait(context.Background()); err != nil {
				t.Fatal(err)
			}
			if got := cleanupCalls.Load(); got != int32(test.jobs) {
				t.Fatalf("collector cleanups=%d want=%d", got, test.jobs)
			}
			if err := admission.CloseDrained(1); err != nil {
				t.Fatal(err)
			}
			closeRunTestUIDs(t, uids)
		})
	}
}

func TestRunGenerationFunctionFlowAndShutdownOrder(t *testing.T) {
	var eventsMu sync.Mutex
	var events []string
	record := func(event string) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
	}
	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(runRecordingWriter{
		target: &output,
		record: func(payload []byte) {
			switch {
			case bytes.HasPrefix(payload, []byte("FUNCTION GLOBAL")) &&
				bytes.Contains(payload, []byte(`"module:method"`)):
				record("publish")
			case bytes.HasPrefix(payload, []byte("FUNCTION_DEL")) &&
				bytes.Contains(payload, []byte(`"module:method"`)):
				record("withdraw")
			case bytes.HasPrefix(payload, []byte("FUNCTION_RESULT_BEGIN")):
				record("result")
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	modules := collectorapi.Registry{
		"module": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &runTestHandler{cleanup: func() { record("cleanup") }}
			},
		},
	}
	admission := lifecycle.NewAdmissionLedger()
	uids := lifecycle.NewUIDLedger()
	generation, err := newRunGeneration(runGenerationConfig{
		Generation: 1, ShutdownTimeout: time.Second,
		Clock: lifecycle.RealClock{}, Admission: admission, UIDs: uids,
		Frames: frames, Modules: modules,
		Jobs:      testRunJobServices(t),
		Discovery: testRunDiscoveryServices(t),
		Planner: func(runPlannerCapabilities) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(func(context.Context, uint64) error {
					record("finalizer")
					return nil
				}),
				nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := generation.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := generation.kernel.SubmitAndWait(context.Background(), jobmgr.Request{
		UID: "function-flow", Source: lifecycle.SourceFunction, Route: "module:method",
	}); err != nil {
		t.Fatal(err)
	}
	generation.Stop()
	if err := generation.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	eventsMu.Lock()
	got := append([]string(nil), events...)
	eventsMu.Unlock()
	want := []string{"publish", "result", "withdraw", "cleanup", "finalizer"}
	if len(got) != len(want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("events=%v want=%v", got, want)
		}
	}
	if err := admission.CloseDrained(1); err != nil {
		t.Fatal(err)
	}
	closeRunTestUIDs(t, uids)
}

func TestRunGenerationDynCfgEnableUsesCatalogTransaction(t *testing.T) {
	var cleanupCalls atomic.Int32
	modules := collectorapi.Registry{
		"module": {
			Create: func() collectorapi.CollectorV1 {
				return &collectorapi.MockCollectorV1{
					CleanupFunc: func(context.Context) {
						cleanupCalls.Add(1)
					},
				}
			},
			Config: func() any {
				return &collectorapi.MockConfiguration{}
			},
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &runTestHandler{cleanup: func() {}}
			},
			JobConfigSchema: collectorapi.MockConfigSchema,
		},
	}
	config := confgroup.Config{
		"module": "module", "name": "job",
		"update_every": 1, "function_only": true,
	}
	config.SetProvider(confgroup.TypeDyncfg)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("test")
	payload, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{
		"module": {UpdateEvery: 1},
	}
	jobs.Graph = []dyncfg.GraphConfig{{
		ID: "module_job", Module: "module", Name: "job",
		Status: dyncfg.StatusAccepted.String(), Payload: payload,
	}}

	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	if err != nil {
		t.Fatal(err)
	}
	admission := lifecycle.NewAdmissionLedger()
	uids := lifecycle.NewUIDLedger()
	generation, err := newRunGeneration(runGenerationConfig{
		Generation: 1, ShutdownTimeout: time.Second,
		Clock: lifecycle.RealClock{}, Admission: admission, UIDs: uids,
		Frames: frames, Modules: modules, Jobs: jobs,
		Discovery: testRunDiscoveryServices(t),
		Planner: func(
			capabilities runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			if capabilities.Jobs == nil ||
				capabilities.DynCfg == nil ||
				capabilities.Graph == nil {
				t.Fatal("planner did not receive sealed job capabilities")
			}
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error { return nil },
				),
				nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := generation.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := generation.kernel.SubmitAndWait(
		context.Background(),
		jobmgr.Request{
			UID:    "enable",
			Source: lifecycle.SourceFunction,
			Route:  "config",
			Args: []string{
				"go.d:collector:module:job",
				string(dyncfg.CommandEnable),
			},
		},
	); err != nil {
		t.Fatal(err)
	}
	record, ok := generation.graph.Lookup("module_job")
	if !ok || record.Status != dyncfg.StatusRunning.String() {
		t.Fatalf("DynCfg graph record=%+v exists=%v", record, ok)
	}
	wire := output.String()
	resultAt := bytes.Index(
		output.Bytes(),
		[]byte("FUNCTION_RESULT_BEGIN enable 200 application/json"),
	)
	statusAt := bytes.Index(
		output.Bytes(),
		[]byte("CONFIG go.d:collector:module:job status running"),
	)
	if !bytes.Contains(
		output.Bytes(),
		[]byte(`FUNCTION GLOBAL "config"`),
	) || resultAt < 0 || statusAt < 0 || resultAt >= statusAt {
		t.Fatalf("DynCfg wire ordering differs: %q", wire)
	}

	generation.Stop()
	if err := generation.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cleanupCalls.Load() != 1 {
		t.Fatalf("collector cleanups=%d want=1", cleanupCalls.Load())
	}
	if err := admission.CloseDrained(1); err != nil {
		t.Fatal(err)
	}
	closeRunTestUIDs(t, uids)
}

func fullCapacityInitialJobs(
	t testing.TB,
	count int,
) []dyncfg.GraphConfig {
	t.Helper()
	records := make([]dyncfg.GraphConfig, count)
	for ordinal := range count {
		name := fmt.Sprintf("job-%03d", ordinal)
		config := confgroup.Config{
			"module":       "module",
			"name":         name,
			"update_every": 1,
			"option_str":   "work",
			"option_int":   1,
		}
		config.SetProvider(confgroup.TypeDyncfg)
		config.SetSourceType(confgroup.TypeDyncfg)
		config.SetSource("test")
		payload, err := yaml.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		records[ordinal] = dyncfg.GraphConfig{
			ID: config.FullName(), Module: config.Module(),
			Name: config.Name(), Status: dyncfg.StatusRunning.String(),
			Payload: payload,
		}
	}
	return records
}

func testRunJobServices(t testing.TB) runJobServices {
	t.Helper()
	resolver, err := secretresolver.NewAtomicResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	creators, err := secretstore.NewCreatorCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	return runJobServices{
		PluginName:    "go.d",
		Defaults:      confgroup.Registry{},
		Resolver:      resolver,
		StoreCreators: creators,
		Vnodes:        vnoderegistry.New(),
	}
}

func testRunDiscoveryServices(t testing.TB) runDiscoveryServices {
	t.Helper()
	factory := agentdiscovery.NewProviderFactory(
		"test",
		func(agentdiscovery.BuildContext) (
			agentdiscovery.Discoverer,
			bool,
			error,
		) {
			return runTestDiscoverer{}, true, nil
		},
	)
	catalog, err := agentdiscovery.NewProviderCatalog(
		[]agentdiscovery.ProviderFactory{factory},
	)
	if err != nil {
		t.Fatal(err)
	}
	return runDiscoveryServices{
		BuildContext: agentdiscovery.BuildContext{
			Registry: confgroup.Registry{"test": {}},
		},
		Providers:  catalog,
		AutoEnable: true,
	}
}

type runTestDiscoverer struct{}

func (runTestDiscoverer) Run(
	ctx context.Context,
	_ chan<- []*confgroup.Group,
) {
	<-ctx.Done()
}

type runRecordingWriter struct {
	target *bytes.Buffer
	record func([]byte)
}

func (writer runRecordingWriter) Write(payload []byte) (int, error) {
	writer.record(payload)
	return writer.target.Write(payload)
}

type runTestHandler struct {
	cleanup func()
}

func (*runTestHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (*runTestHandler) Handle(
	context.Context,
	string,
	funcapi.ResolvedParams,
) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{Status: 200}
}

func (handler *runTestHandler) Cleanup(context.Context) {
	handler.cleanup()
}

type runRejectingPlanner struct{}

func (runRejectingPlanner) Plan(jobmgr.Request) (jobmgr.WorkPlan, error) {
	return jobmgr.RejectionPlan(lifecycle.ControlBadRequest), nil
}

func closeRunTestUIDs(t *testing.T, uids *lifecycle.UIDLedger) {
	t.Helper()
	for {
		more, err := uids.CloseBatch(lifecycle.UIDReturnBatch)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			return
		}
	}
}
