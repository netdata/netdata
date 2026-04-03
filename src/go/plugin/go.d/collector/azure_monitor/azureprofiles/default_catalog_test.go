// SPDX-License-Identifier: GPL-3.0-or-later

package azureprofiles

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromDefaultDirs_LoadsAllStockProfiles(t *testing.T) {
	dir := azureProfilesDirFromThisFile()
	require.NotEmpty(t, dir)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	var want int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		want++
	}

	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	assert.Len(t, catalog.byBaseName, want)
}

func TestDefaultCatalog_CachesSuccessfulLoads(t *testing.T) {
	calls := 0
	restore := stubDefaultCatalog(t, func() bool { return true }, func() (Catalog, error) {
		calls++
		return testCatalog("sql_database"), nil
	})
	defer restore()

	first, err := DefaultCatalog()
	require.NoError(t, err)

	second, err := DefaultCatalog()
	require.NoError(t, err)

	assert.Equal(t, 1, calls)
	assert.Equal(t, []string{"sql_database"}, first.defaultProfileBaseNames())
	assert.Equal(t, []string{"sql_database"}, second.defaultProfileBaseNames())
}

func TestDefaultCatalog_RetriesAfterFailure(t *testing.T) {
	calls := 0
	restore := stubDefaultCatalog(t, func() bool { return true }, func() (Catalog, error) {
		calls++
		if calls == 1 {
			return Catalog{}, errors.New("boom")
		}
		return testCatalog("postgres_flexible"), nil
	})
	defer restore()

	_, err := DefaultCatalog()
	require.Error(t, err)

	catalog, err := DefaultCatalog()
	require.NoError(t, err)

	assert.Equal(t, 2, calls)
	assert.Equal(t, []string{"postgres_flexible"}, catalog.defaultProfileBaseNames())
}

func TestDefaultCatalog_DoesNotCacheWhenDisabled(t *testing.T) {
	calls := 0
	restore := stubDefaultCatalog(t, func() bool { return false }, func() (Catalog, error) {
		calls++
		return testCatalog("storage_accounts"), nil
	})
	defer restore()

	_, err := DefaultCatalog()
	require.NoError(t, err)
	_, err = DefaultCatalog()
	require.NoError(t, err)

	assert.Equal(t, 2, calls)
}

func stubDefaultCatalog(t *testing.T, cacheEnabled func() bool, loader func() (Catalog, error)) func() {
	t.Helper()

	defaultCatalogMu.Lock()
	prevCatalog := defaultCatalog
	prevLoaded := defaultCatalogLoaded
	prevLoader := defaultCatalogLoader
	prevEnabled := defaultCatalogCacheEnabled

	defaultCatalog = Catalog{}
	defaultCatalogLoaded = false
	defaultCatalogLoader = loader
	defaultCatalogCacheEnabled = cacheEnabled
	defaultCatalogMu.Unlock()

	return func() {
		defaultCatalogMu.Lock()
		defaultCatalog = prevCatalog
		defaultCatalogLoaded = prevLoaded
		defaultCatalogLoader = prevLoader
		defaultCatalogCacheEnabled = prevEnabled
		defaultCatalogMu.Unlock()
	}
}

func testCatalog(id string) Catalog {
	profile := Profile{DisplayName: id}
	return Catalog{
		byBaseName: map[string]Profile{
			normalizeKey(id): profile,
		},
		stockProfileBaseNames: map[string]struct{}{
			normalizeKey(id): {},
		},
	}
}
