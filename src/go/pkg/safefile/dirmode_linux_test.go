// SPDX-License-Identifier: GPL-3.0-or-later

package safefile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestDirectoryModeProcFallback(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "state")
	require.NoError(t, os.Mkdir(path, 0700))
	require.NoError(t, os.Chmod(path, 0000))
	t.Cleanup(func() { _ = os.Chmod(path, 0700) })
	fd, err := unix.Open(path, unix.O_PATH|unix.O_NOFOLLOW|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	require.NoError(t, err)
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	// Even after a rename, fallback changes only the held directory.
	moved := path + ".moved"
	require.NoError(t, os.Rename(path, moved))
	t.Cleanup(func() { _ = os.Chmod(moved, 0700) })
	require.NoError(t, os.Mkdir(path, 0755))
	require.NoError(t, os.Chmod(path, 0755))
	require.NoError(t, chmodDirectoryViaProc(file, 0700))
	info, err := os.Stat(moved)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0700), info.Mode().Perm())
	info, err = os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestDirectoryModeRefusesSymlink(t *testing.T) {
	parent, err := os.Open(t.TempDir())
	require.NoError(t, err)
	defer parent.Close()
	target := t.TempDir()
	require.NoError(t, os.Chmod(target, 0755))
	require.NoError(t, os.Symlink(target, filepath.Join(parent.Name(), "link")))
	assert.Error(t, chmodDirectoryAt(int(parent.Fd()), "link", 0700))
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}
