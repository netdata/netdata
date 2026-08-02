// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/executable"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/hostidentity"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collogpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	"google.golang.org/grpc"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

type collectorOTLPFixture struct {
	collogpb.UnimplementedLogsServiceServer
	endpoint string
}

func startCollectorOTLPFixture(t *testing.T) *collectorOTLPFixture {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	fixture := &collectorOTLPFixture{endpoint: "http://" + ln.Addr().String()}
	server := grpc.NewServer()
	collogpb.RegisterLogsServiceServer(server, fixture)
	go func() { _ = server.Serve(ln) }()
	t.Cleanup(func() {
		server.Stop()
		_ = ln.Close()
	})
	return fixture
}

func (f *collectorOTLPFixture) Export(context.Context, *collogpb.ExportLogsServiceRequest) (*collogpb.ExportLogsServiceResponse, error) {
	return &collogpb.ExportLogsServiceResponse{}, nil
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	require.NotEmpty(t, dataConfigJSON)
	require.NotEmpty(t, dataConfigYAML)
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollectorChartTemplateYAML(t *testing.T) {
	collecttest.AssertChartTemplateSchema(t, newTestSNMPTrapsCollector().ChartTemplateYAML())
}

func TestCollectorChartTemplateYAMLChartsDeclareAlgorithms(t *testing.T) {
	charts := chartTemplatesByIDFromYAML(t, newTestSNMPTrapsCollector().ChartTemplateYAML())
	assertAllChartTemplatesDeclareAlgorithm(t, charts)

	for _, id := range []string{
		"events",
		"severity",
		"errors",
		"dedup_suppressed",
		"pipeline",
	} {
		chart, ok := charts[id]
		require.Truef(t, ok, "missing chart %q", id)
		assert.Equalf(t, "incremental", chart.Algorithm, "chart %q algorithm", id)
	}
	for _, id := range []string{
		"sources",
		"source_attribution",
		"source_pipeline",
		"source_errors",
		"source_last_seen",
	} {
		assert.NotContains(t, charts, id)
	}
}

func TestCollectorChartTemplateYAMLIncludesProfileMetricCharts(t *testing.T) {
	idx := testProfileMetricIndex(t)
	cfg, err := normalizeProfileMetricsConfig(ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	})
	require.NoError(t, err)

	rt, tmpl, err := newProfileMetricRuntime(cfg, idx, "test")
	require.NoError(t, err)
	require.NotNil(t, rt)
	require.NotEmpty(t, tmpl)

	c := newTestSNMPTrapsCollector()
	c.profileMetrics = rt
	c.dynamicChartYAML = tmpl

	collecttest.AssertChartTemplateSchema(t, c.ChartTemplateYAML())
	charts := chartTemplatesByIDFromYAML(t, c.ChartTemplateYAML())
	assert.Contains(t, charts, "profile_metric_diagnostics")
	assert.Contains(t, charts, "cisco_config_changes")

	spec, err := charttpl.DecodeYAML([]byte(c.ChartTemplateYAML()))
	require.NoError(t, err)
	program, err := chartengine.Compile(spec, 1)
	require.NoError(t, err)

	contexts := make(map[string]bool)
	for _, chart := range program.Charts() {
		contexts[chart.Meta.Context] = true
	}
	assert.Contains(t, contexts, "snmp.trap.profile_metric_diagnostics")
	assert.Contains(t, contexts, "snmp.trap.cisco.config.changes")
}

func TestCollectorCreatorDefaults(t *testing.T) {
	creator := newCreator(ddsnmp.NewDeviceStore(), snmptopology.NewTrapEnrichmentHandle())
	assert.False(t, creator.Defaults.Disabled)
}

func TestCollectorCreatorSharesHostIdentityService(t *testing.T) {
	creator := newCreator(ddsnmp.NewDeviceStore(), snmptopology.NewTrapEnrichmentHandle())
	first := creator.CreateV2().(*Collector)
	second := creator.CreateV2().(*Collector)

	assert.Same(t, first.hostIdentity, second.hostIdentity)
	assert.Same(t, first.profileCatalog, second.profileCatalog)
	assert.NotNil(t, first.engineStateRoot)
}

