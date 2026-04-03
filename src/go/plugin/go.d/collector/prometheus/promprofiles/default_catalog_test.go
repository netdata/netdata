// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultCatalog_CachesSuccessfulLoads(t *testing.T) {
	calls := 0
	restore := stubDefaultCatalog(t, func() bool { return true }, func() (Catalog, error) {
		calls++
		return testCatalog("demo"), nil
	})
	defer restore()

	first, err := DefaultCatalog()
	require.NoError(t, err)

	second, err := DefaultCatalog()
	require.NoError(t, err)

	assert.Equal(t, 1, calls)
	profiles, err := first.Resolve([]string{"demo"})
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	profiles, err = second.Resolve([]string{"demo"})
	require.NoError(t, err)
	require.Len(t, profiles, 1)
}

func TestDefaultCatalog_RetriesAfterFailure(t *testing.T) {
	calls := 0
	restore := stubDefaultCatalog(t, func() bool { return true }, func() (Catalog, error) {
		calls++
		if calls == 1 {
			return Catalog{}, errors.New("boom")
		}
		return testCatalog("retry"), nil
	})
	defer restore()

	_, err := DefaultCatalog()
	require.Error(t, err)

	catalog, err := DefaultCatalog()
	require.NoError(t, err)

	assert.Equal(t, 2, calls)
	profiles, err := catalog.Resolve([]string{"retry"})
	require.NoError(t, err)
	require.Len(t, profiles, 1)
}

func TestDefaultCatalog_DoesNotCacheWhenDisabled(t *testing.T) {
	calls := 0
	restore := stubDefaultCatalog(t, func() bool { return false }, func() (Catalog, error) {
		calls++
		return testCatalog("nocache"), nil
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

func testCatalog(name string) Catalog {
	return Catalog{
		byName: map[string]Profile{
			name: {Name: name},
		},
		orderedKeys: []string{name},
	}
}
