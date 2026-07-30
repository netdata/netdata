// SPDX-License-Identifier: GPL-3.0-or-later

package safefile

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
		require.Equal(t, int(MaxSize)+bytes.MinRead, cap(got))
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

func TestReadBoundedUnderreportedSize(t *testing.T) {
	t.Run("grows within the bound", func(t *testing.T) {
		want := strings.Repeat("x", 2048)

		got, err := readBounded(strings.NewReader(want), 0)

		require.NoError(t, err)
		require.Equal(t, want, string(got))
	})

	t.Run("rejects growth over the bound", func(t *testing.T) {
		_, err := readBounded(strings.NewReader(strings.Repeat("x", int(MaxSize+1))), 0)

		require.ErrorIs(t, err, ErrTooLarge)
	})
}
