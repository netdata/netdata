// SPDX-License-Identifier: GPL-3.0-or-later

package promprofiles

import (
	"os"
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
		_, err = p.FallbackType()
		require.NoErrorf(t, err, "stock profile %q fallback_type must be valid", p.Name)
	}
}

// TestDefaultCatalog_StockProfilesHaveMetadataDisposition keeps the runtime
// profile catalog and public integration catalog from drifting silently. A
// profile may point at Prometheus metadata or an equivalent first-class
// integration, but every stock profile must make that choice explicitly.
func TestDefaultCatalog_StockProfilesHaveMetadataDisposition(t *testing.T) {
	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	type disposition struct {
		metadataPath  string
		integrationID string
	}
	dispositions := map[string]disposition{
		"ceph": {
			metadataPath:  "../metadata.yaml",
			integrationID: "collector-go.d.plugin-prometheus-ceph",
		},
		"haproxy": {
			metadataPath:  "../../haproxy/metadata.yaml",
			integrationID: "collector-go.d.plugin-haproxy",
		},
		"litellm": {
			metadataPath:  "../metadata.yaml",
			integrationID: "collector-go.d.plugin-prometheus-litellm",
		},
		"vllm": {
			metadataPath:  "../metadata.yaml",
			integrationID: "collector-go.d.plugin-prometheus-vllm",
		},
	}

	profiles := catalog.OrderedProfiles()
	profileNames := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		profileNames[profile.Name] = true
		_, ok := dispositions[profile.Name]
		require.Truef(t, ok, "stock profile %q must declare an integration metadata disposition", profile.Name)
	}

	for name, disposition := range dispositions {
		require.Truef(t, profileNames[name], "metadata disposition %q has no stock profile", name)
		content, err := os.ReadFile(disposition.metadataPath)
		require.NoErrorf(t, err, "read metadata disposition for stock profile %q", name)
		require.Containsf(t, string(content), "id: "+disposition.integrationID,
			"metadata disposition for stock profile %q must reference integration %q", name, disposition.integrationID)
	}
}
