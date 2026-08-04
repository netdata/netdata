// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/attribution"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

type testRuntimeConfig struct {
	Enabled bool
	Include []string
}

type testTrapEntry = model.TrapEntry
type testTrapDef = catalog.TrapDef
type testVarbindDef = catalog.VarbindDef
type testMetricRule = catalog.MetricRule
type testMetricChart = catalog.MetricChart

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

	profileMetricIdentitySource      = catalog.MetricIdentitySource
	profileMetricIdentitySourceLabel = catalog.MetricIdentitySourceLabel
	profileMetricIdentityListener    = catalog.MetricIdentityListener

	profileMetricMissingDrop             = catalog.MetricMissingDrop
	profileMetricMissingZero             = catalog.MetricMissingZero
	profileMetricMissingUnknownDimension = catalog.MetricMissingUnknownDimension
	profileMetricMissingError            = catalog.MetricMissingError
)

// staticTestCatalog is an immutable, exact-reference implementation of the
// catalog contract consumed by profilemetrics. Catalog parsing, validation,
// normalization, and lazy stock resolution are intentionally absent.
type staticTestCatalog struct {
	rulesByName map[string]*catalog.MetricRule
	chartsByID  map[string]*catalog.MetricChart
	trapsByRef  map[string]*catalog.TrapDef
}

func newStaticTestCatalog(traps []*testTrapDef, rules []testMetricRule, charts []testMetricChart) *staticTestCatalog {
	idx := &staticTestCatalog{
		rulesByName: make(map[string]*catalog.MetricRule, len(rules)),
		chartsByID:  make(map[string]*catalog.MetricChart, len(charts)),
		trapsByRef:  make(map[string]*catalog.TrapDef, len(traps)*2),
	}
	return idx.withTraps(traps...).withDefinitions(rules, charts)
}

func (idx *staticTestCatalog) Definitions(names []string) (catalog.MetricDefinitions, error) {
	if idx == nil {
		return catalog.MetricDefinitions{}, fmt.Errorf("profile index not available")
	}
	defs := catalog.MetricDefinitions{
		RulesByName: make(map[string]*catalog.MetricRule, len(names)),
		ChartsByID:  make(map[string]*catalog.MetricChart, len(names)),
	}
	for _, name := range names {
		rule := idx.rulesByName[name]
		if rule == nil {
			continue
		}
		defs.RulesByName[name] = rule
		if chart := idx.chartsByID[rule.Output.Chart]; chart != nil {
			defs.ChartsByID[chart.ID] = chart
		}
	}
	return defs, nil
}

func (idx *staticTestCatalog) ResolveTrap(ref string) (*catalog.TrapDef, error) {
	if idx == nil {
		return nil, fmt.Errorf("profile index not available")
	}
	trap := idx.trapsByRef[ref]
	if trap == nil {
		return nil, fmt.Errorf("trap %q not found", ref)
	}
	return trap, nil
}

func (idx *staticTestCatalog) withDefinitions(rules []testMetricRule, charts []testMetricChart) *staticTestCatalog {
	next := idx.clone()
	for i := range rules {
		rule := cloneTestMetricRule(&rules[i])
		next.rulesByName[rule.Name] = rule
	}
	for i := range charts {
		chart := cloneTestMetricChart(&charts[i])
		next.chartsByID[chart.ID] = chart
	}
	return next
}

func (idx *staticTestCatalog) withTraps(traps ...*testTrapDef) *staticTestCatalog {
	next := idx.clone()
	for _, trap := range traps {
		if trap == nil {
			continue
		}
		trap = cloneTestTrapDef(trap)
		next.trapsByRef[trap.OID] = trap
		next.trapsByRef[trap.Name] = trap
	}
	return next
}

func (idx *staticTestCatalog) withChartLifecycle(id string, lifecycle charttpl.Lifecycle) *staticTestCatalog {
	next := idx.clone()
	chart := cloneTestMetricChart(next.chartsByID[id])
	if chart == nil {
		panic(fmt.Sprintf("profile metric chart %q not found", id))
	}
	chart.Lifecycle = &lifecycle
	next.chartsByID[id] = chart
	return next
}