func TestCollectorCreatorResolvesProfilePathsOnFirstCollector(t *testing.T) {
	originalExecutableDir := executable.Directory
	t.Cleanup(func() { executable.Directory = originalExecutableDir })

	earlyDir := filepath.Join(t.TempDir(), "before-plugin-config")
	require.NoError(t, os.MkdirAll(earlyDir, 0o755))
	executable.Directory = earlyDir
	creator := newCreator(ddsnmp.NewDeviceStore(), snmptopology.NewTrapEnrichmentHandle())

	root := t.TempDir()
	executableDir := filepath.Join(root, "plugins.d")
	stockDir := filepath.Join(root, "config", "go.d", "snmp.trap-profiles", "default")
	require.NoError(t, os.MkdirAll(executableDir, 0o755))
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "minimal.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.1
    name: SNMPv2-MIB::coldStart
    category: state_change
    severity: notice
`)
	writeProfileCatalogue(t, stockDir, map[string]any{
		"minimal": map[string]any{
			"file":      "minimal.yaml",
			"mibs":      []string{"SNMPv2-MIB"},
			"trap_oids": []string{"1.3.6.1.6.3.1.1.5.1"},
		},
	})
	executable.Directory = executableDir

	collector := creator.CreateV2().(*Collector)
	lease, err := collector.profileCatalog.Acquire()
	require.NoError(t, err)
	lease.Close()
}

func TestCollectorNewResolvesProfilePathsAtInit(t *testing.T) {
	originalExecutableDir := executable.Directory
	t.Cleanup(func() { executable.Directory = originalExecutableDir })

	earlyDir := filepath.Join(t.TempDir(), "before-plugin-config")
	require.NoError(t, os.MkdirAll(earlyDir, 0o755))
	executable.Directory = earlyDir
	collector := New(ddsnmp.NewDeviceStore(), snmptopology.NewTrapEnrichmentHandle())

	root := t.TempDir()
	executableDir := filepath.Join(root, "plugins.d")
	stockDir := filepath.Join(root, "config", "go.d", "snmp.trap-profiles", "default")
	require.NoError(t, os.MkdirAll(executableDir, 0o755))
	require.NoError(t, os.MkdirAll(stockDir, 0o755))
	writeProfileYAML(t, stockDir, "minimal.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.1
    name: SNMPv2-MIB::coldStart
    category: state_change
    severity: notice
`)
	writeProfileCatalogue(t, stockDir, map[string]any{
		"minimal": map[string]any{
			"file":      "minimal.yaml",
			"mibs":      []string{"SNMPv2-MIB"},
			"trap_oids": []string{"1.3.6.1.6.3.1.1.5.1"},
		},
	})
	executable.Directory = executableDir

	withTestCacheDir(t)
	collector.Name = "standalone-late-profile-path"
	collector.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	require.NoError(t, collector.Init(context.Background()))
	t.Cleanup(func() { collector.Cleanup(context.Background()) })

	td, err := collector.profileIndex.LookupWithError("1.3.6.1.6.3.1.1.5.1")
	require.NoError(t, err)
	require.NotNil(t, td)
	assert.Equal(t, "SNMPv2-MIB::coldStart", td.Name)
}

func TestCollectorNewUsesIndependentHostIdentityService(t *testing.T) {
	deviceStore := ddsnmp.NewDeviceStore()
	topologyEnricher := snmptopology.NewTrapEnrichmentHandle()
	first := New(deviceStore, topologyEnricher)
	second := New(deviceStore, topologyEnricher)

	assert.NotSame(t, first.hostIdentity, second.hostIdentity)
}

func TestCollectorCreatorRequiresSharedDependencies(t *testing.T) {
	require.PanicsWithValue(t, "snmp_traps Register requires a non-nil device store", func() {
		_ = newCreator(nil, snmptopology.NewTrapEnrichmentHandle())
	})
	require.PanicsWithValue(t, "snmp_traps Register requires a non-nil trap enrichment handle", func() {
		_ = newCreator(ddsnmp.NewDeviceStore(), nil)
	})
}

func TestCollectorNewRequiresSharedDependencies(t *testing.T) {
	require.PanicsWithValue(t, "snmp_traps New requires a non-nil device store", func() {
		_ = New(nil, snmptopology.NewTrapEnrichmentHandle())
	})
	require.PanicsWithValue(t, "snmp_traps New requires a non-nil trap enrichment handle", func() {
		_ = New(ddsnmp.NewDeviceStore(), nil)
	})
}

func chartTemplatesByIDFromYAML(t *testing.T, raw string) map[string]charttpl.Chart {
	t.Helper()

	spec, err := charttpl.DecodeYAML([]byte(raw))
	require.NoError(t, err)

	charts := make(map[string]charttpl.Chart)
	collectChartTemplatesByID(t, charts, spec.Groups)
	return charts
}

func assertAllChartTemplatesDeclareAlgorithm(t *testing.T, charts map[string]charttpl.Chart) {
	t.Helper()

	for id, chart := range charts {
		assert.NotEmptyf(t, chart.Algorithm, "chart %q must not rely on algorithm inference", id)
	}
}

func collectChartTemplatesByID(t *testing.T, charts map[string]charttpl.Chart, groups []charttpl.Group) {
	t.Helper()

	for _, group := range groups {
		for _, chart := range group.Charts {
			require.NotEmpty(t, chart.ID)
			require.NotContainsf(t, charts, chart.ID, "duplicate chart id %q", chart.ID)
			charts[chart.ID] = chart
		}
		collectChartTemplatesByID(t, charts, group.Groups)
	}
}

