// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
)

var (
	defaultCatalogMu           sync.Mutex
	defaultCatalog             Catalog
	defaultCatalogLoaded       bool
	defaultCatalogLoader       = loadFromDefaultDirs
	defaultCatalogCacheEnabled = func() bool { return executable.Name != "test" }
)

// DefaultCatalog returns the process-wide profile catalog loaded from the stock
// and user directories. The result is cached for the agent process; tests load
// fresh each call so fixtures are not shared across cases.
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
