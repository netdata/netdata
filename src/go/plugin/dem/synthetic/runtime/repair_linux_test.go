// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func damageRuntime(t *testing.T, s *Store, old, selected Manifest, damage string) {
	t.Helper()
	name := "selection.json"
	switch damage {
	case "missing-selection":
		require.NoError(t, os.Rename(filepath.Join(s.path, name), filepath.Join(t.TempDir(), "lost")))
		return
	case "mismatched-selection", "legacy-selection":
		record, err := s.readSelectionRecord()
		require.NoError(t, err)
		var value any = record
		if damage == "mismatched-selection" {
			record.Release.Manifest.Version = "different-version"
		} else {
			// The former record stored the manifest beside its release.
			value = map[string]any{
				"manifest": record.Release.Manifest,
				"release":  record.Release,
			}
		}
		data, err := json.Marshal(value)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(s.path, name), data, 0600))
		return
	case "missing-selected":
		require.NoError(
			t,
			os.Rename(
				filepath.Join(s.path, "generations", selected.Archive.SHA256),
				filepath.Join(t.TempDir(), "lost"),
			),
		)
		return
	case "missing-prepared", "invalid-prepared", "invalid-prepared-fields":
		name = filepath.Join("generations", selected.Archive.SHA256, "prepared.json")
	case "invalid-unselected":
		name = filepath.Join("generations", old.Archive.SHA256, "prepared.json")
	}
	if damage == "missing-prepared" {
		require.NoError(t, os.Rename(filepath.Join(s.path, name), filepath.Join(t.TempDir(), "lost")))
		return
	}
	content := "{broken"
	switch damage {
	case "empty-selection":
		content = ""
	case "missing-selection-field":
		content = "{}"
	case "null-selection-record":
		content = "null"
	case "trailing-selection":
		content = "{\"release\":null} {}"
	case "invalid-selection":
		content = "{\"release\":{}}"
	case "legacy-selection-variant":
		// Only the exact empty record earlier builds wrote means uninstalled.
		content = "{\"manifest\":null,\"release\":null}\n"
	case "unknown-selection-field":
		content = "{\"unexpected\":true}"
	case "invalid-prepared-fields":
		content = "{}"
	}
	require.NoError(t, os.WriteFile(filepath.Join(s.path, name), []byte(content), 0600))
}

var metadataDamageCases = []string{
	"malformed-selection",
	"missing-selection",
	"missing-selection-field",
	"null-selection-record",
	"empty-selection",
	"trailing-selection",
	"invalid-selection",
	"unknown-selection-field",
	"mismatched-selection",
	"legacy-selection",
	"legacy-selection-variant",
	"missing-selected",
	"missing-prepared",
	"invalid-prepared",
	"invalid-prepared-fields",
	"invalid-unselected",
}

