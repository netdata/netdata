// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package safefile

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStateFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	require.NoError(t, syscall.Mkfifo(path, 0600))
	_, err := ReadNoFollow(path, 64)
	assert.ErrorIs(t, err, ErrNotRegular)
	assert.ErrorIs(t, Replace(path, []byte("new"), Mode{
		Perm: 0600,
	}), ErrNotRegular)
	// Simulate a FIFO appearing after preflight: the raw open must not wait.
	file, err := openNoFollow(path)
	require.NoError(t, err)
	defer file.Close()
	info, err := file.Stat()
	require.NoError(t, err)
	assert.False(t, info.Mode().IsRegular())
}

func TestReplaceModeHonorsUmaskUnlessExact(t *testing.T) {
	// No parallel tests: umask belongs to the process.
	old := syscall.Umask(0077)
	defer syscall.Umask(old)
	path := filepath.Join(t.TempDir(), "state")
	require.NoError(t, Replace(path, []byte("state"), Mode{
		Perm: 0640,
	}))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	require.NoError(t, Replace(path, []byte("state"), Mode{
		Perm:  0640,
		Exact: true,
	}))
	info, err = os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0640), info.Mode().Perm())
}

func TestEnsureDirExactRepairsPermissions(t *testing.T) {
	for _, mode := range []os.FileMode{0300, 0000} {
		t.Run(mode.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state")
			require.NoError(t, os.Mkdir(path, 0700))
			require.NoError(t, os.Chmod(path, mode))
			t.Cleanup(func() { _ = os.Chmod(path, 0700) })
			require.NoError(t, EnsureDir(path, Mode{
				Perm:  0700,
				Exact: true,
			}))
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
		})
	}
}

func TestEnsureDirExactOverridesUmask(t *testing.T) {
	parent := t.TempDir()
	old := syscall.Umask(0400)
	defer syscall.Umask(old)
	path := filepath.Join(parent, "state")
	require.NoError(t, EnsureDir(path, Mode{
		Perm:  0700,
		Exact: true,
	}))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
}

func TestReadNoFollowNeedsOnlyParentSearchPermission(t *testing.T) {
	for _, mode := range []os.FileMode{0100, 0300} {
		t.Run(mode.String(), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "state")
			require.NoError(t, os.WriteFile(path, []byte("owned state"), 0600))
			require.NoError(t, os.Chmod(dir, mode))
			t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0700)) })
			// A named-file read and absence check need search, not directory enumeration.
			data, err := ReadNoFollow(path, 64)
			require.NoError(t, err)
			assert.Equal(t, "owned state", string(data))
			_, err = ReadNoFollow(path+".missing", 64)
			assert.ErrorIs(t, err, os.ErrNotExist)
			info, err := os.Stat(dir)
			require.NoError(t, err)
			assert.Equal(t, mode, info.Mode().Perm(), "reading must not repair permissions")
		})
	}
}

func TestReplaceNeedsOnlyParentWriteAndSearchPermission(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprint(existing), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "state")
			if existing {
				require.NoError(t, os.WriteFile(path, []byte("old"), 0600))
			}
			require.NoError(t, os.Chmod(dir, 0300))
			t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0700)) })
			require.NoError(t, Replace(path, []byte("new"), Mode{
				Perm: 0600,
			}))
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, "new", string(data))
			info, err := os.Stat(dir)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0300), info.Mode().Perm())
			// Directory durability remains a separate operation requiring a directory handle.
			require.NoError(t, os.Chmod(dir, 0700))
			require.NoError(t, SyncDir(dir))
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Len(t, entries, 1, "replacement must clean up its temporary file")
		})
	}
}
