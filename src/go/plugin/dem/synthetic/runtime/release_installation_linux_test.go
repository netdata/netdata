// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func publishedFixture(t *testing.T, sequence uint64) (Release, string, func(context.Context) (Release, error)) {
	t.Helper()
	m, archive := bundleFixture(t, strings.Repeat("release", int(sequence)))
	m.Archive.URL = "https://artifacts.example.invalid/" + m.Archive.SHA256 + ".tar.gz"
	release := Release{Format: 1, Sequence: sequence, Manifest: m,
		Sources: ReleaseSource{URL: "https://artifacts.example.invalid/sources.tar.gz", SHA256: strings.Repeat("b", 64), Bytes: 100}}
	_, key := releaseFixture(t)
	envelope, err := SignRelease(release, key)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	require.NoError(t, os.WriteFile(path, archive, 0600))
	return release, path, func(context.Context) (Release, error) {
		return VerifyRelease(envelope, []ed25519.PublicKey{key.Public().(ed25519.PublicKey)})
	}
}

func installPublishedFixture(t *testing.T, s *Store, sequence uint64, policy UpdatePolicy) Release {
	t.Helper()
	release, archive, load := publishedFixture(t, sequence)
	candidate, err := s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: InstallRelease, Updates: policy}, load)
	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.NoError(t, candidate.Import(context.Background(), archive))
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
	return release
}

func TestPublishedInstallationConsentAndSequenceAreAtomic(t *testing.T) {
	s, _ := testStore(t)
	current := installPublishedFixture(t, s, 2, AutomaticUpdates)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, current, *status.Release)
	require.True(t, status.AutomaticUpdates)
	old, err := s.AcquireSelected()
	require.NoError(t, err)
	defer old.Close()
	next, archive, load := publishedFixture(t, 3)
	candidate, err := s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: AutomaticReleaseUpdate}, load)
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), archive))
	before, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, current, *before.Release)
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Equal(t, next, *status.Release)
	require.Equal(t, next.Manifest, *status.Selected)
	require.True(t, status.AutomaticUpdates)
	require.Equal(t, current.Manifest, old.Manifest(), "running processes activate only on restart")
	_, _, older := publishedFixture(t, 1)
	_, err = s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: UpdateRelease}, older)
	require.ErrorContains(t, err, "regress")
	_, err = s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: AutomaticReleaseUpdate}, older)
	require.ErrorContains(t, err, "regress")
	unchanged, err := s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: AutomaticReleaseUpdate}, load)
	require.NoError(t, err)
	require.Nil(t, unchanged)
}

func TestAutomaticReleaseDoesNotDiscoverWithoutConsent(t *testing.T) {
	for _, state := range []string{"absent", "local", "manual", "uninstalled"} {
		t.Run(state, func(t *testing.T) {
			s, _ := testStore(t)
			switch state {
			case "local":
				installFixture(t, s, "trusted-local")
			case "manual":
				installPublishedFixture(t, s, 1, ManualUpdates)
			case "uninstalled":
				installPublishedFixture(t, s, 1, AutomaticUpdates)
				require.NoError(t, s.Uninstall(context.Background()))
			}
			candidate, err := s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: AutomaticReleaseUpdate}, func(context.Context) (Release, error) {
				t.Fatal("automatic update discovered a release without installed consent")
				return Release{}, nil
			})
			require.NoError(t, err)
			require.Nil(t, candidate)
		})
	}
}

func TestPublishedDiscoverySerializesUninstallAndCancellation(t *testing.T) {
	s, _ := testStore(t)
	current := installPublishedFixture(t, s, 1, AutomaticUpdates)
	other, err := Open(s.config)
	require.NoError(t, err)
	defer other.Close()
	entered := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := s.BeginPublishedInstall(ctx, ReleaseInstallOptions{Operation: AutomaticReleaseUpdate}, func(ctx context.Context) (Release, error) {
			close(entered)
			<-ctx.Done()
			return Release{}, ctx.Err()
		})
		done <- err
	}()
	<-entered
	consumer, err := other.AcquireSelected()
	require.NoError(t, err, "network discovery must not hold inventory coordination")
	require.Equal(t, current.Manifest, consumer.Manifest())
	require.NoError(t, consumer.Close())
	blocked, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	require.ErrorIs(t, other.Uninstall(blocked), context.DeadlineExceeded)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.NoError(t, other.Uninstall(context.Background()))
	// A check starting after uninstall cannot resurrect the previous consent.
	_, err = s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: AutomaticReleaseUpdate}, func(context.Context) (Release, error) {
		t.Fatal("discovered after uninstall")
		return current, nil
	})
	require.NoError(t, err)
}

func TestPublishedCandidateFailurePreservesSelectedPolicy(t *testing.T) {
	s, _ := testStore(t)
	current := installPublishedFixture(t, s, 2, AutomaticUpdates)
	_, archive, load := publishedFixture(t, 3)
	candidate, err := s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: InstallRelease, Updates: ManualUpdates}, load)
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), archive))
	// A caller aborts after failed qualification without Select.
	require.NoError(t, candidate.Close())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, current, *status.Release)
	require.True(t, status.AutomaticUpdates)
	_, _, same := publishedFixture(t, 2)
	candidate, err = s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: InstallRelease, Updates: ManualUpdates}, same)
	require.NoError(t, err)
	require.Nil(t, candidate)
	status, err = s.Inspect()
	require.NoError(t, err)
	require.False(t, status.AutomaticUpdates)
}

func TestPublishedSelectionUncertaintyRetainsOneCompleteRecord(t *testing.T) {
	s, _ := testStore(t)
	installPublishedFixture(t, s, 1, ManualUpdates)
	next, archive, load := publishedFixture(t, 2)
	candidate, err := s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: InstallRelease, Updates: AutomaticUpdates}, load)
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), archive))
	original := s.syncSelection
	calls := 0
	s.syncSelection = func() error {
		calls++
		if calls > 1 {
			return errors.New("directory sync failed")
		}
		return original()
	}
	var uncertain *SelectionError
	require.ErrorAs(t, candidate.Select(), &uncertain)
	// Inspect reads the visible atomic record without claiming crash durability.
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, next.Manifest, *status.Selected)
	require.Equal(t, next, *status.Release)
	require.True(t, status.AutomaticUpdates)
	s.syncSelection = original
	require.NoError(t, candidate.Close())
}

func TestPublishedRepairAllowsExplicitRecoveryAndRequiresDrain(t *testing.T) {
	s, _ := testStore(t)
	installPublishedFixture(t, s, 3, AutomaticUpdates)
	consumer, err := s.AcquireSelected()
	require.NoError(t, err)
	older, archive, load := publishedFixture(t, 1)
	options := ReleaseInstallOptions{Operation: RepairRelease, Updates: ManualUpdates}
	_, err = s.BeginPublishedInstall(context.Background(), options, load)
	require.ErrorIs(t, err, ErrInUse)
	require.NoError(t, consumer.Close())
	require.NoError(t, os.WriteFile(filepath.Join(s.path, "selection.json"), []byte("broken"), 0600))
	candidate, err := s.BeginPublishedInstall(context.Background(), options, load)
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), archive))
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, older, *status.Release)
	require.False(t, status.AutomaticUpdates)
}