func TestRecoveryUninstallDamagedMetadata(t *testing.T) {
	for _, damage := range metadataDamageCases {
		t.Run(damage, func(t *testing.T) {
			s, _ := testStore(t)
			old := installFixture(t, s, "old")
			selected := installFixture(t, s, "selected")
			damageRuntime(t, s, old, selected, damage)
			require.NoError(t, s.Uninstall(context.Background()))
			_, err := s.AcquireSelected()
			require.ErrorIs(t, err, ErrUnavailable)
			entries, err := os.ReadDir(filepath.Join(s.path, "generations"))
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestRecoveryStatusRetainsDamagedInventory(t *testing.T) {
	for _, damage := range metadataDamageCases {
		t.Run(damage, func(t *testing.T) {
			s, _ := testStore(t)
			old := installFixture(t, s, "old")
			selected := installFixture(t, s, "selected")
			damageRuntime(t, s, old, selected, damage)
			status, err := s.Inspect()
			require.Error(t, err)
			require.Positive(t, status.UsedBytes)
			require.Equal(t, s.config.MaxBytes, status.LimitBytes)
		})
	}
}

func TestRepairRestoresMetadataDamageClass(t *testing.T) {
	for _, damage := range metadataDamageCases {
		t.Run(damage, func(t *testing.T) {
			s, _ := testStore(t)
			old := installFixture(t, s, "old")
			selected := installFixture(t, s, "selected")
			damageRuntime(t, s, old, selected, damage)
			// Damage never reads as an ordinary absence through the strict inventory.
			err := s.Sweep()
			require.ErrorIs(t, err, ErrDamaged)
			require.NotErrorIs(t, err, ErrUnavailable)
			require.NotErrorIs(t, err, os.ErrNotExist)
			status, err := s.Inspect()
			require.ErrorIs(t, err, ErrDamaged)
			require.NotErrorIs(t, err, ErrUnavailable)
			require.NotErrorIs(t, err, os.ErrNotExist)
			require.Empty(t, status.InventoryError)
			if damage == "invalid-unselected" {
				require.Empty(t, status.SelectionError)
			} else {
				require.NotEmpty(t, status.SelectionError)
				_, err := s.AcquireSelected()
				require.ErrorIs(t, err, ErrDamaged)
				require.NotErrorIs(t, err, ErrUnavailable)
				// An unknown/broken selection cannot authorize reclamation.
				require.Positive(t, status.ProtectedBytes)
			}
			candidate, err := s.beginFixture(context.Background(), selected, RepairRelease)
			require.NoError(t, err)
			_, err = s.AcquireSelected()
			require.ErrorIs(t, err, ErrUnavailable)
			// The same identity must be rebuilt, not accepted from its old record.
			require.NoDirExists(t, filepath.Join(s.path, "generations", selected.Archive.SHA256))
			m, archive := bundleFixture(t, "selected")
			require.Equal(t, selected, m)
			input := archiveFile(t, archive)
			require.NoError(t, candidate.Import(context.Background(), input))
			require.NoError(t, candidate.Select())
			require.NoError(t, candidate.Close())
			status, err = s.Inspect()
			require.NoError(t, err)
			require.Equal(t, selected, *status.Selected)
			require.Empty(t, status.SelectionError)
			require.Empty(t, status.GenerationErrors)
			consumer, err := s.AcquireSelected()
			require.NoError(t, err)
			require.NoError(t, consumer.Close())
		})
	}
}

func TestRepairPreservesHealthyDifferentSelectionOnFailure(t *testing.T) {
	s, _ := testStore(t)
	old := installFixture(t, s, "old")
	selected := installFixture(t, s, "selected")
	damageRuntime(t, s, old, selected, "invalid-unselected")
	candidate, err := s.beginFixture(context.Background(), old, RepairRelease)
	require.NoError(t, err)
	require.Error(t, candidate.Import(context.Background(), filepath.Join(t.TempDir(), "missing")))
	require.NoError(t, candidate.Close())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, selected, *status.Selected)
}

func TestRepairRefusesProtectedOrUnverifiableOwnership(t *testing.T) {
	for _, protection := range []string{"process", "execution", "unresolved-execution", "unreadable-execution", "invalid-owner-lock"} {
		t.Run(protection, func(t *testing.T) {
			s, _ := testStore(t)
			m := installFixture(t, s, "selected")
			consumer, err := s.AcquireSelected()
			require.NoError(t, err)
			var use *Use
			if protection != "process" {
				use, err = consumer.BeginUse()
				require.NoError(t, err)
				require.NoError(t, consumer.Close())
				if protection != "execution" {
					require.NoError(t, use.Finish(false))
				}
				if protection == "unreadable-execution" {
					require.NoError(
						t,
						os.WriteFile(filepath.Join(s.path, use.name, "owner.json"), []byte("{broken"), 0600),
					)
				}
				if protection == "invalid-owner-lock" {
					require.NoError(t, os.Chmod(filepath.Join(s.path, use.name, "owner.lock"), 0644))
				}
			}
			damageRuntime(t, s, m, m, "malformed-selection")
			before, err := os.ReadFile(filepath.Join(s.path, "selection.json"))
			require.NoError(t, err)
			_, err = s.beginFixture(context.Background(), m, RepairRelease)
			require.Error(t, err)
			require.Error(t, s.Uninstall(context.Background()))
			after, err := os.ReadFile(filepath.Join(s.path, "selection.json"))
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.DirExists(t, filepath.Join(s.path, "generations", m.Archive.SHA256))
			status, err := s.Inspect()
			require.Error(t, err)
			if protection == "unreadable-execution" {
				require.NotEmpty(t, status.InventoryError)
				require.NotErrorIs(t, err, ErrDamaged, "incomplete safety inventory is not recoverable payload damage")
			}
			if protection == "process" {
				require.ErrorIs(t, consumer.Close(), ErrDamaged)
			} else if protection == "execution" {
				require.NoError(t, use.Finish(true))
			}
		})
	}
}

func TestRepairPreservesOperationalFailures(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure requires the unprivileged service identity")
	}
	for _, name := range []string{"selection.json", "prepared.json"} {
		t.Run(name, func(t *testing.T) {
			s, _ := testStore(t)
			m := installFixture(t, s, "selected")
			path := filepath.Join(s.path, name)
			if name == "prepared.json" {
				path = filepath.Join(s.path, "generations", m.Archive.SHA256, name)
			}
			require.NoError(t, os.Chmod(path, 0))
			t.Cleanup(func() { require.NoError(t, os.Chmod(path, 0600)) })
			status, err := s.Inspect()
			require.ErrorIs(t, err, os.ErrPermission)
			require.NotErrorIs(t, err, ErrDamaged)
			require.NotEmpty(t, status.InventoryError)
			_, err = s.beginFixture(context.Background(), m, RepairRelease)
			require.ErrorIs(t, err, os.ErrPermission)
			require.ErrorIs(t, s.Uninstall(context.Background()), os.ErrPermission)
			require.DirExists(t, filepath.Join(s.path, "generations", m.Archive.SHA256))
		})
	}
}

func TestRepairClearUncertaintyRetainsBytesUntilRetry(t *testing.T) {
	s, _ := testStore(t)
	m := installFixture(t, s, "selected")
	original := s.syncSelection
	calls := 0
	s.syncSelection = func() error {
		calls++
		if calls >= 2 {
			return errors.New("selection directory sync failed")
		}
		return original()
	}
	_, err := s.beginFixture(context.Background(), m, RepairRelease)
	var uncertain *SelectionError
	require.ErrorAs(t, err, &uncertain)
	require.DirExists(t, filepath.Join(s.path, "generations", m.Archive.SHA256))
	_, err = s.beginFixture(context.Background(), m, RepairRelease)
	require.Error(t, err)
	require.DirExists(t, filepath.Join(s.path, "generations", m.Archive.SHA256))
	s.syncSelection = original
	candidate, err := s.beginFixture(context.Background(), m, RepairRelease)
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(s.path, "generations", m.Archive.SHA256))
	require.NoError(t, candidate.Close())
}

func TestRepairWriterExitProcess(t *testing.T) {
	mode := os.Getenv("DEM_REPAIR_EXIT_MODE")
	if mode == "" {
		return
	}
	s, err := Open(DefaultConfig(os.Getenv("DEM_REPAIR_EXIT_STORE")))
	require.NoError(t, err)
	m, err := s.readSelection()
	require.NoError(t, err)
	require.NotNil(t, m)
	if mode == "after-clear-rename" {
		original := s.syncSelection
		calls := 0
		s.syncSelection = func() error {
			calls++
			if calls == 2 {
				os.Exit(helperCrashExit)
			}
			return original()
		}
	}
	_, err = s.beginFixture(context.Background(), *m, RepairRelease)
	require.NoError(t, err)
	os.Exit(helperCrashExit)
}

func TestRepairRetryAfterCallerExit(t *testing.T) {
	for _, mode := range []string{"after-clear-rename", "after-retirement"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := testStore(t)
			m := installFixture(t, s, "selected")
			output, exit := runHelper(t, "TestRepairWriterExitProcess", "DEM_REPAIR_EXIT_MODE="+mode, "DEM_REPAIR_EXIT_STORE="+s.path)
			require.Equal(t, helperCrashExit, exit, string(output))
			candidate, err := s.beginFixture(context.Background(), m, RepairRelease)
			require.NoError(t, err)
			_, archive := bundleFixture(t, "selected")
			require.NoError(t, candidate.Import(context.Background(), archiveFile(t, archive)))
			require.NoError(t, candidate.Select())
			require.NoError(t, candidate.Close())
			status, err := s.Inspect()
			require.NoError(t, err)
			require.Equal(t, m, *status.Selected)
		})
	}
}

