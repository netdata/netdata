// SPDX-License-Identifier: GPL-3.0-or-later

package cwprofiles

import (
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

var (
	defaultCatalogMu           sync.Mutex
	defaultCatalog             Catalog
	defaultCatalogLoaded       bool
	defaultCatalogLoader       = LoadFromDefaultDirs
	defaultCatalogCacheEnabled = func() bool { return executable.Name != "test" }
)

// DefaultCatalog loads (and caches) the stock+user profile catalog. Caching is
// disabled under tests so each test observes a fresh load.
func DefaultCatalog() (Catalog, error) {
	if !defaultCatalogCacheEnabled() {
		return defaultCatalogLoader()
	}

	defaultCatalogMu.Lock()
	defer defaultCatalogMu.Unlock()

	if defaultCatalogLoaded {
		return defaultCatalog, nil
	}

	catalog, err := defaultCatalogLoader()
	if err != nil {
		return Catalog{}, err
	}

	defaultCatalog = catalog
	defaultCatalogLoaded = true

	return defaultCatalog, nil
}
