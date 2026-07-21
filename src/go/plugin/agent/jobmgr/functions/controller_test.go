// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestFunctionControllerJobLifecycle(t *testing.T) {
	handler := &controllerTestHandler{}
	constructions := 0
	modules := collectorapi.Registry{
		"module": {
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				constructions++
				return handler
			},
		},
	}
	controller, catalog, err := NewController(1, modules)
	require.NoError(t, err)
	mutations := controllerTestMutationPort{catalog: catalog}
	publicationPort := newRecordingPublicationPort()
	publication, err := NewPublication(1, publicationPort)
	require.NoError(t, err)

	require.NoError(t, controller.Bind(&mutations, publication))

	require.NoError(t, controller.Activate())

	job := &controllerTestJob{
		fullName: "module_job", module: "module", name: "job", running: true,
	}
	handle, err := controller.PrepareJob(
		lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1},
		job,
	)
	require.NoError(t, err)

	require.NoError(t, handle.Publish())

	require.EqualValues(t, 1, constructions)
	ctx := context.Background()
	allocations := testing.AllocsPerRun(1_000, func() {
		if err := controller.ReconcileModule(ctx, "module"); err != nil {
			panic(err)
		}
	})
	require.EqualValues(t, 0, allocations)
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "request", Route: "module:method",
	})
	require.NoError(t, err)

	_, runTaskErr := decision.Plan.Work(context.Background())
	require.NoError(t, runTaskErr)

	cleanup, err := catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
	require.False(t, cleanup.Ref.Valid())

	require.NoError(t, handle.CloseAndDrain(context.Background()))

	require.NoError(t, handle.Cleanup(context.Background()))

	require.EqualValues(t, 1, handler.cleanupCount())

	got := publicationPort.events
	require.True(t, equalPublicationEvents(
		got,
		[]string{"publish:module:method", "withdraw:module:method"},
	))

	decision, err = catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "after-close", Route: "module:method",
	})
	require.NoError(t, err)
	require.NotEqualValues(t, 0, decision.Rejected)
}

func TestFunctionControllerClosesAdmissionBeforeExternalWithdrawal(t *testing.T) {
	handler := &controllerTestHandler{}
	controller, catalog, err := NewController(1, collectorapi.Registry{
		"module": {
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return handler
			},
		},
	})
	require.NoError(t, err)
	mutations := controllerTestMutationPort{catalog: catalog}
	publicationPort := &blockingWithdrawPublicationPort{
		recordingPublicationPort: newRecordingPublicationPort(),
		entered:                  make(chan struct{}),
		release:                  make(chan struct{}),
	}
	publication, err := NewPublication(1, publicationPort)
	require.NoError(t, err)

	require.NoError(t, controller.Bind(&mutations, publication))

	require.NoError(t, controller.Activate())

	job := &controllerTestJob{
		fullName: "module_job", module: "module", name: "job", running: true,
	}
	handle, err := controller.PrepareJob(
		lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1},
		job,
	)
	require.NoError(t, err)

	require.NoError(t, handle.Publish())

	closed := make(chan error, 1)
	go func() {
		closed <- handle.CloseAndDrain(context.Background())
	}()
	<-publicationPort.entered
	decision, resolveErr := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "during-withdraw", Route: "module:method",
	})
	if decision.Lease.Valid() {
		_, _ = catalog.ReleaseInvocation(decision.Lease)
	}
	close(publicationPort.release)
	closeErr := <-closed

	require.NoError(t, resolveErr)
	require.EqualValues(t, lifecycle.ControlUnavailable, decision.Rejected)
	require.NoError(t, closeErr)
}

func TestFunctionControllerRawRequestFidelity(t *testing.T) {
	handler := &controllerTestHandler{}
	modules := collectorapi.Registry{
		"module": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "raw", RawRequest: true}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return handler
			},
		},
	}
	controller, catalog, err := NewController(1, modules)
	require.NoError(t, err)
	mutations := controllerTestMutationPort{catalog: catalog}
	publicationPort := newRecordingPublicationPort()
	publication, err := NewPublication(1, publicationPort)
	require.NoError(t, err)

	require.NoError(t, controller.Bind(&mutations, publication))

	require.NoError(t, controller.Activate())

	lookup := jobmgr.FunctionLookup{
		UID: "raw-request", Route: "module:raw", Args: []string{"info", "arg"},
		Payload: []byte(`{"value":1}`), ContentType: "application/json",
		Permissions: "0x0013", CallerSource: "cloud",
		Timeout: 17 * time.Second, HasPayload: true,
	}
	decision, err := catalog.ResolveAndAcquire(lookup)
	require.NoError(t, err)

	_, runTaskErr := decision.Plan.Work(context.Background())
	require.NoError(t, runTaskErr)

	if cleanup, err := catalog.ReleaseInvocation(decision.Lease); err != nil {
		require.FailNow(t, "test failed", err)
	} else {
		require.False(t, cleanup.Ref.Valid())
	}
	got := handler.rawRequest()
	require.False(t, got.Method != "raw" || !got.Info ||
		got.ContentType != lookup.ContentType ||
		got.Permissions != lookup.Permissions ||
		got.Source != lookup.CallerSource ||
		got.Timeout != lookup.Timeout ||
		string(got.Payload) != string(lookup.Payload))
	got.Args[0] = "changed"
	got.Payload[0] = 'X'
	require.False(t, lookup.Args[0] != "info" || lookup.Payload[0] != '{')
}

