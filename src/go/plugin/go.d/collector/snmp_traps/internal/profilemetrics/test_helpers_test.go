// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/attribution"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

type testRuntimeConfig struct {
	Enabled bool
	Include []string
}

type testCatalog struct {
	epoch  *catalog.Epoch
	rules  map[string]*catalog.MetricRule
	charts map[string]*catalog.MetricChart
}
type testTrapEntry = model.TrapEntry
type testTrapDef = catalog.TrapDef
type testVarbindDef = catalog.VarbindDef
type testVarbindValue = model.VarbindValue
type testTrapEnrichmentAudit = model.TrapEnrichmentAudit
type testTrapEnrichmentLookup = model.TrapEnrichmentLookup
type testTrapSourceAudit = model.TrapSourceAudit
type testCategory = model.Category
type testSeverity = model.Severity
type profileMetricIdentity = catalog.MetricIdentity
type profileMetricResource = catalog.MetricResource
type profileMetricOutput = catalog.MetricOutput
type profileMetricScale = catalog.MetricScale
type profileMetricState = catalog.MetricState
type profileMetricPredicates = catalog.MetricPredicates

const (
	profileMetricTypeCounter = catalog.MetricTypeCounter
	profileMetricTypeSample  = catalog.MetricTypeSample
	profileMetricTypeState   = catalog.MetricTypeState

	profileMetricIdentitySourceLabel = catalog.MetricIdentitySourceLabel
	profileMetricIdentityListener    = catalog.MetricIdentityListener

	profileMetricMissingDrop             = catalog.MetricMissingDrop
	profileMetricMissingZero             = catalog.MetricMissingZero
	profileMetricMissingUnknownDimension = catalog.MetricMissingUnknownDimension
	profileMetricMissingError            = catalog.MetricMissingError
)

func newTestCatalog() *testCatalog {
	return &testCatalog{
		epoch:  catalog.NewEpoch(),
		rules:  make(map[string]*catalog.MetricRule),
		charts: make(map[string]*catalog.MetricChart),
	}
}

func (idx *testCatalog) addTraps(traps []*testTrapDef) error { return idx.epoch.AddTraps(traps) }

func (idx *testCatalog) addDefinitions(rules []catalog.MetricRule, charts []catalog.MetricChart) error {
	for i := range rules {
		rule := rules[i]
		idx.rules[rule.Name] = &rule
	}
	for i := range charts {
		chart := charts[i]
		if chart.Type == "" {
			chart.Type = "line"
		}
		idx.charts[chart.ID] = &chart
	}
	return nil
}

func (idx *testCatalog) Definitions(names []string) (catalog.MetricDefinitions, error) {
	defs := catalog.MetricDefinitions{
		RulesByName: make(map[string]*catalog.MetricRule),
		ChartsByID:  make(map[string]*catalog.MetricChart),
	}
	if names == nil {
		maps.Copy(defs.RulesByName, idx.rules)
		maps.Copy(defs.ChartsByID, idx.charts)
		return defs, nil
	}
	for _, name := range names {
		rule := idx.rules[name]
		if rule == nil {
			continue
		}
		defs.RulesByName[name] = rule
		if chart := idx.charts[rule.Output.Chart]; chart != nil {
			defs.ChartsByID[chart.ID] = chart
		}
	}
	return defs, nil
}

func (idx *testCatalog) ResolveTrap(ref string) (*testTrapDef, error) {
	return idx.epoch.ResolveTrap(ref)
}

func normalizeTestRuntimeConfig(cfg testRuntimeConfig) (Policy, error) {
	return Normalize(cfg.Enabled, cfg.Include)
}

func newTestRuntime(cfg Policy, idx Catalog, sourceHashSalt string) (*Runtime, string, error) {
	rt, err := New(cfg, idx, Options{
		BaseChartTemplateYAML: testBaseChartTemplate(),
		SourceHashSalt:        sourceHashSalt,
	})
	if err != nil || rt == nil {
		return rt, "", err
	}
	return rt, rt.ChartTemplateYAML(), nil
}

