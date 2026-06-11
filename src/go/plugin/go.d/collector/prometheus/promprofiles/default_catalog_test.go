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
		return testCatalog("haproxy"), nil
	})
	defer restore()

	first, err := DefaultCatalog()
	require.NoError(t, err)

	second, err := DefaultCatalog()
	require.NoError(t, err)

	assert.Equal(t, 1, calls)
	assert.Equal(t, []string{"haproxy"}, profileNames(first.OrderedProfiles()))
	assert.Equal(t, []string{"haproxy"}, profileNames(second.OrderedProfiles()))
}

func TestDefaultCatalog_RetriesAfterFailure(t *testing.T) {
	calls := 0
	restore := stubDefaultCatalog(t, func() bool { return true }, func() (Catalog, error) {
		calls++
		if calls == 1 {
			return Catalog{}, errors.New("boom")
		}
		return testCatalog("nginx"), nil
	})
	defer restore()

	_, err := DefaultCatalog()
	require.Error(t, err)

	catalog, err := DefaultCatalog()
	require.NoError(t, err)

	assert.Equal(t, 2, calls)
	assert.Equal(t, []string{"nginx"}, profileNames(catalog.OrderedProfiles()))
}

func TestDefaultCatalog_DoesNotCacheWhenDisabled(t *testing.T) {
	calls := 0
	restore := stubDefaultCatalog(t, func() bool { return false }, func() (Catalog, error) {
		calls++
		return testCatalog("haproxy"), nil
	})
	defer restore()

	_, err := DefaultCatalog()
	require.NoError(t, err)
	_, err = DefaultCatalog()
	require.NoError(t, err)

	assert.Equal(t, 2, calls)
}

// stubDefaultCatalog swaps the package-level loader and cache-enabled gate for the
// duration of a test, restoring them (and the cached state) afterwards.
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
	key := normalizeKey(name)
	return Catalog{
		byName:      map[string]Profile{key: {Name: name}},
		orderedKeys: []string{key},
	}
}

func profileNames(profiles []Profile) []string {
	names := make([]string, 0, len(profiles))
	for _, p := range profiles {
		names = append(names, p.Name)
	}
	return names
}
