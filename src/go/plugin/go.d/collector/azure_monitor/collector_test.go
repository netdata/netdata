// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	azcloud "github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resourcegraph/armresourcegraph"
	"github.com/google/uuid"
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
	dataConfigJSON, _         = os.ReadFile("testdata/config.json")
	dataConfigYAML, _         = os.ReadFile("testdata/config.yaml")
	dataConfigWorkloadJSON, _ = os.ReadFile("testdata/config_workload.json")
	dataConfigWorkloadYAML, _ = os.ReadFile("testdata/config_workload.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":         dataConfigJSON,
		"dataConfigYAML":         dataConfigYAML,
		"dataConfigWorkloadJSON": dataConfigWorkloadJSON,
		"dataConfigWorkloadYAML": dataConfigWorkloadYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestConfigSchema_RuntimeContract(t *testing.T) {
	raw, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))

	schema := requireMapField(t, doc, "jsonSchema")
	assert.ElementsMatch(t, []string{"subscription_ids", "auth"}, requireStringSliceField(t, schema, "required"))

	properties := requireMapField(t, schema, "properties")

	discovery := requireMapField(t, properties, "discovery")
	assert.NotContains(t, discovery, "required")
	discoveryProps := requireMapField(t, discovery, "properties")
	assert.Contains(t, discoveryProps, "refresh_every")
	assert.Contains(t, discoveryProps, "mode")
	assert.NotContains(t, discoveryProps, "mode_filters")
	assert.NotContains(t, discoveryProps, "mode_query")

	profiles := requireMapField(t, properties, "profiles")
	assert.NotContains(t, profiles, "required")
	profileProps := requireMapField(t, profiles, "properties")
	assert.Contains(t, profileProps, "mode")
	assert.NotContains(t, profileProps, "mode_exact")
	assert.NotContains(t, profileProps, "mode_combined")

	virtualNodes := requireMapField(t, properties, "virtual_nodes")
	assert.NotContains(t, virtualNodes, "required")
	virtualNodeProps := requireMapField(t, virtualNodes, "properties")
	byResourceTag := requireMapField(t, virtualNodeProps, "by_resource_tag")
	assert.Equal(t, "string", byResourceTag["type"])

	uiSchema := requireMapField(t, doc, "uiSchema")
	uiProfiles := requireMapField(t, uiSchema, "profiles")
	_, hasIDs := uiProfiles["ids"]
	assert.False(t, hasIDs)
	_, hasNames := uiProfiles["names"]
	assert.False(t, hasNames)
	_, hasModeAuto := uiProfiles["mode_auto"]
	assert.True(t, hasModeAuto)
	_, hasModeExact := uiProfiles["mode_exact"]
	assert.True(t, hasModeExact)
	_, hasModeCombined := uiProfiles["mode_combined"]
	assert.True(t, hasModeCombined)

	tabs := requireArrayField(t, requireMapField(t, uiSchema, "ui:options"), "tabs")
	var baseFields, vnodeFields []string
	for _, item := range tabs {
		tab, ok := item.(map[string]any)
		require.True(t, ok)
		switch tab["title"] {
		case "Base":
			baseFields = requireStringSliceField(t, tab, "fields")
		case "Virtual Node":
			vnodeFields = requireStringSliceField(t, tab, "fields")
		}
	}
	assert.NotContains(t, baseFields, "vnode")
	assert.NotContains(t, baseFields, "virtual_nodes")
	assert.ElementsMatch(t, []string{"vnode", "virtual_nodes"}, vnodeFields)
}

func TestConfigSchema_ProfileTagHelpText(t *testing.T) {
	raw, err := os.ReadFile("config_schema.json")
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))

	schema := requireMapField(t, doc, "jsonSchema")
	assert.NotContains(t, schema, "allOf")

	uiSchema := requireMapField(t, doc, "uiSchema")
	discovery := requireMapField(t, uiSchema, "discovery")
	modeQuery := requireMapField(t, discovery, "mode_query")
	kql := requireMapField(t, modeQuery, "kql")
	help, ok := kql["ui:help"].(string)
	require.True(t, ok)
	assert.Contains(t, help, "project `tags`")

	uiProfiles := requireMapField(t, uiSchema, "profiles")
	for _, mode := range []string{"mode_auto", "mode_exact", "mode_combined"} {
		modeUI := requireMapField(t, uiProfiles, mode)
		entries := requireMapField(t, modeUI, "entries")
		items := requireMapField(t, entries, "items")
		filters := requireMapField(t, items, "filters")
		tags := requireMapField(t, filters, "tags")
		help, ok := tags["ui:help"].(string)
		require.True(t, ok)
		assert.Contains(t, help, "Only supported when `discovery.mode` is `filters`")
	}
}

func TestAzureMonitorStaticArtifacts_WorkloadVirtualNodes(t *testing.T) {
	tests := map[string]struct {
		path       string
		contains   []string
		notContain []string
	}{
		"metadata": {
			path: "metadata.yaml",
			contains: []string{
				"virtual_nodes.by_resource_tag",
				"_vnode_type=azure_workload",
				"azure_monitor:",
				"More alerts appear after enabling workload virtual nodes",
			},
		},
		"stock config": {
			path: "../../config/go.d/azure_monitor.conf",
			contains: []string{
				"subscription_ids:",
				"virtual_nodes:",
				"by_resource_tag: workload",
				"mode_exact:",
				"mode_combined:",
			},
			notContain: []string{
				"subscription_id:",
				"profile_selection_mode",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(tc.path)
			require.NoError(t, err)
			text := string(raw)
			for _, want := range tc.contains {
				assert.Contains(t, text, want)
			}
			for _, unwanted := range tc.notContain {
				assert.NotContains(t, text, unwanted)
			}
		})
	}

	docs, err := filepath.Glob("integrations/*.md")
	require.NoError(t, err)
	require.Greater(t, len(docs), 1)
	for _, path := range docs {
		raw, err := os.ReadFile(path)
		require.NoError(t, err, path)
		text := string(raw)
		assert.Contains(t, text, "virtual_nodes.by_resource_tag", path)
		assert.Contains(t, text, "_vnode_type=azure_workload", path)
	}
}

func TestWorkloadScopeScenarios(t *testing.T) {
	tests := map[string]struct {
		tags         []resourceTag
		tagKey       string
		wantDefault  bool
		wantUnsafe   string
		wantHostname string
	}{
		"matching tag creates namespaced scope": {
			tags:         []resourceTag{{Key: "workload", Value: " Api "}},
			tagKey:       "WORKLOAD",
			wantHostname: "Api",
		},
		"missing tag uses default": {
			tags:        []resourceTag{{Key: "env", Value: "prod"}},
			tagKey:      "workload",
			wantDefault: true,
		},
		"empty tag value uses default": {
			tags:        []resourceTag{{Key: "workload", Value: "   "}},
			tagKey:      "workload",
			wantDefault: true,
		},
		"unsafe hostname uses default and reports value": {
			tags:        []resourceTag{{Key: "workload", Value: "api\nprod"}},
			tagKey:      "workload",
			wantDefault: true,
			wantUnsafe:  "api\nprod",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scope, unsafeValue := workloadHostScope(resourceInfo{Tags: tc.tags}, tc.tagKey)

			assert.Equal(t, tc.wantUnsafe, unsafeValue)
			if tc.wantDefault {
				assert.True(t, scope.IsDefault())
				return
			}

			wantGUID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte(azureWorkloadGUIDPrefix+tc.wantHostname)).String()
			assert.Equal(t, wantGUID, scope.GUID)
			assert.Equal(t, wantGUID, scope.ScopeKey)
			assert.Equal(t, tc.wantHostname, scope.Hostname)
			assert.Equal(t, map[string]string{azureWorkloadScopeLabelKey: azureWorkloadScopeLabelValue}, scope.Labels)
			assert.NotEqual(t, uuid.NewSHA1(uuid.NameSpaceDNS, []byte(tc.wantHostname)).String(), scope.GUID)
		})
	}
}

func TestConfig_WorkloadResourceTagKey(t *testing.T) {
	tests := map[string]struct {
		virtualNodes *VirtualNodesConfig
		wantKey      string
		wantNil      bool
	}{
		"unset disables workload scoping": {
			wantNil: true,
		},
		"whitespace disables workload scoping": {
			virtualNodes: &VirtualNodesConfig{ByResourceTag: " \t "},
			wantNil:      true,
		},
		"normalizes configured tag key": {
			virtualNodes: &VirtualNodesConfig{ByResourceTag: " Workload "},
			wantKey:      "workload",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := testConfig()
			cfg.VirtualNodes = tc.virtualNodes
			cfg.applyDefaults()

			assert.Equal(t, tc.wantKey, cfg.workloadResourceTagKey())
			if tc.wantNil {
				assert.Nil(t, cfg.VirtualNodes)
				return
			}
			require.NotNil(t, cfg.VirtualNodes)
			assert.Equal(t, tc.wantKey, cfg.VirtualNodes.ByResourceTag)
		})
	}
}

func TestDiscoveryState_WorkloadScopes(t *testing.T) {
	runtime := &collectorRuntime{
		WorkloadResourceTagKey: "workload",
		Profiles: []*profileRuntime{{
			Name:         "sql_database",
			ResourceType: "Microsoft.Sql/servers/databases",
		}},
	}
	resources := []resourceInfo{
		{
			SubscriptionID: "sub-1",
			ID:             "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.Sql/servers/sql-a/databases/db-a",
			UID:            "uid-a",
			Name:           "db-a",
			Type:           "Microsoft.Sql/servers/databases",
			ResourceGroup:  "rg-a",
			Region:         "eastus",
			Tags:           []resourceTag{{Key: "workload", Value: "api"}},
		},
	}

	state := buildDiscoveryState(resources, runtime, time.Unix(0, 0), 300, 7, discoveryFetchResult{})
	require.Len(t, state.Resources, 1)
	scope := state.Resources[0].HostScope
	require.False(t, scope.IsDefault())
	assert.Equal(t, "api", scope.Hostname)
	assert.Equal(t, scope.GUID, scope.ScopeKey)
	assert.Equal(t, state.Resources, state.ByProfile["sql_database"])

	changed := state.Resources[0]
	changed.HostScope = metrix.HostScope{}
	assert.False(t, equalResourceInfo(state.Resources[0], changed))
}

