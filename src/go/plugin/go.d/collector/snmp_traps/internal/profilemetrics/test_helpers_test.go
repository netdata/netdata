// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/attribution"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/profiletest"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

type testRuntimeConfig struct {
	Enabled bool
	Include []string
}

// testProfileBuilder assembles source-profile data. Build is the only path
// from mutable fixture data to a live catalog.
type testProfileBuilder struct {
	tb           testing.TB
	paths        catalog.Paths
	fileVarbinds []testFileVarbind
	traps        []*testTrapDef
	rules        []testMetricRule
	charts       []testMetricChart
}
type testTrapEntry = model.TrapEntry

type testTrapDef struct {
	OID              string            `yaml:"oid"`
	Name             string            `yaml:"name"`
	Category         string            `yaml:"category"`
	Severity         string            `yaml:"severity"`
	Description      string            `yaml:"description,omitempty"`
	Status           string            `yaml:"status,omitempty"`
	Varbinds         []any             `yaml:"varbinds,omitempty"`
	Labels           map[string]string `yaml:"labels,omitempty"`
	DedupKeyVarbinds []string          `yaml:"dedup_key_varbinds,omitempty"`
}

type testFileVarbind struct {
	Name       string
	Definition *testVarbindDef
}

type testVarbindDef struct {
	OID         string            `yaml:"oid"`
	Type        string            `yaml:"type"`
	Enum        map[string]string `yaml:"enum,omitempty"`
	Constraints string            `yaml:"constraints,omitempty"`
}

type testMetricRule struct {
	Name        string `yaml:"name"`
	Type        string `yaml:"type"`
	Enabled     *bool  `yaml:"enabled,omitempty"`
	OnTrap      string `yaml:"on_trap,omitempty"`
	ProblemTrap string `yaml:"problem_trap,omitempty"`
	ClearTrap   string `yaml:"clear_trap,omitempty"`

	Where profileMetricPredicates `yaml:"where,omitempty"`

	Identity profileMetricIdentity `yaml:"identity,omitempty"`
	Output   profileMetricOutput   `yaml:"output,omitempty"`
	State    profileMetricState    `yaml:"state,omitempty"`
	Scale    profileMetricScale    `yaml:"scale,omitempty"`

	Missing          string `yaml:"missing,omitempty"`
	ValueFromVarbind string `yaml:"value_from_varbind,omitempty"`
}

type testMetricChart struct {
	ID          string              `yaml:"id"`
	Title       string              `yaml:"title"`
	Family      string              `yaml:"family,omitempty"`
	Context     string              `yaml:"context"`
	Units       string              `yaml:"units"`
	Algorithm   string              `yaml:"algorithm,omitempty"`
	Type        string              `yaml:"type,omitempty"`
	Description string              `yaml:"description,omitempty"`
	Lifecycle   *charttpl.Lifecycle `yaml:"lifecycle,omitempty"`
}

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

func newTestProfileBuilder(t testing.TB) *testProfileBuilder {
	t.Helper()
	return &testProfileBuilder{
		tb:    t,
		paths: profiletest.CatalogPaths(t),
	}
}

func (b *testProfileBuilder) addTraps(traps []*testTrapDef, fileVarbinds ...testFileVarbind) error {
	candidateVarbinds := append(append([]testFileVarbind(nil), b.fileVarbinds...), fileVarbinds...)
	candidateTraps := append(append([]*testTrapDef(nil), b.traps...), traps...)
	lease, err := loadTestCatalogEpoch(b.tb, b.paths, candidateVarbinds, candidateTraps, b.rules, b.charts)
	if err != nil {
		return err
	}
	lease.Close()
	b.fileVarbinds = candidateVarbinds
	b.traps = candidateTraps
	return nil
}

func (b *testProfileBuilder) addDefinitions(rules []testMetricRule, charts []testMetricChart) error {
	candidateRules := append(append([]testMetricRule(nil), b.rules...), rules...)
	candidateCharts := append(append([]testMetricChart(nil), b.charts...), charts...)
	lease, err := loadTestCatalogEpoch(b.tb, b.paths, b.fileVarbinds, b.traps, candidateRules, candidateCharts)
	if err != nil {
		return err
	}
	lease.Close()
	b.rules = candidateRules
	b.charts = candidateCharts
	return nil
}

