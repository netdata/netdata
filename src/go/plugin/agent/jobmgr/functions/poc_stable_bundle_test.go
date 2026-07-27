// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errPOCFunctionBundleBusy    = errors.New("POC Function bundle is busy")
	errPOCFunctionBundleRetired = errors.New("POC Function bundle is retired")
)

func TestPOCStableFunctionBundlesSurviveCatalogReplacementAndContainedPolling(t *testing.T) {
	handlerA := newPOCBlockingFunctionHandler(false)
	handlerB := newPOCBlockingFunctionHandler(true)
	t.Cleanup(handlerA.releaseCleanup)
	t.Cleanup(handlerB.releaseCleanup)
	handlers := map[string]*pocBlockingFunctionHandler{
		"a": handlerA,
		"b": handlerB,
	}
	constructions := make(map[string]int)
	creator := collectorapi.Creator{
		MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
			constructions[job.Name()]++
			return handlers[job.Name()]
		},
	}
	jobA := &controllerTestJob{
		fullName: "module_a",
		module:   "module",
		name:     "a",
		running:  true,
	}
	jobB := &controllerTestJob{
		fullName: "module_b",
		module:   "module",
		name:     "b",
		running:  true,
	}
	pollAEntered := make(chan struct{})
	pollARelease := make(chan struct{})
	var pollAReleaseOnce sync.Once
	releasePollA := func() {
		pollAReleaseOnce.Do(func() {
			close(pollARelease)
		})
	}
	t.Cleanup(releasePollA)
	var pollACalls atomic.Int32
	var pollBCalls atomic.Int32
	bundleA, err := newPOCStableFunctionBundle(creator, jobA, func() bool {
		pollACalls.Add(1)
		close(pollAEntered)
		<-pollARelease
		return false
	})
	require.NoError(t, err)
	bundleB, err := newPOCStableFunctionBundle(creator, jobB, func() bool {
		pollBCalls.Add(1)
		return true
	})
	require.NoError(t, err)
	require.Equal(t, map[string]int{"a": 1, "b": 1}, constructions)

	first, err := newPOCBundleCatalogGeneration("generation-1", bundleA)
	require.NoError(t, err)
	catalog, err := NewCatalog([]Declaration{pocBundleDeclaration(first)})
	require.NoError(t, err)
	held, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID:   "held",
		Route: "module:method",
	})
	require.NoError(t, err)
	_, err = held.Plan.Work(context.Background())
	require.NoError(t, err)

	pollCtx, cancelPoll := context.WithCancel(context.Background())
	pollResult := make(chan error, 1)
	go func() {
		pollResult <- bundleA.poll(pollCtx)
	}()
	<-pollAEntered
	cancelPoll()
	require.ErrorIs(t, <-pollResult, context.Canceled)
	require.ErrorIs(t, bundleA.poll(context.Background()), errPOCFunctionBundleBusy)
	require.NoError(t, bundleB.poll(context.Background()))

	second, err := newPOCBundleCatalogGeneration("generation-2", bundleA, bundleB)
	require.NoError(t, err)
	require.Equal(t, map[string]int{"a": 1, "b": 1}, constructions)
	mutation, err := catalog.NewMutation(1, []RouteChange{{
		PublicName:  "module:method",
		Declaration: ptrPOCBundleDeclaration(second),
	}})
	require.NoError(t, err)
	assert.Empty(t, commitMutation(t, catalog, mutation))

	cleanup, err := catalog.ReleaseInvocation(held.Lease)
	require.NoError(t, err)
	require.True(t, cleanup.Valid())
	runCleanupPlan(t, catalog, cleanup)
	assert.Zero(t, handlerA.cleanupCount())

	current, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID:   "current",
		Route: "module:method",
	})
	require.NoError(t, err)
	_, err = current.Plan.Work(context.Background())
	require.NoError(t, err)
	cleanup, err = catalog.ReleaseInvocation(current.Lease)
	require.NoError(t, err)
	assert.False(t, cleanup.Valid())

	third, err := newPOCBundleCatalogGeneration("generation-3", bundleB)
	require.NoError(t, err)
	mutation, err = catalog.NewMutation(2, []RouteChange{{
		PublicName:  "module:method",
		Declaration: ptrPOCBundleDeclaration(third),
	}})
	require.NoError(t, err)
	cleanups := commitMutation(t, catalog, mutation)
	require.Len(t, cleanups, 1)
	require.NoError(t, bundleA.retire())
	runCleanupPlan(t, catalog, cleanups[0])

	assert.EqualValues(t, 3, catalog.census().Version)
	assert.Zero(t, catalog.census().PendingCleanups)
	assert.Zero(t, handlerA.cleanupCount())
	select {
	case <-handlerA.cleanupEntered:
		require.FailNow(t, "test failed", "handler cleanup started while its availability callback was live")
	default:
	}

	releasePollA()
	<-handlerA.cleanupEntered
	assert.Zero(t, catalog.census().PendingCleanups)
	handlerA.releaseCleanup()
	<-bundleA.cleanupDone
	assert.EqualValues(t, 1, handlerA.cleanupCount())
	assert.EqualValues(t, 1, pollACalls.Load())
	assert.EqualValues(t, 1, pollBCalls.Load())

	mutation, err = catalog.NewMutation(3, []RouteChange{{
		PublicName: "module:method",
	}})
	require.NoError(t, err)
	cleanups = commitMutation(t, catalog, mutation)
	require.Len(t, cleanups, 1)
	require.NoError(t, bundleB.retire())
	runCleanupPlan(t, catalog, cleanups[0])
	<-bundleB.cleanupDone
	assert.EqualValues(t, 1, handlerB.cleanupCount())
}

