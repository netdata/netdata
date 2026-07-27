// SPDX-License-Identifier: GPL-3.0-or-later

package agenthost

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent"
	"github.com/stretchr/testify/assert"
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
			err: errors.Join(sentinel, agent.ErrNotRunning),
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
			err := waitForRun(test.done, test.timeout)
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
