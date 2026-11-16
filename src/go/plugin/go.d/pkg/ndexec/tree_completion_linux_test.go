// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestTreeCompletionActualSupervisor(t *testing.T) {
	helper := compileTreeHelper(t)
	for _, command := range []string{"exit 0", "exit 23"} {
		t.Run(command, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "execution")
			completion, err := CreateTreeCompletion(path)
			require.NoError(t, err)
			defer completion.Close()
			done, err := ReadTreeCompletion(path, completion.ID())
			require.NoError(t, err)
			require.False(t, done)
			process, err := startProcessTree(context.Background(), helper, TreeOptions{Completion: completion}, "/bin/sh", "-c", command)
			require.NoError(t, err)
			result := process.Wait()
			require.True(t, result.Drained, "%v", result.Err)
			if command == "exit 0" {
				require.NoError(t, result.Err)
			} else {
				require.Error(t, result.Err)
			}
			done, err = ReadTreeCompletion(path, completion.ID())
			require.NoError(t, err)
			require.True(t, done)
			// Same directory/token must never authorize another invocation.
			repeated, err := startProcessTree(context.Background(), helper, TreeOptions{Completion: completion}, "/bin/true")
			require.NoError(t, err)
			require.False(t, repeated.Wait().Drained)
		})
	}
}

// Model a completion reader paused inside its publication-lock critical section.
// Startup must wait, while the invocation claim still excludes a second helper.
func TestTreeCompletionReaderCannotRejectLaunch(t *testing.T) {
	helper := compileTreeHelper(t)
	path := filepath.Join(t.TempDir(), "execution")
	completion, err := CreateTreeCompletion(path)
	require.NoError(t, err)
	defer completion.Close()
	lock, err := os.OpenFile(filepath.Join(path, "publication.lock"), os.O_RDWR, 0)
	require.NoError(t, err)
	defer lock.Close()
	require.NoError(t, unix.Flock(int(lock.Fd()), unix.LOCK_EX))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	process, err := startProcessTree(ctx, helper, TreeOptions{Completion: completion}, "/bin/true")
	require.NoError(t, err)
	done := make(chan TreeResult, 1)
	go func() { done <- process.Wait() }()
	waited := false
	defer func() {
		_ = unix.Flock(int(lock.Fd()), unix.LOCK_UN)
		cancel()
		if !waited {
			<-done
		}
	}()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(filepath.Join(path, "started")); err == nil {
			break
		}
		select {
		case result := <-done:
			waited = true
			t.Fatalf("reader rejected launch: %+v", result)
		case <-ctx.Done():
			t.Fatal("helper did not claim invocation while reader held publication lock")
		case <-ticker.C:
		}
	}
	repeated, err := startProcessTree(ctx, helper, TreeOptions{Completion: completion}, "/bin/true")
	require.NoError(t, err)
	duplicateDone := make(chan TreeResult, 1)
	go func() { duplicateDone <- repeated.Wait() }()
	select {
	case result := <-duplicateDone:
		require.False(t, result.Drained, "duplicate invocation must refuse while first waits")
	case <-ctx.Done():
		_ = unix.Flock(int(lock.Fd()), unix.LOCK_UN)
		<-duplicateDone
		t.Fatal("duplicate invocation waited for publication lock")
	}
	require.NoError(t, unix.Flock(int(lock.Fd()), unix.LOCK_UN))
	result := <-done
	waited = true
	require.True(t, result.Drained, "%v", result.Err)
	require.NoError(t, result.Err)
	complete, err := ReadTreeCompletion(path, completion.ID())
	require.NoError(t, err)
	require.True(t, complete)
}

func TestTreeCompletionRecoveryRejectsUncertainRecords(t *testing.T) {
	helper := compileTreeHelper(t)
	for _, kind := range []string{"wrong-id", "missing-pin", "partial-pin", "symlink-pin", "partial-receipt", "symlink-receipt", "directory-receipt", "fifo-pin", "fifo-receipt"} {
		t.Run(kind, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "execution")
			c, err := CreateTreeCompletion(path)
			require.NoError(t, err)
			defer c.Close()
			p, err := startProcessTree(context.Background(), helper, TreeOptions{Completion: c}, "/bin/true")
			require.NoError(t, err)
			require.True(t, p.Wait().Drained)
			id := c.ID()
			switch kind {
			case "wrong-id":
				id = strings.Repeat("f", 64)
			case "missing-pin":
				require.NoError(t, os.Remove(filepath.Join(path, "pending")))
			case "partial-pin":
				require.NoError(t, os.WriteFile(filepath.Join(path, "pending"), []byte("NDTREE1"), 0600))
			case "partial-receipt":
				require.NoError(t, os.WriteFile(filepath.Join(path, "complete"), []byte("NDTREE1"), 0600))
			case "symlink-pin", "symlink-receipt":
				name := "pending"
				if kind == "symlink-receipt" {
					name = "complete"
				}
				require.NoError(t, os.Rename(filepath.Join(path, name), filepath.Join(path, "other")))
				require.NoError(t, os.Symlink("other", filepath.Join(path, name)))
			case "fifo-pin", "fifo-receipt":
				name := "pending"
				if kind == "fifo-receipt" {
					name = "complete"
				}
				require.NoError(t, os.Remove(filepath.Join(path, name)))
				require.NoError(t, unix.Mkfifo(filepath.Join(path, name), 0600))
			case "directory-receipt":
				require.NoError(t, os.Remove(filepath.Join(path, "complete")))
				require.NoError(t, os.Mkdir(filepath.Join(path, "complete"), 0700))
			}
			done, err := ReadTreeCompletion(path, id)
			assert.Error(t, err)
			assert.False(t, done)
		})
	}
}

func TestTreeCompletionFreshDirectoryAndPrelaunch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "execution")
	c, err := CreateTreeCompletion(path)
	require.NoError(t, err)
	defer c.Close()
	_, err = CreateTreeCompletion(path)
	require.ErrorIs(t, err, os.ErrExist)
	// A caller crash before launch supplies no evidence of completion.
	done, err := ReadTreeCompletion(path, c.ID())
	require.NoError(t, err)
	require.False(t, done)
	_, err = ReadTreeCompletion(path, "bad")
	require.Error(t, err)
}

// Includes real helper launch/drain/join, and for durable runs creation,
// persistence, recovery verification and record removal. Timing is a local
// filesystem trend, not a CI threshold or a host-wide disk performance claim.
func BenchmarkProcessTreeCompletion(b *testing.B) {
	helper := compileTreeHelper(b)
	for _, durable := range []bool{false, true} {
		name := "plain"
		if durable {
			name = "durable"
		}
		b.Run(name, func(b *testing.B) {
			dir := b.TempDir()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				opts := TreeOptions{}
				path := filepath.Join(dir, strconv.Itoa(i))
				if durable {
					c, err := CreateTreeCompletion(path)
					if err != nil {
						b.Fatal(err)
					}
					opts.Completion = c
				}
				p, err := startProcessTree(context.Background(), helper, opts, "/bin/true")
				if err != nil {
					b.Fatal(err)
				}
				result := p.Wait()
				if !result.Drained || result.Err != nil {
					b.Fatalf("supervised run: %+v", result)
				}
				if durable {
					c := opts.Completion
					if err := c.Close(); err != nil {
						b.Fatal(err)
					}
					done, err := ReadTreeCompletion(path, c.ID())
					if err != nil || !done {
						b.Fatalf("persistent completion: %v %v", done, err)
					}
					if err := os.RemoveAll(path); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
