// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
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
	require.True(t, bundle.quarantined)

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
	require.False(t, bundle.quarantined)

	_, err = bundle.invoke(context.Background(), func(context.Context) (lifecycle.SealedResult, error) {
		calls++
		return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, calls)

	bundle.retire()
	require.NoError(t, bundle.wait(context.Background()))
}

func TestFunctionBundleQuarantineWaitsForEveryActiveInvocation(t *testing.T) {
	delegate, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	attempts := &gatedSecondAwaitAuthority{
		delegate:          delegate,
		secondSettled:     make(chan struct{}),
		allowSecondReturn: make(chan struct{}),
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
			attempts.allowSecondReturn,
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
	<-attempts.secondSettled
	cancelFirst()
	require.NoError(t, <-firstDone)
	require.True(t, bundle.quarantined)

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

	close(attempts.allowSecondReturn)
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
	require.False(t, bundle.quarantined)

	_, err = bundle.invoke(context.Background(), func(context.Context) (lifecycle.SealedResult, error) {
		calls++
		return lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, calls)
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
	require.ErrorIs(t, (<-poll.result).err, jobmgr.ErrProcessAttemptSuperseded)
	poll.bundle.release()
	blocking.Store(false)

	poll, err = bundle.startAvailabilityPoll()
	require.NoError(t, err)
	require.NoError(t, poll.attempt.Await(context.Background()))
	require.NoError(t, (<-poll.result).err)
	poll.bundle.release()
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
	require.ErrorIs(t, (<-poll.result).err, jobmgr.ErrProcessAttemptSuperseded)
	poll.bundle.release()

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
	time.Sleep(20 * time.Millisecond)
	require.Zero(t, handler.cleanupCount())

	close(release)
	<-poll.attempt.Released()
	require.NoError(t, (<-poll.result).err)
	poll.bundle.release()
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

type gatedSecondAwaitAuthority struct {
	delegate          jobmgr.ProcessAttemptAuthority
	started           atomic.Int32
	secondSettled     chan struct{}
	allowSecondReturn chan struct{}
}

func (authority *gatedSecondAwaitAuthority) StartProcessAttempt(
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	attempt, err := authority.delegate.StartProcessAttempt(plan)
	if err != nil || authority.started.Add(1) != 2 {
		return attempt, err
	}
	return &gatedAwaitAttempt{
		ProcessAttempt: attempt,
		settled:        authority.secondSettled,
		allowReturn:    authority.allowSecondReturn,
	}, nil
}

func (authority *gatedSecondAwaitAuthority) SupersedeProcessAttempt(
	ctx context.Context,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	return authority.delegate.SupersedeProcessAttempt(ctx, identity)
}

func (authority *gatedSecondAwaitAuthority) CutProcessAttempt(
	identity jobmgr.ProcessAttemptIdentity,
	cause error,
) bool {
	return authority.delegate.CutProcessAttempt(identity, cause)
}

func (authority *gatedSecondAwaitAuthority) ProcessAttemptReleased(
	identity jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return authority.delegate.ProcessAttemptReleased(identity)
}

type gatedAwaitAttempt struct {
	jobmgr.ProcessAttempt
	settled     chan struct{}
	allowReturn chan struct{}
	once        sync.Once
}

func (attempt *gatedAwaitAttempt) Await(ctx context.Context) error {
	err := attempt.ProcessAttempt.Await(ctx)
	attempt.once.Do(func() { close(attempt.settled) })
	<-attempt.allowReturn
	return err
}

func (port *availabilityPublicationPort) Publish(record PublicationRecord) error {
	port.published <- record
	return nil
}

func (*availabilityPublicationPort) Withdraw(string) error {
	return nil
}
