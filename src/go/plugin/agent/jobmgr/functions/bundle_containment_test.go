// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"sync/atomic"
	"testing"

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
	require.EqualValues(t, 1, bundle.retained)
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
	require.Zero(t, bundle.retained)
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
	require.ErrorIs(t, <-poll.settled, jobmgr.ErrProcessAttemptSuperseded)

	_, err = bundle.startAvailabilityPoll()
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptBusy)
	close(release)
	<-poll.attempt.Released()
	blocking.Store(false)

	poll, err = bundle.startAvailabilityPoll()
	require.NoError(t, err)
	require.NoError(t, <-poll.settled)
	bundle.retire()
	require.NoError(t, bundle.wait(context.Background()))
}
