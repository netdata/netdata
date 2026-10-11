// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStartupGuardPreservesSelectionAndIdleTimestamps(t *testing.T) {
	for _, older := range []bool{false, true} {
		name := "expired"
		if older {
			name = "older rollback"
		}
		t.Run(name, func(t *testing.T) {
			s, now := testStore(t)
			selected := importFixture(t, s, "selected")
			if older {
				*now = now.Add(time.Hour)
				importFixture(t, s, "newer")
			} else {
				*now = now.Add(31 * 24 * time.Hour)
			}
			metadata := filepath.Join(s.path, "generations", selected.Archive.SHA256, "prepared.json")
			before, err := os.ReadFile(metadata)
			require.NoError(t, err)
			// Every no-demand restart must preserve the original idle deadline.
			for range 3 {
				owner, guard, err := OpenWithStartupGuard(s.config, selected.Archive.SHA256)
				require.NoError(t, err)
				require.NoError(t, s.Sweep(), "another store must respect startup protection")
				status, err := s.Inspect()
				require.NoError(t, err)
				require.Equal(t, 1, status.StartupGuards)
				require.Positive(t, status.StartupBytes)
				require.Zero(t, status.DemandGenerations)
				require.Zero(t, status.ExecutionPins)
				after, err := os.ReadFile(metadata)
				require.NoError(t, err)
				require.Equal(t, before, after)
				require.NoError(t, guard.Close())
				require.NoError(t, guard.Close(), "release is idempotent")
				after, err = os.ReadFile(metadata)
				require.NoError(t, err)
				require.Equal(t, before, after, "release must not reconcile timestamps")
				require.NoError(t, owner.Close())
			}
			require.NoError(t, s.Sweep())
			_, err = s.BeginUse(selected)
			require.ErrorIs(t, err, ErrUnavailable, "release restores the original retirement policy")
		})
	}
}

