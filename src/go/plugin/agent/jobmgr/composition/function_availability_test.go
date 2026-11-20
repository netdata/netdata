// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	functionadapter "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/functions"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestRunFunctionAvailabilityDiagnosticsAreSafeAndBounded(t *testing.T) {
	const eventName = "Function availability callback failed"
	const secret = "synthetic-backend-password"
	output := newProcessSynchronizedBuffer()
	logs := newProcessSynchronizedBuffer()
	diagnostics := &availabilityTestDiagnostics{
		sink:   newDiagnosticLogger(logs),
		events: make(chan jobmgr.DiagnosticEvent, 16),
	}
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	uids := lifecycle.NewUIDLedger()
	var mode atomic.Int32
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Secrets:         testRunSecrets(t),
		Generation:      7,
		ShutdownTimeout: time.Second,
		Diagnostics:     diagnostics,
		UIDs:            uids,
		Frames:          frames,
		Modules: collectorapi.Registry{
			"module": {
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{
						ID: "method",
						Available: func() bool {
							if mode.Load() == 1 {
								panic(secret)
							}
							return mode.Load() == 2
						},
					}}
				},
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &assemblyTestHandler{}
				},
			},
		},
		Jobs:      testRunJobServices(t),
		Discovery: testRunDiscoveryServices(t),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		generation.Stop()
		require.NoError(t, generation.Wait(context.Background()))
		closeRunTestUIDs(t, uids)
	})
	require.NoError(t, generation.start(context.Background()))
	require.NotContains(t, output.String(), `FUNCTION GLOBAL "module:method"`)

	mode.Store(1)
	for range 3 {
		require.NoError(t, generation.functions.ReconcileModule(t.Context(), "module"))
		select {
		case event := <-diagnostics.events:
			require.Equal(t, eventName, event.Name)
			require.Equal(t, "panic", event.State)
			require.EqualValues(t, 7, event.Generation)
			require.Nil(t, event.Err)
		case <-time.After(time.Second):
			require.FailNow(t, "availability callback failure was not diagnosed")
		}
	}
	require.Equal(t, 1, strings.Count(logs.String(), eventName))
	require.NotContains(t, logs.String(), secret)
	require.NotContains(t, output.String(), secret)
	require.NotContains(t, output.String(), `FUNCTION GLOBAL "module:method"`)
	require.NoError(t, generation.run.DirtyCause())

	// Recovery must still flow through the poll, kernel mutation and normal
	// Function publication; diagnostic suppression must not suppress work.
	mode.Store(2)
	require.NoError(t, generation.functions.ReconcileModule(t.Context(), "module"))
	require.Eventually(t, func() bool {
		return strings.Contains(output.String(), `FUNCTION GLOBAL "module:method"`)
	}, time.Second, time.Millisecond)
	select {
	case event := <-diagnostics.events:
		require.FailNow(t, "successful availability poll emitted a failure", "%+v", event)
	default:
	}
}

func TestFunctionAvailabilityReconciliationCollisionIsDiagnosed(t *testing.T) {
	output := newProcessSynchronizedBuffer()
	logs := newProcessSynchronizedBuffer()
	diagnostics := &availabilityTestDiagnostics{
		sink:   newDiagnosticLogger(logs),
		events: make(chan jobmgr.DiagnosticEvent, 16),
	}
	attempts, err := containment.NewAuthority(diagnostics)
	require.NoError(t, err)
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	var available atomic.Bool
	var initialCalls atomic.Int32
	assembly, err := NewContainedFunctionAssembly(
		context.Background(), 7, attempts, diagnostics,
		collectorapi.Registry{
			"module": {
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "method", Available: available.Load}}
				},
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &assemblyTestHandler{}
				},
			},
		},
		frames,
		functionadapter.InitialRoute{
			Declaration: functionadapter.Declaration{
				ID:         "initial",
				PublicName: "module:method",
				Generation: &functionadapter.HandlerGenerationDeclaration{
					ID: "initial",
					Handler: func(context.Context, functionadapter.HandlerInput) (lifecycle.SealedResult, error) {
						initialCalls.Add(1)
						return lifecycle.NewSealedResult(200, "text/plain", nil)
					},
				},
			},
		},
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, stopFunctionAssembly(assembly, 7))
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, attempts.Shutdown(ctx))
	})
	catalog := assembly.Catalog().(*functionadapter.Catalog)
	require.NoError(t, assembly.Bind(&assemblyMutationPort{
		catalog: catalog,
	}))
	require.NoError(t, assembly.Activate())
	available.Store(true)
	require.NoError(t, assembly.ReconcileModule(t.Context(), "module"))
	select {
	case event := <-diagnostics.events:
		require.Equal(t, "Function availability reconciliation failed", event.Name)
		require.Equal(t, "failed", event.State)
		require.EqualValues(t, 7, event.Generation)
		require.Nil(t, event.Err)
	case <-time.After(time.Second):
		require.FailNow(t, "availability route collision was not diagnosed")
	}
	require.Empty(t, output.String(), "colliding route must not be externally published")
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID:   "retained-initial",
		Route: "module:method",
	})
	require.NoError(t, err)
	require.Zero(t, decision.Rejected)
	_, err = decision.Plan.Work(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 1, initialCalls.Load())
	_, err = catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
}

