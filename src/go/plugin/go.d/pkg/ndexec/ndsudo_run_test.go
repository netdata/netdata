// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin || freebsd

package ndexec

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useTestBinaryAsNDSudo makes the test binary stand in for ndsudo; nd-run is missing, so a run through it would fail.
func useTestBinaryAsNDSudo(t *testing.T) {
	t.Helper()
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	t.Cleanup(SetRunnerPathsForTests(filepath.Join(t.TempDir(), "missing-nd-run"), os.Args[0]))
}

func TestRunNDSudoResult(t *testing.T) {
	for name, tc := range map[string]struct {
		mode       string
		wantOut    string
		wantExit   int
		wantStderr string
	}{
		"success":                {mode: "success"},
		"output and exit status": {mode: "output", wantOut: "stdout\n", wantExit: 1, wantStderr: "(stderr: stderr)"},
	} {
		t.Run(name, func(t *testing.T) {
			useTestBinaryAsNDSudo(t)
			args := ownedHelperArgs(tc.mode, t.TempDir())

			out, cmd, err := RunNDSudoWithCmd(nil, 5*time.Second, args[0], args[1:]...)

			assert.Equal(t, strings.Join(append([]string{os.Args[0]}, args...), " "), cmd)
			assert.Equal(t, tc.wantOut, string(out))
			if tc.wantExit == 0 {
				require.NoError(t, err)
				return
			}
			var exitErr *exec.ExitError
			require.ErrorAs(t, err, &exitErr)
			assert.Equal(t, tc.wantExit, exitErr.ExitCode())
			assert.ErrorContains(t, err, tc.wantStderr)
		})
	}
}

func TestRunNDSudoStartFailure(t *testing.T) {
	t.Cleanup(SetRunnerPathsForTests("", filepath.Join(t.TempDir(), "missing-ndsudo")))

	out, err := RunNDSudo(nil, time.Second, "command")

	assert.Nil(t, out)
	require.ErrorContains(t, err, "RunNDSudo: ")
	assert.ErrorContains(t, err, "missing-ndsudo command")
}

func TestRunNDSudoTimeout(t *testing.T) {
	const timeout = 300 * time.Millisecond
	for name, tc := range map[string]struct {
		mode     string
		denied   bool
		unreaped bool // the command outlives the run
	}{
		"command terminated at the timeout": {mode: "silent"},
		"writer that cannot be signaled":    {mode: "writer", denied: true},
		"silent command that cannot be signaled": {
			mode:     "silent",
			denied:   true,
			unreaped: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			useTestBinaryAsNDSudo(t)
			if tc.denied {
				t.Cleanup(DenyNDSudoSignalsForTests())
			}
			args := ownedHelperArgs(tc.mode, t.TempDir())
			run := func() (time.Duration, error) {
				began := time.Now()
				_, err := RunNDSudo(nil, timeout, args[0], args[1:]...)
				return time.Since(began), err
			}

			elapsed, err := run()
			require.ErrorIs(t, err, context.DeadlineExceeded)
			assert.GreaterOrEqual(t, elapsed, timeout)
			assert.Less(t, elapsed, timeout+ndsudoExitWait+time.Second, "the run outlasted its bound")

			elapsed, err = run()
			if !tc.unreaped {
				require.ErrorIs(t, err, context.DeadlineExceeded, "the same command was refused after it exited")
				return
			}
			require.ErrorIs(t, err, ErrPreviousRunNotExited)
			assert.Less(t, elapsed, timeout, "a refused run waited")
			other := ownedHelperArgs("success", t.TempDir())
			_, err = RunNDSudo(nil, 5*time.Second, other[0], other[1:]...)
			require.NoError(t, err, "another command line was refused")

			// Once the earlier command exits (the silent helper ends after 2s), the command runs again.
			require.Eventually(t, func() bool {
				_, err := run()
				return !errors.Is(err, ErrPreviousRunNotExited)
			}, 5*time.Second, 50*time.Millisecond)
		})
	}
}

func TestRunNDSudoGuardCountsConcurrentRuns(t *testing.T) {
	useTestBinaryAsNDSudo(t)
	t.Cleanup(DenyNDSudoSignalsForTests())
	args := ownedHelperArgs("slot", t.TempDir())
	began := time.Now()

	// Two runs of the same command line time out together; their commands exit after 0.7s and 3s.
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Go(func() { _, errs[i] = RunNDSudo(nil, 300*time.Millisecond, args[0], args[1:]...) })
	}
	wg.Wait()
	for _, err := range errs {
		require.ErrorIs(t, err, context.DeadlineExceeded)
	}

	// The command line stays refused while either command runs.
	time.Sleep(time.Until(began.Add(1500 * time.Millisecond)))
	_, err := RunNDSudo(nil, 300*time.Millisecond, args[0], args[1:]...)
	require.ErrorIs(t, err, ErrPreviousRunNotExited)
}

func TestRunNDSudoDescendantHoldingOutput(t *testing.T) {
	for name, tc := range map[string]struct {
		denied     bool
		wantErr    error
		wantEscape bool
	}{
		// The descendant ends with the command; the run succeeds.
		"signals permitted": {},
		// The descendant survives and keeps the output open; the run reports it after the output wait.
		"signals denied": {denied: true, wantErr: exec.ErrWaitDelay, wantEscape: true},
	} {
		t.Run(name, func(t *testing.T) {
			useTestBinaryAsNDSudo(t)
			if tc.denied {
				t.Cleanup(DenyNDSudoSignalsForTests())
			}
			dir := t.TempDir()
			args := ownedHelperArgs("parent-exit", dir)

			_, err := RunNDSudo(nil, 5*time.Second, args[0], args[1:]...)

			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
			}
			// The descendant writes "escaped" 1.2s after it starts, if it is still alive.
			escaped := filepath.Join(dir, "escaped")
			if tc.wantEscape {
				require.Eventually(t, func() bool { _, err := os.Stat(escaped); return err == nil }, 3*time.Second,
					20*time.Millisecond)
				return
			}
			time.Sleep(1500 * time.Millisecond)
			_, err = os.Stat(escaped)
			assert.True(t, os.IsNotExist(err), "the descendant outlived the command")
		})
	}
}
