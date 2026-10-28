// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package safefile

import (
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
