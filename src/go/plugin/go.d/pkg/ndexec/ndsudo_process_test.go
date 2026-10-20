// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin || freebsd

package ndexec

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNDSudoProcessStartsThroughNDSudo(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	// The test binary stands in for ndsudo; nd-run is missing, so a start through it would fail.
	t.Cleanup(SetRunnerPathsForTests(filepath.Join(t.TempDir(), "missing-nd-run"), os.Args[0]))
	args := ownedHelperArgs("failure", t.TempDir())
	p, err := StartNDSudoProcess(context.Background(), ProcessOptions{}, args[0], args[1:]...)
	require.NoError(t, err)

	var exitErr *exec.ExitError
	require.ErrorAs(t, p.Wait(), &exitErr)
	assert.Equal(t, 23, exitErr.ExitCode())
}

func TestNDSudoProcessTermination(t *testing.T) {
	for name, tc := range map[string]struct {
		denied bool
	}{
		"signals permitted": {denied: false},
		"signals denied":    {denied: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("GORACE", "atexit_sleep_ms=0")
			t.Cleanup(SetRunnerPathsForTests("", os.Args[0]))
			if tc.denied {
				t.Cleanup(DenyNDSudoSignalsForTests())
			}
			r, w, err := os.Pipe()
			require.NoError(t, err)
			args := ownedHelperArgs("writer", t.TempDir())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p, err := StartNDSudoProcess(ctx, ProcessOptions{
				Stdout: w,
			}, args[0], args[1:]...)
			_ = w.Close()
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = r.Close()
				_ = p.Close()
			})
			_, err = bufio.NewReader(r).ReadString('\n')
			require.NoError(t, err, "the command did not start writing")

			cancel()
			done := make(chan error, 1)
			go func() { done <- p.Wait() }()
			if !tc.denied {
				// Termination does not depend on the output: the read end is still open.
				requireSignaled(t, done, syscall.SIGKILL)
				return
			}
			select {
			case err := <-done:
				t.Fatalf("a command whose signals are denied ended on cancellation: %v", err)
			case <-time.After(300 * time.Millisecond):
			}
			require.NoError(t, r.Close())
			requireSignaled(t, done, syscall.SIGPIPE)
		})
	}
}

func requireSignaled(t *testing.T, done <-chan error, sig syscall.Signal) {
	t.Helper()
	select {
	case err := <-done:
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr)
		status, ok := exitErr.Sys().(syscall.WaitStatus)
		require.True(t, ok)
		assert.Equal(t, sig, status.Signal())
	case <-time.After(3 * time.Second):
		t.Fatalf("the command did not end with %v", sig)
	}
}

type terminateStub struct {
	processHandle
	err    error
	called bool
}

func (s *terminateStub) terminate() error {
	s.called = true
	return s.err
}

func TestNDSudoProcessTerminate(t *testing.T) {
	other := errors.New("other failure")
	for name, tc := range map[string]struct {
		denied     bool
		err        error
		wantErr    error
		wantCalled bool
	}{
		"signaled": {
			wantCalled: true,
		},
		"signal denied by the kernel": {
			err:        errors.Join(syscall.EPERM, syscall.EPERM),
			wantCalled: true,
		},
		"other failure": {
			err:        other,
			wantErr:    other,
			wantCalled: true,
		},
		"signals denied for tests": {
			denied: true,
			err:    other,
		},
	} {
		t.Run(name, func(t *testing.T) {
			stub := &terminateStub{
				err: tc.err,
			}
			err := ndsudoProcess{
				processHandle: stub,
				signalsDenied: tc.denied,
			}.terminate()
			assert.Equal(t, tc.wantErr, err)
			assert.Equal(t, tc.wantCalled, stub.called)
		})
	}
}
