// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package native

import (
	"context"
	"os"
	"os/exec"
	"runtime/debug"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartSession_FailedStartupClosesPipes(t *testing.T) {
	if os.Getenv("NATIVE_TEST_FD_LIMIT") != "1" {
		// Never lower the test runner's descriptor limit: isolate this failure path.
		cmd := exec.Command(os.Args[0], "-test.run=^TestStartSession_FailedStartupClosesPipes$")
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
	for range 2 {
		file, err := os.Open(os.DevNull)
		require.NoError(t, err, "failed startup leaked a pipe descriptor")
		files = append(files, file)
	}
}
