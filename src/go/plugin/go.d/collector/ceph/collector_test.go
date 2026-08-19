// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/ceph/cephfunc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Interfaces(t *testing.T) {
	assert.Implements(t, (*collectorapi.CollectorV2)(nil), New())
	assert.Implements(t, (*collectorapi.FunctionAvailability)(nil), New())
	_, v1 := any(New()).(collectorapi.CollectorV1)
	assert.False(t, v1)
}

func TestCollector_SharedFunctionSurface(t *testing.T) {
	var ids []string
	for _, method := range cephfunc.Methods() {
		ids = append(ids, method.ID)
	}
	assert.Equal(t, []string{"health", "osds", "pools", "daemons"}, ids)
}

func TestCollector_Defaults(t *testing.T) {
	c := New()
	assert.Equal(t, "https://127.0.0.1:8443", c.URL)
	assert.Equal(t, 2*time.Second, c.Timeout.Duration())
	assert.Empty(t, c.AllowedRedirectOrigins)
	assert.Equal(t, 100, c.MaxOSDs)
	assert.Equal(t, 100, c.MaxPools)
	assert.Equal(t, "*", c.OSDSelector)
	assert.Equal(t, "*", c.PoolSelector)
}

func TestCollector_ConfigSchemaMatchesMetadata(t *testing.T) {
	collecttest.AssertConfigSchemaMatchesMetadata(t, "config_schema.json", "metadata.yaml")
}

func TestCollector_LegacyConfigPreservesFullCollectionContract(t *testing.T) {
	c := New()
	require.NoError(t, yaml.Unmarshal([]byte(`
url: https://ceph.example:8443
username: netdata
password: test-password
`), c))

	config, err := yaml.Marshal(c.Configuration())
	require.NoError(t, err)
	assert.NotContains(t, string(config), "collect:")
	assert.NotContains(t, string(config), "functions:")
}

func TestCollector_InitValidation(t *testing.T) {
	valid := func() Config {
		cfg := New().Config
		cfg.Username = "netdata"
		cfg.Password = "test-password"
		return cfg
	}

	tests := map[string]struct {
		modify   func(*Config)
		wantFail bool
	}{
		"valid username and password": {},
		"valid bearer token file": {
			modify: func(cfg *Config) {
				cfg.Username = ""
				cfg.Password = ""
				cfg.BearerTokenFile = "/synthetic/token"
			},
		},
		"missing credentials": {
			modify:   func(cfg *Config) { cfg.Username, cfg.Password = "", "" },
			wantFail: true,
		},
		"partial credentials": {
			modify:   func(cfg *Config) { cfg.Password = "" },
			wantFail: true,
		},
		"URL userinfo": {
			modify:   func(cfg *Config) { cfg.URL = "https://user:example" + "@" + "ceph.example" },
			wantFail: true,
		},
		"redirect origin with path": {
			modify: func(cfg *Config) {
				cfg.AllowedRedirectOrigins = []string{"https://mgr-b.example:8443/dashboard"}
			},
			wantFail: true,
		},
		"reserved Authorization header": {
			modify:   func(cfg *Config) { cfg.Headers = map[string]string{"Authorization": "secret"} },
			wantFail: true,
		},
		"generic method": {
			modify:   func(cfg *Config) { cfg.Method = http.MethodPost },
			wantFail: true,
		},
		"function only is atomic": {
			modify: func(cfg *Config) { cfg.FunctionOnly = true },
		},
		"non-positive OSD cap": {
			modify:   func(cfg *Config) { cfg.MaxOSDs = 0 },
			wantFail: true,
		},
		"non-positive pool cap": {
			modify:   func(cfg *Config) { cfg.MaxPools = 0 },
			wantFail: true,
		},
		"unbounded job timeout": {
			modify:   func(cfg *Config) { cfg.Timeout = 0 },
			wantFail: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := valid()
			if test.modify != nil {
				test.modify(&cfg)
			}
			c := New()
			c.Config = cfg
			err := c.Init(context.Background())
			if test.wantFail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				c.Cleanup(context.Background())
			}
		})
	}
}

