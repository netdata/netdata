// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestModulePlanResultDoesNotAcceptAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transfer := newModulePlanTransfer()
	result := modulePlanResult{transfer: transfer}

	err := acceptModulePlanResult(ctx, &result)

	require.ErrorIs(t, err, context.Canceled)
	require.False(t, transfer.wasAccepted())
}

func TestContainedControllerAcceptsInvalidUTF8ModuleIdentity(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	module := string([]byte{0xff})

	controller, catalog, err := NewContainedController(
		context.Background(),
		1,
		attempts,
		collectorapi.Registry{module: {}},
	)
	require.NoError(t, err)
	require.NotNil(t, controller)
	require.NotNil(t, catalog)
	require.NoError(t, controller.AbortConstruction(context.Background()))
	attempts.BeginShutdown()
	require.NoError(t, attempts.Shutdown(context.Background()))
}

func TestContainedControllerAcceptsLongModuleIdentity(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	module := strings.Repeat("m", 4097)

	controller, catalog, err := NewContainedController(
		context.Background(),
		1,
		attempts,
		collectorapi.Registry{module: {}},
	)
	require.NoError(t, err)
	require.NotNil(t, controller)
	require.NotNil(t, catalog)
	require.NoError(t, controller.AbortConstruction(context.Background()))
	attempts.BeginShutdown()
	require.NoError(t, attempts.Shutdown(context.Background()))
}

func TestContainedControllerConsumesPlanPublishedBeforeSuccessfulSettlement(t *testing.T) {
	for index := range 64 {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			attempts, err := containment.NewAuthority(nil)
			require.NoError(t, err)
			controller, _, err := NewContainedController(
				context.Background(),
				1,
				attempts,
				collectorapi.Registry{"module": {}},
			)
			require.NoError(t, err)
			require.NoError(t, controller.AbortConstruction(context.Background()))
		})
	}
}

func TestContainedControllerConstructionSettlesWhileModuleCallbackRemainsOwned(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, buildErr := NewContainedController(
			ctx,
			1,
			attempts,
			collectorapi.Registry{
				"module": {
					AgentFunctions: func() []funcapi.FunctionConfig {
						close(entered)
						<-release
						return nil
					},
				},
			},
		)
		result <- buildErr
	}()
	<-entered
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	require.EqualValues(t, containment.Census{
		Active:    1,
		Contained: 1,
	}, attempts.Census())

	identity := jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptFunctionBundle,
		Key:       modulePlanAttemptIdentity("module").Key,
		Resource:  "module",
	}
	released, ok := attempts.ProcessAttemptReleased(identity)
	require.True(t, ok)
	close(release)
	<-released
	require.EqualValues(t, containment.Census{}, attempts.Census())
}

func TestContainedControllerAbortDoesNotWaitForHandlerCleanup(t *testing.T) {
	handler := &moduleStageBlockingHandler{
		cleanupEntered: make(chan struct{}),
		cleanupRelease: make(chan struct{}),
	}
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	controller, _, err := NewContainedController(
		context.Background(),
		1,
		attempts,
		collectorapi.Registry{
			"module": {
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "method"}}
				},
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					return handler
				},
			},
		},
	)
	require.NoError(t, err)

	require.NoError(t, controller.AbortConstruction(context.Background()))
	<-handler.cleanupEntered
	require.EqualValues(t, containment.Census{
		Active:   1,
		Admitted: 1,
	}, attempts.Census())

	identity := jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptFunctionBundle,
		Key:       modulePlanAttemptIdentity("module").Key,
		Resource:  "module",
	}
	released, ok := attempts.ProcessAttemptReleased(identity)
	require.True(t, ok)
	close(handler.cleanupRelease)
	<-released
	require.EqualValues(t, containment.Census{}, attempts.Census())
}

func TestContainedControllerWaitsForPriorEpochModuleRelease(t *testing.T) {
	handler := &moduleStageBlockingHandler{
		cleanupEntered: make(chan struct{}),
		cleanupRelease: make(chan struct{}),
	}
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	modules := collectorapi.Registry{
		"module": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return handler
			},
		},
	}
	first, _, err := NewContainedController(context.Background(), 1, attempts, modules)
	require.NoError(t, err)
	require.NoError(t, first.AbortConstruction(context.Background()))
	<-handler.cleanupEntered

	type controllerResult struct {
		controller *Controller
		err        error
	}
	secondResult := make(chan controllerResult, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		controller, _, buildErr := NewContainedController(ctx, 2, attempts, modules)
		secondResult <- controllerResult{controller: controller, err: buildErr}
	}()
	select {
	case result := <-secondResult:
		if result.controller != nil {
			_ = result.controller.AbortConstruction(context.Background())
		}
		require.FailNow(t, "test failed", "successor module overlapped prior cleanup")
	case <-time.After(20 * time.Millisecond):
	}

	close(handler.cleanupRelease)
	result := <-secondResult
	require.NoError(t, result.err)
	require.NotNil(t, result.controller)
	require.NoError(t, result.controller.AbortConstruction(context.Background()))
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{})
	}, time.Second, time.Millisecond)
	require.NoError(t, attempts.Shutdown(context.Background()))
}

