// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func compileTreeHelper(t *testing.T) string {
	t.Helper()
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("real supervisor integration requires a C compiler")
	}
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../../../../"))
	build := t.TempDir()
	account := "nobody"
	if os.Geteuid() == 0 {
		account = "root"
	}
	config := fmt.Sprintf(
		"#define _GNU_SOURCE 1\n#define HAVE_SETRESUID 1\n#define HAVE_SETRESGID 1\n#define NETDATA_USER %q\n",
		account,
	)
	require.NoError(t, os.WriteFile(filepath.Join(build, "config.h"), []byte(config), 0600))
	sources := filepath.Join(root, "src/collectors/utils")
	helper := filepath.Join(build, "nd-run")
	cmd := exec.Command(cc, "-std=gnu11", "-Wall", "-Wextra", "-Werror", "-I", build, "-I", sources,
		filepath.Join(sources, "nd-run.c"), filepath.Join(sources, "nd-file-reader.c"),
		filepath.Join(sources, "nd-process-tree.c"), "-o", helper)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "compile real supervisor: %s", output)
	return helper
}

func treePayloadArgs(mode, dir string) []string {
	return []string{"-test.run=^TestTreePayload$", "--", mode, dir}
}

// Each fixture has an independent lifetime bound, including if the owner is
// broken. The driver records the leaf identity with pidfd before cancellation.
func TestTreePayload(t *testing.T) {
	sep := slices.Index(os.Args, "--")
	if sep < 0 {
		return
	}
	mode, dir := os.Args[sep+1], os.Args[sep+2]
	signal.Ignore(syscall.SIGTERM, syscall.SIGINT)
	if mode == "leaf" {
		pidPath := filepath.Join(dir, "leaf.pid")
		pendingPath := pidPath + ".pending"
		// File existence signals readiness; publish only after the PID is written and closed.
		if os.WriteFile(pendingPath, []byte(strconv.Itoa(os.Getpid())), 0600) != nil {
			os.Exit(2)
		}
		if os.Rename(pendingPath, pidPath) != nil {
			os.Exit(2)
		}
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	childMode := "leaf"
	if mode == "double-fork" {
		childMode = "middle"
	}
	child := exec.Command(os.Args[0], treePayloadArgs(childMode, dir)...)
	child.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
	if child.Start() != nil {
		os.Exit(3)
	}
	if mode == "middle" {
		os.Exit(0)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "leaf.pid")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			os.Exit(4)
		}
		time.Sleep(time.Millisecond)
	}
	if os.WriteFile(filepath.Join(dir, "ready"), []byte("ready"), 0600) != nil {
		os.Exit(5)
	}
	if mode == "busy" {
		for time.Now().Before(deadline) {
		}
	} else {
		var input [1]byte
		_, _ = os.Stdin.Read(input[:])
	}
	os.Exit(0)
}

func leafIdentity(t *testing.T, dir string) int {
	t.Helper()
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(dir, "ready"))
		return err == nil
	}, 10*time.Second, time.Millisecond)
	data, err := os.ReadFile(filepath.Join(dir, "leaf.pid"))
	require.NoError(t, err)
	pid, err := strconv.Atoi(string(data))
	require.NoError(t, err)
	fd, err := unix.PidfdOpen(pid, 0)
	require.NoError(t, err, "pidfd fixture requires Linux 5.3+")
	t.Cleanup(func() { _ = unix.Close(fd) })
	return fd
}

