// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
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

func TestCollector_Defaults(t *testing.T) {
	c := New()
	assert.Equal(t, "https://127.0.0.1:8443", c.URL)
	assert.Equal(t, 15*time.Second, c.Timeout.Duration())
	assert.Empty(t, c.AllowedRedirectOrigins)
	assert.Equal(t, 100, c.MaxOSDs)
	assert.Equal(t, 100, c.MaxPools)
	assert.Equal(t, "*", c.OSDSelector)
	assert.Equal(t, "*", c.PoolSelector)
	assert.True(t, c.Metrics.DashboardAPIStatus)
	assert.False(t, c.Metrics.anyHealthEnabled())
	assert.False(t, c.Metrics.OSDs)
	assert.False(t, c.Metrics.Pools)
	assert.True(t, c.Functions.RGWMultisite.Disabled)
	assert.True(t, c.Functions.RGWQuotas.Disabled)
	assert.Equal(t, 100, c.Functions.RGWMultisite.Limit)
	assert.Equal(t, 100, c.Functions.RGWQuotas.Limit)
}

func TestCollector_ConfigSchemaDefaultsMatchRuntime(t *testing.T) {
	var document struct {
		JSONSchema struct {
			Properties map[string]any `json:"properties"`
		} `json:"jsonSchema"`
	}
	require.NoError(t, json.Unmarshal([]byte(configSchema), &document))
	properties := document.JSONSchema.Properties
	require.NotEmpty(t, properties)
	assert.EqualValues(t, 10, schemaDefault(t, properties, "update_every"))
	assert.EqualValues(t, 15, schemaDefault(t, properties, "timeout"))
	assert.Equal(t, "https://127.0.0.1:8443", schemaDefault(t, properties, "url"))
	assert.Equal(t, "*", schemaDefault(t, properties, "osd_selector"))
	assert.EqualValues(t, 100, schemaDefault(t, properties, "max_osds"))
	assert.Equal(t, "*", schemaDefault(t, properties, "pool_selector"))
	assert.EqualValues(t, 100, schemaDefault(t, properties, "max_pools"))

	collect := schemaProperties(t, properties, "collect")
	for name := range collect {
		assert.Equal(t, name == "dashboard_api_status", schemaDefault(t, collect, name), name)
	}

	functions := schemaProperties(t, properties, "functions")
	for _, name := range []string{"health", "osds", "pools", "daemons", "rgw_multisite", "rgw_quotas"} {
		method := schemaProperties(t, functions, name)
		assert.Equal(t, name == "rgw_multisite" || name == "rgw_quotas", schemaDefault(t, method, "disabled"), name)
		assert.EqualValues(t, map[string]int{"health": 500, "osds": 500, "pools": 500, "daemons": 500, "rgw_multisite": 100, "rgw_quotas": 100}[name], schemaDefault(t, method, "limit"), name)
	}
	assert.NotContains(t, properties, "method")
	assert.NotContains(t, properties, "body")
}

func schemaProperties(t *testing.T, properties map[string]any, name string) map[string]any {
	t.Helper()
	entry, ok := properties[name].(map[string]any)
	require.True(t, ok, name)
	children, ok := entry["properties"].(map[string]any)
	require.True(t, ok, name)
	return children
}

func schemaDefault(t *testing.T, properties map[string]any, name string) any {
	t.Helper()
	entry, ok := properties[name].(map[string]any)
	require.True(t, ok, name)
	value, ok := entry["default"]
	require.True(t, ok, name)
	return value
}

