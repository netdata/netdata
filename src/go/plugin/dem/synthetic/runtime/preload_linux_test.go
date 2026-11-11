// SPDX-License-Identifier: GPL-3.0-or-later
package runtime

import (
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestPreloadedIdentityReadOnlyAndManagedExclusion(t *testing.T) {
	s, _ := testStore(t)
	m := importFixture(t, s, "preloaded")
	other, err := Open(s.config)
	require.NoError(t, err)
	defer other.Close()
	_, _, err = ReadPreload(s.path)
	require.Error(t, err)
	candidate, err := s.BeginInstall(context.Background(), m)
	require.NoError(t, err)
	require.NoError(t, candidate.SealPreload())
	require.NoError(t, candidate.Close())
	require.NoError(t, candidate.Close())
	require.ErrorContains(t, other.Sweep(), "protected preload")
	reopened, err := Open(s.config)
	require.Nil(t, reopened)
	require.ErrorContains(t, err, "protected preload")
	before, err := os.ReadFile(filepath.Join(s.path, "generations", m.Archive.SHA256, "prepared.json"))
	require.NoError(t, err)
	require.NoError(t, os.Chmod(s.path, 0500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(s.path, 0700)) })
	paths, actual, err := ReadPreload(s.path)
	require.NoError(t, err)
	require.Equal(t, m, actual)
	require.Equal(t, filepath.Join(s.path, "generations", m.Archive.SHA256, m.Layout.EntryPoint), paths.EntryPoint)
	after, err := os.ReadFile(filepath.Join(s.path, "generations", m.Archive.SHA256, "prepared.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestPreloadCannotMoveOrSealWithUnresolvedExecution(t *testing.T) {
	s, _ := testStore(t)
	m := importFixture(t, s, "preloaded")
	candidate, err := s.BeginInstall(context.Background(), m)
	require.NoError(t, err)
	use, err := candidate.BeginUse()
	require.NoError(t, err)
	require.ErrorIs(t, candidate.SealPreload(), ErrInUse)
	require.NoError(t, use.Finish(true))
	require.NoError(t, candidate.SealPreload())
	require.NoError(t, candidate.Close())
	moved := s.path + "-moved"
	require.NoError(t, os.Rename(s.path, moved))
	_, _, err = ReadPreload(moved)
	require.ErrorContains(t, err, "final path")
}