func TestCollector_V2CollectionAndChartCoverage(t *testing.T) {
	fixture := loadReleaseContract(t, "18.2.8")
	srv, _ := newReleaseContractServer(fixture, false)
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
	require.NoError(t, collectOnce(c))

	requireComponentStatus(t, c, "synthetic-fsid", "health", "success")
	requireMetric(t, c, "cluster_physical_capacity_bytes", metrix.Labels{
		"fsid": "synthetic-fsid", "state": "used",
	}, 250000)
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

	state, ok := c.store.Read().StateSet("cluster_health_status", metrix.Labels{"fsid": "synthetic-fsid"})
	require.True(t, ok)
	assert.False(t, state.States["ok"])
	assert.True(t, state.States["warn"])
	assert.False(t, state.States["err"])

	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{
		RequiredContexts: map[string][]string{
			"ceph.component_collection_status":           {"success", "failed"},
			"ceph.cluster_physical_capacity_utilization": {"utilization"},
			"ceph.osd_status":                            {"up", "down", "in", "out"},
			"ceph.pool_io":                               {"read", "written"},
		},
	})

	chartFamilies := make(map[string]string)
	componentCharts := make(map[string]map[string]string)
	var clusterWrite, osdRead, poolRead *chartengine.CreateDimensionAction
	for _, action := range prepareChartPlan(t, c).Actions {
		switch action := action.(type) {
		case chartengine.CreateChartAction:
			chartFamilies[action.Meta.Context] = action.Meta.Family
			if action.Meta.Context == "ceph.component_collection_status" {
				componentCharts[action.ChartID] = action.Labels
			}
		case chartengine.CreateDimensionAction:
			switch {
			case action.ChartMeta.Context == "ceph.cluster_client_io" && action.Name == "written":
				clusterWrite = &action
			case action.ChartMeta.Context == "ceph.osd_io" && action.Name == "read":
				osdRead = &action
			case action.ChartMeta.Context == "ceph.pool_io" && action.Name == "read":
				poolRead = &action
			}
		}
	}
	assert.Equal(t, expectedChartFamilies(), chartFamilies)
	assert.Equal(t, map[string]map[string]string{
		"component_collection_status_health": {"component": "health", "fsid": "synthetic-fsid"},
		"component_collection_status_osds":   {"component": "osds", "fsid": "synthetic-fsid"},
		"component_collection_status_pools":  {"component": "pools", "fsid": "synthetic-fsid"},
	}, componentCharts)
	require.NotNil(t, clusterWrite)
	assert.Equal(t, chartengine.AlgorithmAbsolute, clusterWrite.Algorithm)
	assert.Equal(t, -1, clusterWrite.Multiplier)
	assert.True(t, clusterWrite.Float)
	require.NotNil(t, osdRead)
	assert.Equal(t, chartengine.AlgorithmAbsolute, osdRead.Algorithm)
	assert.True(t, osdRead.Float)
	require.NotNil(t, poolRead)
	assert.Equal(t, chartengine.AlgorithmIncremental, poolRead.Algorithm)
}

func TestCollector_DefaultCollectionRequestsFullMetricSet(t *testing.T) {
	var pathsMu sync.Mutex
	paths := make(map[string]int)
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		pathsMu.Lock()
		paths[r.URL.Path]++
		pathsMu.Unlock()
		switch r.URL.Path {
		case urlPathApiHealthMinimal:
			writeJSON(w, http.StatusOK, map[string]any{"health": map[string]any{"status": "HEALTH_OK"}})
		case urlPathApiOsd:
			w.Header().Set("X-Total-Count", "0")
			writeJSON(w, http.StatusOK, []any{})
		case urlPathApiPool:
			writeJSON(w, http.StatusOK, []any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
	require.NoError(t, collectOnce(c))

	pathsMu.Lock()
	defer pathsMu.Unlock()
	assert.Positive(t, paths[urlPathApiHealthMinimal])
	assert.Positive(t, paths[urlPathApiOsd])
	assert.Positive(t, paths[urlPathApiPool])
}

func TestCollector_FunctionOnlyChecksIdentityWithoutMetrics(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.FunctionOnly = true
	})
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
	require.NoError(t, collectOnce(c))

	var count int
	c.store.Read(metrix.ReadFlatten()).ForEachSeries(func(string, metrix.LabelView, metrix.SampleValue) {
		count++
	})
	assert.Zero(t, count)
}

