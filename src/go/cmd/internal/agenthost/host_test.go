// SPDX-License-Identifier: GPL-3.0-or-later

package agenthost

import (
	"context"
	"errors"
	"os"
	"strings"
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
	done := make(chan error, 1)
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
	case err := <-done:
		if err != nil {
			t.Fatalf("host returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("host did not stop")
	}
}

func TestSignalLoopBoundsNonCooperativeTermination(t *testing.T) {
	hosted := newBlockingTerminateAgent()
	signals := make(chan os.Signal, 1)
	done := make(chan error, 1)
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
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(10 * time.Millisecond):
		t.Fatal("host started a second shutdown budget after termination expired")
	}
}

func TestSignalLoopBoundsNonCooperativeRestart(t *testing.T) {
	hosted := newBlockingRestartAgent()
	signals := make(chan os.Signal, 1)
	done := make(chan error, 1)
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
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("host waited indefinitely for non-cooperative restart")
	}
	select {
	case <-hosted.terminateCalled:
		t.Fatal("restart deadline initiated a second termination phase")
	default:
	}
}

func TestSignalLoopExitsCleanlyForProcessRestartRequirement(t *testing.T) {
	hosted := newRestartResultAgent(agent.ErrProcessRestartRequired)
	signals := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() {
		done <- runSignalsWithTimeout(hosted, signals, time.Second, time.Second)
	}()

	signals <- syscall.SIGHUP
	require.NoError(t, <-done)
	require.False(t, hosted.terminated.Load())
	close(hosted.stopRun)
}

func TestSignalLoopFailsAndTerminatesForUnexpectedRestartError(t *testing.T) {
	unexpected := errors.New("unexpected")
	hosted := newRestartResultAgent(errors.Join(context.DeadlineExceeded, unexpected))
	signals := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() {
		done <- runSignalsWithTimeout(hosted, signals, time.Second, time.Second)
	}()

	signals <- syscall.SIGHUP
	require.ErrorIs(t, <-done, unexpected)
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
	tests := map[string]struct {
		done    chan error
		timeout time.Duration
		want    error
		match   string
	}{
		"clean": {
			done:    completedRun(nil),
			timeout: time.Second,
		},
		"dirty": {
			done:    completedRun(sentinel),
			timeout: time.Second,
			want:    sentinel,
		},
		"timeout": {
			done:    make(chan error),
			timeout: time.Millisecond,
			match:   "timed out",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), test.timeout)
			defer cancel()
			err := waitForRun(test.done, ctx)
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("terminal error=%v, want %v", err, test.want)
			}
			if test.match != "" && (err == nil || !strings.Contains(err.Error(), test.match)) {
				t.Fatalf("terminal error=%v, want match %q", err, test.match)
			}
			if test.want == nil && test.match == "" && err != nil {
				t.Fatalf("clean terminal returned %v", err)
			}
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
	return nil
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
