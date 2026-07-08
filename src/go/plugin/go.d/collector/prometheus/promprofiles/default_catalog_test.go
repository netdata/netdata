// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Catalog caching (load-once, retry-after-failure, disabled-under-test) is now
// provided and tested by pkg/profilecatalog (Cached); it is not re-tested here.

// TestDefaultCatalog_AllStockTemplatesValidate hydrates and validates every stock
// profile's template. Templates are parsed lazily at runtime (only when a job
// selects a profile), so this test is what keeps a broken stock template from
// slipping through CI — it would otherwise surface only when a job happens to
// select that profile.
func TestDefaultCatalog_AllStockTemplatesValidate(t *testing.T) {
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	profiles := catalog.OrderedProfiles()
	require.NotEmpty(t, profiles, "expected at least one stock profile")

	for _, p := range profiles {
		_, err := p.Template()
		require.NoErrorf(t, err, "stock profile %q template must be valid", p.Name)
	}
}
