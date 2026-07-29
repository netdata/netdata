// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestFunctionBundleRejectsTypedNilMethodHandler(t *testing.T) {
	var handler *typedNilBundleTestHandler
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return handler
			},
		},
		[]funcapi.FunctionConfig{{ID: "method"}},
	)
	if bundle != nil {
		bundle.retire()
		require.NoError(t, bundle.wait(context.Background()))
	}
	require.Nil(t, bundle)
	require.ErrorContains(t, err, "nil method handler")
}

func TestFunctionBundleMarksFailedConstructionRollbackRetained(t *testing.T) {
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &failedRollbackBundleTestHandler{}
			},
		},
		[]funcapi.FunctionConfig{{
			ID: "method",
			Available: func() bool {
				panic("availability failed")
			},
		}},
	)
	require.Nil(t, bundle)
	require.Error(t, err)
	require.ErrorIs(t, err, lifecycle.ErrTaskPanic)
	require.True(t, lifecycle.OwnershipRetained(err))
}

func TestFunctionBundleQuarantinesOnlyAfterRetainedInvocation(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &controllerTestHandler{}
			},
		},
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, bundle.bindContainment(attempts, 1, "1/module/agent", "module"))

	entered := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	first := make(chan error, 1)
	go func() {
		_, invokeErr := bundle.invoke(ctx, func(context.Context) (lifecycle.SealedResult, error) {
			close(entered)
			<-release
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})
		first <- invokeErr
	}()
	<-entered
	cancel()
	require.NoError(t, <-first)
	require.True(t, functionBundleQuarantined(bundle))

	calls := 0
	_, err = bundle.invoke(context.Background(), func(context.Context) (lifecycle.SealedResult, error) {
		calls++
		return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	})
	require.NoError(t, err)
	require.Zero(t, calls)

	identity := jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptFunctionInvocation,
		Key:       "1/module/agent/invocation/1",
		Resource:  "module",
	}
	released, ok := attempts.ProcessAttemptReleased(identity)
	require.True(t, ok)
	close(release)
	<-released
	require.False(t, functionBundleQuarantined(bundle))

	_, err = bundle.invoke(context.Background(), func(context.Context) (lifecycle.SealedResult, error) {
		calls++
		return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, calls)

	bundle.retire()
	require.NoError(t, bundle.wait(context.Background()))
}

func TestFunctionBundleDoesNotStartInvocationAfterCallerCancellation(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		attempts.BeginShutdown()
		require.NoError(t, attempts.Shutdown(context.Background()))
	})
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &controllerTestHandler{}
			},
		},
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, bundle.bindContainment(attempts, 1, "bundle", "module"))
	t.Cleanup(func() {
		bundle.retire()
		require.NoError(t, bundle.wait(context.Background()))
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32

	_, err = bundle.invoke(ctx, func(context.Context) (lifecycle.SealedResult, error) {
		calls.Add(1)
		return lifecycle.NewSealedResult(200, "application/json", nil)
	})

	require.NoError(t, err)
	bundle.mu.Lock()
	invocationID := bundle.invocationID
	bundle.mu.Unlock()
	require.Zero(t, invocationID)
	require.Zero(t, calls.Load())
	require.Equal(t, containment.Census{}, attempts.Census())
}

func TestFunctionBundlePanicPermanentlyClosesBundleWithoutInvocationTombstones(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &controllerTestHandler{}
			},
		},
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, bundle.bindContainment(attempts, 1, "1/module/agent", "module"))

	calls := 0
	for range 3 {
		_, err = bundle.invoke(context.Background(), func(context.Context) (lifecycle.SealedResult, error) {
			calls++
			panic("handler failed")
		})
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, calls)
	require.True(t, functionBundleQuarantined(bundle))
	require.Zero(t, attempts.Census().Quarantined)

	bundle.retire()
	require.NoError(t, bundle.wait(context.Background()))
	require.NoError(t, attempts.Shutdown(context.Background()))
}

type typedNilBundleTestHandler struct{}

func (*typedNilBundleTestHandler) MethodParams(
	context.Context,
	string,
) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (*typedNilBundleTestHandler) Handle(
	context.Context,
	string,
	funcapi.ResolvedParams,
) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{Status: 200}
}

func (*typedNilBundleTestHandler) Cleanup(context.Context) {}

type failedRollbackBundleTestHandler struct {
	typedNilBundleTestHandler
}

func (*failedRollbackBundleTestHandler) Cleanup(context.Context) {
	panic(errors.New("cleanup failed"))
}