func TestParseStrictQueryDiscoveryRow_OptionalTags(t *testing.T) {
	base := map[string]any{
		"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.Sql/servers/sql-a/databases/db-a",
		"name":          "db-a",
		"type":          "Microsoft.Sql/servers/databases",
		"resourceGroup": "rg-a",
		"location":      "eastus",
	}

	tests := map[string]struct {
		mutate        func(map[string]any)
		wantShape     queryTagsShape
		wantTags      []resourceTag
		wantErrSubstr string
	}{
		"absent tags": {
			wantShape: queryTagsShapeAbsent,
		},
		"map tags": {
			mutate: func(row map[string]any) {
				row["tags"] = map[string]any{"Workload": "api"}
			},
			wantShape: queryTagsShapePresentMap,
			wantTags:  []resourceTag{{Key: "workload", Value: "api"}},
		},
		"null tags are projected empty tags": {
			mutate: func(row map[string]any) {
				row["tags"] = nil
			},
			wantShape: queryTagsShapePresentMap,
		},
		"wrong shaped tags do not fail required parsing": {
			mutate: func(row map[string]any) {
				row["tags"] = []any{"not", "a", "map"}
			},
			wantShape: queryTagsShapeWrong,
		},
		"missing required column still fails": {
			mutate: func(row map[string]any) {
				delete(row, "location")
			},
			wantShape:     queryTagsShapeAbsent,
			wantErrSubstr: `missing required column "location"`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			row := make(map[string]any, len(base)+1)
			maps.Copy(row, base)
			if tc.mutate != nil {
				tc.mutate(row)
			}

			resource, shape, err := parseStrictQueryDiscoveryRow(row)
			assert.Equal(t, tc.wantShape, shape)
			if tc.wantErrSubstr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrSubstr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantTags, resource.Tags)
		})
	}
}

func requireMapField(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()

	value, ok := m[key]
	require.Truef(t, ok, "missing key %q", key)
	out, ok := value.(map[string]any)
	require.Truef(t, ok, "key %q is not an object", key)
	return out
}

func requireStringSliceField(t *testing.T, m map[string]any, key string) []string {
	t.Helper()

	value, ok := m[key]
	require.Truef(t, ok, "missing key %q", key)
	items, ok := value.([]any)
	require.Truef(t, ok, "key %q is not an array", key)

	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		require.Truef(t, ok, "key %q contains a non-string item", key)
		out = append(out, s)
	}
	return out
}

func requireArrayField(t *testing.T, m map[string]any, key string) []any {
	t.Helper()

	value, ok := m[key]
	require.Truef(t, ok, "missing key %q", key)
	items, ok := value.([]any)
	require.Truef(t, ok, "key %q is not an array", key)
	return items
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_ConfigurationSerialize_WorkloadVirtualNodes(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigWorkloadJSON, dataConfigWorkloadYAML)
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
				SubscriptionIDs: []string{"sub-1"},
				Timeout:         confopt.Duration(-time.Second),
				Profiles: ProfilesConfig{
					Mode:      profilesModeExact,
					ModeExact: testProfilesModeEntries("postgres_flexible"),
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
				c.Config.Profiles.Mode = profilesModeAuto
				c.Config.Profiles.ModeExact = nil
				c.Config.Profiles.ModeCombined = nil
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
				return c.Check(ctx)
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
				if err := c.Check(ctx); err != nil {
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
	require.NoError(t, c.Check(context.Background()))

	tpl := c.ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, tpl)

	spec, err := charttpl.DecodeYAML([]byte(tpl))
	require.NoError(t, err)
	require.NoError(t, spec.Validate())

	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func TestCollector_InitThenCheckRunsSingleDiscoveryBootstrap(t *testing.T) {
	rg := &mockResourceGraph{
		resources: []map[string]any{
			{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
				"name":          "pg-a",
				"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
				"resourceGroup": "rg-a",
				"location":      "eastus",
			},
		},
	}

	c := New()
	c.Config = testConfig()
	c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
		return rg, nil
	}
	c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
		return &mockMetricsClient{}, nil
	}

	require.NoError(t, c.Init(context.Background()))
	assert.Equal(t, 0, rg.calls())

	require.NoError(t, c.Check(context.Background()))
	assert.Equal(t, 1, rg.calls())
}

func TestCollector_RefreshDiscoveryDisabledWhenRefreshEveryZero(t *testing.T) {
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
		},
	}
	mx := &mockMetricsClient{
		queryResponse: azmetrics.QueryResourcesResponse{MetricResults: azmetrics.MetricResults{Values: []azmetrics.MetricData{
			{
				ResourceID: ptrString("/subscriptions/sub-1/resourcegroups/rg-a/providers/microsoft.dbforpostgresql/flexibleservers/pg-a"),
				Values: []azmetrics.Metric{
					metricWithAvg("cpu_percent", now, 21.5),
				},
			},
		}}},
	}

	c := newTestCollectorWithMocks(rg, mx)
	c.Config = testConfig()
	c.Config.Discovery.RefreshEvery = 0
	c.now = func() time.Time { return now }

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	assert.Equal(t, 1, rg.calls())

	now = now.Add(10 * time.Minute)

	_, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.NoError(t, err)
	assert.Equal(t, 1, rg.calls())
}

func TestCollector_RefreshDiscoveryFailureFallsBackToLastKnownSnapshot(t *testing.T) {
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
		},
	}
	mx := &mockMetricsClient{
		queryResponse: azmetrics.QueryResourcesResponse{MetricResults: azmetrics.MetricResults{Values: []azmetrics.MetricData{
			{
				ResourceID: ptrString("/subscriptions/sub-1/resourcegroups/rg-a/providers/microsoft.dbforpostgresql/flexibleservers/pg-a"),
				Values: []azmetrics.Metric{
					metricWithAvg("cpu_percent", now, 21.5),
				},
			},
		}}},
	}

	c := newTestCollectorWithMocks(rg, mx)
	c.Config = testConfig()
	c.Config.Discovery.RefreshEvery = 60
	c.now = func() time.Time { return now }

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	assert.Equal(t, 1, rg.calls())
	assert.Equal(t, uint64(1), c.discovery.FetchCounter)

	rg.responseErr = errors.New("refresh failed")
	now = now.Add(10 * time.Minute)

	series, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.NoError(t, err)
	assert.NotEmpty(t, series)
	assert.Contains(t, strings.Join(keysFromSeries(series), "\n"), `subscription_id="sub-1"`)
	assert.Equal(t, 2, rg.calls())
	assert.GreaterOrEqual(t, mx.calls(), 1)
	assert.Equal(t, uint64(1), c.discovery.FetchCounter)
}