func testBaseChartTemplate() string {
	raw, err := os.ReadFile(filepath.Join("..", "..", "charts.yaml"))
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func fallbackTrapSourceIdentity(entry *testTrapEntry, jobName, sourceHashSalt string) (string, string) {
	source, ok := attribution.Resolve(entry, jobName, attribution.DeviceSource, sourceHashSalt)
	if !ok {
		return "", ""
	}
	return source.Key.SourceID, source.Key.SourceKind
}

const (
	testCiscoConfigTrapOID        = "1.3.6.1.4.1.9.9.43.2.0.1"
	testCiscoCommandSourceOID     = "1.3.6.1.4.1.9.9.43.1.1.1.1"
	testCiscoTerminalTypeOID      = "1.3.6.1.4.1.9.9.43.1.1.1.2"
	testCiscoTerminalTypeVarbind  = "ccmHistoryEventTerminalType"
	testCiscoCommandSourceVarbind = "ccmHistoryEventCommandSource"
	testIfIndexOID                = "1.3.6.1.2.1.2.2.1.1"
	testPortSecurityTrapOID       = "1.3.6.1.4.1.9.9.46.2.0.1"
	testLinkDownTrapOID           = "1.3.6.1.6.3.1.1.5.3"
	testLinkUpTrapOID             = "1.3.6.1.6.3.1.1.5.4"
	testProfileMetricJobName      = "profile-job"
)

func needCycleManagedStore(t *testing.T, store metrix.CollectorStore) metrix.CycleManagedStore {
	t.Helper()
	ms, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatalf("AsCycleManagedStore returned false")
	}
	return ms
}

func newPopulatedTestCatalog(t *testing.T) *testCatalog {
	t.Helper()
	idx := newTestCatalog()
	traps := []*testTrapDef{
		{
			OID:      testCiscoConfigTrapOID,
			Name:     "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
			Category: "config_change",
			Severity: "notice",
			VarbindRefs: []any{
				testCiscoCommandSourceVarbind,
				testCiscoTerminalTypeVarbind,
				"sysUpTime.0",
			},
			SharedVarbinds: map[string]*testVarbindDef{
				testCiscoCommandSourceOID: {
					OID:         testCiscoCommandSourceOID,
					Type:        "INTEGER",
					RawName:     testCiscoCommandSourceVarbind,
					Constraints: "(1..4)",
				},
				testCiscoTerminalTypeOID: {
					OID:     testCiscoTerminalTypeOID,
					Type:    "INTEGER",
					RawName: testCiscoTerminalTypeVarbind,
					Enum: map[string]string{
						"1": "none",
						"2": "console",
						"3": "virtual",
						"4": "aux",
					},
				},
				model.SysUpTimeOID: {
					OID:     model.SysUpTimeOID,
					Type:    "TimeTicks",
					RawName: "sysUpTime.0",
				},
			},
		},
		{
			OID:      testPortSecurityTrapOID,
			Name:     "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
			Category: "security",
			Severity: "warning",
			VarbindRefs: []any{
				"ifIndex",
			},
			SharedVarbinds: map[string]*testVarbindDef{
				testIfIndexOID: {
					OID:         testIfIndexOID,
					Type:        "INTEGER",
					RawName:     "ifIndex",
					Constraints: "(1..48)",
				},
			},
		},
	}
	if err := idx.addTraps(traps); err != nil {
		t.Fatalf("addTraps failed: %v", err)
	}
	charts := []profileMetricChart{
		{
			ID:         "cisco_config_changes",
			Title:      "Cisco config changes",
			Context:    "snmp.trap.cisco.config.changes",
			Units:      "events/s",
			Algorithm:  "incremental",
			Type:       "line",
			SourceFile: "test-profile.yaml",
		},
		{
			ID:         "cisco_terminal_type",
			Title:      "Cisco terminal type",
			Context:    "snmp.trap.cisco.terminal.type",
			Units:      "type",
			Algorithm:  "absolute",
			Type:       "line",
			SourceFile: "test-profile.yaml",
		},
		{
			ID:         "port_security_violations",
			Title:      "Port security violations",
			Context:    "snmp.trap.cisco.port.security.violations",
			Units:      "events/s",
			Algorithm:  "incremental",
			Type:       "line",
			SourceFile: "test-profile.yaml",
		},
	}
	rules := []profileMetricRule{
		{
			Name:   "cisco.config.changed",
			Type:   profileMetricTypeCounter,
			OnTrap: "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_config_events",
				Dimension: "events",
				Chart:     "cisco_config_changes",
			},
			SourceFile: "test-profile.yaml",
		},
		{
			Name:             "cisco.config.terminal_type",
			Type:             profileMetricTypeSample,
			OnTrap:           testCiscoConfigTrapOID,
			ValueFromVarbind: testCiscoTerminalTypeVarbind,
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_terminal_type",
				Dimension: "terminal_type",
				Chart:     "cisco_terminal_type",
			},
			SourceFile: "test-profile.yaml",
		},
		{
			Name:     "cisco.port_security.ifindex",
			Type:     profileMetricTypeCounter,
			OnTrap:   testPortSecurityTrapOID,
			Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 48}},
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_port_security_violations",
				Dimension: "violations",
				Chart:     "port_security_violations",
			},
			SourceFile: "test-profile.yaml",
		},
	}
	if err := idx.addDefinitions(rules, charts); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	return idx
}

