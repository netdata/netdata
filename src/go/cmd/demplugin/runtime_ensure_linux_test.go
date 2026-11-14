// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestRuntimeEnsureWarmSelectionAvoidsQualificationAndInventory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("ensure requires the nonroot service identity")
	}
	root, cache, _, _, selected := selectedAdminFixture(t)
	// The selected fixture cannot qualify, and no release URL is configured.
	// Any acquisition, qualification or deferred inventory would fail this call.
	unrelated := filepath.Join(cache, "generations", strings.Repeat("f", 64))
	require.NoError(t, os.Mkdir(unrelated, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(unrelated, "prepared.json"), []byte("broken"), 0600))
	selectionPath := filepath.Join(cache, "selection.json")
	before, err := os.ReadFile(selectionPath)
	require.NoError(t, err)
	report, err := runtimeCommand(t, "runtime", "ensure", "--allow-download", "--config-dir", root, "--cache", cache, "--expected-root", cache)
	require.NoError(t, err)
	require.Equal(t, "already-selected", report.Outcome)
	require.Equal(t, "installed", report.State)
	require.Equal(t, selected.Archive.SHA256, report.Generation)
	require.Nil(t, report.Storage)
	require.Nil(t, report.Qualification)
	require.Nil(t, report.Candidate)
	require.Contains(t, report.Notice, "restart")
	after, err := os.ReadFile(selectionPath)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestRuntimeEnsureExpectedRootRefusesBeforeCreatingStore(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("ensure requires the nonroot service identity")
	}
	root := t.TempDir()
	cache := filepath.Join(root, "absent", "cache")
	report, err := runtimeCommand(t, "runtime", "ensure", "--allow-download", "--config-dir", root, "--cache", cache, "--expected-root", "/var/lib/netdata/dem/runtime")
	require.ErrorContains(t, err, "differs from --expected-root")
	require.Equal(t, "failed", report.Outcome)
	require.NoDirExists(t, filepath.Dir(cache))
}

func TestRuntimeEnsureCanceledBeforeStorageMutation(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("ensure requires the nonroot service identity")
	}
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runRuntimeCommand(ctx, []string{"ensure", "--allow-download", "--config-dir", root, "--cache", cache}, nil, os.Stdout, os.Stderr)
	require.ErrorIs(t, err, context.Canceled)
	require.NoDirExists(t, cache)
}

func TestRuntimeEnsureRejectsRoot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to test identity refusal")
	}
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	_, err := runtimeCommand(t, "runtime", "ensure", "--allow-download", "--config-dir", root, "--cache", cache)
	require.ErrorContains(t, err, "nonroot DEM service account")
	require.NoDirExists(t, cache)
}

// Run the actual CLI boundary in a separate process with blocking inherited
// stdin, exactly as docker exec -i supplies it. The writer remains open until
// the command returns; finishing must join the reader without waiting for EOF.
func TestRuntimeEnsurePrivateStdinSubprocess(t *testing.T) {
	if os.Getenv("NETDATA_TEST_ENSURE_STDIN") == "child" {
		os.Args = []string{"dem.plugin", "runtime", "ensure", "--allow-download", "--config-dir", os.Getenv("NETDATA_TEST_ENSURE_CONFIG"), "--cache", os.Getenv("NETDATA_TEST_ENSURE_CACHE"), "--cancel-on-stdin-close"}
		main()
		os.Exit(0)
	}
	if os.Geteuid() == 0 {
		t.Skip("ensure requires the nonroot service identity")
	}
	root, cache, _, _, _ := selectedAdminFixture(t)
	for _, kind := range []string{"pipe", "socket"} {
		t.Run(kind, func(t *testing.T) {
			input, _ := runtimeControlTestFiles(t, kind)
			inputFD := input.Fd()
			flags, err := unix.FcntlInt(inputFD, unix.F_GETFL, 0)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRuntimeEnsurePrivateStdinSubprocess$")
			cmd.Env = append(os.Environ(), "NETDATA_TEST_ENSURE_STDIN=child", "NETDATA_TEST_ENSURE_CONFIG="+root, "NETDATA_TEST_ENSURE_CACHE="+cache)
			cmd.Stdin = input
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			require.Contains(t, string(output), `"outcome": "already-selected"`)
			after, err := unix.FcntlInt(inputFD, unix.F_GETFL, 0)
			require.NoError(t, err)
			require.Equal(t, flags, after)
		})
	}
}

func TestRuntimeEnsureControlEOFCancelsInstallerWait(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("ensure requires the nonroot service identity")
	}
	root, cache, _, _, _ := selectedAdminFixture(t)
	lock := flock.New(filepath.Join(cache, "installation.lock"))
	require.NoError(t, lock.Lock())
	defer lock.Close()
	for _, kind := range []string{"pipe", "socket"} {
		for _, action := range []string{"EOF", "cancel"} {
			t.Run(kind+"/"+action, func(t *testing.T) {
				input, writer := runtimeControlTestFiles(t, kind)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan error, 1)
				go func() {
					err := runRuntimeCommand(ctx, []string{"ensure", "--allow-download", "--config-dir", root, "--cache", cache, "--cancel-on-stdin-close"}, input, io.Discard, io.Discard)
					done <- err
				}()
				if action == "cancel" {
					cancel()
				} else if kind == "socket" {
					require.NoError(t, unix.Shutdown(int(writer.Fd()), unix.SHUT_WR))
				} else {
					require.NoError(t, writer.Close())
				}
				select {
				case err := <-done:
					require.ErrorIs(t, err, context.Canceled)
				case <-time.After(5 * time.Second):
					t.Fatal("cancellation did not finish installer lock wait and join watcher")
				}
			})
		}
	}
}