func requireTreeExited(t *testing.T, identity int) {
	t.Helper()
	fds := []unix.PollFd{{Fd: int32(identity), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, 0)
	require.NoError(t, err)
	require.Equal(t, 1, n, "leaf still alive after verified drain")
	assert.NotZero(t, fds[0].Revents&unix.POLLIN)
}

func TestProcessTreeActualDescendants(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	helper := compileTreeHelper(t)
	for _, test := range []struct{ mode, action string }{
		{"parent", "command exit"}, {"parent", "cancel"},
		{"double-fork", "close"}, {"busy", "cancel"},
	} {
		t.Run(test.mode+"/"+test.action, func(t *testing.T) {
			dir := t.TempDir()
			stdin, input, err := os.Pipe()
			require.NoError(t, err)
			defer stdin.Close()
			defer input.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			args := treePayloadArgs(test.mode, dir)
			process, err := startProcessTree(ctx, helper, ProcessOptions{
				Stdin: stdin,
			}, os.Args[0], args...)
			require.NoError(t, err)
			t.Cleanup(func() { process.Close() })
			identity := leafIdentity(t, dir)
			if test.action == "command exit" {
				_, err = input.Write([]byte("x"))
				require.NoError(t, err)
			}
			if test.action == "cancel" {
				cancel()
			}
			done := make(chan TreeResult, 1)
			go func() {
				if test.action == "close" {
					done <- process.Close()
				} else {
					done <- process.Wait()
				}
			}()
			select {
			case result := <-done:
				require.True(t, result.Drained, "cleanup not verified: %v", result.Err)
				if test.action == "command exit" {
					assert.NoError(t, result.Err)
				} else {
					assert.ErrorIs(t, result.Err, context.Canceled)
				}
				requireTreeExited(t, identity)
				assert.Equal(t, result, process.Wait())
				assert.Equal(t, result, process.Close())
			case <-time.After(15 * time.Second):
				t.Fatal("supervisor did not finish fixture cleanup")
			}
		})
	}
}

func TestProcessTreeCommandOutcomeAndPrivateProtocol(t *testing.T) {
	helper := compileTreeHelper(t)
	restorePaths := SetRunnerPathsForTests(helper, "")
	defer restorePaths()
	output, err := os.CreateTemp(t.TempDir(), "stdout")
	require.NoError(t, err)
	defer output.Close()
	process, err := StartUnprivilegedProcessTree(
		context.Background(),
		ProcessOptions{
			Stdout: output,
		},
		"/bin/sh",
		"-c",
		"printf 'NDTREE1 0\n'; exit 23",
	)
	require.NoError(t, err)
	result := process.Wait()
	require.True(t, result.Drained, "cleanup not verified: %v", result.Err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, result.Err, &exitErr)
	assert.Equal(t, 23, exitErr.ExitCode())
	_, err = output.Seek(0, io.SeekStart)
	require.NoError(t, err)
	data, err := io.ReadAll(output)
	require.NoError(t, err)
	assert.Equal(t, "NDTREE1 0\n", string(data))

	missing, err := startProcessTree(
		context.Background(),
		helper,
		ProcessOptions{},
		"/netdata-tree-fixture-does-not-exist",
	)
	require.NoError(t, err)
	outcome := missing.Wait()
	require.True(t, outcome.Drained, "failed exec must still finish cleanup: %v", outcome.Err)
	require.ErrorAs(t, outcome.Err, &exitErr)
	assert.Equal(t, 127, exitErr.ExitCode())
}

func TestProcessTreeMissingCompletionIsNotDrain(t *testing.T) {
	for _, script := range []string{
		"exit 0", "printf 'NDTREE1 unavailable\n' >&4; exit 0", "printf 'NDTREE1 ' >&4; exit 0", "printf 'NDTREE1 256\n' >&4; exit 0",
	} {
		t.Run(strconv.Itoa(len(script)), func(t *testing.T) {
			helper := filepath.Join(t.TempDir(), "broken-helper")
			require.NoError(t, os.WriteFile(helper, []byte("#!/bin/sh\n"+script+"\n"), 0700))
			process, err := startProcessTree(context.Background(), helper, ProcessOptions{}, "unused")
			require.NoError(t, err)
			result := process.Wait()
			assert.False(t, result.Drained)
			assert.ErrorIs(t, result.Err, ErrTreeNotDrained)
		})
	}
}

func TestProcessTreePreCanceledAndMissingHelper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	process, err := startProcessTree(ctx, "missing", ProcessOptions{}, "unused")
	assert.Nil(t, process)
	assert.ErrorIs(t, err, context.Canceled)
	process, err = startProcessTree(
		context.Background(),
		filepath.Join(t.TempDir(), "missing"),
		ProcessOptions{},
		"unused",
	)
	assert.Nil(t, process)
	assert.True(t, errors.Is(err, os.ErrNotExist))
}

func TestProcessTreeConcurrentWaitClose(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	helper := compileTreeHelper(t)
	dir := t.TempDir()
	stdin, input, err := os.Pipe()
	require.NoError(t, err)
	defer stdin.Close()
	defer input.Close()
	process, err := startProcessTree(
		context.Background(),
		helper,
		ProcessOptions{
			Stdin: stdin,
		},
		os.Args[0],
		treePayloadArgs("parent", dir)...)
	require.NoError(t, err)
	t.Cleanup(func() { process.Close() })
	identity := leafIdentity(t, dir)
	var wg sync.WaitGroup
	results := make(chan TreeResult, 8)
	for i := range 8 {
		wg.Go(func() {
			if i%2 == 0 {
				results <- process.Close()
			} else {
				results <- process.Wait()
			}
		})
	}
	wg.Wait()
	close(results)
	result := process.Wait()
	require.True(t, result.Drained, "cleanup not verified: %v", result.Err)
	assert.ErrorIs(t, result.Err, context.Canceled)
	for received := range results {
		assert.Equal(t, result, received)
	}
	requireTreeExited(t, identity)
}

func TestTreeCompletionRejectsNonterminalAndExtraFrames(t *testing.T) {
	// Protocol corruption cannot turn a helper's successful exit into evidence.
	for _, frame := range []string{"NDTREE1 127\n", "NDTREE1 128\n", "NDTREE1 0\nNDTREE1 0\n", "NDTREE1 +0\n", "NDTREE1 00\n"} {
		t.Run(strings.ReplaceAll(frame, "\n", "_"), func(t *testing.T) {
			result := treeCompletion([]byte(frame), nil)
			assert.False(t, result.Drained)
			assert.ErrorIs(t, result.Err, ErrTreeNotDrained)
		})
	}
}

func TestProcessTreeUnavailableIsDrainedSetupFailure(t *testing.T) {
	helper := filepath.Join(t.TempDir(), "unavailable-helper")
	require.NoError(t, os.WriteFile(helper, []byte("#!/bin/sh\nprintf 'NDTREE1 unavailable\\n' >&4\nexit 126\n"), 0700))
	process, err := startProcessTree(context.Background(), helper, ProcessOptions{}, "unused")
	require.NoError(t, err)
	result := process.Wait()
	assert.True(t, result.Drained)
	assert.ErrorIs(t, result.Err, ErrTreeSupervisionUnavailable)
	assert.NotErrorIs(t, result.Err, ErrTreeNotDrained)
	var exitErr *exec.ExitError
	require.ErrorAs(t, result.Err, &exitErr)
	assert.Equal(t, 126, exitErr.ExitCode())
}