func (idx *staticTestCatalog) clone() *staticTestCatalog {
	if idx == nil {
		return &staticTestCatalog{
			rulesByName: make(map[string]*catalog.MetricRule),
			chartsByID:  make(map[string]*catalog.MetricChart),
			trapsByRef:  make(map[string]*catalog.TrapDef),
		}
	}
	next := &staticTestCatalog{
		rulesByName: make(map[string]*catalog.MetricRule, len(idx.rulesByName)),
		chartsByID:  make(map[string]*catalog.MetricChart, len(idx.chartsByID)),
		trapsByRef:  make(map[string]*catalog.TrapDef, len(idx.trapsByRef)),
	}
	for name, rule := range idx.rulesByName {
		next.rulesByName[name] = rule
	}
	for id, chart := range idx.chartsByID {
		next.chartsByID[id] = chart
	}
	for ref, trap := range idx.trapsByRef {
		next.trapsByRef[ref] = trap
	}
	return next
}

func cloneTestMetricRule(src *testMetricRule) *testMetricRule {
	if src == nil {
		return nil
	}
	dst := *src
	if src.Identity.Resource != nil {
		resource := *src.Identity.Resource
		dst.Identity.Resource = &resource
	}
	if src.Where != nil {
		dst.Where = append(profileMetricPredicates(nil), src.Where...)
		for i := range dst.Where {
			dst.Where[i].In = append([]any(nil), src.Where[i].In...)
			dst.Where[i].Range = append([]any(nil), src.Where[i].Range...)
		}
	}
	if src.State.SetWhen != nil {
		pred := *src.State.SetWhen
		dst.State.SetWhen = &pred
	}
	if src.State.ClearWhen != nil {
		pred := *src.State.ClearWhen
		dst.State.ClearWhen = &pred
	}
	if src.State.ProblemValue != nil {
		value := *src.State.ProblemValue
		dst.State.ProblemValue = &value
	}
	return &dst
}

func cloneTestMetricChart(src *testMetricChart) *testMetricChart {
	if src == nil {
		return nil
	}
	dst := *src
	if src.Lifecycle != nil {
		lifecycle := *src.Lifecycle
		dst.Lifecycle = &lifecycle
	}
	return &dst
}