func TestCollector_CheckFallsBackToLegacyMonitorIdentity(t *testing.T) {
	monitor, err := os.ReadFile("testdata/v16.2.15/api_monitor.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIClusterFSID:
			w.WriteHeader(http.StatusNotFound)
		case urlPathApiMonitor:
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(monitor)
		case urlPathApiAuth:
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"token":"test-token"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
}

func TestCollectorCleanupDoesNotLogout(t *testing.T) {
	var logoutRequests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIClusterFSID:
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = io.WriteString(w, `"synthetic-fsid"`)
		case urlPathApiAuth:
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"token":"test-token"}`)
		case "/api/auth/logout":
			logoutRequests.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	require.NoError(t, c.Check(context.Background()))
	c.Cleanup(context.Background())
	assert.Zero(t, logoutRequests.Load())
}

func TestCollector_PartialFailureCommitsDataAndComponentStatus(t *testing.T) {
	health, err := os.ReadFile("testdata/v16.2.15/api_health_minimal.json")
	require.NoError(t, err)
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiHealthMinimal:
			_, _ = w.Write(health)
		case urlPathApiOsd:
			w.WriteHeader(http.StatusInternalServerError)
		case urlPathApiPool:
			_, _ = io.WriteString(w, `[]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, collectOnce(c))

	requireComponentStatus(t, c, "synthetic-fsid", "health", "success")
	requireComponentStatus(t, c, "synthetic-fsid", "osds", "failed")
	requireComponentStatus(t, c, "synthetic-fsid", "pools", "success")
	_, ok := c.store.Read().StateSet("cluster_health_status", metrix.Labels{"fsid": "synthetic-fsid"})
	assert.True(t, ok)
	assert.Equal(t, map[string]float64{"success": 0, "failed": 1},
		chartUpdateValues(t, prepareChartPlan(t, c), "component_collection_status_osds"))
}

func TestCollector_FullyFailedCycleAbortsWithoutPublishingMetrics(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	err := collectOnce(c)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collect health features")

	var count int
	c.store.Read(metrix.ReadFlatten()).ForEachSeries(func(string, metrix.LabelView, metrix.SampleValue) {
		count++
	})
	assert.Zero(t, count)
}

func TestCollector_CanceledCycleAbortsStagedPartialMetrics(t *testing.T) {
	var cancel context.CancelFunc
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiHealthMinimal:
			writeJSON(w, http.StatusOK, map[string]any{"health": map[string]any{"status": "HEALTH_OK"}})
		case urlPathApiOsd:
			cancel()
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	ctx, cancelFn := context.WithCancel(context.Background())
	cancel = cancelFn
	defer cancelFn()

	cycle := mustCycleController(t, c.store)
	cycle.BeginCycle()
	err := c.Collect(ctx)
	cycle.AbortCycle()
	require.ErrorIs(t, err, context.Canceled)

	var count int
	c.store.Read(metrix.ReadFlatten()).ForEachSeries(func(string, metrix.LabelView, metrix.SampleValue) {
		count++
	})
	assert.Zero(t, count)
}

func TestCollector_HealthCountsSkipUnboundedEntityRequests(t *testing.T) {
	osds := make([]map[string]any, 101)
	for i := range osds {
		osds[i] = map[string]any{"up": 1, "in": 1}
	}
	pools := make([]map[string]any, 101)
	var osdRequests, poolRequests atomic.Int64
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiHealthMinimal:
			writeJSON(w, http.StatusOK, map[string]any{
				"health":  map[string]any{"status": "HEALTH_OK"},
				"osd_map": map[string]any{"osds": osds},
				"pools":   pools,
			})
		case urlPathApiOsd:
			osdRequests.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		case urlPathApiPool:
			poolRequests.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, collectOnce(c))

	requireMetric(t, c, "cluster_osds", metrix.Labels{"fsid": "synthetic-fsid"}, 101)
	requireMetric(t, c, "cluster_pools", metrix.Labels{"fsid": "synthetic-fsid"}, 101)
	assert.Zero(t, osdRequests.Load())
	assert.Zero(t, poolRequests.Load())
	requireComponentStatus(t, c, "synthetic-fsid", "osds", "success")
	requireComponentStatus(t, c, "synthetic-fsid", "pools", "success")
}

