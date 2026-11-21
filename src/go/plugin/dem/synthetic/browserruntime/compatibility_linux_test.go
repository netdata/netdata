// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Persist the records a different-contract Agent would have written, keeping
// the selection and prepared payload identity consistent. No ownership proof
// is changed. Cross-binary delivery tests exercise the actual previous writer.
func setPersistedContract(t *testing.T, s *Store, m Manifest, contract int) Manifest {
	t.Helper()
	g, err := loadGeneration(s.root, m.Archive.SHA256)
	require.NoError(t, err)
	g.Manifest.Contract = contract
	require.NoError(t, saveJSON(s.root, generationRecord(m.Archive.SHA256), g))
	return g.Manifest
}

func TestHealthyIncompatibleSelectionRemainsManageable(t *testing.T) {
	s, _ := testStore(t)
	m := installFixture(t, s, "previous-contract")
	record, err := s.readSelectionRecord()
	require.NoError(t, err)
	m = setPersistedContract(t, s, m, 1)
	record.Release.Manifest = m
	require.NoError(t, saveJSON(s.root, "selection.json", record))
	require.NoError(t, s.Close())

	reopened, err := Open(s.config)
	require.NoError(t, err)
	defer reopened.Close()
	status, err := reopened.Inspect()
	require.NoError(t, err)
	require.NotNil(t, status.Release, "healthy incompatible selection is still an installed release")
	require.Equal(t, m, status.Release.Manifest)
	require.Empty(t, status.SelectionError)
	require.NoError(t, reopened.Sweep(), "compatibility must not obstruct ownership and retention")
	require.FileExists(t, filepath.Join(s.path, generationRecord(m.Archive.SHA256)))

	consumer, err := reopened.AcquireSelected()
	require.Nil(t, consumer)
	require.ErrorContains(t, err, "unsupported runtime execution contract 1")
	require.ErrorIs(t, err, ErrIncompatible)
	require.NotErrorIs(t, err, ErrDamaged)

	// Ordinary replacement can inspect the old selection; repair is not needed.
	next := installFixture(t, reopened, "current-contract")
	consumer, err = reopened.AcquireSelected()
	require.NoError(t, err)
	require.Equal(t, next, consumer.Manifest())
	require.NoError(t, consumer.Close())
	require.NoError(t, reopened.Uninstall(context.Background()))
}

func TestHealthyIncompatiblePreloadIsNotDamaged(t *testing.T) {
	s, _ := testStore(t)
	m := importFixture(t, s, "preloaded-previous-contract")
	candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
	require.NoError(t, err)
	require.NoError(t, candidate.SealPreload())
	require.NoError(t, candidate.Close())
	m = setPersistedContract(t, s, m, 1)
	require.NoError(t, saveJSON(s.root, "preload.json", preloadRecord{Manifest: m, Path: s.path}))

	_, _, err = ReadPreload(s.path, nil)
	require.ErrorContains(t, err, "unsupported runtime execution contract 1")
	require.ErrorIs(t, err, ErrIncompatible)
	require.NotErrorIs(t, err, errPreloadDamaged)
	require.NotErrorIs(t, err, ErrDamaged)
}
