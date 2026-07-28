// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

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
		Key:       "1/module/agent",
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
		Key:       "1/module/agent",
		Resource:  "module",
	}
	released, ok := attempts.ProcessAttemptReleased(identity)
	require.True(t, ok)
	close(handler.cleanupRelease)
	<-released
	require.EqualValues(t, containment.Census{}, attempts.Census())
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

func (handler *moduleStageBlockingHandler) Cleanup(context.Context) {
	handler.cleanupOnce.Do(func() {
		close(handler.cleanupEntered)
	})
	<-handler.cleanupRelease
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

func (authority *controlledModuleAdmissionAuthority) StartProcessAttempt(
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	work := plan.Work
	plan.Work = func(
		ctx context.Context,
		admission jobmgr.ProcessAttemptAdmission,
	) error {
		return work(ctx, &controlledModuleAdmission{
			ProcessAttemptAdmission: admission,
			authority:               authority,
		})
	}
	delegate, err := authority.delegate.StartProcessAttempt(plan)
	if err != nil {
		return nil, err
	}
	authority.attempt = delegate
	return delegate, nil
}

func (authority *controlledModuleAdmissionAuthority) SupersedeProcessAttempt(
	ctx context.Context,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	return authority.delegate.SupersedeProcessAttempt(ctx, identity)
}

func (authority *controlledModuleAdmissionAuthority) CutProcessAttempt(
	identity jobmgr.ProcessAttemptIdentity,
	cause error,
) bool {
	return authority.delegate.CutProcessAttempt(identity, cause)
}

func (authority *controlledModuleAdmissionAuthority) ProcessAttemptReleased(
	identity jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return authority.delegate.ProcessAttemptReleased(identity)
}

func (admission *controlledModuleAdmission) Admit() error {
	if err := admission.ProcessAttemptAdmission.Admit(); err != nil {
		return err
	}
	admission.admitOnce.Do(func() {
		close(admission.authority.admitted)
	})
	<-admission.authority.allowAdmitReturn
	return nil
}
