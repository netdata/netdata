// SPDX-License-Identifier: GPL-3.0-or-later

package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRead(t *testing.T) {
	t.Run("reads a regular file at the limit", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "value")
		want := make([]byte, MaxSize)
		require.NoError(t, os.WriteFile(path, want, 0o600))

		got, err := Read(path)

		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("rejects a regular file over the limit", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "value")
		require.NoError(t, os.WriteFile(path, make([]byte, MaxSize+1), 0o600))

		_, err := Read(path)

		require.ErrorIs(t, err, ErrFile)
		require.ErrorIs(t, err, ErrTooLarge)
	})

	t.Run("rejects a non-regular opened object", func(t *testing.T) {
		path := t.TempDir()

		_, err := Read(path)

		require.ErrorIs(t, err, ErrFile)
		require.ErrorIs(t, err, ErrNotRegular)
	})

	t.Run("follows a symlink to a regular file", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "target")
		link := filepath.Join(dir, "link")
		require.NoError(t, os.WriteFile(target, []byte("value"), 0o600))
		if err := os.Symlink(target, link); err != nil {
			t.Skipf("symlinks are unavailable: %v", err)
		}

		got, err := Read(link)

		require.NoError(t, err)
		require.Equal(t, []byte("value"), got)
	})

	t.Run("preserves the private filesystem cause", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing")

		_, err := Read(path)

		require.ErrorIs(t, err, ErrFile)
		require.True(t, errors.Is(err, os.ErrNotExist))
		require.Contains(t, err.Error(), path)
	})
}
