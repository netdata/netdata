// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package credentialfile

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/stretchr/testify/require"
)

// This synthetic peer verifies the client, not the C privilege transition.
func TestProtocolPeer(t *testing.T) {
	if os.Getenv("NETDATA_CREDENTIALFILE_TEST_PEER") != "1" {
		return
	}
	args := os.Args
	for len(args) > 0 && args[0] != "--" {
		args = args[1:]
	}
	args = args[1:]
	path := args[len(args)-1]
	finish := func(code int, errno syscall.Errno) {
		fmt.Fprintf(os.Stderr, "NDFILE %d %d\n", code, errno)
		if code != 0 {
			os.Exit(1)
		}
		os.Exit(0)
	}
	switch path {
	case "@block":
		fmt.Fprint(os.Stdout, "r")
		for {
			time.Sleep(time.Hour)
		}
	case "@stdout-closed":
		fmt.Fprint(os.Stdout, "r")
		os.Stdout.Close()
		fmt.Fprint(os.Stderr, "NDFILE 0 0\n")
		for {
			time.Sleep(time.Hour)
		}
	case "@partial-error":
		fmt.Fprint(os.Stdout, "SYNTHETIC_PRIVATE_BYTES")
		finish(3, syscall.EIO)
	case "@permission":
		finish(1, syscall.EACCES)
	case "@pid":
		fmt.Fprint(os.Stdout, os.Getpid())
		finish(0, 0)
	case "@absent-result":
		fmt.Fprint(os.Stdout, "SYNTHETIC_PRIVATE_BYTES")
		os.Exit(0)
	case "@duplicate":
		fmt.Fprint(os.Stderr, "NDFILE 0 0\nNDFILE 0 0\n")
		os.Exit(0)
	case "@truncated":
		fmt.Fprint(os.Stderr, "NDFILE 0 0")
		os.Exit(0)
	case "@diagnostic":
		fmt.Fprint(os.Stderr, strings.Repeat("SYNTHETIC_PRIVATE_BYTES", 10000))
		os.Exit(1)
	case "@success-exit1":
		fmt.Fprint(os.Stderr, "NDFILE 0 0\n")
		os.Exit(1)
	case "@error-exit0":
		fmt.Fprint(os.Stderr, "NDFILE 1 2\n")
		os.Exit(0)
	case "@error-exit2":
		fmt.Fprint(os.Stderr, "NDFILE 1 2\n")
		os.Exit(2)
	case "@signal":
		fmt.Fprint(os.Stderr, "NDFILE 1 2\n")
		syscall.Kill(os.Getpid(), syscall.SIGKILL)
	case "@bad-code":
		fmt.Fprint(os.Stderr, "NDFILE 7 0\n")
		os.Exit(1)
	case "@negative-errno":
		fmt.Fprint(os.Stderr, "NDFILE 1 -2\n")
		os.Exit(1)
	case "@missing-errno":
		fmt.Fprint(os.Stderr, "NDFILE 1 0\n")
		os.Exit(1)
	case "@policy-errno":
		fmt.Fprint(os.Stderr, "NDFILE 5 2\n")
		os.Exit(1)
	case "@success-errno":
		fmt.Fprint(os.Stderr, "NDFILE 0 2\n")
		os.Exit(0)
	case "@bad-stat":
		fmt.Fprint(os.Stdout, "0 1000000000\n")
		finish(0, 0)
	case "@extra-stat":
		fmt.Fprint(os.Stdout, "0 0\nextra")
		finish(0, 0)
	case "@large-output":
		os.Stdout.Write(bytes.Repeat([]byte("x"), int(safefile.MaxSize)+1))
		finish(0, 0)
	}
	if args[0] == "stat" {
		info, err := os.Stat(path)
		if err != nil {
			finish(2, err.(*os.PathError).Err.(syscall.Errno))
		}
		fmt.Fprintf(os.Stdout, "%d %d\n", info.ModTime().Unix(), info.ModTime().Nanosecond())
		finish(0, 0)
	}
	if len(args) != 4 || args[0] != "read" {
		os.Exit(2)
	}
	limit, err := strconv.ParseInt(args[2], 10, 64)
	if err != nil {
		os.Exit(2)
	}
	file, err := os.Open(path)
	if err != nil {
		finish(1, err.(*os.PathError).Err.(syscall.Errno))
	}
	if args[1] == "regular" {
		if limit != safefile.MaxSize {
			os.Exit(2)
		}
		info, err := file.Stat()
		if err != nil {
			finish(2, syscall.EIO)
		}
		if !info.Mode().IsRegular() {
			finish(5, 0)
		}
		if info.Size() > limit {
			finish(6, 0)
		}
	} else if args[1] != "stream" || limit != 0 {
		os.Exit(2)
	}
	if _, err = io.Copy(os.Stdout, file); err != nil {
		finish(3, syscall.EIO)
	}
	file.Close()
	finish(0, 0)
}

