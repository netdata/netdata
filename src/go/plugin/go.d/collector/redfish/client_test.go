// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/testutil"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const asyncTestTimeout = 10 * time.Second

func TestProtocolClientBasicCheckAndCollect(t *testing.T) {
	t.Parallel()

	server := testutil.NewServer(t, testutil.ServerConfig{
		RequireBasic: true,
	})
	defer server.Close()

	cfg := testConfig(server.URL, "basic")
	client := newTestCollector(t, cfg)
	require.NoError(t, client.Check(context.Background()))

	result, err := client.collect(context.Background())
	require.NoError(t, err)
	assert.True(t, result.Complete)
	assert.Equal(t, "success", result.Metrics.Status)
	assert.NotEmpty(t, result.Hardware)
	assert.Positive(t, result.Metrics.HTTPRequests["started"])

	coverage := New()
	managed, ok := metrix.AsCycleManagedStore(coverage.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	coverage.metrics.observe("endpoint-key", "endpoint-job", result.Metrics)
	coverage.hardware.observe(result.Hardware)
	require.NoError(t, cycle.CommitCycleSuccess())
	collecttest.AssertChartCoverage(t, coverage, collecttest.ChartCoverageExpectation{})
}

func TestProtocolClientRetainsLastCompleteMembershipWithoutReplayingCurrentState(t *testing.T) {
	t.Parallel()

	var FailSystems atomic.Bool
	server := testutil.NewServer(t, testutil.ServerConfig{
		FailSystems: &FailSystems,
	})
	defer server.Close()

	client := newTestCollector(t, testConfig(server.URL, "none"))
	first, err := client.collect(context.Background())
	require.NoError(t, err)
	require.True(t, first.Complete)

	FailSystems.Store(true)
	second, err := client.collect(context.Background())
	require.Error(t, err)
	assert.False(t, second.Complete)
	assert.Equal(t, "partial", second.Metrics.Status)
	assert.Positive(t, second.Metrics.Resources["unknown"])
	foundSystem := false
	for _, observation := range second.Hardware {
		switch observation.Metric {
		case "system_health", "system_state":
			t.Errorf("failed membership replayed prior source state: %s=%s", observation.Metric, observation.State)
		case "system_acquisition_state":
			foundSystem = true
			assert.Equal(t, "unknown", observation.State)
		}
	}
	require.True(t, foundSystem)
}

func TestProtocolClientRetainsPartialMembershipUntilAuthoritativeRemoval(t *testing.T) {
	var phase atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/":
			testutil.ServeServiceRoot(w, false)
		case "/redfish/v1/Systems":
			switch phase.Load() {
			case 0:
				testutil.WriteJSON(w, map[string]any{
					"@odata.id": r.URL.Path, "@odata.type": "#ComputerSystemCollection.ComputerSystemCollection",
					"Members@odata.count": 2,
					"Members": []any{
						map[string]any{"@odata.id": "/redfish/v1/Systems/1"},
						map[string]any{"Id": "invalid"},
					},
				})
			case 1:
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
			default:
				testutil.ServeCollection(w, r.URL.Path)
			}
		case "/redfish/v1/Systems/1":
			testutil.ServeBaseResource(w, r.URL.Path, "#ComputerSystem.v1_21_0.ComputerSystem", "System")
		case "/redfish/v1/Chassis", "/redfish/v1/Managers":
			testutil.ServeCollection(w, r.URL.Path)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newTestCollector(t, testConfig(server.URL, "none"))
	first, err := client.collect(context.Background())
	require.Error(t, err)
	assert.False(t, first.Complete)
	assert.Equal(t, map[string]int{"discovered": 2, "readable": 2}, first.Metrics.Resources)
	phase.Store(1)
	second, err := client.collect(context.Background())
	require.Error(t, err)
	assert.Equal(t, map[string]int{"discovered": 2, "readable": 1, "unknown": 1}, second.Metrics.Resources)
	for _, metric := range second.Hardware {
		assert.NotEqual(t, "system_health", metric.Metric, "previous health must never be replayed")
	}
	phase.Store(2)
	third, err := client.collect(context.Background())
	require.NoError(t, err)
	assert.True(t, third.Complete)
	assert.Equal(t, map[string]int{"discovered": 1, "readable": 1}, third.Metrics.Resources)
	for _, metric := range third.Hardware {
		assert.False(t, strings.HasPrefix(metric.Metric, "system_"), "authoritatively removed system must not survive")
	}
}

func TestMalformedBaseLinkRetainsUnknownMembership(t *testing.T) {
	var malformed atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redfish/v1/":
			systems := any(testutil.Link("/redfish/v1/Systems"))
			if malformed.Load() {
				systems = map[string]any{"Name": "missing link"}
			}
			testutil.WriteJSON(w, testutil.Resource(r.URL.Path, "ServiceRoot", "Root", map[string]any{
				"RedfishVersion": "1.20.0", "Systems": systems,
			}))
		case "/redfish/v1/Systems":
			testutil.ServeCollection(w, r.URL.Path, "/redfish/v1/Systems/1")
		case "/redfish/v1/Systems/1":
			testutil.ServeBaseResource(w, r.URL.Path, "#ComputerSystem.v1_0_0.ComputerSystem", "System")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newTestCollector(t, testConfig(server.URL, "none"))
	result, err := client.collect(t.Context())
	require.NoError(t, err)
	assert.True(t, result.Complete)
	malformed.Store(true)
	result, err = client.collect(t.Context())
	require.Error(t, err)
	assert.Equal(t, "partial", result.Metrics.Status)
	assert.Equal(t, 1, result.Metrics.Resources["unknown"])
	for _, observation := range result.Hardware {
		if observation.Metric == "system_acquisition_state" {
			assert.Equal(t, "unknown", observation.State)
		}
		assert.NotEqual(t, "system_health", observation.Metric)
	}
}
