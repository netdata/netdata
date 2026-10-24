// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package secretresolver

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileRequiresUnprivilegedHelper(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "secret")
	require.NoError(t, os.WriteFile(path, []byte("synthetic-secret"), 0o600))
	t.Cleanup(ndexec.SetRunnerPathsForTests(filepath.Join(directory, "missing-nd-run"), ""))
	resolver, err := NewDefaultAtomicResolver()
	require.NoError(t, err)
	value, err := resolver.Resolve(t.Context(), "${file:"+path+"}", nil)
	require.Error(t, err)
	assert.Nil(t, value)
	var providerErr *AtomicResolveError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, AtomicErrorProvider, providerErr.Kind)
}

func TestCommandRequiresUnprivilegedHelper(t *testing.T) {
	t.Setenv("NETDATA_TEST_CMD_AUTH", "synthetic-auth value=with spaces")
	t.Cleanup(ndexec.SetRunnerPathsForTests(filepath.Join(t.TempDir(), "missing-nd-run"), ""))
	resolver, err := NewDefaultAtomicResolver()
	require.NoError(t, err)
	value, err := resolver.Resolve(t.Context(), "${cmd:/usr/bin/printenv NETDATA_TEST_CMD_AUTH}", nil)
	require.Error(t, err)
	assert.Nil(t, value)
	var providerErr *AtomicResolveError
	require.ErrorAs(t, err, &providerErr)
	assert.Equal(t, AtomicErrorProvider, providerErr.Kind)
}

func TestCommandPreservesEnvironmentThroughHelper(t *testing.T) {
	t.Setenv("NETDATA_TEST_CMD_AUTH", "synthetic-auth value=with spaces")
	helper := filepath.Join(t.TempDir(), "nd-run")
	// Require the opt-in syntax before forwarding to the actual command.
	require.NoError(t, os.WriteFile(helper, []byte(`#!/bin/sh
[ "$1" = "--preserve-env" ] && [ "$2" = "--" ] || exit 64
shift 2
exec "$@"
`), 0o700))
	t.Cleanup(ndexec.SetRunnerPathsForTests(helper, ""))
	resolver, err := NewDefaultAtomicResolver()
	require.NoError(t, err)
	value, err := resolver.Resolve(t.Context(), "${cmd:/usr/bin/printenv NETDATA_TEST_CMD_AUTH}", nil)
	require.NoError(t, err)
	assert.Equal(t, "synthetic-auth value=with spaces", value)
}

func TestLocalProvidersOutput(t *testing.T) {
	useTestLocalHelper(t)
	logger.Level.Set(slog.LevelDebug)
	t.Cleanup(func() { logger.Level.Set(slog.LevelInfo) })

	for name, tc := range map[string]struct {
		scheme string
		body   string
		want   string
		fail   bool
	}{
		"file with literal spaces and shell characters": {
			scheme: "file",
			body:   " \t synthetic-stdout \n",
			want:   "synthetic-stdout",
		},
		"command trims stdout and discards stderr": {
			scheme: "cmd",
			body:   "#!/bin/sh\nprintf ' \\t synthetic-stdout \\n'\nprintf 'synthetic-stderr' >&2\n",
			want:   "synthetic-stdout",
		},
		"failed command discards both streams": {
			scheme: "cmd",
			body:   "#!/bin/sh\nprintf 'synthetic-stdout'\nprintf 'synthetic-stderr' >&2\nexit 17\n",
			fail:   true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			filename := "source"
			if tc.scheme == "file" {
				filename = "source with spaces;$(exit 17)"
			}
			path := filepath.Join(t.TempDir(), filename)
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0o700))
			var logs bytes.Buffer
			ctx := logger.ContextWithLogger(t.Context(), logger.NewWithWriter(&logs))
			resolver, err := NewDefaultAtomicResolver()
			require.NoError(t, err)
			value, err := resolver.Resolve(ctx, "${"+tc.scheme+":"+path+"}", nil)
			if tc.fail {
				require.Error(t, err)
				assert.Nil(t, value)
				assert.NotContains(t, err.Error(), "synthetic-stdout")
				assert.NotContains(t, err.Error(), "synthetic-stderr")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.want, value)
				assert.Contains(t, logs.String(), "resolved secret via")
			}
			assert.NotContains(t, logs.String(), "synthetic-stdout")
			assert.NotContains(t, logs.String(), "synthetic-stderr")
		})
	}
}

