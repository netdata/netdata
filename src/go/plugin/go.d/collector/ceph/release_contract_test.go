// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/ceph/cephfunc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

const (
	wireAcceptV1  = "application/vnd.ceph.api.v1.0+json"
	wireAcceptV11 = "application/vnd.ceph.api.v1.1+json"
	wireAcceptV2  = "application/vnd.ceph.api.v2.0+json"

	wireOSDWholeListQuery = "limit=-1&offset=0&sort=%2Bid"
	wireOSDFunctionQuery  = "limit=500&offset=0&sort=%2Bid"
	wirePoolPolicyQuery   = "attrs=pool_name%2Ctype%2Csize%2Cmin_size%2Cpg_num%2Cpg_placement_num%2Cpg_autoscale_mode%2Ccrush_rule%2Capplication_metadata%2Cerasure_code_profile%2Cquota_max_bytes%2Cquota_max_objects%2Cflags_names&stats=false"
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

type wireRequest struct {
	Method        string
	Path          string
	Accept        string
	ContentType   string
	Query         string
	Authenticated bool
}

type wireRecorder struct {
	mu       sync.Mutex
	requests []wireRequest
}

func (r *wireRecorder) observe(req *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, wireRequest{
		Method:        req.Method,
		Path:          req.URL.Path,
		Accept:        req.Header.Get("Accept"),
		ContentType:   req.Header.Get("Content-Type"),
		Query:         req.URL.Query().Encode(),
		Authenticated: req.Header.Get("Authorization") != "",
	})
}

func (r *wireRecorder) snapshot() []wireRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]wireRequest(nil), r.requests...)
}

