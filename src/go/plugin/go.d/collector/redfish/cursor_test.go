// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/filelock"
)

const testCursorSourceKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

const (
	testCursorSourceKey2 = "1123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	testCursorSourceKey3 = "2123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func TestCursorEncodingRejectsCorruption(t *testing.T) {
	payload := cursorPayload{
		EndpointKey:  "endpoint",
		OriginDigest: "origin",
		Sources: map[string]logSourceCursor{
			testCursorSourceKey: {Mode: "full", Initialized: true},
		},
	}
	encoded, err := encodeCursor(payload)
	require.NoError(t, err)
	decoded, err := decodeCursor(encoded)
	require.NoError(t, err)
	require.Equal(t, payload.EndpointKey, decoded.EndpointKey)

	bodyCorrupted := append([]byte(nil), encoded...)
	bodyCorrupted[len(bodyCorrupted)-1] ^= 0xff
	_, err = decodeCursor(bodyCorrupted)
	require.ErrorContains(t, err, "checksum")

	metadataCorrupted := append([]byte(nil), encoded...)
	metadataCorrupted[cursorRetentionOffset] ^= 0xff
	_, err = decodeCursor(metadataCorrupted)
	require.ErrorContains(t, err, "metadata checksum")
}

func TestCursorCheckpointRoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(100, 0).UTC()
	first := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	first.root = dir
	first.path = filepath.Join(dir, "endpoint.cursor")
	first.claimed = true
	first.loaded = true
	expected := logSourceCursor{
		Mode: "client_context", ClientContext: "0123456789abcdef0123456789abcdef",
		ContextDirty:        true,
		ExactComplete:       true,
		BoundaryKeys:        []string{testCursorSourceKey, testCursorSourceKey},
		ReconcileSourceKeys: []string{testCursorSourceKey, testCursorSourceKey},
	}
	require.NoError(t, first.CheckpointSource(testCursorSourceKey, expected, now))

	second := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	second.root = dir
	second.path = first.path
	require.NoError(t, second.loadLocked())
	actual := second.Source(testCursorSourceKey)
	require.Equal(t, expected.Mode, actual.Mode)
	require.Equal(t, expected.ClientContext, actual.ClientContext)
	require.True(t, actual.ContextDirty)
	require.Equal(t, []string{testCursorSourceKey}, actual.BoundaryKeys)
	require.Equal(t, []string{testCursorSourceKey}, actual.ReconcileSourceKeys)
	require.Equal(t, now.UnixMicro(), actual.LastActiveUsec)
}

func TestCursorCheckpointFailureRetainsProgressForRetry(t *testing.T) {
	now := time.Unix(200, 0).UTC()
	coordinator := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	coordinator.root = filepath.Join(t.TempDir(), "missing")
	coordinator.path = filepath.Join(coordinator.root, "endpoint.cursor")
	coordinator.claimed = true
	coordinator.loaded = true
	cursor := logSourceCursor{Mode: "full", Continuation: "/redfish/v1/next"}
	require.Error(t, coordinator.CheckpointSource(testCursorSourceKey, cursor, now))
	require.Equal(t, cursor.Continuation, coordinator.Source(testCursorSourceKey).Continuation)

	coordinator.root = t.TempDir()
	coordinator.path = filepath.Join(coordinator.root, "endpoint.cursor")
	require.NoError(t, coordinator.Persist(now.Add(time.Second)))
	_, err := os.Stat(coordinator.path)
	require.NoError(t, err)
}

func TestCursorRejectsInvalidReplacementAndPreservesLastValidState(t *testing.T) {
	now := time.Unix(300, 0).UTC()
	coordinator := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	valid := logSourceCursor{
		Mode:              "full",
		ReconcileStarted:  true,
		ReconcileExpected: 2,
		ReconcileFetched:  1,
	}
	require.NoError(t, coordinator.UpdateSource(testCursorSourceKey, valid, now))

	invalid := valid
	invalid.ReconcileFetched = 3
	require.ErrorContains(
		t,
		coordinator.UpdateSource(testCursorSourceKey, invalid, now.Add(time.Second)),
		"reconciliation counters",
	)
	actual := coordinator.Source(testCursorSourceKey)
	require.Equal(t, 1, actual.ReconcileFetched)
	require.Equal(t, 2, actual.ReconcileExpected)
}

