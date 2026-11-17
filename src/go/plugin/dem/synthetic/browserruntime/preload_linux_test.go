// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"context"
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
	_, _, err = ReadPreload(s.path, nil)
	require.Error(t, err)
	candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
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
	paths, actual, err := ReadPreload(s.path, nil)
	require.NoError(t, err)
	require.Equal(t, m, actual)
	require.Equal(t, filepath.Join(s.path, "generations", m.Archive.SHA256, m.Layout.EntryPoint), paths.EntryPoint)
	after, err := os.ReadFile(filepath.Join(s.path, "generations", m.Archive.SHA256, "prepared.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
	alias := filepath.Join(t.TempDir(), "preload-link")
	require.NoError(t, os.Symlink(s.path, alias))
	aliasPaths, _, err := ReadPreload(alias, nil)
	require.NoError(t, err)
	require.Equal(t, paths, aliasPaths, "an alias must validate against the sealed final prefix")
}

func TestPreloadCannotMoveOrSealWithUnresolvedExecution(t *testing.T) {
	s, _ := testStore(t)
	m := importFixture(t, s, "preloaded")
	candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
	require.NoError(t, err)
	use, err := candidate.BeginUse()
	require.NoError(t, err)
	require.ErrorIs(t, candidate.SealPreload(), ErrInUse)
	require.NoError(t, use.Finish(true))
	require.NoError(t, candidate.SealPreload())
	require.NoError(t, candidate.Close())
	moved := s.path + "-moved"
	require.NoError(t, os.Rename(s.path, moved))
	_, _, err = ReadPreload(moved, nil)
	require.ErrorContains(t, err, "final path")
}

func TestReadPreloadRejectsDamagedBoundPayload(t *testing.T) {
	for _, fault := range []string{"missing-node", "not-executable", "escaped-loader", launcherName, fontConfigName, "node interpreter", "browser interpreter"} {
		t.Run(fault, func(t *testing.T) {
			s, _ := testStore(t)
			m := importFixture(t, s, "preloaded")
			candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
			require.NoError(t, err)
			require.NoError(t, candidate.SealPreload())
			require.NoError(t, candidate.Close())
			root := filepath.Join(s.path, "generations", m.Archive.SHA256)
			node := filepath.Join(root, m.Layout.Node)
			switch fault {
			case "missing-node":
				require.NoError(t, os.Remove(node))
			case "not-executable":
				require.NoError(t, os.Chmod(node, 0600))
			case "escaped-loader":
				require.NoError(t, os.Remove(filepath.Join(root, m.Layout.Loader)))
				require.NoError(t, os.Symlink("/outside-preload", filepath.Join(root, m.Layout.Loader)))
			default:
				payload, err := os.OpenRoot(root)
				require.NoError(t, err)
				corruptBoundPayload(t, payload, m, fault)
				require.NoError(t, payload.Close())
			}
			_, _, err = ReadPreload(s.path, nil)
			require.ErrorIs(t, err, errPreloadDamaged, "metadata alone does not establish a usable sealed payload")
			require.NotErrorIs(t, err, ErrDamaged, "managed repair refuses sealed preloads")
		})
	}
}