func TestFunctionControllerAgentAvailabilityIsMonotonic(t *testing.T) {
	available := false
	handler := &controllerTestHandler{}
	modules := collectorapi.Registry{
		"module": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{
					ID: "delayed", Available: func() bool { return available },
				}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return handler
			},
		},
	}
	controller, catalog, err := NewController(1, modules)
	require.NoError(t, err)
	mutations := controllerTestMutationPort{catalog: catalog}
	publicationPort := newRecordingPublicationPort()
	publication, err := NewPublication(1, publicationPort)
	require.NoError(t, err)

	require.NoError(t, controller.Bind(&mutations, publication))

	require.NoError(t, controller.Activate())

	require.EqualValues(t, 0, len(publicationPort.events))
	available = true

	require.NoError(t, controller.ReconcileModule(context.Background(), "module"))

	available = false

	require.NoError(t, controller.ReconcileModule(context.Background(), "module"))

	got := publicationPort.events
	require.True(t, equalPublicationEvents(
		got,
		[]string{"publish:module:delayed"},
	))

	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "still-published", Route: "module:delayed",
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, decision.Rejected)

	_, releaseInvocationErr := catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, releaseInvocationErr)

}

func TestFunctionControllerRejectsInvalidDeclarations(t *testing.T) {
	tests := map[string]struct {
		methods []funcapi.FunctionConfig
	}{
		"empty method ID": {
			methods: []funcapi.FunctionConfig{{}},
		},
		"duplicate method ID": {
			methods: []funcapi.FunctionConfig{{ID: "same"}, {ID: "same"}},
		},
		"empty alias": {
			methods: []funcapi.FunctionConfig{{ID: "method", Aliases: []string{""}}},
		},
		"duplicate primary alias": {
			methods: []funcapi.FunctionConfig{{
				ID: "method", Aliases: []string{"module:method"},
			}},
		},
		"duplicate alias": {
			methods: []funcapi.FunctionConfig{{
				ID: "method", Aliases: []string{"alias", "alias"},
			}},
		},
		"invalid public name": {
			methods: []funcapi.FunctionConfig{{
				ID: "method", FunctionName: "invalid name",
			}},
		},
		"unmarshalable presentation": {
			methods: []funcapi.FunctionConfig{(funcapi.FunctionConfig{
				ID: "method",
			}).WithPresentation(func() {})},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := NewController(1, collectorapi.Registry{
				"module": {
					AgentFunctions: func() []funcapi.FunctionConfig {
						return test.methods
					},
					MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
						return &controllerTestHandler{}
					},
				},
			})
			require.Error(t, err)
		})
	}
}

func TestFunctionControllerReportsInitialRouteCleanupFailure(t *testing.T) {
	cleanupErr := errors.New("test cleanup")
	tests := map[string]struct {
		cleanup func(context.Context) error
		assert  func(*testing.T, error)
	}{
		"returned error": {
			cleanup: func(context.Context) error { return cleanupErr },
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, cleanupErr)
			},
		},
		"panic": {
			cleanup: func(context.Context) error {
				panic("test cleanup panic")
			},
			assert: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "initial route cleanup panic: test cleanup panic")
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			generation := &HandlerGenerationDeclaration{
				ID: "initial",
				Handler: func(context.Context, HandlerInput) (lifecycle.SealedResult, error) {
					return lifecycle.SealedResult{}, nil
				},
				Cleanup: test.cleanup,
			}
			valid := InitialRoute{
				Declaration: Declaration{
					ID:         "initial",
					Generation: generation,
					PublicName: "initial",
				},
				Publication: PublicationRecord{
					Name:       "initial",
					Generation: 1,
					Access:     "signed-id",
				},
			}
			invalid := valid
			invalid.Publication.Access = ""

			_, _, err := NewController(
				1,
				collectorapi.Registry{},
				valid,
				invalid,
			)
			require.ErrorContains(t, err, "invalid initial publication")
			test.assert(t, err)
		})
	}
}

