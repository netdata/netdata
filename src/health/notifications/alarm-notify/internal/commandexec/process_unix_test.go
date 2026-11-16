// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package commandexec

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunnerHelper(t *testing.T) {
	mode := os.Getenv("COMMANDEXEC_TEST_HELPER")
	if mode == "" {
		return
	}
	if mode == "session" {
		_, err := io.Copy(os.Stdout, os.Stdin)
		if err != nil {
			os.Exit(80)
		}
		os.Exit(0)
	}
	if mode == "private" {
		home := os.Getenv("HOME")
		info, err := os.Stat(home)
		if err != nil || info.Mode().Perm() != 0700 {
			os.Exit(81)
		}
		if err := os.WriteFile(filepath.Join(home, "cache"), []byte("synthetic-secret"), 0600); err != nil {
			os.Exit(82)
		}
		if err := os.WriteFile(os.Getenv("COMMANDEXEC_TEST_CAPTURE"), []byte(home), 0600); err != nil {
			os.Exit(83)
		}
	}
	if os.Getenv("COMMANDEXEC_TEST_FAIL") == "1" {
		os.Exit(7)
	}
	os.Exit(0)
}

func TestRunWithPrivateHome(t *testing.T) {
	for name, test := range map[string]struct {
		fail bool
		want string
	}{"success": {}, "exit error": {fail: true, want: "command exited with status 7"}} {
		t.Run(name, func(t *testing.T) {
			executable, err := os.Executable()
			require.NoError(t, err)
			capture := filepath.Join(t.TempDir(), "home")
			env := []string{"COMMANDEXEC_TEST_HELPER=private", "COMMANDEXEC_TEST_CAPTURE=" + capture, "GORACE=atexit_sleep_ms=0"}
			if test.fail {
				env = append(env, "COMMANDEXEC_TEST_FAIL=1")
			}
			var runner Runner
			err = runner.RunWithPrivateHome(t.Context(), executable, []string{"-test.run=^TestRunnerHelper$"}, env, nil)
			if test.want != "" {
				require.EqualError(t, err, test.want)
			} else {
				require.NoError(t, err)
			}
			runner.CloseAndWait()
			home, err := os.ReadFile(capture)
			require.NoError(t, err)
			_, err = os.Stat(string(home))
			assert.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestRunnerClosedAdmission(t *testing.T) {
	for name, run := range map[string]func(*Runner) error{
		"command":      func(r *Runner) error { return r.Run(t.Context(), "must-not-start", nil, nil, nil) },
		"private home": func(r *Runner) error { return r.RunWithPrivateHome(t.Context(), "must-not-start", nil, nil, nil) },
		"session": func(r *Runner) error {
			return r.RunSession(t.Context(), "must-not-start", nil, nil, func(io.Reader, io.WriteCloser) error {
				t.Error("closed runner started a session")
				return nil
			})
		},
	} {
		t.Run(name, func(t *testing.T) {
			var runner Runner
			runner.CloseAndWait()
			require.ErrorIs(t, run(&runner), context.Canceled)
		})
	}
}

func TestRunnerCloseWaitsForSession(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	var runner Runner
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runner.RunSession(ctx, executable, []string{"-test.run=^TestRunnerHelper$"},
			[]string{"COMMANDEXEC_TEST_HELPER=session", "GORACE=atexit_sleep_ms=0"}, func(input io.Reader, output io.WriteCloser) error {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
				if _, err := io.WriteString(output, "session exchange"); err != nil {
					return err
				}
				if err := output.Close(); err != nil {
					return err
				}
				data, err := io.ReadAll(input)
				assert.Equal(t, "session exchange", string(data))
				return err
			})
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("session did not start")
	}
	closed := make(chan struct{})
	go func() { runner.CloseAndWait(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("CloseAndWait returned before the admitted session completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-done)
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("CloseAndWait did not finish")
	}
	require.ErrorIs(t, runner.Run(ctx, executable, nil, nil, nil), context.Canceled)
}
