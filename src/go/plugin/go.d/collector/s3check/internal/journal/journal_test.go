// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFingerprint = "96e160c527c1903398e8bcf47ca8f165ab2e70c16ba90a9a66c06330f705f151"

type testState struct {
	Pending []string `json:"pending"`
}

func TestNewValidatesDurableIdentity(t *testing.T) {
	tests := map[string]struct {
		agentID     string
		jobName     string
		fingerprint string
	}{
		"empty agent identity":     {jobName: "job", fingerprint: testFingerprint},
		"untrimmed agent identity": {agentID: " agent ", jobName: "job", fingerprint: testFingerprint},
		"empty job name":           {agentID: "agent", fingerprint: testFingerprint},
		"untrimmed job name":       {agentID: "agent", jobName: " job ", fingerprint: testFingerprint},
		"invalid fingerprint":      {agentID: "agent", jobName: "job", fingerprint: "invalid"},
		"untrimmed fingerprint":    {agentID: "agent", jobName: "job", fingerprint: " " + testFingerprint},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := New(t.TempDir(), tc.agentID, tc.jobName, tc.fingerprint)
			assert.Error(t, err)
		})
	}
}

func TestOwnerIdentityIsStableAndScoped(t *testing.T) {
	first, err := New(t.TempDir(), "agent-a", "job-a", testFingerprint)
	require.NoError(t, err)
	second, err := New(t.TempDir(), "agent-a", "job-a", testFingerprint)
	require.NoError(t, err)
	otherJob, err := New(t.TempDir(), "agent-a", "job-b", testFingerprint)
	require.NoError(t, err)
	otherAgent, err := New(t.TempDir(), "agent-b", "job-a", testFingerprint)
	require.NoError(t, err)

	assert.Equal(t, first.OwnerID(), second.OwnerID())
	assert.NotEqual(t, first.OwnerID(), otherJob.OwnerID())
	assert.NotEqual(t, first.OwnerID(), otherAgent.OwnerID())
	assert.Regexp(t, "^[a-f0-9]{64}$", first.OwnerID())
	assert.NotContains(t, first.Path(), "agent-a")
	assert.NotContains(t, first.Path(), "job-a")
}

func TestLoadIsReadOnly(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing", "journal")
	j, err := New(root, "agent", "job", testFingerprint)
	require.NoError(t, err)

	var state testState
	found, err := j.Load(&state)
	require.NoError(t, err)
	assert.False(t, found)
	_, statErr := os.Stat(root)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestTryLockDurablyCreatesJournalDirectory(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "s3check")
	originalSyncDirectory := syncDirectory
	var synced []string
	syncDirectory = func(dir string) error {
		synced = append(synced, filepath.Clean(dir))
		return nil
	}
	t.Cleanup(func() { syncDirectory = originalSyncDirectory })

	j, err := New(root, "agent", "job", testFingerprint)
	require.NoError(t, err)
	locked, err := j.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	t.Cleanup(j.Unlock)

	assert.Contains(t, synced, filepath.Clean(root))
	assert.Contains(t, synced, filepath.Clean(parent))
}

func TestConcurrentOwnerSyncsParentBeforeAcquiringLock(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "s3check")
	first, err := New(root, "agent", "job-a", testFingerprint)
	require.NoError(t, err)
	second, err := New(root, "agent", "job-b", testFingerprint)
	require.NoError(t, err)

	originalSyncDirectory := syncDirectory
	var blockFirstRootSync sync.Once
	firstRootSyncStarted := make(chan struct{})
	releaseFirstRootSync := make(chan struct{})
	var syncedMu sync.Mutex
	var synced []string
	syncDirectory = func(dir string) error {
		clean := filepath.Clean(dir)
		block := false
		if clean == filepath.Clean(root) {
			blockFirstRootSync.Do(func() {
				block = true
				close(firstRootSyncStarted)
			})
		}
		syncedMu.Lock()
		synced = append(synced, clean)
		syncedMu.Unlock()
		if block {
			<-releaseFirstRootSync
		}
		return nil
	}
	t.Cleanup(func() { syncDirectory = originalSyncDirectory })

	type lockResult struct {
		locked bool
		err    error
	}
	firstDone := make(chan lockResult, 1)
	go func() {
		locked, lockErr := first.TryLock()
		firstDone <- lockResult{
			locked: locked,
			err:    lockErr,
		}
	}()
	<-firstRootSyncStarted

	secondLocked, secondErr := second.TryLock()
	// The first owner is still blocked before locking. Any parent sync observed
	// here must therefore come from the concurrent owner's own lock preparation.
	syncedMu.Lock()
	syncedBeforeFirstCompletes := append([]string(nil), synced...)
	syncedMu.Unlock()
	close(releaseFirstRootSync)
	firstResult := <-firstDone

	require.NoError(t, firstResult.err)
	require.True(t, firstResult.locked)
	require.NoError(t, secondErr)
	require.True(t, secondLocked)
	t.Cleanup(first.Unlock)
	t.Cleanup(second.Unlock)
	assert.Contains(t, syncedBeforeFirstCompletes, filepath.Clean(parent))
}

