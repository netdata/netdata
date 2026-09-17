// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnavailableCollectionReportsAuthenticationFailure(t *testing.T) {
	server := newRedfishTestServer(t, redfishTestServerConfig{
		sessionStatus: http.StatusUnauthorized,
	})
	t.Cleanup(server.Close)
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	result, err := client.Collect(t.Context())
	require.Error(t, err)
	require.Equal(t, "unavailable", result.Metrics.Status)
	require.Positive(t, result.Metrics.Failures["auth"])
}

func TestCollectionDoesNotRequestExpansion(t *testing.T) {
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
			// An advertised expansion capability does not change the request path.
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
	// binding to exercise the production fatal return after real HTTP collection.
	require.NoError(t, client.identities.register([]identityBinding{{
		Domain: "resource", Key: resourceKey(client.origin, "system", system), Preimage: "other-resource",
	}}))
	result, err := client.Collect(t.Context())
	require.ErrorIs(t, err, errIdentityIntegrity)
	require.Equal(t, "partial", result.Metrics.Status)
	require.Zero(t, expanded.Load())
}

func TestFatalGraphCollectionKeepsEarlierDiagnostics(t *testing.T) {
	const root = "/redfish/v1/"
	const chassis = root + "Chassis/1"
	const sensors = chassis + "/Sensors"
	const controls = chassis + "/Controls"
	const control = controls + "/1"
	docs := map[string]map[string]any{
		root: sourceTestResource(root, "ServiceRoot", "Service", map[string]any{
			"RedfishVersion": "1.20.0", "Chassis": sourceTestLink(root + "Chassis"),
		}),
		root + "Chassis": sourceTestCollection(root+"Chassis", "Chassis", chassis),
		chassis: sourceTestResource(chassis, "Chassis", "Chassis", map[string]any{
			"Sensors": sourceTestLink(sensors), "Controls": sourceTestLink(controls),
		}),
		controls: sourceTestCollection(controls, "Control", control),
		control:  sourceTestResource(control, "Control", "Control", nil),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("OData-Version", "4.0")
		if r.URL.Path == sensors {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if document := docs[r.URL.Path]; document != nil {
			writeJSON(w, document)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	ordinary := newTestProtocolClient(t, testConfig(server.URL, "none"))
	before, err := ordinary.Collect(t.Context())
	require.False(t, identityIntegrityError(err))
	var expected string
	for _, diagnostic := range before.Diagnostics {
		if strings.Contains(diagnostic, "Sensors") {
			expected = diagnostic
			break
		}
	}
	require.NotEmpty(t, expected, "control run must record Sensors failure")
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	// A real hash collision is impractical; inject its registry state to test this error branch.
	require.NoError(t, client.identities.register([]identityBinding{{
		Domain: "resource", Key: resourceKey(client.origin, "control", control), Preimage: "other-control",
	}}))
	result, err := client.Collect(t.Context())
	require.ErrorIs(t, err, errIdentityIntegrity)
	require.Empty(t, result.Hardware, "hardware must remain suppressed after identity failure")
	require.Contains(t, result.Diagnostics, expected, "earlier graph diagnostic must survive fatal return")
}
