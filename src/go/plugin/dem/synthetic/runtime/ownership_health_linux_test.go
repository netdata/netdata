// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
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
		version = s.cleanupErrorVersion
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
	s.finishSweep(version, err)
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