func TestFunctionBundleQuarantineWaitsForEveryActiveInvocation(t *testing.T) {
	delegate, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	attempts := &gatedAwaitAuthority{
		delegate:    delegate,
		namespace:   jobmgr.ProcessAttemptFunctionInvocation,
		ordinal:     2,
		settled:     make(chan struct{}),
		allowReturn: make(chan struct{}),
	}
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &controllerTestHandler{}
			},
		},
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, bundle.bindContainment(attempts, 1, "1/module/agent", "module"))

	firstEntered := make(chan struct{})
	firstRelease := make(chan struct{})
	secondEntered := make(chan struct{})
	secondRelease := make(chan struct{})
	t.Cleanup(func() {
		for _, signal := range []chan struct{}{
			firstRelease,
			secondRelease,
			attempts.allowReturn,
		} {
			select {
			case <-signal:
			default:
				close(signal)
			}
		}
		bundle.retire()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = bundle.wait(ctx)
		_ = delegate.Shutdown(ctx)
	})

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, invokeErr := bundle.invoke(firstCtx, func(context.Context) (lifecycle.SealedResult, error) {
			close(firstEntered)
			<-firstRelease
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})
		firstDone <- invokeErr
	}()
	<-firstEntered

	secondCtx, cancelSecond := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		_, invokeErr := bundle.invoke(secondCtx, func(context.Context) (lifecycle.SealedResult, error) {
			close(secondEntered)
			<-secondRelease
			return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
		})
		secondDone <- invokeErr
	}()
	<-secondEntered

	cancelSecond()
	<-attempts.settled
	cancelFirst()
	require.NoError(t, <-firstDone)
	require.True(t, functionBundleQuarantined(bundle))

	firstIdentity := jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptFunctionInvocation,
		Key:       "1/module/agent/invocation/1",
		Resource:  "module",
	}
	firstReleased, ok := attempts.ProcessAttemptReleased(firstIdentity)
	require.True(t, ok)
	close(firstRelease)
	<-firstReleased

	calls := 0
	_, err = bundle.invoke(context.Background(), func(context.Context) (lifecycle.SealedResult, error) {
		calls++
		return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	})
	require.NoError(t, err)
	require.Zero(t, calls)

	close(attempts.allowReturn)
	require.NoError(t, <-secondDone)
	secondIdentity := jobmgr.ProcessAttemptIdentity{
		Namespace: jobmgr.ProcessAttemptFunctionInvocation,
		Key:       "1/module/agent/invocation/2",
		Resource:  "module",
	}
	secondReleased, ok := attempts.ProcessAttemptReleased(secondIdentity)
	require.True(t, ok)
	close(secondRelease)
	<-secondReleased
	require.False(t, functionBundleQuarantined(bundle))

	_, err = bundle.invoke(context.Background(), func(context.Context) (lifecycle.SealedResult, error) {
		calls++
		return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, calls)
}

func TestFunctionBundleQuarantineRejectsAvailabilityPoll(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	var availabilityCalls atomic.Int32
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &controllerTestHandler{}
			},
		},
		[]funcapi.FunctionConfig{{
			ID: "method",
			Available: func() bool {
				availabilityCalls.Add(1)
				return true
			},
		}},
	)
	require.NoError(t, err)
	require.NoError(t, bundle.bindContainment(attempts, 1, "1/module/agent", "module"))
	baseline := availabilityCalls.Load()

	entered := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, invokeErr := bundle.invoke(ctx, func(context.Context) (lifecycle.SealedResult, error) {
			close(entered)
			<-release
			return lifecycle.NewSealedResult(200, "application/json", nil)
		})
		done <- invokeErr
	}()
	<-entered
	cancel()
	require.NoError(t, <-done)
	require.True(t, functionBundleQuarantined(bundle))

	poll, err := bundle.startAvailabilityPoll()
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptBusy)
	require.Nil(t, poll.attempt)
	require.Equal(t, baseline, availabilityCalls.Load())

	close(release)
	require.Eventually(t, func() bool {
		return !functionBundleQuarantined(bundle)
	}, time.Second, time.Millisecond)
	bundle.retire()
	require.NoError(t, bundle.wait(context.Background()))
}

