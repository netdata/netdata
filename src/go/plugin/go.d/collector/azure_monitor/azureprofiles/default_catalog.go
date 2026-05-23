// SPDX-License-Identifier: GPL-3.0-or-later

package azureprofiles

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