func TestCollector_CollectScenarios(t *testing.T) {
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		prepare func(*Collector, *mockResourceGraph, *mockMetricsClient)
		check   func(*testing.T, map[string]metrix.SampleValue, error, *mockResourceGraph, *mockMetricsClient)
	}{
		"success on discovered resources": {
			prepare: func(c *Collector, rg *mockResourceGraph, mx *mockMetricsClient) {
				c.Config = testConfig()
				c.now = func() time.Time { return now }
				rg.resources = []map[string]any{
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
				}
				mx.queryResponse = azmetrics.QueryResourcesResponse{MetricResults: azmetrics.MetricResults{Values: []azmetrics.MetricData{
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
				}}}
			},
			check: func(t *testing.T, series map[string]metrix.SampleValue, err error, rg *mockResourceGraph, mx *mockMetricsClient) {
				require.NoError(t, err)
				assert.GreaterOrEqual(t, len(series), 4)
				assert.Equal(t, 1, rg.calls())
				assert.GreaterOrEqual(t, mx.calls(), 1)
			},
		},
		"collects across multiple subscriptions": {
			prepare: func(c *Collector, rg *mockResourceGraph, mx *mockMetricsClient) {
				c.Config = testConfig()
				c.Config.SubscriptionIDs = []string{"sub-1", "sub-2"}
				c.now = func() time.Time { return now }
				rg.resources = []map[string]any{
					{
						"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
						"name":          "pg-a",
						"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
						"resourceGroup": "rg-a",
						"location":      "eastus",
					},
					{
						"id":            "/subscriptions/sub-2/resourceGroups/rg-b/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-b",
						"name":          "pg-b",
						"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
						"resourceGroup": "rg-b",
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
					{
						ResourceID: ptrString("/subscriptions/sub-2/resourcegroups/rg-b/providers/microsoft.dbforpostgresql/flexibleservers/pg-b"),
						Values: []azmetrics.Metric{
							metricWithAvg("cpu_percent", now, 33.1),
						},
					},
				}}}
			},
			check: func(t *testing.T, series map[string]metrix.SampleValue, err error, rg *mockResourceGraph, mx *mockMetricsClient) {
				require.NoError(t, err)

				var keys []string
				for key := range series {
					keys = append(keys, key)
				}

				assert.Equal(t, 1, rg.calls())
				assert.ElementsMatch(t, []string{"sub-1", "sub-2"}, rg.lastSubscriptions())
				assert.GreaterOrEqual(t, mx.calls(), 2)
				assert.ElementsMatch(t, []string{"sub-1", "sub-2"}, uniqueStrings(mx.subscriptionCalls()))
				assert.Contains(t, strings.Join(keys, "\n"), `subscription_id="sub-1"`)
				assert.Contains(t, strings.Join(keys, "\n"), `subscription_id="sub-2"`)
			},
		},
		"idle when no resources match": {
			prepare: func(c *Collector, rg *mockResourceGraph, mx *mockMetricsClient) {
				c.Config = testConfig()
			},
			check: func(t *testing.T, series map[string]metrix.SampleValue, err error, rg *mockResourceGraph, mx *mockMetricsClient) {
				require.NoError(t, err)
				assert.Empty(t, series)
			},
		},
		"partial batch failure still succeeds": {
			prepare: func(c *Collector, rg *mockResourceGraph, mx *mockMetricsClient) {
				c.Config = testConfig()
				c.Config.SubscriptionIDs = []string{"sub-1", "sub-2"}
				c.now = func() time.Time { return now }
				rg.resources = []map[string]any{
					{
						"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
						"name":          "pg-a",
						"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
						"resourceGroup": "rg-a",
						"location":      "eastus",
					},
					{
						"id":            "/subscriptions/sub-2/resourceGroups/rg-b/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-b",
						"name":          "pg-b",
						"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
						"resourceGroup": "rg-b",
						"location":      "eastus",
					},
				}
				mx.queryResponses = map[string]azmetrics.QueryResourcesResponse{
					"sub-1": {MetricResults: azmetrics.MetricResults{Values: []azmetrics.MetricData{
						{
							ResourceID: ptrString("/subscriptions/sub-1/resourcegroups/rg-a/providers/microsoft.dbforpostgresql/flexibleservers/pg-a"),
							Values: []azmetrics.Metric{
								metricWithAvg("cpu_percent", now, 21.5),
							},
						},
					}}},
				}
				mx.queryErrors = map[string]error{
					"sub-2": assert.AnError,
				}
			},
			check: func(t *testing.T, series map[string]metrix.SampleValue, err error, rg *mockResourceGraph, mx *mockMetricsClient) {
				require.NoError(t, err)
				assert.NotEmpty(t, series)
				assert.Contains(t, strings.Join(keysFromSeries(series), "\n"), `subscription_id="sub-1"`)
			},
		},
		"all batches fail": {
			prepare: func(c *Collector, rg *mockResourceGraph, mx *mockMetricsClient) {
				c.Config = testConfig()
				c.Config.SubscriptionIDs = []string{"sub-1", "sub-2"}
				rg.resources = []map[string]any{
					{
						"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
						"name":          "pg-a",
						"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
						"resourceGroup": "rg-a",
						"location":      "eastus",
					},
					{
						"id":            "/subscriptions/sub-2/resourceGroups/rg-b/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-b",
						"name":          "pg-b",
						"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
						"resourceGroup": "rg-b",
						"location":      "eastus",
					},
				}
				mx.queryErrors = map[string]error{
					"sub-1": assert.AnError,
					"sub-2": assert.AnError,
				}
			},
			check: func(t *testing.T, series map[string]metrix.SampleValue, err error, rg *mockResourceGraph, mx *mockMetricsClient) {
				require.Error(t, err)
				assert.ErrorContains(t, err, "all Azure Monitor batch queries failed")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rg := &mockResourceGraph{}
			mx := &mockMetricsClient{}
			c := newTestCollectorWithMocks(rg, mx)
			tc.prepare(c, rg, mx)

			require.NoError(t, c.Init(context.Background()))

			series, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
			tc.check(t, series, err, rg, mx)
		})
	}
}

func TestCollector_WorkloadMode_MixedTaggedAndUntaggedResources_RouteToCorrectScopes(t *testing.T) {
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	taggedID := "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a"
	untaggedID := "/subscriptions/sub-1/resourceGroups/rg-b/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-b"

	rg := &mockResourceGraph{
		resources: []map[string]any{
			{
				"id":            taggedID,
				"name":          "pg-a",
				"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
				"resourceGroup": "rg-a",
				"location":      "eastus",
				"tags":          map[string]any{"workload": "api"},
			},
			{
				"id":            untaggedID,
				"name":          "pg-b",
				"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
				"resourceGroup": "rg-b",
				"location":      "eastus",
				"tags":          map[string]any{"env": "prod"},
			},
		},
	}
	mx := &mockMetricsClient{
		queryResponse: azmetrics.QueryResourcesResponse{MetricResults: azmetrics.MetricResults{Values: []azmetrics.MetricData{
			{
				ResourceID: ptrString(strings.ToLower(taggedID)),
				Values: []azmetrics.Metric{
					metricWithAvg("cpu_percent", now, 21.5),
				},
			},
			{
				ResourceID: ptrString(strings.ToLower(untaggedID)),
				Values: []azmetrics.Metric{
					metricWithAvg("cpu_percent", now, 33.1),
				},
			},
		}}},
	}
	c := newTestCollectorWithMocks(rg, mx)
	c.Config = testConfig()
	c.Config.VirtualNodes = &VirtualNodesConfig{ByResourceTag: "workload"}
	c.now = func() time.Time { return now }

	require.NoError(t, c.Init(context.Background()))
	defaultSeries, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.NoError(t, err)
	assert.Contains(t, strings.Join(keysFromSeries(defaultSeries), "\n"), `resource_name="pg-b"`)
	assert.NotContains(t, strings.Join(keysFromSeries(defaultSeries), "\n"), `resource_name="pg-a"`)

	var workloadScope metrix.HostScope
	for _, resource := range c.discovery.Resources {
		if resource.Name == "pg-a" {
			workloadScope = resource.HostScope
		}
	}
	require.False(t, workloadScope.IsDefault())

	scopedSeries := scalarSeriesFromReader(c.store.Read(metrix.ReadRaw(), metrix.ReadHostScope(workloadScope.ScopeKey)))
	assert.Contains(t, strings.Join(keysFromSeries(scopedSeries), "\n"), `resource_name="pg-a"`)
	assert.NotContains(t, strings.Join(keysFromSeries(scopedSeries), "\n"), `resource_name="pg-b"`)
}

func TestCollector_RefreshDiscovery_PushesModeFiltersIntoQuery(t *testing.T) {
	tests := map[string]struct {
		resources  []map[string]any
		filters    *ResourceFiltersConfig
		wantQuery  string
		wantCounts int
	}{
		"resource groups only": {
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
			filters:    &ResourceFiltersConfig{ResourceGroups: []string{"RG-B", " rg-a ", "rg-b"}},
			wantCounts: 2,
			wantQuery:  "resources | where type in~ ('Microsoft.DBforPostgreSQL/flexibleServers') | where resourceGroup in~ ('rg-a', 'rg-b') | project id, name, type, resourceGroup, location, tags",
		},
		"resource groups regions and tags": {
			resources: []map[string]any{
				{
					"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
					"name":          "pg-a",
					"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
					"resourceGroup": "rg-a",
					"location":      "eastus",
				},
			},
			filters: &ResourceFiltersConfig{
				ResourceGroups: []string{"RG-B", " rg-a ", "rg-b"},
				Regions:        []string{" WestEurope ", "eastus", "EASTUS"},
				Tags: map[string][]string{
					"ROLE":  {"worker", "api", "worker"},
					" env ": {"prod"},
				},
			},
			wantQuery: "resources | where type in~ ('Microsoft.DBforPostgreSQL/flexibleServers') | where resourceGroup in~ ('rg-a', 'rg-b') | where location in~ ('eastus', 'westeurope') | extend tagsBag = tags | mv-expand bagexpansion=array tags | where isnotempty(tags) | extend tagKey = tostring(tags[0]), tagValue = tostring(tags[1]) | where (tagKey =~ 'env' and tagValue == 'prod') or (tagKey =~ 'role' and tagValue in ('api', 'worker')) | summarize tags = take_any(tagsBag), matchedTagKeys = dcount(tolower(tagKey)) by id, name, type, resourceGroup, location | where matchedTagKeys == 2 | project id, name, type, resourceGroup, location, tags",
		},
		"deduplicates ids case insensitively": {
			resources: []map[string]any{
				{
					"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
					"name":          "pg-a",
					"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
					"resourceGroup": "rg-a",
					"location":      "eastus",
				},
				{
					"id":            "/SUBSCRIPTIONS/sub-1/resourcegroups/rg-a/providers/microsoft.dbforpostgresql/flexibleservers/pg-a",
					"name":          "pg-a-duplicate",
					"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
					"resourceGroup": "rg-a",
					"location":      "eastus",
				},
			},
			wantCounts: 1,
			wantQuery:  "resources | where type in~ ('Microsoft.DBforPostgreSQL/flexibleServers') | project id, name, type, resourceGroup, location, tags",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rg := &mockResourceGraph{resources: tc.resources}
			c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
			c.Config = testConfig()
			c.Config.Discovery.ModeFilters = tc.filters

			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))

			if tc.wantCounts > 0 {
				require.Len(t, c.discovery.Resources, tc.wantCounts)
			}
			assert.Equal(t, tc.wantQuery, rg.lastQuery())
		})
	}
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
	cfg.Profiles.ModeExact = testProfilesModeEntries("storage_slow")

	catalog := mustLoadStockCatalog(t, map[string]string{
		"storage_slow.yaml": `
display_name: Azure Storage Slow
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

func TestCollector_QueryOffsetUsesEffectivePerBatchOffset(t *testing.T) {
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
		queryResponse: azmetrics.QueryResourcesResponse{MetricResults: azmetrics.MetricResults{}},
	}

	cfg := testConfig()
	cfg.Profiles.ModeExact = testProfilesModeEntries("storage_mixed")

	catalog := mustLoadStockCatalog(t, map[string]string{
		"storage_mixed.yaml": `
display_name: Azure Storage Mixed Grains
resource_type: Microsoft.Storage/storageAccounts
metrics:
  - id: transactions
    azure_name: Transactions
    time_grain: PT1M
    series:
      - aggregation: average
        kind: gauge
  - id: used_capacity
    azure_name: UsedCapacity
    time_grain: PT5M
    series:
      - aggregation: average
        kind: gauge
template:
  family: Azure Storage Mixed
  context_namespace: storage_mixed
  charts:
    - id: am_storage_mixed_transactions
      title: Azure Storage Mixed Transactions
      context: transactions
      family: Throughput
      type: line
      units: ops/s
      algorithm: absolute
      dimensions:
        - selector: ` + azureprofiles.ExportedSeriesName("storage_mixed", "transactions", "average") + `
          name: average
    - id: am_storage_mixed_used_capacity
      title: Azure Storage Mixed Used Capacity
      context: used_capacity
      family: Capacity
      type: line
      units: bytes
      algorithm: absolute
      dimensions:
        - selector: ` + azureprofiles.ExportedSeriesName("storage_mixed", "used_capacity", "average") + `
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

	calls := mx.queryCalls()
	require.Len(t, calls, 2)

	byInterval := make(map[string]metricsQueryCall, len(calls))
	for _, call := range calls {
		byInterval[call.Interval] = call
	}

	require.Contains(t, byInterval, "PT1M")
	require.Contains(t, byInterval, "PT5M")

	assert.Equal(t, "2026-03-07T11:56:00Z", byInterval["PT1M"].StartTime)
	assert.Equal(t, "2026-03-07T11:57:00Z", byInterval["PT1M"].EndTime)
	assert.Equal(t, []string{"Transactions"}, byInterval["PT1M"].MetricNames)

	assert.Equal(t, "2026-03-07T11:50:00Z", byInterval["PT5M"].StartTime)
	assert.Equal(t, "2026-03-07T11:55:00Z", byInterval["PT5M"].EndTime)
	assert.Equal(t, []string{"UsedCapacity"}, byInterval["PT5M"].MetricNames)
}

func TestSamplesFromQueryResponse_UsesResourceHostScope(t *testing.T) {
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	scope, unsafeValue := workloadHostScope(resourceInfo{Tags: []resourceTag{{Key: "workload", Value: "api"}}}, "workload")
	require.Empty(t, unsafeValue)

	tests := map[string]struct {
		scope metrix.HostScope
	}{
		"default scope": {},
		"workload scope": {
			scope: scope,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resourceID := "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a"
			samples := samplesFromQueryResponse(
				[]azmetrics.MetricData{{
					ResourceID: ptrString(strings.ToLower(resourceID)),
					Values: []azmetrics.Metric{
						metricWithAvg("cpu_percent", now, 10),
					},
				}},
				"postgres_flexible",
				map[string]*metricRuntime{
					"cpu_percent": {
						Series: []*seriesRuntime{{
							Aggregation: "average",
							Kind:        azureprofiles.SeriesKindGauge,
							Instrument:  "postgres_flexible.cpu_percent.average",
						}},
					},
				},
				map[string]resourceInfo{
					strings.ToLower(resourceID): {
						SubscriptionID: "sub-1",
						ID:             resourceID,
						UID:            "uid-a",
						Name:           "pg-a",
						Type:           "Microsoft.DBforPostgreSQL/flexibleServers",
						ResourceGroup:  "rg-a",
						Region:         "eastus",
						HostScope:      tc.scope,
					},
				},
			)

			require.Len(t, samples, 1)
			assert.Equal(t, tc.scope, samples[0].Scope)
			if tc.scope.IsDefault() {
				assert.True(t, samples[0].Scope.IsDefault())
			}
		})
	}
}