func TestConfigSchemaDynCfgListFieldsHaveSafeDefaults(t *testing.T) {
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(configSchema), &schema))

	assertSchemaArrayProperty(t, schema, "listen.endpoints", []string{"jsonSchema", "properties", "listen", "properties", "endpoints"}, "array", []any{
		map[string]any{"protocol": "udp", "address": "0.0.0.0", "port": float64(162)},
	})
	assertSchemaArrayProperty(t, schema, "versions", []string{"jsonSchema", "properties", "versions"}, "array", []any{"v1", "v2c"})

	for _, tc := range []struct {
		name        string
		path        []string
		wantDefault []any
	}{
		{name: "communities", path: []string{"jsonSchema", "properties", "communities"}},
		{name: "usm_users", path: []string{"jsonSchema", "properties", "usm_users"}},
		{name: "engine_id_whitelist", path: []string{"jsonSchema", "properties", "engine_id_whitelist"}},
		{name: "allowlist.source_cidrs", path: []string{"jsonSchema", "properties", "allowlist", "properties", "source_cidrs"}, wantDefault: []any{"0.0.0.0/0", "::/0"}},
		{name: "source.trusted_relays", path: []string{"jsonSchema", "properties", "source", "properties", "trusted_relays"}},
		{name: "dedup.key_varbinds", path: []string{"jsonSchema", "properties", "dedup", "properties", "key_varbinds"}},
		{name: "overrides", path: []string{"jsonSchema", "properties", "overrides"}},
		{name: "profile_metrics.include", path: []string{"jsonSchema", "properties", "profile_metrics", "properties", "include"}},
	} {
		wantDefault := tc.wantDefault
		if wantDefault == nil {
			wantDefault = []any{}
		}
		assertSchemaArrayProperty(t, schema, tc.name, tc.path, []any{"array", "null"}, wantDefault)
	}
}

func TestConfigSchemaDynCfgObjectFieldsHaveSafeDefaults(t *testing.T) {
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(configSchema), &schema))

	for _, tc := range []struct {
		name string
		path []string
	}{
		{name: "listen", path: []string{"jsonSchema", "properties", "listen"}},
		{name: "reverse_dns", path: []string{"jsonSchema", "properties", "reverse_dns"}},
		{name: "allowlist", path: []string{"jsonSchema", "properties", "allowlist"}},
		{name: "source", path: []string{"jsonSchema", "properties", "source"}},
		{name: "rate_limit", path: []string{"jsonSchema", "properties", "rate_limit"}},
		{name: "dedup", path: []string{"jsonSchema", "properties", "dedup"}},
		{name: "journal", path: []string{"jsonSchema", "properties", "journal"}},
		{name: "otlp", path: []string{"jsonSchema", "properties", "otlp"}},
		{name: "retention", path: []string{"jsonSchema", "properties", "retention"}},
		{name: "overrides.labels", path: []string{"jsonSchema", "properties", "overrides", "items", "properties", "labels"}},
		{name: "profile_metrics", path: []string{"jsonSchema", "properties", "profile_metrics"}},
	} {
		prop := schemaProperty(t, schema, tc.path...)
		require.Containsf(t, prop, "default", "schema property %q has no default", tc.name)
		assert.NotNilf(t, prop["default"], "schema property %q has nil default", tc.name)
	}
}

func TestConfigSchemaDynCfgListenDefaultIncludesReceiveBuffer(t *testing.T) {
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(configSchema), &schema))

	listen := schemaProperty(t, schema, "jsonSchema", "properties", "listen")
	defaults, ok := listen["default"].(map[string]any)
	require.Truef(t, ok, "listen default is %T", listen["default"])
	assert.Equal(t, float64(defaultListenerReceiveBuffer), defaults["receive_buffer"])

	receiveBuffer := schemaProperty(t, schema, "jsonSchema", "properties", "listen", "properties", "receive_buffer")
	assert.Equal(t, float64(defaultListenerReceiveBuffer), receiveBuffer["default"])
	assert.Equal(t, float64(maxListenerReceiveBuffer), receiveBuffer["maximum"])
}

func TestConfigSchemaDynCfgRetentionDefaultDisablesTimeRotation(t *testing.T) {
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(configSchema), &schema))

	retention := schemaProperty(t, schema, "jsonSchema", "properties", "retention")
	defaults, ok := retention["default"].(map[string]any)
	require.Truef(t, ok, "retention default is %T", retention["default"])
	assert.Nil(t, defaults["rotation_duration"])

	rotationDuration := schemaProperty(t, schema, "jsonSchema", "properties", "retention", "properties", "rotation_duration")
	assert.Nil(t, rotationDuration["default"])
}