func TestCollector_OSDWholeListCapIsAllOrNone(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiHealthMinimal:
			writeJSON(w, http.StatusOK, map[string]any{"health": map[string]any{"status": "HEALTH_OK"}})
		case urlPathApiOsd:
			osds := make([]apiOsdResponse, 3)
			for i := range osds {
				osds[i] = validOSDResponse(int64(i + 1))
			}
			w.Header().Set("X-Total-Count", "3")
			writeJSON(w, http.StatusOK, osds)
		case urlPathApiPool:
			writeJSON(w, http.StatusOK, []any{})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.MaxOSDs = 2
	})
	defer c.Cleanup(context.Background())
	require.NoError(t, collectOnce(c))

	var osdSeries int
	c.store.Read(metrix.ReadFlatten()).ForEachSeries(func(name string, _ metrix.LabelView, _ metrix.SampleValue) {
		if len(name) >= 4 && name[:4] == "osd_" {
			osdSeries++
		}
	})
	assert.Zero(t, osdSeries)
}

func TestCollector_DynamicPoolMetricsPreserveNamesWithKnownChartCollision(t *testing.T) {
	poolA := validPoolResponse("pool.a")
	poolB := validPoolResponse("pool_a")
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiHealthMinimal:
			writeJSON(w, http.StatusOK, map[string]any{"health": map[string]any{"status": "HEALTH_OK"}})
		case urlPathApiOsd:
			w.Header().Set("X-Total-Count", "0")
			writeJSON(w, http.StatusOK, []any{})
		case urlPathApiPool:
			writeJSON(w, http.StatusOK, []apiPoolResponse{poolA, poolB})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, collectOnce(c))

	requireMetric(t, c, "pool_space_bytes", metrix.Labels{
		"fsid": "synthetic-fsid", "pool_name": "pool.a", "state": "avail",
	}, 80)
	requireMetric(t, c, "pool_space_bytes", metrix.Labels{
		"fsid": "synthetic-fsid", "pool_name": "pool_a", "state": "avail",
	}, 80)
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})

	chartIDs := make(map[string]bool)
	for _, action := range prepareChartPlan(t, c).Actions {
		if action, ok := action.(chartengine.CreateChartAction); ok && action.Meta.Context == "ceph.pool_space_usage" {
			chartIDs[action.ChartID] = true
		}
	}
	// Chart IDs use a lossy wire-safe normalization. The underlying series stay
	// distinct, but these unlikely colliding names intentionally share a chart.
	assert.Equal(t, map[string]bool{"pool_space_usage_pool_a": true}, chartIDs)
}

func TestCollector_PoolsRejectDuplicateSelectedIdentity(t *testing.T) {
	pools := []apiPoolResponse{validPoolResponse("pool-a"), validPoolResponse("pool-a")}
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiHealthMinimal:
			writeJSON(w, http.StatusOK, map[string]any{"health": map[string]any{"status": "HEALTH_OK"}})
		case urlPathApiOsd:
			w.Header().Set("X-Total-Count", "0")
			writeJSON(w, http.StatusOK, []any{})
		case urlPathApiPool:
			writeJSON(w, http.StatusOK, pools)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, collectOnce(c))
	requireComponentStatus(t, c, "synthetic-fsid", "pools", "failed")
	_, ok := c.store.Read().Value("pool_objects", metrix.Labels{
		"fsid": "synthetic-fsid", "pool_name": pools[0].PoolName,
	})
	assert.False(t, ok)
}