func cloneTestTrapDef(src *testTrapDef) *testTrapDef {
	if src == nil {
		return nil
	}
	dst := *src
	dst.VarbindRefs = append([]any(nil), src.VarbindRefs...)
	dst.DedupKeyVarbinds = append([]string(nil), src.DedupKeyVarbinds...)
	if src.Labels != nil {
		dst.Labels = make(map[string]string, len(src.Labels))
		for key, value := range src.Labels {
			dst.Labels[key] = value
		}
	}
	if src.SharedVarbinds != nil {
		dst.SharedVarbinds = make(map[string]*catalog.VarbindDef, len(src.SharedVarbinds))
		for oid, def := range src.SharedVarbinds {
			if def == nil {
				dst.SharedVarbinds[oid] = nil
				continue
			}
			cp := *def
			if def.Enum != nil {
				cp.Enum = make(map[string]string, len(def.Enum))
				for key, value := range def.Enum {
					cp.Enum[key] = value
				}
			}
			dst.SharedVarbinds[oid] = &cp
		}
	}
	return &dst
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

func newPopulatedTestCatalog(t *testing.T) *staticTestCatalog {
	t.Helper()
	commandSource := &testVarbindDef{
		OID:         testCiscoCommandSourceOID,
		Type:        "INTEGER",
		Constraints: "(1..4)",
		RawName:     testCiscoCommandSourceVarbind,
	}
	terminalType := &testVarbindDef{
		OID:  testCiscoTerminalTypeOID,
		Type: "INTEGER",
		Enum: map[string]string{
			"1": "none",
			"2": "console",
			"3": "virtual",
			"4": "aux",
		},
		RawName: testCiscoTerminalTypeVarbind,
	}
	sysUpTime := &testVarbindDef{OID: model.SysUpTimeOID, Type: "TimeTicks", RawName: "sysUpTime.0"}
	ifIndex := &testVarbindDef{
		OID:         testIfIndexOID,
		Type:        "INTEGER",
		Constraints: "(1..48)",
		RawName:     "ifIndex",
	}
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
				testCiscoCommandSourceOID: commandSource,
				testCiscoTerminalTypeOID:  terminalType,
				model.SysUpTimeOID:        sysUpTime,
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
			SharedVarbinds: map[string]*testVarbindDef{testIfIndexOID: ifIndex},
		},
		{OID: testLinkDownTrapOID, Name: "IF-MIB::linkDown", Category: "state_change", Severity: "warning"},
	}
	charts := []testMetricChart{
		{
			ID:        "cisco_config_changes",
			Title:     "Cisco config changes",
			Context:   "snmp.trap.cisco.config.changes",
			Units:     "events/s",
			Algorithm: "incremental",
			Type:      "line",
			Lifecycle: &charttpl.Lifecycle{
				MaxInstances:      catalog.DefaultMetricChartMaxInstances,
				ExpireAfterCycles: catalog.DefaultMetricExpireAfterCycles,
			},
		},
		{
			ID:        "cisco_terminal_type",
			Title:     "Cisco terminal type",
			Context:   "snmp.trap.cisco.terminal.type",
			Units:     "type",
			Algorithm: "absolute",
			Type:      "line",
			Lifecycle: &charttpl.Lifecycle{
				MaxInstances:      catalog.DefaultMetricChartMaxInstances,
				ExpireAfterCycles: catalog.DefaultMetricExpireAfterCycles,
			},
		},
		{
			ID:        "port_security_violations",
			Title:     "Port security violations",
			Context:   "snmp.trap.cisco.port.security.violations",
			Units:     "events/s",
			Algorithm: "incremental",
			Type:      "line",
			Lifecycle: &charttpl.Lifecycle{
				MaxInstances:      catalog.DefaultMetricChartMaxInstances,
				ExpireAfterCycles: catalog.DefaultMetricExpireAfterCycles,
			},
		},
	}
	rules := []testMetricRule{
		{
			Name:   "cisco.config.changed",
			Type:   profileMetricTypeCounter,
			OnTrap: "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
			Identity: profileMetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_config_events",
				Dimension: "events",
				Chart:     "cisco_config_changes",
			},
			Missing: profileMetricMissingDrop,
			Scale:   profileMetricScale{Multiplier: 1, Divisor: 1},
		},
		{
			Name:             "cisco.config.terminal_type",
			Type:             profileMetricTypeSample,
			OnTrap:           testCiscoConfigTrapOID,
			ValueFromVarbind: testCiscoTerminalTypeVarbind,
			Identity: profileMetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_terminal_type",
				Dimension: "terminal_type",
				Chart:     "cisco_terminal_type",
			},
			Missing: profileMetricMissingDrop,
			Scale:   profileMetricScale{Multiplier: 1, Divisor: 1},
		},
		{
			Name:   "cisco.port_security.ifindex",
			Type:   profileMetricTypeCounter,
			OnTrap: testPortSecurityTrapOID,
			Identity: profileMetricIdentity{
				Device:   catalog.MetricIdentitySource,
				Resource: &profileMetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 48},
			},
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_port_security_violations",
				Dimension: "violations",
				Chart:     "port_security_violations",
			},
			Missing: profileMetricMissingDrop,
			Scale:   profileMetricScale{Multiplier: 1, Divisor: 1},
		},
	}
	return newStaticTestCatalog(traps, rules, charts)
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

func newTestProfileMetricRuntimeWithPolicy(
	t *testing.T,
	idx Catalog,
	cfg testRuntimeConfig,
	configure func(*Policy),
) *Runtime {
	t.Helper()
	return newTestProfileMetricRuntimeFromCatalog(t, idx, cfg, configure)
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

func profileMetricChartForTest(id, title, context, units, algorithm string) testMetricChart {
	return testMetricChart{
		ID:        id,
		Title:     title,
		Context:   context,
		Units:     units,
		Algorithm: algorithm,
		Type:      "line",
		Lifecycle: &charttpl.Lifecycle{
			MaxInstances:      catalog.DefaultMetricChartMaxInstances,
			ExpireAfterCycles: catalog.DefaultMetricExpireAfterCycles,
		},
	}
}

func addProfileMetricRuleWithChart(idx *staticTestCatalog, rule testMetricRule, chart testMetricChart) *staticTestCatalog {
	return idx.withDefinitions([]testMetricRule{rule}, []testMetricChart{chart})
}

func profileMetricCatalogForTest(idx *staticTestCatalog) profileMetricCatalog {
	return profileMetricCatalog{rulesByName: idx.rulesByName, chartsByID: idx.chartsByID}
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
