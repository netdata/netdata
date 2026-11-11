// SPDX-License-Identifier: GPL-3.0-or-later
package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func installFixture(t *testing.T, s *Store, label string) Manifest {
	t.Helper()
	m, archive := bundleFixture(t, label)
	input := filepath.Join(t.TempDir(), "archive")
	require.NoError(t, os.WriteFile(input, archive, 0600))
	candidate, err := s.BeginInstall(context.Background(), m)
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), input))
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
	return m
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
	input := filepath.Join(t.TempDir(), "archive")
	require.NoError(t, os.WriteFile(input, archive, 0600))
	candidate, err := other.BeginInstall(context.Background(), next)
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

func TestCandidateProtectionAndSerializedUninstall(t *testing.T) {
	s, now := testStore(t)
	m := importFixture(t, s, "candidate")
	candidate, err := s.BeginInstall(context.Background(), m)
	require.NoError(t, err)
	*now = now.Add(365 * 24 * time.Hour)
	other, err := Open(s.config)
	require.NoError(t, err)
	defer other.Close()
	other.now = s.now
	require.NoError(t, other.Sweep())
	require.NoError(t, other.Available(m))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, other.Uninstall(ctx), context.Canceled)
	_, err = other.BeginInstall(ctx, m)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, candidate.Close())
	require.NoError(t, other.Uninstall(context.Background()))
	_, err = candidate.BeginUse()
	require.ErrorContains(t, err, "closed")
}

func TestSelectionPublicationUncertaintyBlocksDeletionUntilReconciled(t *testing.T) {
	s, now := testStore(t)
	first := installFixture(t, s, "first")
	next := importFixture(t, s, "second")
	candidate, err := s.BeginInstall(context.Background(), next)
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
	candidate, err := s.BeginInstall(context.Background(), next)
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
		data, err := os.ReadFile(os.Getenv("DEM_SELECTION_EXIT_MANIFEST"))
		require.NoError(t, err)
		var m Manifest
		require.NoError(t, json.Unmarshal(data, &m))
		candidate, err = s.BeginInstall(context.Background(), m)
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
			file := filepath.Join(t.TempDir(), "manifest.json")
			data, err := json.Marshal(next)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(file, data, 0600))
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSelectionWriterExitProcess$")
			cmd.Env = append(os.Environ(), "DEM_SELECTION_EXIT_MODE="+mode, "DEM_SELECTION_EXIT_STORE="+s.path, "DEM_SELECTION_EXIT_MANIFEST="+file)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			reopened, err := Open(s.config)
			require.NoError(t, err)
			defer reopened.Close()
			consumer, err := reopened.AcquireSelected()
			if mode == "select" {
				require.NoError(t, err)
				require.Equal(t, next, consumer.Manifest())
				require.NoError(t, consumer.Close())
				require.NoError(t, reopened.Sweep())
				require.NoError(t, reopened.Available(next))
			} else {
				require.ErrorIs(t, err, ErrUnavailable)
				require.NoError(t, reopened.Uninstall(context.Background()))
			}
		})
	}
}

func TestPreparationReportsOneCandidateClaim(t *testing.T) {
	s, _ := testStore(t)
	m, archive := bundleFixture(t, "status")
	candidate, err := s.BeginInstall(context.Background(), m)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, candidate.Close()) })
	ready, resume := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.prepare(context.Background(), m, func() (io.ReadCloser, error) {
			return io.NopCloser(&resumeReader{ready: ready, resume: resume, tail: bytes.NewReader(archive)}), nil
		})
	}()
	joined := false
	t.Cleanup(func() {
		select {
		case <-resume:
		default:
			close(resume)
		}
		if !joined {
			require.NoError(t, <-done)
		}
	})
	<-ready
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.CandidateClaims, "one installer is not two claims while staging")
	require.Positive(t, status.ReservedBytes)
	require.Positive(t, status.PreparingBytes)
	require.Zero(t, status.ProcessClaims)
	close(resume)
	result := <-done
	joined = true
	require.NoError(t, result)
	require.NoError(t, candidate.Select())
}

func TestUnsupportedContractKeepsInstalledSelection(t *testing.T) {
	s, _ := testStore(t)
	selected := installFixture(t, s, "selected")
	unsupported := selected
	unsupported.Contract = 2
	candidate, err := s.BeginInstall(context.Background(), unsupported)
	require.Nil(t, candidate)
	require.ErrorContains(t, err, "unsupported runtime execution contract 2")
	require.ErrorContains(t, err, "upgrade the Agent")
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, selected, *status.Selected)
}

func TestManagedClaimCloseStartsGraceImmediately(t *testing.T) {
	for _, kind := range []string{"consumer", "candidate"} {
		t.Run(kind, func(t *testing.T) {
			s, now := testStore(t)
			var manifest Manifest
			var owner io.Closer
			if kind == "consumer" {
				manifest = installFixture(t, s, "old-consumer")
				consumer, err := s.AcquireSelected()
				require.NoError(t, err)
				owner = consumer
				installFixture(t, s, "replacement")
			} else {
				manifest = importFixture(t, s, "unselected-candidate")
				candidate, err := s.BeginInstall(context.Background(), manifest)
				require.NoError(t, err)
				owner = candidate
			}
			// Establish that the live claim prevents a grace deadline.
			require.NoError(t, s.Sweep())
			before, err := s.loadGeneration(manifest.Archive.SHA256)
			require.NoError(t, err)
			require.True(t, before.Unneeded.IsZero())
			*now = now.Add(time.Hour)
			require.NoError(t, owner.Close())
			after, err := s.loadGeneration(manifest.Archive.SHA256)
			require.NoError(t, err)
			require.Equal(t, now.UTC(), after.Unneeded, "close must reconcile grace without waiting for a sweep")
			*now = now.Add(time.Hour)
			require.NoError(t, owner.Close())
			again, err := s.loadGeneration(manifest.Archive.SHA256)
			require.NoError(t, err)
			require.Equal(t, after.Unneeded, again.Unneeded)
		})
	}
}