func peerCommand(args ...string) *exec.Cmd {
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestProtocolPeer$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "NETDATA_CREDENTIALFILE_TEST_PEER=1")
	return cmd
}

func testFile(t *testing.T, contents string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "credential")
	require.NoError(t, os.WriteFile(p, []byte(contents), 0600))
	return p
}

func TestReadFreshnessAndPolicies(t *testing.T) {
	ctx := context.Background()
	p := testFile(t, "first")
	got, err := read(ctx, p, true, peerCommand)
	require.NoError(t, err)
	require.Equal(t, "first", string(got))
	replacement := testFile(t, "rotated")
	require.NoError(t, os.Rename(replacement, p))
	got, err = read(ctx, p, true, peerCommand)
	require.NoError(t, err)
	require.Equal(t, "rotated", string(got))
	info, err := os.Stat(p)
	require.NoError(t, err)
	mt, err := stat(ctx, p, peerCommand)
	require.NoError(t, err)
	require.True(t, mt.Equal(info.ModTime()))
	for _, size := range []int{int(safefile.MaxSize), int(safefile.MaxSize) + 1} {
		require.NoError(t, os.WriteFile(p, bytes.Repeat([]byte("x"), size), 0600))
		got, err = read(ctx, p, true, peerCommand)
		if size > int(safefile.MaxSize) {
			require.ErrorIs(t, err, safefile.ErrTooLarge)
			require.Nil(t, got)
		} else {
			require.NoError(t, err)
			require.Len(t, got, size)
		}
		got, err = read(ctx, p, false, peerCommand)
		require.NoError(t, err)
		require.Len(t, got, size)
	}
	first, err := read(ctx, "@pid", true, peerCommand)
	require.NoError(t, err)
	second, err := read(ctx, "@pid", true, peerCommand)
	require.NoError(t, err)
	require.NotEqual(t, string(first), string(second))
}

func TestReadFileErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		path string
		want error
	}{
		"missing": {filepath.Join(t.TempDir(), "missing"), os.ErrNotExist},
		"empty":   {"", os.ErrNotExist}, "directory": {t.TempDir(), safefile.ErrNotRegular},
		"permission": {"@permission", os.ErrPermission}, "read": {"@partial-error", syscall.EIO},
		"nul": {"bad\x00path", syscall.EINVAL}, "long": {strings.Repeat("x", 4096), syscall.ENAMETOOLONG},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := read(context.Background(), tc.path, true, peerCommand)
			require.Nil(t, got)
			require.ErrorIs(t, err, safefile.ErrFile)
			require.ErrorIs(t, err, tc.want)
			require.NotContains(t, err.Error(), "SYNTHETIC_PRIVATE_BYTES")
		})
	}
}

