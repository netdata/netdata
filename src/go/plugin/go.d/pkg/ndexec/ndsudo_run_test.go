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

			// Once the earlier command exits (the silent helper ends after 2s), the command runs again.
			require.Eventually(t, func() bool {
				_, err := run()
				return !errors.Is(err, ErrPreviousRunNotExited)
			}, 5*time.Second, 50*time.Millisecond)
		})
	}
}