func TestCollector_CheckBootstrapProfileScenarios(t *testing.T) {
	tests := map[string]struct {
		resources       []map[string]any
		prepare         func(*Collector)
		wantErrContains string
		check           func(*testing.T, *Collector, *mockResourceGraph)
	}{
		"auto discover": {
			resources: []map[string]any{
				{
					"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
					"name":          "pg-a",
					"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
					"resourceGroup": "rg-a",
					"location":      "eastus",
				},
			},
			prepare: func(c *Collector) {
				c.Config = testConfig()
				c.Config.Profiles.Mode = profilesModeAuto
				c.Config.Profiles.ModeExact = nil
				c.Config.Profiles.ModeCombined = nil
			},
			check: func(t *testing.T, c *Collector, rg *mockResourceGraph) {
				require.NotEmpty(t, c.runtime.Profiles)
				assert.Equal(t, "postgres_flexible", c.runtime.Profiles[0].Name)
			},
		},
		"combined with explicit profiles": {
			resources: []map[string]any{
				{
					"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
					"name":          "pg-a",
					"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
					"resourceGroup": "rg-a",
					"location":      "eastus",
				},
			},
			prepare: func(c *Collector) {
				c.Config = testConfig()
				c.Config.Profiles.Mode = profilesModeCombined
				c.Config.Profiles.ModeExact = nil
				c.Config.Profiles.ModeCombined = testProfilesModeEntries("cosmos_db")
			},
			check: func(t *testing.T, c *Collector, rg *mockResourceGraph) {
				var ids []string
				for _, p := range c.runtime.Profiles {
					ids = append(ids, p.Name)
				}
				assert.Contains(t, ids, "cosmos_db")
				assert.Contains(t, ids, "postgres_flexible")
			},
		},
		"combined mode allows no auto matches": {
			prepare: func(c *Collector) {
				c.Config = testConfig()
				c.Config.Profiles.Mode = profilesModeCombined
				c.Config.Profiles.ModeExact = nil
				c.Config.Profiles.ModeCombined = testProfilesModeEntries("cosmos_db")
			},
			check: func(t *testing.T, c *Collector, rg *mockResourceGraph) {
				require.Len(t, c.runtime.Profiles, 1)
				assert.Equal(t, "cosmos_db", c.runtime.Profiles[0].Name)
			},
		},
		"default mode resolves to auto and fails with no matches": {
			prepare: func(c *Collector) {
				c.Config = testConfig()
				c.Config.Profiles.Mode = ""
				c.Config.Profiles.ModeExact = nil
				c.Config.Profiles.ModeCombined = nil
			},
			wantErrContains: "auto-discovery found no Azure resources",
		},
		"auto discover fails when no resources match": {
			prepare: func(c *Collector) {
				c.Config = testConfig()
				c.Config.Profiles.Mode = profilesModeAuto
				c.Config.Profiles.ModeExact = nil
				c.Config.Profiles.ModeCombined = nil
			},
			wantErrContains: "auto-discovery found no Azure resources",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rg := &mockResourceGraph{resources: tc.resources}
			c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
			tc.prepare(c)

			require.NoError(t, c.Init(context.Background()))
			err := c.Check(context.Background())
			if tc.wantErrContains != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tc.wantErrContains)
				return
			}

			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, c, rg)
			}
		})
	}
}

func TestCollector_CheckBootstrapQueryModeScenarios(t *testing.T) {
	const kql = "resources | project id, name, type, resourceGroup, location"

	tests := map[string]struct {
		resources []map[string]any
		prepare   func(*Collector)
		check     func(*testing.T, *Collector, *mockResourceGraph)
	}{
		"auto discover from custom query": {
			resources: []map[string]any{
				{
					"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
					"name":          "pg-a",
					"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
					"resourceGroup": "rg-a",
					"location":      "eastus",
				},
			},
			prepare: func(c *Collector) {
				c.Config = testConfig()
				c.Config.Profiles.Mode = profilesModeAuto
				c.Config.Profiles.ModeExact = nil
				c.Config.Profiles.ModeCombined = nil
				c.Config.Discovery.Mode = discoveryModeQuery
				c.Config.Discovery.ModeFilters = nil
				c.Config.Discovery.ModeQuery = &DiscoveryQueryConfig{KQL: kql}
			},
			check: func(t *testing.T, c *Collector, rg *mockResourceGraph) {
				require.NotEmpty(t, c.runtime.Profiles)
				assert.Equal(t, "postgres_flexible", c.runtime.Profiles[0].Name)
				assert.Equal(t, kql, rg.lastQuery())
			},
		},
		"normalizes empty location to global": {
			resources: []map[string]any{
				{
					"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
					"name":          "pg-a",
					"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
					"resourceGroup": "rg-a",
					"location":      "",
				},
			},
			prepare: func(c *Collector) {
				c.Config = testConfig()
				c.Config.Discovery.Mode = discoveryModeQuery
				c.Config.Discovery.ModeFilters = nil
				c.Config.Discovery.ModeQuery = &DiscoveryQueryConfig{KQL: kql}
			},
			check: func(t *testing.T, c *Collector, rg *mockResourceGraph) {
				require.Len(t, c.discovery.Resources, 1)
				assert.Equal(t, "global", c.discovery.Resources[0].Region)
			},
		},
		"exact mode ignores unsupported discovered types": {
			resources: []map[string]any{
				{
					"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.FakeService/fakeResources/fake-a",
					"name":          "fake-a",
					"type":          "Microsoft.FakeService/fakeResources",
					"resourceGroup": "rg-a",
					"location":      "eastus",
				},
			},
			prepare: func(c *Collector) {
				c.Config = testConfig()
				c.Config.Discovery.Mode = discoveryModeQuery
				c.Config.Discovery.ModeFilters = nil
				c.Config.Discovery.ModeQuery = &DiscoveryQueryConfig{KQL: kql}
			},
			check: func(t *testing.T, c *Collector, rg *mockResourceGraph) {
				require.Len(t, c.runtime.Profiles, 1)
				assert.Equal(t, "postgres_flexible", c.runtime.Profiles[0].Name)
				assert.Empty(t, c.discovery.Resources)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rg := &mockResourceGraph{resources: tc.resources}
			c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
			tc.prepare(c)

			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			tc.check(t, c, rg)
		})
	}
}

