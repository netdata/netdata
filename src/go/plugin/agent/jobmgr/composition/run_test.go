// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestRunGenerationGrowsBeyondFormerJobLimitWithDiscoveryPipeline(
	t *testing.T,
) {
	tests := map[string]struct {
		jobs int
	}{
		"one pipeline plus the former limit and one job": {
			jobs: 257,
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
			require.NoError(t, err)
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
			require.NoError(t, err)
			if err := generation.start(context.Background()); err != nil {
				waitErr := generation.Wait(context.Background())
				require.FailNowf(t, "test failed", "full-capacity generation start: %v; shutdown: %v", err, waitErr)
			}

			require.EqualValues(t, test.jobs, generation.scheduler.Census())

			require.EqualValues(t, test.jobs, len(generation.graph.IDs()))

			require.EqualValues(t, test.jobs+1, generation.tasks.LongLivedCensus().Active)

			generation.Stop()

			require.NoError(t, generation.Wait(context.Background()))

			require.EqualValues(t, int32(test.jobs), cleanupCalls.Load())

			require.NoError(t, admission.CloseDrained(1))

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
	require.NoError(t, err)
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
	require.NoError(t, err)

	require.NoError(t, generation.start(context.Background()))

	require.NoError(t, generation.kernel.SubmitAndWait(context.Background(), jobmgr.Request{
		UID: "function-flow", Source: lifecycle.SourceFunction, Route: "module:method",
	}),
	)

	generation.Stop()

	require.NoError(t, generation.Wait(context.Background()))

	eventsMu.Lock()
	got := append([]string(nil), events...)
	eventsMu.Unlock()
	want := []string{"publish", "result", "withdraw", "cleanup", "finalizer"}
	require.EqualValues(t, len(want), len(got))
	for index := range want {
		require.EqualValues(t, want[index], got[index])
	}

	require.NoError(t, admission.CloseDrained(1))

	closeRunTestUIDs(t, uids)
}

func TestRunGenerationConstructionFailureClosesSecretStore(t *testing.T) {
	var storeCensus func() secretstore.SecretStoreCensus
	plannerErr := errors.New("planner construction failed")
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)

	generation, err := newRunGeneration(runGenerationConfig{
		Generation: 1, ShutdownTimeout: time.Second,
		Clock: lifecycle.RealClock{}, Admission: lifecycle.NewAdmissionLedger(),
		UIDs: lifecycle.NewUIDLedger(), Frames: frames,
		Modules: collectorapi.Registry{}, Jobs: testRunJobServices(t),
		Discovery: testRunDiscoveryServices(t),
		Planner: func(
			capabilities runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			storeCensus = capabilities.StoreCensus
			return nil, nil, plannerErr
		},
	})
	require.ErrorIs(t, err, plannerErr)
	require.Nil(t, generation)
	require.NotNil(t, storeCensus)
	require.True(t, storeCensus().Closed)
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
	require.NoError(t, err)
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
	require.NoError(t, err)
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
			require.False(t, capabilities.Jobs == nil || capabilities.DynCfg == nil || capabilities.Graph == nil)
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error { return nil },
				),
				nil
		},
	})
	require.NoError(t, err)

	require.NoError(t, generation.start(context.Background()))

	require.NoError(t, generation.kernel.SubmitAndWait(
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
	),
	)

	record, ok := generation.graph.Lookup("module_job")
	require.False(t, !ok || record.Status != dyncfg.StatusRunning.String())
	wire := output.String()
	resultAt := bytes.Index(output.Bytes(), []byte("FUNCTION_RESULT_BEGIN enable 200 application/json"))
	statusAt := bytes.Index(output.Bytes(), []byte("CONFIG go.d:collector:module:job status running"))
	require.False(t, !bytes.Contains(output.Bytes(), []byte(`FUNCTION GLOBAL "config"`)) || resultAt < 0 || statusAt < 0 || resultAt >= statusAt, "wire=%q", wire)

	generation.Stop()

	require.NoError(t, generation.Wait(context.Background()))

	require.EqualValues(t, 1, cleanupCalls.Load())

	require.NoError(t, admission.CloseDrained(1))

	closeRunTestUIDs(t, uids)
}