func TestRetainedAvailabilityPollQuarantinesInvocations(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	var blocking atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &controllerTestHandler{}
			},
		},
		[]funcapi.FunctionConfig{{
			ID: "method",
			Available: func() bool {
				if blocking.Load() {
					close(entered)
					<-release
				}
				return true
			},
		}},
	)
	require.NoError(t, err)
	require.NoError(t, bundle.bindContainment(attempts, 1, "1/module/agent", "module"))
	blocking.Store(true)

	poll, err := bundle.startAvailabilityPoll()
	require.NoError(t, err)
	<-entered
	require.True(t, poll.attempt.Cut(jobmgr.ErrProcessAttemptSuperseded))
	require.ErrorIs(t, poll.attempt.Await(context.Background()), jobmgr.ErrProcessAttemptSuperseded)
	require.Eventually(t, func() bool {
		return functionBundleQuarantined(bundle)
	}, time.Second, time.Millisecond)

	calls := 0
	_, err = bundle.invoke(context.Background(), func(context.Context) (lifecycle.SealedResult, error) {
		calls++
		return lifecycle.NewSealedResult(200, "application/json", nil)
	})
	require.NoError(t, err)
	require.Zero(t, calls)

	close(release)
	<-poll.attempt.Released()
	require.Eventually(t, func() bool {
		return !functionBundleQuarantined(bundle)
	}, time.Second, time.Millisecond)
	bundle.retire()
	require.NoError(t, bundle.wait(context.Background()))
}

func TestContainedAvailabilityPollFinisherDoesNotWaitForPhysicalRelease(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		attempts.BeginShutdown()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, attempts.Shutdown(ctx))
	})
	var blocking atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})
	released := false
	defer func() {
		if !released {
			close(release)
		}
	}()
	creator := collectorapi.Creator{
		MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
			return &controllerTestHandler{}
		},
	}
	bundle, err := newAgentFunctionBundle(
		"module",
		creator,
		[]funcapi.FunctionConfig{{
			ID: "method",
			Available: func() bool {
				if blocking.Load() {
					close(entered)
					<-release
				}
				return true
			},
		}},
	)
	require.NoError(t, err)
	require.NoError(t, bundle.bindContainment(attempts, 1, "1/module/agent", "module"))
	blocking.Store(true)
	poll, err := bundle.startAvailabilityPoll()
	require.NoError(t, err)
	waitBundleContainmentTestValue(t, entered, "availability callback entry")
	finished := make(chan struct{})
	go func() {
		(&Controller{}).finishAvailabilityPoll("module", creator, poll)
		close(finished)
	}()

	require.True(t, poll.attempt.Cut(jobmgr.ErrProcessAttemptSuperseded))
	waitBundleContainmentTestValue(t, finished, "availability poll finisher")

	close(release)
	released = true
	<-poll.attempt.Released()
	bundle.retire()
	require.NoError(t, bundle.wait(context.Background()))
}

func TestContainedAvailabilityPollFencesInvocationBeforeAwaitReturns(t *testing.T) {
	delegate, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	attempts := &gatedAwaitAuthority{
		delegate:    delegate,
		namespace:   jobmgr.ProcessAttemptFunctionPoll,
		ordinal:     1,
		settled:     make(chan struct{}),
		allowReturn: make(chan struct{}),
	}
	var blocking atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &controllerTestHandler{}
			},
		},
		[]funcapi.FunctionConfig{{
			ID: "method",
			Available: func() bool {
				if blocking.Load() {
					close(entered)
					<-release
				}
				return true
			},
		}},
	)
	require.NoError(t, err)
	require.NoError(t, bundle.bindContainment(attempts, 1, "1/module/agent", "module"))
	blocking.Store(true)
	t.Cleanup(func() {
		for _, signal := range []chan struct{}{release, attempts.allowReturn} {
			select {
			case <-signal:
			default:
				close(signal)
			}
		}
		bundle.retire()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = bundle.wait(ctx)
		_ = delegate.Shutdown(ctx)
	})

	poll, err := bundle.startAvailabilityPoll()
	require.NoError(t, err)
	waitBundleContainmentTestValue(t, entered, "availability callback entry")
	finished := make(chan struct{})
	go func() {
		(&Controller{}).finishAvailabilityPoll("module", collectorapi.Creator{}, poll)
		close(finished)
	}()
	require.True(t, poll.attempt.Cut(jobmgr.ErrProcessAttemptSuperseded))
	waitBundleContainmentTestValue(t, attempts.settled, "availability poll settlement")

	calls := 0
	_, err = bundle.invoke(context.Background(), func(context.Context) (lifecycle.SealedResult, error) {
		calls++
		return lifecycle.NewSealedResult(200, "application/json", nil)
	})
	require.NoError(t, err)
	require.Zero(t, calls)

	close(attempts.allowReturn)
	waitBundleContainmentTestValue(t, finished, "availability poll finisher")
	close(release)
	<-poll.attempt.Released()
	require.Eventually(t, func() bool {
		return !functionBundleQuarantined(bundle)
	}, time.Second, time.Millisecond)
}

