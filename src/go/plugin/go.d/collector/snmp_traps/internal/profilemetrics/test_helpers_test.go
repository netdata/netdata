// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"fmt"
	"maps"
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

// staticTestCatalog is an immutable, exact-reference implementation of the
// catalog contract consumed by profilemetrics. Catalog parsing, validation,
// normalization, and lazy stock resolution are intentionally absent.
type staticTestCatalog struct {
	rulesByName map[string]*catalog.MetricRule
	chartsByID  map[string]*catalog.MetricChart
	trapsByRef  map[string]*catalog.TrapDef
}

func newStaticTestCatalog(traps []*catalog.TrapDef, rules []catalog.MetricRule, charts []catalog.MetricChart) *staticTestCatalog {
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

func (idx *staticTestCatalog) withDefinitions(rules []catalog.MetricRule, charts []catalog.MetricChart) *staticTestCatalog {
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

func (idx *staticTestCatalog) withTraps(traps ...*catalog.TrapDef) *staticTestCatalog {
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
	chart.Lifecycle = cloneTestChartLifecycle(&lifecycle)
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
	maps.Copy(next.rulesByName, idx.rulesByName)
	maps.Copy(next.chartsByID, idx.chartsByID)
	maps.Copy(next.trapsByRef, idx.trapsByRef)
	return next
}

func cloneTestMetricRule(src *catalog.MetricRule) *catalog.MetricRule {
	if src == nil {
		return nil
	}
	dst := *src
	if src.Enabled != nil {
		enabled := *src.Enabled
		dst.Enabled = &enabled
	}
	if src.Identity.Resource != nil {
		resource := *src.Identity.Resource
		dst.Identity.Resource = &resource
	}
	if src.Where != nil {
		dst.Where = make(catalog.MetricPredicates, len(src.Where))
		for i := range src.Where {
			dst.Where[i] = cloneTestMetricPredicate(src.Where[i])
		}
	}
	if src.State.SetWhen != nil {
		pred := cloneTestMetricPredicate(*src.State.SetWhen)
		dst.State.SetWhen = &pred
	}
	if src.State.ClearWhen != nil {
		pred := cloneTestMetricPredicate(*src.State.ClearWhen)
		dst.State.ClearWhen = &pred
	}
	if src.State.ProblemValue != nil {
		value := *src.State.ProblemValue
		dst.State.ProblemValue = &value
	}
	return &dst
}

func cloneTestMetricPredicate(src catalog.MetricPredicate) catalog.MetricPredicate {
	dst := src
	dst.Equals = cloneTestCatalogValue(src.Equals)
	dst.In = cloneTestCatalogValues(src.In)
	dst.GreaterThan = cloneTestCatalogValue(src.GreaterThan)
	dst.LessThan = cloneTestCatalogValue(src.LessThan)
	dst.Range = cloneTestCatalogValues(src.Range)
	if src.Exists != nil {
		exists := *src.Exists
		dst.Exists = &exists
	}
	if src.Absent != nil {
		absent := *src.Absent
		dst.Absent = &absent
	}
	return dst
}

func cloneTestCatalogValues(src []any) []any {
	if src == nil {
		return nil
	}
	dst := make([]any, len(src))
	for i := range src {
		dst[i] = cloneTestCatalogValue(src[i])
	}
	return dst
}

func cloneTestCatalogValue(src any) any {
	switch value := src.(type) {
	case []any:
		return cloneTestCatalogValues(value)
	case map[any]any:
		dst := make(map[any]any, len(value))
		for key, item := range value {
			dst[cloneTestCatalogValue(key)] = cloneTestCatalogValue(item)
		}
		return dst
	case map[string]any:
		dst := make(map[string]any, len(value))
		for key, item := range value {
			dst[key] = cloneTestCatalogValue(item)
		}
		return dst
	default:
		return src
	}
}

func cloneTestMetricChart(src *catalog.MetricChart) *catalog.MetricChart {
	if src == nil {
		return nil
	}
	dst := *src
	dst.Lifecycle = cloneTestChartLifecycle(src.Lifecycle)
	return &dst
}

func cloneTestChartLifecycle(src *charttpl.Lifecycle) *charttpl.Lifecycle {
	if src == nil {
		return nil
	}
	dst := *src
	if src.Dimensions != nil {
		dimensions := *src.Dimensions
		dst.Dimensions = &dimensions
	}
	return &dst
}

func cloneTestTrapDef(src *catalog.TrapDef) *catalog.TrapDef {
	if src == nil {
		return nil
	}
	dst := *src
	dst.VarbindRefs = cloneTestCatalogValues(src.VarbindRefs)
	dst.DedupKeyVarbinds = append([]string(nil), src.DedupKeyVarbinds...)
	if src.Labels != nil {
		dst.Labels = make(map[string]string, len(src.Labels))
		maps.Copy(dst.Labels, src.Labels)
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
				maps.Copy(cp.Enum, def.Enum)
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

func fallbackTrapSourceIdentity(entry *model.TrapEntry, jobName, sourceHashSalt string) (string, string) {
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
	commandSource := &catalog.VarbindDef{
		OID:         testCiscoCommandSourceOID,
		Type:        "INTEGER",
		Constraints: "(1..4)",
		RawName:     testCiscoCommandSourceVarbind,
	}
	terminalType := &catalog.VarbindDef{
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
	sysUpTime := &catalog.VarbindDef{OID: model.SysUpTimeOID, Type: "TimeTicks", RawName: "sysUpTime.0"}
	ifIndex := &catalog.VarbindDef{
		OID:         testIfIndexOID,
		Type:        "INTEGER",
		Constraints: "(1..48)",
		RawName:     "ifIndex",
	}
	traps := []*catalog.TrapDef{
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
			SharedVarbinds: map[string]*catalog.VarbindDef{
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
			SharedVarbinds: map[string]*catalog.VarbindDef{testIfIndexOID: ifIndex},
		},
		{OID: testLinkDownTrapOID, Name: "IF-MIB::linkDown", Category: "state_change", Severity: "warning"},
	}
	charts := []catalog.MetricChart{
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
	rules := []catalog.MetricRule{
		{
			Name:   "cisco.config.changed",
			Type:   catalog.MetricTypeCounter,
			OnTrap: "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			Output: catalog.MetricOutput{
				Metric:    "snmp_trap_cisco_config_events",
				Dimension: "events",
				Chart:     "cisco_config_changes",
			},
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
		{
			Name:             "cisco.config.terminal_type",
			Type:             catalog.MetricTypeSample,
			OnTrap:           testCiscoConfigTrapOID,
			ValueFromVarbind: testCiscoTerminalTypeVarbind,
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			Output: catalog.MetricOutput{
				Metric:    "snmp_trap_cisco_terminal_type",
				Dimension: "terminal_type",
				Chart:     "cisco_terminal_type",
			},
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
		{
			Name:   "cisco.port_security.ifindex",
			Type:   catalog.MetricTypeCounter,
			OnTrap: testPortSecurityTrapOID,
			Identity: catalog.MetricIdentity{
				Device:   catalog.MetricIdentitySource,
				Resource: &catalog.MetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 48},
			},
			Output: catalog.MetricOutput{
				Metric:    "snmp_trap_cisco_port_security_violations",
				Dimension: "violations",
				Chart:     "port_security_violations",
			},
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
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

func ciscoConfigTrapEntry(jobName string) *model.TrapEntry {
	return &model.TrapEntry{
		JobName:       jobName,
		TrapOID:       testCiscoConfigTrapOID,
		TrapName:      "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment: &model.TrapEnrichmentAudit{Source: &model.TrapSourceAudit{
			Selected: "192.0.2.10",
			Method:   "udp_peer",
		}},
		Varbinds: []model.VarbindValue{
			{OID: testCiscoCommandSourceOID, Type: "INTEGER", Value: 2},
			{OID: testCiscoTerminalTypeOID, Type: "INTEGER", Value: 2},
			{OID: model.SysUpTimeOID, Type: "TimeTicks", Value: uint64(12345)},
		},
	}
}

func ciscoConfigTrapEntryFromSource(jobName, source string) *model.TrapEntry {
	entry := ciscoConfigTrapEntry(jobName)
	setTrapEntrySource(entry, source)
	return entry
}

func setTrapEntrySource(entry *model.TrapEntry, source string) {
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

func profileMetricChartForTest(id, title, context, units, algorithm string) catalog.MetricChart {
	return catalog.MetricChart{
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

func addProfileMetricRuleWithChart(idx *staticTestCatalog, rule catalog.MetricRule, chart catalog.MetricChart) *staticTestCatalog {
	return idx.withDefinitions([]catalog.MetricRule{rule}, []catalog.MetricChart{chart})
}

func profileMetricCatalogForTest(idx *staticTestCatalog) profileMetricCatalog {
	return profileMetricCatalog{rulesByName: idx.rulesByName, chartsByID: idx.chartsByID}
}

func profileMetricOutputForTest(metric, dimension, chart string) catalog.MetricOutput {
	return catalog.MetricOutput{Metric: metric, Dimension: dimension, Chart: chart}
}

func portSecurityTrapEntry(resource any) *model.TrapEntry {
	return &model.TrapEntry{
		JobName:       testProfileMetricJobName,
		TrapOID:       testPortSecurityTrapOID,
		TrapName:      "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment:    &model.TrapEnrichmentAudit{Source: &model.TrapSourceAudit{Selected: "192.0.2.10", Method: "udp_peer"}},
		Varbinds:      []model.VarbindValue{{OID: testIfIndexOID, Type: "INTEGER", Value: resource}},
	}
}
