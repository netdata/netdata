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

	"github.com/netdata/netdata/go/plugins/pkg/web"
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
	assert.Implements(t, (*collectorapi.CollectorV1)(nil), New())
	assert.Implements(t, (*collectorapi.FunctionAvailability)(nil), New())
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
	mx := c.Collect(context.Background())

	assert.NotEmpty(t, mx)
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
	assert.Nil(t, c.Collect(context.Background()))
	assert.Empty(t, *c.Charts())
}

func TestCollector_CheckFallsBackToLegacyMonitorIdentity(t *testing.T) {
	monitor, err := os.ReadFile("testdata/v16.2.15/api_monitor.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIClusterFSID:
			w.WriteHeader(http.StatusNotFound)
		case "/api/monitor":
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
	assert.NotEmpty(t, c.clusterFSID())
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

func TestCollector_PeriodicCollectionRevalidatesIdentity(t *testing.T) {
	var authenticatedFSIDRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiAuth:
			writeJSON(w, http.StatusCreated, authLoginResp{
				Token: "test-token",
			})
		case urlPathAPIClusterFSID:
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			authenticatedFSIDRequests++
			writeJSON(w, http.StatusOK, "synthetic-fsid")
		case urlPathApiHealthMinimal:
			writeJSON(w, http.StatusOK, map[string]any{"hosts": 3})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
	assert.EqualValues(t, 3, c.Collect(context.Background())["hosts_num"])
	assert.Equal(t, 2, authenticatedFSIDRequests)
}

func TestCollector_CheckAllowsUnavailableOptionalFeatureWhenStatusWorks(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == urlPathApiHealthMinimal {
			writeJSON(w, http.StatusOK, map[string]any{"health": map[string]any{"status": "HEALTH_OK"}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
}

func TestCollector_HealthMissingSection(t *testing.T) {
	var healthRequests int
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiHealthMinimal:
			healthRequests++
			writeJSON(w, http.StatusOK, map[string]any{"health": map[string]any{"status": "HEALTH_OK"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	requireClusterIdentity(t, c)
	mx := make(map[string]int64)
	err := c.collectHealth(context.Background(), mx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `response section "hosts" is unavailable`)
	assert.NotContains(t, mx, "hosts_num")
	assert.Equal(t, 1, healthRequests)
}

func TestCollector_HealthSemantics(t *testing.T) {
	health := map[string]any{
		"df": map[string]any{"stats": map[string]any{
			"total_bytes": 100, "total_avail_bytes": 0, "total_used_raw_bytes": 100,
		}},
		"pg_info": map[string]any{
			"object_stats": map[string]any{
				"num_objects": 10, "num_object_copies": 30, "num_objects_degraded": 3,
				"num_objects_misplaced": 2, "num_objects_unfound": 1,
			},
			"statuses": map[string]any{
				"active+clean": 1, "active+failed_repair": 2, "active+premerge": 3,
				"active+laggy": 4, "active+wait": 5,
			},
			"pgs_per_osd": 1.5,
		},
	}
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == urlPathApiHealthMinimal {
			writeJSON(w, http.StatusOK, health)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	requireClusterIdentity(t, c)
	mx := make(map[string]int64)
	_ = c.collectHealth(context.Background(), mx)

	assert.EqualValues(t, 100000, mx["raw_capacity_utilization"])
	assert.EqualValues(t, 100, mx["raw_capacity_used_bytes"])
	assert.EqualValues(t, 0, mx["raw_capacity_avail_bytes"])
	assert.EqualValues(t, 10, mx["objects_num"])
	assert.NotContains(t, mx, "objects_healthy_num")
	assert.EqualValues(t, 6666, mx["objects_misplaced_ratio"])
	assert.EqualValues(t, 10000, mx["objects_degraded_ratio"])
	assert.EqualValues(t, 10000, mx["objects_unfound_ratio"])
	assert.EqualValues(t, 15, mx["pgs_num"])
	assert.EqualValues(t, 1, mx["pg_status_category_clean"])
	assert.EqualValues(t, 8, mx["pg_status_category_working"])
	assert.EqualValues(t, 6, mx["pg_status_category_warning"])
	assert.EqualValues(t, 0, mx["pg_status_category_unknown"])
	assert.EqualValues(t, 1500, mx["pgs_per_osd"])
	collecttest.TestMetricsHasAllChartsDims(t, c.Charts(), mx)
}

func TestCollector_OSDWholeListCapIsAllOrNone(t *testing.T) {
	var offsets []int
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathApiOsd {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		offsets = append(offsets, offset)
		w.Header().Set("X-Total-Count", "101")
		count := 101
		page := make([]apiOsdResponse, 0, count)
		for i := range count {
			id := offset + i
			var osd apiOsdResponse
			osd.ID = int64(id)
			osd.UUID = fmt.Sprintf("uuid-%03d", id)
			osd.Up, osd.In = 1, 1
			osd.Tree.Name = fmt.Sprintf("osd.%d", id)
			osd.Tree.DeviceClass = "ssd"
			osd.OsdStats.Statfs.Total = 100
			osd.OsdStats.Statfs.Available = 75
			osd.Stats.OpR, osd.Stats.OpW = 1, 2
			osd.Stats.OpOutBytes, osd.Stats.OpInBytes = 3, 4
			osd.OsdStats.PerfStat.CommitLatencyMs = 1.25
			osd.OsdStats.PerfStat.ApplyLatencyMs = 2.5
			page = append(page, osd)
		}
		writeJSON(w, http.StatusOK, page)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.MaxOSDs = 2
	})
	defer c.Cleanup(context.Background())
	requireClusterIdentity(t, c)
	mx := make(map[string]int64)
	err := c.collectOsds(context.Background(), mx)
	require.NoError(t, err)

	assert.Equal(t, []int{0}, offsets)
	for key := range mx {
		assert.NotContains(t, key, "osd_")
	}
	assert.Empty(t, c.seenOsds)
	collecttest.TestMetricsHasAllChartsDims(t, c.Charts(), mx)
}

func TestCollector_PoolCapIsAllOrNone(t *testing.T) {
	pool := func(name string) apiPoolResponse {
		var value apiPoolResponse
		value.PoolName = name
		value.Stats.Objects.Latest = "1"
		value.Stats.AvailRaw.Latest = "80"
		value.Stats.BytesUsed.Latest = "20"
		value.Stats.PercentUsed.Latest = "0.2"
		value.Stats.Reads.Latest = "1"
		value.Stats.ReadBytes.Latest = "2"
		value.Stats.Writes.Latest = "3"
		value.Stats.WrittenBytes.Latest = "4"
		return value
	}
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == urlPathApiPool {
			writeJSON(w, http.StatusOK, []apiPoolResponse{pool("a"), pool("b"), pool("c")})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.MaxPools = 2
	})
	defer c.Cleanup(context.Background())
	requireClusterIdentity(t, c)
	mx := make(map[string]int64)
	err := c.collectPools(context.Background(), mx)
	require.NoError(t, err)
	for key := range mx {
		assert.NotContains(t, key, "pool_")
	}
	assert.Empty(t, c.seenPools)
}

func TestCollector_CycleWithoutDataReturnsNoMetrics(t *testing.T) {
	tests := map[string]struct {
		failAll bool
	}{
		"all components fail":          {failAll: true},
		"components return no metrics": {},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
				if test.failAll {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				switch r.URL.Path {
				case urlPathApiHealthMinimal:
					_, _ = io.WriteString(w, `{}`)
				case urlPathApiOsd:
					w.Header().Set("X-Total-Count", "0")
					_, _ = io.WriteString(w, `[]`)
				case urlPathApiPool:
					_, _ = io.WriteString(w, `[]`)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			defer srv.Close()

			c := newInitializedCollector(t, srv.URL, nil)
			defer c.Cleanup(context.Background())
			assert.Nil(t, c.Collect(context.Background()))
		})
	}
}

func TestCollector_PartialFailureRetainsDataAndReportsComponentStatus(t *testing.T) {
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
	mx := c.Collect(context.Background())
	require.NotEmpty(t, mx)
	assert.Contains(t, mx, "health_ok")
	assert.EqualValues(t, 0, mx["health_collection_failed"])
	assert.EqualValues(t, 1, mx["osd_collection_failed"])
	assert.EqualValues(t, 0, mx["pool_collection_failed"])
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
	mx := c.Collect(context.Background())
	require.NotEmpty(t, mx)
	assert.EqualValues(t, 101, mx["osds_num"])
	assert.EqualValues(t, 101, mx["pools_num"])
	assert.Zero(t, osdRequests.Load())
	assert.Zero(t, poolRequests.Load())
	assert.EqualValues(t, 0, mx["osd_collection_failed"])
	assert.EqualValues(t, 0, mx["pool_collection_failed"])
}

func TestCollector_CustomSelectorFetchesInventoryDespiteHealthCount(t *testing.T) {
	var osdRequests atomic.Int64
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiHealthMinimal:
			osds := make([]map[string]any, 101)
			for i := range osds {
				osds[i] = map[string]any{"up": 1, "in": 1}
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"health":  map[string]any{"status": "HEALTH_OK"},
				"osd_map": map[string]any{"osds": osds},
				"pools":   []any{},
			})
		case urlPathApiOsd:
			osdRequests.Add(1)
			w.Header().Set("X-Total-Count", "1")
			writeJSON(w, http.StatusOK, []map[string]any{{
				"id": 1, "uuid": "uuid-1", "up": 1, "in": 1,
				"tree": map[string]any{"name": "osd.1", "device_class": "ssd"},
			}})
		case urlPathApiPool:
			_, _ = io.WriteString(w, `[]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.OSDSelector = "osd.1"
		c.MaxOSDs = 1
	})
	defer c.Cleanup(context.Background())
	mx := c.Collect(context.Background())
	require.NotEmpty(t, mx)
	assert.EqualValues(t, 1, osdRequests.Load())
	assert.Contains(t, mx, "osd_uuid-1_status_up")
}

func TestCollector_CapSuppressionImmediatelyObsoletesEntityCharts(t *testing.T) {
	c := New()
	c.identityMu.Lock()
	c.fsid = "synthetic-fsid"
	c.identityMu.Unlock()
	require.NoError(t, c.addOsdCharts("uuid-1", "ssd", "osd.1"))
	c.seenOsds["uuid-1"] = &entityState{
		lastSeen: time.Now(),
	}

	c.suppressEntityMetrics("osd", c.seenOsds)
	assert.Empty(t, c.seenOsds)
	for _, chart := range *c.Charts() {
		assert.True(t, chart.IsRemoved(), chart.ID)
	}
}

func TestCollectObjectHealthAllowsIndependentCopyStates(t *testing.T) {
	stats := struct {
		NumObjects          int64 `json:"num_objects"`
		NumObjectCopies     int64 `json:"num_object_copies"`
		NumObjectsDegraded  int64 `json:"num_objects_degraded"`
		NumObjectsMisplaced int64 `json:"num_objects_misplaced"`
		NumObjectsUnfound   int64 `json:"num_objects_unfound"`
	}{
		NumObjects: 10, NumObjectCopies: 30, NumObjectsDegraded: 20,
		NumObjectsMisplaced: 20, NumObjectsUnfound: 10,
	}
	mx := make(map[string]int64)
	var errs []error
	collectObjectHealth(stats, mx, &errs)
	require.Empty(t, errs)
	assert.EqualValues(t, 66666, mx["objects_degraded_ratio"])
	assert.EqualValues(t, 66666, mx["objects_misplaced_ratio"])
	assert.EqualValues(t, 100000, mx["objects_unfound_ratio"])
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

func TestValidateOSDsRejectsUnsafeDynamicIdentitiesAndCapacity(t *testing.T) {
	valid := apiOsdResponse{
		ID:   1,
		UUID: "uuid-1",
		Up:   1,
		In:   1,
	}
	valid.OsdStats.Statfs.Total = 100
	valid.OsdStats.Statfs.Available = 50
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

func TestValidateOSDChartKeysRejectsLegacyNormalizationCollision(t *testing.T) {
	err := validateOSDChartKeys([]osdMetricSample{{key: "uuid.a"}, {key: "uuid_a"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collide")
}

func TestCollector_PoolAvailableIsNotReducedTwice(t *testing.T) {
	var pool apiPoolResponse
	pool.PoolName = "pool-a"
	pool.Stats.Objects.Latest = "5"
	pool.Stats.AvailRaw.Latest = "80"
	pool.Stats.BytesUsed.Latest = "20"
	pool.Stats.PercentUsed.Latest = "0.2"
	pool.Stats.Reads.Latest = "1"
	pool.Stats.ReadBytes.Latest = "2"
	pool.Stats.Writes.Latest = "3"
	pool.Stats.WrittenBytes.Latest = "4"

	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == urlPathApiPool && r.URL.Query().Get("stats") == "true" {
			writeJSON(w, http.StatusOK, []apiPoolResponse{pool})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	requireClusterIdentity(t, c)
	mx := make(map[string]int64)
	err := c.collectPools(context.Background(), mx)
	require.NoError(t, err)
	assert.EqualValues(t, 80, mx["pool_pool-a_space_avail_bytes"])
	assert.EqualValues(t, 20, mx["pool_pool-a_space_used_bytes"])
	assert.EqualValues(t, 20000, mx["pool_pool-a_space_utilization"])
	collecttest.TestMetricsHasAllChartsDims(t, c.Charts(), mx)
}

func TestCollector_DynamicEntityAbsenceGrace(t *testing.T) {
	c := New()
	c.identityMu.Lock()
	c.fsid = "synthetic-fsid"
	c.identityMu.Unlock()
	require.NoError(t, c.addPoolCharts("pool-a"))
	base := time.Unix(100, 0)
	c.seenPools["pool-a"] = &entityState{
		lastSeen: base,
	}

	c.expireMissingEntities("pool", c.seenPools, map[string]bool{}, base.Add(59*time.Second))
	assert.Contains(t, c.seenPools, "pool-a")
	for _, chart := range *c.Charts() {
		assert.False(t, chart.Obsolete)
	}

	c.expireMissingEntities("pool", c.seenPools, map[string]bool{}, base.Add(61*time.Second))
	assert.NotContains(t, c.seenPools, "pool-a")
	for _, chart := range *c.Charts() {
		assert.True(t, chart.Obsolete)
	}

	require.NoError(t, c.addPoolCharts("pool-a"))
	require.Len(t, *c.Charts(), len(poolChartsTmpl))
	for _, chart := range *c.Charts() {
		assert.False(t, chart.IsRemoved())
		assert.False(t, chart.Obsolete)
	}
}

func TestCollector_DynamicEntityLifecycleUsesExactChartIDs(t *testing.T) {
	c := New()
	c.identityMu.Lock()
	c.fsid = "synthetic-fsid"
	c.identityMu.Unlock()
	require.NoError(t, c.addPoolCharts("foo"))
	require.NoError(t, c.addPoolCharts("foo_bar"))
	base := time.Unix(100, 0)
	c.seenPools["foo"] = &entityState{
		lastSeen: base,
	}
	c.seenPools["foo_bar"] = &entityState{
		lastSeen: base,
	}

	c.expireMissingEntities("pool", c.seenPools, map[string]bool{"foo_bar": true}, base.Add(61*time.Second))
	for _, id := range entityChartIDs("pool", "foo") {
		require.NotNil(t, c.Charts().Get(id))
		assert.True(t, c.Charts().Get(id).IsRemoved(), id)
	}
	for _, id := range entityChartIDs("pool", "foo_bar") {
		require.NotNil(t, c.Charts().Get(id))
		assert.False(t, c.Charts().Get(id).IsRemoved(), id)
	}

	require.NoError(t, c.addPoolCharts("foo"))
	assert.Len(t, *c.Charts(), 2*len(poolChartsTmpl))
	for _, id := range entityChartIDs("pool", "foo_bar") {
		assert.False(t, c.Charts().Get(id).IsRemoved(), id)
	}
}

func TestCollector_PoolNormalizedChartOwnerReplacement(t *testing.T) {
	var current atomic.Value
	current.Store(validPoolResponse("pool.a"))
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == urlPathApiPool && r.URL.Query().Get("stats") == "true" {
			writeJSON(w, http.StatusOK, []apiPoolResponse{current.Load().(apiPoolResponse)})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	requireClusterIdentity(t, c)
	now := time.Unix(100, 0)
	c.now = func() time.Time { return now }

	mx := make(map[string]int64)
	require.NoError(t, c.collectPools(context.Background(), mx))
	assert.Contains(t, c.seenPools, "pool.a")
	collecttest.TestMetricsHasAllChartsDims(t, c.Charts(), mx)

	current.Store(validPoolResponse("pool_a"))
	now = now.Add(10 * time.Second)
	mx = make(map[string]int64)
	require.NoError(t, c.collectPools(context.Background(), mx))
	assert.Empty(t, mx)
	assert.NotContains(t, c.seenPools, "pool.a")
	assert.NotContains(t, c.seenPools, "pool_a")
	for _, id := range entityChartIDs("pool", "pool.a") {
		require.NotNil(t, c.Charts().Get(id))
		assert.True(t, c.Charts().Get(id).IsRemoved(), id)
	}

	now = now.Add(10 * time.Second)
	mx = make(map[string]int64)
	require.NoError(t, c.collectPools(context.Background(), mx))
	assert.Contains(t, c.seenPools, "pool_a")
	assert.NotContains(t, c.seenPools, "pool.a")
	require.Len(t, *c.Charts(), len(poolChartsTmpl))
	for _, chart := range *c.Charts() {
		assert.False(t, chart.IsRemoved(), chart.ID)
		assert.Equal(t, "pool_a", chart.Labels[1].Value, chart.ID)
	}
	collecttest.TestMetricsHasAllChartsDims(t, c.Charts(), mx)
}

func TestCollector_PoolChartAdmissionIsTransactional(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == urlPathApiPool && r.URL.Query().Get("stats") == "true" {
			writeJSON(w, http.StatusOK, []apiPoolResponse{validPoolResponse("pool-a")})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	requireClusterIdentity(t, c)
	require.NoError(t, c.addPoolCharts("pool-a"))
	ids := entityChartIDs("pool", "pool-a")
	for _, id := range ids {
		if id != ids[1] {
			require.NoError(t, c.Charts().Remove(id))
		}
	}
	require.Len(t, *c.Charts(), 1)

	mx := make(map[string]int64)
	err := c.collectPools(context.Background(), mx)
	require.Error(t, err)
	assert.Empty(t, mx)
	assert.Empty(t, c.seenPools)
	require.Len(t, *c.Charts(), 1)
	assert.Nil(t, c.Charts().Get(ids[0]))
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

func TestValidatePoolChartKeysRejectsLegacyNormalizationCollision(t *testing.T) {
	err := validatePoolChartKeys([]poolMetricSample{{key: "pool.a"}, {key: "pool_a"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collide")
}

func TestPoolSampleRejectsInvalidUpstreamValues(t *testing.T) {
	tests := map[string]struct {
		objects     string
		percentUsed string
		wantObjects int64
		wantError   string
	}{
		"preserves integers above float precision": {
			objects:     "9007199254740993",
			percentUsed: "0.5",
			wantObjects: 9007199254740993,
		},
		"rejects utilization above one": {
			objects:     "0",
			percentUsed: "1.1",
			wantError:   "0-1",
		},
		"rejects fractional object count": {
			objects:     "1.5",
			percentUsed: "0.5",
			wantError:   "objects",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var pool apiPoolResponse
			pool.PoolName = "pool-a"
			pool.Stats.Objects.Latest = json.Number(test.objects)
			pool.Stats.AvailRaw.Latest = "0"
			pool.Stats.BytesUsed.Latest = "0"
			pool.Stats.PercentUsed.Latest = json.Number(test.percentUsed)
			pool.Stats.Reads.Latest = "0"
			pool.Stats.ReadBytes.Latest = "0"
			pool.Stats.Writes.Latest = "0"
			pool.Stats.WrittenBytes.Latest = "0"

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

func TestCollector_PoolsRejectDuplicateSelectedNames(t *testing.T) {
	var pool apiPoolResponse
	pool.PoolName = "pool-a"
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == urlPathApiPool {
			writeJSON(w, http.StatusOK, []apiPoolResponse{pool, pool})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	requireClusterIdentity(t, c)
	err := c.collectPools(context.Background(), make(map[string]int64))
	require.ErrorContains(t, err, "duplicate selected pool name")
}

func TestCollector_HealthRejectsInvalidSectionValues(t *testing.T) {
	tests := map[string]struct {
		response map[string]any
		want     string
	}{
		"hosts": {
			response: map[string]any{"hosts": -1},
			want:     "negative host count",
		},
		"OSD summary": {
			response: map[string]any{"osd_map": map[string]any{"osds": []map[string]any{{"up": 2, "in": 1}}}},
			want:     "invalid up/in state",
		},
		"capacity": {
			response: map[string]any{"df": map[string]any{"stats": map[string]any{
				"total_bytes": 100, "total_used_raw_bytes": 101, "total_avail_bytes": 0,
			}}},
			want: "invalid capacity values",
		},
		"objects": {
			response: map[string]any{"pg_info": map[string]any{"object_stats": map[string]any{
				"num_objects": -1, "num_object_copies": 1,
			}}},
			want: "invalid object/copy health counters",
		},
		"PGs": {
			response: map[string]any{"pg_info": map[string]any{
				"statuses": map[string]any{"active+clean": -1}, "pgs_per_osd": 1,
			}},
			want: "invalid PG state counts",
		},
		"client IO": {
			response: map[string]any{"client_perf": map[string]any{"read_bytes_sec": -1}},
			want:     "invalid performance value",
		},
		"RGW": {
			response: map[string]any{"rgw": -1},
			want:     "negative gateway count",
		},
		"iSCSI": {
			response: map[string]any{"iscsi_daemons": map[string]any{"up": -1, "down": 0}},
			want:     "invalid gateway counts",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == urlPathApiHealthMinimal {
					writeJSON(w, http.StatusOK, test.response)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			})
			defer srv.Close()

			c := newInitializedCollector(t, srv.URL, nil)
			defer c.Cleanup(context.Background())
			requireClusterIdentity(t, c)
			mx := make(map[string]int64)
			err := c.collectHealth(context.Background(), mx)
			require.ErrorContains(t, err, test.want)
			if name == "objects" {
				assert.NotContains(t, mx, "objects_num")
			}
		})
	}
}

func TestCollector_ChartAlgorithms(t *testing.T) {
	assert.Equal(t, collectorapi.Line, clusterObjectCopiesHealthChart.Type)
	assert.Equal(t, collectorapi.Line, clusterObjectsUnfoundChart.Type)
	for _, dim := range osdIOChartTmpl.Dims {
		assert.Equal(t, string(collectorapi.Absolute), dim.Algo.String())
	}
	for _, dim := range osdIOPSChartTmpl.Dims {
		assert.Equal(t, string(collectorapi.Absolute), dim.Algo.String())
	}
	for _, dim := range poolIOChartTmpl.Dims {
		assert.Equal(t, collectorapi.Incremental, dim.Algo)
	}
	for _, chart := range []*collectorapi.Chart{&clusterObjectCopiesHealthChart, &clusterObjectsUnfoundChart} {
		for _, dim := range chart.Dims {
			assert.EqualValues(t, precision, dim.Div)
		}
	}
}

func TestCollector_PublicChartIdentityContract(t *testing.T) {
	want := map[string]string{
		"component_collection_status":            "ceph.component_collection_status|status|line|health_collection_failed=health,osd_collection_failed=osds,pool_collection_failed=pools",
		"cluster_status":                         "ceph.cluster_status|status|line|health_ok=ok,health_err=err,health_warn=warn",
		"cluster_hosts_count":                    "ceph.cluster_hosts_count|hosts|line|hosts_num=hosts",
		"cluster_monitors_count":                 "ceph.cluster_monitors_count|monitors|line|monitors_num=monitors",
		"cluster_osds_count":                     "ceph.cluster_osds_count|osds|line|osds_num=osds",
		"cluster_osds_by_status_count":           "ceph.cluster_osds_by_status_count|osds|line|osds_up_num=up,osds_down_num=down,osds_in_num=in,osds_out_num=out",
		"cluster_managers_count":                 "ceph.cluster_managers_count|managers|line|mgr_active_num=active,mgr_standby_num=standby",
		"cluster_object_gateways_count":          "ceph.cluster_object_gateways_count|gateways|line|rgw_num=object",
		"cluster_iscsi_gateways_count":           "ceph.cluster_iscsi_gateways_count|gateways|line|iscsi_daemons_num=iscsi",
		"cluster_iscsi_gateways_by_status_count": "ceph.cluster_iscsi_gateways_by_status_count|gateways|line|iscsi_daemons_up_num=up,iscsi_daemons_down_num=down",
		"cluster_physical_capacity_utilization":  "ceph.cluster_physical_capacity_utilization|percent|area|raw_capacity_utilization=utilization",
		"cluster_physical_capacity_usage":        "ceph.cluster_physical_capacity_usage|bytes|stacked|raw_capacity_avail_bytes=avail,raw_capacity_used_bytes=used",
		"cluster_objects_count":                  "ceph.cluster_objects_count|objects|line|objects_num=objects",
		"cluster_object_copies_health":           "ceph.cluster_object_copies_health|percent|line|objects_degraded_ratio=degraded,objects_misplaced_ratio=misplaced",
		"cluster_objects_unfound":                "ceph.cluster_objects_unfound|percent|line|objects_unfound_ratio=unfound",
		"cluster_pools_count":                    "ceph.cluster_pools_count|pools|line|pools_num=pools",
		"cluster_pgs_count":                      "ceph.cluster_pgs_count|pgs|line|pgs_num=pgs",
		"cluster_pgs_by_status_count":            "ceph.cluster_pgs_by_status_count|pgs|stacked|pg_status_category_clean=clean,pg_status_category_working=working,pg_status_category_warning=warning,pg_status_category_unknown=unknown",
		"cluster_pgs_per_osd_count":              "ceph.cluster_pgs_per_osd_count|pgs|line|pgs_per_osd=per_osd",
		"cluster_client_io":                      "ceph.cluster_client_io|bytes/s|area|client_perf_read_bytes_sec=read,client_perf_write_bytes_sec=written",
		"cluster_client_iops":                    "ceph.cluster_client_iops|ops/s|line|client_perf_read_op_per_sec=read,client_perf_write_op_per_sec=write",
		"cluster_recovery_throughput":            "ceph.cluster_recovery_throughput|bytes/s|line|client_perf_recovering_bytes_per_sec=recovery",
		"cluster_scrub_status":                   "ceph.cluster_scrub_status|status|line|scrub_status_disabled=disabled,scrub_status_active=active,scrub_status_inactive=inactive",
		"osd_%s_status":                          "ceph.osd_status|status|line|osd_%s_status_up=up,osd_%s_status_down=down,osd_%s_status_in=in,osd_%s_status_out=out",
		"osd_%s_space_usage":                     "ceph.osd_space_usage|bytes|stacked|osd_%s_space_avail_bytes=avail,osd_%s_space_used_bytes=used",
		"osd_%s_io":                              "ceph.osd_io|bytes/s|area|osd_%s_read_bytes=read,osd_%s_written_bytes=written",
		"osd_%s_iops":                            "ceph.osd_iops|ops/s|line|osd_%s_read_ops=read,osd_%s_write_ops=write",
		"osd_%s_latency":                         "ceph.osd_latency|milliseconds|line|osd_%s_commit_latency_ms=commit,osd_%s_apply_latency_ms=apply",
		"pool_%s_space_utilization":              "ceph.pool_space_utilization|percent|area|pool_%s_space_utilization=utilization",
		"pool_%s_space_usage":                    "ceph.pool_space_usage|bytes|stacked|pool_%s_space_avail_bytes=avail,pool_%s_space_used_bytes=used",
		"pool_%s_objects_count":                  "ceph.pool_objects_count|objects|line|pool_%s_objects=objects",
		"pool_%s_io":                             "ceph.pool_io|bytes/s|area|pool_%s_read_bytes=read,pool_%s_written_bytes=written",
		"pool_%s_iops":                           "ceph.pool_iops|ops/s|line|pool_%s_read_ops=read,pool_%s_write_ops=write",
	}

	got := make(map[string]string, len(want))
	for _, templates := range []*collectorapi.Charts{&clusterCharts, &osdChartsTmpl, &poolChartsTmpl} {
		for _, chart := range *templates {
			dims := make([]string, 0, len(chart.Dims))
			for _, dim := range chart.Dims {
				dims = append(dims, dim.ID+"="+dim.Name)
			}
			got[chart.ID] = fmt.Sprintf(
				"%s|%s|%s|%s",
				chart.Ctx,
				chart.Units,
				chart.Type.String(),
				strings.Join(dims, ","),
			)
		}
	}
	assert.Equal(t, want, got)

	c := New()
	c.identityMu.Lock()
	c.fsid = "synthetic-fsid"
	c.identityMu.Unlock()
	c.addClusterCharts()
	require.NoError(t, c.addOsdCharts("uuid-1", "ssd", "osd.1"))
	require.NoError(t, c.addPoolCharts("pool-a"))
	for _, chart := range *c.Charts() {
		keys := make([]string, 0, len(chart.Labels))
		for _, label := range chart.Labels {
			keys = append(keys, label.Key)
		}
		switch {
		case strings.HasPrefix(chart.ID, "osd_"):
			assert.Equal(t, []string{"fsid", "osd_uuid", "osd_name", "device_class"}, keys, chart.ID)
		case strings.HasPrefix(chart.ID, "pool_"):
			assert.Equal(t, []string{"fsid", "pool_name"}, keys, chart.ID)
		default:
			assert.Equal(t, []string{"fsid"}, keys, chart.ID)
		}
	}
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

func newFakeDashboard(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiAuth:
			writeJSON(w, http.StatusCreated, authLoginResp{
				Token: "test-token",
			})
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
		RequestConfig: web.RequestConfig{
			URL: "https://ceph.example",
		},
	}
	assert.Equal(t, "https://ceph.example", c.URL)
}
