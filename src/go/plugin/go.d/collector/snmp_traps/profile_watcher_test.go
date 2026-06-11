// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserProfileWatcherRefreshReloadsUserProfiles(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "base.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()
	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()
	assert.NotNil(t, idx.Lookup("1.3.6.1.6.3.1.1.5.3"))

	watcher := newUserProfileWatcher([]string{dir})
	watcher.lastFingerprint, err = fingerprintUserProfileFiles([]string{dir})
	require.NoError(t, err)

	writeProfileYAML(t, dir, "new.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkUp
    category: state_change
    severity: notice
`)

	watcher.refresh(context.Background())

	reloaded := CurrentProfileIndex()
	require.NotNil(t, reloaded)
	assert.NotSame(t, idx, reloaded)
	assert.NotNil(t, reloaded.Lookup("1.3.6.1.6.3.1.1.5.3"))
	assert.NotNil(t, reloaded.Lookup("1.3.6.1.6.3.1.1.5.4"))
}

func TestUserProfileWatcherFailedReloadKeepsOldIndexAndFailsNextAcquire(t *testing.T) {
	dir := t.TempDir()
	writeProfileYAML(t, dir, "base.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	setTestDirs(t, dir)
	resetProfileCacheForTest()

	const jobName = "watcher-reload-failure"
	removeJobMetrics(jobName)
	metrics := getJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	idx, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()

	watcher := newUserProfileWatcher([]string{dir})
	watcher.lastFingerprint, err = fingerprintUserProfileFiles([]string{dir})
	require.NoError(t, err)
	previousFingerprint := watcher.lastFingerprint

	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkUp
    category: invalid_category
    severity: notice
`)

	watcher.refresh(context.Background())

	assert.Same(t, idx, CurrentProfileIndex())
	assert.Equal(t, uint64(1), metrics.errors.profileLoadFailed.Load())
	assert.Equal(t, previousFingerprint, watcher.lastFingerprint)

	_, err = AcquireProfileCache()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid category")

	writeProfileYAML(t, dir, "bad.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkUp
    category: state_change
    severity: notice
`)

	watcher.refresh(context.Background())
	reloaded, err := AcquireProfileCache()
	require.NoError(t, err)
	defer ReleaseProfileCache()
	assert.NotNil(t, reloaded.Lookup("1.3.6.1.6.3.1.1.5.4"))
	assert.NotEqual(t, previousFingerprint, watcher.lastFingerprint)
}

func TestUserProfileWatcherStartStopRunLoopReloadsUserProfiles(t *testing.T) {
	dir := t.TempDir()
	watcher := newUserProfileWatcher([]string{dir})

	runWatcherReloadTest(t, watcher, dir)

	assert.Nil(t, watcher.cancel)
	assert.Nil(t, watcher.done)
}

func TestUserProfileWatcherStartFallsBackToPeriodicScans(t *testing.T) {
	dir := t.TempDir()
	watcher := newUserProfileWatcher([]string{dir})
	watcher.newWatcher = func() (*fsnotify.Watcher, error) {
		return nil, errors.New("watcher unavailable")
	}

	runWatcherReloadTest(t, watcher, dir)

	assert.Nil(t, watcher.watcher)
	assert.Nil(t, watcher.cancel)
	assert.Nil(t, watcher.done)
}

func runWatcherReloadTest(t *testing.T, watcher *profileWatcher, dir string) {
	t.Helper()

	watcher.refreshEvery = 10 * time.Millisecond
	watcher.eventSettle = time.Millisecond

	initialFingerprint := make(chan struct{})
	var initialOnce sync.Once
	watcher.fingerprint = func(dirs []string) (string, error) {
		fp, err := fingerprintUserProfileFiles(dirs)
		initialOnce.Do(func() {
			close(initialFingerprint)
		})
		return fp, err
	}

	reloaded := make(chan struct{}, 1)
	watcher.markDirty = func() {}
	watcher.reload = func() error {
		select {
		case reloaded <- struct{}{}:
		default:
		}
		return nil
	}

	require.NoError(t, watcher.Start())
	defer watcher.Stop()

	require.Eventually(t, func() bool {
		select {
		case <-initialFingerprint:
			return true
		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	writeProfileYAML(t, dir, "new.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkUp
    category: state_change
    severity: notice
`)

	require.Eventually(t, func() bool {
		select {
		case <-reloaded:
			return true
		default:
			return false
		}
	}, 2*time.Second, 10*time.Millisecond)

	watcher.Stop()
}

func TestUserProfileWatcherForgetsRemovedDirectoryWatch(t *testing.T) {
	dir := filepath.Clean(t.TempDir())
	watcher := newUserProfileWatcher([]string{dir})
	watcher.watches = map[string]struct{}{dir: {}}

	watcher.forgetRemovedWatch(fsnotify.Event{Name: dir, Op: fsnotify.Remove})

	assert.NotContains(t, watcher.watches, dir)
}

