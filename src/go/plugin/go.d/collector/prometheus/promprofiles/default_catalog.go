// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

// defaultCatalog memoizes the stock+user catalog for the process lifetime.
// Caching is disabled under tests so each test observes a fresh load.
var defaultCatalog = profilecatalog.NewCached(LoadFromDefaultDirs)

// DefaultCatalog returns the process-wide profile catalog loaded from the stock
// and user directories.
func DefaultCatalog() (Catalog, error) { return defaultCatalog.Get() }
