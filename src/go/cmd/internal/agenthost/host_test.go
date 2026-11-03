// SPDX-License-Identifier: GPL-3.0-or-later

package agenthost

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignalLoopTerminatesWhileRestartIsInProgress(t *testing.T) {
	hosted := newBlockingRestartAgent()
	signals := make(chan os.Signal, 2)
	done := make(chan Result, 1)
	go func() {
		done <- runSignals(hosted, signals)
	}()

	signals <- syscall.SIGHUP
	select {
	case <-hosted.restartEntered:
	case <-time.After(time.Second):
		t.Fatal("restart did not start")
	}

	signals <- syscall.SIGTERM
	select {
	case <-hosted.terminateCalled:
	case <-time.After(time.Second):
		t.Fatal("signal loop waited for restart before delivering termination")
	}

	close(hosted.releaseRestart)
	select {
	case result := <-done:
		require.Equal(t, Result{}, result)
	case <-time.After(time.Second):
		t.Fatal("host did not stop")
	}
}

func TestSignalLoopBoundsNonCooperativeTermination(t *testing.T) {
	hosted := newBlockingTerminateAgent()
	signals := make(chan os.Signal, 1)
	done := make(chan Result, 1)
	go func() {
		done <- runSignalsWithTimeout(hosted, signals, time.Second, 20*time.Millisecond)
	}()
	t.Cleanup(func() {
		hosted.release()
	})

	signals <- syscall.SIGTERM
	select {
	case <-hosted.terminateDeadline:
	case <-time.After(time.Second):
		t.Fatal("termination did not observe its deadline")
	}

	select {
	case result := <-done:
		require.ErrorIs(t, result.Err, context.DeadlineExceeded)
		require.True(t, result.ExitRequired)
	case <-time.After(10 * time.Millisecond):
		t.Fatal("host started a second shutdown budget after termination expired")
	}
}

func TestSignalLoopBoundsNonCooperativeRestart(t *testing.T) {
	hosted := newBlockingRestartAgent()
	signals := make(chan os.Signal, 1)
	done := make(chan Result, 1)
	go func() {
		done <- runSignalsWithTimeout(hosted, signals, 20*time.Millisecond, time.Second)
	}()
	t.Cleanup(func() {
		close(hosted.releaseRestart)
		close(hosted.stopRun)
	})

	signals <- syscall.SIGHUP
	select {
	case <-hosted.restartEntered:
	case <-time.After(time.Second):
		t.Fatal("restart did not start")
	}

	select {
	case result := <-done:
		require.Equal(t, Result{
			ExitRequired: true,
		}, result)
	case <-time.After(time.Second):
		t.Fatal("host waited indefinitely for non-cooperative restart")
	}
	select {
	case <-hosted.terminateCalled:
		t.Fatal("restart deadline initiated a second termination phase")
	default:
	}
}

func TestSignalLoopDoesNotPermitCleanupBeforeRunCompletes(t *testing.T) {
	hosted := newRestartResultAgent(agent.ErrProcessRestartRequired)
	signals := make(chan os.Signal, 1)
	done := make(chan Result, 1)
	go func() {
		done <- runSignalsWithTimeout(hosted, signals, time.Second, time.Second)
	}()

	signals <- syscall.SIGHUP
	require.Equal(t, Result{
		ExitRequired: true,
	}, <-done)
	require.False(t, hosted.terminated.Load())
	close(hosted.stopRun)
}

func TestSignalLoopFailsAndTerminatesForUnexpectedRestartError(t *testing.T) {
	unexpected := errors.New("unexpected")
	hosted := newRestartResultAgent(errors.Join(context.DeadlineExceeded, unexpected))
	signals := make(chan os.Signal, 1)
	done := make(chan Result, 1)
	go func() {
		done <- runSignalsWithTimeout(hosted, signals, time.Second, time.Second)
	}()

	signals <- syscall.SIGHUP
	result := <-done
	require.ErrorIs(t, result.Err, unexpected)
	require.False(t, result.ExitRequired)
	require.True(t, hosted.terminated.Load())
}

func TestRestartControlErrorTreatsStoppedProcessAsBenign(t *testing.T) {
	sentinel := errors.New("restart failed")
	tests := map[string]struct {
		err  error
		want error
	}{
		"success": {},
		"process already stopped": {
			err: agent.ErrNotRunning,
		},
		"wrapped process already stopped": {
			err:  errors.Join(sentinel, agent.ErrNotRunning),
			want: sentinel,
		},
		"restart failure": {
			err:  sentinel,
			want: sentinel,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, restartControlError(test.err), test.want)
		})
	}
}

func TestRestartRecoveryRequiresOnlyApprovedDispositions(t *testing.T) {
	unexpected := errors.New("unexpected")
	tests := map[string]struct {
		err  error
		want bool
	}{
		"deadline": {
			err:  context.DeadlineExceeded,
			want: true,
		},
		"process restart": {
			err:  agent.ErrProcessRestartRequired,
			want: true,
		},
		"deadline and process restart": {
			err: errors.Join(
				context.DeadlineExceeded,
				agent.ErrProcessRestartRequired,
			),
			want: true,
		},
		"unexpected failure": {
			err: unexpected,
		},
		"deadline plus unexpected failure": {
			err: errors.Join(context.DeadlineExceeded, unexpected),
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, restartRecoveryRequired(test.err))
		})
	}
}