func (b *testProfileBuilder) Build() *catalog.Epoch {
	b.tb.Helper()
	lease, err := loadTestCatalogEpoch(b.tb, b.paths, b.fileVarbinds, b.traps, b.rules, b.charts)
	if err != nil {
		b.tb.Fatalf("build test profile catalog: %v", err)
	}
	b.tb.Cleanup(lease.Close)
	return lease.Epoch()
}

func loadTestCatalogEpoch(
	t testing.TB,
	paths catalog.Paths,
	fileVarbinds []testFileVarbind,
	traps []*testTrapDef,
	rules []testMetricRule,
	charts []testMetricChart,
) (*catalog.Lease, error) {
	t.Helper()
	data, err := marshalTestCatalogSource(fileVarbinds, traps, rules, charts)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(paths.UserDirs[0], "test-traps.yaml"), data, 0o600); err != nil {
		return nil, fmt.Errorf("write test trap profile: %w", err)
	}
	return catalog.NewManager(paths).Acquire()
}

func marshalTestCatalogSource(
	fileVarbinds []testFileVarbind,
	traps []*testTrapDef,
	rules []testMetricRule,
	charts []testMetricChart,
) ([]byte, error) {
	profile := struct {
		Varbinds yaml.MapSlice     `yaml:"varbinds,omitempty"`
		Traps    []*testTrapDef    `yaml:"traps"`
		Charts   []testMetricChart `yaml:"charts,omitempty"`
		Metrics  []testMetricRule  `yaml:"metrics,omitempty"`
	}{
		Traps:   make([]*testTrapDef, 0, len(traps)),
		Charts:  append([]testMetricChart(nil), charts...),
		Metrics: append([]testMetricRule(nil), rules...),
	}
	for _, vb := range fileVarbinds {
		profile.Varbinds = append(profile.Varbinds, yaml.MapItem{Key: vb.Name, Value: vb.Definition})
	}
	for _, src := range traps {
		if src == nil {
			profile.Traps = append(profile.Traps, nil)
			continue
		}
		profile.Traps = append(profile.Traps, src)
	}
	data, err := yaml.Marshal(profile)
	if err != nil {
		return nil, fmt.Errorf("marshal test trap profile: %w", err)
	}
	return data, nil
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

func newPopulatedTestProfile(t *testing.T) *testProfileBuilder {
	t.Helper()
	idx := newTestProfileBuilder(t)
	fileVarbinds := []testFileVarbind{
		{Name: testCiscoCommandSourceVarbind, Definition: &testVarbindDef{
			OID:         testCiscoCommandSourceOID,
			Type:        "INTEGER",
			Constraints: "(1..4)",
		}},
		{Name: testCiscoTerminalTypeVarbind, Definition: &testVarbindDef{
			OID:  testCiscoTerminalTypeOID,
			Type: "INTEGER",
			Enum: map[string]string{
				"1": "none",
				"2": "console",
				"3": "virtual",
				"4": "aux",
			},
		}},
		{Name: "sysUpTime.0", Definition: &testVarbindDef{OID: model.SysUpTimeOID, Type: "TimeTicks"}},
		{Name: "ifIndex", Definition: &testVarbindDef{
			OID:         testIfIndexOID,
			Type:        "INTEGER",
			Constraints: "(1..48)",
		}},
	}
	traps := []*testTrapDef{
		{
			OID:      testCiscoConfigTrapOID,
			Name:     "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
			Category: "config_change",
			Severity: "notice",
			Varbinds: []any{
				testCiscoCommandSourceVarbind,
				testCiscoTerminalTypeVarbind,
				"sysUpTime.0",
			},
		},
		{
			OID:      testPortSecurityTrapOID,
			Name:     "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
			Category: "security",
			Severity: "warning",
			Varbinds: []any{
				"ifIndex",
			},
		},
	}
	if err := idx.addTraps(traps, fileVarbinds...); err != nil {
		t.Fatalf("addTraps failed: %v", err)
	}
	charts := []testMetricChart{
		{
			ID:        "cisco_config_changes",
			Title:     "Cisco config changes",
			Context:   "snmp.trap.cisco.config.changes",
			Units:     "events/s",
			Algorithm: "incremental",
			Type:      "line",
		},
		{
			ID:        "cisco_terminal_type",
			Title:     "Cisco terminal type",
			Context:   "snmp.trap.cisco.terminal.type",
			Units:     "type",
			Algorithm: "absolute",
			Type:      "line",
		},
		{
			ID:        "port_security_violations",
			Title:     "Port security violations",
			Context:   "snmp.trap.cisco.port.security.violations",
			Units:     "events/s",
			Algorithm: "incremental",
			Type:      "line",
		},
	}
	rules := []testMetricRule{
		{
			Name:   "cisco.config.changed",
			Type:   profileMetricTypeCounter,
			OnTrap: "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_config_events",
				Dimension: "events",
				Chart:     "cisco_config_changes",
			},
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
		},
	}
	if err := idx.addDefinitions(rules, charts); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	return idx
}

