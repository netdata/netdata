// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLegacyEmptySelectionIsUninstalled(t *testing.T) {
	s, _ := testStore(t)
	path := filepath.Join(s.path, "selection.json")
	require.NoError(t, os.WriteFile(path, []byte(legacyEmptySelection), 0600))
	_, err := s.AcquireSelected()
	require.ErrorIs(t, err, ErrUnavailable)
	require.NotErrorIs(t, err, ErrDamaged)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Nil(t, status.Selected)
	require.Empty(t, status.SelectionError)
	// Reads never rewrite the record; the next selection replaces it.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, legacyEmptySelection, string(data))
	selected := installFixture(t, s, "after-legacy")
	consumer, err := s.AcquireSelected()
	require.NoError(t, err)
	require.Equal(t, selected, consumer.Manifest())
	require.NoError(t, consumer.Close())
	data, err = os.ReadFile(path)
	require.NoError(t, err)
	require.NotEqual(t, legacyEmptySelection, string(data))
}

func TestSelectedInstallationSurvivesIdleRestartAndPressure(t *testing.T) {
	s, now := testStore(t)
	s.config.MaxBytes = 128 << 10
	selected := installFixture(t, s, strings.Repeat("A", 80<<10))
	*now = now.Add(365 * 24 * time.Hour)
	other, err := Open(s.config)
	require.NoError(t, err)
	defer other.Close()
	other.now = s.now
	require.NoError(t, other.Sweep())
	next, archive := bundleFixture(t, strings.Repeat("B", 80<<10))
	input := archiveFile(t, archive)
	candidate, err := other.beginFixture(context.Background(), next, InstallRelease)
	require.NoError(t, err)
	require.ErrorIs(t, candidate.Import(context.Background(), input), ErrAllowance)
	require.NoError(t, candidate.Close())
	status, err := other.Inspect()
	require.NoError(t, err)
	require.Equal(t, selected, *status.Selected)
	require.Positive(t, status.SelectedBytes)
	consumer, err := other.AcquireSelected()
	require.NoError(t, err)
	require.Equal(t, selected, consumer.Manifest())
	require.NoError(t, consumer.Close())
	require.NoError(t, other.Uninstall(context.Background()))
	_, err = other.AcquireSelected()
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestActivationKeepsOldProcessAndUninstallRefusesEveryGeneration(t *testing.T) {
	s, now := testStore(t)
	first := installFixture(t, s, "first")
	old, err := s.AcquireSelected()
	require.NoError(t, err)
	second := installFixture(t, s, "second")
	fresh, err := s.AcquireSelected()
	require.NoError(t, err)
	require.Equal(t, first, old.Manifest())
	require.Equal(t, second, fresh.Manifest())
	*now = now.Add(365 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	use, err := old.BeginUse()
	require.NoError(t, err)
	require.Contains(t, use.Paths.Node, first.Archive.SHA256)
	require.ErrorIs(t, s.Uninstall(context.Background()), ErrInUse)
	require.NoError(t, fresh.Close())
	require.NoError(t, old.Close())
	require.ErrorIs(t, s.Uninstall(context.Background()), ErrInUse, "execution outlives process claim")
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, second, *status.Selected)
	require.NoError(t, use.Finish(true))
	require.NoError(t, s.Uninstall(context.Background()))
	_, err = old.BeginUse()
	require.ErrorContains(t, err, "closed")
}

func TestSelectionPublicationUncertaintyBlocksDeletionUntilReconciled(t *testing.T) {
	s, now := testStore(t)
	first := installFixture(t, s, "first")
	next := importFixture(t, s, "second")
	candidate, err := s.beginFixture(context.Background(), next, InstallRelease)
	require.NoError(t, err)
	original := s.syncSelection
	calls := 0
	failure := errors.New("directory sync failed")
	s.syncSelection = func() error {
		calls++
		if calls >= 2 {
			return failure
		}
		return original()
	}
	err = candidate.Select()
	var uncertain *SelectionError
	require.ErrorAs(t, err, &uncertain)
	// A real rename occurred; do not claim first is still selected.
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, next, *status.Selected)
	*now = now.Add(365 * 24 * time.Hour)
	require.ErrorIs(t, s.Sweep(), failure)
	require.FileExists(t, filepath.Join(s.path, "generations", first.Archive.SHA256, "prepared.json"))
	require.FileExists(t, filepath.Join(s.path, "generations", next.Archive.SHA256, "prepared.json"))
	require.ErrorIs(t, candidate.Close(), failure)
	s.syncSelection = original
	reopened, err := Open(s.config)
	require.NoError(t, err)
	defer reopened.Close()
	reopened.now = s.now
	require.NoError(t, reopened.Sweep())
	consumer, err := reopened.AcquireSelected()
	require.NoError(t, err)
	require.Equal(t, next, consumer.Manifest())
	require.NoError(t, consumer.Close())
	require.NoError(t, reopened.Uninstall(context.Background()))
}

