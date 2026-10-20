// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func blockCoordination(t *testing.T, s *Store) func() {
	t.Helper()
	path := filepath.Join(s.path, "coordination.lock")
	require.NoError(t, os.Rename(path, path+".saved"))
	require.NoError(t, os.Mkdir(path, 0700))
	return func() {
		require.NoError(t, os.Rename(path, path+".blocked"))
		require.NoError(t, os.Rename(path+".saved", path))
	}
}

func TestPreparationCoordinationFailureReleasesTerminalOwners(t *testing.T) {
	s, _ := testStore(t)
	m, _ := bundleFixture(t, "terminal-owners")
	failed := errors.New("source failed")
	var restore func()
	err := s.prepare(context.Background(), m, func() (io.ReadCloser, error) {
		restore = blockCoordination(t, s)
		return nil, failed
	})
	restore()
	require.ErrorIs(t, err, failed)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.DemandGenerations, "finished preparation must not retain local demand")
	require.Zero(t, status.ReservedBytes, "finished preparation must not retain live staging")
	require.NotEmpty(t, status.CleanupError)
	require.NoError(t, s.Sweep())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Empty(t, status.CleanupError)
	for _, dir := range []string{"claims", "staging", "trash"} {
		entries, err := s.childDirectories(dir)
		require.NoError(t, err)
		require.Empty(t, entries)
	}
	// The same generation can reserve again instead of remaining ErrPreparing.
	calls := 0
	err = s.prepare(context.Background(), m, func() (io.ReadCloser, error) { calls++; return nil, failed })
	require.ErrorIs(t, err, failed)
	require.Equal(t, 1, calls)
}

func TestOlderSweepPreservesPreparationCleanupHealth(t *testing.T) {
	s, _ := testStore(t)
	m, _ := bundleFixture(t, "preparation-health")
	var version uint64
	require.NoError(t, s.withLock(func() error { version = s.cleanupErrorVersion; return nil }))
	var restore func()
	err := s.prepare(context.Background(), m, func() (io.ReadCloser, error) {
		restore = blockCoordination(t, s)
		return nil, errors.New("source failed")
	})
	restore()
	require.Error(t, err)
	require.NoError(t, s.flushTrash())
	s.finishSweep(version, nil)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.NotEmpty(t, status.CleanupError, "older sweep cannot clear finalizer failure")
}

func TestFailedCommitCleanupReleasesClaimWithoutCoordination(t *testing.T) {
	s, _ := testStore(t)
	m := importFixture(t, s, "failed-commit")
	claim, err := s.ReserveDemand(m.Archive.SHA256)
	require.NoError(t, err)
	restore := blockCoordination(t, s)
	require.Error(t, claim.Commit())
	require.Error(t, claim.Close())
	restore()
	// The caller may discard the failed candidate. The dead record is the retry owner.
	require.NoError(t, s.Sweep())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.PendingClaims)
	require.Zero(t, status.DemandGenerations)
	require.Error(t, claim.Commit(), "a terminal claim cannot become accepted again")
}

// Native unlock failure is not safely reproducible. Fail the first Close while
// retaining the real flock, then let the existing maintenance owner retry it.
type retryOwnerClose struct {
	io.Closer
	fail bool
}

func (c *retryOwnerClose) Close() error {
	if c.fail {
		return errors.New("owner unlock failed")
	}
	return c.Closer.Close()
}

func TestTerminalOwnerCloseFailureRetriedBySweep(t *testing.T) {
	for _, kind := range []string{"claim", "staging", "use-drained", "use-unknown"} {
		t.Run(kind, func(t *testing.T) {
			s, _ := testStore(t)
			m := importFixture(t, s, "release-"+kind)
			var closer *retryOwnerClose
			switch kind {
			case "claim":
				claim, err := s.ReserveDemand(m.Archive.SHA256)
				require.NoError(t, err)
				closer = &retryOwnerClose{Closer: claim.lock, fail: true}
				claim.lock = closer
				require.Error(t, claim.Close())
				require.Error(t, claim.Commit(), "terminal close forbids subsequent acceptance")
			case "staging":
				other, _ := bundleFixture(t, "staging-release")
				name, owner, err := s.reserve(other)
				require.NoError(t, err)
				require.NotNil(t, owner)
				closer = &retryOwnerClose{Closer: owner, fail: true}
				s.finishPreparation(name, closer)
			default:
				use, err := s.BeginUse(m)
				require.NoError(t, err)
				closer = &retryOwnerClose{Closer: use.lock, fail: true}
				use.lock = closer
				require.Error(t, use.Finish(kind == "use-drained"))
			}
			require.ErrorContains(t, s.Sweep(), "owner unlock failed")
			status, err := s.Inspect()
			require.NoError(t, err)
			require.Contains(t, status.CleanupError, "owner unlock failed")
			closer.fail = false
			require.NoError(t, s.Sweep())
			status, err = s.Inspect()
			require.NoError(t, err)
			require.Empty(t, status.CleanupError)
			require.Zero(t, status.PendingClaims)
			require.Zero(t, status.ReservedBytes)
			if kind == "use-unknown" {
				require.Equal(t, 1, status.UncertainPins, "local release is not process-tree completion")
			} else {
				require.Zero(t, status.ExecutionPins)
			}
			require.Empty(t, s.releasingOwners)
		})
	}
}

func TestStoreCloseRetriesTerminalOwnerRelease(t *testing.T) {
	s, _ := testStore(t)
	m, _ := bundleFixture(t, "store-close-release")
	claim, err := s.ReserveDemand(m.Archive.SHA256)
	require.NoError(t, err)
	closer := &retryOwnerClose{Closer: claim.lock, fail: true}
	claim.lock = closer
	require.Error(t, claim.Close())
	require.ErrorContains(t, s.Close(), "owner unlock failed")
	require.Len(t, s.releasingOwners, 1)
	closer.fail = false
	require.NoError(t, s.Close())
	require.Empty(t, s.releasingOwners)
}

func TestBeginUseConstructorCleanupFailureReportsHealth(t *testing.T) {
	s, _ := testStore(t)
	m := importFixture(t, s, "constructor-health")
	for _, dir := range []string{"uses", "trash"} {
		path := filepath.Join(s.path, dir)
		require.NoError(t, os.Rename(path, path+".saved"))
		require.NoError(t, os.WriteFile(path, nil, 0600))
	}
	use, err := s.BeginUse(m)
	require.Error(t, err)
	require.Nil(t, use)
	for _, dir := range []string{"uses", "trash"} {
		path := filepath.Join(s.path, dir)
		require.NoError(t, os.Rename(path, path+".blocked"))
		require.NoError(t, os.Rename(path+".saved", path))
	}
	status, err := s.Inspect()
	require.NoError(t, err)
	require.NotEmpty(t, status.CleanupError)
	require.NoError(t, s.Sweep())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Empty(t, status.CleanupError)
	require.Zero(t, status.ExecutionPins)
	require.Empty(t, s.completedUses)
}

func TestOlderSweepPreservesNewerSweepFailure(t *testing.T) {
	s, _ := testStore(t)
	var version uint64
	require.NoError(t, s.withLock(func() error { version = s.cleanupErrorVersion; return nil }))
	restore := blockCoordination(t, s)
	require.Error(t, s.Sweep())
	restore()
	s.finishSweep(version, nil)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.NotEmpty(t, status.CleanupError)
	require.NoError(t, s.Sweep())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Empty(t, status.CleanupError)
}
