// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

type stagedIntentReservation struct {
	aborted      atomic.Int32
	abortEntered chan struct{}
	abortRelease chan struct{}
	abortErr     error
}

func (*stagedIntentReservation) Commit() { panic("rejected staged intent committed") }
func (r *stagedIntentReservation) Abort() error {
	r.aborted.Add(1)
	if r.abortEntered != nil {
		close(r.abortEntered)
	}
	if r.abortRelease != nil {
		<-r.abortRelease
	}
	return r.abortErr
}

func TestConfigIntentStageTransfersOrAbortsPublishedReservation(t *testing.T) {
	for _, take := range []bool{false, true} {
		name := "release"
		if take {
			name = "take"
		}
		t.Run(name, func(t *testing.T) {
			controller, _, _, _, _ := newDynCfgJobTestHarness(t)
			attempts := controller.factory.config.Attempts.(*containment.Authority)
			reservation := &stagedIntentReservation{}
			stage, err := newPreparedConfigOperation(controller.factory, factoryTestConfig(false), configOperationValidateIntent,
				func(context.Context, confgroup.Config) configOperationResult {
					return configOperationResult{intent: &preparedJobConfigIntent{reservation: reservation}}
				})
			require.NoError(t, err)
			t.Cleanup(stage.Release)
			require.NoError(t, stage.Await(t.Context()))
			require.Equal(t, 1, attempts.Census().Admitted)
			if take {
				result, err := stage.take()
				require.NoError(t, err)
				require.NotNil(t, result.intent)
				stage.Release()
				require.Equal(t, containment.Census{}, attempts.Census())
				require.Zero(t, reservation.aborted.Load())
				require.NoError(t, result.intent.abort())
				require.NoError(t, result.intent.abort())
			} else {
				stage.Release()
				_, err := stage.take()
				require.Error(t, err)
			}
			stage.Release()
			require.Eventually(t, func() bool { return attempts.Census() == (containment.Census{}) }, time.Second, time.Millisecond)
			require.EqualValues(t, 1, reservation.aborted.Load())
		})
	}
}

func TestConfigIntentStageContainsLateReservationAndItsCleanup(t *testing.T) {
	for name, cause := range map[string]error{"cancel": context.Canceled, "deadline": jobmgr.ErrProcessAttemptDeadline} {
		t.Run(name, func(t *testing.T) {
			controller, _, _, _, _ := newDynCfgJobTestHarness(t)
			attempts := controller.factory.config.Attempts.(*containment.Authority)
			entered, release := make(chan struct{}), make(chan struct{})
			reservation := &stagedIntentReservation{abortEntered: make(chan struct{}), abortRelease: make(chan struct{})}
			var releaseOnce, cleanupOnce sync.Once
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(release) })
				cleanupOnce.Do(func() { close(reservation.abortRelease) })
			})
			stage, err := newPreparedConfigOperation(controller.factory, factoryTestConfig(false), configOperationValidateIntent,
				func(context.Context, confgroup.Config) configOperationResult {
					close(entered)
					<-release
					return configOperationResult{intent: &preparedJobConfigIntent{reservation: reservation}}
				})
			require.NoError(t, err)
			t.Cleanup(stage.Release)
			stage.Start()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("reservation did not start")
			}
			stage.Cancel(cause)
			requireConfigOperationReady(t, stage)
			_, err = stage.take()
			require.ErrorIs(t, err, cause)
			stage.Release()
			require.Equal(t, 1, attempts.Census().Contained)
			_, err = attempts.StartProcessAttempt(t.Context(), jobmgr.ProcessAttemptPlan{Identity: stage.identity, Target: stage.target, Work: func(context.Context, jobmgr.ProcessAttemptAdmission) error { return nil }})
			require.ErrorIs(t, err, jobmgr.ErrProcessAttemptBusy)
			releaseOnce.Do(func() { close(release) })
			select {
			case <-reservation.abortEntered:
			case <-time.After(time.Second):
				t.Fatal("late reservation was not aborted")
			}
			require.Equal(t, 1, attempts.Census().Active, "cleanup must remain physically owned")
			shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			require.Error(t, attempts.Shutdown(shutdown), "shutdown must not report joined while cleanup remains")
			cleanupOnce.Do(func() { close(reservation.abortRelease) })
			require.Eventually(t, func() bool { return attempts.Census() == (containment.Census{}) }, time.Second, time.Millisecond)
			require.EqualValues(t, 1, reservation.aborted.Load())
		})
	}
}