func TestCollector_LegacyConfigMigratesToComplementaryDefaults(t *testing.T) {
	c := New()
	require.NoError(t, yaml.Unmarshal([]byte(`
url: https://ceph.example:8443
username: netdata
password: test-password
`), c))

	assert.True(t, c.Metrics.DashboardAPIStatus)
	assert.False(t, c.Metrics.anyHealthEnabled())
	assert.False(t, c.Metrics.OSDs)
	assert.False(t, c.Metrics.Pools)
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
		"function only with metric": {
			modify:   func(cfg *Config) { cfg.FunctionOnly = true },
			wantFail: true,
		},
		"explicit function only": {
			modify: func(cfg *Config) {
				cfg.FunctionOnly = true
				cfg.Metrics.DashboardAPIStatus = false
			},
		},
		"no metric without function only": {
			modify:   func(cfg *Config) { cfg.Metrics.DashboardAPIStatus = false },
			wantFail: true,
		},
		"non-positive OSD cap": {
			modify:   func(cfg *Config) { cfg.MaxOSDs = 0 },
			wantFail: true,
		},
		"unbounded job timeout": {
			modify:   func(cfg *Config) { cfg.Timeout = 0 },
			wantFail: true,
		},
		"negative Function timeout": {
			modify:   func(cfg *Config) { cfg.Functions.Health.Timeout = -1 },
			wantFail: true,
		},
		"function limit above hard maximum": {
			modify:   func(cfg *Config) { cfg.Functions.Health.Limit = maxFunctionRows + 1 },
			wantFail: true,
		},
		"enabled RGW quotas without targets": {
			modify:   func(cfg *Config) { cfg.Functions.RGWQuotas.Disabled = false },
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

func TestCollector_DefaultCollectionOnlyProbesDashboardAPI(t *testing.T) {
	var pathsMu sync.Mutex
	paths := make(map[string]int)
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		pathsMu.Lock()
		paths[r.URL.Path]++
		pathsMu.Unlock()
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
	mx := c.Collect(context.Background())

	assert.Equal(t, map[string]int64{
		"dashboard_api_reachable":   1,
		"dashboard_api_unreachable": 0,
	}, mx)
	require.Len(t, *c.Charts(), 1)
	assert.Equal(t, dashboardAPIStatusChart.ID, (*c.Charts())[0].ID)
	pathsMu.Lock()
	defer pathsMu.Unlock()
	assert.Zero(t, paths[urlPathApiHealthMinimal])
	assert.Zero(t, paths[urlPathApiOsd])
	assert.Zero(t, paths[urlPathApiPool])
}

func TestCollector_FunctionOnlyChecksIdentityWithoutMetrics(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.FunctionOnly = true
		c.Metrics.DashboardAPIStatus = false
	})
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
	assert.Nil(t, c.Collect(context.Background()))
	assert.Empty(t, *c.Charts())
}

func TestCollector_DisabledStatusCachesIdentityInsteadOfPollingIt(t *testing.T) {
	var authenticatedFSIDRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiAuth:
			writeJSON(t, w, http.StatusCreated, authLoginResp{Token: "test-token"})
		case urlPathAPIClusterFSID:
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			authenticatedFSIDRequests++
			writeJSON(t, w, http.StatusOK, "synthetic-fsid")
		case urlPathApiHealthMinimal:
			writeJSON(t, w, http.StatusOK, map[string]any{"hosts": 3})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.Metrics.DashboardAPIStatus = false
		c.Metrics.Hosts = true
	})
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
	assert.EqualValues(t, 3, c.Collect(context.Background())["hosts_num"])
	assert.Equal(t, 1, authenticatedFSIDRequests)
}