func TestTargetReleaseDashboardContracts(t *testing.T) {
	tests := map[string]struct {
		legacyOSD bool
	}{
		"17.2.7": {legacyOSD: true},
		"18.2.8": {},
		"19.2.5": {},
		"20.2.2": {},
	}

	for release, test := range tests {
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

			srv, recorder := newReleaseContractServer(fixture, test.legacyOSD)
			defer srv.Close()
			c := newInitializedCollector(t, srv.URL, nil)
			defer c.Cleanup(context.Background())

			require.NoError(t, c.Check(context.Background()))
			require.NoError(t, collectOnce(c))
			requireComponentStatus(t, c, "synthetic-fsid", "health", "success")
			requireMetric(t, c, "cluster_physical_capacity_bytes", metrix.Labels{
				"fsid": "synthetic-fsid", "state": "used",
			}, 250000)
			requireMetric(t, c, "cluster_client_io_bytes_per_sec", metrix.Labels{
				"fsid": "synthetic-fsid", "direction": "read",
			}, 10.25)
			requireMetric(t, c, "osd_io_bytes_per_sec", metrix.Labels{
				"fsid": "synthetic-fsid", "osd_uuid": "00000000-0000-4000-8000-000000000001",
				"osd_name": "osd.0", "device_class": "ssd", "direction": "read",
			}, 1024.25)
			requireMetric(t, c, "pool_space_bytes", metrix.Labels{
				"fsid": "synthetic-fsid", "pool_name": "pool-a", "state": "avail",
			}, 900)
			requireMetric(t, c, "pool_space_utilization_percent", metrix.Labels{
				"fsid": "synthetic-fsid", "pool_name": "pool-a",
			}, 10)
			collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})

			for _, method := range cephfunc.Methods() {
				response := c.funcRouter.Handle(context.Background(), method.ID, nil)
				expectedStatus := http.StatusOK
				if test.legacyOSD && method.ID == cephfunc.MethodOSDs {
					expectedStatus = http.StatusUnsupportedMediaType
				}
				require.Equal(t, expectedStatus, response.Status, method.ID)
				if expectedStatus == http.StatusOK {
					assert.NotEmpty(t, response.Data, method.ID)
				} else {
					assert.Nil(t, response.Data, method.ID)
				}
			}
			if test.legacyOSD {
				assert.Equal(t, expectedQuincyRequests(), recorder.snapshot())
			} else {
				assert.Equal(t, expectedModernRequests(), recorder.snapshot())
			}
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
	var identity struct {
		MonStatus struct {
			MonMap struct {
				FSID string `json:"fsid"`
			} `json:"monmap"`
		} `json:"mon_status"`
	}
	require.NoError(t, json.Unmarshal(monitor, &identity))
	require.NotEmpty(t, identity.MonStatus.MonMap.FSID)
	recorder := &wireRecorder{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.observe(r)
		switch r.URL.Path {
		case "/api/health/get_cluster_fsid":
			w.WriteHeader(http.StatusNotFound)
			return
		case "/api/monitor":
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			writeFixtureJSON(w, monitor)
		case "/api/auth":
			writeJSON(w, http.StatusCreated, map[string]any{"token": "test-token"})
		case "/api/health/minimal":
			writeFixtureJSON(w, health)
		case "/api/osd":
			if r.Header.Get("Accept") == wireAcceptV11 {
				w.WriteHeader(http.StatusUnsupportedMediaType)
				return
			}
			writeFixtureJSON(w, osds)
		case "/api/pool":
			writeFixtureJSON(w, pools)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
	require.NoError(t, collectOnce(c))
	requireMetric(t, c, "pool_space_utilization_percent", metrix.Labels{
		"fsid": identity.MonStatus.MonMap.FSID, "pool_name": "mySuperPool",
	}, 100)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
	assert.Equal(t, expectedPacificRequests(), recorder.snapshot())
}

func loadReleaseContract(t *testing.T, release string) releaseContractFixture {
	t.Helper()
	bs, err := os.ReadFile("testdata/v" + release + "/dashboard_contract.json")
	require.NoError(t, err)
	var fixture releaseContractFixture
	require.NoError(t, json.Unmarshal(bs, &fixture))
	return fixture
}

func newReleaseContractServer(fixture releaseContractFixture, legacyOSD bool) (*httptest.Server, *wireRecorder) {
	recorder := &wireRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder.observe(r)
		switch r.URL.Path {
		case "/api/health/get_cluster_fsid":
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			writeJSON(w, http.StatusOK, "synthetic-fsid")
		case "/api/auth":
			writeJSON(w, http.StatusCreated, map[string]any{"token": "test-token"})
		case "/api/health/minimal":
			writeFixtureJSON(w, fixture.HealthMinimal)
		case "/api/osd":
			if legacyOSD && r.Header.Get("Accept") == wireAcceptV11 {
				w.WriteHeader(http.StatusUnsupportedMediaType)
				return
			}
			if !legacyOSD {
				w.Header().Set("X-Total-Count", "1")
			}
			writeFixtureJSON(w, fixture.OSDs)
		case "/api/pool":
			if r.URL.Query().Get("stats") == "false" {
				writeFixtureJSON(w, fixture.PoolsPolicy)
			} else {
				writeFixtureJSON(w, fixture.PoolsStats)
			}
		case "/api/crush_rule":
			writeFixtureJSON(w, fixture.CrushRules)
		case "/api/daemon":
			writeFixtureJSON(w, fixture.Daemons)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return server, recorder
}

func writeFixtureJSON(w http.ResponseWriter, payload []byte) {
	if len(payload) == 0 {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func expectedModernRequests() []wireRequest {
	return []wireRequest{
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", false),
		postWire("/api/auth"),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/health/minimal", wireAcceptV1, "", true),
		getWire("/api/osd", wireAcceptV11, wireOSDWholeListQuery, true),
		getWire("/api/pool", wireAcceptV1, "stats=true", true),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/health/minimal", wireAcceptV1, "", true),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/osd", wireAcceptV11, wireOSDFunctionQuery, true),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/pool", wireAcceptV1, wirePoolPolicyQuery, true),
		getWire("/api/crush_rule", wireAcceptV2, "", true),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/daemon", wireAcceptV1, "", true),
	}
}

func expectedQuincyRequests() []wireRequest {
	return []wireRequest{
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", false),
		postWire("/api/auth"),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/health/minimal", wireAcceptV1, "", true),
		getWire("/api/osd", wireAcceptV11, wireOSDWholeListQuery, true),
		getWire("/api/osd", wireAcceptV1, "", true),
		getWire("/api/pool", wireAcceptV1, "stats=true", true),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/health/minimal", wireAcceptV1, "", true),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/osd", wireAcceptV11, wireOSDFunctionQuery, true),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/pool", wireAcceptV1, wirePoolPolicyQuery, true),
		getWire("/api/crush_rule", wireAcceptV2, "", true),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/daemon", wireAcceptV1, "", true),
	}
}

func expectedPacificRequests() []wireRequest {
	return []wireRequest{
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", false),
		getWire("/api/monitor", wireAcceptV1, "", false),
		postWire("/api/auth"),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/monitor", wireAcceptV1, "", true),
		getWire("/api/health/get_cluster_fsid", wireAcceptV1, "", true),
		getWire("/api/monitor", wireAcceptV1, "", true),
		getWire("/api/health/minimal", wireAcceptV1, "", true),
		getWire("/api/osd", wireAcceptV11, wireOSDWholeListQuery, true),
		getWire("/api/osd", wireAcceptV1, "", true),
		getWire("/api/pool", wireAcceptV1, "stats=true", true),
	}
}

func getWire(path, accept, query string, authenticated bool) wireRequest {
	return wireRequest{
		Method:        http.MethodGet,
		Path:          path,
		Accept:        accept,
		Query:         query,
		Authenticated: authenticated,
	}
}

func postWire(path string) wireRequest {
	return wireRequest{
		Method:        http.MethodPost,
		Path:          path,
		Accept:        wireAcceptV1,
		ContentType:   "application/json",
		Authenticated: false,
	}
}