func TestWaitForRunReturnsExactTerminalDisposition(t *testing.T) {
	sentinel := errors.New("dirty run")
	expired, cancel := context.WithCancel(t.Context())
	cancel()
	for name, tc := range map[string]struct {
		done         chan error
		ctx          context.Context
		want         error
		match        string
		exitRequired bool
	}{
		"clean":                            {done: completedRun(nil), ctx: t.Context()},
		"dirty":                            {done: completedRun(sentinel), ctx: t.Context(), want: sentinel},
		"completed despite expired wait":   {done: completedRun(nil), ctx: expired},
		"failure despite expired wait":     {done: completedRun(sentinel), ctx: expired, want: sentinel},
		"expired wait with unfinished run": {done: make(chan error), ctx: expired, want: context.Canceled, match: "timed out", exitRequired: true},
	} {
		t.Run(name, func(t *testing.T) {
			result := waitForRun(tc.done, tc.ctx)
			require.ErrorIs(t, result.Err, tc.want)
			if tc.match != "" {
				require.ErrorContains(t, result.Err, tc.match)
			}
			require.Equal(t, tc.exitRequired, result.ExitRequired)
		})
	}
}

func TestRecoveryResultRequiresExitUnlessRunCompleted(t *testing.T) {
	failed := errors.New("shutdown failed")
	for name, tc := range map[string]struct {
		done chan error
		want Result
	}{
		"unfinished run": {done: make(chan error), want: Result{
			ExitRequired: true,
		}},
		"completed clean run": {done: completedRun(nil), want: Result{}},
		"completed failed run": {done: completedRun(failed), want: Result{
			Err: failed,
		}},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, recoveryResult(tc.done))
		})
	}
}

func TestSignalLoopReportsCompletedRun(t *testing.T) {
	failed := errors.New("run failed")
	for name, runErr := range map[string]error{"clean": nil, "failed": failed} {
		t.Run(name, func(t *testing.T) {
			hosted := newRestartResultAgent(nil)
			hosted.runResult = runErr
			close(hosted.stopRun)
			signals := make(chan os.Signal)
			result := runSignalsWithTimeout(hosted, signals, time.Second, time.Second)
			require.Equal(t, Result{
				Err: runErr,
			}, result)
			require.False(t, hosted.terminated.Load())
		})
	}
}

func completedRun(err error) chan error {
	done := make(chan error, 1)
	done <- err
	return done
}

type blockingRestartAgent struct {
	restartEntered  chan struct{}
	releaseRestart  chan struct{}
	terminateCalled chan struct{}
	stopRun         chan struct{}
	terminateOnce   sync.Once
}

func newBlockingRestartAgent() *blockingRestartAgent {
	return &blockingRestartAgent{
		restartEntered:  make(chan struct{}),
		releaseRestart:  make(chan struct{}),
		terminateCalled: make(chan struct{}),
		stopRun:         make(chan struct{}),
	}
}

func (agent *blockingRestartAgent) RunContext(context.Context) error {
	<-agent.stopRun
	return nil
}

func (agent *blockingRestartAgent) Restart(context.Context) error {
	close(agent.restartEntered)
	<-agent.releaseRestart
	return nil
}

func (agent *blockingRestartAgent) Terminate(context.Context) error {
	agent.terminateOnce.Do(func() {
		close(agent.terminateCalled)
		close(agent.stopRun)
	})
	return nil
}

func (*blockingRestartAgent) Info(...any)           {}
func (*blockingRestartAgent) Infof(string, ...any)  {}
func (*blockingRestartAgent) Errorf(string, ...any) {}

type blockingTerminateAgent struct {
	terminateEntered  chan struct{}
	terminateDeadline chan struct{}
	terminateRelease  chan struct{}
	runDone           chan struct{}
	enterOnce         sync.Once
	releaseOnce       sync.Once
}

func newBlockingTerminateAgent() *blockingTerminateAgent {
	return &blockingTerminateAgent{
		terminateEntered:  make(chan struct{}),
		terminateDeadline: make(chan struct{}),
		terminateRelease:  make(chan struct{}),
		runDone:           make(chan struct{}),
	}
}

func (agent *blockingTerminateAgent) RunContext(context.Context) error {
	<-agent.runDone
	return nil
}

func (*blockingTerminateAgent) Restart(context.Context) error {
	return nil
}

func (agent *blockingTerminateAgent) Terminate(ctx context.Context) error {
	agent.enterOnce.Do(func() {
		close(agent.terminateEntered)
	})
	<-ctx.Done()
	close(agent.terminateDeadline)
	<-agent.terminateRelease
	return ctx.Err()
}

func (agent *blockingTerminateAgent) release() {
	agent.releaseOnce.Do(func() {
		close(agent.terminateRelease)
		close(agent.runDone)
	})
}

func (*blockingTerminateAgent) Info(...any)           {}
func (*blockingTerminateAgent) Infof(string, ...any)  {}
func (*blockingTerminateAgent) Errorf(string, ...any) {}

type restartResultAgent struct {
	runResult  error
	result     error
	stopRun    chan struct{}
	terminated atomic.Bool
}

func newRestartResultAgent(result error) *restartResultAgent {
	return &restartResultAgent{
		result:  result,
		stopRun: make(chan struct{}),
	}
}

func (a *restartResultAgent) RunContext(context.Context) error {
	<-a.stopRun
	return a.runResult
}

func (a *restartResultAgent) Restart(context.Context) error {
	return a.result
}

func (a *restartResultAgent) Terminate(context.Context) error {
	if a.terminated.CompareAndSwap(false, true) {
		close(a.stopRun)
	}
	return nil
}

func (*restartResultAgent) Info(...any)           {}
func (*restartResultAgent) Infof(string, ...any)  {}
func (*restartResultAgent) Errorf(string, ...any) {}
