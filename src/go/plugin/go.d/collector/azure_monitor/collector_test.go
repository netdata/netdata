// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/azure_monitor/azureprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cloudauth"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		cfg      Config
		wantFail bool
	}{
		"fails on invalid config": {
			cfg: Config{
				UpdateEvery: 1,
			},
			wantFail: true,
		},
		"success on valid config": {
			cfg:      testConfig(),
			wantFail: false,
		},
		"fails on negative timeout": {
			cfg: Config{
				SubscriptionID:       "sub-1",
				Timeout:              confopt.Duration(-time.Second),
				ProfileSelectionMode: profileSelectionModeExact,
				ProfileSelectionModeExact: &ProfileSelectionModeExactConfig{
					Profiles: []string{"postgres_flexible"},
				},
				Auth: cloudauth.AzureADAuthConfig{
					Mode: cloudauth.AzureADAuthModeDefault,
				},
			},
			wantFail: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rg := &mockResourceGraph{}
			mx := &mockMetricsClient{}

			c := New()
			c.Config = tc.cfg
			c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
				return rg, nil
			}
			c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
				return mx, nil
			}

			err := c.Init(context.Background())
			if tc.wantFail {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCollector_InitAuthValidation(t *testing.T) {
	tests := map[string]struct {
		auth           cloudauth.AzureADAuthConfig
		wantErrContain string
	}{
		"missing mode": {
			auth:           cloudauth.AzureADAuthConfig{},
			wantErrContain: "auth.mode is required",
		},
		"missing service principal secret": {
			auth: cloudauth.AzureADAuthConfig{
				Mode: cloudauth.AzureADAuthModeServicePrincipal,
				ModeServicePrincipal: &cloudauth.AzureADModeServicePrincipalConfig{
					TenantID: "tenant",
					ClientID: "client",
				},
			},
			wantErrContain: "auth.mode_service_principal.client_secret is required",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := New()
			c.Config = testConfig()
			c.Config.Auth = tc.auth
			c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
				return &mockResourceGraph{}, nil
			}
			c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
				return &mockMetricsClient{}, nil
			}

			err := c.Init(context.Background())
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantErrContain)
			assert.NotContains(t, err.Error(), "cloud_auth.azure_ad")
		})
	}
}

func TestCollector_UsesConfiguredTimeout(t *testing.T) {
	const timeout = 7 * time.Second

	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		setup    func(*Collector, *mockResourceGraph, *mockMetricsClient)
		act      func(context.Context, *Collector) error
		deadline func(*mockResourceGraph, *mockMetricsClient) (time.Duration, bool)
	}{
		"init auto-discovery": {
			setup: func(c *Collector, rg *mockResourceGraph, mx *mockMetricsClient) {
				c.Config = testConfig()
				c.Config.Timeout = confopt.Duration(timeout)
				c.Config.ProfileSelectionMode = profileSelectionModeAuto
				c.Config.ProfileSelectionModeExact = nil
				rg.resources = []map[string]any{
					{"type": "microsoft.dbforpostgresql/flexibleservers", "count_": int64(1)},
				}
			},
			act: func(ctx context.Context, c *Collector) error {
				return c.Init(ctx)
			},
			deadline: func(rg *mockResourceGraph, _ *mockMetricsClient) (time.Duration, bool) {
				return rg.lastTimeout()
			},
		},
		"resource discovery refresh": {
			setup: func(c *Collector, rg *mockResourceGraph, mx *mockMetricsClient) {
				c.Config = testConfig()
				c.Config.Timeout = confopt.Duration(timeout)
				rg.resources = []map[string]any{
					{
						"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
						"name":          "pg-a",
						"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
						"resourceGroup": "rg-a",
						"location":      "eastus",
					},
				}
			},
			act: func(ctx context.Context, c *Collector) error {
				if err := c.Init(ctx); err != nil {
					return err
				}
				_, err := c.refreshDiscovery(ctx, true)
				return err
			},
			deadline: func(rg *mockResourceGraph, _ *mockMetricsClient) (time.Duration, bool) {
				return rg.lastTimeout()
			},
		},
		"metrics query": {
			setup: func(c *Collector, rg *mockResourceGraph, mx *mockMetricsClient) {
				c.Config = testConfig()
				c.Config.Timeout = confopt.Duration(timeout)
				c.now = func() time.Time { return now }
				rg.resources = []map[string]any{
					{
						"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
						"name":          "pg-a",
						"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
						"resourceGroup": "rg-a",
						"location":      "eastus",
					},
				}
				mx.queryResponse = azmetrics.QueryResourcesResponse{MetricResults: azmetrics.MetricResults{Values: []azmetrics.MetricData{
					{
						ResourceID: ptrString("/subscriptions/sub-1/resourcegroups/rg-a/providers/microsoft.dbforpostgresql/flexibleservers/pg-a"),
						Values: []azmetrics.Metric{
							metricWithAvg("cpu_percent", now, 21.5),
						},
					},
				}}}
			},
			act: func(ctx context.Context, c *Collector) error {
				if err := c.Init(ctx); err != nil {
					return err
				}
				_, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
				return err
			},
			deadline: func(_ *mockResourceGraph, mx *mockMetricsClient) (time.Duration, bool) {
				return mx.lastTimeout()
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rg := &mockResourceGraph{}
			mx := &mockMetricsClient{}

			c := New()
			tc.setup(c, rg, mx)
			c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
				return rg, nil
			}
			c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
				return mx, nil
			}

			require.NoError(t, tc.act(context.Background(), c))

			got, ok := tc.deadline(rg, mx)
			require.True(t, ok)
			assertTimeoutClose(t, got, timeout)
		})
	}
}

