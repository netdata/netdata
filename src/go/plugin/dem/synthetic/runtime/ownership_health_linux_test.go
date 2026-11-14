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

func TestFinishFailureAppearsInStorageHealthUntilSweep(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store")
	s, err := Open(DefaultConfig(path))
	require.NoError(t, err)
	defer s.Close()
	manifest := importFixture(t, s, "finish-health")
	use, err := s.beginUse(manifest)
	require.NoError(t, err)
	trash := filepath.Join(path, "trash")
	require.NoError(t, os.Rename(trash, trash+".saved"))
	require.NoError(t, os.WriteFile(trash, []byte("blocked"), 0600))
	finishErr := use.Finish(true)
	require.NoError(t, os.Rename(trash, trash+".failed"))
	require.NoError(t, os.Rename(trash+".saved", trash))
	require.Error(t, finishErr)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.NotEmpty(t, status.CleanupError)
	require.NoError(t, s.Sweep())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Empty(t, status.CleanupError)
	require.Zero(t, status.ExecutionPins)
}

func TestOlderSweepDoesNotClearConcurrentFinishFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store")
	s, err := Open(DefaultConfig(path))
	require.NoError(t, err)
	defer s.Close()
	manifest := importFixture(t, s, "finish-during-sweep")
	use, err := s.beginUse(manifest)
	require.NoError(t, err)
	// Run the inventory phase, then interleave the real failed Finish before the
	// older sweep publishes its result after deleting trash.
	var version uint64
	require.NoError(t, s.withLock(func() error {
		version = s.ledger.cleanupErrorVersion
		_, err := s.inventoryLocked(true)
		return err
	}))
	trash := filepath.Join(path, "trash")
	require.NoError(t, os.Rename(trash, trash+".saved"))
	require.NoError(t, os.WriteFile(trash, []byte("blocked"), 0600))
	finishErr := use.Finish(true)
	require.NoError(t, os.Rename(trash, trash+".failed"))
	require.NoError(t, os.Rename(trash+".saved", trash))
	require.Error(t, finishErr)
	err = s.flushTrash()
	require.NoError(t, err)
	s.ledger.finishSweep(version, err)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.NotEmpty(t, status.CleanupError)
	require.NoError(t, s.Sweep())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Empty(t, status.CleanupError)
	require.Zero(t, status.ExecutionPins)
}

func TestFinishCoordinationFailureRetainsDrainedProof(t *testing.T) {
	s, _ := testStore(t)
	manifest := importFixture(t, s, "finish-coordination")
	use, err := s.beginUse(manifest)
	require.NoError(t, err)
	coordination := filepath.Join(s.path, "coordination.lock")
	require.NoError(t, os.Rename(coordination, coordination+".saved"))
	require.NoError(t, os.Mkdir(coordination, 0700))
	finishErr := use.Finish(true) // No child was launched, so no receipt exists.
	require.NoError(t, os.Rename(coordination, coordination+".blocked"))
	require.NoError(t, os.Rename(coordination+".saved", coordination))
	require.Error(t, finishErr)
	// Production callers discard the handle after their one Finish call.
	require.NoError(t, s.Sweep())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.ExecutionPins)
	require.Empty(t, status.CleanupError)
}