func TestMutationRequiresLifetimeLock(t *testing.T) {
	j, err := New(t.TempDir(), "agent", "job", testFingerprint)
	require.NoError(t, err)

	err = j.Save(testState{
		Pending: []string{"key"},
	})
	assert.ErrorIs(t, err, ErrNotLocked)
	err = j.Clear()
	assert.ErrorIs(t, err, ErrNotLocked)

	locked, err := j.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	t.Cleanup(j.Unlock)

	require.NoError(t, j.Save(testState{
		Pending: []string{"key"},
	}))
	var got testState
	found, err := j.Load(&got)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []string{"key"}, got.Pending)
	require.NoError(t, j.Clear())
	found, err = j.Load(&got)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestSavePublicationFailureRejectsFurtherMutation(t *testing.T) {
	j, err := New(t.TempDir(), "agent", "job", testFingerprint)
	require.NoError(t, err)
	locked, err := j.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	t.Cleanup(j.Unlock)

	originalSyncDirectory := syncDirectory
	syncDirectory = func(string) error {
		return errors.New("injected directory sync failure")
	}
	t.Cleanup(func() { syncDirectory = originalSyncDirectory })

	err = j.Save(testState{
		Pending: []string{"published"},
	})
	assert.ErrorIs(t, err, ErrPublicationUncertain)
	assert.ErrorContains(t, err, "journal publication is uncertain")

	var got testState
	found, err := j.Load(&got)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []string{"published"}, got.Pending)

	err = j.Save(testState{
		Pending: []string{"must-not-replace"},
	})
	assert.ErrorIs(t, err, ErrPublicationUncertain)
	assert.ErrorContains(t, err, "journal publication is uncertain")
	found, err = j.Load(&got)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []string{"published"}, got.Pending)
}

func TestClearPublicationFailureRejectsFurtherMutation(t *testing.T) {
	j, err := New(t.TempDir(), "agent", "job", testFingerprint)
	require.NoError(t, err)
	locked, err := j.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	t.Cleanup(j.Unlock)
	require.NoError(t, j.Save(testState{
		Pending: []string{"published"},
	}))

	originalSyncDirectory := syncDirectory
	syncDirectory = func(string) error {
		return errors.New("injected directory sync failure")
	}
	t.Cleanup(func() { syncDirectory = originalSyncDirectory })

	err = j.Clear()
	assert.ErrorIs(t, err, ErrPublicationUncertain)
	assert.ErrorContains(t, err, "journal publication is uncertain")

	var got testState
	found, err := j.Load(&got)
	require.NoError(t, err)
	assert.False(t, found)

	err = j.Save(testState{
		Pending: []string{"must-not-republish"},
	})
	assert.ErrorIs(t, err, ErrPublicationUncertain)
	assert.ErrorContains(t, err, "journal publication is uncertain")
	found, err = j.Load(&got)
	require.NoError(t, err)
	assert.False(t, found)
}

func TestSameOwnerSerializesButDifferentOwnersDoNot(t *testing.T) {
	root := t.TempDir()
	first, err := New(root, "agent", "job-a", testFingerprint)
	require.NoError(t, err)
	same, err := New(root, "agent", "job-a", testFingerprint)
	require.NoError(t, err)
	other, err := New(root, "agent", "job-b", testFingerprint)
	require.NoError(t, err)

	locked, err := first.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	t.Cleanup(first.Unlock)

	locked, err = same.TryLock()
	require.NoError(t, err)
	assert.False(t, locked)

	locked, err = other.TryLock()
	require.NoError(t, err)
	assert.True(t, locked)
	other.Unlock()
}

func TestTryTakeoverLoadsStateAfterWaitingOwnerReleases(t *testing.T) {
	root := t.TempDir()
	incumbent, err := New(root, "agent", "job", testFingerprint)
	require.NoError(t, err)
	successor, err := New(root, "agent", "job", testFingerprint)
	require.NoError(t, err)

	locked, err := incumbent.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	require.NoError(t, incumbent.Save(testState{
		Pending: []string{"old"},
	}))

	var stale testState
	found, err := successor.Load(&stale)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []string{"old"}, stale.Pending)

	require.NoError(t, incumbent.Save(testState{
		Pending: []string{"authoritative"},
	}))
	incumbent.Unlock()

	var authoritative testState
	locked, found, err = successor.TryTakeover(&authoritative)
	require.NoError(t, err)
	require.True(t, locked)
	require.True(t, found)
	t.Cleanup(successor.Unlock)
	assert.Equal(t, []string{"authoritative"}, authoritative.Pending)
}

func TestFingerprintMismatchRequiresOldConfiguration(t *testing.T) {
	root := t.TempDir()
	original, err := New(root, "agent", "job", testFingerprint)
	require.NoError(t, err)
	locked, err := original.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	require.NoError(t, original.Save(testState{
		Pending: []string{"key"},
	}))
	original.Unlock()

	changedFingerprint := "a6e160c527c1903398e8bcf47ca8f165ab2e70c16ba90a9a66c06330f705f151"
	changed, err := New(root, "agent", "job", changedFingerprint)
	require.NoError(t, err)
	var state testState
	found, err := changed.Load(&state)
	assert.False(t, found)
	assert.ErrorIs(t, err, ErrFingerprintMismatch)
}

func TestStateFileIsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not authoritative on Windows")
	}
	j, err := New(t.TempDir(), "agent", "job", testFingerprint)
	require.NoError(t, err)
	locked, err := j.TryLock()
	require.NoError(t, err)
	require.True(t, locked)
	t.Cleanup(j.Unlock)
	require.NoError(t, j.Save(testState{}))

	info, err := os.Stat(j.Path())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	info, err = os.Stat(filepath.Dir(j.Path()))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
}

func TestLoadRejectsOversizedState(t *testing.T) {
	j, err := New(t.TempDir(), "agent", "job", testFingerprint)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(j.Path()), 0o700))
	require.NoError(t, os.WriteFile(j.Path(), make([]byte, maxStateBytes+1), 0o600))

	var state testState
	found, err := j.Load(&state)
	assert.False(t, found)
	assert.True(t, errors.Is(err, ErrStateTooLarge))
}