func TestCollector_ChartTemplateYAML(t *testing.T) {
	c := New()
	c.Config = testConfig()
	c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
		return &mockResourceGraph{}, nil
	}
	c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
		return &mockMetricsClient{}, nil
	}

	require.NoError(t, c.Init(context.Background()))

	tpl := c.ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, tpl)

	spec, err := charttpl.DecodeYAML([]byte(tpl))
	require.NoError(t, err)
	require.NoError(t, spec.Validate())

	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func TestCollector_Collect(t *testing.T) {
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	rg := &mockResourceGraph{
		resources: []map[string]any{
			{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
				"name":          "pg-a",
				"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
				"resourceGroup": "rg-a",
				"location":      "eastus",
			},
			{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-b/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-b",
				"name":          "pg-b",
				"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
				"resourceGroup": "rg-b",
				"location":      "eastus",
			},
		},
	}

	mx := &mockMetricsClient{
		queryResponse: azmetrics.QueryResourcesResponse{MetricResults: azmetrics.MetricResults{Values: []azmetrics.MetricData{
			{
				ResourceID: ptrString("/subscriptions/sub-1/resourcegroups/rg-a/providers/microsoft.dbforpostgresql/flexibleservers/pg-a"),
				Values: []azmetrics.Metric{
					metricWithAvg("cpu_percent", now, 21.5),
					metricWithAvg("storage_percent", now, 61.2),
				},
			},
			{
				ResourceID: ptrString("/subscriptions/sub-1/resourcegroups/rg-b/providers/microsoft.dbforpostgresql/flexibleservers/pg-b"),
				Values: []azmetrics.Metric{
					metricWithAvg("cpu_percent", now, 33.1),
					metricWithAvg("storage_percent", now, 72.8),
				},
			},
		}}},
	}

	c := New()
	c.Config = testConfig()
	c.now = func() time.Time { return now }
	c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
		return rg, nil
	}
	c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
		return mx, nil
	}

	require.NoError(t, c.Init(context.Background()))

	series, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(series), 4)
	assert.GreaterOrEqual(t, rg.calls(), 1)
	assert.GreaterOrEqual(t, mx.calls(), 1)
}

func TestCollector_TimeGrainScheduling(t *testing.T) {
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	rg := &mockResourceGraph{
		resources: []map[string]any{
			{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.Storage/storageAccounts/st-a",
				"name":          "st-a",
				"type":          "Microsoft.Storage/storageAccounts",
				"resourceGroup": "rg-a",
				"location":      "eastus",
			},
		},
	}

	mx := &mockMetricsClient{
		queryResponse: azmetrics.QueryResourcesResponse{MetricResults: azmetrics.MetricResults{Values: []azmetrics.MetricData{
			{
				ResourceID: ptrString("/subscriptions/sub-1/resourcegroups/rg-a/providers/microsoft.storage/storageaccounts/st-a"),
				Values: []azmetrics.Metric{
					metricWithAvg("UsedCapacity", now, 100),
				},
			},
		}}},
	}

	cfg := testConfig()
	cfg.ProfileSelectionModeExact = &ProfileSelectionModeExactConfig{Profiles: []string{"storage_slow"}}

	catalog := mustLoadStockCatalog(t, map[string]string{
		"storage_slow.yaml": `
id: storage_slow
name: Azure Storage Slow
resource_type: Microsoft.Storage/storageAccounts
metrics:
  - id: used_capacity
    azure_name: UsedCapacity
    time_grain: PT5M
    series:
      - aggregation: average
        kind: gauge
template:
  family: Azure Storage Slow
  context_namespace: storage_slow
  charts:
    - id: am_storage_slow_used_capacity
      title: Azure Storage Slow Used Capacity
      context: used_capacity
      family: Capacity
      type: line
      units: bytes
      algorithm: absolute
      label_promotion: [resource_name, resource_group, region, resource_type, profile]
      instances:
        by_labels: [resource_uid]
      dimensions:
        - selector: ` + azureprofiles.ExportedSeriesName("storage_slow", "used_capacity", "average") + `
          name: average
`,
	})

	c := New()
	c.Config = cfg
	c.now = func() time.Time { return now }
	c.loadProfileCatalog = func() (azureprofiles.Catalog, error) {
		return catalog, nil
	}
	c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
		return rg, nil
	}
	c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
		return mx, nil
	}

	require.NoError(t, c.Init(context.Background()))

	_, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.NoError(t, err)

	_, err = collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.NoError(t, err)

	assert.Equal(t, 1, mx.calls())
}

