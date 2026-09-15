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

func cReader(t testing.TB, path string) *Reader {
	t.Helper()
	r := newReader(func() *exec.Cmd { return exec.Command(path, "--read-file-server-v1") })
	t.Cleanup(func() {
		require.NoError(t, r.Close())
		r.mu.Lock()
		s := r.session
		r.mu.Unlock()
		if s != nil {
			select {
			case <-s.done:
			case <-time.After(5 * time.Second):
				t.Error("synthetic C helper did not exit after Close")
			}
		}
	})
	return r
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
	r := cReader(t, helper)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p := filepath.Join(dir, "credential")
	first := []byte("synthetic-initial-value")
	cWrite(t, p, first, 0644)
	got, err := r.Read(ctx, p)
	require.NoError(t, err)
	require.Equal(t, first, got)
	initial := r.session
	// Rotation replaces the inode, as projected secrets and atomic writers do.
	replacement := filepath.Join(dir, "replacement")
	rotated := []byte("synthetic-rotated-value")
	cWrite(t, replacement, rotated, 0644)
	require.NoError(t, os.Rename(replacement, p))
	got, err = r.Read(ctx, p)
	require.NoError(t, err)
	require.Equal(t, rotated, got)
	require.Same(t, initial, r.session)
	link := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(p, link))
	got, err = r.Read(ctx, link)
	require.NoError(t, err)
	require.Equal(t, rotated, got)
	info, err := os.Stat(p)
	require.NoError(t, err)
	mod, err := r.Stat(ctx, link)
	require.NoError(t, err)
	require.True(t, info.ModTime().Equal(mod))
	_, err = r.Read(ctx, filepath.Join(dir, "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = r.Stat(ctx, filepath.Join(dir, "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = r.Read(ctx, "")
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = r.Read(ctx, dir)
	require.ErrorIs(t, err, safefile.ErrNotRegular)
	fifo := filepath.Join(dir, "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0644))
	_, err = r.Read(ctx, fifo)
	require.ErrorIs(t, err, safefile.ErrNotRegular)
	large := bytes.Repeat([]byte("x"), int(safefile.MaxSize)+1)
	cWrite(t, p, large, 0644)
	got, err = r.Read(ctx, p)
	require.Nil(t, got)
	require.ErrorIs(t, err, safefile.ErrTooLarge)
	got, err = r.ReadAll(ctx, p)
	require.NoError(t, err)
	require.Equal(t, large, got)
	stream, err := r.Open(ctx, p)
	require.NoError(t, err)
	defer stream.Close()
	got, err = io.ReadAll(stream)
	require.NoError(t, err)
	require.Equal(t, large, got)
	require.NoError(t, stream.Close())
	require.Same(t, initial, r.session)
}

func TestCReaderPermissionDenial(t *testing.T) {
	helper := cHelperPath(t)
	dir := cFixtureDir(t)
	r := cReader(t, helper)
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
	got, err := r.Read(ctx, p)
	require.Nil(t, got)
	require.ErrorIs(t, err, os.ErrPermission)
	require.ErrorIs(t, err, safefile.ErrFile)
	require.NotContains(t, err.Error(), string(private))
}

func TestCReaderBlockedFIFOCancellation(t *testing.T) {
	helper := cHelperPath(t)
	dir := cFixtureDir(t)
	r := cReader(t, helper)
	fifo := filepath.Join(dir, "fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0644))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	stream, err := r.Open(ctx, fifo)
	require.NoError(t, err)
	_, err = io.ReadAll(stream)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, stream.Close())
	p := filepath.Join(dir, "readable")
	cWrite(t, p, []byte("after-cancellation"), 0644)
	nextCtx, nextCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer nextCancel()
	got, err := r.Read(nextCtx, p)
	require.NoError(t, err)
	require.Equal(t, "after-cancellation", string(got))
	// A second blocked stream verifies that Close interrupts actual C open(2).
	stream, err = r.Open(nextCtx, fifo)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(stream); done <- err }()
	require.NoError(t, r.Close())
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("Close did not interrupt FIFO read")
	}
	require.NoError(t, stream.Close())
}

// This is a machine-local cost comparison, not a timing CI gate. Both paths read
// a synthetic 256-byte regular file without caching. Persistent setup is outside
// the measured loop; each read remains O(path bytes + file bytes).
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
	b.Run("persistent", func(b *testing.B) {
		r := cReader(b, helper)
		ctx := context.Background()
		got, err := r.Read(ctx, p)
		require.NoError(b, err)
		require.Len(b, got, 256)
		b.ReportAllocs()
		b.SetBytes(256)
		for b.Loop() {
			got, err := r.Read(ctx, p)
			if err != nil || len(got) != 256 {
				b.Fatalf("read failed: bytes=%d error=%v", len(got), err)
			}
		}
	})
}