func TestTransportFailures(t *testing.T) {
	for _, path := range []string{"@absent-result", "@duplicate", "@truncated", "@diagnostic", "@success-exit1", "@error-exit0", "@error-exit2", "@signal", "@bad-code", "@negative-errno", "@missing-errno", "@policy-errno", "@success-errno", "@large-output"} {
		t.Run(path, func(t *testing.T) {
			got, err := read(context.Background(), path, true, peerCommand)
			require.Nil(t, got)
			require.ErrorIs(t, err, safefile.ErrFile)
			require.NotErrorIs(t, err, os.ErrNotExist)
			require.NotContains(t, err.Error(), "SYNTHETIC_PRIVATE_BYTES")
		})
	}
	_, err := read(
		context.Background(),
		"target",
		true,
		func(...string) *exec.Cmd { return exec.Command(filepath.Join(t.TempDir(), "missing-helper")) },
	)
	require.ErrorIs(t, err, safefile.ErrFile)
	require.NotErrorIs(t, err, os.ErrNotExist)
}

func TestStreamReportsTerminalFailure(t *testing.T) {
	for _, path := range []string{"@partial-error", "@absent-result", "@success-exit1"} {
		t.Run(path, func(t *testing.T) {
			s, err := open(context.Background(), path, peerCommand, "read", "stream", "0", path)
			require.NoError(t, err)
			defer s.Close()
			_, err = io.ReadAll(s)
			require.Error(t, err)
			require.NotErrorIs(t, err, io.EOF)
			_, again := s.Read(make([]byte, 1))
			require.Equal(t, err, again)
		})
	}
}

func waitReaped(t *testing.T, s *stream) {
	t.Helper()
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		t.Fatal("helper was not reaped")
	}
	require.NotNil(t, s.cmd.ProcessState)
}

func TestStreamCancellationAndClose(t *testing.T) {
	for _, path := range []string{"@block", "@stdout-closed"} {
		for _, action := range []string{"cancel", "close"} {
			t.Run(path+"/"+action, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				s, err := open(ctx, path, peerCommand, "read", "stream", "0", path)
				require.NoError(t, err)
				defer s.Close()
				_, err = io.ReadFull(s, make([]byte, 1))
				require.NoError(t, err)
				done := make(chan error, 1)
				go func() { _, err := io.ReadAll(s); done <- err }()
				if action == "cancel" {
					cancel()
				} else {
					require.NoError(t, s.Close())
					require.NoError(t, s.Close())
				}
				select {
				case err = <-done:
					require.Error(t, err)
				case <-time.After(time.Second):
					t.Fatal("active read was not interrupted")
				}
				if action == "cancel" {
					require.ErrorIs(t, err, context.Canceled)
				}
				waitReaped(t, s)
			})
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := read(
		ctx,
		"unused",
		true,
		func(...string) *exec.Cmd { t.Fatal("started for canceled context"); return nil },
	)
	require.ErrorIs(t, err, context.Canceled)
}

func TestCloseAndCancellationAreIsolated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	first, err := open(ctx, "@pid", peerCommand, "read", "stream", "0", "@pid")
	require.NoError(t, err)
	defer first.Close()
	_, err = io.ReadAll(first)
	require.NoError(t, err)
	second, err := open(context.Background(), "@block", peerCommand, "read", "stream", "0", "@block")
	require.NoError(t, err)
	defer second.Close()
	cancel()
	require.NoError(t, first.Close())
	select {
	case <-second.done:
		t.Fatal("unrelated process was stopped")
	case <-time.After(20 * time.Millisecond):
	}
	require.NoError(t, second.Close())
	waitReaped(t, second)
}

func TestConcurrentOperations(t *testing.T) {
	p := testFile(t, strings.Repeat("value", 20000))
	errs := make(chan error, 8)
	for i := 0; i < cap(errs); i++ {
		go func() {
			got, err := read(context.Background(), p, true, peerCommand)
			if err == nil && len(got) != 100000 {
				err = fmt.Errorf("unexpected byte count: %d", len(got))
			}
			errs <- err
		}()
	}
	for i := 0; i < cap(errs); i++ {
		require.NoError(t, <-errs)
	}
}

func TestStatFailures(t *testing.T) {
	_, err := stat(context.Background(), filepath.Join(t.TempDir(), "missing"), peerCommand)
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, path := range []string{"@bad-stat", "@extra-stat", "@large-output", "@absent-result", "@pid"} {
		_, err = stat(context.Background(), path, peerCommand)
		require.ErrorIs(t, err, safefile.ErrFile)
	}
}