func TestCollector_WorkloadMode_QueryModeTagFallbackState(t *testing.T) {
	const kql = "resources | project id, name, type, resourceGroup, location"

	tests := map[string]struct {
		tags                      any
		tagsPresent               bool
		wantMissingWarning        bool
		wantWrongShapeWarning     bool
		wantDefaultScope          bool
		wantWorkloadScopeHostname string
	}{
		"tags column missing falls back and records warning": {
			wantMissingWarning: true,
			wantDefaultScope:   true,
		},
		"tags wrong shape falls back and records warning": {
			tagsPresent:           true,
			tags:                  "not-a-map",
			wantWrongShapeWarning: true,
			wantDefaultScope:      true,
		},
		"configured tag missing in tags map falls back silently": {
			tagsPresent:      true,
			tags:             map[string]any{"env": "prod"},
			wantDefaultScope: true,
		},
		"tags map derives workload scope": {
			tagsPresent:               true,
			tags:                      map[string]any{"Workload": "api"},
			wantWorkloadScopeHostname: "api",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			row := map[string]any{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
				"name":          "pg-a",
				"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
				"resourceGroup": "rg-a",
				"location":      "eastus",
			}
			if tc.tagsPresent {
				row["tags"] = tc.tags
			}

			rg := &mockResourceGraph{resources: []map[string]any{row}}
			c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
			c.Config = testConfig()
			c.Config.VirtualNodes = &VirtualNodesConfig{ByResourceTag: "workload"}
			c.Config.Profiles.Mode = profilesModeAuto
			c.Config.Profiles.ModeExact = nil
			c.Config.Profiles.ModeCombined = nil
			c.Config.Discovery.Mode = discoveryModeQuery
			c.Config.Discovery.ModeFilters = nil
			c.Config.Discovery.ModeQuery = &DiscoveryQueryConfig{KQL: kql}

			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))

			require.Len(t, c.discovery.Resources, 1)
			assert.Equal(t, tc.wantMissingWarning, c.tagsColumnMissingWarned)
			assert.Equal(t, tc.wantWrongShapeWarning, c.tagsWrongShapeWarned)
			if tc.wantDefaultScope {
				assert.True(t, c.discovery.Resources[0].HostScope.IsDefault())
				return
			}
			require.False(t, c.discovery.Resources[0].HostScope.IsDefault())
			assert.Equal(t, tc.wantWorkloadScopeHostname, c.discovery.Resources[0].HostScope.Hostname)
		})
	}
}

func TestCollector_WorkloadMode_QueryModeWarningWatermarks(t *testing.T) {
	c := New()
	runtime := &collectorRuntime{WorkloadResourceTagKey: "workload"}

	state := discoveryState{
		FetchCounter:           1,
		QueryTagsColumnMissing: true,
		QueryTagsWrongShape:    true,
		UnsafeWorkloadValues: map[string]int{
			"bad\nvalue": 2,
		},
	}

	c.warnDiscoveryScopeFallbacks(state, runtime)
	c.warnDiscoveryScopeFallbacks(state, runtime)
	assert.True(t, c.tagsColumnMissingWarned)
	assert.True(t, c.tagsWrongShapeWarned)
	assert.Equal(t, uint64(1), c.tagsColumnMissingWarnedAt)
	assert.Equal(t, uint64(1), c.tagsWrongShapeWarnedAt)

	state.FetchCounter = 2
	c.warnDiscoveryScopeFallbacks(state, runtime)
	assert.Equal(t, uint64(2), c.tagsColumnMissingWarnedAt)
	assert.Equal(t, uint64(2), c.tagsWrongShapeWarnedAt)
}

func TestCollector_WorkloadMode_QueryModeWarningDedupAcrossRefreshes(t *testing.T) {
	const kql = "resources | project id, name, type, resourceGroup, location"

	tests := map[string]struct {
		tagsPresent   bool
		tags          any
		wantMissingAt func(*Collector) uint64
		wantWrongAt   func(*Collector) uint64
	}{
		"missing tags column": {
			wantMissingAt: func(c *Collector) uint64 { return c.tagsColumnMissingWarnedAt },
		},
		"wrong shaped tags column": {
			tagsPresent: true,
			tags:        "not-a-map",
			wantWrongAt: func(c *Collector) uint64 { return c.tagsWrongShapeWarnedAt },
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			row := map[string]any{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
				"name":          "pg-a",
				"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
				"resourceGroup": "rg-a",
				"location":      "eastus",
			}
			if tc.tagsPresent {
				row["tags"] = tc.tags
			}

			rg := &mockResourceGraph{resources: []map[string]any{row}}
			c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
			c.Config = testConfig()
			c.Config.VirtualNodes = &VirtualNodesConfig{ByResourceTag: "workload"}
			c.Config.Profiles.Mode = profilesModeAuto
			c.Config.Profiles.ModeExact = nil
			c.Config.Profiles.ModeCombined = nil
			c.Config.Discovery.Mode = discoveryModeQuery
			c.Config.Discovery.ModeFilters = nil
			c.Config.Discovery.ModeQuery = &DiscoveryQueryConfig{KQL: kql}

			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))
			require.Equal(t, uint64(1), c.discovery.FetchCounter)
			if tc.wantMissingAt != nil {
				assert.Equal(t, uint64(1), tc.wantMissingAt(c))
			}
			if tc.wantWrongAt != nil {
				assert.Equal(t, uint64(1), tc.wantWrongAt(c))
			}

			c.warnDiscoveryScopeFallbacks(c.discovery, c.runtime)
			if tc.wantMissingAt != nil {
				assert.Equal(t, uint64(1), tc.wantMissingAt(c))
			}
			if tc.wantWrongAt != nil {
				assert.Equal(t, uint64(1), tc.wantWrongAt(c))
			}

			_, err := c.refreshDiscovery(context.Background(), true)
			require.NoError(t, err)
			require.Equal(t, uint64(2), c.discovery.FetchCounter)
			if tc.wantMissingAt != nil {
				assert.Equal(t, uint64(2), tc.wantMissingAt(c))
			}
			if tc.wantWrongAt != nil {
				assert.Equal(t, uint64(2), tc.wantWrongAt(c))
			}
		})
	}
}

func TestCollector_WorkloadMode_UnsafeValueReportResetsPerRefresh(t *testing.T) {
	runtime := &collectorRuntime{
		WorkloadResourceTagKey: "workload",
		Profiles: []*profileRuntime{{
			Name:         "postgres_flexible",
			ResourceType: "Microsoft.DBforPostgreSQL/flexibleServers",
		}},
	}
	base := resourceInfo{
		SubscriptionID: "sub-1",
		ID:             "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
		UID:            "uid-a",
		Name:           "pg-a",
		Type:           "Microsoft.DBforPostgreSQL/flexibleServers",
		ResourceGroup:  "rg-a",
		Region:         "eastus",
	}
	tests := map[string]struct {
		refreshes []struct {
			tags       []resourceTag
			wantUnsafe map[string]int
		}
	}{
		"unsafe report is scoped to the current refresh": {
			refreshes: []struct {
				tags       []resourceTag
				wantUnsafe map[string]int
			}{
				{
					tags:       []resourceTag{{Key: "workload", Value: "bad\none"}},
					wantUnsafe: map[string]int{"bad\none": 1},
				},
				{
					tags:       []resourceTag{{Key: "workload", Value: "bad\ntwo"}},
					wantUnsafe: map[string]int{"bad\ntwo": 1},
				},
				{
					tags: []resourceTag{{Key: "workload", Value: "api"}},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			for i, refresh := range tc.refreshes {
				resource := base
				resource.Tags = refresh.tags
				fetchCounter := uint64(i + 1)

				state := buildDiscoveryState([]resourceInfo{resource}, runtime, time.Unix(int64(fetchCounter), 0), 300, fetchCounter, discoveryFetchResult{})

				assert.Equal(t, refresh.wantUnsafe, state.UnsafeWorkloadValues)
			}
		})
	}
}

func TestCollector_WorkloadMode_DisabledLeavesQueryAndScopesDefault(t *testing.T) {
	const kql = "resources | project id, name, type, resourceGroup, location"

	tests := map[string]struct {
		virtualNodes *VirtualNodesConfig
	}{
		"unset": {},
		"whitespace": {
			virtualNodes: &VirtualNodesConfig{ByResourceTag: " \t "},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rg := &mockResourceGraph{resources: []map[string]any{{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
				"name":          "pg-a",
				"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
				"resourceGroup": "rg-a",
				"location":      "eastus",
			}}}
			c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
			c.Config = testConfig()
			c.Config.VirtualNodes = tc.virtualNodes
			c.Config.Profiles.Mode = profilesModeAuto
			c.Config.Profiles.ModeExact = nil
			c.Config.Profiles.ModeCombined = nil
			c.Config.Discovery.Mode = discoveryModeQuery
			c.Config.Discovery.ModeFilters = nil
			c.Config.Discovery.ModeQuery = &DiscoveryQueryConfig{KQL: kql}

			require.NoError(t, c.Init(context.Background()))
			require.NoError(t, c.Check(context.Background()))

			assert.Equal(t, kql, rg.lastQuery())
			assert.Empty(t, c.runtime.WorkloadResourceTagKey)
			require.Len(t, c.discovery.Resources, 1)
			assert.True(t, c.discovery.Resources[0].HostScope.IsDefault())
			assert.False(t, c.tagsColumnMissingWarned)
			assert.False(t, c.tagsWrongShapeWarned)
		})
	}
}

