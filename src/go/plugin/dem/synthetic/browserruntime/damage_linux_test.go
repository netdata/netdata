// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// damageGeneration makes the prepared record of generation id unusable: broken
// JSON, a missing record, or a valid record of a different executable.
func damageGeneration(t *testing.T, s *Store, id, damage string) {
	t.Helper()
	name := filepath.Join("generations", id, "prepared.json")
	switch damage {
	case "broken record":
		require.NoError(t, os.WriteFile(filepath.Join(s.path, name), []byte("broken"), 0600))
	case "missing record":
		require.NoError(t, os.Remove(filepath.Join(s.path, name)))
	case "different executable":
		g, err := loadGeneration(s.root, id)
		require.NoError(t, err)
		g.Manifest.Version += ".other"
		require.NoError(t, saveJSON(s.root, name, g))
	default:
		t.Fatalf("unknown damage %q", damage)
	}
}

func TestManagedOperationsReportMetadataDamageAsErrDamaged(t *testing.T) {
	// Each operation damages the generation it needs and returns its error.
	operations := map[string]func(t *testing.T, s *Store, damage string) (string, error){
		"consumer use": func(t *testing.T, s *Store, damage string) (string, error) {
			m := installFixture(t, s, "consumer")
			consumer, err := s.AcquireSelected()
			require.NoError(t, err)
			t.Cleanup(func() { _ = consumer.Close() })
			damageGeneration(t, s, m.Archive.SHA256, damage)
			_, err = consumer.BeginUse()
			return m.Archive.SHA256, err
		},
		"candidate select": func(t *testing.T, s *Store, damage string) (string, error) {
			m, archive := bundleFixture(t, "select")
			candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
			require.NoError(t, err)
			t.Cleanup(func() { _ = candidate.Close() })
			require.NoError(t, candidate.Import(context.Background(), archiveFile(t, archive)))
			damageGeneration(t, s, m.Archive.SHA256, damage)
			return m.Archive.SHA256, candidate.Select()
		},
		"candidate seal preload": func(t *testing.T, s *Store, damage string) (string, error) {
			m, archive := bundleFixture(t, "seal")
			candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
			require.NoError(t, err)
			t.Cleanup(func() { _ = candidate.Close() })
			require.NoError(t, candidate.Import(context.Background(), archiveFile(t, archive)))
			damageGeneration(t, s, m.Archive.SHA256, damage)
			return m.Archive.SHA256, candidate.SealPreload()
		},
		"candidate import": func(t *testing.T, s *Store, damage string) (string, error) {
			m, archive := bundleFixture(t, "import")
			path := archiveFile(t, archive)
			require.NoError(t, s.importUnselected(t, context.Background(), m, path))
			damageGeneration(t, s, m.Archive.SHA256, damage)
			candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
			require.NoError(t, err)
			t.Cleanup(func() { _ = candidate.Close() })
			return m.Archive.SHA256, candidate.Import(context.Background(), path)
		},
		"candidate download": func(t *testing.T, s *Store, damage string) (string, error) {
			m := importFixture(t, s, "download")
			damageGeneration(t, s, m.Archive.SHA256, damage)
			candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
			require.NoError(t, err)
			t.Cleanup(func() { _ = candidate.Close() })
			// The prepared generation is checked before any network access.
			return m.Archive.SHA256, candidate.Download(context.Background())
		},
	}
	for name, operation := range operations {
		for _, damage := range []string{"broken record", "missing record", "different executable"} {
			t.Run(name+"/"+damage, func(t *testing.T) {
				s, _ := testStore(t)
				id, err := operation(t, s, damage)
				require.ErrorIs(t, err, ErrDamaged)
				require.ErrorContains(t, err, "runtime repair")
				require.NotErrorIs(t, err, os.ErrNotExist, "damage hides ordinary sentinels")
				require.ErrorContains(t, err, filepath.Join("generations", id, "prepared.json"))
				require.NotContains(t, err.Error(), "selection.json", "only the generation record is damaged")
			})
		}
	}
}

func TestManagedUseKeepsOperationalFailures(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure requires the unprivileged service identity")
	}
	s, _ := testStore(t)
	m := installFixture(t, s, "operational")
	consumer, err := s.AcquireSelected()
	require.NoError(t, err)
	defer consumer.Close()
	path := filepath.Join(s.path, "generations", m.Archive.SHA256, "prepared.json")
	require.NoError(t, os.Chmod(path, 0))
	t.Cleanup(func() { require.NoError(t, os.Chmod(path, 0600)) })
	_, err = consumer.BeginUse()
	require.ErrorIs(t, err, os.ErrPermission)
	require.NotErrorIs(t, err, ErrDamaged, "an unreadable record is not proven damage")
}

func TestReadPreloadReportsDamageWithPreloadAdvice(t *testing.T) {
	for _, damage := range []string{
		"broken record",
		"missing record",
		"different executable",
		"broken preload record",
		"invalid preload manifest",
	} {
		t.Run(damage, func(t *testing.T) {
			s, _ := testStore(t)
			m := importFixture(t, s, "preloaded")
			candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
			require.NoError(t, err)
			require.NoError(t, candidate.SealPreload())
			require.NoError(t, candidate.Close())
			switch damage {
			case "broken preload record":
				require.NoError(t, os.WriteFile(filepath.Join(s.path, "preload.json"), []byte("broken"), 0600))
			case "invalid preload manifest":
				var record preloadRecord
				require.NoError(t, readJSON(s.root, "preload.json", &record))
				record.Manifest.Bindings = nil
				require.NoError(t, saveJSON(s.root, "preload.json", record))
			default:
				damageGeneration(t, s, m.Archive.SHA256, damage)
			}
			_, _, err = ReadPreload(s.path)
			require.ErrorIs(t, err, errPreloadDamaged)
			require.NotErrorIs(t, err, ErrDamaged, "managed repair refuses sealed preloads")
			require.NotErrorIs(t, err, os.ErrNotExist, "damage hides ordinary sentinels")
			require.NotContains(t, err.Error(), "selection.json")
		})
	}
}

func TestPublishedInstallRejectsTheEnsureOperation(t *testing.T) {
	s, _ := testStore(t)
	candidate, err := s.BeginPublishedInstall(
		context.Background(),
		ReleaseInstallOptions{
			Operation: ensureRelease,
			Updates:   AutomaticUpdates,
		},
		func(context.Context) (VerifiedRelease, error) {
			t.Fatal("rejected operation invoked release discovery")
			return VerifiedRelease{}, nil
		},
	)
	require.ErrorContains(t, err, "invalid published runtime operation")
	require.Nil(t, candidate)
}