func TestCollectorDefaultListenReceiveBuffer(t *testing.T) {
	assert.Equal(t, defaultListenerReceiveBuffer, newTestSNMPTrapsCollector().Listen.ReceiveBuffer)
}

func TestConfigValidateIsPure(t *testing.T) {
	cfg := Config{
		Name:     "local",
		Listen:   ListenConfig{Endpoints: []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: 162}}},
		Versions: []string{" V2C "},
	}
	before, err := json.Marshal(cfg)
	require.NoError(t, err)

	require.NoError(t, cfg.Validate())

	after, err := json.Marshal(cfg)
	require.NoError(t, err)
	assert.JSONEq(t, string(before), string(after))
	assert.Equal(t, []string{" V2C "}, cfg.Versions)
}

func TestCollectorInitValidatesBeforeAcquiringResources(t *testing.T) {
	profileDir := t.TempDir()
	writeProfileYAML(t, profileDir, "invalid.yaml", "unknown_profile_key: true\n")
	setTestDirs(t, profileDir)

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: 162}}
	c.ProfileMetrics = ProfileMetricsConfig{Enabled: true}

	err := c.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "profile_metrics.include")
	assert.NotContains(t, err.Error(), "unknown_profile_key")
	assert.Nil(t, c.profileLease)
	assert.Nil(t, c.listener)
	assert.Nil(t, c.trapWriter)
	assert.Nil(t, c.metrics)
}

func TestConfigSchemaDynCfgTabsRenderAllTopLevelFieldsOnce(t *testing.T) {
	var schema map[string]any
	require.NoError(t, json.Unmarshal([]byte(configSchema), &schema))

	topLevelProps := schemaProperty(t, schema, "jsonSchema", "properties")
	uiSchema := schemaProperty(t, schema, "uiSchema")
	assert.Equal(t, "tabs", uiSchema["ui:flavour"])

	uiOptions := schemaProperty(t, schema, "uiSchema", "ui:options")
	tabsRaw, ok := uiOptions["tabs"].([]any)
	require.Truef(t, ok, "uiSchema.ui:options.tabs is %T", uiOptions["tabs"])

	wantTabs := []struct {
		title  string
		fields []string
	}{
		{title: "Base", fields: []string{"update_every", "vnode"}},
		{title: "Listener", fields: []string{"listen"}},
		{title: "SNMP", fields: []string{
			"versions",
			"communities",
			"usm_users",
			"engine_id_whitelist",
			"local_engine_id",
			"dynamic_engine_id_discovery",
			"dynamic_engine_id_max_pairs",
		}},
		{title: "Filtering", fields: []string{"allowlist", "source", "rate_limit", "dedup"}},
		{title: "Outputs", fields: []string{"journal", "otlp"}},
		{title: "Storage", fields: []string{"retention"}},
		{title: "Enrichment", fields: []string{"reverse_dns", "overrides"}},
		{title: "Metrics", fields: []string{"profile_metrics"}},
	}
	require.Len(t, tabsRaw, len(wantTabs))

	seen := make(map[string]int, len(topLevelProps))
	for i, tabRaw := range tabsRaw {
		tab, ok := tabRaw.(map[string]any)
		require.Truef(t, ok, "tab %d is %T", i, tabRaw)

		assert.Equalf(t, wantTabs[i].title, tab["title"], "tab %d title", i)
		fields := schemaStringSlice(t, tab["fields"], "tab fields")
		assert.Equalf(t, wantTabs[i].fields, fields, "tab %q fields", wantTabs[i].title)

		for _, field := range fields {
			assert.Containsf(t, topLevelProps, field, "tab %q references unknown field %q", wantTabs[i].title, field)
			seen[field]++
		}
	}

	for field := range topLevelProps {
		assert.Equalf(t, 1, seen[field], "top-level schema field %q tab references", field)
	}
}

func assertSchemaArrayProperty(t *testing.T, schema map[string]any, name string, path []string, wantType any, wantDefault []any) {
	t.Helper()
	prop := schemaProperty(t, schema, path...)
	assert.Equalf(t, wantType, prop["type"], "schema property %q type", name)
	assert.Equalf(t, wantDefault, prop["default"], "schema property %q default", name)
}

func schemaStringSlice(t *testing.T, raw any, name string) []string {
	t.Helper()
	items, ok := raw.([]any)
	require.Truef(t, ok, "%s is %T", name, raw)

	out := make([]string, 0, len(items))
	for i, item := range items {
		s, ok := item.(string)
		require.Truef(t, ok, "%s[%d] is %T", name, i, item)
		out = append(out, s)
	}
	return out
}