func TestCollector_InitQueryModeRejectsMalformedRows(t *testing.T) {
	const kql = "resources | project id, name, type, resourceGroup, location"

	tests := map[string]struct {
		rows           []map[string]any
		wantErrContain string
	}{
		"missing required column": {
			rows: []map[string]any{
				{
					"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
					"name":          "pg-a",
					"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
					"resourceGroup": "rg-a",
				},
			},
			wantErrContain: `missing required column "location"`,
		},
		"duplicate id": {
			rows: []map[string]any{
				{
					"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
					"name":          "pg-a",
					"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
					"resourceGroup": "rg-a",
					"location":      "eastus",
				},
				{
					"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
					"name":          "pg-a-dup",
					"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
					"resourceGroup": "rg-a",
					"location":      "eastus",
				},
			},
			wantErrContain: `duplicate id "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a"`,
		},
		"invalid arm id": {
			rows: []map[string]any{
				{
					"id":            "not-an-arm-id",
					"name":          "pg-a",
					"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
					"resourceGroup": "rg-a",
					"location":      "eastus",
				},
			},
			wantErrContain: `invalid ARM resource id "not-an-arm-id"`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rg := &mockResourceGraph{resources: tc.rows}

			c := New()
			c.Config = testConfig()
			c.Config.Profiles.Mode = profilesModeAuto
			c.Config.Profiles.ModeExact = nil
			c.Config.Profiles.ModeCombined = nil
			c.Config.Discovery.Mode = discoveryModeQuery
			c.Config.Discovery.ModeFilters = nil
			c.Config.Discovery.ModeQuery = &DiscoveryQueryConfig{KQL: kql}
			c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
				return rg, nil
			}
			c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
				return &mockMetricsClient{}, nil
			}

			require.NoError(t, c.Init(context.Background()))
			err := c.Check(context.Background())
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantErrContain)
		})
	}
}

func TestCollector_ProfileFilters_NarrowMatchedResourcesInFiltersMode(t *testing.T) {
	rg := &mockResourceGraph{
		resources: []map[string]any{
			{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.Sql/servers/sql-a/databases/db-a",
				"name":          "db-a",
				"type":          "Microsoft.Sql/servers/databases",
				"resourceGroup": "rg-a",
				"location":      "eastus",
				"tags": map[string]any{
					"env": "prod",
				},
			},
			{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.Sql/servers/sql-a/databases/db-b",
				"name":          "db-b",
				"type":          "Microsoft.Sql/servers/databases",
				"resourceGroup": "rg-a",
				"location":      "eastus",
				"tags": map[string]any{
					"env": "dev",
				},
			},
		},
	}

	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.Config.Profiles.ModeExact = &ProfilesModeConfig{
		Entries: []ProfileEntryConfig{{
			Name: "sql_database",
			Filters: &ResourceFiltersConfig{
				Tags: map[string][]string{
					"env": {"prod"},
				},
			},
		}},
	}

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))

	require.Len(t, c.discovery.Resources, 2)
	require.Len(t, c.discovery.ByProfile["sql_database"], 1)
	assert.Equal(t, "db-a", c.discovery.ByProfile["sql_database"][0].Name)
}

func TestCollector_ProfileFilters_NarrowMatchedResourcesInQueryMode(t *testing.T) {
	const kql = "resources | project id, name, type, resourceGroup, location"

	rg := &mockResourceGraph{
		resources: []map[string]any{
			{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.Sql/servers/sql-a/databases/db-a",
				"name":          "db-a",
				"type":          "Microsoft.Sql/servers/databases",
				"resourceGroup": "rg-a",
				"location":      "eastus",
			},
			{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.Sql/servers/sql-a/databases/db-b",
				"name":          "db-b",
				"type":          "Microsoft.Sql/servers/databases",
				"resourceGroup": "rg-a",
				"location":      "westeurope",
			},
		},
	}

	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.Config.Discovery.Mode = discoveryModeQuery
	c.Config.Discovery.ModeFilters = nil
	c.Config.Discovery.ModeQuery = &DiscoveryQueryConfig{KQL: kql}
	c.Config.Profiles.ModeExact = &ProfilesModeConfig{
		Entries: []ProfileEntryConfig{{
			Name: "sql_database",
			Filters: &ResourceFiltersConfig{
				Regions: []string{"eastus"},
			},
		}},
	}

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))

	require.Len(t, c.discovery.Resources, 2)
	require.Len(t, c.discovery.ByProfile["sql_database"], 1)
	assert.Equal(t, "db-a", c.discovery.ByProfile["sql_database"][0].Name)
}

func TestCollector_CombinedExplicitEntryOverlaysAutoProfile(t *testing.T) {
	rg := &mockResourceGraph{
		resources: []map[string]any{
			{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
				"name":          "pg-a",
				"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
				"resourceGroup": "rg-a",
				"location":      "eastus",
			},
		},
	}

	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.Config.Profiles.Mode = profilesModeCombined
	c.Config.Profiles.ModeExact = nil
	c.Config.Profiles.ModeCombined = &ProfilesModeConfig{
		Entries: []ProfileEntryConfig{{
			Name: "postgres_flexible",
			Filters: &ResourceFiltersConfig{
				Regions: []string{"westeurope"},
			},
		}},
	}

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))

	require.Len(t, c.runtime.Profiles, 1)
	assert.Equal(t, "postgres_flexible", c.runtime.Profiles[0].Name)
	require.NotNil(t, c.runtime.Profiles[0].Filters)
	assert.Equal(t, []string{"westeurope"}, c.runtime.Profiles[0].Filters.Regions)
	require.Len(t, c.discovery.Resources, 1)
	assert.NotContains(t, c.discovery.ByProfile, "postgres_flexible")
}

func TestCollector_AutoModeEntriesStayDormantAcrossRefresh(t *testing.T) {
	rg := &mockResourceGraph{
		resources: []map[string]any{
			{
				"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-a",
				"name":          "pg-a",
				"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
				"resourceGroup": "rg-a",
				"location":      "eastus",
			},
		},
	}

	c := newTestCollectorWithMocks(rg, &mockMetricsClient{})
	c.Config = testConfig()
	c.Config.Profiles.Mode = profilesModeAuto
	c.Config.Profiles.ModeExact = nil
	c.Config.Profiles.ModeCombined = nil
	c.Config.Profiles.ModeAuto = &ProfilesModeConfig{
		Entries: []ProfileEntryConfig{{
			Name: "sql_database",
			Filters: &ResourceFiltersConfig{
				ResourceGroups: []string{"prod-rg"},
			},
		}},
	}

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))

	require.Len(t, c.runtime.Profiles, 1)
	assert.Equal(t, "postgres_flexible", c.runtime.Profiles[0].Name)
	assert.NotContains(t, c.discovery.ByProfile, "sql_database")

	rg.setResources([]map[string]any{
		{
			"id":            "/subscriptions/sub-1/resourceGroups/prod-rg/providers/Microsoft.Sql/servers/sql-a/databases/db-a",
			"name":          "db-a",
			"type":          "Microsoft.Sql/servers/databases",
			"resourceGroup": "prod-rg",
			"location":      "eastus",
		},
	})

	_, err := c.refreshDiscovery(context.Background(), true)
	require.NoError(t, err)

	require.Len(t, c.runtime.Profiles, 1)
	assert.Equal(t, "postgres_flexible", c.runtime.Profiles[0].Name)
	assert.Empty(t, c.discovery.Resources)
	assert.NotContains(t, c.discovery.ByProfile, "sql_database")
	assert.Contains(t, rg.lastQuery(), "Microsoft.DBforPostgreSQL/flexibleServers")
	assert.NotContains(t, rg.lastQuery(), "Microsoft.Sql/servers/databases")
}

func TestObservationState_PruneStaleResources_RemovesOldProfileMembership(t *testing.T) {
	resource := resourceInfo{
		SubscriptionID: "sub-1",
		UID:            "uid-a",
		Name:           "db-a",
		ResourceGroup:  "rg-a",
		Region:         "eastus",
		Type:           "Microsoft.Sql/servers/databases",
	}
	labels := labelValues(resourceLabels(resource, "sql_database"))
	key := sampleObservationKey("sql.cpu", metrix.HostScope{}, labels)

	state := &observationState{
		accumulators: map[observationKey]float64{
			key: 10,
		},
		lastObserved: map[observationKey]lastObservation{
			key: {
				instrument:  "sql.cpu",
				labelValues: append([]string(nil), labels...),
				value:       10,
			},
		},
	}

	state.pruneStaleResources(map[string][]resourceInfo{
		"postgres_flexible": {resource},
	})

	assert.Empty(t, state.lastObserved)
	assert.Empty(t, state.accumulators)
}

func TestObservationState_PruneStaleResources_RemovesLabelChurnForSameResource(t *testing.T) {
	oldResource := resourceInfo{
		SubscriptionID: "sub-1",
		UID:            "uid-a",
		Name:           "db-a",
		ResourceGroup:  "rg-a",
		Region:         "eastus",
		Type:           "Microsoft.Sql/servers/databases",
	}
	newResource := oldResource
	newResource.Name = "db-a-renamed"

	labels := labelValues(resourceLabels(oldResource, "sql_database"))
	key := sampleObservationKey("sql.cpu", metrix.HostScope{}, labels)

	state := &observationState{
		accumulators: map[observationKey]float64{
			key: 10,
		},
		lastObserved: map[observationKey]lastObservation{
			key: {
				instrument:  "sql.cpu",
				labelValues: append([]string(nil), labels...),
				value:       10,
			},
		},
	}

	state.pruneStaleResources(map[string][]resourceInfo{
		"sql_database": {newResource},
	})

	assert.Empty(t, state.lastObserved)
	assert.Empty(t, state.accumulators)
}