func TestStartupGuardRealDemandRestartsIdleGrace(t *testing.T) {
	s, now := testStore(t)
	selected := importFixture(t, s, "selected")
	*now = now.Add(31 * 24 * time.Hour)
	owner, guard, err := OpenWithStartupGuard(s.config, selected.Archive.SHA256)
	require.NoError(t, err)
	defer owner.Close()
	defer guard.Close()
	owner.now = s.now
	claim, err := owner.Demand(selected.Archive.SHA256)
	require.NoError(t, err)
	defer claim.Close()
	metadata := filepath.Join(s.path, "generations", selected.Archive.SHA256, "prepared.json")
	before, err := os.ReadFile(metadata)
	require.NoError(t, err)
	var demanded generation
	require.NoError(t, json.Unmarshal(before, &demanded))
	require.True(t, demanded.Unneeded.IsZero(), "real demand clears idle time")
	require.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), demanded.LastUsed)
	require.NoError(t, guard.Close())
	after, err := os.ReadFile(metadata)
	require.NoError(t, err)
	require.Equal(t, before, after)
	*now = now.Add(time.Hour)
	require.NoError(t, claim.Close())
	after, err = os.ReadFile(metadata)
	require.NoError(t, err)
	var released generation
	require.NoError(t, json.Unmarshal(after, &released))
	require.Equal(t, *now, released.Unneeded)
	require.Equal(t, demanded.LastUsed, released.LastUsed)
	*now = now.Add(29 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	require.FileExists(t, metadata, "actual demand release earns a new idle grace")
	*now = now.Add(2 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	_, err = s.BeginUse(selected)
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestStartupGuardProcessPressureAndOwnerRecovery(t *testing.T) {
	s, now := testStore(t)
	s.config.MaxBytes = 128 << 10
	selected := importFixture(t, s, strings.Repeat("A", 80<<10))
	metadata := filepath.Join(s.path, "generations", selected.Archive.SHA256, "prepared.json")
	before, err := os.ReadFile(metadata)
	require.NoError(t, err)
	stop := startRuntimeOwner(t, s, selected, "startup")
	*now = now.Add(365 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	incoming, archive := bundleFixture(t, strings.Repeat("B", 80<<10))
	file := filepath.Join(t.TempDir(), "archive")
	require.NoError(t, os.WriteFile(file, archive, 0600))
	require.ErrorIs(t, s.Import(context.Background(), incoming, file), ErrAllowance)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.StartupGuards)
	require.Positive(t, status.StartupBytes)
	require.GreaterOrEqual(t, status.ProtectedBytes, status.StartupBytes)
	require.Equal(t, status.UsedBytes, status.ProtectedBytes+status.ReclaimableBytes)
	require.Zero(t, status.DemandGenerations)
	require.Zero(t, status.ExecutionPins)
	after, err := os.ReadFile(metadata)
	require.NoError(t, err)
	require.Equal(t, before, after)
	stop() // exits without closing the guard: flock recovery is sufficient
	require.NoError(t, s.Import(context.Background(), incoming, file))
	_, err = s.BeginUse(selected)
	require.ErrorIs(t, err, ErrUnavailable)
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.StartupGuards)
	require.Zero(t, status.ExecutionPins)
}

func TestStartupGuardInvalidInventoryFailsClosed(t *testing.T) {
	for _, damage := range []string{"metadata", "identity", "lock", "inventory"} {
		t.Run(damage, func(t *testing.T) {
			s, now := testStore(t)
			s.config.MaxBytes = 128 << 10
			selected := importFixture(t, s, strings.Repeat("A", 80<<10))
			owner, guard, err := OpenWithStartupGuard(s.config, selected.Archive.SHA256)
			require.NoError(t, err)
			defer owner.Close()
			defer guard.Close()
			*now = now.Add(365 * 24 * time.Hour)
			switch damage {
			case "metadata":
				require.NoError(t, os.WriteFile(filepath.Join(s.path, guard.name, "owner.json"), []byte("broken"), 0600))
			case "identity":
				require.NoError(t, os.WriteFile(filepath.Join(s.path, guard.name, "owner.json"), []byte(`{"generation":"invalid"}`), 0600))
			case "lock":
				require.NoError(t, os.Chmod(filepath.Join(s.path, guard.name, "owner.lock"), 0644))
			case "inventory":
				require.NoError(t, os.WriteFile(filepath.Join(s.path, "startup", "unexpected"), nil, 0600))
			}
			_, err = s.Inspect()
			require.Error(t, err)
			require.Error(t, s.Sweep())
			incoming, _ := bundleFixture(t, strings.Repeat("B", 80<<10))
			require.Error(t, s.Import(context.Background(), incoming, "/must-not-open"))
			require.FileExists(t, filepath.Join(s.path, "generations", selected.Archive.SHA256, "prepared.json"))
		})
	}
}

func TestStartupGuardProtectsAbsentSelectionWithoutDemand(t *testing.T) {
	config := DefaultConfig(filepath.Join(t.TempDir(), "store"))
	s, guard, err := OpenWithStartupGuard(config, "invalid")
	require.Error(t, err)
	require.Nil(t, s)
	require.Nil(t, guard)
	require.NoDirExists(t, config.Path)
	selected, _ := bundleFixture(t, "not prepared")
	s, guard, err = OpenWithStartupGuard(config, selected.Archive.SHA256)
	require.NoError(t, err)
	defer s.Close()
	defer guard.Close()
	require.NoError(t, s.Sweep())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.StartupGuards)
	require.Zero(t, status.StartupBytes)
	require.Zero(t, status.DemandGenerations)
	_, err = s.BeginUse(selected)
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestStartupGuardRecoveryDoesNotReleaseExecution(t *testing.T) {
	s, now := testStore(t)
	selected := importFixture(t, s, "selected")
	stop := startRuntimeOwner(t, s, selected, "startup")
	use, err := s.BeginUse(selected)
	require.NoError(t, err)
	require.NoError(t, use.Finish(false))
	stop()
	*now = now.Add(365 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.StartupGuards)
	require.Equal(t, 1, status.ExecutionPins)
	require.Equal(t, 1, status.UncertainPins)
	require.FileExists(t, filepath.Join(s.path, "generations", selected.Archive.SHA256, "prepared.json"))
}

func TestStartupGuardRunHandoffPreservesProtection(t *testing.T) {
	s, now := testStore(t)
	s.config.MaxBytes = 128 << 10
	selected := importFixture(t, s, strings.Repeat("A", 80<<10))
	oldGuard, err := s.GuardStartup(selected.Archive.SHA256)
	require.NoError(t, err)
	defer oldGuard.Close()
	oldDemand, err := s.Demand(selected.Archive.SHA256)
	require.NoError(t, err)
	defer oldDemand.Close()
	metadata := filepath.Join(s.path, "generations", selected.Archive.SHA256, "prepared.json")
	before, err := os.ReadFile(metadata)
	require.NoError(t, err)
	newGuard, err := s.GuardStartup(selected.Archive.SHA256)
	require.NoError(t, err)
	defer newGuard.Close()
	after, err := os.ReadFile(metadata)
	require.NoError(t, err)
	require.Equal(t, before, after, "new run acquisition must not change timestamps")
	invalid, err := s.GuardStartup("invalid")
	require.Error(t, err)
	require.Nil(t, invalid)
	other, err := Open(s.config)
	require.NoError(t, err)
	defer other.Close()
	other.now = s.now
	status, err := other.Inspect()
	require.NoError(t, err)
	require.Equal(t, 2, status.StartupGuards)
	require.Equal(t, 1, status.DemandGenerations)
	*now = now.Add(time.Hour)
	require.NoError(t, oldGuard.Close())
	require.NoError(t, oldDemand.Close())
	before, err = os.ReadFile(metadata)
	require.NoError(t, err)
	*now = now.Add(365 * 24 * time.Hour)
	require.NoError(t, other.Sweep())
	incoming, archive := bundleFixture(t, strings.Repeat("B", 80<<10))
	file := filepath.Join(t.TempDir(), "archive")
	require.NoError(t, os.WriteFile(file, archive, 0600))
	require.ErrorIs(t, other.Import(context.Background(), incoming, file), ErrAllowance)
	status, err = other.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.StartupGuards)
	require.Zero(t, status.DemandGenerations)
	require.NoError(t, newGuard.Close())
	after, err = os.ReadFile(metadata)
	require.NoError(t, err)
	require.Equal(t, before, after, "old demand release sets the deadline; new guard must not change it")
	require.NoError(t, other.Import(context.Background(), incoming, file))
	_, err = s.BeginUse(selected)
	require.ErrorIs(t, err, ErrUnavailable)
}
