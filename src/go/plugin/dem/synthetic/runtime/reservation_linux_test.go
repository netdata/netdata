// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReservedDemandAbortPreservesIdleAndProtectsFromPressure(t *testing.T) {
	s, now := testStore(t)
	s.config.MaxBytes = 128 << 10
	m := importFixture(t, s, strings.Repeat("A", 80<<10))
	path := filepath.Join(s.path, "generations", m.Archive.SHA256, "prepared.json")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	*now = now.Add(365 * 24 * time.Hour)
	claim, err := s.ReserveDemand(m.Archive.SHA256)
	require.NoError(t, err)
	defer claim.Close()
	other, err := Open(s.config)
	require.NoError(t, err)
	defer other.Close()
	other.now = s.now
	require.NoError(t, other.Sweep())
	status, err := other.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.PendingClaims)
	require.Zero(t, status.DemandGenerations)
	require.Positive(t, status.PendingBytes)
	next, archive := bundleFixture(t, strings.Repeat("B", 80<<10))
	input := filepath.Join(t.TempDir(), "archive")
	require.NoError(t, os.WriteFile(input, archive, 0600))
	require.ErrorIs(t, other.Import(context.Background(), next, input), ErrAllowance)
	require.NoError(t, claim.Close())
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.NoError(t, other.Sweep())
	require.ErrorIs(t, other.Available(m), ErrUnavailable)
}

func TestReservedDemandCommitFailureRetainsProtectionAndCanRetry(t *testing.T) {
	s, now := testStore(t)
	m := importFixture(t, s, "selected")
	*now = now.Add(365 * 24 * time.Hour)
	claim, err := s.ReserveDemand(m.Archive.SHA256)
	require.NoError(t, err)
	defer claim.Close()
	owner := filepath.Join(s.path, claim.name, "owner.json")
	require.NoError(t, os.Rename(owner, owner+".saved"))
	require.NoError(t, os.Mkdir(owner, 0700))
	require.Error(t, claim.Commit(), "simulate an owner publication failure")
	require.NoError(t, os.Rename(owner, owner+".failed"))
	require.NoError(t, os.Rename(owner+".saved", owner))
	require.NoError(t, s.Sweep())
	require.NoError(t, s.Available(m))
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.PendingClaims)
	require.Zero(t, status.DemandGenerations)
	require.NoError(t, claim.Commit())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.PendingClaims)
	require.Equal(t, 1, status.DemandGenerations)
	require.NoError(t, claim.Close())
	*now = now.Add(29 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	require.NoError(t, s.Available(m))
	*now = now.Add(2 * 24 * time.Hour)
	require.NoError(t, s.Sweep())
	require.ErrorIs(t, s.Available(m), ErrUnavailable)
}