func TestExecutionOwnerDamageAppearsInStorageHealthWithoutReleasingPin(t *testing.T) {
	s, _ := testStore(t)
	manifest := importFixture(t, s, "owner-health")
	use, err := s.beginUse(manifest)
	require.NoError(t, err)
	owner := filepath.Join(s.path, use.name, "owner.lock")
	require.NoError(t, os.Chmod(owner, 0644))
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.ExecutionPins)
	require.Equal(t, 1, status.UncertainPins)
	require.Contains(t, status.CleanupError, "invalid runtime operation owner")
	require.NoError(t, os.Chmod(owner, 0600))
	require.NoError(t, use.Finish(true))
}

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
	require.Zero(t, status.ProcessGenerations, "finished preparation must not retain process ownership")
	require.Zero(t, status.ReservedBytes, "finished preparation must not retain live staging")
	require.NotEmpty(t, status.CleanupError)
	require.NoError(t, s.Sweep())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Empty(t, status.CleanupError)
	for _, dir := range []string{"claims", "staging", "trash"} {
		entries, err := s.ledger.childDirectories(dir)
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
	require.NoError(t, s.withLock(func() error { version = s.ledger.cleanupErrorVersion; return nil }))
	var restore func()
	err := s.prepare(context.Background(), m, func() (io.ReadCloser, error) {
		restore = blockCoordination(t, s)
		return nil, errors.New("source failed")
	})
	restore()
	require.Error(t, err)
	require.NoError(t, s.flushTrash())
	s.ledger.finishSweep(version, nil)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.NotEmpty(t, status.CleanupError, "older sweep cannot clear finalizer failure")
}

func TestTerminalCleanupReleasesClaimWithoutCoordination(t *testing.T) {
	s, _ := testStore(t)
	m := importFixture(t, s, "failed-commit")
	claim, err := s.testProcessClaim(m.Archive.SHA256)
	require.NoError(t, err)
	restore := blockCoordination(t, s)
	require.Error(t, claim.Close())
	restore()
	// The caller may discard the failed candidate. The dead record is the retry owner.
	require.NoError(t, s.Sweep())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.ProcessClaims)
	require.Zero(t, status.ProcessGenerations)
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
				claim, err := s.testProcessClaim(m.Archive.SHA256)
				require.NoError(t, err)
				closer = &retryOwnerClose{Closer: claim.lock, fail: true}
				claim.lock = closer
				require.Error(t, claim.Close())
			case "staging":
				other, _ := bundleFixture(t, "staging-release")
				name, owner, err := s.reserve(other)
				require.NoError(t, err)
				require.NotNil(t, owner)
				closer = &retryOwnerClose{Closer: owner, fail: true}
				s.finishPreparation(name, closer)
			default:
				use, err := s.beginUse(m)
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
			require.Zero(t, status.ProcessClaims)
			require.Zero(t, status.ReservedBytes)
			if kind == "use-unknown" {
				require.Equal(t, 1, status.UncertainPins, "local release is not process-tree completion")
			} else {
				require.Zero(t, status.ExecutionPins)
			}
			require.Empty(t, s.ledger.releasingOwners)
		})
	}
}

func TestStoreCloseRetriesTerminalOwnerRelease(t *testing.T) {
	s, _ := testStore(t)
	m, _ := bundleFixture(t, "store-close-release")
	claim, err := s.testProcessClaim(m.Archive.SHA256)
	require.NoError(t, err)
	closer := &retryOwnerClose{Closer: claim.lock, fail: true}
	claim.lock = closer
	require.Error(t, claim.Close())
	require.ErrorContains(t, s.Close(), "owner unlock failed")
	require.Len(t, s.ledger.releasingOwners, 1)
	closer.fail = false
	require.NoError(t, s.Close())
	require.Empty(t, s.ledger.releasingOwners)
}

func TestBeginUseConstructorCleanupFailureReportsHealth(t *testing.T) {
	s, _ := testStore(t)
	m := importFixture(t, s, "constructor-health")
	for _, dir := range []string{"uses", "trash"} {
		path := filepath.Join(s.path, dir)
		require.NoError(t, os.Rename(path, path+".saved"))
		require.NoError(t, os.WriteFile(path, nil, 0600))
	}
	use, err := s.beginUse(m)
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
	require.Empty(t, s.ledger.completedUses)
}

func TestOlderSweepPreservesNewerSweepFailure(t *testing.T) {
	s, _ := testStore(t)
	var version uint64
	require.NoError(t, s.withLock(func() error { version = s.ledger.cleanupErrorVersion; return nil }))
	restore := blockCoordination(t, s)
	require.Error(t, s.Sweep())
	restore()
	s.ledger.finishSweep(version, nil)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.NotEmpty(t, status.CleanupError)
	require.NoError(t, s.Sweep())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Empty(t, status.CleanupError)
}
