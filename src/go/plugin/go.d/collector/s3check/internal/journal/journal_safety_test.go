// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSavePreservesForeignTemps(t *testing.T) {
	for _, kind := range []string{"orphan", "symlink", "dangling symlink"} {
		t.Run(kind, func(t *testing.T) {
			j, err := New(t.TempDir(), "agent", "job", testFingerprint)
			require.NoError(t, err)
			ok, err := j.TryLock()
			require.NoError(t, err)
			require.True(t, ok)
			t.Cleanup(j.Unlock)
			target := filepath.Join(t.TempDir(), "target")
			if kind != "dangling symlink" {
				require.NoError(t, os.WriteFile(target, []byte("sentinel"), 0600))
			}
			temp := j.Path() + ".tmp"
			if kind == "orphan" {
				require.NoError(t, os.WriteFile(temp, []byte("orphan"), 0600))
			} else if err := os.Symlink(target, temp); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			require.NoError(t, j.Save(testState{
				Pending: []string{"new"},
			}))
			var state testState
			found, err := j.Load(&state)
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, []string{"new"}, state.Pending)
			require.NoError(t, j.Clear())
			_, err = os.Lstat(temp)
			assert.NoError(t, err)
			if kind == "dangling symlink" {
				_, err = os.Lstat(target)
				assert.ErrorIs(t, err, os.ErrNotExist)
			} else {
				data, err := os.ReadFile(target)
				require.NoError(t, err)
				assert.Equal(t, "sentinel", string(data))
			}
		})
	}
}

func TestLoadRejectsSymlink(t *testing.T) {
	for _, dangling := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing", true: "dangling"}[dangling], func(t *testing.T) {
			j, err := New(t.TempDir(), "agent", "job", testFingerprint)
			require.NoError(t, err)
			target := filepath.Join(t.TempDir(), "target")
			if !dangling {
				ok, err := j.TryLock()
				require.NoError(t, err)
				require.True(t, ok)
				t.Cleanup(j.Unlock)
				require.NoError(t, j.Save(testState{
					Pending: []string{"existing"},
				}))
				require.NoError(t, os.Rename(j.Path(), target))
			}
			if err := os.Symlink(target, j.Path()); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			var state testState
			found, err := j.Load(&state)
			assert.Error(t, err)
			assert.False(t, found)
		})
	}
}

func TestTryLockRejectsSymlinkDirectory(t *testing.T) {
	target := t.TempDir()
	require.NoError(t, os.Chmod(target, 0755))
	before, err := os.Stat(target)
	require.NoError(t, err)
	root := filepath.Join(t.TempDir(), "journal")
	if err := os.Symlink(target, root); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	j, err := New(root, "agent", "job", testFingerprint)
	require.NoError(t, err)
	ok, err := j.TryLock()
	t.Cleanup(j.Unlock)
	assert.Error(t, err)
	assert.False(t, ok)
	after, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, before.Mode(), after.Mode())
	entries, err := os.ReadDir(target)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestTryLockRejectsSymlinkLock(t *testing.T) {
	for _, dangling := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing", true: "dangling"}[dangling], func(t *testing.T) {
			root := t.TempDir()
			j, err := New(root, "agent", "job", testFingerprint)
			require.NoError(t, err)
			target := filepath.Join(t.TempDir(), "target")
			if !dangling {
				require.NoError(t, os.WriteFile(target, []byte("sentinel"), 0600))
			}
			if err := os.Symlink(target, filepath.Join(root, j.OwnerID()+".collector.lock")); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			ok, err := j.TryLock()
			t.Cleanup(j.Unlock)
			assert.Error(t, err)
			assert.False(t, ok)
			if dangling {
				_, err = os.Lstat(target)
				assert.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

func TestTakeoverRejectsUnsafeStateAndReleasesLock(t *testing.T) {
	root := t.TempDir()
	j, err := New(root, "agent", "job", testFingerprint)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(j.Path(), 0700))
	var state testState
	locked, found, err := j.TryTakeover(&state)
	assert.Error(t, err)
	assert.False(t, locked)
	assert.False(t, found)
	contender, err := New(root, "agent", "job", testFingerprint)
	require.NoError(t, err)
	locked, err = contender.TryLock()
	require.NoError(t, err)
	assert.True(t, locked)
	t.Cleanup(contender.Unlock)
}

func TestSavePrePublicationFailureKeepsStateAndAllowsRetry(t *testing.T) {
	j, err := New(t.TempDir(), "agent", "job", testFingerprint)
	require.NoError(t, err)
	locked, err := j.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	t.Cleanup(j.Unlock)
	require.NoError(t, j.Save(testState{
		Pending: []string{"old"},
	}))
	backup := j.Path() + ".backup"
	require.NoError(t, os.Rename(j.Path(), backup))
	require.NoError(t, os.Mkdir(j.Path(), 0700))
	err = j.Save(testState{
		Pending: []string{"new"},
	})
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrPublicationUncertain)
	assert.NoError(t, j.MutationError())
	// Restore the deliberately obstructed namespace, then retry with the same owner.
	require.NoError(t, os.Remove(j.Path()))
	require.NoError(t, os.Rename(backup, j.Path()))
	var state testState
	found, err := j.Load(&state)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []string{"old"}, state.Pending)
	require.NoError(t, j.Save(testState{
		Pending: []string{"new"},
	}))
}