func TestPrepublicationFailurePreservesSelection(t *testing.T) {
	s, _ := testStore(t)
	first := installFixture(t, s, "first")
	next := importFixture(t, s, "second")
	candidate, err := s.beginFixture(context.Background(), next, InstallRelease)
	require.NoError(t, err)
	defer candidate.Close()
	s.syncSelection = func() error { return errors.New("failed before publication") }
	err = candidate.Select()
	require.Error(t, err)
	var uncertain *SelectionError
	require.False(t, errors.As(err, &uncertain))
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, first, *status.Selected)
	s.syncSelection = func() error { return syncDirectory(s.root, ".") }
}

// Exit exactly after the real rename and before its directory durability barrier.
// This exercises process loss, not physical power failure or kernel crash.
func TestSelectionWriterExitProcess(t *testing.T) {
	mode := os.Getenv("DEM_SELECTION_EXIT_MODE")
	if mode == "" {
		return
	}
	s, err := Open(DefaultConfig(os.Getenv("DEM_SELECTION_EXIT_STORE")))
	require.NoError(t, err)
	var candidate *Candidate
	if mode == "select" {
		// The parent allocates the release: this process has its own sequence counter.
		data, err := os.ReadFile(os.Getenv("DEM_SELECTION_EXIT_RELEASE"))
		require.NoError(t, err)
		var release Release
		require.NoError(t, json.Unmarshal(data, &release))
		candidate, err = s.beginRelease(context.Background(), release, InstallRelease)
		require.NoError(t, err)
	}
	count := 0
	original := s.syncSelection
	s.syncSelection = func() error {
		count++
		if count == 2 {
			os.Exit(0)
		}
		return original()
	}
	if candidate != nil {
		err = candidate.Select()
	} else {
		err = s.Uninstall(context.Background())
	}
	t.Fatalf("expected exit at selection publication, got %v", err)
}

func TestRecoverSelectionAfterWriterExit(t *testing.T) {
	for _, mode := range []string{"select", "uninstall"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := testStore(t)
			first := installFixture(t, s, "first")
			next := first
			if mode == "select" {
				next = importFixture(t, s, "next")
			}
			file := filepath.Join(t.TempDir(), "release.json")
			data, err := json.Marshal(fixtureRelease(next))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(file, data, 0600))
			output, status := runHelper(t, "TestSelectionWriterExitProcess", "DEM_SELECTION_EXIT_MODE="+mode, "DEM_SELECTION_EXIT_STORE="+s.path, "DEM_SELECTION_EXIT_RELEASE="+file)
			require.Zero(t, status, "%s", output)
			reopened, err := Open(s.config)
			require.NoError(t, err)
			defer reopened.Close()
			consumer, err := reopened.AcquireSelected()
			if mode == "select" {
				require.NoError(t, err)
				require.Equal(t, next, consumer.Manifest())
				require.NoError(t, consumer.Close())
				require.NoError(t, reopened.Sweep())
				requirePrepared(t, reopened, next)
			} else {
				require.ErrorIs(t, err, ErrUnavailable)
				require.NoError(t, reopened.Uninstall(context.Background()))
			}
		})
	}
}