func schemaProperty(t *testing.T, schema map[string]any, path ...string) map[string]any {
	t.Helper()
	var cur any = schema
	for _, key := range path {
		m, ok := cur.(map[string]any)
		require.Truef(t, ok, "schema path %v: %q parent is %T", path, key, cur)
		cur, ok = m[key]
		require.Truef(t, ok, "schema path %v: missing %q", path, key)
	}
	prop, ok := cur.(map[string]any)
	require.Truef(t, ok, "schema path %v resolved to %T", path, cur)
	return prop
}

func TestJournalBackendConfigEnabledDefault(t *testing.T) {
	disabled := false
	enabled := true

	assert.True(t, JournalBackendConfig{}.enabled())
	assert.False(t, JournalBackendConfig{Enabled: &disabled}.enabled())
	assert.True(t, JournalBackendConfig{Enabled: &enabled}.enabled())
}

func TestValidateJobName(t *testing.T) {
	tests := map[string]struct {
		name    string
		wantErr bool
	}{
		"valid simple":            {name: "local"},
		"valid with underscore":   {name: "my_job"},
		"valid with dash":         {name: "my-job"},
		"valid with numbers":      {name: "job123"},
		"valid alphanumeric":      {name: "a1_b2-c3"},
		"valid single char":       {name: "a"},
		"valid 64 chars":          {name: "a123456789012345678901234567890123456789012345678901234567890123"},
		"empty":                   {name: "", wantErr: true},
		"too long 65 chars":       {name: "a1234567890123456789012345678901234567890123456789012345678901234", wantErr: true},
		"contains dot":            {name: "my.job", wantErr: true},
		"contains slash":          {name: "my/job", wantErr: true},
		"contains backslash":      {name: "my\\job", wantErr: true},
		"contains control char":   {name: "my\x00job", wantErr: true},
		"contains colon":          {name: "my:job", wantErr: true},
		"contains space":          {name: "my job", wantErr: true},
		"starts with dash":        {name: "-job", wantErr: true},
		"starts with underscore":  {name: "_job", wantErr: true},
		"valid starts with digit": {name: "1job"},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			err := validateJobName(tc.name)
			if tc.wantErr {
				assert.Error(t, err, "validateJobName(%q) should fail", tc.name)
			} else {
				assert.NoError(t, err, "validateJobName(%q) should pass", tc.name)
			}
		})
	}
}