type availabilityTestDiagnostics struct {
	sink   jobmgr.DiagnosticObserver
	events chan jobmgr.DiagnosticEvent
}

func (d *availabilityTestDiagnostics) ObserveDiagnostic(event jobmgr.DiagnosticEvent) {
	d.sink.ObserveDiagnostic(event)
	if strings.HasPrefix(event.Name, "Function availability ") {
		d.events <- event
	}
}

type handoffJob struct {
	assemblyTestJob
	name     string
	ready    atomic.Bool
	observed chan struct{}
	resume   chan struct{}
}

func (j *handoffJob) FullName() string { return "module_" + j.name }
func (j *handoffJob) Name() string     { return j.name }
func (j *handoffJob) Collector() any   { return j }
func (j *handoffJob) FunctionAvailable(method string) bool {
	if method == "shared" && j.observed != nil {
		close(j.observed)
		if j.resume != nil {
			<-j.resume
		}
	}
	return j.ready.Load() && (method == "shared" || method == j.name)
}

func TestSingleTickSharedFunctionReadinessHandoff(t *testing.T) {
	output := newProcessSynchronizedBuffer()
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	uids := lifecycle.NewUIDLedger()
	modules := collectorapi.Registry{}
	modules.Register("module", collectorapi.Creator{
		SharedFunctions: func() []funcapi.FunctionConfig {
			return []funcapi.FunctionConfig{{ID: "shared", Aliases: []string{"alias"}}, {ID: "first"}, {ID: "second"}}
		},
		MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler { return &assemblyTestHandler{} },
	})
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Secrets: testRunSecrets(t), Generation: 7, ShutdownTimeout: time.Second, UIDs: uids, Frames: frames,
		Modules: modules, Jobs: testRunJobServices(t), Discovery: testRunDiscoveryServices(t),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		generation.Stop()
		require.NoError(t, generation.Wait(context.Background()))
		closeRunTestUIDs(t, uids)
	})
	require.NoError(t, generation.start(context.Background()))
	start := func(name string, ready bool) *handoffJob {
		t.Helper()
		job := &handoffJob{name: name}
		job.ready.Store(ready)
		stage := generation.functions.jobLifecycle()
		staged, err := stage.Stage(job)
		require.NoError(t, err)
		id := lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1}
		handle, err := stage.Attach(id, staged)
		require.NoError(t, err)
		require.NoError(t, generation.scheduler.Register(id, job))
		t.Cleanup(func() {
			require.NoError(t, generation.scheduler.Unregister(id, job))
			require.NoError(t, handle.CloseAndDrain(context.Background()))
		})
		require.NoError(t, handle.Publish())
		return job
	}
	first, second := start("first", true), start("second", false)
	first.observed, second.observed = make(chan struct{}), make(chan struct{})
	second.resume = make(chan struct{})
	resume := sync.OnceFunc(func() { close(second.resume) })
	t.Cleanup(resume)
	startOffset := len(output.String())
	second.ready.Store(true)
	first.ready.Store(false)
	require.NoError(t, generation.tick(t.Context(), 1))
	for _, job := range []*handoffJob{first, second} {
		select {
		case <-job.observed:
		case <-time.After(time.Second):
			t.Fatalf("poll did not reach %s", job.name)
		}
	}

	// Give the outgoing job time to settle while the incoming job is still evaluating.
	require.Never(t, func() bool {
		return strings.Contains(output.String()[startOffset:], "FUNCTION_DEL GLOBAL")
	}, 30*time.Millisecond, time.Millisecond)
	assertSelector := func(uid, want, absent string) {
		t.Helper()
		offset := len(output.String())
		require.NoError(t, generation.kernel.Submit(t.Context(), jobmgr.Request{
			UID: uid, Source: lifecycle.SourceFunction, Route: "alias", Args: []string{"info"},
		}))
		require.Eventually(t, func() bool {
			return strings.Contains(output.String()[offset:], "FUNCTION_RESULT_END")
		}, time.Second, time.Millisecond)
		wire := output.String()[offset:]
		require.Contains(t, wire, "FUNCTION_RESULT_BEGIN "+uid+" 200")
		require.Contains(t, wire, `"id":"`+want+`"`)
		require.NotContains(t, wire, `"id":"`+absent+`"`)
	}
	assertSelector("during-handoff", "first", "second")
	resume()
	require.Eventually(t, func() bool {
		wire := output.String()[startOffset:]
		return strings.Contains(wire, `FUNCTION_DEL GLOBAL "module:first"`) &&
			strings.Contains(wire, `FUNCTION GLOBAL "module:second"`)
	}, time.Second, time.Millisecond)
	assertSelector("after-handoff", "second", "first")
	wire := output.String()[startOffset:]
	for _, name := range []string{"alias", "module:shared"} {
		require.NotContains(t, wire, `FUNCTION_DEL GLOBAL "`+name+`"`)
		require.NotContains(t, wire, `FUNCTION GLOBAL "`+name+`"`)
	}
}
