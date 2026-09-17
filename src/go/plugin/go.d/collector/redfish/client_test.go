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

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const asyncTestTimeout = 10 * time.Second

func TestProtocolClientBasicCheckAndCollect(t *testing.T) {
	t.Parallel()

	server := newRedfishTestServer(t, redfishTestServerConfig{
		requireBasic: true,
	})
	defer server.Close()

	cfg := testConfig(server.URL, "basic")
	client := newTestProtocolClient(t, cfg)
	require.NoError(t, client.Check(context.Background()))

	result, err := client.Collect(context.Background())
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

func TestProtocolClientCancellationMarksUnvisitedMembersUnknown(t *testing.T) {
	t.Parallel()

	blocked := make(chan struct{}, 2)
	server := newResourceTestServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/Systems/1" {
			select {
			case blocked <- struct{}{}:
			case <-r.Context().Done():
				return
			}
			<-r.Context().Done()
			return
		}
		serveBaseResource(w, r.URL.Path, "#ComputerSystem.v1_20_0.ComputerSystem", r.URL.Path)
	}))
	defer server.Close()

	client := newTestResourceClient(t, testConfig(server.URL, "none"))
	recorder := &requestRecordingTransport{
		base: client.http.Transport,
	}
	client.http.Transport = recorder
	members := []collectionMember{
		{Ref: redfishLink{
			ODataID: "/redfish/v1/Systems/1",
		}},
		{Ref: redfishLink{
			ODataID: "/redfish/v1/Systems/2",
		}},
		{Ref: redfishLink{
			ODataID: "/redfish/v1/Systems/3",
		}},
	}
	for range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			resources, err := client.fetchBaseMembers(ctx, "system", members, nil)
			assert.Equal(t, []baseResource{
				{Kind: "system", URI: "/redfish/v1/Systems/1", AcquisitionState: "unreadable"},
				{Kind: "system", URI: "/redfish/v1/Systems/2", AcquisitionState: "unknown"},
				{Kind: "system", URI: "/redfish/v1/Systems/3", AcquisitionState: "unknown"},
			}, resources)
			result <- err
		}()
		select {
		case <-blocked:
		case <-time.After(asyncTestTimeout):
			cancel()
			t.Fatal("timed out waiting for blocked Redfish member request")
		}
		cancel()
		select {
		case err := <-result:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(asyncTestTimeout):
			t.Fatal("timed out waiting for canceled Redfish member collection")
		}
	}
	assert.Equal(t, []string{"/redfish/v1/Systems/1", "/redfish/v1/Systems/1"}, recorder.paths())
}

func TestProtocolClientRetainsLastCompleteMembershipWithoutReplayingCurrentState(t *testing.T) {
	t.Parallel()

	var failSystems atomic.Bool
	server := newRedfishTestServer(t, redfishTestServerConfig{
		failSystems: &failSystems,
	})
	defer server.Close()

	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	first, err := client.Collect(context.Background())
	require.NoError(t, err)
	require.True(t, first.Complete)

	failSystems.Store(true)
	second, err := client.Collect(context.Background())
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
			serveServiceRoot(w, false)
		case "/redfish/v1/Systems":
			switch phase.Load() {
			case 0:
				writeJSON(w, map[string]any{
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
				serveCollection(w, r.URL.Path)
			}
		case "/redfish/v1/Systems/1":
			serveBaseResource(w, r.URL.Path, "#ComputerSystem.v1_21_0.ComputerSystem", "System")
		case "/redfish/v1/Chassis", "/redfish/v1/Managers":
			serveCollection(w, r.URL.Path)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	first, err := client.Collect(context.Background())
	require.Error(t, err)
	assert.False(t, first.Complete)
	assert.Equal(t, map[string]int{"discovered": 2, "readable": 2}, first.Metrics.Resources)
	phase.Store(1)
	second, err := client.Collect(context.Background())
	require.Error(t, err)
	assert.Equal(t, map[string]int{"discovered": 2, "readable": 1, "unknown": 1}, second.Metrics.Resources)
	for _, metric := range second.Hardware {
		assert.NotEqual(t, "system_health", metric.Metric, "previous health must never be replayed")
	}
	phase.Store(2)
	third, err := client.Collect(context.Background())
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
			systems := any(sourceTestLink("/redfish/v1/Systems"))
			if malformed.Load() {
				systems = map[string]any{"Name": "missing link"}
			}
			writeJSON(w, sourceTestResource(r.URL.Path, "ServiceRoot", "Root", map[string]any{
				"RedfishVersion": "1.20.0", "Systems": systems,
			}))
		case "/redfish/v1/Systems":
			serveCollection(w, r.URL.Path, "/redfish/v1/Systems/1")
		case "/redfish/v1/Systems/1":
			serveBaseResource(w, r.URL.Path, "#ComputerSystem.v1_0_0.ComputerSystem", "System")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newTestProtocolClient(t, testConfig(server.URL, "none"))
	result, err := client.Collect(t.Context())
	require.NoError(t, err)
	assert.True(t, result.Complete)
	malformed.Store(true)
	result, err = client.Collect(t.Context())
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
