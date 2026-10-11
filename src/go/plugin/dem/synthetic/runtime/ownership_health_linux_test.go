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
	use, err := s.BeginUse(manifest)
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
	use, err := s.BeginUse(manifest)
	require.NoError(t, err)
	// Run the inventory phase, then interleave the real failed Finish before the
	// older sweep publishes its result after deleting trash.
	var version uint64
	require.NoError(t, s.withLock(func() error {
		version = s.finishErrorVersion
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