func TestUserProfileFingerprintOnlyWalksGivenDirs(t *testing.T) {
	userDir := t.TempDir()
	stockDir := t.TempDir()
	writeProfileYAML(t, userDir, "user.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	before, err := fingerprintUserProfileFiles([]string{userDir})
	require.NoError(t, err)

	writeProfileYAML(t, stockDir, "stock.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.4
    name: IF-MIB::linkUp
    category: state_change
    severity: notice
`)
	afterStockChange, err := fingerprintUserProfileFiles([]string{userDir})
	require.NoError(t, err)
	assert.Equal(t, before, afterStockChange)

	writeProfileYAML(t, userDir, "_base.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.5
    name: IF-MIB::authFailure
    category: auth
    severity: warning
`)
	afterUserBaseChange, err := fingerprintUserProfileFiles([]string{userDir})
	require.NoError(t, err)
	assert.NotEqual(t, before, afterUserBaseChange)

	require.NoError(t, os.Chmod(filepath.Join(userDir, "user.yaml"), 0600))
	afterUserModeChange, err := fingerprintUserProfileFiles([]string{userDir})
	require.NoError(t, err)
	assert.NotEqual(t, afterUserBaseChange, afterUserModeChange)
}

func TestUserProfileWatcherFingerprintFailureMarksDirtyAndReloads(t *testing.T) {
	dir := t.TempDir()
	watcher := newUserProfileWatcher([]string{dir})
	watcher.lastFingerprint = "previous"

	var dirty atomic.Bool
	var reloads atomic.Int64
	watcher.markDirty = func() {
		dirty.Store(true)
	}
	watcher.reload = func() error {
		reloads.Add(1)
		return errors.New("reload failed")
	}
	watcher.fingerprint = func([]string) (string, error) {
		return "", errors.New("fingerprint failed")
	}

	watcher.refresh(context.Background())

	assert.True(t, dirty.Load())
	assert.Equal(t, int64(1), reloads.Load())
	assert.Equal(t, "previous", watcher.lastFingerprint)
}

func TestReloadUserProfileCachePreservesExistingStockStore(t *testing.T) {
	userDir := t.TempDir()
	stockDir := t.TempDir()
	writeProfileYAML(t, userDir, "user.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	const stockOID = "1.3.6.1.6.3.1.1.5.4"
	current := loadedStockIndexForWatcherTest(stockDir, "vendor.yaml", stockOID)
	setTestDirs(t, userDir)

	reloaded, err := loadUserProfileCache(current)
	require.NoError(t, err)

	require.NotNil(t, reloaded.stock)
	assert.Equal(t, "vendor.yaml", reloaded.stock.exactRoutes[stockOID])
	assert.NotNil(t, reloaded.Lookup("1.3.6.1.6.3.1.1.5.3"))
	stockTrap := reloaded.Lookup(stockOID)
	require.NotNil(t, stockTrap)
	assert.Equal(t, filepath.Join(stockDir, "vendor.yaml"), stockTrap.sourceFile)
}

func TestReloadUserProfileCacheFiltersStockFileReplacedByUserProfile(t *testing.T) {
	userDir := t.TempDir()
	stockDir := t.TempDir()
	writeProfileYAML(t, userDir, "vendor.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.3
    name: IF-MIB::linkDown
    category: state_change
    severity: warning
`)

	const stockOID = "1.3.6.1.6.3.1.1.5.4"
	current := loadedStockIndexForWatcherTest(stockDir, "vendor.yaml", stockOID)
	setTestDirs(t, userDir)

	reloaded, err := loadUserProfileCache(current)
	require.NoError(t, err)

	require.NotNil(t, reloaded.stock)
	assert.Empty(t, reloaded.stock.exactRoutes)
	assert.Empty(t, reloaded.stock.loaded)
	assert.NotNil(t, reloaded.Lookup("1.3.6.1.6.3.1.1.5.3"))
	assert.Nil(t, reloaded.Lookup(stockOID))
}

func loadedStockIndexForWatcherTest(stockDir, name, oid string) *ProfileIndex {
	path := filepath.Join(stockDir, name)
	trap := &TrapDef{
		OID:        oid,
		Name:       "STOCK-MIB::trap",
		Category:   "state_change",
		Severity:   "notice",
		sourceFile: path,
	}
	return &ProfileIndex{
		trapsByOID: map[string]*TrapDef{
			oid: trap,
		},
		namesByTrapName: map[string]*TrapDef{
			trap.Name: trap,
		},
		stock: &stockProfileStore{
			dir:              stockDir,
			extendsPaths:     multipath.New(stockDir),
			files:            map[string]string{name: path},
			exactRoutes:      map[string]string{oid: name},
			enterpriseRoutes: make(map[string]string),
			loaded:           map[string]bool{name: true},
		},
	}
}
