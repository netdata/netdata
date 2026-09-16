// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestFunctionFinalizerHonorsRetainedBundleWaitContext(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "cancellation"
		if deadline {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			frames, err := lifecycle.NewFrameOwner(io.Discard)
			require.NoError(t, err)
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseCleanup := func() { releaseOnce.Do(func() { close(release) }) }
			handlers := map[string]*finalizerTestHandler{
				"first":  newFinalizerTestHandler(release),
				"second": newFinalizerTestHandler(release),
			}
			assembly, err := newTestFunctionAssembly(t, 1, collectorapi.Registry{
				"module": {
					SharedFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "method"}}
					},
					MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
						return handlers[job.Name()]
					},
				},
			}, frames)
			require.NoError(t, err)
			// Release blocked callbacks before the assembly helper's cleanup, including
			// when the baseline ignores cancellation and this regression fails.
			t.Cleanup(releaseCleanup)
			require.NoError(t, assembly.Bind(&assemblyMutationPort{
				catalog: assembly.catalog,
			}))
			require.NoError(t, assembly.Activate())
			for name := range handlers {
				job := &finalizerTestJob{
					name: name,
				}
				staged, err := assembly.jobLifecycle().Stage(job)
				require.NoError(t, err)
				handle, err := assembly.jobLifecycle().Attach(lifecycle.ResourceIdentity{
					ID:         job.FullName(),
					Generation: 1,
				}, staged)
				require.NoError(t, err)
				require.NoError(t, handle.Publish())
			}
			// Exercise the retained-bundle fallback through real attachment and catalog
			// transitions. Normal kernel shutdown detaches these job handles first.
			require.NoError(t, closeFunctionAssemblyCatalog(assembly, 1))
			ctx, cancel := context.WithCancel(context.Background())
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			}
			defer cancel()
			finished := make(chan error, 1)
			go func() { finished <- assembly.FinalizeRun(ctx, 1) }()
			require.Eventually(t, func() bool {
				return handlers["first"].calls.Load()+handlers["second"].calls.Load() > 0
			}, time.Second, time.Millisecond)
			if !deadline {
				cancel()
			}
			select {
			case err := <-finished:
				require.ErrorIs(t, err, ctx.Err())
			case <-time.After(time.Second):
				t.Fatal("Function finalization ignored its caller's wait context")
			}
			for _, handler := range handlers {
				select {
				case cleanupCtx := <-handler.entered:
					require.NoError(t, cleanupCtx.Err(), "wait cancellation must not cancel physical cleanup")
				case <-time.After(time.Second):
					t.Fatal("expired wait prevented another bundle from retiring")
				}
				select {
				case <-handler.done:
					t.Fatal("caller wait reported physical cleanup completion before callback returned")
				default:
				}
				require.EqualValues(t, 1, handler.calls.Load())
			}
			require.Error(t, assembly.Activate(), "finalization must not reopen publication")
			releaseCleanup()
			for _, handler := range handlers {
				waitShutdownFunctionGate(t, handler.done, "retained handler cleanup")
				require.EqualValues(t, 1, handler.calls.Load())
			}
		})
	}
}

func TestFunctionFinalizerRejectsNilContextWithoutStopping(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(io.Discard)
	require.NoError(t, err)
	assembly, err := newTestFunctionAssembly(t, 1, collectorapi.Registry{}, frames)
	require.NoError(t, err)
	require.NoError(t, assembly.Bind(&assemblyMutationPort{
		catalog: assembly.catalog,
	}))
	require.NoError(t, assembly.Activate())
	require.NoError(t, closeFunctionAssemblyCatalog(assembly, 1))
	require.Error(t, assembly.FinalizeRun(nil, 1))
	require.NoError(t, assembly.FinalizeRun(context.Background(), 1))
}

func TestFunctionRunFinalizationRetainsPhysicalModuleCleanup(t *testing.T) {
	output := newProcessSynchronizedBuffer()
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, attempts.Shutdown(ctx))
	})
	release := make(chan struct{})
	var once sync.Once
	releaseCleanup := func() { once.Do(func() { close(release) }) }
	handler := newFinalizerTestHandler(release)
	uids := lifecycle.NewUIDLedger()
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Generation:      1,
		Attempts:        attempts,
		ShutdownTimeout: time.Second,
		UIDs:            uids,
		Frames:          frames,
		Modules: collectorapi.Registry{
			"module": {
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "method"}}
				},
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler { return handler },
			},
		},
		Jobs:      testRunJobServices(t),
		Discovery: testRunDiscoveryServices(t),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		releaseCleanup()
		generation.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, generation.Wait(ctx))
		closeRunTestUIDs(t, uids)
	})
	require.NoError(t, generation.start(context.Background()))
	generation.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, generation.Wait(ctx), "run finalization must not wait for process-owned handler cleanup")
	select {
	case cleanupCtx := <-handler.entered:
		require.NoError(t, cleanupCtx.Err())
	case <-ctx.Done():
		t.Fatal("module handler cleanup did not start")
	}
	require.Equal(t, containment.Census{
		Active:   1,
		Admitted: 1,
	}, attempts.Census())
	require.Contains(t, output.String(), `FUNCTION_DEL GLOBAL "module:method"`)
	require.NoError(t, generation.run.DirtyCause())
	releaseCleanup()
	waitShutdownFunctionGate(t, handler.done, "process-owned module cleanup")
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{})
	}, time.Second, time.Millisecond)
	require.EqualValues(t, 1, handler.calls.Load())
}

type finalizerTestHandler struct {
	assemblyTestHandler
	release <-chan struct{}
	entered chan context.Context
	done    chan struct{}
	calls   atomic.Int32
}

func newFinalizerTestHandler(release <-chan struct{}) *finalizerTestHandler {
	return &finalizerTestHandler{
		release: release,
		entered: make(chan context.Context, 1),
		done:    make(chan struct{}),
	}
}

func (h *finalizerTestHandler) Cleanup(ctx context.Context) {
	h.calls.Add(1)
	h.entered <- ctx
	<-h.release
	close(h.done)
}

type finalizerTestJob struct {
	assemblyTestJob
	name string
}

func (job *finalizerTestJob) FullName() string { return "module_" + job.name }
func (job *finalizerTestJob) Name() string     { return job.name }
