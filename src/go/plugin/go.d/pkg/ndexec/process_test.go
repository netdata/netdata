// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin || freebsd || windows

package ndexec

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The test binary stands in for nd-run; its explicit test arguments act as argv.
func ownedHelperArgs(mode, dir string) []string {
	return []string{"-test.run=^TestOwnedProcessHelper$", "--", mode, dir}
}

func TestOwnedProcessHelper(t *testing.T) {
	sep := slices.Index(os.Args, "--")
	if sep < 0 {
		return
	}
	mode, dir := os.Args[sep+1], os.Args[sep+2]
	if mode == "success" {
		os.Exit(0)
	}
	if mode == "failure" {
		os.Exit(23)
	}
	if mode == "writer" {
		// Writes until a closed output ends it with SIGPIPE; a broken owner cannot leave it alive indefinitely.
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
			fmt.Println("line")
			time.Sleep(10 * time.Millisecond)
		}
		os.Exit(0)
	}
	if mode == "descendant" {
		if err := os.WriteFile(filepath.Join(dir, "child-ready"), []byte("ready"), 0600); err != nil {
			os.Exit(2)
		}
		time.Sleep(1200 * time.Millisecond)
		if err := os.WriteFile(filepath.Join(dir, "escaped"), []byte("survived"), 0600); err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	child := exec.Command(os.Args[0], ownedHelperArgs("descendant", dir)...)
	child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(4)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "child-ready")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = child.Process.Kill()
			_ = child.Wait()
			os.Exit(5)
		}
		time.Sleep(time.Millisecond)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent-ready"), []byte("ready"), 0600); err != nil {
		os.Exit(6)
	}
	if mode == "parent-exit" {
		os.Exit(0)
	}
	// A broken owner cannot leave the fixture alive indefinitely.
	time.Sleep(4 * time.Second)
	_ = child.Wait()
	os.Exit(0)
}

func TestOwnedProcessContainsChildren(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	restore := SetRunnerPathsForTests(os.Args[0], "")
	t.Cleanup(restore)
	for _, action := range []string{"leader exit", "cancel", "close"} {
		t.Run(action, func(t *testing.T) {
			dir := t.TempDir()
			mode := "parent-wait"
			if action == "leader exit" {
				mode = "parent-exit"
			}
			args := ownedHelperArgs(mode, dir)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			p, err := StartUnprivilegedProcess(ctx, ProcessOptions{}, args[0], args[1:]...)
			require.NoError(t, err)
			t.Cleanup(func() { _ = p.Close() })
			require.Eventually(
				t,
				func() bool { _, err := os.Stat(filepath.Join(dir, "parent-ready")); return err == nil },
				3*time.Second,
				time.Millisecond,
			)
			done := make(chan error, 1)
			if action == "close" {
				go func() { done <- p.Close() }()
			} else {
				if action == "cancel" {
					cancel()
				}
				go func() { done <- p.Wait() }()
			}
			select {
			case err := <-done:
				if action == "leader exit" {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			case <-time.After(time.Second):
				t.Fatal("owned process cleanup did not finish")
			}
			time.Sleep(1300 * time.Millisecond)
			_, err = os.Stat(filepath.Join(dir, "escaped"))
			assert.True(t, os.IsNotExist(err), "descendant survived cleanup")
		})
	}
}

func TestOwnedProcessExitAndStartErrors(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	restore := SetRunnerPathsForTests(os.Args[0], "")
	t.Cleanup(restore)
	for _, mode := range []string{"success", "failure"} {
		t.Run(mode, func(t *testing.T) {
			args := ownedHelperArgs(mode, t.TempDir())
			p, err := StartUnprivilegedProcess(context.Background(), ProcessOptions{}, args[0], args[1:]...)
			require.NoError(t, err)
			err = p.Wait()
			if mode == "success" {
				require.NoError(t, err)
			} else {
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr)
				assert.Equal(t, 23, exitErr.ExitCode())
			}
			assert.Equal(t, err, p.Close())
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p, err := StartUnprivilegedProcess(ctx, ProcessOptions{}, "unused")
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, p)
	restoreMissing := SetRunnerPathsForTests(filepath.Join(t.TempDir(), "missing-helper"), "")
	defer restoreMissing()
	p, err = StartUnprivilegedProcess(context.Background(), ProcessOptions{}, "unused")
	require.Error(t, err)
	assert.Nil(t, p)
}

type recordingProcess struct {
	processHandle
	mu    sync.Mutex
	calls []string
}

func (p *recordingProcess) record(call string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, call)
}
func (p *recordingProcess) terminate() error {
	p.record("terminate")
	return p.processHandle.terminate()
}
func (p *recordingProcess) reap() error    { p.record("reap"); return p.processHandle.reap() }
func (p *recordingProcess) release() error { p.record("release"); return p.processHandle.release() }

func TestOwnedProcessDisarmsTerminationBeforeReaping(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	// Use a real child and platform observer; record the actual termination/reap
	// boundary. Never force PID reuse or signal another task's processes.
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	require.NoError(t, err)
	defer null.Close()
	opts := ProcessOptions{
		Stdin:  null,
		Stdout: null,
		Stderr: null,
	}
	for i := 0; i < 10; i++ {
		handle, err := startOwnedProcess(os.Args[0], ownedHelperArgs("success", t.TempDir()), opts)
		require.NoError(t, err)
		record := &recordingProcess{
			processHandle: handle,
		}
		ctx, cancel := context.WithCancel(context.Background())
		p := ownProcess(ctx, record)
		require.NoError(t, p.Wait())
		cancel()
		var wg sync.WaitGroup
		for j := 0; j < 10; j++ {
			wg.Add(1)
			go func() { defer wg.Done(); _ = p.Close() }()
		}
		wg.Wait()
		assert.Equal(t, []string{"terminate", "reap", "release"}, record.calls)
	}
}
