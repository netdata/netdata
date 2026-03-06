// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/monitor/armmonitor"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
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
			c.newMetricDefinitions = func(string, azcore.TokenCredential, azcloud.Configuration) (metricDefinitionsClient, error) {
				return mockMetricDefinitions{}, nil
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

func TestCollector_ChartTemplateYAML(t *testing.T) {
	c := New()
	c.Config = testConfig()
	c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
		return &mockResourceGraph{}, nil
	}
	c.newMetricDefinitions = func(string, azcore.TokenCredential, azcloud.Configuration) (metricDefinitionsClient, error) {
		return mockMetricDefinitions{}, nil
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
	c.newMetricDefinitions = func(string, azcore.TokenCredential, azcloud.Configuration) (metricDefinitionsClient, error) {
		return mockMetricDefinitions{}, nil
	}
	c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
		return mx, nil
	}

	require.NoError(t, c.Init(context.Background()))

	series, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(series), 4)
	assert.GreaterOrEqual(t, rg.calls, 1)
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
	cfg.Profiles = []string{"storage_slow"}

	catalog := profileCatalog{
		byName: map[string]ProfileConfig{
			"storage_slow": {
				Name:         "Azure Storage Slow",
				ResourceType: "Microsoft.Storage/storageAccounts",
				Metrics: []MetricConfig{
					{Name: "UsedCapacity", Aggregations: []string{"average"}, TimeGrain: "PT5M", Units: "bytes"},
				},
			},
		},
		stockProfileSet: map[string]struct{}{
			"storage_slow": {},
		},
	}

	c := New()
	c.Config = cfg
	c.now = func() time.Time { return now }
	c.loadProfileCatalog = func() (profileCatalog, error) {
		return catalog, nil
	}
	c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
		return rg, nil
	}
	c.newMetricDefinitions = func(string, azcore.TokenCredential, azcloud.Configuration) (metricDefinitionsClient, error) {
		return mockMetricDefinitions{}, nil
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

func TestBuildRuntimePlan_DetectsProfileCollision(t *testing.T) {
	c := New()
	cfg := testConfig()
	cfg.Profiles = []string{"redis_upper", "redis_lower"}

	catalog := profileCatalog{
		byName: map[string]ProfileConfig{
			"redis_upper": {
				Name:         "Azure Redis Cache",
				ResourceType: "Microsoft.Cache/Redis",
				Metrics:      []MetricConfig{{Name: "connectedclients", Aggregations: []string{"average"}, TimeGrain: "PT1M"}},
			},
			"redis_lower": {
				Name:         "azure redis cache",
				ResourceType: "Microsoft.Cache/Redis",
				Metrics:      []MetricConfig{{Name: "cachehits", Aggregations: []string{"average"}, TimeGrain: "PT1M"}},
			},
		},
		stockProfileSet: map[string]struct{}{
			"redis_upper": {},
			"redis_lower": {},
		},
	}

	_, err := c.buildRuntimePlanFromConfig(cfg, catalog)
	require.Error(t, err)
}

func testConfig() Config {
	return Config{
		UpdateEvery:        60,
		AutoDetectionRetry: 0,
		SubscriptionID:     "sub-1",
		Cloud:              "public",
		DiscoveryEvery:     300,
		QueryOffset:        180,
		MaxConcurrency:     4,
		MaxBatchResources:  50,
		MaxMetricsPerQuery: 20,
		Profiles:           []string{"postgres_flexible"},
		Auth:               AuthConfig{Mode: "default"},
	}
}

type mockResourceGraph struct {
	resources []map[string]any
	calls     int
}

func (m *mockResourceGraph) Resources(_ context.Context, _ armresourcegraph.QueryRequest, _ *armresourcegraph.ClientResourcesOptions) (armresourcegraph.ClientResourcesResponse, error) {
	m.calls++
	rows := make([]any, 0, len(m.resources))
	for _, r := range m.resources {
		rows = append(rows, r)
	}
	return armresourcegraph.ClientResourcesResponse{QueryResponse: armresourcegraph.QueryResponse{Data: rows}}, nil
}

type mockMetricDefinitions struct{}

func (mockMetricDefinitions) NewListPager(string, *armmonitor.MetricDefinitionsClientListOptions) *runtime.Pager[armmonitor.MetricDefinitionsClientListResponse] {
	return nil
}

type mockMetricsClient struct {
	mu            sync.Mutex
	queryResponse azmetrics.QueryResourcesResponse
	queryErr      error
	count         int
}

func (m *mockMetricsClient) QueryResources(_ context.Context, _ string, _ string, _ []string, _ azmetrics.ResourceIDList, _ *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.count++
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

func metricWithAvg(name string, ts time.Time, value float64) azmetrics.Metric {
	return azmetrics.Metric{
		Name: &azmetrics.LocalizableString{Value: ptrString(name)},
		TimeSeries: []azmetrics.TimeSeriesElement{
			{Data: []azmetrics.MetricValue{{TimeStamp: &ts, Average: &value}}},
		},
	}
}

func ptrString(v string) *string { return &v }