func TestCollector_InitAutoDiscover(t *testing.T) {
	rg := &mockResourceGraph{
		resources: []map[string]any{
			{"type": "microsoft.dbforpostgresql/flexibleservers", "count_": int64(2)},
		},
	}

	c := New()
	c.Config = testConfig()
	c.Config.ProfileSelectionMode = profileSelectionModeAuto
	c.Config.ProfileSelectionModeExact = nil
	c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
		return rg, nil
	}
	c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
		return &mockMetricsClient{}, nil
	}

	require.NoError(t, c.Init(context.Background()))
	require.NotEmpty(t, c.runtime.Profiles)
	assert.Equal(t, "postgres_flexible", c.runtime.Profiles[0].ID)
}

func TestCollector_InitAutoDiscoverWithExplicit(t *testing.T) {
	rg := &mockResourceGraph{
		resources: []map[string]any{
			{"type": "microsoft.dbforpostgresql/flexibleservers", "count_": int64(2)},
		},
	}

	c := New()
	c.Config = testConfig()
	c.Config.ProfileSelectionMode = profileSelectionModeCombined
	c.Config.ProfileSelectionModeExact = nil
	c.Config.ProfileSelectionModeCombined = &ProfileSelectionModeCombinedConfig{
		Profiles: []string{"cosmos_db"},
	}
	c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
		return rg, nil
	}
	c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
		return &mockMetricsClient{}, nil
	}

	require.NoError(t, c.Init(context.Background()))
	var ids []string
	for _, p := range c.runtime.Profiles {
		ids = append(ids, p.ID)
	}
	assert.Contains(t, ids, "cosmos_db")
	assert.Contains(t, ids, "postgres_flexible")
}

func TestCollector_InitDefaultModeIsAuto(t *testing.T) {
	c := New()
	c.Config = testConfig()
	c.Config.ProfileSelectionMode = ""
	c.Config.ProfileSelectionModeExact = nil
	c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
		return &mockResourceGraph{}, nil
	}
	c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
		return &mockMetricsClient{}, nil
	}

	err := c.Init(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "auto-discovery found no Azure resources")
}

func TestCollector_InitAutoDiscoverNoMatchFails(t *testing.T) {
	rg := &mockResourceGraph{
		resources: []map[string]any{
			{"type": "microsoft.fakeservice/fakeresources", "count_": int64(3)},
		},
	}

	c := New()
	c.Config = testConfig()
	c.Config.ProfileSelectionMode = profileSelectionModeAuto
	c.Config.ProfileSelectionModeExact = nil
	c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
		return rg, nil
	}
	c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
		return &mockMetricsClient{}, nil
	}

	err := c.Init(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "auto-discovery found no Azure resources")
}

