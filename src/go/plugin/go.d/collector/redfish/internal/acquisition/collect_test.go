// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
	"github.com/stretchr/testify/require"
)

func TestUnavailableCollectionReportsAuthenticationFailure(t *testing.T) {
	server := testutil.NewServer(t, testutil.ServerConfig{
		SessionStatus: http.StatusUnauthorized,
	})
	t.Cleanup(server.Close)
	client := newTestProtocolClient(t, testConfig(server.URL, "session"))
	result, err := client.Acquire(t.Context())
	require.Error(t, err)
	require.False(t, result.Available)
	require.Positive(t, result.Statistics.Failures["auth"])
}

func TestCollectionDoesNotRequestExpansion(t *testing.T) {
	const root = "/redfish/v1/"
	const system = root + "Systems/1"
	var expanded atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("OData-Version", "4.0")
		switch r.URL.Path {
		case root:
			testutil.WriteJSON(w, testutil.Resource(root, "ServiceRoot", "Service", map[string]any{
				"RedfishVersion":            "1.20.0",
				"Systems":                   testutil.Link(root + "Systems"),
				"ProtocolFeaturesSupported": map[string]any{"ExpandQuery": map[string]any{"NoLinks": true}},
			}))
		case root + "Systems":
			if r.URL.Query().Has("$expand") {
				expanded.Add(1)
			}
			// An advertised expansion capability does not change the request path.
			testutil.WriteJSON(w, testutil.Collection(root+"Systems", "ComputerSystem", system))
		case system:
			testutil.WriteJSON(w, testutil.Resource(system, "ComputerSystem", "System", nil))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	// A real digest collision is impractical to construct; seed the conflicting
	// binding to exercise the production fatal return after real HTTP collection.
	require.NoError(t, client.identities.Register([]identity.Binding{{
		Domain: "resource", Key: identity.ResourceKey(client.origin, "system", system), Preimage: "other-resource",
	}}))
	result, err := client.Acquire(t.Context())
	require.ErrorIs(t, err, identity.ErrIntegrity)
	require.True(t, result.Available)
	require.False(t, result.Complete)
	require.Zero(t, expanded.Load())
}

func TestFatalGraphCollectionKeepsEarlierDiagnostics(t *testing.T) {
	const root = "/redfish/v1/"
	const chassis = root + "Chassis/1"
	const sensors = chassis + "/Sensors"
	const controls = chassis + "/Controls"
	const control = controls + "/1"
	docs := map[string]map[string]any{
		root: testutil.Resource(root, "ServiceRoot", "Service", map[string]any{
			"RedfishVersion": "1.20.0", "Chassis": testutil.Link(root + "Chassis"),
		}),
		root + "Chassis": testutil.Collection(root+"Chassis", "Chassis", chassis),
		chassis: testutil.Resource(chassis, "Chassis", "Chassis", map[string]any{
			"Sensors": testutil.Link(sensors), "Controls": testutil.Link(controls),
		}),
		controls: testutil.Collection(controls, "Control", control),
		control:  testutil.Resource(control, "Control", "Control", nil),
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("OData-Version", "4.0")
		if r.URL.Path == sensors {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		if document := docs[r.URL.Path]; document != nil {
			testutil.WriteJSON(w, document)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	ordinary := newTestProtocolClient(t, testConfig(server.URL, "none"))
	before, err := ordinary.Acquire(t.Context())
	require.False(t, identity.IsIntegrityError(err))
	var expected string
	for _, diagnostic := range before.Diagnostics.Values() {
		if strings.Contains(diagnostic, "Sensors") {
			expected = diagnostic
			break
		}
	}
	require.NotEmpty(t, expected, "control run must record Sensors failure")
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	// A real hash collision is impractical; inject its registry state to test this error branch.
	require.NoError(t, client.identities.Register([]identity.Binding{{
		Domain: "resource", Key: identity.ResourceKey(client.origin, "control", control), Preimage: "other-control",
	}}))
	result, err := client.Acquire(t.Context())
	require.ErrorIs(t, err, identity.ErrIntegrity)
	require.False(t, result.Complete)
	require.Contains(t, result.Diagnostics.Values(), expected, "earlier graph diagnostic must survive fatal return")
}