func TestLocalProvidersBoundedOutput(t *testing.T) {
	useTestLocalHelper(t)
	for name, size := range map[string]int{
		"exact limit": MaximumAtomicResolvedBytes,
		"over limit":  MaximumAtomicResolvedBytes + 1,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source")
			data := strings.Repeat("x", size)
			require.NoError(t, os.WriteFile(path, []byte(data), 0o600))
			for scheme, resolve := range map[string]func() (string, error){
				"file": func() (string, error) { return resolveFile(t.Context(), path, "file reference") },
				"cmd": func() (string, error) {
					return resolveCmd(t.Context(), "/bin/cat "+path, "command reference", time.Second*5)
				},
			} {
				t.Run(scheme, func(t *testing.T) {
					value, err := resolve()
					if size > MaximumAtomicResolvedBytes {
						require.ErrorIs(t, err, errResolvedValueTooLarge)
						assert.Empty(t, value)
					} else {
						require.NoError(t, err)
						assert.True(t, value == data, "all bytes within the limit must be preserved")
					}
				})
			}
		})
	}
}

func TestLocalProvidersDiscardHelperOutputOnFailure(t *testing.T) {
	helper := filepath.Join(t.TempDir(), "nd-run")
	require.NoError(t, os.WriteFile(helper, []byte(
		"#!/bin/sh\nprintf 'synthetic-stdout'\nprintf 'synthetic-stderr' >&2\nexit 73\n"), 0o700))
	t.Cleanup(ndexec.SetRunnerPathsForTests(helper, ""))
	resolver, err := NewDefaultAtomicResolver()
	require.NoError(t, err)
	for scheme, operand := range map[string]string{
		"file": "/unused",
		"cmd":  "/bin/echo direct-fallback",
	} {
		t.Run(scheme, func(t *testing.T) {
			value, err := resolver.Resolve(t.Context(), "${"+scheme+":"+operand+"}", nil)
			require.Error(t, err)
			assert.Nil(t, value)
			assert.NotContains(t, err.Error(), "synthetic-stdout")
			assert.NotContains(t, err.Error(), "synthetic-stderr")
		})
	}
}

func TestCommandTimeout(t *testing.T) {
	useTestLocalHelper(t)
	value, err := resolveCmd(context.Background(), "/bin/sleep 30", "command reference", 50*time.Millisecond)
	require.ErrorContains(t, err, "command timed out after 50ms")
	assert.Empty(t, value)
}

func TestCommandDeadlineBeforeStart(t *testing.T) {
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	value, err := resolveCmd(ctx, "/bin/sleep 30", "command reference", time.Second)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "command failed")
	assert.Empty(t, value)
}

func TestFileOverflowStopsReader(t *testing.T) {
	useTestLocalHelper(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	value, err := resolveFile(ctx, "/dev/zero", "file reference")
	require.ErrorIs(t, err, errResolvedValueTooLarge)
	assert.Empty(t, value)
}

func TestFileDeadlineBeforeStart(t *testing.T) {
	useTestLocalHelper(t)
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	value, err := resolveFile(ctx, "/unused", "file reference")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotContains(t, err.Error(), "file read timed out")
	assert.Empty(t, value)
}

func TestFileTimeoutStopsAndReapsReader(t *testing.T) {
	for _, tc := range []struct {
		name     string
		deadline time.Duration
		maxWait  time.Duration
	}{
		{
			name:    "default timeout",
			maxWait: 5 * time.Second,
		},
		{
			name:     "earlier caller deadline",
			deadline: time.Second,
			maxWait:  3 * time.Second,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "secret.fifo")
			require.NoError(t, syscall.Mkfifo(path, 0o600))
			helper := filepath.Join(dir, "nd-run")
			require.NoError(t, os.WriteFile(helper, []byte(
				"#!/bin/sh\nprintf '%s\\n' \"$$\" > \"$2.pid\"\nexec \"$@\"\n"), 0o700))
			t.Cleanup(ndexec.SetRunnerPathsForTests(helper, ""))
			resolver, err := NewDefaultAtomicResolver()
			require.NoError(t, err)

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			// Bound a failing test without giving the default-timeout case a caller deadline.
			safety := time.AfterFunc(6*time.Second, cancel)
			defer safety.Stop()
			if tc.deadline != 0 {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, tc.deadline)
				defer deadlineCancel()
			}
			start := time.Now()
			value, err := resolver.Resolve(ctx, "${file:"+path+"}", nil)
			assert.Less(t, time.Since(start), tc.maxWait, "blocking file read must be bounded")
			assert.Nil(t, value)
			assert.ErrorIs(t, err, context.DeadlineExceeded)
			require.ErrorContains(t, err, "file read timed out: context deadline exceeded")
			assert.NotContains(t, err.Error(), "signal: killed")
			assert.NotContains(t, err.Error(), "\n")
			var providerErr *AtomicResolveError
			require.ErrorAs(t, err, &providerErr)
			assert.Equal(t, AtomicErrorProvider, providerErr.Kind)

			data, err := os.ReadFile(path + ".pid")
			require.NoError(t, err, "reader must have started")
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			require.NoError(t, err)
			require.Positive(t, pid)
			var status syscall.WaitStatus
			_, err = syscall.Wait4(pid, &status, syscall.WNOHANG, nil)
			assert.ErrorIs(t, err, syscall.ECHILD, "reader must already be reaped")
		})
	}
}