func TestContainedControllerRejectsPriorEpochModuleQuarantine(t *testing.T) {
	handler := &moduleStagePanicCleanupHandler{}
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	modules := collectorapi.Registry{
		"module": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return handler
			},
		},
	}
	first, _, err := NewContainedController(context.Background(), 1, attempts, modules)
	require.NoError(t, err)
	require.NoError(t, first.AbortConstruction(context.Background()))
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{Quarantined: 1})
	}, time.Second, time.Millisecond)

	second, _, err := NewContainedController(context.Background(), 2, attempts, modules)
	require.Nil(t, second)
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptQuarantined)
	require.Equal(t, containment.Census{Quarantined: 1}, attempts.Census())
	require.NoError(t, attempts.Shutdown(context.Background()))
}

func TestContainedControllerCanceledAfterModuleAdmissionCleansAbandonedPlan(t *testing.T) {
	delegate, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	attempts := newControlledModuleAdmissionAuthority(delegate)
	handler := &controllerTestHandler{}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, _, buildErr := NewContainedController(
			ctx,
			1,
			attempts,
			collectorapi.Registry{
				"module": {
					AgentFunctions: func() []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: "method"}}
					},
					MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
						return handler
					},
				},
			},
		)
		result <- buildErr
	}()

	<-attempts.admitted
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
	close(attempts.allowAdmitReturn)
	require.Eventually(t, func() bool {
		return handler.cleanupCount() == 1
	}, time.Second, time.Millisecond)
	select {
	case <-attempts.attempt.Released():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "abandoned module-plan attempt was not released")
	}
	require.EqualValues(t, containment.Census{}, delegate.Census())
	require.NoError(t, delegate.Shutdown(context.Background()))
}

type moduleStageBlockingHandler struct {
	cleanupEntered chan struct{}
	cleanupRelease chan struct{}
	cleanupOnce    sync.Once
}

type moduleStagePanicCleanupHandler struct{}

func (*moduleStagePanicCleanupHandler) MethodParams(
	context.Context,
	string,
) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (*moduleStagePanicCleanupHandler) Handle(
	context.Context,
	string,
	funcapi.ResolvedParams,
) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{Status: 200}
}

func (*moduleStagePanicCleanupHandler) Cleanup(context.Context) {
	panic("cleanup failed")
}

func (*moduleStageBlockingHandler) MethodParams(
	context.Context,
	string,
) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (*moduleStageBlockingHandler) Handle(
	context.Context,
	string,
	funcapi.ResolvedParams,
) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{Status: 200}
}

func (msbh *moduleStageBlockingHandler) Cleanup(context.Context) {
	msbh.cleanupOnce.Do(func() {
		close(msbh.cleanupEntered)
	})
	<-msbh.cleanupRelease
}

type controlledModuleAdmissionAuthority struct {
	delegate         *containment.Authority
	admitted         chan struct{}
	allowAdmitReturn chan struct{}
	attempt          jobmgr.ProcessAttempt
}

type controlledModuleAdmission struct {
	jobmgr.ProcessAttemptAdmission
	authority *controlledModuleAdmissionAuthority
	admitOnce sync.Once
}

func newControlledModuleAdmissionAuthority(
	delegate *containment.Authority,
) *controlledModuleAdmissionAuthority {
	return &controlledModuleAdmissionAuthority{
		delegate:         delegate,
		admitted:         make(chan struct{}),
		allowAdmitReturn: make(chan struct{}),
	}
}

func (cmaa *controlledModuleAdmissionAuthority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	work := plan.Work
	plan.Work = func(
		ctx context.Context,
		admission jobmgr.ProcessAttemptAdmission,
	) error {
		return work(ctx, &controlledModuleAdmission{
			ProcessAttemptAdmission: admission,
			authority:               cmaa,
		})
	}
	delegate, err := cmaa.delegate.StartProcessAttempt(ctx, plan)
	if err != nil {
		return nil, err
	}
	cmaa.attempt = delegate
	return delegate, nil
}

func (cmaa *controlledModuleAdmissionAuthority) SupersedeProcessAttempt(
	ctx context.Context,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	return cmaa.delegate.SupersedeProcessAttempt(ctx, identity)
}

func (cmaa *controlledModuleAdmissionAuthority) CutProcessAttempt(
	identity jobmgr.ProcessAttemptIdentity,
	cause error,
) bool {
	return cmaa.delegate.CutProcessAttempt(identity, cause)
}

func (cmaa *controlledModuleAdmissionAuthority) ProcessAttemptReleased(
	identity jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return cmaa.delegate.ProcessAttemptReleased(identity)
}

func (cma *controlledModuleAdmission) Admit() error {
	if err := cma.ProcessAttemptAdmission.Admit(); err != nil {
		return err
	}
	cma.admitOnce.Do(func() {
		close(cma.authority.admitted)
	})
	<-cma.authority.allowAdmitReturn
	return nil
}