func TestCollectObjectHealthAllowsIndependentCopyStates(t *testing.T) {
	c := New()
	cycle := mustCycleController(t, c.store)
	cycle.BeginCycle()
	err := c.collectObjectHealth(struct {
		NumObjects          int64 `json:"num_objects"`
		NumObjectCopies     int64 `json:"num_object_copies"`
		NumObjectsDegraded  int64 `json:"num_objects_degraded"`
		NumObjectsMisplaced int64 `json:"num_objects_misplaced"`
		NumObjectsUnfound   int64 `json:"num_objects_unfound"`
	}{
		NumObjects: 10, NumObjectCopies: 30, NumObjectsDegraded: 20,
		NumObjectsMisplaced: 20, NumObjectsUnfound: 10,
	}, c.metrics.clusterLabels("synthetic-fsid"))
	require.NoError(t, err)
	require.NoError(t, cycle.CommitCycleSuccess())

	requireMetric(t, c, "cluster_object_copies_health_percent", metrix.Labels{
		"fsid": "synthetic-fsid", "state": "degraded",
	}, 200.0/3)
	requireMetric(t, c, "cluster_object_copies_health_percent", metrix.Labels{
		"fsid": "synthetic-fsid", "state": "misplaced",
	}, 200.0/3)
	requireMetric(t, c, "cluster_objects_unfound_percent", metrix.Labels{
		"fsid": "synthetic-fsid",
	}, 100)
}

