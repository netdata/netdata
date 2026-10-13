// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package credentialfile

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/stretchr/testify/require"
)

// These opt-in tests exercise the actual nd-run wire implementation. The caller
// supplies a prebuilt helper; ordinary package tests need no C toolchain.
func cHelperPath(t testing.TB) string {
	t.Helper()
	path := os.Getenv("NETDATA_TEST_ND_RUN")
	if path == "" {
		t.Skip("set NETDATA_TEST_ND_RUN to a prebuilt nd-run to test C interoperability")
	}
	path, err := filepath.Abs(path)
	require.NoError(t, err)
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular())
	return path
}

func cCommand(path string) commandFunc {
	return func(args ...string) *exec.Cmd {
		return exec.Command(path, append([]string{"--file-reader"}, args...)...)
	}
}

func cFixtureDir(t testing.TB) string {
	t.Helper()
	// /tmp is traversable by a reduced-identity helper, unlike testing.TempDir's
	// potentially private per-user parent on macOS. Only synthetic fixtures live here.
	dir, err := os.MkdirTemp("/tmp", "netdata-credentialfile-test-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	require.NoError(t, os.Chmod(dir, 0755))
	return dir
}

func cWrite(t testing.TB, path string, data []byte, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, data, mode))
	require.NoError(t, os.Chmod(path, mode))
}

func TestCReaderInteroperability(t *testing.T) {
	helper := cHelperPath(t)
	dir := cFixtureDir(t)
	command := cCommand(helper)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p := filepath.Join(dir, "-credential with spaces")
	first := []byte("synthetic-initial-value")
	cWrite(t, p, first, 0644)
	got, err := read(ctx, p, true, command)
	require.NoError(t, err)
	require.Equal(t, first, got)
	// Rotation replaces the inode, as projected secrets and atomic writers do.
	replacement := filepath.Join(dir, "replacement")
	rotated := []byte("synthetic-rotated-value")
	cWrite(t, replacement, rotated, 0644)
	require.NoError(t, os.Rename(replacement, p))
	got, err = read(ctx, p, true, command)
	require.NoError(t, err)
	require.Equal(t, rotated, got)
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(p, link))
	got, err = read(ctx, link, true, command)
	require.NoError(t, err)
	require.Equal(t, rotated, got)
	info, err := os.Stat(p)
	require.NoError(t, err)
	mod, err := stat(ctx, link, command)
	require.NoError(t, err)
	require.True(t, info.ModTime().Equal(mod))
	_, err = read(ctx, filepath.Join(dir, "missing"), true, command)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = stat(ctx, filepath.Join(dir, "missing"), command)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = read(ctx, "", true, command)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = read(ctx, dir, true, command)
	require.ErrorIs(t, err, safefile.ErrNotRegular)
	fifo := filepath.Join(dir, "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0644))
	_, err = read(ctx, fifo, true, command)
	require.ErrorIs(t, err, safefile.ErrNotRegular)
	large := bytes.Repeat([]byte("x"), int(safefile.MaxSize)+1)
	cWrite(t, p, large, 0644)
	got, err = read(ctx, p, true, command)
	require.Nil(t, got)
	require.ErrorIs(t, err, safefile.ErrTooLarge)
	got, err = read(ctx, p, false, command)
	require.NoError(t, err)
	require.Equal(t, large, got)
	stream, err := open(ctx, p, command, "read", "stream", "0", p)
	require.NoError(t, err)
	defer stream.Close()
	got, err = io.ReadAll(stream)
	require.NoError(t, err)
	require.Equal(t, large, got)
	require.NoError(t, stream.Close())
}

func TestCReaderPermissionDenial(t *testing.T) {
	helper := cHelperPath(t)
	dir := cFixtureDir(t)
	command := cCommand(helper)
	p := filepath.Join(dir, "restricted")
	private := []byte("SYNTHETIC_RESTRICTED_CONTENT")
	mode := os.FileMode(0000)
	if os.Geteuid() == 0 {
		mode = 0600
	}
	cWrite(t, p, private, mode)
	if os.Geteuid() == 0 {
		// An elevated parent can read this fixture; its reduced helper must not.
		got, err := safefile.Read(p)
		require.NoError(t, err)
		require.Equal(t, private, got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := read(ctx, p, true, command)
	require.Nil(t, got)
	require.ErrorIs(t, err, os.ErrPermission)
	require.ErrorIs(t, err, safefile.ErrFile)
	require.NotContains(t, err.Error(), string(private))
}

func TestCReaderBlockedFIFOCancellation(t *testing.T) {
	helper := cHelperPath(t)
	dir := cFixtureDir(t)
	command := cCommand(helper)
	fifo := filepath.Join(dir, "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0644))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	stream, err := open(ctx, fifo, command, "read", "stream", "0", fifo)
	require.NoError(t, err)
	_, err = io.ReadAll(stream)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, stream.Close())
	waitReaped(t, stream)
	p := filepath.Join(dir, "readable")
	cWrite(t, p, []byte("after-cancellation"), 0644)
	nextCtx, nextCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer nextCancel()
	got, err := read(nextCtx, p, true, command)
	require.NoError(t, err)
	require.Equal(t, "after-cancellation", string(got))
	// Keep a writer open so the next stream blocks in read(2), after proving
	// the helper opened the FIFO and delivered a byte.
	writer, err := os.OpenFile(fifo, os.O_RDWR, 0)
	require.NoError(t, err)
	defer writer.Close()
	stream, err = open(nextCtx, fifo, command, "read", "stream", "0", fifo)
	require.NoError(t, err)
	_, err = writer.Write([]byte("r"))
	require.NoError(t, err)
	_, err = io.ReadFull(stream, make([]byte, 1))
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(stream); done <- err }()
	require.NoError(t, stream.Close())
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("Close did not interrupt FIFO read")
	}
	require.NoError(t, stream.Close())
	waitReaped(t, stream)
}

// This is a machine-local cost comparison, not a timing CI gate. Both paths read
// a synthetic 256-byte regular file without caching. The helper case includes
// process startup and reaping per operation; each read is O(path bytes + file bytes).
func BenchmarkCredentialRead256(b *testing.B) {
	helper := cHelperPath(b)
	dir := cFixtureDir(b)
	p := filepath.Join(dir, "credential")
	cWrite(b, p, bytes.Repeat([]byte("x"), 256), 0644)
	b.Run("safefile", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(256)
		for b.Loop() {
			got, err := safefile.Read(p)
			if err != nil || len(got) != 256 {
				b.Fatalf("read failed: bytes=%d error=%v", len(got), err)
			}
		}
	})
	b.Run("one-shot", func(b *testing.B) {
		command := cCommand(helper)
		ctx := context.Background()
		got, err := read(ctx, p, true, command)
		require.NoError(b, err)
		require.Len(b, got, 256)
		b.ReportAllocs()
		b.SetBytes(256)
		for b.Loop() {
			got, err := read(ctx, p, true, command)
			if err != nil || len(got) != 256 {
				b.Fatalf("read failed: bytes=%d error=%v", len(got), err)
			}
		}
	})
}
