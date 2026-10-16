// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreloadedIdentityReadOnlyAndManagedExclusion(t *testing.T) {
	s, _ := testStore(t)
	m := importFixture(t, s, "preloaded")
	other, err := Open(s.config)
	require.NoError(t, err)
	defer other.Close()
	_, err = ReadPreload(s.path, m)
	require.Error(t, err, "an ordinary cache is not a protected preload")
	require.NoError(t, s.SealPreload(m))
	require.ErrorContains(t, other.Sweep(), "protected preload")
	reopened, err := Open(s.config)
	require.Nil(t, reopened)
	require.ErrorContains(t, err, "protected preload")
	before, err := os.ReadFile(filepath.Join(s.path, "generations", m.Archive.SHA256, "prepared.json"))
	require.NoError(t, err)
	require.NoError(t, os.Chmod(s.path, 0500))
	t.Cleanup(func() { require.NoError(t, os.Chmod(s.path, 0700)) })
	paths, err := ReadPreload(s.path, m)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(s.path, "generations", m.Archive.SHA256, m.Layout.Node), paths.Node)
	mirror := m
	mirror.Archive.URL = "https://mirror.invalid/runtime.tar.gz"
	_, err = ReadPreload(s.path, mirror)
	require.NoError(t, err)
	changed := m
	changed.Version = "another-required-build"
	_, err = ReadPreload(s.path, changed)
	require.ErrorContains(t, err, "differs from installed requirement")
	after, err := os.ReadFile(filepath.Join(s.path, "generations", m.Archive.SHA256, "prepared.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestPreloadCannotMoveOrSelectAnotherIdentity(t *testing.T) {
	s, _ := testStore(t)
	m := importFixture(t, s, "preloaded")
	require.NoError(t, s.SealPreload(m))
	wrong, _ := bundleFixture(t, "other")
	_, err := ReadPreload(s.path, wrong)
	require.ErrorContains(t, err, "identity or final path")
	moved := s.path + "-moved"
	require.NoError(t, os.Rename(s.path, moved))
	_, err = ReadPreload(moved, m)
	require.ErrorContains(t, err, "identity or final path")
}