func TestCollector_ReefOSDUsesSingleWholeListRequest(t *testing.T) {
	var requests atomic.Int64
	var limit string
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathApiOsd {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		requests.Add(1)
		limit = r.URL.Query().Get("limit")
		w.Header().Set("X-Total-Count", "1")
		_, _ = io.WriteString(w, `[{"id":1,"uuid":"uuid-1"}]`)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	requireClusterIdentity(t, c)
	osds, err := c.fetchAllOSDs(context.Background())
	require.NoError(t, err)
	require.Len(t, osds, 1)
	assert.Equal(t, "-1", limit)
	assert.EqualValues(t, 1, requests.Load())
}

func TestCollector_OSDFallsBackToLegacyWholeListProtocol(t *testing.T) {
	legacy, err := os.ReadFile("testdata/v16.2.15/api_osd.json")
	require.NoError(t, err)

	var acceptsMu sync.Mutex
	var accepts []string
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathApiOsd {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		acceptsMu.Lock()
		accepts = append(accepts, r.Header.Get("Accept"))
		acceptsMu.Unlock()
		if r.Header.Get("Accept") == hdrAcceptVersionV11 {
			w.WriteHeader(http.StatusUnsupportedMediaType)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(legacy)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	requireClusterIdentity(t, c)
	osds, err := c.fetchAllOSDs(context.Background())
	require.NoError(t, err)
	assert.NotEmpty(t, osds)
	acceptsMu.Lock()
	defer acceptsMu.Unlock()
	assert.Equal(t, []string{hdrAcceptVersionV11, hdrAcceptVersion}, accepts)
}

func TestCollector_OSDWholeListCompleteness(t *testing.T) {
	tests := map[string]struct {
		advertised int
		rows       int
		wantError  string
	}{
		"supports more than ten thousand rows":  {advertised: 10001, rows: 10001},
		"rejects inconsistent advertised total": {advertised: 101, rows: 50, wantError: "advertised 101"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != urlPathApiOsd {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				page := make([]apiOsdResponse, test.rows)
				for i := range page {
					page[i].ID = int64(i)
					page[i].UUID = fmt.Sprintf("uuid-%05d", i)
				}
				w.Header().Set("X-Total-Count", strconv.Itoa(test.advertised))
				writeJSON(w, http.StatusOK, page)
			})
			defer srv.Close()

			c := newInitializedCollector(t, srv.URL, nil)
			defer c.Cleanup(context.Background())
			requireClusterIdentity(t, c)
			osds, err := c.fetchAllOSDs(context.Background())
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			assert.Len(t, osds, test.rows)
		})
	}
}

func TestValidateOSDsRejectsInvalidResponses(t *testing.T) {
	valid := validOSDResponse(1)
	duplicateID := valid
	duplicateID.UUID = "uuid-2"
	duplicateUUID := valid
	duplicateUUID.ID = 2
	invalidCapacity := valid
	invalidCapacity.OsdStats.Statfs.Available = 101
	negativeRate := valid
	negativeRate.Stats.OpR = -1

	tests := map[string]struct {
		osds     []apiOsdResponse
		wantFail bool
	}{
		"valid":                   {osds: []apiOsdResponse{valid}},
		"empty UUID":              {osds: []apiOsdResponse{{ID: 1}}, wantFail: true},
		"duplicate ID":            {osds: []apiOsdResponse{valid, duplicateID}, wantFail: true},
		"duplicate UUID":          {osds: []apiOsdResponse{valid, duplicateUUID}, wantFail: true},
		"available exceeds total": {osds: []apiOsdResponse{invalidCapacity}, wantFail: true},
		"negative rate":           {osds: []apiOsdResponse{negativeRate}, wantFail: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateOSDs(test.osds)
			if test.wantFail {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestPoolSampleRejectsInvalidUpstreamValues(t *testing.T) {
	tests := map[string]struct {
		objects     string
		percentUsed string
		wantObjects int64
		wantError   string
	}{
		"parses integers above float precision without intermediate loss": {
			objects: "9007199254740993", percentUsed: "0.5", wantObjects: 9007199254740993,
		},
		"rejects utilization above one": {
			objects: "0", percentUsed: "1.1", wantError: "0-1",
		},
		"rejects fractional object count": {
			objects: "1.5", percentUsed: "0.5", wantError: "objects",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pool := validPoolResponse("pool-a")
			pool.Stats.Objects.Latest = json.Number(test.objects)
			pool.Stats.PercentUsed.Latest = json.Number(test.percentUsed)

			sample, err := poolSample(pool)
			if test.wantError != "" {
				require.ErrorContains(t, err, test.wantError)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantObjects, sample.objects)
		})
	}
}

func TestCollector_ChartTemplateContract(t *testing.T) {
	template := New().ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, template)
	assert.NotContains(t, template, "priority:")
	assert.NotContains(t, template, "lifecycle:")
	assert.NotContains(t, template, "expire_after_cycles:")

	spec, err := charttpl.DecodeYAML([]byte(template))
	require.NoError(t, err)
	assert.Equal(t, "ceph", spec.ContextNamespace)
	require.Len(t, spec.Groups, 4)

	rootGroups := make(map[string]charttpl.Group, len(spec.Groups))
	for _, group := range spec.Groups {
		rootGroups[group.Family] = group
	}
	require.Contains(t, rootGroups, "Internal")
	require.Contains(t, rootGroups, "Cluster")
	require.Contains(t, rootGroups, "OSD")
	require.Contains(t, rootGroups, "Pool")

	require.NotNil(t, rootGroups["Internal"].ChartDefaults)
	require.NotNil(t, rootGroups["Internal"].ChartDefaults.Instances)
	assert.Equal(t, []string{"component"}, rootGroups["Internal"].ChartDefaults.Instances.ByLabels)
	assert.Equal(t, []string{"fsid"}, rootGroups["Internal"].ChartDefaults.LabelPromoted)
	require.NotNil(t, rootGroups["Cluster"].ChartDefaults)
	assert.Equal(t, []string{"fsid"}, rootGroups["Cluster"].ChartDefaults.LabelPromoted)
	require.NotNil(t, rootGroups["OSD"].ChartDefaults)
	require.NotNil(t, rootGroups["OSD"].ChartDefaults.Instances)
	assert.Equal(t, []string{"osd_uuid"}, rootGroups["OSD"].ChartDefaults.Instances.ByLabels)
	assert.Equal(t, []string{"fsid", "osd_name", "device_class"}, rootGroups["OSD"].ChartDefaults.LabelPromoted)
	require.NotNil(t, rootGroups["Pool"].ChartDefaults)
	require.NotNil(t, rootGroups["Pool"].ChartDefaults.Instances)
	assert.Equal(t, []string{"pool_name"}, rootGroups["Pool"].ChartDefaults.Instances.ByLabels)
	assert.Equal(t, []string{"fsid"}, rootGroups["Pool"].ChartDefaults.LabelPromoted)

	actualFamilies := make(map[string]string)
	var visit func(parent string, groups []charttpl.Group)
	visit = func(parent string, groups []charttpl.Group) {
		for _, group := range groups {
			family := group.Family
			if parent != "" {
				family = parent + "/" + family
			}
			for _, chart := range group.Charts {
				assert.Zero(t, chart.Priority)
				assert.Nil(t, chart.Lifecycle)
				assert.Empty(t, chart.Family)
				ctx := spec.ContextNamespace + "." + chart.Context
				if _, ok := actualFamilies[ctx]; ok {
					t.Fatalf("duplicate chart context %q", ctx)
				}
				actualFamilies[ctx] = family
			}
			visit(family, group.Groups)
		}
	}
	visit("", spec.Groups)
	assert.Equal(t, expectedChartFamilies(), actualFamilies)
}

func TestCollector_ComponentCollectionAlertContract(t *testing.T) {
	alert, err := os.ReadFile("../../../../../health/health.d/ceph.conf")
	require.NoError(t, err)

	config := string(alert)
	start := strings.Index(config, "template: ceph_component_collection_failed")
	require.NotEqual(t, -1, start)
	block := config[start:]
	if end := strings.Index(block, "\n template:"); end >= 0 {
		block = block[:end]
	}
	assert.Contains(t, block, "on: ceph.component_collection_status")
	assert.Contains(t, block, "calc: $failed")
	assert.Contains(t, block, "${label:component}")
	assert.NotContains(t, block, "$health_collection_failed")
	assert.NotContains(t, block, "$osd_collection_failed")
	assert.NotContains(t, block, "$pool_collection_failed")
}

func expectedChartFamilies() map[string]string {
	contextsByFamily := map[string][]string{
		"Internal": {
			"component_collection_status",
		},
		"Cluster/Status": {
			"cluster_status",
			"cluster_hosts_count",
			"cluster_monitors_count",
			"cluster_osds_count",
			"cluster_osds_by_status_count",
			"cluster_managers_count",
			"cluster_object_gateways_count",
			"cluster_iscsi_gateways_count",
			"cluster_iscsi_gateways_by_status_count",
		},
		"Cluster/Capacity": {
			"cluster_physical_capacity_utilization",
			"cluster_physical_capacity_usage",
			"cluster_objects_count",
			"cluster_object_copies_health",
			"cluster_objects_unfound",
			"cluster_pools_count",
			"cluster_pgs_count",
			"cluster_pgs_by_status_count",
			"cluster_pgs_per_osd_count",
		},
		"Cluster/Performance": {
			"cluster_client_io",
			"cluster_client_iops",
			"cluster_recovery_throughput",
			"cluster_scrub_status",
		},
		"OSD/Status": {
			"osd_status",
		},
		"OSD/Space": {
			"osd_space_usage",
		},
		"OSD/Operations": {
			"osd_io",
			"osd_iops",
			"osd_latency",
		},
		"Pool/Space": {
			"pool_space_utilization",
			"pool_space_usage",
		},
		"Pool/Objects": {
			"pool_objects_count",
		},
		"Pool/Operations": {
			"pool_io",
			"pool_iops",
		},
	}

	families := make(map[string]string)
	for family, contexts := range contextsByFamily {
		for _, ctx := range contexts {
			families["ceph."+ctx] = family
		}
	}
	return families
}

func TestPGStatusCategoryAllTargetReleaseStates(t *testing.T) {
	tests := map[string]struct {
		status string
		want   string
	}{
		"clean":         {status: "active+clean", want: "clean"},
		"premerge":      {status: "active+premerge", want: "working"},
		"wait":          {status: "active+wait", want: "working"},
		"failed repair": {status: "active+failed_repair", want: "warning"},
		"laggy":         {status: "active+laggy", want: "warning"},
		"new state":     {status: "active+new_state", want: "unknown"},
		"unknown":       {status: "unknown", want: "unknown"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, pgStatusCategory(test.status))
		})
	}
}

func validOSDResponse(id int64) apiOsdResponse {
	var osd apiOsdResponse
	osd.ID = id
	osd.UUID = fmt.Sprintf("uuid-%d", id)
	osd.Up, osd.In = 1, 1
	osd.Tree.Name = fmt.Sprintf("osd.%d", id)
	osd.Tree.DeviceClass = "ssd"
	osd.OsdStats.Statfs.Total = 100
	osd.OsdStats.Statfs.Available = 75
	osd.Stats.OpR, osd.Stats.OpW = 1, 2
	osd.Stats.OpOutBytes, osd.Stats.OpInBytes = 3, 4
	osd.OsdStats.PerfStat.CommitLatencyMs = 1.25
	osd.OsdStats.PerfStat.ApplyLatencyMs = 2.5
	return osd
}

func validPoolResponse(name string) apiPoolResponse {
	var value apiPoolResponse
	value.PoolName = name
	value.Stats.Objects.Latest = "5"
	value.Stats.AvailRaw.Latest = "80"
	value.Stats.BytesUsed.Latest = "20"
	value.Stats.PercentUsed.Latest = "0.2"
	value.Stats.Reads.Latest = "1"
	value.Stats.ReadBytes.Latest = "2"
	value.Stats.Writes.Latest = "3"
	value.Stats.WrittenBytes.Latest = "4"
	return value
}

func collectOnce(c *Collector) error {
	_, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	return err
}

func requireMetric(t *testing.T, c *Collector, name string, labels metrix.Labels, want float64) {
	t.Helper()
	got, ok := c.store.Read(metrix.ReadFlatten()).Value(name, labels)
	require.True(t, ok, "%s%v", name, labels)
	assert.InDelta(t, want, got, 1e-9, "%s%v", name, labels)
}

func requireComponentStatus(t *testing.T, c *Collector, fsid, component, want string) {
	t.Helper()
	require.Contains(t, []string{"success", "failed"}, want)
	state, ok := c.store.Read().StateSet("component_collection_status", metrix.Labels{
		"fsid": fsid, "component": component,
	})
	require.True(t, ok, "component_collection_status{fsid=%q,component=%q}", fsid, component)
	assert.Equal(t, map[string]bool{
		"success": want == "success",
		"failed":  want == "failed",
	}, state.States)
}

func chartUpdateValues(t *testing.T, plan chartengine.Plan, chartID string) map[string]float64 {
	t.Helper()
	for _, action := range plan.Actions {
		update, ok := action.(chartengine.UpdateChartAction)
		if !ok || update.ChartID != chartID {
			continue
		}
		values := make(map[string]float64, len(update.Values))
		for _, value := range update.Values {
			require.False(t, value.IsEmpty, value.Name)
			if value.IsFloat {
				values[value.Name] = value.Float64
			} else {
				values[value.Name] = float64(value.Int64)
			}
		}
		return values
	}
	require.FailNow(t, "chart update not found", chartID)
	return nil
}

func mustCycleController(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	return managed.CycleController()
}

func prepareChartPlan(t *testing.T, c *Collector) chartengine.Plan {
	t.Helper()
	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(c.ChartTemplateYAML()), 1))
	attempt, err := engine.PreparePlan(c.store.Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())
	return plan
}

func newFakeDashboard(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiAuth:
			writeJSON(w, http.StatusCreated, authLoginResp{Token: "test-token"})
			return
		case urlPathAPIClusterFSID:
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.Header.Get("Authorization") != "Bearer test-token" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			writeJSON(w, http.StatusOK, "synthetic-fsid")
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler(w, r)
	}))
}

func newInitializedCollector(t *testing.T, rawURL string, configure func(*Collector)) *Collector {
	t.Helper()
	c := New()
	c.URL = rawURL
	c.Username = "netdata"
	c.Password = "test-password"
	if configure != nil {
		configure(c)
	}
	require.NoError(t, c.Init(context.Background()))
	return c
}

func requireClusterIdentity(t *testing.T, c *Collector) {
	t.Helper()
	_, err := c.probeClusterIdentity(context.Background())
	require.NoError(t, err)
}

func TestCollector_RequestConfigRemainsCompatibleWithProxyAndTLSFields(t *testing.T) {
	c := New()
	c.Config.HTTPConfig = web.HTTPConfig{
		RequestConfig: web.RequestConfig{URL: "https://ceph.example"},
	}
	assert.Equal(t, "https://ceph.example", c.URL)
}
