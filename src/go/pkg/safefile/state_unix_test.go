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