func TestCollector_CheckAllowsUnavailableOptionalFeatureWhenStatusWorks(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == urlPathApiHealthMinimal {
			writeJSON(t, w, http.StatusOK, map[string]any{"health": map[string]any{"status": "HEALTH_OK"}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) { c.Metrics.Hosts = true })
	defer c.Cleanup(context.Background())
	require.NoError(t, c.Check(context.Background()))
}

func TestCollector_HealthFeatureRequestGatingAndMissingSection(t *testing.T) {
	var healthRequests int
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiHealthMinimal:
			healthRequests++
			writeJSON(t, w, http.StatusOK, map[string]any{"health": map[string]any{"status": "HEALTH_OK"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.Metrics.DashboardAPIStatus = false
		c.Metrics.Hosts = true
	})
	defer c.Cleanup(context.Background())
	mx, err := c.collect(context.Background())
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
			writeJSON(t, w, http.StatusOK, health)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.Metrics.DashboardAPIStatus = false
		c.Metrics.Capacity = true
		c.Metrics.Objects = true
		c.Metrics.PGs = true
	})
	defer c.Cleanup(context.Background())
	mx, err := c.collect(context.Background())
	require.NoError(t, err)

	assert.EqualValues(t, 100000, mx["raw_capacity_utilization"])
	assert.EqualValues(t, 100, mx["raw_capacity_used_bytes"])
	assert.EqualValues(t, 0, mx["raw_capacity_avail_bytes"])
	assert.EqualValues(t, 10, mx["objects_num"])
	assert.EqualValues(t, 80000, mx["objects_healthy_num"])
	assert.EqualValues(t, 6666, mx["objects_misplaced_num"])
	assert.EqualValues(t, 10000, mx["objects_degraded_num"])
	assert.EqualValues(t, 3333, mx["objects_unfound_num"])
	assert.EqualValues(t, 15, mx["pgs_num"])
	assert.EqualValues(t, 1, mx["pg_status_category_clean"])
	assert.EqualValues(t, 8, mx["pg_status_category_working"])
	assert.EqualValues(t, 6, mx["pg_status_category_warning"])
	assert.EqualValues(t, 0, mx["pg_status_category_unknown"])
	assert.EqualValues(t, 1500, mx["pgs_per_osd"])
	collecttest.TestMetricsHasAllChartsDims(t, c.Charts(), mx)
}

func TestCollector_OSDPaginationCapAndAggregateOther(t *testing.T) {
	var offsets []int
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathApiOsd {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
		require.NoError(t, err)
		offsets = append(offsets, offset)
		w.Header().Set("X-Total-Count", "101")
		count := 100
		if offset == 100 {
			count = 1
		}
		page := make([]apiOsdResponse, 0, count)
		for i := 0; i < count; i++ {
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
		writeJSON(t, w, http.StatusOK, page)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.Metrics.DashboardAPIStatus = false
		c.Metrics.OSDs = true
		c.MaxOSDs = 2
	})
	defer c.Cleanup(context.Background())
	mx, err := c.collect(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []int{0, 100}, offsets)
	assert.NotContains(t, mx, "osd_other_status_up")
	assert.NotContains(t, mx, "osd_other_status_down")
	assert.NotContains(t, mx, "osd_other_status_in")
	assert.NotContains(t, mx, "osd_other_status_out")
	assert.EqualValues(t, 2475, mx["osd_other_space_used_bytes"])
	assert.EqualValues(t, 99000, mx["osd_other_read_ops"])
	assert.EqualValues(t, 1250, mx["osd_uuid-000_commit_latency_ms"])
	assert.Len(t, *c.Charts(), 14)
	collecttest.TestMetricsHasAllChartsDims(t, c.Charts(), mx)
}

func TestCollector_OSDPaginationSupportsClustersLargerThanTenThousand(t *testing.T) {
	const total = 10001
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathApiOsd {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
		require.NoError(t, err)
		count := min(osdPageSize, total-offset)
		page := make([]apiOsdResponse, count)
		for i := range page {
			page[i].ID = int64(offset + i)
			page[i].UUID = fmt.Sprintf("uuid-%05d", offset+i)
		}
		w.Header().Set("X-Total-Count", strconv.Itoa(total))
		writeJSON(t, w, http.StatusOK, page)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, nil)
	defer c.Cleanup(context.Background())
	osds, err := c.fetchAllOSDs(context.Background())
	require.NoError(t, err)
	assert.Len(t, osds, total)
}

func TestCollector_OSDPaginationRejectsShortPageBeforeAdvertisedTotal(t *testing.T) {
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathApiOsd {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("X-Total-Count", "101")
		page := make([]apiOsdResponse, 50)
		for i := range page {
			page[i].ID = int64(i)
			page[i].UUID = fmt.Sprintf("uuid-%03d", i)
		}
		writeJSON(t, w, http.StatusOK, page)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.Metrics.DashboardAPIStatus = false
		c.Metrics.OSDs = true
	})
	defer c.Cleanup(context.Background())
	_, err := c.collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "short page")
}