func TestValidateEndpoints(t *testing.T) {
	tests := map[string]struct {
		endpoints []EndpointConfig
		wantErr   bool
		errMsg    string
	}{
		"valid single endpoint": {
			endpoints: []EndpointConfig{{Protocol: "udp", Address: "0.0.0.0", Port: 162}},
		},
		"valid IPv6 endpoint": {
			endpoints: []EndpointConfig{{Protocol: "udp", Address: "::1", Port: 162}},
		},
		"valid multiple endpoints": {
			endpoints: []EndpointConfig{
				{Protocol: "udp", Address: "0.0.0.0", Port: 162},
				{Protocol: "udp", Address: "::1", Port: 3162},
			},
		},
		"duplicate endpoint": {
			endpoints: []EndpointConfig{
				{Protocol: "udp", Address: "127.0.0.1", Port: 162},
				{Protocol: "UDP", Address: "127.0.0.1", Port: 162},
			},
			wantErr: true, errMsg: "duplicate endpoint",
		},
		"empty endpoints": {
			endpoints: nil, wantErr: true, errMsg: "at least one endpoint",
		},
		"unsupported protocol": {
			endpoints: []EndpointConfig{{Protocol: "tcp", Address: "0.0.0.0", Port: 162}},
			wantErr:   true, errMsg: "unsupported protocol",
		},
		"missing address": {
			endpoints: []EndpointConfig{{Protocol: "udp", Address: "", Port: 162}},
			wantErr:   true, errMsg: "address is required",
		},
		"invalid port zero": {
			endpoints: []EndpointConfig{{Protocol: "udp", Address: "0.0.0.0", Port: 0}},
			wantErr:   true, errMsg: "port must be",
		},
		"invalid port too high": {
			endpoints: []EndpointConfig{{Protocol: "udp", Address: "0.0.0.0", Port: 65536}},
			wantErr:   true, errMsg: "port must be",
		},
		"invalid address": {
			endpoints: []EndpointConfig{{Protocol: "udp", Address: "not-an-address", Port: 162}},
			wantErr:   true, errMsg: "invalid address/port",
		},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			err := validateEndpoints(tc.endpoints)
			if tc.wantErr {
				assert.Error(t, err)
				if tc.errMsg != "" {
					assert.Contains(t, err.Error(), tc.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateVersions(t *testing.T) {
	tests := map[string]struct {
		versions []string
		want     []string
		wantErr  bool
		errMsg   string
	}{
		"valid v1": {
			versions: []string{"v1"},
			want:     []string{"v1"},
		},
		"valid v2c": {
			versions: []string{"v2c"},
			want:     []string{"v2c"},
		},
		"valid both normalized": {
			versions: []string{" V1 ", "V2C"},
			want:     []string{"v1", "v2c"},
		},
		"empty": {
			versions: nil, wantErr: true, errMsg: "at least one SNMP version",
		},
		"valid v3": {
			versions: []string{"v3"},
			want:     []string{"v3"},
		},
		"duplicate normalized": {
			versions: []string{"v2c", "V2C"}, wantErr: true, errMsg: "duplicate SNMP version",
		},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			got, err := validateVersions(tc.versions)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCollectorInit_BindsEndpointsAndCheckIsNoop(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)
	port := freeUDPPort(t)

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port}}
	c.Versions = []string{" V1 ", "V2C"}

	startJournalJobs := activeDirectJournalJobs.Load()
	require.NoError(t, c.Init(context.Background()))
	require.NotNil(t, c.listener)
	assert.Equal(t, []string{"v1", "v2c"}, c.Versions)
	configured, ok := c.Configuration().(Config)
	require.True(t, ok)
	assert.Equal(t, "local", configured.Name)
	assert.Equal(t, []string{"v1", "v2c"}, configured.Versions)
	require.NotEmpty(t, c.journalDir)
	require.DirExists(t, c.journalDir)
	assert.Equal(t, startJournalJobs+1, activeDirectJournalJobs.Load())
	assert.True(t, directJournalLogsAvailable())
	assert.Equal(t, trapWriteFailureJournal, c.trapWriteFailureDim())
	require.NoError(t, c.Check(context.Background()))

	c.Cleanup(context.Background())
	require.Nil(t, c.listener)
	assert.Equal(t, startJournalJobs, activeDirectJournalJobs.Load())
}

func TestCollectorInit_JournalHostFailureRetriesFreshProvider(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)
	provider := newTestJournalHostProvider()
	loadCalls := 0
	service := hostidentity.NewWithLoader(
		func() hostidentity.LoadConfig { return hostidentity.LoadConfig{StateDir: t.TempDir()} },
		func(hostidentity.LoadConfig) (hostidentity.Provider, error) {
			loadCalls++
			if loadCalls == 1 {
				return nil, errors.New("identity unavailable")
			}
			return provider, nil
		},
	)
	c := newCollector(
		ddsnmp.NewDeviceStore(),
		snmptopology.NewTrapEnrichmentHandle(),
		service,
		currentTestCatalogManager,
		func() string { return t.TempDir() },
	)
	c.Name = "journal-host-retry"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 503, coded.DyncfgCode())
	assert.Nil(t, c.listener)

	require.NoError(t, c.Init(context.Background()))
	assert.Equal(t, 2, loadCalls)
	assert.Same(t, provider, c.journalHost)
	assert.Equal(t, int64(1000), c.monotonicUsec())

	c.Cleanup(context.Background())
}

func TestCollectorInit_IdempotentDoubleInit(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)
	port := freeUDPPort(t)

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port}}

	require.NoError(t, c.Init(context.Background()))
	require.NotNil(t, c.listener)
	first := c.listener
	require.NoError(t, c.Init(context.Background()))
	assert.Same(t, first, c.listener)

	c.Cleanup(context.Background())
	require.Nil(t, c.listener)
}

func TestCollectorInit_InvalidJobNameIsCodedError(t *testing.T) {
	withTestCacheDir(t)

	c := newTestSNMPTrapsCollector()
	c.Name = "../bad"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: 162}}

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 422, coded.DyncfgCode())
	var retryable interface{ DyncfgRetryable() bool }
	require.ErrorAs(t, err, &retryable)
	assert.False(t, retryable.DyncfgRetryable())
	assert.Nil(t, c.listener)
}

func TestCollectorInit_InvalidEndpointsIsCodedError(t *testing.T) {
	withTestCacheDir(t)

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "tcp", Address: "127.0.0.1", Port: 162}}

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 422, coded.DyncfgCode())
	assert.Nil(t, c.listener)
}

func TestCollectorInit_InvalidReceiveBufferIsCodedError(t *testing.T) {
	withTestCacheDir(t)

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	c.Listen.ReceiveBuffer = -1

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 422, coded.DyncfgCode())
	assert.Contains(t, err.Error(), "listen.receive_buffer")
	assert.Nil(t, c.listener)
}

func TestCollectorInit_TooLargeReceiveBufferIsCodedError(t *testing.T) {
	withTestCacheDir(t)

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	c.Listen.ReceiveBuffer = maxListenerReceiveBuffer + 1

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 422, coded.DyncfgCode())
	assert.Contains(t, err.Error(), "listen.receive_buffer")
	assert.Nil(t, c.listener)
}