func newTestProfileMetricRuntime(t *testing.T, idx Catalog, include []string) *Runtime {
	t.Helper()
	return newTestProfileMetricRuntimeWithConfig(t, idx, testRuntimeConfig{
		Enabled: true,
		Include: include,
	})
}

func newTestProfileMetricRuntimeWithConfig(t *testing.T, idx Catalog, cfg testRuntimeConfig) *Runtime {
	t.Helper()
	return newTestProfileMetricRuntimeWithPolicy(t, idx, cfg, nil)
}

func newTestProfileMetricRuntimeWithPolicy(t *testing.T, idx Catalog, cfg testRuntimeConfig, configure func(*Policy)) *Runtime {
	t.Helper()
	normalized, err := normalizeTestRuntimeConfig(cfg)
	if err != nil {
		t.Fatalf("normalizeTestRuntimeConfig failed: %v", err)
	}
	if configure != nil {
		configure(&normalized)
	}
	rt, tmpl, err := newTestRuntime(normalized, idx, "test")
	if err != nil {
		t.Fatalf("newTestRuntime failed: %v", err)
	}
	if rt == nil {
		t.Fatalf("newTestRuntime returned nil runtime")
	}
	collecttest.AssertChartTemplateSchema(t, tmpl)
	return rt
}

func collectProfileMetricsOnce(t *testing.T, rt *Runtime, store metrix.CollectorStore, jobName string) {
	t.Helper()
	managed := needCycleManagedStore(t, store)
	managed.CycleController().BeginCycle()
	rt.Collect(store, jobName)
	if err := managed.CycleController().CommitCycleSuccess(); err != nil {
		t.Fatalf("CommitCycleSuccess failed: %v", err)
	}
}

func ciscoConfigTrapEntry(jobName string) *testTrapEntry {
	return &testTrapEntry{
		JobName:       jobName,
		TrapOID:       testCiscoConfigTrapOID,
		TrapName:      "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment: &testTrapEnrichmentAudit{Source: &testTrapSourceAudit{
			Selected: "192.0.2.10",
			Method:   "udp_peer",
		}},
		Varbinds: []testVarbindValue{
			{OID: testCiscoCommandSourceOID, Type: "INTEGER", Value: 2},
			{OID: testCiscoTerminalTypeOID, Type: "INTEGER", Value: 2},
			{OID: model.SysUpTimeOID, Type: "TimeTicks", Value: uint64(12345)},
		},
	}
}

func ciscoConfigTrapEntryFromSource(jobName, source string) *testTrapEntry {
	entry := ciscoConfigTrapEntry(jobName)
	setTrapEntrySource(entry, source)
	return entry
}

func setTrapEntrySource(entry *testTrapEntry, source string) {
	entry.SourceIP = source
	entry.SourceUDPPeer = source
	entry.Enrichment.Source.Selected = source
}

func profileMetricSourceLabels(source string) metrix.Labels {
	entry := ciscoConfigTrapEntryFromSource(testProfileMetricJobName, source)
	sourceID, sourceKind := fallbackTrapSourceIdentity(entry, testProfileMetricJobName, "test")
	return metrix.Labels{"job_name": testProfileMetricJobName, "source_id": sourceID, "source_kind": sourceKind}
}

func profileMetricJobLabels() metrix.Labels {
	return metrix.Labels{"job_name": testProfileMetricJobName}
}

func portSecurityResourceLabels(resourceID string) metrix.Labels {
	labels := profileMetricSourceLabels("192.0.2.10")
	labels["resource_class"] = "interface"
	labels["resource_id"] = resourceID
	return labels
}