func pocBundleDeclaration(
	generation *HandlerGenerationDeclaration,
) Declaration {
	return Declaration{
		ID:         "method",
		Generation: generation,
		PublicName: "module:method",
	}
}

func ptrPOCBundleDeclaration(
	generation *HandlerGenerationDeclaration,
) *Declaration {
	declaration := pocBundleDeclaration(generation)
	return &declaration
}

type pocStableFunctionBundle struct {
	mu sync.Mutex

	handler      funcapi.MethodHandler
	availability func() bool
	available    bool
	references   int
	polling      bool
	retired      bool
	cleanupStart bool
	cleanupDone  chan struct{}
}

func newPOCStableFunctionBundle(
	creator collectorapi.Creator,
	job collectorapi.RuntimeJob,
	availability func() bool,
) (*pocStableFunctionBundle, error) {
	if creator.MethodHandler == nil || job == nil || availability == nil {
		return nil, errors.New("POC invalid Function bundle")
	}
	handler := creator.MethodHandler(job)
	if handler == nil {
		return nil, errors.New("POC nil Function handler")
	}
	return &pocStableFunctionBundle{
		handler:      handler,
		availability: availability,
		available:    true,
		references:   1,
		cleanupDone:  make(chan struct{}),
	}, nil
}

func (bundle *pocStableFunctionBundle) acquire() error {
	bundle.mu.Lock()
	defer bundle.mu.Unlock()
	if bundle.retired {
		return errPOCFunctionBundleRetired
	}
	bundle.references++
	return nil
}

func (bundle *pocStableFunctionBundle) release() {
	bundle.mu.Lock()
	if bundle.references <= 0 {
		bundle.mu.Unlock()
		panic("POC Function bundle reference underflow")
	}
	bundle.references--
	start := bundle.startCleanupLocked()
	bundle.mu.Unlock()
	if start {
		go bundle.cleanup()
	}
}

func (bundle *pocStableFunctionBundle) retire() error {
	bundle.mu.Lock()
	if bundle.retired || bundle.references <= 0 {
		bundle.mu.Unlock()
		return errPOCFunctionBundleRetired
	}
	bundle.retired = true
	bundle.references--
	start := bundle.startCleanupLocked()
	bundle.mu.Unlock()
	if start {
		go bundle.cleanup()
	}
	return nil
}

