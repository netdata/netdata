// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package credentialfile

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
	"github.com/stretchr/testify/require"
)

// This test peer exercises the Go protocol client; it does not validate the C
// implementation or its privilege transition.
func TestProtocolPeer(t *testing.T) {
	if os.Getenv("NETDATA_CREDENTIALFILE_TEST_PEER") != "1" {
		return
	}
	defer os.Exit(0)
	hello := make([]byte, 16)
	copy(hello, "NDFILE01")
	binary.BigEndian.PutUint32(hello[8:], 4096)
	binary.BigEndian.PutUint32(hello[12:], chunkMax)
	if os.Getenv("NETDATA_CREDENTIALFILE_TEST_BAD_HELLO") == "1" {
		hello[0] = 'X'
	}
	os.Stdout.Write(hello)
	frame := func(kind, code, errno uint32, data []byte) {
		h := make([]byte, 16)
		binary.BigEndian.PutUint32(h, kind)
		binary.BigEndian.PutUint32(h[4:], uint32(len(data)))
		binary.BigEndian.PutUint32(h[8:], code)
		binary.BigEndian.PutUint32(h[12:], errno)
		os.Stdout.Write(h)
		os.Stdout.Write(data)
	}
	for {
		var h [24]byte
		if _, err := io.ReadFull(os.Stdin, h[:]); err != nil {
			return
		}
		op := binary.BigEndian.Uint32(h[:])
		flags := binary.BigEndian.Uint32(h[4:])
		limit := binary.BigEndian.Uint64(h[8:])
		n := binary.BigEndian.Uint32(h[16:])
		if n >= 4096 || binary.BigEndian.Uint32(h[20:]) != 0 {
			return
		}
		path := make([]byte, n)
		if _, err := io.ReadFull(os.Stdin, path); err != nil {
			return
		}
		switch string(path) {
		case "@block":
			var b [1]byte
			os.Stdin.Read(b[:])
			return
		case "@partial-error":
			frame(1, 0, 0, []byte("SYNTHETIC_PRIVATE_BYTES"))
			frame(2, 3, uint32(syscall.EIO), nil)
			continue
		case "@permission":
			frame(2, 1, uint32(syscall.EACCES), nil)
			continue
		case "@malformed":
			frame(1, 0, 0, make([]byte, chunkMax+1))
			return
		case "@truncated":
			os.Stdout.Write([]byte{0, 0, 0})
			return
		case "@bad-result":
			frame(2, 7, 0, nil)
			return
		case "@bad-stat":
			frame(3, 0, 0, make([]byte, 15))
			return
		case "@pid":
			frame(1, 0, 0, []byte(strings.Repeat("p", os.Getpid()%100+1)))
			frame(2, 0, 0, nil)
			continue
		}
		p := string(path)
		if op == 2 {
			info, err := os.Stat(p)
			if err != nil {
				frame(2, 2, uint32(syscall.ENOENT), nil)
				continue
			}
			data := make([]byte, 16)
			binary.BigEndian.PutUint64(data, uint64(info.ModTime().Unix()))
			binary.BigEndian.PutUint64(data[8:], uint64(info.ModTime().Nanosecond()))
			frame(3, 0, 0, data)
			continue
		}
		if op != 1 || flags > 1 || flags == 1 && limit != uint64(safefile.MaxSize) {
			return
		}
		file, err := os.Open(p)
		if err != nil {
			frame(2, 1, uint32(syscall.ENOENT), nil)
			continue
		}
		info, err := file.Stat()
		if err != nil {
			file.Close()
			frame(2, 2, uint32(syscall.EIO), nil)
			continue
		}
		if flags == 1 && !info.Mode().IsRegular() {
			file.Close()
			frame(2, 5, 0, nil)
			continue
		}
		if limit != 0 && uint64(info.Size()) > limit {
			file.Close()
			frame(2, 6, 0, nil)
			continue
		}
		buf := make([]byte, chunkMax)
		for {
			n, err := file.Read(buf)
			if n > 0 {
				frame(1, 0, 0, buf[:n])
			}
			if err != nil {
				file.Close()
				if err == io.EOF {
					frame(2, 0, 0, nil)
				} else {
					frame(2, 3, uint32(syscall.EIO), nil)
				}
				break
			}
		}
	}
}

