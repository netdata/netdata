// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Catalog caching (load-once, retry-after-failure, disabled-under-test) is now
// provided and tested by pkg/profilecatalog (Cached); it is not re-tested here.

// TestDefaultCatalog_AllStockProfilesHydrate hydrates and validates every stock
// profile's lazy fields. Production hydrates only selected profiles, so the regular
// test suite is what keeps a broken unselected stock profile from shipping.
func TestDefaultCatalog_AllStockProfilesHydrate(t *testing.T) {
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	profiles := catalog.OrderedProfiles()
	require.NotEmpty(t, profiles, "expected at least one stock profile")

	for _, p := range profiles {
		_, err := p.Template()
		require.NoErrorf(t, err, "stock profile %q template must be valid", p.Name)
		_, err = p.Relabeling()
		require.NoErrorf(t, err, "stock profile %q relabeling must be valid", p.Name)
	}
}