func TestValidateOSDsRejectsUnsafeDynamicIdentitiesAndCapacity(t *testing.T) {
	valid := apiOsdResponse{ID: 1, UUID: "uuid-1", Up: 1, In: 1}
	valid.OsdStats.Statfs.Total = 100
	valid.OsdStats.Statfs.Available = 50
	require.NoError(t, validateOSDs([]apiOsdResponse{valid}))

	tests := map[string][]apiOsdResponse{
		"empty UUID": {{ID: 1}},
		"duplicate ID": {valid, func() apiOsdResponse {
			other := valid
			other.UUID = "uuid-2"
			return other
		}()},
		"duplicate UUID": {valid, func() apiOsdResponse {
			other := valid
			other.ID = 2
			return other
		}()},
		"available exceeds total": {func() apiOsdResponse {
			other := valid
			other.OsdStats.Statfs.Available = 101
			return other
		}()},
		"negative rate": {func() apiOsdResponse {
			other := valid
			other.Stats.OpR = -1
			return other
		}()},
	}
	for name, osds := range tests {
		t.Run(name, func(t *testing.T) {
			require.Error(t, validateOSDs(osds))
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
			writeJSON(t, w, http.StatusOK, []apiPoolResponse{pool})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.Metrics.DashboardAPIStatus = false
		c.Metrics.Pools = true
	})
	defer c.Cleanup(context.Background())
	mx, err := c.collect(context.Background())
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
	c.addPoolCharts("pool-a", true)
	base := time.Unix(100, 0)
	c.seenPools["pool-a"] = &entityState{lastSeen: base}

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

	c.addPoolCharts("pool-a", true)
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
	c.addPoolCharts("foo", true)
	c.addPoolCharts("foo_bar", true)
	base := time.Unix(100, 0)
	c.seenPools["foo"] = &entityState{lastSeen: base}
	c.seenPools["foo_bar"] = &entityState{lastSeen: base}

	c.expireMissingEntities("pool", c.seenPools, map[string]bool{"foo_bar": true}, base.Add(61*time.Second))
	for _, id := range entityChartIDs("pool", "foo") {
		require.NotNil(t, c.Charts().Get(id))
		assert.True(t, c.Charts().Get(id).IsRemoved(), id)
	}
	for _, id := range entityChartIDs("pool", "foo_bar") {
		require.NotNil(t, c.Charts().Get(id))
		assert.False(t, c.Charts().Get(id).IsRemoved(), id)
	}

	c.addPoolCharts("foo", true)
	assert.Len(t, *c.Charts(), 2*len(poolChartsTmpl))
	for _, id := range entityChartIDs("pool", "foo_bar") {
		assert.False(t, c.Charts().Get(id).IsRemoved(), id)
	}
}

func TestCollector_PoolOverflowEmitsOnlyAdditiveObjects(t *testing.T) {
	c := New()
	c.identityMu.Lock()
	c.fsid = "synthetic-fsid"
	c.identityMu.Unlock()
	c.addPoolCharts("other", false)

	require.Len(t, *c.Charts(), 1)
	for _, chart := range *c.Charts() {
		assert.Equal(t, poolObjectsCountChartTmpl.Ctx, chart.Ctx)
		assert.NotEqual(t, poolSpaceUtilizationChartTmpl.Ctx, chart.Ctx)
		assert.NotEqual(t, poolSpaceUsageChartTmpl.Ctx, chart.Ctx)
		assert.NotEqual(t, poolIOChartTmpl.Ctx, chart.Ctx)
		assert.NotEqual(t, poolIOPSChartTmpl.Ctx, chart.Ctx)
	}

	mx := make(map[string]int64)
	emitPoolMetrics(mx, poolMetricSample{
		key: "other", overflow: true, objects: 10, available: 80, used: 20, utilization: 20,
		readOps: 100, writeOps: 200, readBytes: 300, writtenBytes: 400,
	})
	assert.EqualValues(t, 10, mx["pool_other_objects"])
	assert.NotContains(t, mx, "pool_other_space_used_bytes")
	assert.NotContains(t, mx, "pool_other_space_avail_bytes")
	assert.NotContains(t, mx, "pool_other_space_utilization")
	assert.NotContains(t, mx, "pool_other_read_ops")
	assert.NotContains(t, mx, "pool_other_read_bytes")
	assert.NotContains(t, mx, "pool_other_write_ops")
	assert.NotContains(t, mx, "pool_other_written_bytes")
}

func TestValidatePoolChartKeysRejectsLegacyNormalizationCollision(t *testing.T) {
	err := validatePoolChartKeys([]poolMetricSample{{key: "pool.a"}, {key: "pool_a"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collide")
}

func TestPoolSampleRejectsInvalidUpstreamValues(t *testing.T) {
	var valid apiPoolResponse
	valid.PoolName = "pool-a"
	valid.Stats.Objects.Latest = "0"
	valid.Stats.AvailRaw.Latest = "0"
	valid.Stats.BytesUsed.Latest = "0"
	valid.Stats.PercentUsed.Latest = "0.5"
	valid.Stats.Reads.Latest = "0"
	valid.Stats.ReadBytes.Latest = "0"
	valid.Stats.Writes.Latest = "0"
	valid.Stats.WrittenBytes.Latest = "0"
	valid.Stats.Objects.Latest = "9007199254740993"
	sample, err := poolSample(valid)
	require.NoError(t, err)
	assert.EqualValues(t, 9007199254740993, sample.objects)

	invalid := valid
	invalid.Stats.PercentUsed.Latest = "1.1"
	_, err = poolSample(invalid)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "0-1")

	invalid = valid
	invalid.Stats.Objects.Latest = "1.5"
	_, err = poolSample(invalid)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "objects")
}

func TestCollector_PoolsRejectDuplicateSelectedNames(t *testing.T) {
	var pool apiPoolResponse
	pool.PoolName = "pool-a"
	srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == urlPathApiPool {
			writeJSON(t, w, http.StatusOK, []apiPoolResponse{pool, pool})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	defer srv.Close()

	c := newInitializedCollector(t, srv.URL, func(c *Collector) {
		c.Metrics.DashboardAPIStatus = false
		c.Metrics.Pools = true
	})
	defer c.Cleanup(context.Background())
	_, err := c.collect(context.Background())
	require.ErrorContains(t, err, "duplicate selected pool name")
}

func TestCollector_HealthRejectsInvalidEnabledSectionValues(t *testing.T) {
	tests := map[string]struct {
		response  map[string]any
		configure func(*Collector)
		want      string
	}{
		"hosts": {
			response:  map[string]any{"hosts": -1},
			configure: func(c *Collector) { c.Metrics.Hosts = true },
			want:      "negative host count",
		},
		"OSD summary": {
			response:  map[string]any{"osd_map": map[string]any{"osds": []map[string]any{{"up": 2, "in": 1}}}},
			configure: func(c *Collector) { c.Metrics.OSDsSummary = true },
			want:      "invalid up/in state",
		},
		"capacity": {
			response: map[string]any{"df": map[string]any{"stats": map[string]any{
				"total_bytes": 100, "total_used_raw_bytes": 101, "total_avail_bytes": 0,
			}}},
			configure: func(c *Collector) { c.Metrics.Capacity = true },
			want:      "invalid capacity values",
		},
		"objects": {
			response: map[string]any{"pg_info": map[string]any{"object_stats": map[string]any{
				"num_objects": -1, "num_object_copies": 1,
			}}},
			configure: func(c *Collector) { c.Metrics.Objects = true },
			want:      "invalid object/copy health counters",
		},
		"PGs": {
			response: map[string]any{"pg_info": map[string]any{
				"statuses": map[string]any{"active+clean": -1}, "pgs_per_osd": 1,
			}},
			configure: func(c *Collector) { c.Metrics.PGs = true },
			want:      "invalid PG state counts",
		},
		"client IO": {
			response:  map[string]any{"client_perf": map[string]any{"read_bytes_sec": -1}},
			configure: func(c *Collector) { c.Metrics.ClientIO = true },
			want:      "invalid performance value",
		},
		"RGW": {
			response:  map[string]any{"rgw": -1},
			configure: func(c *Collector) { c.Metrics.ObjectGateways = true },
			want:      "negative gateway count",
		},
		"iSCSI": {
			response:  map[string]any{"iscsi_daemons": map[string]any{"up": -1, "down": 0}},
			configure: func(c *Collector) { c.Metrics.ISCSIGateways = true },
			want:      "invalid gateway counts",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv := newFakeDashboard(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == urlPathApiHealthMinimal {
					writeJSON(t, w, http.StatusOK, test.response)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			})
			defer srv.Close()

			c := newInitializedCollector(t, srv.URL, func(c *Collector) {
				c.Metrics.DashboardAPIStatus = false
				test.configure(c)
			})
			defer c.Cleanup(context.Background())
			mx, err := c.collect(context.Background())
			require.ErrorContains(t, err, test.want)
			if name == "objects" {
				assert.NotContains(t, mx, "objects_num")
			}
		})
	}
}

func TestCollector_ChartAlgorithms(t *testing.T) {
	assert.Equal(t, collectorapi.Stacked, clusterObjectsByStatusPercentChart.Type)
	for _, dim := range osdIOChartTmpl.Dims {
		assert.Equal(t, string(collectorapi.Absolute), dim.Algo.String())
	}
	for _, dim := range osdIOPSChartTmpl.Dims {
		assert.Equal(t, string(collectorapi.Absolute), dim.Algo.String())
	}
	for _, dim := range poolIOChartTmpl.Dims {
		assert.Equal(t, collectorapi.Incremental, dim.Algo)
	}
	for _, dim := range clusterObjectsByStatusPercentChart.Dims {
		assert.EqualValues(t, precision, dim.Div)
	}
}

func TestCollector_PublicChartIdentityContract(t *testing.T) {
	want := map[string]string{
		"dashboard_api_status":                   "ceph.dashboard_api_status|status|line|dashboard_api_reachable=reachable,dashboard_api_unreachable=unreachable",
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
		"cluster_objects_by_status":              "ceph.cluster_objects_by_status_distribution|percent|stacked|objects_healthy_num=healthy,objects_misplaced_num=misplaced,objects_degraded_num=degraded,objects_unfound_num=unfound",
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
			got[chart.ID] = fmt.Sprintf("%s|%s|%s|%s", chart.Ctx, chart.Units, chart.Type.String(), strings.Join(dims, ","))
		}
	}
	assert.Equal(t, want, got)

	c := New()
	c.identityMu.Lock()
	c.fsid = "synthetic-fsid"
	c.identityMu.Unlock()
	c.Metrics = CollectConfig{
		DashboardAPIStatus: true, HealthStatus: true, Hosts: true, Monitors: true,
		OSDsSummary: true, Managers: true, ObjectGateways: true, ISCSIGateways: true,
		Capacity: true, Objects: true, PoolsSummary: true, PGs: true,
		ClientIO: true, Recovery: true, ScrubStatus: true,
	}
	c.addClusterCharts()
	c.addOsdCharts("uuid-1", "ssd", "osd.1", true)
	c.addPoolCharts("pool-a", true)
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
	tests := map[string]string{
		"active+clean":         "clean",
		"active+premerge":      "working",
		"active+wait":          "working",
		"active+failed_repair": "warning",
		"active+laggy":         "warning",
		"active+new_state":     "unknown",
		"unknown":              "unknown",
	}
	for status, want := range tests {
		assert.Equal(t, want, pgStatusCategory(status), status)
	}
}

func newFakeDashboard(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathApiAuth:
			writeJSON(t, w, http.StatusCreated, authLoginResp{Token: "test-token"})
			return
		case urlPathApiAuthLogout:
			w.WriteHeader(http.StatusOK)
			return
		case urlPathAPIClusterFSID:
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
			writeJSON(t, w, http.StatusOK, "synthetic-fsid")
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

func TestCollector_RequestConfigRemainsCompatibleWithProxyAndTLSFields(t *testing.T) {
	c := New()
	c.Config.HTTPConfig = web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "https://ceph.example"}}
	assert.Equal(t, "https://ceph.example", c.URL)
}