func TestRepairSerializesStartupAndInstallers(t *testing.T) {
	s, _ := testStore(t)
	m := installFixture(t, s, "selected")
	other, err := Open(s.config)
	require.NoError(t, err)
	defer other.Close()
	for range 12 {
		start := make(chan struct{})
		type acquisition struct {
			consumer *Consumer
			err      error
		}
		acquired := make(chan acquisition, 1)
		go func() {
			<-start
			c, err := other.AcquireSelected()
			acquired <- acquisition{
				consumer: c,
				err:      err,
			}
		}()
		close(start)
		candidate, repairErr := s.beginFixture(context.Background(), m, RepairRelease)
		result := <-acquired
		if repairErr == nil {
			require.ErrorIs(t, result.err, ErrUnavailable)
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, err := other.beginFixture(ctx, m, InstallRelease)
			require.ErrorIs(t, err, context.Canceled)
			require.ErrorIs(t, other.Uninstall(ctx), context.Canceled)
			require.NoError(t, candidate.Close())
			installFixture(t, s, "selected")
		} else {
			require.ErrorIs(t, repairErr, ErrInUse)
			require.NoError(t, result.err)
			require.NoError(t, result.consumer.Close())
		}
	}
}

func TestRepairRetirementFailureIsRetryable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure requires the unprivileged service identity")
	}
	s, _ := testStore(t)
	m := installFixture(t, s, "selected")
	trash := filepath.Join(s.path, "trash")
	require.NoError(t, os.Chmod(trash, 0))
	_, err := s.beginFixture(context.Background(), m, RepairRelease)
	require.ErrorIs(t, err, os.ErrPermission)
	require.NoError(t, os.Chmod(trash, 0700))
	require.DirExists(t, filepath.Join(s.path, "generations", m.Archive.SHA256))
	_, err = s.AcquireSelected()
	require.ErrorIs(t, err, ErrUnavailable)
	candidate, err := s.beginFixture(context.Background(), m, RepairRelease)
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(s.path, "generations", m.Archive.SHA256))
	require.NoError(t, candidate.Close())
}