func TestCollectorInit_NoOutputBackendIsCodedError(t *testing.T) {
	disabled := false
	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	c.Journal.Enabled = &disabled

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 422, coded.DyncfgCode())
	assert.Contains(t, err.Error(), "at least one SNMP trap output backend")
	assert.Nil(t, c.listener)
}

func TestCollectorInit_MissingNetdataLogRootIsRetryableCodedError(t *testing.T) {
	setMinimalProfileDir(t)
	root := filepath.Join(t.TempDir(), "missing")
	withNetdataLogDir(t, root)

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}

	startJournalJobs := activeDirectJournalJobs.Load()
	err := c.Init(context.Background())
	require.Error(t, err)

	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 503, coded.DyncfgCode())
	var retryable interface{ DyncfgRetryable() bool }
	require.ErrorAs(t, err, &retryable)
	assert.True(t, retryable.DyncfgRetryable())
	assert.Contains(t, err.Error(), "Netdata log directory")
	assert.Nil(t, c.listener)
	assert.Equal(t, startJournalJobs, activeDirectJournalJobs.Load())
	assert.NoDirExists(t, root)
}

func TestCollectorInit_OTELOnlySkipsJournalCreation(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)
	disabled := false
	badRetention := "not-a-size"
	srv := startCollectorOTLPFixture(t)

	const jobName = "otel-only"
	c := newTestSNMPTrapsCollector()
	c.Name = jobName
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	c.Journal.Enabled = &disabled
	c.Retention.MaxSize = &badRetention
	c.OTLP = OTLPConfig{
		Enabled:       true,
		Endpoint:      srv.endpoint,
		FlushInterval: "1h",
		QueueCapacity: 16,
	}

	startJournalJobs := activeDirectJournalJobs.Load()
	require.NoError(t, c.Init(context.Background()))
	require.NotNil(t, c.listener)
	assert.Empty(t, c.journalDir)
	assert.Equal(t, startJournalJobs, activeDirectJournalJobs.Load())
	assert.NoDirExists(t, journal.Root(jobName))
	assert.Equal(t, trapWriteFailureOTLP, c.trapWriteFailureDim())

	c.Cleanup(context.Background())
	require.Nil(t, c.listener)
	assert.Equal(t, startJournalJobs, activeDirectJournalJobs.Load())
}

func TestCollectorInit_OTLPPreflightFailureIsRetryableCodedError(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)
	disabled := false
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	endpoint := "http://" + ln.Addr().String()
	require.NoError(t, ln.Close())

	c := newTestSNMPTrapsCollector()
	c.Name = "otlp-preflight"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	c.Journal.Enabled = &disabled
	c.OTLP = OTLPConfig{
		Enabled:        true,
		Endpoint:       endpoint,
		RequestTimeout: "50ms",
	}

	err = c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 503, coded.DyncfgCode())
	var retryable interface{ DyncfgRetryable() bool }
	require.ErrorAs(t, err, &retryable)
	assert.True(t, retryable.DyncfgRetryable())
	assert.Nil(t, c.listener)
	assert.Empty(t, c.journalDir)
}

func TestCollectorInit_BindsMultipleEndpoints(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)
	firstPort := freeUDPPort(t)
	secondPort := freeUDPPort(t)

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{
		{Protocol: "udp", Address: "127.0.0.1", Port: firstPort},
		{Protocol: "udp", Address: "127.0.0.1", Port: secondPort},
	}

	require.NoError(t, c.Init(context.Background()))
	require.NotNil(t, c.listener)
	require.Len(t, c.listener.endpoints, 2)

	c.Cleanup(context.Background())
	require.Nil(t, c.listener)

	firstConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: firstPort})
	require.NoError(t, err, "first endpoint should close on cleanup")
	require.NoError(t, firstConn.Close())

	secondConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: secondPort})
	require.NoError(t, err, "second endpoint should close on cleanup")
	require.NoError(t, secondConn.Close())
}

func TestCollectorInit_BindFailureIsRetryableCodedError(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	port := conn.LocalAddr().(*net.UDPAddr).Port

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port}}

	err = c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 503, coded.DyncfgCode())
	var retryable interface{ DyncfgRetryable() bool }
	require.ErrorAs(t, err, &retryable)
	assert.True(t, retryable.DyncfgRetryable())
	assert.Nil(t, c.listener)
}

func TestCollectorInit_ReceiveBufferFailureIsRetryableCodedError(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)

	oldSetUDPReadBuffer := setUDPReadBuffer
	t.Cleanup(func() { setUDPReadBuffer = oldSetUDPReadBuffer })
	setUDPReadBuffer = func(_ *net.UDPConn, _ int) error {
		return errors.New("set buffer failed")
	}

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	c.Listen.ReceiveBuffer = defaultListenerReceiveBuffer + 1

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 503, coded.DyncfgCode())
	var retryable interface{ DyncfgRetryable() bool }
	require.ErrorAs(t, err, &retryable)
	assert.True(t, retryable.DyncfgRetryable())
	assert.Contains(t, err.Error(), "set receive buffer")
	assert.Nil(t, c.listener)
}