func (bundle *pocStableFunctionBundle) poll(ctx context.Context) error {
	bundle.mu.Lock()
	if bundle.retired {
		bundle.mu.Unlock()
		return errPOCFunctionBundleRetired
	}
	if bundle.polling {
		bundle.mu.Unlock()
		return errPOCFunctionBundleBusy
	}
	bundle.polling = true
	bundle.references++
	bundle.mu.Unlock()

	done := make(chan struct{})
	go func() {
		available := bundle.availability()
		bundle.mu.Lock()
		if !bundle.retired {
			bundle.available = available
		}
		bundle.polling = false
		bundle.references--
		start := bundle.startCleanupLocked()
		bundle.mu.Unlock()
		if start {
			go bundle.cleanup()
		}
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (bundle *pocStableFunctionBundle) snapshotAvailable() bool {
	bundle.mu.Lock()
	defer bundle.mu.Unlock()
	return !bundle.retired && bundle.available
}

func (bundle *pocStableFunctionBundle) startCleanupLocked() bool {
	if !bundle.retired || bundle.references != 0 || bundle.cleanupStart {
		return false
	}
	bundle.cleanupStart = true
	return true
}

func (bundle *pocStableFunctionBundle) cleanup() {
	bundle.handler.Cleanup(context.Background())
	close(bundle.cleanupDone)
}

func newPOCBundleCatalogGeneration(
	id string,
	candidates ...*pocStableFunctionBundle,
) (*HandlerGenerationDeclaration, error) {
	selected := make([]*pocStableFunctionBundle, 0, len(candidates))
	for _, bundle := range candidates {
		if bundle == nil || !bundle.snapshotAvailable() {
			continue
		}
		if err := bundle.acquire(); err != nil {
			for _, acquired := range selected {
				acquired.release()
			}
			return nil, err
		}
		selected = append(selected, bundle)
	}
	if len(selected) == 0 {
		return nil, errors.New("POC Function generation has no available bundles")
	}
	var cleanupOnce sync.Once
	return &HandlerGenerationDeclaration{
		ID: id,
		Handler: func(
			ctx context.Context,
			input HandlerInput,
		) (lifecycle.SealedResult, error) {
			response := selected[0].handler.Handle(ctx, input.Method, funcapi.ResolvedParams{})
			if response == nil {
				return functionErrorResult(500, "POC Function handler returned no response")
			}
			return functionJSONResult(200, map[string]any{
				"status": 200,
			})
		},
		Cleanup: func(context.Context) error {
			cleanupOnce.Do(func() {
				for _, bundle := range selected {
					bundle.release()
				}
			})
			return nil
		},
	}, nil
}

type pocBlockingFunctionHandler struct {
	cleanupEntered chan struct{}
	cleanupRelease chan struct{}
	releaseOnce    sync.Once
	cleanups       atomic.Int32
}

func newPOCBlockingFunctionHandler(cooperative bool) *pocBlockingFunctionHandler {
	handler := &pocBlockingFunctionHandler{
		cleanupEntered: make(chan struct{}),
		cleanupRelease: make(chan struct{}),
	}
	if cooperative {
		handler.releaseCleanup()
	}
	return handler
}

func (*pocBlockingFunctionHandler) MethodParams(
	context.Context,
	string,
) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (*pocBlockingFunctionHandler) Handle(
	context.Context,
	string,
	funcapi.ResolvedParams,
) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{Status: 200}
}

func (handler *pocBlockingFunctionHandler) Cleanup(context.Context) {
	close(handler.cleanupEntered)
	<-handler.cleanupRelease
	handler.cleanups.Add(1)
}

func (handler *pocBlockingFunctionHandler) releaseCleanup() {
	handler.releaseOnce.Do(func() {
		close(handler.cleanupRelease)
	})
}

func (handler *pocBlockingFunctionHandler) cleanupCount() int32 {
	return handler.cleanups.Load()
}