func TestRunGenerationShutdownDrainsJobActivationAndFunctionPublication(
	t *testing.T,
) {
	checkEntered := make(chan struct{})
	checkRelease := make(chan struct{})
	var cleanupCalls atomic.Int32
	modules := collectorapi.Registry{
		"module": {
			Create: func() collectorapi.CollectorV1 {
				return &collectorapi.MockCollectorV1{
					CheckFunc: func(context.Context) error {
						close(checkEntered)
						<-checkRelease
						return nil
					},
					CleanupFunc: func(context.Context) {
						cleanupCalls.Add(1)
					},
				}
			},
			Config: func() any {
				return &collectorapi.MockConfiguration{}
			},
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(
				collectorapi.RuntimeJob,
			) funcapi.MethodHandler {
				return &runTestHandler{cleanup: func() {}}
			},
			JobConfigSchema: collectorapi.MockConfigSchema,
		},
	}
	config := confgroup.Config{
		"module": "module", "name": "job",
		"update_every": 1, "function_only": true,
		"option_str": "work", "option_int": 1,
	}
	config.SetProvider(confgroup.TypeDyncfg)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("test")
	payload, err := yaml.Marshal(config)
	require.NoError(t, err)
	jobs := testRunJobServices(t)
	jobs.Defaults = confgroup.Registry{
		"module": {UpdateEvery: 1},
	}
	jobs.Graph = []dyncfg.GraphConfig{{
		ID: config.FullName(), Module: config.Module(),
		Name: config.Name(), Status: dyncfg.StatusAccepted.String(),
		Payload: payload,
	}}

	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	admission := lifecycle.NewAdmissionLedger()
	uids := lifecycle.NewUIDLedger()
	generation, err := newRunGeneration(runGenerationConfig{
		Generation: 1, ShutdownTimeout: time.Second,
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
	require.NoError(t, err)
	require.NoError(t, generation.start(context.Background()))

	submitted := make(chan error, 1)
	go func() {
		submitted <- generation.kernel.SubmitAndWait(
			context.Background(),
			jobmgr.Request{
				UID:    "shutdown-enable",
				Source: lifecycle.SourceFunction,
				Route:  "config",
				Args: []string{
					"go.d:collector:module:job",
					string(dyncfg.CommandEnable),
				},
			},
		)
	}()
	select {
	case <-checkEntered:
	case <-time.After(time.Second):
		require.FailNow(
			t,
			"test failed",
			"managed autodetection did not enter",
		)
	}

	generation.Stop()
	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancelShutdown()
	require.NoError(
		t,
		generation.kernel.WaitShutdownStarted(shutdownCtx),
	)
	close(checkRelease)
	select {
	case err := <-submitted:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(
			t,
			"test failed",
			"protected Job activation did not settle",
		)
	}
	require.NoError(t, generation.Wait(context.Background()))
	require.NoError(t, generation.run.DirtyCause())

	publishedAt := bytes.Index(
		output.Bytes(),
		[]byte(`FUNCTION GLOBAL "module:method"`),
	)
	withdrawnAt := bytes.Index(
		output.Bytes(),
		[]byte(`FUNCTION_DEL GLOBAL "module:method"`),
	)
	require.GreaterOrEqual(t, publishedAt, 0)
	require.Greater(t, withdrawnAt, publishedAt)
	require.EqualValues(t, 1, cleanupCalls.Load())
	require.Equal(
		t,
		lifecycle.InheritedTaskCensus{},
		generation.tasks.InheritedCensus(),
	)
	require.Equal(
		t,
		lifecycle.LongLivedCensus{},
		generation.tasks.LongLivedCensus(),
	)
	require.NoError(t, admission.CloseDrained(1))
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
		require.NoError(t, err)
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
	require.NoError(t, err)
	creators, err := secretstore.NewCreatorCatalog(nil)
	require.NoError(t, err)
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
	require.NoError(t, err)
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

func (rrw runRecordingWriter) Write(payload []byte) (int, error) {
	rrw.record(payload)
	return rrw.target.Write(payload)
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

func (rth *runTestHandler) Cleanup(context.Context) {
	rth.cleanup()
}

type runRejectingPlanner struct{}

func (runRejectingPlanner) Plan(jobmgr.Request) (jobmgr.WorkPlan, error) {
	return jobmgr.RejectionPlan(lifecycle.ControlBadRequest), nil
}

func TestCloseProcessUIDsObservesShutdownContextBetweenBatches(t *testing.T) {
	tests := map[string]struct {
		cancelled      bool
		wantErr        error
		wantTombstones int
		wantClosed     bool
	}{
		"live shutdown drains every batch": {
			wantTombstones: 0,
			wantClosed:     true,
		},
		"expired shutdown leaves process-exit containment": {
			cancelled:      true,
			wantErr:        context.Canceled,
			wantTombstones: 257,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			uids := lifecycle.NewUIDLedger()
			now := time.Now()
			for index := range 257 {
				uid := fmt.Sprintf("close-uid-%d", index)

				require.NoError(t, uids.Admit(uid, now))

				require.NoError(t, uids.Complete(uid, true, now))
			}
			ctx, cancel := context.WithCancel(context.Background())
			if test.cancelled {
				cancel()
			} else {
				defer cancel()
			}
			err := closeProcessUIDs(ctx, uids)
			require.ErrorIs(t, err, test.wantErr)
			active, tombstones, closed := uids.Census()
			require.False(t, active != 0 || tombstones != test.wantTombstones || closed != test.wantClosed)
		})
	}
}

func closeRunTestUIDs(t *testing.T, uids *lifecycle.UIDLedger) {
	t.Helper()

	require.NoError(t, closeProcessUIDs(context.Background(), uids))
}