func TestContainedInvocationFencesSiblingBeforeAwaitReturns(t *testing.T) {
	delegate, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	attempts := &gatedAwaitAuthority{
		delegate:    delegate,
		namespace:   jobmgr.ProcessAttemptFunctionInvocation,
		ordinal:     1,
		settled:     make(chan struct{}),
		allowReturn: make(chan struct{}),
	}
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &controllerTestHandler{}
			},
		},
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, bundle.bindContainment(attempts, 1, "1/module/agent", "module"))
	entered := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	t.Cleanup(func() {
		cancel()
		for _, signal := range []chan struct{}{release, attempts.allowReturn} {
			select {
			case <-signal:
			default:
				close(signal)
			}
		}
		bundle.retire()
		waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
		defer waitCancel()
		_ = bundle.wait(waitCtx)
		_ = delegate.Shutdown(waitCtx)
	})

	go func() {
		_, invokeErr := bundle.invoke(ctx, func(context.Context) (lifecycle.SealedResult, error) {
			close(entered)
			<-release
			return lifecycle.NewSealedResult(200, "application/json", nil)
		})
		firstDone <- invokeErr
	}()
	waitBundleContainmentTestValue(t, entered, "invocation callback entry")
	cancel()
	waitBundleContainmentTestValue(t, attempts.settled, "invocation settlement")

	calls := 0
	_, err = bundle.invoke(context.Background(), func(context.Context) (lifecycle.SealedResult, error) {
		calls++
		return lifecycle.NewSealedResult(200, "application/json", nil)
	})
	require.NoError(t, err)
	require.Zero(t, calls)

	close(attempts.allowReturn)
	require.NoError(t, waitBundleContainmentTestValue(t, firstDone, "contained invocation result"))
	close(release)
	require.Eventually(t, func() bool {
		return !functionBundleQuarantined(bundle)
	}, time.Second, time.Millisecond)
}

func functionBundleQuarantined(bundle *functionBundle) bool {
	bundle.mu.Lock()
	defer bundle.mu.Unlock()
	return bundle.quarantined
}

func waitBundleContainmentTestValue[T any](
	t *testing.T,
	result <-chan T,
	name string,
) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		require.FailNowf(t, "test failed", "%s did not settle", name)
		var zero T
		return zero
	}
}

func TestFunctionBundleAvailabilityPollIsPerBundleBusy(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	var blocking atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &controllerTestHandler{}
			},
		},
		[]funcapi.FunctionConfig{{
			ID: "method",
			Available: func() bool {
				if blocking.Load() {
					close(entered)
					<-release
				}
				return true
			},
		}},
	)
	require.NoError(t, err)
	require.NoError(t, bundle.bindContainment(attempts, 1, "1/module/agent", "module"))
	blocking.Store(true)

	poll, err := bundle.startAvailabilityPoll()
	require.NoError(t, err)
	<-entered
	require.True(t, poll.attempt.Cut(jobmgr.ErrProcessAttemptSuperseded))
	require.ErrorIs(t, poll.attempt.Await(context.Background()), jobmgr.ErrProcessAttemptSuperseded)

	_, err = bundle.startAvailabilityPoll()
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptBusy)
	close(release)
	<-poll.attempt.Released()
	blocking.Store(false)

	poll, err = bundle.startAvailabilityPoll()
	require.NoError(t, err)
	require.NoError(t, poll.attempt.Await(context.Background()))
	require.NoError(t, (<-poll.workerResult).err)
	bundle.retire()
	require.NoError(t, bundle.wait(context.Background()))
}

func TestFunctionBundleCutAvailabilityPollCannotPublishLateResult(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	var available atomic.Bool
	var blocking atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &controllerTestHandler{}
			},
		},
		[]funcapi.FunctionConfig{{
			ID: "method",
			Available: func() bool {
				if blocking.Load() {
					close(entered)
					<-release
				}
				return available.Load()
			},
		}},
	)
	require.NoError(t, err)
	require.NoError(t, bundle.bindContainment(attempts, 1, "1/module/agent", "module"))
	available.Store(true)
	blocking.Store(true)

	poll, err := bundle.startAvailabilityPoll()
	require.NoError(t, err)
	<-entered
	require.True(t, poll.attempt.Cut(jobmgr.ErrProcessAttemptSuperseded))
	close(release)
	<-poll.attempt.Released()

	require.False(t, bundle.available("method"))
	bundle.retire()
	require.NoError(t, bundle.wait(context.Background()))
}

