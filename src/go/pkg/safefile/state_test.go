// SPDX-License-Identifier: GPL-3.0-or-later

package safefile

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenNoFollowDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, []byte("synthetic contents"), 0600))
	path := filepath.Join(dir, "state")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// Exercise the open separately: an Lstat check alone cannot protect a raced-in link.
	f, err := openNoFollow(path)
	if err == nil {
		defer f.Close()
		info, err := f.Stat()
		require.NoError(t, err)
		assert.False(t, info.Mode().IsRegular(), "must open the link itself, never its regular-file target")
	}
}

func TestReadNoFollowBoundsAndTypes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	for _, n := range []int{0, 64, 65} {
		data := make([]byte, n)
		require.NoError(t, os.WriteFile(path, data, 0600))
		got, err := ReadNoFollow(path, 64)
		if n > 64 {
			assert.ErrorIs(t, err, ErrTooLarge)
		} else {
			require.NoError(t, err)
			assert.Equal(t, data, got)
		}
	}
	for _, limit := range []int64{-1, math.MaxInt64} {
		_, err := ReadNoFollow(path, limit)
		assert.ErrorIs(t, err, os.ErrInvalid)
	}
	for _, special := range []string{t.TempDir(), os.DevNull} {
		_, err := ReadNoFollow(special, 64)
		assert.ErrorIs(t, err, ErrNotRegular)
	}
	_, err := ReadNoFollow(path+"missing", 64)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestStateSymlinkLeaves(t *testing.T) {
	for _, dangling := range []bool{false, true} {
		t.Run(fmt.Sprint(dangling), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state")
			target := filepath.Join(t.TempDir(), "target")
			if !dangling {
				require.NoError(t, os.WriteFile(target, []byte("sentinel"), 0600))
			}
			if err := os.Symlink(target, path); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			_, err := ReadNoFollow(path, 64)
			assert.ErrorIs(t, err, ErrNotRegular)
			assert.ErrorIs(t, Replace(path, []byte("new"), Mode{
				Perm: 0600,
			}), ErrNotRegular)
			link, err := os.Readlink(path)
			require.NoError(t, err)
			assert.Equal(t, target, link)
			if dangling {
				_, err = os.Lstat(target)
				assert.ErrorIs(t, err, os.ErrNotExist)
			} else {
				data, err := os.ReadFile(target)
				require.NoError(t, err)
				assert.Equal(t, "sentinel", string(data))
			}
		})
	}
}

func TestReplacePreservesOrphansAndCleansFailedTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	for _, name := range []string{"state.tmp", "state.orphan.tmp", "state"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("old"), 0600))
	}
	failed := errors.New("rename failed")
	err := replace(path, []byte("new"), Mode{
		Perm:  0600,
		Exact: true,
	}, func(old, new string) error {
		data, err := os.ReadFile(old)
		require.NoError(t, err)
		assert.Equal(t, "new", string(data))
		assert.Equal(t, path, new)
		return failed
	})
	require.ErrorIs(t, err, failed)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "old", string(data))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
	require.NoError(t, Replace(path, []byte("new"), Mode{
		Perm:  0600,
		Exact: true,
	}))
	require.NoError(t, SyncDir(dir))
	got, err := ReadNoFollow(path, 64)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
	entries, err = os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
	for _, name := range []string{"state.tmp", "state.orphan.tmp"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.Equal(t, "old", string(data))
	}
}

func TestStateDirectorySafety(t *testing.T) {
	target := t.TempDir()
	require.NoError(t, os.Chmod(target, 0755))
	before, err := os.Stat(target)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "dir")
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	assert.Error(t, EnsureDir(path, Mode{
		Perm:  0700,
		Exact: true,
	}))
	assert.Error(t, SyncDir(path))
	_, err = ReadNoFollow(filepath.Join(path, "missing"), 64)
	require.Error(t, err)
	assert.NotErrorIs(t, err, os.ErrNotExist)
	assert.Error(t, Replace(filepath.Join(path, "missing"), []byte("new"), Mode{
		Perm: 0600,
	}))
	after, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, before.Mode(), after.Mode())
	entries, err := os.ReadDir(target)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestEnsureDirModePolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir")
	require.NoError(t, EnsureDir(path, Mode{
		Perm: 0750,
	}))
	require.NoError(t, os.Chmod(path, 0755))
	before, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, EnsureDir(path, Mode{
		Perm: 0750,
	}))
	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, before.Mode(), after.Mode())
	require.NoError(t, EnsureDir(path, Mode{
		Perm:  0700,
		Exact: true,
	}))
	after, err = os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0700), after.Mode().Perm())
	}
}