func collectProfileMetricStore(t *testing.T, rt *Runtime) metrix.CollectorStore {
	t.Helper()
	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, testProfileMetricJobName)
	return store
}

func assertProfileMetricValue(t *testing.T, store metrix.CollectorStore, metric string, labels metrix.Labels, want float64) {
	t.Helper()
	if v, ok := store.Read().Value(metric, labels); !ok || v != want {
		t.Fatalf("%s = %v/%v, want %v/true", metric, v, ok, want)
	}
}

func assertProfileMetricAbsent(t *testing.T, store metrix.CollectorStore, metric string, labels metrix.Labels) {
	t.Helper()
	if v, ok := store.Read().Value(metric, labels); ok {
		t.Fatalf("%s = %v/true, want metric absent", metric, v)
	}
}

func assertProfileMetricOverflow(t *testing.T, store metrix.CollectorStore, want float64) {
	t.Helper()
	assertProfileMetricValue(t, store, "snmp_trap_profile_metrics_overflow_dropped", profileMetricJobLabels(), want)
}

func sourceRuntimeWithLimits(t *testing.T, idx Catalog, limits profileMetricLimitsPolicy) *Runtime {
	t.Helper()
	return newTestProfileMetricRuntimeWithPolicy(t, idx, testRuntimeConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	}, func(cfg *Policy) {
		if limits.MaxRules != 0 {
			cfg.limits.MaxRules = limits.MaxRules
		}
		if limits.MaxSources != 0 {
			cfg.limits.MaxSources = limits.MaxSources
		}
		if limits.MaxResourcesPerSource != 0 {
			cfg.limits.MaxResourcesPerSource = limits.MaxResourcesPerSource
		}
		if limits.MaxInstancesPerJob != 0 {
			cfg.limits.MaxInstancesPerJob = limits.MaxInstancesPerJob
		}
	})
}

func profileMetricChartForTest(id, title, context, units, algorithm string) profileMetricChart {
	return profileMetricChart{
		ID:         id,
		Title:      title,
		Context:    context,
		Units:      units,
		Algorithm:  algorithm,
		Type:       "line",
		SourceFile: "test-profile.yaml",
	}
}

func addProfileMetricRuleWithChart(t *testing.T, idx *testCatalog, rule profileMetricRule, chart profileMetricChart) {
	t.Helper()
	if err := idx.addDefinitions([]profileMetricRule{rule}, []profileMetricChart{chart}); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
}

func profileMetricCatalogForTest(t *testing.T, idx *testCatalog) profileMetricCatalog {
	t.Helper()
	return profileMetricCatalog{rulesByName: idx.rules, chartsByID: idx.charts}
}

func profileMetricChartFromIndex(t *testing.T, idx *testCatalog, id string) *profileMetricChart {
	t.Helper()
	chart := profileMetricCatalogForTest(t, idx).chartsByID[id]
	require.NotNil(t, chart)
	return chart
}

func profileMetricOutputForTest(metric, dimension, chart string) profileMetricOutput {
	return profileMetricOutput{Metric: metric, Dimension: dimension, Chart: chart}
}

func portSecurityTrapEntry(resource any) *testTrapEntry {
	return &testTrapEntry{
		JobName:       testProfileMetricJobName,
		TrapOID:       testPortSecurityTrapOID,
		TrapName:      "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment:    &testTrapEnrichmentAudit{Source: &testTrapSourceAudit{Selected: "192.0.2.10", Method: "udp_peer"}},
		Varbinds:      []testVarbindValue{{OID: testIfIndexOID, Type: "INTEGER", Value: resource}},
	}
}

func writeProfileYAML(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

func writeProfileCatalogue(t *testing.T, stockDir string, manifest any) {
	t.Helper()
	data, err := json.Marshal(manifest)
	require.NoError(t, err)
	var entries map[string]map[string]any
	require.NoError(t, json.Unmarshal(data, &entries))
	for owner, entry := range entries {
		file, _ := entry["file"].(string)
		profileData, err := os.ReadFile(filepath.Join(stockDir, file))
		require.NoError(t, err, "read stock profile for catalogue entry %q", owner)
		entry["sha256"] = fmt.Sprintf("%x", sha256.Sum256(profileData))
	}
	data, err = json.Marshal(entries)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(stockDir), "catalogue.json"), data, 0o644))
}
