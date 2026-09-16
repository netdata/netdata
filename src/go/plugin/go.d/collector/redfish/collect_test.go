// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnavailableCollectionKeepsResponseDiagnostics(t *testing.T) {
	server := newRedfishTestServer(t, redfishTestServerConfig{
		sessionStatus: http.StatusUnauthorized,
	})
	t.Cleanup(server.Close)
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	result, err := client.Collect(t.Context())
	require.Error(t, err)
	require.Equal(t, "unavailable", result.Metrics.Status)
	require.Positive(t, result.Metrics.Operations["successful"])
	require.Contains(t, result.Diagnostics, "Redfish compatibility: response OData-Version header is missing")
}

func TestFatalGraphCollectionKeepsExpansionDiagnostic(t *testing.T) {
	const root = "/redfish/v1/"
	const system = root + "Systems/1"
	var expanded atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("OData-Version", "4.0")
		switch r.URL.Path {
		case root:
			writeJSON(w, sourceTestResource(root, "ServiceRoot", "Service", map[string]any{
				"RedfishVersion":            "1.20.0",
				"Systems":                   sourceTestLink(root + "Systems"),
				"ProtocolFeaturesSupported": map[string]any{"ExpandQuery": map[string]any{"NoLinks": true}},
			}))
		case root + "Systems":
			if r.URL.Query().Has("$expand") {
				expanded.Add(1)
			}
			// A service advertising expansion but returning ordinary links triggers fallback.
			writeJSON(w, sourceTestCollection(root+"Systems", "ComputerSystem", system))
		case system:
			writeJSON(w, sourceTestResource(system, "ComputerSystem", "System", nil))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	// A real digest collision is impractical to construct; seed the conflicting
	// binding to exercise the production fatal return after real HTTP fallback.
	require.NoError(t, client.identities.register([]identityBinding{{
		Domain: "resource", Key: resourceKey(client.origin, "system", system), Preimage: "other-resource",
	}}))
	result, err := client.Collect(t.Context())
	require.ErrorIs(t, err, errIdentityIntegrity)
	require.Equal(t, "partial", result.Metrics.Status)
	require.Equal(t, int64(1), expanded.Load())
	require.Contains(t, result.Diagnostics,
		"Redfish compatibility: advertised collection query expansion was rejected; using ordinary member links")
	require.Empty(t, client.takeExpansionFallbackDiagnostic(), "reported warning must be consumed")
}