func TestObservationState_PruneStaleResources_RespectsScopeBoundary(t *testing.T) {
	scopeA, unsafeValue := workloadHostScope(resourceInfo{Tags: []resourceTag{{Key: "workload", Value: "api"}}}, "workload")
	require.Empty(t, unsafeValue)
	scopeB, unsafeValue := workloadHostScope(resourceInfo{Tags: []resourceTag{{Key: "workload", Value: "worker"}}}, "workload")
	require.Empty(t, unsafeValue)

	resource := resourceInfo{
		SubscriptionID: "sub-1",
		UID:            "uid-a",
		Name:           "db-a",
		ResourceGroup:  "rg-a",
		Region:         "eastus",
		Type:           "Microsoft.Sql/servers/databases",
		HostScope:      scopeA,
	}
	labels := labelValues(resourceLabels(resource, "sql_database"))
	keyA := sampleObservationKey("sql.cpu", scopeA, labels)
	keyB := sampleObservationKey("sql.cpu", scopeB, labels)

	state := &observationState{
		accumulators: map[observationKey]float64{
			keyA: 10,
			keyB: 20,
		},
		lastObserved: map[observationKey]lastObservation{
			keyA: {
				instrument:  "sql.cpu",
				scope:       scopeA,
				labelValues: append([]string(nil), labels...),
				value:       10,
			},
			keyB: {
				instrument:  "sql.cpu",
				scope:       scopeB,
				labelValues: append([]string(nil), labels...),
				value:       20,
			},
		},
	}

	state.pruneStaleResources(map[string][]resourceInfo{
		"sql_database": {resource},
	})

	assert.Contains(t, state.lastObserved, keyA)
	assert.NotContains(t, state.lastObserved, keyB)
	assert.Contains(t, state.accumulators, keyA)
	assert.NotContains(t, state.accumulators, keyB)
}

func TestObservationState_CounterAccumulatorFollowsScopeMobility(t *testing.T) {
	scopes := map[string]metrix.HostScope{}
	for _, workload := range []string{"api", "worker", "batch"} {
		scope, unsafeValue := workloadHostScope(resourceInfo{Tags: []resourceTag{{Key: "workload", Value: workload}}}, "workload")
		require.Empty(t, unsafeValue)
		scopes[workload] = scope
	}

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	cycle := managed.CycleController()
	vec := store.Write().SnapshotMeter("").Vec("resource_uid", "subscription_id", "resource_name", "resource_group", "region", "resource_type", "profile")
	state := &observationState{
		instruments: map[string]*instrumentRuntime{
			"sql.transactions": {
				Kind:    azureprofiles.SeriesKindCounter,
				Counter: vec.Counter("sql.transactions"),
			},
		},
		accumulators: make(map[observationKey]float64),
		lastObserved: make(map[observationKey]lastObservation),
	}

	baseResource := resourceInfo{
		SubscriptionID: "sub-1",
		UID:            "uid-a",
		Name:           "db-a",
		ResourceGroup:  "rg-a",
		Region:         "eastus",
		Type:           "Microsoft.Sql/servers/databases",
	}
	steps := map[string]struct {
		path []struct {
			workload string
			value    float64
		}
	}{
		"a to b to c to a": {
			path: []struct {
				workload string
				value    float64
			}{
				{workload: "api", value: 5},
				{workload: "worker", value: 7},
				{workload: "batch", value: 11},
				{workload: "api", value: 13},
			},
		},
	}

	for name, tc := range steps {
		t.Run(name, func(t *testing.T) {
			for _, step := range tc.path {
				scope := scopes[step.workload]
				resource := baseResource
				resource.HostScope = scope
				labels := labelValues(resourceLabels(resource, "sql_database"))

				state.pruneStaleResources(map[string][]resourceInfo{"sql_database": {resource}})
				cycle.BeginCycle()
				observed := state.observeSamples([]metricSample{{
					Instrument: "sql.transactions",
					Kind:       azureprofiles.SeriesKindCounter,
					Scope:      scope,
					Labels:     resourceLabels(resource, "sql_database"),
					Value:      step.value,
				}})
				state.reobserveCachedObservations(map[string]bool{}, observed)
				require.NoError(t, cycle.CommitCycleSuccess())

				key := sampleObservationKey("sql.transactions", scope, labels)
				require.Len(t, state.accumulators, 1)
				assert.Equal(t, step.value, state.accumulators[key])
				assert.Contains(t, state.lastObserved, key)
			}
		})
	}
}

func TestObservationState_ReobserveUsesStoredScope(t *testing.T) {
	scope, unsafeValue := workloadHostScope(resourceInfo{Tags: []resourceTag{{Key: "workload", Value: "api"}}}, "workload")
	require.Empty(t, unsafeValue)

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	cycle := managed.CycleController()
	vec := store.Write().SnapshotMeter("").Vec("resource_uid", "subscription_id", "resource_name", "resource_group", "region", "resource_type", "profile")
	labels := []string{"uid-a", "sub-1", "db-a", "rg-a", "eastus", "Microsoft.Sql/servers/databases", "sql_database"}

	state := &observationState{
		instruments: map[string]*instrumentRuntime{
			"sql.cpu": {
				Kind:  azureprofiles.SeriesKindGauge,
				Gauge: vec.Gauge("sql.cpu"),
			},
		},
		accumulators: make(map[observationKey]float64),
		lastObserved: map[observationKey]lastObservation{
			sampleObservationKey("sql.cpu", scope, labels): {
				instrument:  "sql.cpu",
				scope:       scope,
				labelValues: append([]string(nil), labels...),
				value:       42,
			},
		},
	}

	cycle.BeginCycle()
	state.reobserveCachedObservations(map[string]bool{}, map[observationKey]bool{})
	require.NoError(t, cycle.CommitCycleSuccess())

	_, ok = store.Read(metrix.ReadRaw()).Value("sql.cpu", metrix.Labels{
		"resource_uid":    "uid-a",
		"subscription_id": "sub-1",
		"resource_name":   "db-a",
		"resource_group":  "rg-a",
		"region":          "eastus",
		"resource_type":   "Microsoft.Sql/servers/databases",
		"profile":         "sql_database",
	})
	assert.False(t, ok)

	_, ok = store.Read(metrix.ReadRaw(), metrix.ReadHostScope(scope.ScopeKey)).Value("sql.cpu", metrix.Labels{
		"resource_uid":    "uid-a",
		"subscription_id": "sub-1",
		"resource_name":   "db-a",
		"resource_group":  "rg-a",
		"region":          "eastus",
		"resource_type":   "Microsoft.Sql/servers/databases",
		"profile":         "sql_database",
	})
	assert.True(t, ok)
}

func TestConfig_ValidateDiscoveryContracts(t *testing.T) {
	tests := map[string]struct {
		cfg            Config
		wantErr        bool
		wantErrContain string
	}{
		"filters mode ignores query block": {
			cfg: func() Config {
				cfg := testConfig()
				cfg.Discovery.Mode = discoveryModeFilters
				cfg.Discovery.ModeQuery = &DiscoveryQueryConfig{KQL: "resources | project id, name, type, resourceGroup, location"}
				return cfg
			}(),
			wantErr: false,
		},
		"query mode ignores filters block": {
			cfg: func() Config {
				cfg := testConfig()
				cfg.Discovery.Mode = discoveryModeQuery
				cfg.Discovery.ModeQuery = &DiscoveryQueryConfig{KQL: "resources | project id, name, type, resourceGroup, location"}
				cfg.Discovery.ModeFilters = &ResourceFiltersConfig{ResourceGroups: []string{"rg-a"}}
				return cfg
			}(),
			wantErr: false,
		},
		"query mode requires kql": {
			cfg: func() Config {
				cfg := testConfig()
				cfg.Discovery.Mode = discoveryModeQuery
				cfg.Discovery.ModeFilters = nil
				cfg.Discovery.ModeQuery = &DiscoveryQueryConfig{}
				return cfg
			}(),
			wantErr:        true,
			wantErrContain: "'discovery.mode_query.kql' must not be empty when discovery.mode is 'query'",
		},
		"filters mode rejects empty resource group item": {
			cfg: func() Config {
				cfg := testConfig()
				cfg.Discovery.Mode = discoveryModeFilters
				cfg.Discovery.ModeFilters = &ResourceFiltersConfig{ResourceGroups: []string{""}}
				return cfg
			}(),
			wantErr:        true,
			wantErrContain: "'discovery.mode_filters.resource_groups[0]' must not be empty",
		},
		"filters mode rejects empty region item": {
			cfg: func() Config {
				cfg := testConfig()
				cfg.Discovery.Mode = discoveryModeFilters
				cfg.Discovery.ModeFilters = &ResourceFiltersConfig{Regions: []string{"  "}}
				return cfg
			}(),
			wantErr:        true,
			wantErrContain: "'discovery.mode_filters.regions[0]' must not be empty",
		},
		"auto mode ignores explicit names": {
			cfg: func() Config {
				cfg := testConfig()
				cfg.Profiles.Mode = profilesModeAuto
				cfg.Profiles.ModeCombined = testProfilesModeEntries("cosmos_db")
				return cfg
			}(),
			wantErr: false,
		},
		"exact mode ignores combined block": {
			cfg: func() Config {
				cfg := testConfig()
				cfg.Profiles.Mode = profilesModeExact
				cfg.Profiles.ModeCombined = testProfilesModeEntries("cosmos_db")
				return cfg
			}(),
			wantErr: false,
		},
		"exact mode rejects duplicate entry names": {
			cfg: func() Config {
				cfg := testConfig()
				cfg.Profiles.Mode = profilesModeExact
				cfg.Profiles.ModeExact = &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "postgres_flexible"}, {Name: "postgres_flexible"}}}
				return cfg
			}(),
			wantErr:        true,
			wantErrContain: "'profiles.mode_exact.entries' contains duplicate entry name 'postgres_flexible'",
		},
		"exact mode rejects non-lowercase entry names": {
			cfg: func() Config {
				cfg := testConfig()
				cfg.Profiles.Mode = profilesModeExact
				cfg.Profiles.ModeExact = &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "POSTGRES_FLEXIBLE"}}}
				return cfg
			}(),
			wantErr:        true,
			wantErrContain: "'profiles.mode_exact.entries[0].name' must match",
		},
		"query mode ignores profile tag filters during validation": {
			cfg: func() Config {
				cfg := testConfig()
				cfg.Discovery.Mode = discoveryModeQuery
				cfg.Discovery.ModeFilters = nil
				cfg.Discovery.ModeQuery = &DiscoveryQueryConfig{KQL: "resources | project id, name, type, resourceGroup, location"}
				cfg.Profiles.ModeExact = &ProfilesModeConfig{
					Entries: []ProfileEntryConfig{{
						Name: "postgres_flexible",
						Filters: &ResourceFiltersConfig{
							Tags: map[string][]string{"env": {"prod"}},
						},
					}},
				}
				return cfg
			}(),
			wantErr: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.validate()
			if !tc.wantErr {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantErrContain)
		})
	}
}