func TestCursorPersistReportsPendingRetryWithoutBlockingUpdates(t *testing.T) {
	now := time.Unix(400, 0).UTC()
	coordinator := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	coordinator.root = filepath.Join(t.TempDir(), "missing")
	coordinator.path = filepath.Join(coordinator.root, "endpoint.cursor")
	coordinator.claimed = true
	coordinator.loaded = true
	require.NoError(t, coordinator.UpdateSource(
		testCursorSourceKey,
		logSourceCursor{Mode: "full"},
		now,
	))
	require.Error(t, coordinator.Persist(now))
	require.ErrorContains(t, coordinator.Persist(now.Add(500*time.Millisecond)), "pending retry")

	require.NoError(t, coordinator.UpdateSource(
		testCursorSourceKey,
		logSourceCursor{Mode: "recovery"},
		now.Add(time.Second),
	))
	require.Equal(t, "recovery", coordinator.Source(testCursorSourceKey).Mode)
}

func TestCursorPersistPrunesInactiveSourcesDurably(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(10_000, 0).UTC()
	coordinator := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	coordinator.root = dir
	coordinator.path = filepath.Join(dir, "endpoint.cursor")
	coordinator.claimed = true
	coordinator.loaded = true
	coordinator.payload.Sources = map[string]logSourceCursor{
		testCursorSourceKey:  {Mode: "full", LastActiveUsec: now.Add(-2 * time.Hour).UnixMicro()},
		testCursorSourceKey3: {Mode: "full", LastActiveUsec: now.Add(-30 * time.Minute).UnixMicro()},
	}

	require.NoError(t, coordinator.Persist(now))
	require.NotContains(t, coordinator.payload.Sources, testCursorSourceKey)
	require.Contains(t, coordinator.payload.Sources, testCursorSourceKey3)

	reloaded := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	reloaded.root = dir
	reloaded.path = coordinator.path
	require.NoError(t, reloaded.loadLocked())
	require.NotContains(t, reloaded.payload.Sources, testCursorSourceKey)
	require.Contains(t, reloaded.payload.Sources, testCursorSourceKey3)
}

func TestCursorTouchPreservesActiveSourceDuringPruning(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(15_000, 0).UTC()
	coordinator := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	coordinator.root = dir
	coordinator.path = filepath.Join(dir, "endpoint.cursor")
	coordinator.claimed = true
	coordinator.loaded = true
	coordinator.payload.Sources = map[string]logSourceCursor{
		testCursorSourceKey:  {Mode: "full", LastActiveUsec: now.Add(-2 * time.Hour).UnixMicro()},
		testCursorSourceKey2: {Mode: "full", LastActiveUsec: now.Add(-2 * time.Hour).UnixMicro()},
	}
	coordinator.resetPayloadSizeLocked()

	require.NoError(t, coordinator.TouchSources([]string{testCursorSourceKey2}, now))
	require.NoError(t, coordinator.Persist(now))
	require.NotContains(t, coordinator.payload.Sources, testCursorSourceKey)
	require.Equal(t, now.UnixMicro(), coordinator.Source(testCursorSourceKey2).LastActiveUsec)
}

func TestCursorZeroOrphanRetentionPreservesInactiveSources(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(40_000_000, 0).UTC()
	lastActive := now.Add(-365 * 24 * time.Hour)
	first := newCursorCoordinator("endpoint", "https://192.0.2.1:443", 0)
	first.root = dir
	first.path = filepath.Join(dir, "endpoint.cursor")
	first.claimed = true
	first.loaded = true
	require.NoError(t, first.UpdateSource(
		testCursorSourceKey,
		logSourceCursor{Mode: "full"},
		lastActive,
	))
	require.NoError(t, first.Persist(now))

	second := newCursorCoordinator("endpoint", "https://192.0.2.1:443", 0)
	second.root = dir
	second.path = first.path
	require.NoError(t, second.loadLocked())
	require.Equal(t, "full", second.Source(testCursorSourceKey).Mode)
	require.Equal(t, lastActive.UnixMicro(), second.Source(testCursorSourceKey).LastActiveUsec)
}

func TestCursorLoadRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation may require additional privileges on Windows")
	}

	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.WriteFile(target, []byte("not-a-cursor"), 0o600))
	link := filepath.Join(dir, "endpoint.cursor")
	require.NoError(t, os.Symlink(target, link))
	coordinator := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	coordinator.path = link
	require.ErrorContains(t, coordinator.loadLocked(), "bounded regular file")
}

func TestCursorDirectoryRejectsBroadPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows directory ACLs are not represented by POSIX permission bits")
	}
	dir := filepath.Join(t.TempDir(), "cursors")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, os.Chmod(dir, 0o755))
	require.ErrorContains(t, ensureCursorDirectory(dir), "permissions")
}

func TestCursorDuplicateOwnerCanClaimAfterIncumbentCloses(t *testing.T) {
	t.Setenv("NETDATA_LIB_DIR", t.TempDir())

	first := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	second := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	require.True(t, mustClaimCursor(t, first))
	require.False(t, mustClaimCursor(t, second))

	first.Close()
	require.True(t, mustClaimCursor(t, second))
	second.Close()
}

func mustClaimCursor(t *testing.T, coordinator *cursorCoordinator) bool {
	t.Helper()
	claimed, err := coordinator.Claim(context.Background())
	require.NoError(t, err)
	return claimed
}

func TestCursorCleanupRemovesOnlyExpiredUnlockedOrphans(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1_000, 0).UTC()
	current := testEndpointKey(1)
	expired := testEndpointKey(2)
	fresh := testEndpointKey(3)
	locked := testEndpointKey(4)
	for _, key := range []string{current, expired, fresh, locked} {
		path := filepath.Join(dir, key+".cursor")
		writeTestCursorFile(t, path, key, time.Hour)
	}
	expiredTemporary := filepath.Join(dir, "."+expired+".cursor-expired")
	lockedTemporary := filepath.Join(dir, "."+locked+".cursor-locked")
	for _, path := range []string{expiredTemporary, lockedTemporary} {
		require.NoError(t, os.WriteFile(path, []byte("temporary"), 0o600))
	}
	for _, key := range []string{expired, locked} {
		require.NoError(t, os.Chtimes(
			filepath.Join(dir, key+".cursor"),
			now.Add(-2*time.Hour),
			now.Add(-2*time.Hour),
		))
	}
	for _, path := range []string{expiredTemporary, lockedTemporary} {
		require.NoError(t, os.Chtimes(path, now.Add(-2*time.Hour), now.Add(-2*time.Hour)))
	}
	incumbent := filelock.New(dir)
	claimed, err := incumbent.Lock(locked + "-" + cursorLockName)
	require.NoError(t, err)
	require.True(t, claimed)
	t.Cleanup(incumbent.UnlockAll)

	coordinator := newCursorCoordinator(current, "https://192.0.2.1:443", time.Hour)
	coordinator.root = dir
	coordinator.path = filepath.Join(dir, current+".cursor")
	coordinator.cleanup = newCursorCleanupGate()
	coordinator.cleanupOrphansLocked(now)

	_, err = os.Stat(filepath.Join(dir, expired+".cursor"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(cursorEndpointLockPath(dir, expired))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(expiredTemporary)
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, key := range []string{current, fresh, locked} {
		_, err := os.Stat(filepath.Join(dir, key+".cursor"))
		require.NoError(t, err)
	}
	_, err = os.Stat(cursorEndpointLockPath(dir, fresh))
	require.NoError(t, err)
	_, err = os.Stat(lockedTemporary)
	require.NoError(t, err)
}

func TestCursorCleanupRetiresLockOnlyEndpoint(t *testing.T) {
	dir := t.TempDir()
	endpointKey := testEndpointKey(1)
	locker := filelock.New(dir)
	ok, err := locker.Lock(endpointKey + "-" + cursorLockName)
	require.NoError(t, err)
	require.True(t, ok)
	locker.UnlockAll()
	require.FileExists(t, cursorEndpointLockPath(dir, endpointKey))

	coordinator := newCursorCoordinator(testEndpointKey(2), "https://192.0.2.1:443", time.Hour)
	coordinator.root = dir
	coordinator.path = filepath.Join(dir, coordinator.endpointKey+".cursor")
	coordinator.cleanup = newCursorCleanupGate()
	coordinator.cleanupOrphansLocked(time.Unix(10_000, 0).UTC())

	_, err = os.Stat(cursorEndpointLockPath(dir, endpointKey))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCursorClaimAndRetirementShareNamespaceLock(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("NETDATA_LIB_DIR", stateDir)
	root := filepath.Join(stateDir, filepath.FromSlash(cursorDirectoryName))
	require.NoError(t, ensureCursorDirectory(root))

	namespace := filelock.New(root)
	ok, err := namespace.Lock(cursorNamespaceLock)
	require.NoError(t, err)
	require.True(t, ok)

	endpointKey := testEndpointKey(1)
	coordinator := newCursorCoordinator(endpointKey, "https://192.0.2.1:443", time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	claimed, err := coordinator.Claim(ctx)
	require.False(t, claimed)
	require.ErrorIs(t, err, context.Canceled)
	_, err = os.Stat(cursorEndpointLockPath(root, endpointKey))
	require.ErrorIs(t, err, os.ErrNotExist)
	namespace.UnlockAll()

	claimed, err = coordinator.Claim(context.Background())
	require.NoError(t, err)
	require.True(t, claimed)
	require.FileExists(t, cursorEndpointLockPath(root, endpointKey))

	cleaner := newCursorCoordinator(testEndpointKey(2), "https://192.0.2.2:443", time.Hour)
	cleaner.root = root
	cleaner.path = filepath.Join(root, cleaner.endpointKey+".cursor")
	cleaner.cleanup = newCursorCleanupGate()
	now := time.Unix(20_000, 0).UTC()
	cleaner.cleanupOrphansLocked(now)
	require.FileExists(t, cursorEndpointLockPath(root, endpointKey))

	coordinator.Close()
	cleaner.cleanupOrphansLocked(now.Add(cursorCleanupInterval))
	_, err = os.Stat(cursorEndpointLockPath(root, endpointKey))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCursorCleanupBoundsLockFilesAfterEndpointChurn(t *testing.T) {
	dir := t.TempDir()
	const endpoints = 4*cursorCleanupBatchSize - 1
	for i := range endpoints {
		endpointKey := testEndpointKey(i + 1)
		locker := filelock.New(dir)
		ok, err := locker.Lock(endpointKey + "-" + cursorLockName)
		require.NoError(t, err)
		require.True(t, ok)
		locker.UnlockAll()
	}

	coordinator := newCursorCoordinator(testEndpointKey(endpoints+1), "https://192.0.2.1:443", time.Hour)
	coordinator.root = dir
	coordinator.path = filepath.Join(dir, coordinator.endpointKey+".cursor")
	coordinator.cleanup = newCursorCleanupGate()
	now := time.Unix(30_000, 0).UTC()
	for i := range 4 {
		coordinator.cleanupOrphansLocked(now.Add(time.Duration(i) * cursorCleanupInterval))
	}

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, cursorNamespaceLock+cursorLockFileSuffix, entries[0].Name())
}

func TestCursorCleanupUsesEachSnapshotRetention(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(20_000, 0).UTC()
	current := testEndpointKey(1)
	expired := testEndpointKey(2)
	longLived := testEndpointKey(3)
	indefinite := testEndpointKey(4)
	for key, retention := range map[string]time.Duration{
		expired: time.Hour, longLived: 30 * 24 * time.Hour, indefinite: 0,
	} {
		path := filepath.Join(dir, key+".cursor")
		writeTestCursorFile(t, path, key, retention)
		require.NoError(t, os.Chtimes(path, now.Add(-2*time.Hour), now.Add(-2*time.Hour)))
	}

	coordinator := newCursorCoordinator(current, "https://192.0.2.1:443", time.Minute)
	coordinator.root = dir
	coordinator.path = filepath.Join(dir, current+".cursor")
	coordinator.cleanup = newCursorCleanupGate()
	coordinator.cleanupOrphansLocked(now)

	_, err := os.Stat(filepath.Join(dir, expired+".cursor"))
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, key := range []string{longLived, indefinite} {
		_, err := os.Stat(filepath.Join(dir, key+".cursor"))
		require.NoError(t, err)
	}
}

func TestCursorCleanupEventuallyProgressesPastFreshLockedAndTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(30_000, 0).UTC()
	locker := filelock.New(dir)
	t.Cleanup(locker.UnlockAll)

	for i := range 12 {
		path := filepath.Join(dir, "."+testEndpointKey(i+1)+".cursor-test")
		require.NoError(t, os.WriteFile(path, []byte("temporary"), 0o600))
	}
	for i := range 20 {
		key := testEndpointKey(i + 1)
		path := filepath.Join(dir, key+".cursor")
		writeTestCursorFile(t, path, key, time.Hour)
	}
	for i := 20; i < 30; i++ {
		key := testEndpointKey(i + 1)
		path := filepath.Join(dir, key+".cursor")
		writeTestCursorFile(t, path, key, time.Hour)
		require.NoError(t, os.Chtimes(path, now.Add(-2*time.Hour), now.Add(-2*time.Hour)))
		ok, err := locker.Lock(key + "-" + cursorLockName)
		require.NoError(t, err)
		require.True(t, ok)
	}
	target := strings.Repeat("f", endpointKeyHexChars)
	targetPath := filepath.Join(dir, target+".cursor")
	writeTestCursorFile(t, targetPath, target, time.Hour)
	require.NoError(t, os.Chtimes(targetPath, now.Add(-2*time.Hour), now.Add(-2*time.Hour)))

	coordinator := newCursorCoordinator(testEndpointKey(999), "https://192.0.2.1:443", time.Hour)
	coordinator.root = dir
	coordinator.path = filepath.Join(dir, coordinator.endpointKey+".cursor")
	coordinator.cleanup = newCursorCleanupGate()
	coordinator.cleanupOrphansLocked(now)
	_, err := os.Stat(targetPath)
	require.NoError(t, err)
	coordinator.cleanupOrphansLocked(now.Add(cursorCleanupInterval))
	_, err = os.Stat(targetPath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCursorPersistRunsThrottledCleanupDuringOrdinaryCollection(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(40_000, 0).UTC()
	orphan := testEndpointKey(2)
	orphanPath := filepath.Join(dir, orphan+".cursor")
	writeTestCursorFile(t, orphanPath, orphan, time.Hour)

	gate := newCursorCleanupGate()
	first := newCursorCoordinator(testEndpointKey(1), "https://192.0.2.1:443", time.Hour)
	first.root = dir
	first.path = filepath.Join(dir, first.endpointKey+".cursor")
	first.claimed = true
	first.loaded = true
	first.cleanup = gate
	require.NoError(t, first.Persist(now))
	require.NoError(t, os.Chtimes(orphanPath, now.Add(-2*time.Hour), now.Add(-2*time.Hour)))
	require.NoError(t, first.Persist(now.Add(cursorCleanupInterval)))
	_, err := os.Stat(orphanPath)
	require.ErrorIs(t, err, os.ErrNotExist)

	second := newCursorCoordinator(testEndpointKey(3), "https://192.0.2.2:443", time.Hour)
	second.root = dir
	second.path = filepath.Join(dir, second.endpointKey+".cursor")
	second.claimed = true
	second.loaded = true
	second.cleanup = gate
	require.NoError(t, second.Persist(now.Add(cursorCleanupInterval)))
	require.Equal(t, uint64(2), gate.roots[dir].scans)
}

func TestCursorGlobalPayloadBudgetRejectsOnlyCandidateSource(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(50_000, 0).UTC()
	coordinator := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	coordinator.root = dir
	coordinator.path = filepath.Join(dir, "endpoint.cursor")
	coordinator.claimed = true
	coordinator.loaded = true
	keys := makeTestCursorKeys(maxCursorExactKeys)
	for i := range 3 {
		key := fmt.Sprintf("%064x", i+1)
		require.NoError(t, coordinator.UpdateSource(key, logSourceCursor{
			Mode: "full", Initialized: true, ExactComplete: true, ExactRecordKeys: keys,
		}, now))
	}
	require.NoError(t, coordinator.Persist(now))
	before, err := os.ReadFile(coordinator.path)
	require.NoError(t, err)

	rejected := fmt.Sprintf("%064x", 4)
	candidate := logSourceCursor{
		Mode: "full", Initialized: true, ExactComplete: true, ExactRecordKeys: keys,
	}
	require.ErrorContains(t, coordinator.UpdateSource(rejected, candidate, now), "checkpoint capacity")
	require.NotContains(t, coordinator.payload.Sources, rejected)
	require.ErrorContains(t, coordinator.CheckpointSource(rejected, candidate, now), "checkpoint capacity")
	require.NotContains(t, coordinator.payload.Sources, rejected)
	require.NoError(t, coordinator.Persist(now.Add(time.Second)))
	after, err := os.ReadFile(coordinator.path)
	require.NoError(t, err)
	require.Equal(t, before, after)

	reloaded := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	reloaded.root = dir
	reloaded.path = coordinator.path
	require.NoError(t, reloaded.loadLocked())
	require.Len(t, reloaded.payload.Sources, 3)
}

func TestCursorGlobalPayloadBudgetRejectsSimultaneousReconciliationSets(t *testing.T) {
	now := time.Unix(60_000, 0).UTC()
	coordinator := newCursorCoordinator("endpoint", "https://192.0.2.1:443", time.Hour)
	keys := makeTestCursorKeys(maxCursorExactKeys)
	candidate := logSourceCursor{
		Mode: "full", Initialized: true, ExactComplete: true, ReconcileStarted: true,
		ExactRecordKeys: keys, ReconcileSourceKeys: keys, ReconcileRecordKeys: keys, ContinuationKeys: keys,
	}
	require.ErrorContains(
		t,
		coordinator.UpdateSource(testCursorSourceKey, candidate, now),
		"checkpoint capacity",
	)
	require.Empty(t, coordinator.payload.Sources)
}

func TestCursorRetentionMetadataDoesNotConsumePayloadCapacity(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(70_000, 0).UTC()
	coordinator := newCursorCoordinator("endpoint", "https://192.0.2.1:443", 0)
	coordinator.root = dir
	coordinator.path = filepath.Join(dir, "endpoint.cursor")
	coordinator.claimed = true
	coordinator.loaded = true
	keys := makeTestCursorKeys(maxCursorExactKeys)
	for i := range 3 {
		require.NoError(t, coordinator.UpdateSource(fmt.Sprintf("%064x", i+1), logSourceCursor{
			Mode: "full", Initialized: true, ExactComplete: true, ExactRecordKeys: keys,
		}, now))
	}

	lastKey := fmt.Sprintf("%064x", 4)
	low, high := 0, len(keys)
	for low < high {
		mid := low + (high-low+1)/2
		candidate := logSourceCursor{
			Mode: "full", LastActiveUsec: now.UnixMicro(), ExactRecordKeys: keys[:mid],
		}
		size := coordinator.payloadBytes + 1 + cursorSourceEntrySize(lastKey, candidate)
		if size <= maxCursorPayload {
			low = mid
		} else {
			high = mid - 1
		}
	}
	candidate := logSourceCursor{
		Mode: "full", LastActiveUsec: now.UnixMicro(), ExactRecordKeys: keys[:low],
	}
	require.NoError(t, coordinator.UpdateSource(lastKey, candidate, now))

	remaining := maxCursorPayload - coordinator.payloadBytes
	withOneByte := candidate
	withOneByte.Continuation = "x"
	continuationOverhead := cursorSourceEntrySize(lastKey, withOneByte) -
		cursorSourceEntrySize(lastKey, candidate) - 1
	fillThreshold := continuationOverhead + 1
	if remaining >= fillThreshold {
		candidate.Continuation = strings.Repeat("x", remaining-continuationOverhead)
		require.NoError(t, coordinator.UpdateSource(lastKey, candidate, now))
	}
	require.Less(t, maxCursorPayload-coordinator.payloadBytes, fillThreshold)
	require.NoError(t, coordinator.Persist(now))
	before, err := os.ReadFile(coordinator.path)
	require.NoError(t, err)
	require.Equal(t, coordinator.payloadBytes, len(before)-cursorHeaderSize)
	require.NotContains(t, string(before[cursorHeaderSize:]), "orphan_retention_nsec")

	reloaded := newCursorCoordinator(
		"endpoint",
		"https://192.0.2.1:443",
		time.Duration(1<<63-1),
	)
	reloaded.root = dir
	reloaded.path = coordinator.path
	reloaded.claimed = true
	require.NoError(t, reloaded.loadLocked())
	require.True(t, reloaded.dirty)
	require.NoError(t, reloaded.Persist(now.Add(time.Second)))
	after, err := os.ReadFile(reloaded.path)
	require.NoError(t, err)
	require.Len(t, after, len(before))
	decoded, err := decodeCursor(after)
	require.NoError(t, err)
	require.Equal(t, int64(1<<63-1), decoded.OrphanRetentionNsec)
}

func writeTestCursorFile(t *testing.T, path, endpointKey string, retention time.Duration) {
	t.Helper()
	raw, err := encodeCursor(cursorPayload{
		EndpointKey: endpointKey, OriginDigest: strings.Repeat("a", 64),
		OrphanRetentionNsec: retention.Nanoseconds(), Sources: map[string]logSourceCursor{},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o600))
}

func testEndpointKey(value int) string {
	return fmt.Sprintf("%0*x", endpointKeyHexChars, value)
}

func makeTestCursorKeys(count int) []string {
	result := make([]string, count)
	for i := range result {
		result[i] = fmt.Sprintf("%064x", i)
	}
	return result
}
