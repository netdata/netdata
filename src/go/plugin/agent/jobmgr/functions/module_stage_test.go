// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

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