func TestSanitizeIgnoredProfileTagFilters(t *testing.T) {
	cfg := testConfig()
	cfg.Discovery.Mode = discoveryModeQuery
	cfg.Discovery.ModeFilters = nil
	cfg.Discovery.ModeQuery = &DiscoveryQueryConfig{KQL: "resources | project id, name, type, resourceGroup, location"}
	cfg.Profiles.Mode = profilesModeExact
	cfg.Profiles.ModeAuto = nil
	cfg.Profiles.ModeCombined = nil
	cfg.Profiles.ModeExact = &ProfilesModeConfig{
		Entries: []ProfileEntryConfig{
			{
				Name: "postgres_flexible",
				Filters: &ResourceFiltersConfig{
					ResourceGroups: []string{"rg-a"},
					Tags:           map[string][]string{"env": {"prod"}},
				},
			},
			{
				Name: "sql_database",
				Filters: &ResourceFiltersConfig{
					Tags: map[string][]string{"tier": {"critical"}},
				},
			},
		},
	}

	sanitized, warnings := sanitizeIgnoredProfileTagFilters(cfg)

	assert.Equal(t, []string{
		"profiles.mode_exact.entries[0].filters.tags",
		"profiles.mode_exact.entries[1].filters.tags",
	}, warnings)
	require.NotNil(t, sanitized.Profiles.ModeExact)
	require.Len(t, sanitized.Profiles.ModeExact.Entries, 2)
	require.NotNil(t, sanitized.Profiles.ModeExact.Entries[0].Filters)
	assert.Equal(t, []string{"rg-a"}, sanitized.Profiles.ModeExact.Entries[0].Filters.ResourceGroups)
	assert.Nil(t, sanitized.Profiles.ModeExact.Entries[0].Filters.Tags)
	assert.Nil(t, sanitized.Profiles.ModeExact.Entries[1].Filters)

	require.NotNil(t, cfg.Profiles.ModeExact)
	require.Len(t, cfg.Profiles.ModeExact.Entries, 2)
	require.NotNil(t, cfg.Profiles.ModeExact.Entries[0].Filters)
	assert.Equal(t, map[string][]string{"env": {"prod"}}, cfg.Profiles.ModeExact.Entries[0].Filters.Tags)
	require.NotNil(t, cfg.Profiles.ModeExact.Entries[1].Filters)
	assert.Equal(t, map[string][]string{"tier": {"critical"}}, cfg.Profiles.ModeExact.Entries[1].Filters.Tags)
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
			got := mergeProfileNames(tc.explicit, tc.discovered)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuildCollectorRuntime_DetectsChartIDCollision(t *testing.T) {
	profileNames := []string{"redis_upper", "redis_lower"}

	catalog := mustLoadStockCatalog(t, map[string]string{
		"redis_upper.yaml": `
display_name: Azure Redis Cache Upper
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
display_name: Azure Redis Cache Lower
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

	_, err := buildCollectorRuntimeFromConfig(profileNames, nil, catalog, "")
	require.Error(t, err)
}

func testConfig() Config {
	return Config{
		UpdateEvery:        60,
		AutoDetectionRetry: 0,
		SubscriptionIDs:    []string{"sub-1"},
		Cloud:              "public",
		Discovery: DiscoveryConfig{
			RefreshEvery: 300,
			Mode:         discoveryModeFilters,
		},
		Profiles: ProfilesConfig{
			Mode:      profilesModeExact,
			ModeExact: testProfilesModeEntries("postgres_flexible"),
		},
		QueryOffset: 180,
		Timeout:     defaultTimeout,
		Limits: LimitsConfig{
			MaxConcurrency:     4,
			MaxBatchResources:  50,
			MaxMetricsPerQuery: 20,
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

func testProfilesModeEntries(names ...string) *ProfilesModeConfig {
	entries := make([]ProfileEntryConfig, 0, len(names))
	for _, name := range names {
		entries = append(entries, ProfileEntryConfig{Name: name})
	}
	return &ProfilesModeConfig{Entries: entries}
}

func newTestCollectorWithMocks(rg *mockResourceGraph, mx *mockMetricsClient) *Collector {
	c := New()
	c.newResourceGraph = func(string, azcore.TokenCredential, azcloud.Configuration) (resourceGraphClient, error) {
		return rg, nil
	}
	c.newMetricsClient = func(string, azcore.TokenCredential, azcloud.Configuration) (metricsQueryClient, error) {
		return mx, nil
	}
	return c
}

type mockResourceGraph struct {
	mu          sync.Mutex
	resources   []map[string]any
	responseErr error
	count       int
	query       string
	subs        []string
	timeout     time.Duration
	hasDL       bool
}

func (m *mockResourceGraph) Resources(ctx context.Context, req armresourcegraph.QueryRequest, _ *armresourcegraph.ClientResourcesOptions) (armresourcegraph.ClientResourcesResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.count++
	if req.Query != nil {
		m.query = *req.Query
	} else {
		m.query = ""
	}
	m.subs = m.subs[:0]
	for _, sub := range req.Subscriptions {
		if sub == nil {
			continue
		}
		m.subs = append(m.subs, *sub)
	}
	deadline, ok := ctx.Deadline()
	m.hasDL = ok
	if ok {
		m.timeout = time.Until(deadline)
	} else {
		m.timeout = 0
	}
	if m.responseErr != nil {
		return armresourcegraph.ClientResourcesResponse{}, m.responseErr
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

func (m *mockResourceGraph) lastQuery() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.query
}

func (m *mockResourceGraph) lastSubscriptions() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.subs...)
}

func (m *mockResourceGraph) setResources(resources []map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources = append([]map[string]any(nil), resources...)
}

type mockMetricsClient struct {
	mu             sync.Mutex
	queryResponse  azmetrics.QueryResourcesResponse
	queryResponses map[string]azmetrics.QueryResourcesResponse
	queryErr       error
	queryErrors    map[string]error
	queryCallsLog  []metricsQueryCall
	count          int
	subs           []string
	timeout        time.Duration
	hasDL          bool
}

type metricsQueryCall struct {
	SubscriptionID string
	MetricNames    []string
	StartTime      string
	EndTime        string
	Interval       string
	Aggregation    string
}

func (m *mockMetricsClient) QueryResources(ctx context.Context, subscriptionID string, _ string, metricNames []string, _ azmetrics.ResourceIDList, opts *azmetrics.QueryResourcesOptions) (azmetrics.QueryResourcesResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.count++
	m.subs = append(m.subs, subscriptionID)
	var startTime, endTime, interval, aggregation string
	if opts != nil {
		startTime = derefString(opts.StartTime)
		endTime = derefString(opts.EndTime)
		interval = derefString(opts.Interval)
		aggregation = derefString(opts.Aggregation)
	}
	m.queryCallsLog = append(m.queryCallsLog, metricsQueryCall{
		SubscriptionID: subscriptionID,
		MetricNames:    append([]string(nil), metricNames...),
		StartTime:      startTime,
		EndTime:        endTime,
		Interval:       interval,
		Aggregation:    aggregation,
	})
	deadline, ok := ctx.Deadline()
	m.hasDL = ok
	if ok {
		m.timeout = time.Until(deadline)
	} else {
		m.timeout = 0
	}
	if err, ok := m.queryErrors[subscriptionID]; ok && err != nil {
		return azmetrics.QueryResourcesResponse{}, err
	}
	if m.queryErr != nil {
		return azmetrics.QueryResourcesResponse{}, m.queryErr
	}
	if resp, ok := m.queryResponses[subscriptionID]; ok {
		return resp, nil
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

func (m *mockMetricsClient) subscriptionCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.subs...)
}

func (m *mockMetricsClient) queryCalls() []metricsQueryCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]metricsQueryCall(nil), m.queryCallsLog...)
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

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func assertTimeoutClose(t *testing.T, got, want time.Duration) {
	t.Helper()
	assert.InDelta(t, want.Seconds(), got.Seconds(), 1.0)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func keysFromSeries(series map[string]metrix.SampleValue) []string {
	out := make([]string, 0, len(series))
	for key := range series {
		out = append(out, key)
	}
	return out
}

func scalarSeriesFromReader(reader metrix.Reader) map[string]metrix.SampleValue {
	out := make(map[string]metrix.SampleValue)
	reader.ForEachSeries(func(name string, labels metrix.LabelView, value metrix.SampleValue) {
		out[scalarKeyFromLabelView(name, labels)] = value
	})
	return out
}

func scalarKeyFromLabelView(name string, labels metrix.LabelView) string {
	if labels == nil || labels.Len() == 0 {
		return strings.TrimSpace(name)
	}

	labelsMap := labels.CloneMap()
	keys := make([]string, 0, len(labelsMap))
	for key := range labelsMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(strings.TrimSpace(name))
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(labelsMap[key]))
	}
	b.WriteByte('}')
	return b.String()
}
