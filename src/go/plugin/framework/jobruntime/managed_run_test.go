// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/require"
)

func TestManagedRunSettlement(t *testing.T) {
	tests := []struct {
		name    string
		execute func(*ManagedRun)
		ready   bool
		reason  string
		retry   bool
	}{
		{"early error", func(r *ManagedRun) { r.Complete(errors.New("bind")); r.Ready() }, false, "error", true},
		{"early nil", func(r *ManagedRun) { r.Complete(nil); r.Ready() }, false, "unexpected_return", false},
		{"ready then error", func(r *ManagedRun) { r.Ready(); r.Complete(errors.New("read")) }, true, "error", false},
		{"ready then nil", func(r *ManagedRun) { r.Ready(); r.Complete(nil) }, true, "unexpected_return", false},
		{"timeout", func(r *ManagedRun) { r.Timeout(); r.Ready(); r.Complete(context.Canceled) }, false, "startup_timeout", true},
		{"failure survives stop", func(r *ManagedRun) { r.Complete(errors.New("bind")); r.Stop(nil) }, false, "error", true},
		{"stop retains unrelated failure", func(r *ManagedRun) {
			r.Ready()
			r.Stop(nil)
			r.Complete(errors.Join(context.Canceled, errors.New("read")))
		}, true, "error", false},
		{"stop retains panic", func(r *ManagedRun) { r.Ready(); r.Stop(nil); r.Complete(newRunFailure(context.Canceled, "panic", nil)) }, true, "panic", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var revoked atomic.Int32
			r := NewManagedRun(t.Context(), func() { revoked.Add(1) })
			test.execute(r)
			select {
			case <-r.StartupDone():
			default:
				t.Fatal("startup not settled")
			}
			require.Equal(t, test.ready, r.StartupErr() == nil)
			require.NotNil(t, r.Failure())
			require.Equal(t, test.ready, r.Failure().AfterReady())
			require.Equal(t, test.reason, r.Failure().Reason())
			require.Equal(t, test.retry, r.Failure().DyncfgRetryable())
			require.False(t, r.Running())
			r.Ready()
			r.Timeout()
			r.Complete(errors.New("duplicate"))
			require.EqualValues(t, 1, revoked.Load())
		})
	}
}

func TestManagedRunReadinessAndTimeoutHaveOneWinner(t *testing.T) {
	for range 100 {
		r := NewManagedRun(t.Context(), nil)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); r.Ready() }()
		go func() { defer wg.Done(); r.Timeout() }()
		wg.Wait()
		<-r.StartupDone()
		if r.StartupErr() == nil {
			require.True(t, r.Running())
			require.NoError(t, r.Context().Err())
			r.Timeout()
			require.True(t, r.Running(), "settled startup timer cannot cancel serving")
		} else {
			r.Ready()
			require.False(t, r.Running())
			require.Error(t, r.Context().Err())
		}
		r.Stop(nil)
	}
}

func TestManagedRunRequestedCancellation(t *testing.T) {
	for _, afterReady := range []bool{false, true} {
		for _, err := range []error{nil, context.Canceled, fmt.Errorf("stop: %w", context.Canceled), errors.Join(context.Canceled, context.Canceled)} {
			r := NewManagedRun(t.Context(), nil)
			if afterReady {
				r.Ready()
			}
			r.Stop(nil)
			r.Complete(err)
			r.Ready()
			require.Nil(t, r.Failure())
			require.False(t, r.Running())
			if afterReady {
				require.NoError(t, r.StartupErr())
			} else {
				require.ErrorIs(t, r.StartupErr(), context.Canceled)
			}
		}
	}
}

type readinessTestModule struct {
	*mockModuleV2
	run func(context.Context, func()) error
}

func (m *readinessTestModule) Run(ctx context.Context, ready func()) error { return m.run(ctx, ready) }

func TestJobV2WaitsForReadinessAndObservesExitDuringCollect(t *testing.T) {
	entered, releaseReady := make(chan struct{}), make(chan struct{})
	collectEntered, releaseCollect := make(chan struct{}), make(chan struct{})
	exitRunner, revoked, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	module := &readinessTestModule{
		mockModuleV2: &mockModuleV2{
			store: metrix.NewCollectorStore(), template: chartTemplateV2(),
			collectFunc: func(context.Context) error { calls.Add(1); close(collectEntered); <-releaseCollect; return nil },
		},
		run: func(ctx context.Context, ready func()) error {
			close(entered)
			<-releaseReady
			ready()
			<-exitRunner
			return errors.New("required listener failed")
		},
	}
	job := newTestJobV2(module, &bytes.Buffer{})
	require.NoError(t, job.AutoDetectionManaged(t.Context()))
	run := NewManagedRun(t.Context(), func() { close(revoked) })
	go func() { defer close(done); job.StartManaged(run) }()
	<-entered
	require.False(t, job.IsRunning())
	job.Tick(1)
	require.Zero(t, calls.Load())
	close(releaseReady)
	<-run.StartupDone()
	require.True(t, job.IsRunning())
	require.Eventually(t, func() bool { job.Tick(2); return calls.Load() > 0 }, time.Second, time.Millisecond)
	<-collectEntered
	close(exitRunner)
	select {
	case <-revoked:
	case <-time.After(time.Second):
		t.Fatal("blocked Collect delayed output revocation")
	}
	require.False(t, job.IsRunning())
	require.True(t, run.Failure().AfterReady())
	close(releaseCollect)
	<-done
	require.EqualValues(t, 1, calls.Load())
	job.Cleanup()
}

func TestJobV2RunnerClassificationSurvivesSanitization(t *testing.T) {
	for _, afterReady := range []bool{false, true} {
		module := &readinessTestModule{
			mockModuleV2: &mockModuleV2{},
			run: func(ctx context.Context, ready func()) error {
				if afterReady {
					ready()
				}
				panic(context.Canceled)
			},
		}
		job := newTestJobV2(module, &bytes.Buffer{})
		job.lifecycleErrorSanitizer = func(error) error { return errors.New("safe diagnostic") }
		run := NewManagedRun(t.Context(), nil)
		job.runCollectorRunner(run.Context(), module, run)
		require.Equal(t, "panic", run.Failure().Reason())
		require.Equal(t, afterReady, run.Failure().AfterReady())
		require.False(t, run.Failure().DyncfgRetryable())
		require.Equal(t, "safe diagnostic", run.Failure().Error())
	}
}

func TestManagedRunCancellationCauseAndMixedErrors(t *testing.T) {
	cause := errors.New("owner retired")
	for _, test := range []struct {
		name    string
		result  error
		failure bool
	}{
		{"context error", context.Canceled, false},
		{"cancellation cause", cause, false},
		{"both cancellation forms", errors.Join(context.Canceled, cause), false},
		{"unrelated joined error", errors.Join(cause, errors.New("listener close failed")), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := NewManagedRun(t.Context(), nil)
			r.Ready()
			r.Stop(cause)
			r.Complete(test.result)
			require.Equal(t, test.failure, r.Failure() != nil)
		})
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	r := NewManagedRun(ctx, nil)
	cancel(cause)
	r.Complete(nil)
	<-r.StartupDone()
	require.ErrorIs(t, r.StartupErr(), cause)
	require.Nil(t, r.Failure())
}