func newTestProfileMetricRuntime(t *testing.T, profile *testProfileBuilder, include []string) *Runtime {
	t.Helper()
	return newTestProfileMetricRuntimeWithConfig(t, profile, testRuntimeConfig{
		Enabled: true,
		Include: include,
	})
}

func newTestProfileMetricRuntimeWithConfig(t *testing.T, profile *testProfileBuilder, cfg testRuntimeConfig) *Runtime {
	t.Helper()
	return newTestProfileMetricRuntimeWithPolicy(t, profile, cfg, nil)
}

func newTestProfileMetricRuntimeWithPolicy(
	t *testing.T,
	profile *testProfileBuilder,
	cfg testRuntimeConfig,
	configure func(*Policy),
) *Runtime {
	t.Helper()
	return newTestProfileMetricRuntimeFromCatalog(t, profile.Build(), cfg, configure)
}

func newTestProfileMetricRuntimeFromCatalogWithConfig(t *testing.T, idx Catalog, cfg testRuntimeConfig) *Runtime {
	t.Helper()
	return newTestProfileMetricRuntimeFromCatalog(t, idx, cfg, nil)
}

func newTestProfileMetricRuntimeFromCatalog(
	t *testing.T,
	idx Catalog,
	cfg testRuntimeConfig,
	configure func(*Policy),
) *Runtime {
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

func sourceRuntimeWithLimits(t *testing.T, idx *testProfileBuilder, limits profileMetricLimitsPolicy) *Runtime {
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

func profileMetricChartForTest(id, title, context, units, algorithm string) testMetricChart {
	return testMetricChart{
		ID:        id,
		Title:     title,
		Context:   context,
		Units:     units,
		Algorithm: algorithm,
		Type:      "line",
	}
}

func addProfileMetricRuleWithChart(t *testing.T, idx *testProfileBuilder, rule testMetricRule, chart testMetricChart) {
	t.Helper()
	if err := idx.addDefinitions([]testMetricRule{rule}, []testMetricChart{chart}); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
}

func profileMetricCatalogForTest(t *testing.T, idx *testProfileBuilder) profileMetricCatalog {
	t.Helper()
	names := make([]string, 0, len(idx.rules))
	for i := range idx.rules {
		names = append(names, idx.rules[i].Name)
	}
	defs, err := idx.Build().Definitions(names)
	require.NoError(t, err)
	return profileMetricCatalog{rulesByName: defs.RulesByName, chartsByID: defs.ChartsByID}
}

func profileMetricChartFromIndex(t *testing.T, idx *testProfileBuilder, id string) *testMetricChart {
	t.Helper()
	for i := range idx.charts {
		if idx.charts[i].ID == id {
			return &idx.charts[i]
		}
	}
	t.Fatalf("profile metric chart %q not found", id)
	return nil
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