func TestRuntimeControlReaderPreservesInputFlags(t *testing.T) {
	for _, kind := range []string{"pipe", "socket"} {
		for _, mode := range []string{"blocking", "nonblocking"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				input, _ := runtimeControlTestFiles(t, kind)
				inputFD := input.Fd()
				require.NoError(t, unix.SetNonblock(int(inputFD), mode == "nonblocking"))
				flags, err := unix.FcntlInt(inputFD, unix.F_GETFL, 0)
				require.NoError(t, err)
				_, finish, err := runtimeInputContext(context.Background(), input)
				require.NoError(t, err)
				after, err := unix.FcntlInt(inputFD, unix.F_GETFL, 0)
				require.NoError(t, err)
				require.Equal(t, flags, after, "control reader changed the inherited open file description")
				finish()
				after, err = unix.FcntlInt(inputFD, unix.F_GETFL, 0)
				require.NoError(t, err)
				require.Equal(t, flags, after)
			})
		}
	}
}

func TestRuntimeControlReaderRejectsNonChannels(t *testing.T) {
	for _, path := range []string{"/dev/null", "/dev/ptmx"} {
		t.Run(path, func(t *testing.T) {
			input, err := os.Open(path)
			require.NoError(t, err)
			defer input.Close()
			inputFD := input.Fd()
			flags, err := unix.FcntlInt(inputFD, unix.F_GETFL, 0)
			require.NoError(t, err)
			_, finish, err := runtimeInputContext(context.Background(), input)
			if finish != nil {
				finish()
			}
			require.ErrorContains(t, err, "pipe or stream socket")
			after, err := unix.FcntlInt(inputFD, unix.F_GETFL, 0)
			require.NoError(t, err)
			require.Equal(t, flags, after)
		})
	}
	input, err := os.CreateTemp(t.TempDir(), "regular")
	require.NoError(t, err)
	defer input.Close()
	_, finish, err := runtimeInputContext(context.Background(), input)
	if finish != nil {
		finish()
	}
	require.ErrorContains(t, err, "pipe or stream socket")
}

func runtimeControlTestFiles(t *testing.T, kind string) (*os.File, *os.File) {
	t.Helper()
	var input, writer *os.File
	if kind == "socket" {
		fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
		require.NoError(t, err)
		input, writer = os.NewFile(uintptr(fds[0]), "socket-input"), os.NewFile(uintptr(fds[1]), "socket-writer")
	} else {
		var err error
		input, writer, err = os.Pipe()
		require.NoError(t, err)
	}
	// Fd makes Go-created pipes blocking, matching inherited stdin.
	_ = input.Fd()
	t.Cleanup(func() { _ = input.Close(); _ = writer.Close() })
	return input, writer
}

func TestRuntimeInputEOFCancelsAndReturnJoins(t *testing.T) {
	for _, kind := range []string{"pipe", "socket"} {
		for _, action := range []string{"EOF", "return", "cancel"} {
			t.Run(kind+"/"+action, func(t *testing.T) {
				input, writer := runtimeControlTestFiles(t, kind)
				parent, cancel := context.WithCancel(context.Background())
				defer cancel()
				ctx, finish, err := runtimeInputContext(parent, input)
				require.NoError(t, err)
				// The private channel has no payload; bytes must not request cancellation.
				_, err = writer.Write([]byte("ignored control payload"))
				require.NoError(t, err)
				select {
				case <-ctx.Done():
					t.Fatal("payload canceled the operation")
				default:
				}
				switch action {
				case "EOF":
					if kind == "socket" {
						// EOF includes write half-close while the socket's read half stays open.
						require.NoError(t, unix.Shutdown(int(writer.Fd()), unix.SHUT_WR))
					} else {
						require.NoError(t, writer.Close())
					}
					select {
					case <-ctx.Done():
					case <-time.After(5 * time.Second):
						t.Fatal("channel EOF did not cancel")
					}
				case "cancel":
					cancel()
				case "return":
				}
				done := make(chan struct{})
				go func() { finish(); close(done) }()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("command return did not join watcher")
				}
				require.ErrorIs(t, ctx.Err(), context.Canceled)
				_, err = input.Stat()
				require.NoError(t, err, "caller must retain ownership of original stdin")
			})
		}
	}
}

func TestRuntimeInputClosedFile(t *testing.T) {
	input, _ := runtimeControlTestFiles(t, "pipe")
	require.NoError(t, input.Close())
	ctx, finish, err := runtimeInputContext(context.Background(), input)
	require.Nil(t, ctx)
	require.Nil(t, finish)
	require.Error(t, err)
}