func TestConfigIntentStageRejectsCutAfterPublication(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := controller.factory.config.Attempts.(*containment.Authority)
	reservation := &stagedIntentReservation{}
	stage, err := newPreparedConfigOperation(controller.factory, factoryTestConfig(false), configOperationValidateIntent,
		func(context.Context, confgroup.Config) configOperationResult {
			return configOperationResult{intent: &preparedJobConfigIntent{reservation: reservation}}
		})
	require.NoError(t, err)
	t.Cleanup(stage.Release)
	require.NoError(t, stage.Await(t.Context()))
	stage.Cancel(context.Canceled)
	_, err = stage.take()
	require.ErrorIs(t, err, context.Canceled)
	stage.Release()
	require.Eventually(t, func() bool { return attempts.Census() == (containment.Census{}) }, time.Second, time.Millisecond)
	require.EqualValues(t, 1, reservation.aborted.Load())
}

func TestConfigIntentStageRetainsFailedLateCleanup(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := controller.factory.config.Attempts.(*containment.Authority)
	reservation := &stagedIntentReservation{abortErr: errors.New("release failed")}
	stage, err := newPreparedConfigOperation(controller.factory, factoryTestConfig(false), configOperationValidateIntent,
		func(context.Context, confgroup.Config) configOperationResult {
			return configOperationResult{intent: &preparedJobConfigIntent{reservation: reservation}}
		})
	require.NoError(t, err)
	require.NoError(t, stage.Await(t.Context()))
	stage.Release()
	require.Eventually(t, func() bool { return attempts.Census().Active == 0 }, time.Second, time.Millisecond)
	require.Equal(t, 1, attempts.Census().Quarantined)
	require.EqualValues(t, 1, reservation.aborted.Load())
}

func TestConfigIntentStageAbortsWhenGraphReacquisitionFails(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := controller.factory.config.Attempts.(*containment.Authority)
	reservation := &stagedIntentReservation{}
	reacquireErr := errors.New("graph reacquisition rejected")
	controller.factory.runWithoutClaims = func(ctx context.Context, work func(context.Context) error) (error, error) {
		return work(ctx), reacquireErr
	}
	result, err := controller.runPreparedConfigOperation(t.Context(), factoryTestConfig(false), configOperationValidateIntent, true,
		func(context.Context, confgroup.Config) configOperationResult {
			return configOperationResult{intent: &preparedJobConfigIntent{reservation: reservation}}
		})
	require.ErrorIs(t, err, reacquireErr)
	require.Nil(t, result.intent)
	require.Eventually(t, func() bool { return attempts.Census() == (containment.Census{}) }, time.Second, time.Millisecond)
	require.EqualValues(t, 1, reservation.aborted.Load())
}

func TestConfigIntentStageAbortsCancellationDuringGraphReacquisition(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := controller.factory.config.Attempts.(*containment.Authority)
	reservation := &stagedIntentReservation{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	controller.factory.runWithoutClaims = func(ctx context.Context, work func(context.Context) error) (error, error) {
		err := work(ctx)
		cancel()
		return err, nil
	}
	result, err := controller.runPreparedConfigOperation(ctx, factoryTestConfig(false), configOperationValidateIntent, true,
		func(context.Context, confgroup.Config) configOperationResult {
			return configOperationResult{intent: &preparedJobConfigIntent{reservation: reservation}}
		})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result.intent)
	require.Eventually(t, func() bool { return attempts.Census() == (containment.Census{}) }, time.Second, time.Millisecond)
	require.EqualValues(t, 1, reservation.aborted.Load())
}

func TestConfigIntentStageCallerCancellationLeavesLateWorkContained(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := controller.factory.config.Attempts.(*containment.Authority)
	reservation := &stagedIntentReservation{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	done := make(chan error, 1)
	go func() {
		result, err := controller.runPreparedConfigOperation(ctx, factoryTestConfig(false), configOperationValidateIntent, true,
			func(context.Context, confgroup.Config) configOperationResult {
				close(entered)
				<-release
				return configOperationResult{intent: &preparedJobConfigIntent{reservation: reservation}}
			})
		if result.intent != nil {
			err = errors.New("cancelled caller took a reservation")
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("reservation did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("caller did not cancel")
	}
	require.Equal(t, 1, attempts.Census().Contained)
	releaseOnce.Do(func() { close(release) })
	require.Eventually(t, func() bool { return attempts.Census() == (containment.Census{}) }, time.Second, time.Millisecond)
	require.EqualValues(t, 1, reservation.aborted.Load())
}

func TestConfigIntentStagePreservesLateOwnershipFailureAfterCancellation(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := controller.factory.config.Attempts.(*containment.Authority)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	stage, err := newPreparedConfigOperation(controller.factory, factoryTestConfig(false), configOperationValidateIntent,
		func(context.Context, confgroup.Config) configOperationResult {
			close(entered)
			<-release
			return configOperationResult{err: lifecycle.RetainOwnership(errors.New("owner failed to release rejected reservation"))}
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
