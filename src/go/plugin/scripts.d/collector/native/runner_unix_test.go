// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package native

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCancellationContainsDescendant(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(
		t,
		"dir=$(dirname \"$0\")\n(sleep 1; printf escaped > \"$dir/escaped\") &\nprintf started > \"$dir/started\"\nwait\n",
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := runCommand(ctx, 5*time.Second, c.definition.Command, nil); done <- err }()
	require.Eventually(
		t,
		func() bool { _, err := os.Stat(filepath.Join(dir, "started")); return err == nil },
		2*time.Second,
		10*time.Millisecond,
	)
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("command did not stop")
	}
	// The descendant has passed its scheduled write time. A surviving child would
	// create this marker even after the parent and command's pipes were closed.
	time.Sleep(1100 * time.Millisecond)
	_, err := os.Stat(filepath.Join(dir, "escaped"))
	assert.True(t, os.IsNotExist(err), "descendant survived cancellation")
}

func TestPersistentContainsDescendantAfterParentExit(t *testing.T) {
	setupRunner(t)
	for _, phase := range []string{"startup", "idle"} {
		t.Run(phase, func(t *testing.T) {
			body := `dir=$(dirname "$0")
(sleep 2; printf escaped > "$dir/escaped") &
printf started > "$dir/started"
`
			if phase == "idle" {
				body += bashHelper(t) + "nd_ready\nnd_next\nnd_begin\nnd_end\n"
			}
			body += "exit 0\n"
			c, dir := persistentCollector(t, body)
			r := startRuntime(t, c)
			if phase == "idle" {
				r.waitReady(t)
				// Either the final reply or the observed process exit may win.
				_, _ = collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			}
			r.wait(t)
			require.Error(t, r.err)
			_, err := os.Stat(filepath.Join(dir, "started"))
			require.NoError(t, err)
			time.Sleep(2100 * time.Millisecond)
			_, err = os.Stat(filepath.Join(dir, "escaped"))
			assert.True(t, os.IsNotExist(err), "descendant survived parent exit")
		})
	}
}

func TestPersistentFailedStartupClosesPipes(t *testing.T) {
	if os.Getenv("NATIVE_TEST_FD_LIMIT") != "1" {
		// Never lower the test runner's descriptor limit: isolate this failure path.
		cmd := exec.Command(os.Args[0], "-test.run=^TestPersistentFailedStartupClosesPipes$")
		cmd.Env = append(os.Environ(), "NATIVE_TEST_FD_LIMIT=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return
	}
	debug.SetGCPercent(-1) // A finalizer must not disguise a leaked owned descriptor.
	var limit syscall.Rlimit
	require.NoError(t, syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit))
	limit.Cur = 64
	require.NoError(t, syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit))
	// Initialize the runtime pipe poller before exhausting descriptors.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	require.NoError(t, r.Close())
	require.NoError(t, w.Close())
	var files []*os.File
	defer func() {
		for _, file := range files {
			_ = file.Close()
		}
	}()
	for {
		file, err := os.Open(os.DevNull)
		if err != nil {
			require.ErrorIs(t, err, syscall.EMFILE)
			break
		}
		files = append(files, file)
	}
	require.Greater(t, len(files), 2)
	// Only stdin's two descriptors fit; stdout allocation must fail.
	for _, file := range files[len(files)-2:] {
		require.NoError(t, file.Close())
	}
	files = files[:len(files)-2]
	_, err = startSession(context.Background(), []string{"/never-started"})
	require.ErrorIs(t, err, syscall.EMFILE)
	for i := 0; i < 2; i++ {
		file, err := os.Open(os.DevNull)
		require.NoError(t, err, "failed startup leaked a pipe descriptor")
		files = append(files, file)
	}
}

func TestPersistentStopAfterReadiness(t *testing.T) {
	setupRunner(t)
	for i := 0; i < 10; i++ {
		c, dir := persistentCollector(t, bashHelper(t)+`
printf '%s' "$$" > "$(dirname "$0")/pid"
nd_ready
while nd_next; do nd_begin; nd_end; done
`)
		ctx, cancel := context.WithCancel(context.Background())
		err := c.Run(ctx, func() {
			data, err := os.ReadFile(filepath.Join(dir, "pid"))
			require.NoError(t, err)
			pid, err := strconv.Atoi(string(data))
			require.NoError(t, err)
			cancel()
			// Model a scheduler pause after readiness: cancellation and the reaped
			// process are both observable before Run resumes its idle select.
			require.Eventually(
				t,
				func() bool { return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) },
				time.Second,
				time.Millisecond,
			)
		})
		cancel()
		require.ErrorIs(t, err, context.Canceled)
	}
}