func testReader(t *testing.T) *Reader {
	t.Helper()
	r := newReader(func() *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestProtocolPeer$")
		cmd.Env = append(os.Environ(), "NETDATA_CREDENTIALFILE_TEST_PEER=1")
		return cmd
	})
	t.Cleanup(func() {
		r.Close()
		r.mu.Lock()
		s := r.session
		r.mu.Unlock()
		if s != nil {
			select {
			case <-s.done:
			case <-time.After(5 * time.Second):
				t.Error("test peer was not reaped")
			}
		}
	})
	return r
}
func testFile(t *testing.T, contents string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "credential")
	require.NoError(t, os.WriteFile(p, []byte(contents), 0600))
	return p
}
func TestReaderFreshnessAndPolicies(t *testing.T) {
	r := testReader(t)
	ctx := context.Background()
	p := testFile(t, "first")
	got, err := r.Read(ctx, p)
	require.NoError(t, err)
	require.Equal(t, "first", string(got))
	s := r.session
	require.NoError(t, os.WriteFile(p, []byte("rotated"), 0600))
	got, err = r.Read(ctx, p)
	require.NoError(t, err)
	require.Equal(t, "rotated", string(got))
	require.Same(t, s, r.session)
	info, err := os.Stat(p)
	require.NoError(t, err)
	mt, err := r.Stat(ctx, p)
	require.NoError(t, err)
	require.True(t, mt.Equal(info.ModTime()))
	large := strings.Repeat("x", int(safefile.MaxSize)+1)
	require.NoError(t, os.WriteFile(p, []byte(large), 0600))
	got, err = r.Read(ctx, p)
	require.Nil(t, got)
	require.ErrorIs(t, err, safefile.ErrTooLarge)
	got, err = r.ReadAll(ctx, p)
	require.NoError(t, err)
	require.Equal(t, large, string(got))
	stream, err := r.Open(ctx, p)
	require.NoError(t, err)
	got, err = io.ReadAll(stream)
	require.NoError(t, err)
	require.NoError(t, stream.Close())
	require.Equal(t, large, string(got))
	require.Same(t, s, r.session)
	// Closing a completed stream after reuse must not retire that active session.
	next, err := r.Open(ctx, p)
	require.NoError(t, err)
	require.NoError(t, stream.Close())
	_, err = io.ReadAll(next)
	require.NoError(t, err)
	require.NoError(t, next.Close())
}
func TestReaderFileErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		path string
		want error
	}{
		"missing": {filepath.Join(t.TempDir(), "missing"), os.ErrNotExist}, "empty": {"", os.ErrNotExist},
		"directory": {t.TempDir(), safefile.ErrNotRegular}, "permission": {"@permission", os.ErrPermission},
		"read failure": {"@partial-error", syscall.EIO}, "nul": {"bad\x00path", syscall.EINVAL}, "long": {strings.Repeat("x", 4096), syscall.ENAMETOOLONG},
	} {
		t.Run(name, func(t *testing.T) {
			r := testReader(t)
			got, err := r.Read(context.Background(), tc.path)
			require.Nil(t, got)
			require.ErrorIs(t, err, safefile.ErrFile)
			require.ErrorIs(t, err, tc.want)
			require.NotContains(t, err.Error(), "SYNTHETIC_PRIVATE_BYTES")
		})
	}
}
func TestReaderTransportFailures(t *testing.T) {
	for _, path := range []string{"@malformed", "@truncated", "@bad-result", "@bad-stat"} {
		t.Run(path, func(t *testing.T) {
			r := testReader(t)
			got, err := r.Read(context.Background(), path)
			require.Nil(t, got)
			require.ErrorIs(t, err, safefile.ErrFile)
			require.False(t, errors.Is(err, os.ErrNotExist))
			p := testFile(t, "recovered")
			got, err = r.Read(context.Background(), p)
			require.NoError(t, err)
			require.Equal(t, "recovered", string(got))
		})
	}
	r := newReader(func() *exec.Cmd { return exec.Command(filepath.Join(t.TempDir(), "missing-helper")) })
	defer r.Close()
	_, err := r.Read(context.Background(), "missing-file")
	require.ErrorIs(t, err, safefile.ErrFile)
	require.NotErrorIs(t, err, os.ErrNotExist)
	r2 := newReader(func() *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=^TestProtocolPeer$")
		cmd.Env = append(os.Environ(), "NETDATA_CREDENTIALFILE_TEST_PEER=1", "NETDATA_CREDENTIALFILE_TEST_BAD_HELLO=1")
		return cmd
	})
	defer r2.Close()
	_, err = r2.Read(context.Background(), "anything")
	require.ErrorIs(t, err, safefile.ErrFile)
}
func TestReaderCancellationAndClose(t *testing.T) {
	r := testReader(t)
	ctx, cancel := context.WithCancel(context.Background())
	active, err := r.Open(ctx, "@block")
	require.NoError(t, err)
	waiting, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	_, err = r.Read(waiting, "unused")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(active); done <- err }()
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("active cancellation blocked")
	}
	require.NoError(t, active.Close())
	old := r.session
	p := testFile(t, "replacement")
	got, err := r.Read(context.Background(), p)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(got))
	require.NotSame(t, old, r.session)
	// A late cancellation/close on the old request cannot kill its replacement.
	cancel()
	require.NoError(t, active.Close())
	got, err = r.Read(context.Background(), p)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(got))
	stream, err := r.Open(context.Background(), "@block")
	require.NoError(t, err)
	go func() { _, err := io.ReadAll(stream); done <- err }()
	require.NoError(t, r.Close())
	require.NoError(t, r.Close())
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("reader close blocked")
	}
	require.NoError(t, stream.Close())
	_, err = r.Read(context.Background(), p)
	require.Error(t, err)
}
func TestReaderEarlyStreamClose(t *testing.T) {
	r := testReader(t)
	s, err := r.Open(context.Background(), "@block")
	require.NoError(t, err)
	require.NoError(t, s.Close())
	p := testFile(t, "next")
	got, err := r.Read(context.Background(), p)
	require.NoError(t, err)
	require.Equal(t, "next", string(got))
}
func TestReaderSessionOutlivesFirstContext(t *testing.T) {
	r := testReader(t)
	p := testFile(t, "fresh")
	ctx, cancel := context.WithCancel(context.Background())
	_, err := r.Read(ctx, p)
	require.NoError(t, err)
	s := r.session
	cancel()
	_, err = r.Read(context.Background(), p)
	require.NoError(t, err)
	require.Same(t, s, r.session)
}

func TestReaderConcurrentRequests(t *testing.T) {
	r := testReader(t)
	p := testFile(t, strings.Repeat("value", 20000))
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				got, err := r.Read(context.Background(), p)
				if err != nil {
					errs <- err
					return
				}
				if len(got) != 100000 {
					errs <- errors.New("interleaved response")
					return
				}
			}
			errs <- nil
		}()
	}
	for i := 0; i < 12; i++ {
		require.NoError(t, <-errs)
	}
}

func TestReaderStatFailure(t *testing.T) {
	r := testReader(t)
	_, err := r.Stat(context.Background(), filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = r.Stat(context.Background(), "@bad-stat")
	require.ErrorIs(t, err, safefile.ErrFile)
	_, err = r.Stat(context.Background(), "@pid")
	require.ErrorIs(t, err, safefile.ErrFile)
}