func TestMergeProfileIDs(t *testing.T) {
	tests := map[string]struct {
		explicit   []string
		discovered []string
		want       []string
	}{
		"no overlap": {
			explicit:   []string{"vm"},
			discovered: []string{"sql"},
			want:       []string{"vm", "sql"},
		},
		"overlap deduplicated": {
			explicit:   []string{"vm", "sql"},
			discovered: []string{"sql", "redis"},
			want:       []string{"vm", "sql", "redis"},
		},
		"both empty": {
			explicit:   []string{},
			discovered: []string{},
			want:       []string{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := mergeProfileIDs(tc.explicit, tc.discovered)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuildCollectorRuntime_DetectsChartIDCollision(t *testing.T) {
	profileIDs := []string{"redis_upper", "redis_lower"}

	catalog := mustLoadStockCatalog(t, map[string]string{
		"redis_upper.yaml": `
id: redis_upper
name: Azure Redis Cache
resource_type: Microsoft.Cache/Redis
metrics:
  - id: connectedclients
    azure_name: connectedclients
    time_grain: PT1M
    series:
      - aggregation: average
        kind: gauge
template:
  family: Azure Redis Cache
  context_namespace: redis_upper
  charts:
    - id: am_collision_chart
      title: Azure Redis Cache Connected Clients
      context: connected_clients_upper
      family: Connections
      type: line
      units: connections
      algorithm: absolute
      dimensions:
        - selector: ` + azureprofiles.ExportedSeriesName("redis_upper", "connectedclients", "average") + `
          name: average
`,
		"redis_lower.yaml": `
id: redis_lower
name: azure redis cache
resource_type: Microsoft.Cache/Redis
metrics:
  - id: cachehits
    azure_name: cachehits
    time_grain: PT1M
    series:
      - aggregation: average
        kind: gauge
template:
  family: azure redis cache
  context_namespace: redis_lower
  charts:
    - id: am_collision_chart
      title: Azure Redis Cache Hits
      context: cache_hits_lower
      family: Throughput
      type: line
      units: hits/s
      algorithm: absolute
      dimensions:
        - selector: ` + azureprofiles.ExportedSeriesName("redis_lower", "cachehits", "average") + `
          name: average
`,
	})

	_, err := buildCollectorRuntimeFromConfig(profileIDs, catalog)
	require.Error(t, err)
}

func testConfig() Config {
	return Config{
		UpdateEvery:          60,
		AutoDetectionRetry:   0,
		SubscriptionID:       "sub-1",
		Cloud:                "public",
		DiscoveryEvery:       300,
		QueryOffset:          180,
		Timeout:              defaultTimeout,
		MaxConcurrency:       4,
		MaxBatchResources:    50,
		MaxMetricsPerQuery:   20,
		ProfileSelectionMode: profileSelectionModeExact,
		ProfileSelectionModeExact: &ProfileSelectionModeExactConfig{
			Profiles: []string{"postgres_flexible"},
		},
		Auth: cloudauth.AzureADAuthConfig{
			Mode: cloudauth.AzureADAuthModeDefault,
		},
	}
}

func mustLoadStockCatalog(t *testing.T, files map[string]string) azureprofiles.Catalog {
	t.Helper()

	dir := t.TempDir()
	stockDir := filepath.Join(dir, "stock")

	for name, data := range files {
		require.NoError(t, writeProfileFile(filepath.Join(stockDir, name), data))
	}

	catalog, err := azureprofiles.LoadFromDirs([]azureprofiles.DirSpec{{Path: stockDir, IsStock: true}})
	require.NoError(t, err)
	return catalog
}

type mockResourceGraph struct {
	mu        sync.Mutex
	resources []map[string]any
	count     int
	timeout   time.Duration
	hasDL     bool
}

func (m *mockResourceGraph) Resources(ctx context.Context, _ armresourcegraph.QueryRequest, _ *armresourcegraph.ClientResourcesOptions) (armresourcegraph.ClientResourcesResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.count++
	deadline, ok := ctx.Deadline()
	m.hasDL = ok
	if ok {
		m.timeout = time.Until(deadline)
	} else {
		m.timeout = 0
	}

	rows := make([]any, 0, len(m.resources))
	for _, r := range m.resources {
		rows = append(rows, r)
	}
	return armresourcegraph.ClientResourcesResponse{QueryResponse: armresourcegraph.QueryResponse{Data: rows}}, nil
}

func (m *mockResourceGraph) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

func (m *mockResourceGraph) lastTimeout() (time.Duration, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.timeout, m.hasDL
}

type mockMetricsClient struct {
	mu            sync.Mutex
	queryResponse azmetrics.QueryResourcesResponse
	queryErr      error
	count         int
	timeout       time.Duration
	hasDL         bool
}

func (m *mockMetricsClient) QueryResources(ctx context.Context, _ string, _ string, _ []string, _ azmetrics.ResourceIDList, _ *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.count++
	deadline, ok := ctx.Deadline()
	m.hasDL = ok
	if ok {
		m.timeout = time.Until(deadline)
	} else {
		m.timeout = 0
	}
	if m.queryErr != nil {
		return azmetrics.QueryResourcesResponse{}, m.queryErr
	}
	return m.queryResponse, nil
}

func (m *mockMetricsClient) calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.count
}

func (m *mockMetricsClient) lastTimeout() (time.Duration, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.timeout, m.hasDL
}

func metricWithAvg(name string, ts time.Time, value float64) azmetrics.Metric {
	return azmetrics.Metric{
		Name: &azmetrics.LocalizableString{Value: ptrString(name)},
		TimeSeries: []azmetrics.TimeSeriesElement{
			{Data: []azmetrics.MetricValue{{TimeStamp: &ts, Average: &value}}},
		},
	}
}

func ptrString(v string) *string { return &v }

func assertTimeoutClose(t *testing.T, got, want time.Duration) {
	t.Helper()
	assert.InDelta(t, want.Seconds(), got.Seconds(), 1.0)
}