func TestFunctionBundleAvailabilityPollPreventsConcurrentCleanup(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	handler := &controllerTestHandler{}
	var blocking atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})
	bundle, err := newAgentFunctionBundle(
		"module",
		collectorapi.Creator{
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return handler
			},
		},
		[]funcapi.FunctionConfig{{
			ID: "method",
			Available: func() bool {
				if blocking.Load() {
					close(entered)
					<-release
				}
				return true
			},
		}},
	)
	require.NoError(t, err)
	require.NoError(t, bundle.bindContainment(attempts, 1, "1/module/agent", "module"))
	blocking.Store(true)

	poll, err := bundle.startAvailabilityPoll()
	require.NoError(t, err)
	<-entered
	bundle.retire()
	bundle.mu.Lock()
	cleanupStarted := bundle.cleanupStart
	references := bundle.references
	bundle.mu.Unlock()
	require.False(t, cleanupStarted)
	require.Positive(t, references)
	require.Zero(t, handler.cleanupCount())

	close(release)
	<-poll.attempt.Released()
	require.NoError(t, (<-poll.workerResult).err)
	require.Eventually(t, func() bool {
		return handler.cleanupCount() == 1
	}, time.Second, time.Millisecond)
	require.NoError(t, bundle.wait(context.Background()))
}

func TestContainedFunctionControllerPublishesSuccessfulAvailabilityTransition(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	var available atomic.Bool
	controller, catalog, err := NewContainedController(
		context.Background(),
		1,
		attempts,
		collectorapi.Registry{
			"module": {
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{
						ID:        "method",
						Available: available.Load,
					}}
				},
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					return &controllerTestHandler{}
				},
			},
		},
	)
	require.NoError(t, err)
	mutations := controllerTestMutationPort{catalog: catalog}
	publicationPort := &availabilityPublicationPort{
		published: make(chan PublicationRecord, 1),
	}
	publication, err := NewPublication(1, publicationPort)
	require.NoError(t, err)
	require.NoError(t, controller.Bind(&mutations, publication))
	require.NoError(t, controller.Activate())

	available.Store(true)
	require.NoError(t, controller.ReconcileModule(context.Background(), "module"))
	select {
	case record := <-publicationPort.published:
		require.Equal(t, "module:method", record.Name)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "availability transition was not published")
	}

	require.NoError(t, controller.BeginShutdown(1))
	require.NoError(t, catalog.BeginClose())
	for {
		cleanups, more, closeErr := catalog.CloseStep(jobmgr.MaximumFunctionCloseQuantum)
		require.NoError(t, closeErr)
		for _, cleanup := range cleanups {
			runCleanupPlan(t, catalog, cleanup)
		}
		if !more {
			break
		}
	}
	require.NoError(t, controller.Stop(1))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, attempts.Shutdown(shutdownCtx))
}

type availabilityPublicationPort struct {
	published chan PublicationRecord
}

type gatedAwaitAuthority struct {
	delegate    jobmgr.ProcessAttemptAuthority
	namespace   jobmgr.ProcessAttemptNamespace
	ordinal     int32
	started     atomic.Int32
	settled     chan struct{}
	allowReturn chan struct{}
}

func (gaa *gatedAwaitAuthority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	attempt, err := gaa.delegate.StartProcessAttempt(ctx, plan)
	if err != nil ||
		plan.Identity.Namespace != gaa.namespace ||
		gaa.started.Add(1) != gaa.ordinal {
		return attempt, err
	}
	return &gatedAwaitAttempt{
		ProcessAttempt: attempt,
		settled:        gaa.settled,
		allowReturn:    gaa.allowReturn,
	}, nil
}

func (gaa *gatedAwaitAuthority) SupersedeProcessAttempt(
	ctx context.Context,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	return gaa.delegate.SupersedeProcessAttempt(ctx, identity)
}

func (gaa *gatedAwaitAuthority) CutProcessAttempt(
	identity jobmgr.ProcessAttemptIdentity,
	cause error,
) bool {
	return gaa.delegate.CutProcessAttempt(identity, cause)
}

func (gaa *gatedAwaitAuthority) ProcessAttemptReleased(
	identity jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return gaa.delegate.ProcessAttemptReleased(identity)
}

type gatedAwaitAttempt struct {
	jobmgr.ProcessAttempt
	settled     chan struct{}
	allowReturn chan struct{}
	once        sync.Once
}

func (gat *gatedAwaitAttempt) Await(ctx context.Context) error {
	err := gat.ProcessAttempt.Await(ctx)
	gat.once.Do(func() { close(gat.settled) })
	<-gat.allowReturn
	return err
}

func (app *availabilityPublicationPort) Publish(record PublicationRecord) error {
	app.published <- record
	return nil
}

func (*availabilityPublicationPort) Withdraw(string) error {
	return nil
}
