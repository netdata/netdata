// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

type releaseContractFixture struct {
	Release       string          `json:"release"`
	HealthMinimal json.RawMessage `json:"health_minimal"`
	OSDs          json.RawMessage `json:"osds"`
	PoolsStats    json.RawMessage `json:"pools_stats"`
	PoolsPolicy   json.RawMessage `json:"pools_policy"`
	CrushRules    json.RawMessage `json:"crush_rules"`
	Daemons       json.RawMessage `json:"daemons"`
}

func TestTargetReleaseDashboardContracts(t *testing.T) {
	for _, release := range []string{"18.2.8", "19.2.5", "20.2.2"} {
		t.Run(release, func(t *testing.T) {
			fixture := loadReleaseContract(t, release)
			assert.Equal(t, release, fixture.Release)

			var health apiHealthMinimalResponse
			require.NoError(t, json.Unmarshal(fixture.HealthMinimal, &health))
			require.NotNil(t, health.Health)
			require.NotNil(t, health.PgInfo)
			assert.Positive(t, health.PgInfo.ObjectStats.NumObjectCopies)

			var osds []apiOsdResponse
			require.NoError(t, json.Unmarshal(fixture.OSDs, &osds))
			require.Len(t, osds, 1)
			assert.NotEmpty(t, osds[0].UUID)

			var pools []apiPoolResponse
			require.NoError(t, json.Unmarshal(fixture.PoolsStats, &pools))
			require.Len(t, pools, 1)
			assert.Equal(t, "pool-a", pools[0].PoolName)

			srv := newReleaseContractServer(t, fixture)
			defer srv.Close()
			c := newInitializedCollector(t, srv.URL, nil)
			defer c.Cleanup(context.Background())

			mx, err := c.collect(context.Background())
			require.NoError(t, err)
			assert.EqualValues(t, 0, mx["health_collection_failed"])
			assert.EqualValues(t, 250000, mx["raw_capacity_used_bytes"])
			assert.EqualValues(t, 10250, mx["client_perf_read_bytes_sec"])
			assert.EqualValues(t, 1024250, mx["osd_00000000-0000-4000-8000-000000000001_read_bytes"])
			assert.EqualValues(t, 900, mx["pool_pool-a_space_avail_bytes"])
			assert.EqualValues(t, 10000, mx["pool_pool-a_space_utilization"])
			collecttest.TestMetricsHasAllChartsDims(t, c.Charts(), mx)

			deps := funcDepsAdapter{collector: c}
			healthResult, err := deps.Health(context.Background(), 500)
			require.NoError(t, err)
			assert.NotEmpty(t, healthResult.Rows)
			osdResult, err := deps.OSDs(context.Background(), 500)
			require.NoError(t, err)
			assert.NotEmpty(t, osdResult.Rows)
			poolResult, err := deps.Pools(context.Background(), 500)
			require.NoError(t, err)
			assert.NotEmpty(t, poolResult.Rows)
			daemonResult, err := deps.Daemons(context.Background(), 500)
			require.NoError(t, err)
			assert.NotEmpty(t, daemonResult.Rows)
		})
	}
}

func TestLegacyPacificDashboardFixturesRemainParseable(t *testing.T) {
	read := func(name string) []byte {
		t.Helper()
		bs, err := os.ReadFile("testdata/v16.2.15/" + name)
		require.NoError(t, err)
		return bs
	}

	var health apiHealthMinimalResponse
	require.NoError(t, json.Unmarshal(read("api_health_minimal.json"), &health))
	require.NotNil(t, health.Health)
	require.NotNil(t, health.PgInfo)

	var monitor apiHealthMinimalResponse
	require.NoError(t, json.Unmarshal(read("api_monitor.json"), &monitor))
	require.NotNil(t, monitor.MonStatus)

	var osds []apiOsdResponse
	require.NoError(t, json.Unmarshal(read("api_osd.json"), &osds))
	require.NotEmpty(t, osds)
	require.NoError(t, validateOSDs(osds))

	var pools []apiPoolResponse
	require.NoError(t, json.Unmarshal(read("api_pool_stats.json"), &pools))
	require.NotEmpty(t, pools)
	for _, pool := range pools {
		_, err := poolSample(pool)
		require.NoError(t, err, pool.PoolName)
	}
}

func TestLegacyPacificDashboardContractEndToEnd(t *testing.T) {
	read := func(name string) []byte {
		t.Helper()
		bs, err := os.ReadFile("testdata/v16.2.15/" + name)
		require.NoError(t, err)
		return bs
	}
	monitor := read("api_monitor.json")
	health := read("api_health_minimal.json")
	osds := read("api_osd.json")
	pools := read("api_pool_stats.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response []byte
		switch r.URL.Path {
		case urlPathAPIClusterFSID:
			w.WriteHeader(http.StatusNotFound)
			return
		case urlPathApiMonitor:
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			response = monitor
		case urlPathApiAuth:
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"token":"test-token"}`)
			return
		case urlPathApiHealthMinimal:
			response = health
		case urlPathApiOsd:
			if r.Header.Get("Accept") == hdrAcceptVersionV11 {
				w.WriteHeader(http.StatusUnsupportedMediaType)
				return
			}
			response = osds
		case urlPathApiPool:
			response = pools
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(response)
	}))
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
	mx, err := c.collect(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, mx)
	assert.NotEmpty(t, c.clusterFSID())
	collecttest.TestMetricsHasAllChartsDims(t, c.Charts(), mx)
}

func loadReleaseContract(t *testing.T, release string) releaseContractFixture {
	t.Helper()
	bs, err := os.ReadFile("testdata/v" + release + "/dashboard_contract.json")
	require.NoError(t, err)
	var fixture releaseContractFixture
	require.NoError(t, json.Unmarshal(bs, &fixture))
	return fixture
}

func newReleaseContractServer(t *testing.T, fixture releaseContractFixture) *httptest.Server {
	t.Helper()
	return newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		var response json.RawMessage
		switch r.URL.Path {
		case urlPathApiHealthMinimal:
			assert.Equal(t, hdrAcceptVersion, r.Header.Get("Accept"))
			response = fixture.HealthMinimal
		case urlPathApiOsd:
			assert.Equal(t, hdrAcceptVersionV11, r.Header.Get("Accept"))
			assert.Equal(t, "0", r.URL.Query().Get("offset"))
			assert.Equal(t, "+id", r.URL.Query().Get("sort"))
			assert.NotEmpty(t, r.URL.Query().Get("limit"))
			w.Header().Set("X-Total-Count", "1")
			response = fixture.OSDs
		case urlPathApiPool:
			if r.URL.Query().Get("stats") == "true" {
				assert.Equal(t, hdrAcceptVersion, r.Header.Get("Accept"))
				response = fixture.PoolsStats
			} else {
				assert.Equal(t, "false", r.URL.Query().Get("stats"))
				assert.Contains(t, r.URL.Query().Get("attrs"), "pg_autoscale_mode")
				response = fixture.PoolsPolicy
			}
		case urlPathAPICrushRule:
			assert.Equal(t, hdrAcceptVersionV2, r.Header.Get("Accept"))
			response = fixture.CrushRules
		case urlPathAPIDaemon:
			response = fixture.Daemons
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write(response)
		require.NoError(t, err)
	})
}