func TestFunctionControllerReportsUnpublishedGroupCleanupFailure(t *testing.T) {
	_, _, err := NewController(1, collectorapi.Registry{
		"a": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &controllerPanickingCleanupHandler{}
			},
		},
		"z": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return nil
			},
		},
	})
	require.ErrorContains(t, err, "nil agent handler")
	require.ErrorIs(t, err, lifecycle.ErrTaskPanic)
	require.ErrorContains(t, err, "test handler cleanup panic")
}

type controllerTestMutationPort struct {
	catalog *Catalog
}

func (ctmp *controllerTestMutationPort) QuiesceFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) error {
	if ctmp == nil || ctmp.catalog == nil {
		return errors.New("nil mutation port")
	}
	if err := ctmp.catalog.BeginMutation(mutation); err != nil {
		return err
	}
	for {
		progress, err := ctmp.catalog.AdvanceMutationQuiesce(jobmgr.MaximumFunctionMutationQuantum)
		if err != nil {
			return err
		}
		if progress.Quiesced {
			return nil
		}
	}
}

func (ctmp *controllerTestMutationPort) CommitFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) (uint64, error) {
	if ctmp == nil || ctmp.catalog == nil {
		return 0, errors.New("nil mutation port")
	}
	if err := ctmp.catalog.ResumeMutation(mutation); err != nil {
		return 0, err
	}
	for {
		var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
		progress, count, err := ctmp.catalog.AdvanceMutation(jobmgr.MaximumFunctionMutationQuantum, &cleanups)
		if err != nil {
			return 0, err
		}
		for index := range count {
			cleanup := cleanups[index]
			_, cleanupErr := cleanup.Work(context.Background())
			if err := ctmp.catalog.CompleteCleanup(cleanup.Ref); err != nil {
				return 0, errors.Join(cleanupErr, err)
			}
		}
		if progress.Done {
			return progress.Version, nil
		}
	}
}

func (ctmp *controllerTestMutationPort) AbortFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) error {
	if ctmp == nil || ctmp.catalog == nil {
		return errors.New("nil mutation port")
	}
	if err := ctmp.catalog.ResumeMutation(mutation); err != nil {
		return err
	}
	var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
	count, err := ctmp.catalog.AbortMutation(&cleanups)
	if err != nil {
		return err
	}
	for index := range count {
		cleanup := cleanups[index]
		_, cleanupErr := cleanup.Work(context.Background())
		if err := ctmp.catalog.CompleteCleanup(cleanup.Ref); err != nil {
			return errors.Join(cleanupErr, err)
		}
	}
	return nil
}

type controllerTestJob struct {
	fullName  string
	module    string
	name      string
	running   bool
	collector any
}

func (job *controllerTestJob) FullName() string   { return job.fullName }
func (job *controllerTestJob) ModuleName() string { return job.module }
func (job *controllerTestJob) Name() string       { return job.name }
func (job *controllerTestJob) IsRunning() bool    { return job.running }
func (job *controllerTestJob) Collector() any     { return job.collector }

type controllerTestHandler struct {
	mu       sync.Mutex
	raw      funcapi.RawMethodRequest
	cleanups int
}

type controllerPanickingCleanupHandler struct {
	controllerTestHandler
}

func (*controllerPanickingCleanupHandler) Cleanup(context.Context) {
	panic("test handler cleanup panic")
}

type blockingWithdrawPublicationPort struct {
	*recordingPublicationPort
	entered chan struct{}
	release chan struct{}
}

func (bwpp *blockingWithdrawPublicationPort) Withdraw(
	handle PublicationHandle,
) error {
	close(bwpp.entered)
	<-bwpp.release
	return bwpp.recordingPublicationPort.Withdraw(handle)
}

func (cth *controllerTestHandler) MethodParams(
	context.Context,
	string,
) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (cth *controllerTestHandler) Handle(
	context.Context,
	string,
	funcapi.ResolvedParams,
) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{Status: 200}
}

func (cth *controllerTestHandler) HandleRaw(
	_ context.Context,
	request funcapi.RawMethodRequest,
) *funcapi.FunctionResponse {
	cth.mu.Lock()
	cth.raw = request
	cth.mu.Unlock()
	return &funcapi.FunctionResponse{Status: 200}
}

func (cth *controllerTestHandler) Cleanup(context.Context) {
	cth.mu.Lock()
	cth.cleanups++
	cth.mu.Unlock()
}

func (cth *controllerTestHandler) rawRequest() funcapi.RawMethodRequest {
	cth.mu.Lock()
	defer cth.mu.Unlock()
	return cth.raw
}

func (cth *controllerTestHandler) cleanupCount() int {
	cth.mu.Lock()
	defer cth.mu.Unlock()
	return cth.cleanups
}
