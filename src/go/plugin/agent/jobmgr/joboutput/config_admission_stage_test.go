// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestConfigAdmissionRejectsWhenGraphReacquisitionFails(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := controller.factory.config.Attempts.(*containment.Authority)
	reacquireErr := errors.New("graph reacquisition rejected")
	controller.factory.runWithoutClaims = func(ctx context.Context, work func(context.Context) error) (error, error) {
		return work(ctx), reacquireErr
	}
	result, err := controller.runPreparedConfigOperation(t.Context(), factoryTestConfig(false), configOperationAdmit, true,
		func(context.Context, confgroup.Config) configOperationResult {
			return configOperationResult{payload: []byte("validated")}
		})
	require.ErrorIs(t, err, reacquireErr)
	require.Nil(t, result.payload)
	require.Eventually(t, func() bool { return attempts.Census() == (containment.Census{}) }, time.Second, time.Millisecond)
}

func TestConfigAdmissionRejectsCancellationDuringGraphReacquisition(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := controller.factory.config.Attempts.(*containment.Authority)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	controller.factory.runWithoutClaims = func(ctx context.Context, work func(context.Context) error) (error, error) {
		err := work(ctx)
		cancel()
		return err, nil
	}
	result, err := controller.runPreparedConfigOperation(ctx, factoryTestConfig(false), configOperationAdmit, true,
		func(context.Context, confgroup.Config) configOperationResult {
			return configOperationResult{payload: []byte("validated")}
		})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result.payload)
	require.Eventually(t, func() bool { return attempts.Census() == (containment.Census{}) }, time.Second, time.Millisecond)
}

func TestConfigAdmissionCallerCancellationLeavesLateWorkContained(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := controller.factory.config.Attempts.(*containment.Authority)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	done := make(chan error, 1)
	go func() {
		result, err := controller.runPreparedConfigOperation(ctx, factoryTestConfig(false), configOperationAdmit, true,
			func(context.Context, confgroup.Config) configOperationResult {
				close(entered)
				<-release
				return configOperationResult{payload: []byte("validated")}
			})
		if result.payload != nil {
			err = errors.New("cancelled caller took a result")
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("validation did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("caller did not cancel")
	}
	// Work can enter before StartProcessAttempt returns its handle to the stage.
	// Cancellation is cut as soon as that registration finishes.
	require.Eventually(t, func() bool { return attempts.Census().Contained == 1 }, time.Second, time.Millisecond)
	releaseOnce.Do(func() { close(release) })
	require.Eventually(t, func() bool { return attempts.Census() == (containment.Census{}) }, time.Second, time.Millisecond)
}

func TestConfigAdmissionPreservesLateOwnershipFailureAfterCancellation(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := controller.factory.config.Attempts.(*containment.Authority)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	stage, err := newPreparedConfigOperation(controller.factory, factoryTestConfig(false), configOperationAdmit,
		func(context.Context, confgroup.Config) configOperationResult {
			close(entered)
			<-release
			return configOperationResult{err: lifecycle.RetainOwnership(errors.New("validation cleanup failed"))}
		})
	require.NoError(t, err)
	t.Cleanup(stage.Release)
	stage.Start()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("operation did not start")
	}
	stage.Cancel(context.Canceled)
	requireConfigOperationReady(t, stage)
	stage.Release()
	releaseOnce.Do(func() { close(release) })
	require.Eventually(t, func() bool { return attempts.Census().Active == 0 }, time.Second, time.Millisecond)
	require.Equal(t, 1, attempts.Census().Quarantined)
}