func TestCollectorInit_InvalidVersionIsCodedError(t *testing.T) {
	withTestCacheDir(t)

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: 162}}
	c.Versions = []string{"v5"}

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 422, coded.DyncfgCode())
	assert.Nil(t, c.listener)
}

func TestCollectorInit_ProfileLoadFailureIsCodedError(t *testing.T) {
	setTestDirs(t, t.TempDir())
	withTestCacheDir(t)

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	c.Versions = []string{" V1 ", "V2C"}

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 422, coded.DyncfgCode())
	assert.Nil(t, c.listener)
	assert.Equal(t, []string{" V1 ", "V2C"}, c.Versions)
}

func TestCollectorInit_PartialBindFailureClosesPriorSockets(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)
	firstPort := freeUDPPort(t)
	secondConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer secondConn.Close()
	secondPort := secondConn.LocalAddr().(*net.UDPAddr).Port

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{
		{Protocol: "udp", Address: "127.0.0.1", Port: firstPort},
		{Protocol: "udp", Address: "127.0.0.1", Port: secondPort},
	}

	err = c.Init(context.Background())
	require.Error(t, err)
	assert.Nil(t, c.listener)

	firstConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: firstPort})
	require.NoError(t, err, "first endpoint should have been closed after partial bind failure")
	require.NoError(t, firstConn.Close())
}

func TestEngineStatePathExistsCheckedReturnsStatError(t *testing.T) {
	dir := t.TempDir()
	notDir := filepath.Join(dir, "not-a-dir")
	require.NoError(t, os.WriteFile(notDir, []byte("not a directory"), 0644))

	exists, err := engineStatePathExistsChecked(filepath.Join(notDir, "engine-boots"))
	require.Error(t, err)
	assert.False(t, exists)
}

func TestCollectorInit_EngineStateStatErrorIsRetryableCodedError(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)

	const jobName = "engine-state-stat-error"
	root := t.TempDir()
	paths := newEngineStatePaths(root, jobName)
	require.NoError(t, os.WriteFile(paths.dir, []byte("not a directory"), 0644))

	c := newTestSNMPTrapsCollector()
	c.engineStateRoot = func() string { return root }
	c.Name = jobName
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	c.Versions = []string{"v3"}
	c.USMUsers = []USMUserConfig{{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	}}
	c.EngineIDWhitelist = []string{testEngineIDHex}

	err := c.Init(context.Background())
	require.Error(t, err)
	var coded interface{ DyncfgCode() int }
	require.ErrorAs(t, err, &coded)
	assert.Equal(t, 503, coded.DyncfgCode())
	var retryable interface{ DyncfgRetryable() bool }
	require.ErrorAs(t, err, &retryable)
	assert.True(t, retryable.DyncfgRetryable())
	assert.Nil(t, c.listener)
	assert.FileExists(t, paths.dir, "pre-existing invalid state path must not be removed")
}

func TestCollectorInit_CleansCreatedV3StateOnEngineBootsFailure(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)

	const jobName = "cleanup-v3-state"
	root := t.TempDir()
	paths := newEngineStatePaths(root, jobName)
	require.NoError(t, os.MkdirAll(paths.engineBoots, 0750))

	c := newTestSNMPTrapsCollector()
	c.engineStateRoot = func() string { return root }
	c.Name = jobName
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	c.Versions = []string{"v3"}
	c.USMUsers = []USMUserConfig{{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	}}
	c.EngineIDWhitelist = []string{testEngineIDHex}

	err := c.Init(context.Background())
	require.Error(t, err)
	assert.NoFileExists(t, paths.localEngineID)
	assert.DirExists(t, paths.engineBoots, "pre-existing state path must not be removed")
	assert.Nil(t, c.listener)
}

func TestCollectorCleanupIsIdempotent(t *testing.T) {
	setMinimalProfileDir(t)
	withTestCacheDir(t)
	port := freeUDPPort(t)

	c := newTestSNMPTrapsCollector()
	c.Name = "local"
	c.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: port}}

	require.NoError(t, c.Init(context.Background()))
	require.NotNil(t, c.listener)

	c.Cleanup(context.Background())
	c.Cleanup(context.Background())
	require.Nil(t, c.listener)
}

func TestCollectorCollectRequiresStartedListener(t *testing.T) {
	c := newTestSNMPTrapsCollector()
	err := c.Collect(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "listener not started")
}

func freeUDPPort(t *testing.T) int {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	defer conn.Close()

	return conn.LocalAddr().(*net.UDPAddr).Port
}
