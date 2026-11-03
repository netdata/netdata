// SPDX-License-Identifier: GPL-3.0-or-later

package functions_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	rumfunctions "github.com/netdata/netdata/go/plugins/plugin/dem/rum/functions"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/geoip"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/query"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSitesGeoIPSourceEvidence(t *testing.T) {
	path := "../../../../../crates/netflow-plugin/testdata/mmdb/GeoLite2-City-Test.mmdb"
	resolver := geoip.New(path, geoip.Paths{})
	t.Cleanup(resolver.Close)
	require.NoError(t, resolver.Refresh())
	hub := registry.New()
	handler := rumfunctions.New(query.New(hub, nil))
	collector := func() map[string]any {
		response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
			Method: "rum-sites",
		}).RawResponse
		data, err := json.Marshal(response["collector"])
		require.NoError(t, err)
		assert.NotContains(t, string(data), path)
		assert.NotContains(t, string(data), "81.2.69.142")
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(data, &decoded))
		return decoded
	}
	initial := collector()["geoip"].(map[string]any)
	assert.Equal(t, "unavailable", initial["state"])
	assert.NotContains(t, initial, "selection")
	publication := hub.PublishReceiver(registry.Availability{
		Serving:   true,
		PublicURL: "https://rum.example",
	})
	publication.SetGeoIP(resolver.Status())
	active := collector()
	assert.Equal(t, true, active["ingress_available"])
	evidence := active["geoip"].(map[string]any)
	assert.Equal(t, "explicit", evidence["selection"])
	assert.Equal(t, "loaded", evidence["state"])
	assert.Equal(t, "GeoLite2-City", evidence["database_type"])
	assert.Equal(t, float64(resolver.Status().BuildAt), evidence["build_at"])
	assert.Equal(t, float64(resolver.Status().LoadedAt), evidence["loaded_at"])
	assert.Equal(t, float64(resolver.Status().LastCheckedAt), evidence["last_checked_at"])
	assert.Equal(t, float64(0), evidence["lookup_errors"])
	publication.Close()
	retired := collector()
	assert.Equal(t, false, retired["ingress_available"])
	evidence = retired["geoip"].(map[string]any)
	assert.Equal(t, "unavailable", evidence["state"])
	assert.NotContains(t, evidence, "source")
	assert.NotContains(t, evidence, "database_type")
	assert.NotContains(t, evidence, "loaded_at")
}
