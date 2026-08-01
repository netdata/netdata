// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"encoding/hex"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

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

func testProfileMetricIndex(t *testing.T) *ProfileIndex {
	t.Helper()
	idx := &ProfileIndex{
		trapsByOID:      make(map[string]*TrapDef),
		namesByTrapName: make(map[string]*TrapDef),
	}
	traps := []*TrapDef{
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
			sharedVarbinds: map[string]*VarbindDef{
				testCiscoCommandSourceOID: {
					OID:         testCiscoCommandSourceOID,
					Type:        "INTEGER",
					rawName:     testCiscoCommandSourceVarbind,
					Constraints: "(1..4)",
				},
				testCiscoTerminalTypeOID: {
					OID:     testCiscoTerminalTypeOID,
					Type:    "INTEGER",
					rawName: testCiscoTerminalTypeVarbind,
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
					rawName: "sysUpTime.0",
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
			sharedVarbinds: map[string]*VarbindDef{
				testIfIndexOID: {
					OID:         testIfIndexOID,
					Type:        "INTEGER",
					rawName:     "ifIndex",
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
			sourceFile: "test-profile.yaml",
		},
		{
			ID:         "cisco_terminal_type",
			Title:      "Cisco terminal type",
			Context:    "snmp.trap.cisco.terminal.type",
			Units:      "type",
			Algorithm:  "absolute",
			Type:       "line",
			sourceFile: "test-profile.yaml",
		},
		{
			ID:         "port_security_violations",
			Title:      "Port security violations",
			Context:    "snmp.trap.cisco.port.security.violations",
			Units:      "events/s",
			Algorithm:  "incremental",
			Type:       "line",
			sourceFile: "test-profile.yaml",
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
			sourceFile: "test-profile.yaml",
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
			sourceFile: "test-profile.yaml",
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
			sourceFile: "test-profile.yaml",
		},
	}
	if err := idx.addProfileMetrics(rules, charts); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	return idx
}

func newTestProfileMetricRuntime(t *testing.T, idx *ProfileIndex, include []string) *profileMetricRuntime {
	t.Helper()
	return newTestProfileMetricRuntimeWithConfig(t, idx, ProfileMetricsConfig{
		Enabled: true,
		Include: include,
	})
}

func newTestProfileMetricRuntimeWithConfig(t *testing.T, idx *ProfileIndex, cfg ProfileMetricsConfig) *profileMetricRuntime {
	t.Helper()
	return newTestProfileMetricRuntimeWithPolicy(t, idx, cfg, nil)
}

func newTestProfileMetricRuntimeWithPolicy(t *testing.T, idx *ProfileIndex, cfg ProfileMetricsConfig, configure func(*normalizedProfileMetricsConfig)) *profileMetricRuntime {
	t.Helper()
	normalized, err := normalizeProfileMetricsConfig(cfg)
	if err != nil {
		t.Fatalf("normalizeProfileMetricsConfig failed: %v", err)
	}
	if configure != nil {
		configure(&normalized)
	}
	rt, tmpl, err := newProfileMetricRuntime(normalized, idx, "test")
	if err != nil {
		t.Fatalf("newProfileMetricRuntime failed: %v", err)
	}
	if rt == nil {
		t.Fatalf("newProfileMetricRuntime returned nil runtime")
	}
	collecttest.AssertChartTemplateSchema(t, tmpl)
	return rt
}

func collectProfileMetricsOnce(t *testing.T, rt *profileMetricRuntime, store metrix.CollectorStore, jobName string) {
	t.Helper()
	managed := needCycleManagedStore(t, store)
	managed.CycleController().BeginCycle()
	rt.collect(store, jobName)
	if err := managed.CycleController().CommitCycleSuccess(); err != nil {
		t.Fatalf("CommitCycleSuccess failed: %v", err)
	}
}

func ciscoConfigTrapEntry(jobName string) *TrapEntry {
	return &TrapEntry{
		JobName:       jobName,
		TrapOID:       testCiscoConfigTrapOID,
		TrapName:      "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment: &TrapEnrichmentAudit{Source: &TrapSourceAudit{
			Selected: "192.0.2.10",
			Method:   "udp_peer",
		}},
		Varbinds: []VarbindValue{
			{OID: testCiscoCommandSourceOID, Type: "INTEGER", Value: 2},
			{OID: testCiscoTerminalTypeOID, Type: "INTEGER", Value: 2},
			{OID: model.SysUpTimeOID, Type: "TimeTicks", Value: uint64(12345)},
		},
	}
}

func ciscoConfigTrapEntryFromSource(jobName, source string) *TrapEntry {
	entry := ciscoConfigTrapEntry(jobName)
	setTrapEntrySource(entry, source)
	return entry
}

func setTrapEntrySource(entry *TrapEntry, source string) {
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

func collectProfileMetricStore(t *testing.T, rt *profileMetricRuntime) metrix.CollectorStore {
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

func sourceRuntimeWithLimits(t *testing.T, idx *ProfileIndex, limits profileMetricLimitsPolicy) *profileMetricRuntime {
	t.Helper()
	return newTestProfileMetricRuntimeWithPolicy(t, idx, ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	}, func(cfg *normalizedProfileMetricsConfig) {
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
		sourceFile: "test-profile.yaml",
	}
}

func addProfileMetricRuleWithChart(t *testing.T, idx *ProfileIndex, rule profileMetricRule, chart profileMetricChart) {
	t.Helper()
	if err := idx.addProfileMetrics([]profileMetricRule{rule}, []profileMetricChart{chart}); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
}

func profileMetricOutputForTest(metric, dimension, chart string) profileMetricOutput {
	return profileMetricOutput{Metric: metric, Dimension: dimension, Chart: chart}
}

func portSecurityTrapEntry(resource any) *TrapEntry {
	return &TrapEntry{
		JobName:       testProfileMetricJobName,
		TrapOID:       testPortSecurityTrapOID,
		TrapName:      "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment:    &TrapEnrichmentAudit{Source: &TrapSourceAudit{Selected: "192.0.2.10", Method: "udp_peer"}},
		Varbinds:      []VarbindValue{{OID: testIfIndexOID, Type: "INTEGER", Value: resource}},
	}
}

func TestProfileMetricSelection(t *testing.T) {
	idx := testProfileMetricIndex(t)
	idx.metricRulesByName["disabled.rule"] = &profileMetricRule{
		Name:    "disabled.rule",
		Type:    profileMetricTypeCounter,
		Enabled: new(false),
		OnTrap:  testCiscoConfigTrapOID,
		Output:  profileMetricOutput{Metric: "snmp_trap_disabled_events", Dimension: "events", Chart: "cisco_config_changes"},
	}
	cat := idx.profileMetricCatalog()

	tests := map[string]struct {
		cfg   ProfileMetricsConfig
		want  []string
		error bool
	}{
		"disabled": {
			cfg: ProfileMetricsConfig{},
		},
		"explicit include": {
			cfg:  ProfileMetricsConfig{Enabled: true, Include: []string{"cisco.config.terminal_type", "cisco.config.changed"}},
			want: []string{"cisco.config.changed", "cisco.config.terminal_type"},
		},
		"missing include": {
			cfg:   ProfileMetricsConfig{Enabled: true, Include: []string{"missing.rule"}},
			error: true,
		},
		"disabled include": {
			cfg:   ProfileMetricsConfig{Enabled: true, Include: []string{"disabled.rule"}},
			error: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, err := normalizeProfileMetricsConfig(tc.cfg)
			if err != nil {
				t.Fatalf("normalizeProfileMetricsConfig failed: %v", err)
			}
			rules, err := selectProfileMetricRules(cfg, cat)
			if tc.error {
				if err == nil {
					t.Fatalf("selectProfileMetricRules returned nil error")
				}
				return
			}
			if err != nil {
				t.Fatalf("selectProfileMetricRules failed: %v", err)
			}
			var got []string
			for _, rule := range rules {
				got = append(got, rule.Name)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("selected rules = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestProfileMetricSelectionRejectsMoreThanMaxRules(t *testing.T) {
	idx := testProfileMetricIndex(t)
	cat := idx.profileMetricCatalog()
	cfg, err := normalizeProfileMetricsConfig(ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed", "cisco.config.terminal_type"},
	})
	if err != nil {
		t.Fatalf("normalizeProfileMetricsConfig failed: %v", err)
	}

	cfg.limits.MaxRules = 1
	if _, err := selectProfileMetricRules(cfg, cat); err == nil {
		t.Fatalf("selectProfileMetricRules accepted more selected rules than max_rules")
	}
}

func TestNewProfileMetricRuntimeRejectsNilProfileIndex(t *testing.T) {
	cfg, err := normalizeProfileMetricsConfig(ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	})
	if err != nil {
		t.Fatalf("normalizeProfileMetricsConfig failed: %v", err)
	}

	if _, _, err := newProfileMetricRuntime(cfg, nil, "test"); err == nil || !strings.Contains(err.Error(), "profile index not available") {
		t.Fatalf("newProfileMetricRuntime nil index error = %v, want profile index not available", err)
	}
}

func TestProfileMetricRuntimePredicateFiltersByEnumLabel(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.console",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{{
			Varbind: testCiscoTerminalTypeVarbind,
			Equals:  "console",
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_console_config_events",
			Dimension: "console_events",
			Chart:     "cisco_config_changes",
		},
		sourceFile: "test-profile.yaml",
	}}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.console"})
	consoleEntry := ciscoConfigTrapEntry("profile-job")
	virtualEntry := ciscoConfigTrapEntry("profile-job")
	virtualEntry.Varbinds[1].Value = 3

	rt.update(consoleEntry)
	rt.update(virtualEntry)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_console_config_events", profileMetricSourceLabels("192.0.2.10"), 1)
	assertProfileMetricValue(t, store, "snmp_trap_profile_metrics_rule_missed", profileMetricJobLabels(), 1)
}

func TestProfileMetricRuntimePredicateFiltersBySyntheticFields(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.synthetic_fields",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{
			{Field: "category", Equals: "config_change"},
			{Field: "severity", In: []any{"notice"}},
			{Field: "trap_name", Equals: "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged"},
			{Field: "trap_oid", Equals: testCiscoConfigTrapOID},
		},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_config_synthetic_field_events",
			Dimension: "synthetic_field_events",
			Chart:     "cisco_config_changes",
		},
		sourceFile: "test-profile.yaml",
	}}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.synthetic_fields"})
	pass := ciscoConfigTrapEntry("profile-job")
	pass.Category = Category("config_change")
	pass.Severity = Severity("notice")
	fail := ciscoConfigTrapEntry("profile-job")
	fail.Category = Category("security")
	fail.Severity = Severity("notice")

	rt.update(pass)
	rt.update(fail)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_config_synthetic_field_events", profileMetricSourceLabels("192.0.2.10"), 1)
	assertProfileMetricValue(t, store, "snmp_trap_profile_metrics_rule_missed", profileMetricJobLabels(), 1)
}

func TestProfileMetricRuntimeIncludedCounterBySource(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")

	rt.update(entry)
	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_config_events", labels); !ok || v != 2 {
		t.Fatalf("snmp_trap_cisco_config_events = %v/%v, want 2/true", v, ok)
	}
}

func TestProfileMetricRuntimeIncludedSampleUsesVarbindValue(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.terminal_type"})
	entry := ciscoConfigTrapEntry("profile-job")

	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_terminal_type", labels); !ok || v != 2 {
		t.Fatalf("snmp_trap_cisco_terminal_type = %v/%v, want 2/true", v, ok)
	}
}

func TestProfileMetricRuntimeSampleWherePredicate(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics([]profileMetricRule{{
		Name:             "cisco.config.console_terminal_type",
		Type:             profileMetricTypeSample,
		OnTrap:           testCiscoConfigTrapOID,
		ValueFromVarbind: testCiscoTerminalTypeVarbind,
		Where: profileMetricPredicates{{
			Varbind: testCiscoTerminalTypeVarbind,
			Equals:  "console",
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_console_terminal_type",
			Dimension: "terminal_type",
			Chart:     "cisco_terminal_type",
		},
		sourceFile: "test-profile.yaml",
	}}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.console_terminal_type"})
	pass := ciscoConfigTrapEntry("profile-job")
	fail := ciscoConfigTrapEntry("profile-job")
	fail.Varbinds[1].Value = 3

	rt.update(pass)
	rt.update(fail)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_terminal_type", labels); !ok || v != 2 {
		t.Fatalf("snmp_trap_cisco_console_terminal_type = %v/%v, want 2/true", v, ok)
	}
	if v, ok := store.Read().Value("snmp_trap_profile_metrics_rule_missed", metrix.Labels{"job_name": "profile-job"}); !ok || v != 1 {
		t.Fatalf("snmp_trap_profile_metrics_rule_missed = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricRuntimeSampleEmitsContinuouslyUntilLifecycleExpiry(t *testing.T) {
	idx := testProfileMetricIndex(t)
	idx.metricChartsByID["cisco_terminal_type"].Lifecycle = &charttpl.Lifecycle{MaxInstances: 10, ExpireAfterCycles: 3}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.terminal_type"})
	entry := ciscoConfigTrapEntry("profile-job")
	store := metrix.NewCollectorStore()
	labels := profileMetricSourceLabels("192.0.2.10")

	rt.update(entry)
	for cycle := 1; cycle <= 3; cycle++ {
		collectProfileMetricsOnce(t, rt, store, "profile-job")
		if v, ok := store.Read().Value("snmp_trap_cisco_terminal_type", labels); !ok || v != 2 {
			t.Fatalf("cycle %d snmp_trap_cisco_terminal_type = %v/%v, want 2/true", cycle, v, ok)
		}
	}
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if _, ok := store.Read().Value("snmp_trap_cisco_terminal_type", labels); ok {
		t.Fatalf("snmp_trap_cisco_terminal_type remained after lifecycle expiry")
	}
}

func TestProfileMetricRuntimeSampleScaleAndMissingZero(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics([]profileMetricRule{{
		Name:             "cisco.config.terminal_type_scaled",
		Type:             profileMetricTypeSample,
		OnTrap:           testCiscoConfigTrapOID,
		ValueFromVarbind: testCiscoTerminalTypeVarbind,
		Missing:          profileMetricMissingZero,
		Scale:            profileMetricScale{Multiplier: 10, Divisor: 2},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_terminal_type_scaled",
			Dimension: "terminal_type_scaled",
			Chart:     "cisco_terminal_type",
		},
		sourceFile: "test-profile.yaml",
	}}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.terminal_type_scaled"})
	entry := ciscoConfigTrapEntry("profile-job")

	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_terminal_type_scaled", labels); !ok || v != 10 {
		t.Fatalf("snmp_trap_cisco_terminal_type_scaled = %v/%v, want 10/true", v, ok)
	}

	entry.Varbinds = entry.Varbinds[:1]
	rt.update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_terminal_type_scaled", labels); !ok || v != 0 {
		t.Fatalf("snmp_trap_cisco_terminal_type_scaled after missing varbind = %v/%v, want 0/true", v, ok)
	}
}

func TestProfileMetricRuntimeConvertsTimeTicksSamplesToSeconds(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics([]profileMetricRule{{
		Name:             "cisco.config.sysuptime_seconds",
		Type:             profileMetricTypeSample,
		OnTrap:           testCiscoConfigTrapOID,
		ValueFromVarbind: "sysUpTime.0",
		Scale:            profileMetricScale{Multiplier: 2, Divisor: 1},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_sysuptime_scaled_seconds",
			Dimension: "seconds",
			Chart:     "cisco_sysuptime_seconds",
		},
		sourceFile: "test-profile.yaml",
	}}, []profileMetricChart{{
		ID:         "cisco_sysuptime_seconds",
		Title:      "Cisco sysUpTime seconds",
		Context:    "snmp.trap.cisco.sysuptime.seconds",
		Units:      "seconds",
		Algorithm:  "absolute",
		Type:       "line",
		sourceFile: "test-profile.yaml",
	}}); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.sysuptime_seconds"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.Varbinds[2].Value = uint64(10000)

	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_sysuptime_scaled_seconds", labels); !ok || v != 200 {
		t.Fatalf("snmp_trap_cisco_sysuptime_scaled_seconds = %v/%v, want 200/true", v, ok)
	}
}

func TestProfileMetricRuntimeMissingDropAndErrorDiagnostics(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics([]profileMetricRule{
		{
			Name:             "cisco.config.terminal_type_missing_drop",
			Type:             profileMetricTypeSample,
			OnTrap:           testCiscoConfigTrapOID,
			ValueFromVarbind: testCiscoTerminalTypeVarbind,
			Missing:          profileMetricMissingDrop,
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_terminal_type_missing_drop",
				Dimension: "missing_drop",
				Chart:     "cisco_terminal_type",
			},
			sourceFile: "test-profile.yaml",
		},
		{
			Name:             "cisco.config.terminal_type_missing_error",
			Type:             profileMetricTypeSample,
			OnTrap:           testCiscoConfigTrapOID,
			ValueFromVarbind: testCiscoTerminalTypeVarbind,
			Missing:          profileMetricMissingError,
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_terminal_type_missing_error",
				Dimension: "missing_error",
				Chart:     "cisco_terminal_type",
			},
			sourceFile: "test-profile.yaml",
		},
	}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{
		"cisco.config.terminal_type_missing_drop",
		"cisco.config.terminal_type_missing_error",
	})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.Varbinds = entry.Varbinds[:1]

	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if _, ok := store.Read().Value("snmp_trap_cisco_terminal_type_missing_drop", labels); ok {
		t.Fatalf("missing=drop sample emitted a metric")
	}
	if _, ok := store.Read().Value("snmp_trap_cisco_terminal_type_missing_error", labels); ok {
		t.Fatalf("missing=error sample emitted a metric")
	}
	diagLabels := metrix.Labels{"job_name": "profile-job"}
	if v, ok := store.Read().Value("snmp_trap_profile_metrics_rule_missed", diagLabels); !ok || v != 1 {
		t.Fatalf("snmp_trap_profile_metrics_rule_missed = %v/%v, want 1/true", v, ok)
	}
	if v, ok := store.Read().Value("snmp_trap_profile_metrics_extraction_failed", diagLabels); !ok || v != 1 {
		t.Fatalf("snmp_trap_profile_metrics_extraction_failed = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricRuntimePredicateOperators(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics([]profileMetricRule{
		{
			Name:   "cisco.config.rich_predicates",
			Type:   profileMetricTypeCounter,
			OnTrap: testCiscoConfigTrapOID,
			Where: profileMetricPredicates{
				{Varbind: testCiscoTerminalTypeVarbind, Exists: new(true)},
				{Varbind: testCiscoTerminalTypeVarbind, In: []any{"console", "virtual"}},
				{Varbind: testCiscoCommandSourceVarbind, GreaterThan: 1},
				{Varbind: testCiscoCommandSourceVarbind, LessThan: 4},
				{Varbind: testCiscoCommandSourceVarbind, Range: []any{2, 3}},
				{Varbind: testCiscoTerminalTypeVarbind, Equals: "aux", Not: true},
			},
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_config_rich_predicate_events",
				Dimension: "events",
				Chart:     "cisco_config_changes",
			},
			sourceFile: "test-profile.yaml",
		},
		{
			Name:   "cisco.config.absent_predicate",
			Type:   profileMetricTypeCounter,
			OnTrap: testCiscoConfigTrapOID,
			Where: profileMetricPredicates{{
				Varbind: "sysUpTime.0",
				Absent:  new(true),
			}},
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_config_absent_predicate_events",
				Dimension: "absent_events",
				Chart:     "cisco_config_changes",
			},
			sourceFile: "test-profile.yaml",
		},
	}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{
		"cisco.config.rich_predicates",
		"cisco.config.absent_predicate",
	})
	pass := ciscoConfigTrapEntry("profile-job")
	pass.Varbinds = pass.Varbinds[:2]
	fail := ciscoConfigTrapEntry("profile-job")
	fail.Varbinds[1].Value = 4

	rt.update(pass)
	rt.update(fail)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_config_rich_predicate_events", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_cisco_config_rich_predicate_events = %v/%v, want 1/true", v, ok)
	}
	if v, ok := store.Read().Value("snmp_trap_cisco_config_absent_predicate_events", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_cisco_config_absent_predicate_events = %v/%v, want 1/true", v, ok)
	}
	if v, ok := store.Read().Value("snmp_trap_profile_metrics_rule_missed", metrix.Labels{"job_name": "profile-job"}); !ok || v != 2 {
		t.Fatalf("snmp_trap_profile_metrics_rule_missed = %v/%v, want 2/true", v, ok)
	}
}

func TestProfileMetricRuntimePredicateEdgeCases(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics([]profileMetricRule{
		{
			Name:   "cisco.config.exists_false",
			Type:   profileMetricTypeCounter,
			OnTrap: testCiscoConfigTrapOID,
			Where: profileMetricPredicates{{
				Varbind: "sysUpTime.0",
				Exists:  new(false),
			}},
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_config_exists_false_events",
				Dimension: "exists_false_events",
				Chart:     "cisco_config_changes",
			},
			sourceFile: "test-profile.yaml",
		},
		{
			Name:   "cisco.config.numeric_in",
			Type:   profileMetricTypeCounter,
			OnTrap: testCiscoConfigTrapOID,
			Where: profileMetricPredicates{{
				Varbind: testCiscoCommandSourceVarbind,
				In:      []any{2, 3},
			}},
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_config_numeric_in_events",
				Dimension: "numeric_in_events",
				Chart:     "cisco_config_changes",
			},
			sourceFile: "test-profile.yaml",
		},
		{
			Name:   "cisco.config.synthetic_not",
			Type:   profileMetricTypeCounter,
			OnTrap: testCiscoConfigTrapOID,
			Where: profileMetricPredicates{{
				Field:  "category",
				Equals: "security",
				Not:    true,
			}},
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cisco_config_synthetic_not_events",
				Dimension: "synthetic_not_events",
				Chart:     "cisco_config_changes",
			},
			sourceFile: "test-profile.yaml",
		},
	}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{
		"cisco.config.exists_false",
		"cisco.config.numeric_in",
		"cisco.config.synthetic_not",
	})

	pass := ciscoConfigTrapEntry("profile-job")
	pass.Category = "config_change"
	pass.Varbinds = pass.Varbinds[:2]
	fail := ciscoConfigTrapEntry("profile-job")
	fail.Category = "security"
	fail.Varbinds[0].Value = 4

	rt.update(pass)
	rt.update(fail)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	for metric, expected := range map[string]float64{
		"snmp_trap_cisco_config_exists_false_events":  1,
		"snmp_trap_cisco_config_numeric_in_events":    1,
		"snmp_trap_cisco_config_synthetic_not_events": 1,
	} {
		if v, ok := store.Read().Value(metric, labels); !ok || v != expected {
			t.Fatalf("%s = %v/%v, want %v/true", metric, v, ok, expected)
		}
	}
	if v, ok := store.Read().Value("snmp_trap_profile_metrics_rule_missed", metrix.Labels{"job_name": "profile-job"}); !ok || v != 3 {
		t.Fatalf("snmp_trap_profile_metrics_rule_missed = %v/%v, want 3/true", v, ok)
	}
}

func TestProfileMetricRuntimeRejectsNonFinitePredicateActual(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.finite_range",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{{
			Varbind: testCiscoCommandSourceVarbind,
			Range:   []any{1, 4},
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_config_finite_range_events",
			Dimension: "finite_range_events",
			Chart:     "cisco_config_changes",
		},
		sourceFile: "test-profile.yaml",
	}}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.finite_range"})

	for _, value := range []any{"NaN", "+Inf", "-Inf"} {
		entry := ciscoConfigTrapEntry("profile-job")
		entry.Varbinds[0].Value = value
		rt.update(entry)
	}

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_config_finite_range_events", labels); ok {
		t.Fatalf("snmp_trap_cisco_config_finite_range_events = %v/true, want metric absent", v)
	}
	if v, ok := store.Read().Value("snmp_trap_profile_metrics_rule_missed", metrix.Labels{"job_name": "profile-job"}); !ok || v != 3 {
		t.Fatalf("snmp_trap_profile_metrics_rule_missed = %v/%v, want 3/true", v, ok)
	}
}

func TestProfileMetricRuntimeSameOIDStateRule(t *testing.T) {
	idx := testProfileMetricIndex(t)
	addProfileMetricRuleWithChart(
		t,
		idx,
		profileMetricRule{
			Name:   "cisco.config.console_state",
			Type:   profileMetricTypeState,
			OnTrap: testCiscoConfigTrapOID,
			State: profileMetricState{
				SetWhen:   &profileMetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "console"},
				ClearWhen: &profileMetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "virtual"},
			},
			Output:     profileMetricOutputForTest("snmp_trap_cisco_console_session_state", "active", "cisco_console_state"),
			sourceFile: "test-profile.yaml",
		},
		profileMetricChartForTest("cisco_console_state", "Cisco console configuration state", "snmp.trap.cisco.console.state", "state", "absolute"),
	)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.console_state"})
	entry := ciscoConfigTrapEntry("profile-job")

	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_session_state", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_cisco_console_session_state after set = %v/%v, want 1/true", v, ok)
	}

	entry.Varbinds[1].Value = 3
	rt.update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_session_state", labels); !ok || v != 0 {
		t.Fatalf("snmp_trap_cisco_console_session_state after clear = %v/%v, want 0/true", v, ok)
	}
}

func TestProfileMetricRuntimeSameOIDStateCustomValuesAndWhere(t *testing.T) {
	idx := testProfileMetricIndex(t)
	addProfileMetricRuleWithChart(
		t,
		idx,
		profileMetricRule{
			Name:   "cisco.config.console_custom_state",
			Type:   profileMetricTypeState,
			OnTrap: testCiscoConfigTrapOID,
			Where: profileMetricPredicates{{
				Field:  "category",
				Equals: "config_change",
			}},
			State: profileMetricState{
				SetWhen:      &profileMetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "console"},
				ClearWhen:    &profileMetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "virtual"},
				ProblemValue: new(float64(5)),
				ClearValue:   2,
			},
			Output:     profileMetricOutputForTest("snmp_trap_cisco_console_custom_state", "active", "cisco_console_custom_state"),
			sourceFile: "test-profile.yaml",
		},
		profileMetricChartForTest("cisco_console_custom_state", "Cisco console custom state", "snmp.trap.cisco.console.custom.state", "state", "absolute"),
	)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.console_custom_state"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.Category = Category("config_change")
	store := metrix.NewCollectorStore()
	labels := profileMetricSourceLabels("192.0.2.10")

	rt.update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_custom_state", labels); !ok || v != 5 {
		t.Fatalf("snmp_trap_cisco_console_custom_state after set = %v/%v, want 5/true", v, ok)
	}

	filtered := ciscoConfigTrapEntry("profile-job")
	filtered.Category = "security"
	filtered.Varbinds[1].Value = 3
	rt.update(filtered)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_custom_state", labels); !ok || v != 5 {
		t.Fatalf("snmp_trap_cisco_console_custom_state after filtered clear = %v/%v, want 5/true", v, ok)
	}
	if v, ok := store.Read().Value("snmp_trap_profile_metrics_rule_missed", metrix.Labels{"job_name": "profile-job"}); !ok || v != 1 {
		t.Fatalf("snmp_trap_profile_metrics_rule_missed = %v/%v, want 1/true", v, ok)
	}

	entry.Varbinds[1].Value = 3
	rt.update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_custom_state", labels); !ok || v != 2 {
		t.Fatalf("snmp_trap_cisco_console_custom_state after clear = %v/%v, want 2/true", v, ok)
	}
}

func TestProfileMetricRuntimeSeparateOIDStateRuleSupportsZeroProblemValue(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addTraps([]*TrapDef{
		{OID: testLinkDownTrapOID, Name: "SNMPv2-MIB::linkDown", Category: "state_change", Severity: "warning", sourceFile: "test-profile.yaml"},
		{OID: testLinkUpTrapOID, Name: "SNMPv2-MIB::linkUp", Category: "state_change", Severity: "notice", sourceFile: "test-profile.yaml"},
	}); err != nil {
		t.Fatalf("addTraps failed: %v", err)
	}
	addProfileMetricRuleWithChart(
		t,
		idx,
		profileMetricRule{
			Name:        "if.link_state",
			Type:        profileMetricTypeState,
			ProblemTrap: "SNMPv2-MIB::linkDown",
			ClearTrap:   "SNMPv2-MIB::linkUp",
			State: profileMetricState{
				ProblemValue: new(float64(0)),
				ClearValue:   1,
			},
			Output:     profileMetricOutputForTest("snmp_trap_if_link_state", "up", "if_link_state"),
			sourceFile: "test-profile.yaml",
		},
		profileMetricChartForTest("if_link_state", "Interface link state", "snmp.trap.if.link.state", "state", "absolute"),
	)
	rt := newTestProfileMetricRuntime(t, idx, []string{"if.link_state"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.Varbinds = nil

	entry.TrapOID = testLinkUpTrapOID
	entry.TrapName = "SNMPv2-MIB::linkUp"
	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_if_link_state", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_if_link_state after clear-before-problem = %v/%v, want 1/true", v, ok)
	}

	entry.TrapOID = testLinkDownTrapOID
	entry.TrapName = "SNMPv2-MIB::linkDown"
	rt.update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_if_link_state", labels); !ok || v != 0 {
		t.Fatalf("snmp_trap_if_link_state after problem = %v/%v, want 0/true", v, ok)
	}

	entry.TrapOID = testLinkUpTrapOID
	entry.TrapName = "SNMPv2-MIB::linkUp"
	rt.update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_if_link_state", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_if_link_state after clear = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricRuntimeStateTTLClearsAndExpires(t *testing.T) {
	idx := testProfileMetricIndex(t)
	addProfileMetricRuleWithChart(
		t,
		idx,
		profileMetricRule{
			Name:   "cisco.config.console_state_ttl",
			Type:   profileMetricTypeState,
			OnTrap: testCiscoConfigTrapOID,
			State: profileMetricState{
				SetWhen:   &profileMetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "console"},
				ClearWhen: &profileMetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "virtual"},
				TTL:       "1ms",
			},
			Output:     profileMetricOutputForTest("snmp_trap_cisco_console_ttl_state", "active", "cisco_console_ttl_state"),
			sourceFile: "test-profile.yaml",
		},
		profileMetricChartForTest("cisco_console_ttl_state", "Cisco console TTL state", "snmp.trap.cisco.console.ttl.state", "state", "absolute"),
	)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.console_state_ttl"})
	entry := ciscoConfigTrapEntry("profile-job")
	store := metrix.NewCollectorStore()
	labels := profileMetricSourceLabels("192.0.2.10")

	rt.update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_ttl_state", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_cisco_console_ttl_state after set = %v/%v, want 1/true", v, ok)
	}

	time.Sleep(5 * time.Millisecond)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_ttl_state", labels); !ok || v != 0 {
		t.Fatalf("snmp_trap_cisco_console_ttl_state after TTL clear = %v/%v, want 0/true", v, ok)
	}
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if _, ok := store.Read().Value("snmp_trap_cisco_console_ttl_state", labels); ok {
		t.Fatalf("snmp_trap_cisco_console_ttl_state remained after clear-and-expire")
	}
}

func TestProfileMetricRuntimeUsesVnodeHostScopeWhenAvailable(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.SourceVnodeID = "vnode-1"
	entry.DeviceHostname = "switch-1"

	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	defaultLabels := metrix.Labels{"job_name": "profile-job", "source_id": "vnode-1", "source_kind": "vnode"}
	if _, ok := store.Read().Value("snmp_trap_cisco_config_events", defaultLabels); ok {
		t.Fatalf("profile metric appeared in default host scope; want vnode-scoped only")
	}
	if v, ok := store.Read(metrix.ReadHostScope("vnode-1")).Value("snmp_trap_cisco_config_events", defaultLabels); !ok || v != 1 {
		t.Fatalf("vnode-scoped snmp_trap_cisco_config_events = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricRuntimeFallsBackWhenVnodeAttributionConflicts(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.SourceVnodeID = "vnode-1"
	entry.DeviceHostname = "switch-1"
	entry.Enrichment.Topology = &TrapEnrichmentLookup{
		Status: "conflict",
		Reason: "vnode_mismatch",
	}

	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	vnodeLabels := metrix.Labels{"job_name": "profile-job", "source_id": "vnode-1", "source_kind": "vnode"}
	if _, ok := store.Read(metrix.ReadHostScope("vnode-1")).Value("snmp_trap_cisco_config_events", vnodeLabels); ok {
		t.Fatalf("conflicting vnode attribution emitted vnode-scoped profile metric")
	}
	fallbackLabels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_config_events", fallbackLabels); !ok || v != 1 {
		t.Fatalf("fallback-scoped snmp_trap_cisco_config_events = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricRuntimeDefaultHashSourcePrivacy(t *testing.T) {
	idx := testProfileMetricIndex(t)
	cfg, err := normalizeProfileMetricsConfig(ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	})
	if err != nil {
		t.Fatalf("normalizeProfileMetricsConfig failed: %v", err)
	}
	rt, _, err := newProfileMetricRuntime(cfg, idx, "test")
	if err != nil {
		t.Fatalf("newProfileMetricRuntime failed: %v", err)
	}
	entry := ciscoConfigTrapEntry("profile-job")

	rt.update(entry)

	sourceID, sourceKind := rt.fallbackSourceIdentity(entry)
	if sourceID == "192.0.2.10" {
		t.Fatalf("hashed source ID leaked raw source address")
	}
	if len(sourceID) != 16 {
		t.Fatalf("hashed source ID length = %d, want 16", len(sourceID))
	}
	if _, err := hex.DecodeString(sourceID); err != nil {
		t.Fatalf("hashed source ID is not hex: %v", err)
	}

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	labels := metrix.Labels{"job_name": "profile-job", "source_id": sourceID, "source_kind": sourceKind}
	if v, ok := store.Read().Value("snmp_trap_cisco_config_events", labels); !ok || v != 1 {
		t.Fatalf("hashed-source snmp_trap_cisco_config_events = %v/%v, want 1/true", v, ok)
	}
}

func TestRawFallbackTrapSourceIdentityMapsUnknownMethodToOther(t *testing.T) {
	entry := ciscoConfigTrapEntry("profile-job")
	entry.Enrichment.Source.Method = "future_method_name"

	sourceID, sourceKind := rawFallbackTrapSourceIdentity(entry)
	if sourceID != "192.0.2.10" || sourceKind != "other" {
		t.Fatalf("raw fallback identity = %q/%q, want 192.0.2.10/other", sourceID, sourceKind)
	}
}

func TestProfileMetricRuntimeSourceLabelIgnoresVnodeScope(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := newTestProfileMetricRuntimeWithPolicy(t, idx, ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	}, func(cfg *normalizedProfileMetricsConfig) {
		cfg.identity.Device = profileMetricIdentitySourceLabel
	})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.SourceVnodeID = "vnode-1"
	entry.DeviceHostname = "switch-1"

	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_config_events", labels); !ok || v != 1 {
		t.Fatalf("source-label snmp_trap_cisco_config_events = %v/%v, want 1/true", v, ok)
	}
	if _, ok := store.Read(metrix.ReadHostScope("vnode-1")).Value("snmp_trap_cisco_config_events", labels); ok {
		t.Fatalf("source_label metric appeared in vnode host scope")
	}
}

func TestProfileMetricRuntimeAttributionFailureDiagnostics(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.SourceIP = ""
	entry.SourceUDPPeer = ""
	entry.Enrichment = nil

	rt.update(entry)

	rt.mu.Lock()
	series := len(rt.series)
	rt.mu.Unlock()
	if series != 0 {
		t.Fatalf("series after attribution failure = %d, want 0", series)
	}

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_profile_metrics_attribution_failed", metrix.Labels{"job_name": "profile-job"}); !ok || v != 1 {
		t.Fatalf("snmp_trap_profile_metrics_attribution_failed = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricRuntimeListenerIdentity(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := newTestProfileMetricRuntimeWithPolicy(t, idx, ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	}, func(cfg *normalizedProfileMetricsConfig) {
		cfg.identity.Device = profileMetricIdentityListener
	})
	first := ciscoConfigTrapEntry("profile-job")
	second := ciscoConfigTrapEntry("profile-job")
	second.SourceIP = "192.0.2.11"
	second.SourceUDPPeer = "192.0.2.11"
	second.Enrichment.Source.Selected = "192.0.2.11"

	rt.update(first)
	rt.update(second)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := metrix.Labels{"job_name": "profile-job", "source_id": "profile-job", "source_kind": "listener"}
	if v, ok := store.Read().Value("snmp_trap_cisco_config_events", labels); !ok || v != 2 {
		t.Fatalf("listener-scoped snmp_trap_cisco_config_events = %v/%v, want 2/true", v, ok)
	}
}

func TestProfileMetricRuntimeCountsSourceRouteTransitions(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")

	rt.update(entry)
	entry.SourceVnodeID = "vnode-1"
	entry.DeviceHostname = "switch-1"
	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	if v, ok := store.Read().Value("snmp_trap_profile_metrics_source_transitions", metrix.Labels{"job_name": "profile-job"}); !ok || v != 1 {
		t.Fatalf("snmp_trap_profile_metrics_source_transitions = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricRuntimeResourceIdentity(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.port_security.ifindex"})
	entry := &TrapEntry{
		JobName:       "profile-job",
		TrapOID:       testPortSecurityTrapOID,
		TrapName:      "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment: &TrapEnrichmentAudit{Source: &TrapSourceAudit{
			Selected: "192.0.2.10",
			Method:   "udp_peer",
		}},
		Varbinds: []VarbindValue{
			{OID: testIfIndexOID, Type: "INTEGER", Value: 7},
		},
	}

	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := portSecurityResourceLabels("7")
	if v, ok := store.Read().Value("snmp_trap_cisco_port_security_violations", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_cisco_port_security_violations = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricChartTemplateUsesResourceLabels(t *testing.T) {
	idx := testProfileMetricIndex(t)
	cfg, err := normalizeProfileMetricsConfig(ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.port_security.ifindex"},
	})
	if err != nil {
		t.Fatalf("normalizeProfileMetricsConfig failed: %v", err)
	}
	_, tmpl, err := newProfileMetricRuntime(cfg, idx, "test")
	if err != nil {
		t.Fatalf("newProfileMetricRuntime failed: %v", err)
	}
	spec, err := charttpl.DecodeYAML([]byte(tmpl))
	if err != nil {
		t.Fatalf("DecodeYAML failed: %v", err)
	}
	var labels []string
	for _, group := range spec.Groups {
		for _, chart := range group.Charts {
			if chart.ID == "port_security_violations" && chart.Instances != nil {
				labels = chart.Instances.ByLabels
			}
		}
	}
	for _, want := range []string{"job_name", "source_id", "source_kind", "resource_class", "resource_id"} {
		if !slices.Contains(labels, want) {
			t.Fatalf("resource chart labels %v missing %q", labels, want)
		}
	}
}

func TestProfileMetricRuntimeConcurrentUpdates(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")

	var wg sync.WaitGroup
	for range 25 {
		wg.Go(func() {
			rt.update(entry)
		})
	}
	wg.Wait()

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_config_events", labels); !ok || v != 25 {
		t.Fatalf("snmp_trap_cisco_config_events after concurrent updates = %v/%v, want 25/true", v, ok)
	}
}

func TestProfileMetricRuntimeConcurrentUpdateAndCollect(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")

	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Go(func() {
		for range 100 {
			store := metrix.NewCollectorStore()
			managed, ok := metrix.AsCycleManagedStore(store)
			if !ok {
				errCh <- errors.New("AsCycleManagedStore returned false")
				return
			}
			managed.CycleController().BeginCycle()
			rt.collect(store, "profile-job")
			if err := managed.CycleController().CommitCycleSuccess(); err != nil {
				errCh <- err
				return
			}
		}
	})

	for range 1000 {
		rt.update(entry)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent collect failed: %v", err)
		}
	}
}

func TestProfileMetricRuntimeSourceCapSkipsOnlyMetricInstance(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := sourceRuntimeWithLimits(t, idx, profileMetricLimitsPolicy{MaxSources: 1})
	first := ciscoConfigTrapEntry(testProfileMetricJobName)
	second := ciscoConfigTrapEntryFromSource(testProfileMetricJobName, "192.0.2.11")

	rt.update(first)
	rt.update(second)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.10"), 1)
	assertProfileMetricAbsent(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.11"))
	assertProfileMetricOverflow(t, store, 1)
}

func TestProfileMetricRuntimeMaxInstancesSkipsOnlyNewMetricInstance(t *testing.T) {
	idx := testProfileMetricIndex(t)
	rt := sourceRuntimeWithLimits(t, idx, profileMetricLimitsPolicy{MaxSources: 10, MaxInstancesPerJob: 1})
	first := ciscoConfigTrapEntry(testProfileMetricJobName)
	second := ciscoConfigTrapEntryFromSource(testProfileMetricJobName, "192.0.2.11")

	rt.update(first)
	rt.update(second)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.10"), 1)
	assertProfileMetricAbsent(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.11"))
	assertProfileMetricOverflow(t, store, 1)
}

func TestProfileMetricRuntimeChartMaxInstancesSkipsOnlyNewChartInstance(t *testing.T) {
	idx := testProfileMetricIndex(t)
	idx.metricChartsByID["cisco_config_changes"].Lifecycle = &charttpl.Lifecycle{MaxInstances: 1, ExpireAfterCycles: 60}
	rt := sourceRuntimeWithLimits(t, idx, profileMetricLimitsPolicy{MaxSources: 10, MaxInstancesPerJob: 10})
	first := ciscoConfigTrapEntry(testProfileMetricJobName)
	second := ciscoConfigTrapEntryFromSource(testProfileMetricJobName, "192.0.2.11")

	rt.update(first)
	rt.update(second)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.10"), 1)
	assertProfileMetricAbsent(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.11"))
	assertProfileMetricOverflow(t, store, 1)
}

func TestProfileMetricRuntimeReleasesChartMaxInstancesAfterLifecycleExpiry(t *testing.T) {
	idx := testProfileMetricIndex(t)
	idx.metricChartsByID["cisco_config_changes"].Lifecycle = &charttpl.Lifecycle{MaxInstances: 1, ExpireAfterCycles: 1}
	rt := sourceRuntimeWithLimits(t, idx, profileMetricLimitsPolicy{MaxSources: 10, MaxInstancesPerJob: 10})
	first := ciscoConfigTrapEntry(testProfileMetricJobName)
	second := ciscoConfigTrapEntryFromSource(testProfileMetricJobName, "192.0.2.11")
	store := metrix.NewCollectorStore()

	rt.update(first)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	rt.update(second)
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	assertProfileMetricValue(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.11"), 1)
	assertProfileMetricOverflow(t, store, 0)
}

func TestProfileMetricRuntimeMaxInstancesUsesDeterministicRuleOrder(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics(
		[]profileMetricRule{
			{
				Name:   "z.tie_a_chart",
				Type:   profileMetricTypeCounter,
				OnTrap: testCiscoConfigTrapOID,
				Output: profileMetricOutput{
					Metric:    "snmp_trap_tie_a_chart_events",
					Dimension: "events",
					Chart:     "a_tie_chart",
				},
				sourceFile: "test-profile.yaml",
			},
			{
				Name:   "a.tie_z_chart",
				Type:   profileMetricTypeCounter,
				OnTrap: testCiscoConfigTrapOID,
				Output: profileMetricOutput{
					Metric:    "snmp_trap_tie_z_chart_events",
					Dimension: "events",
					Chart:     "z_tie_chart",
				},
				sourceFile: "test-profile.yaml",
			},
		},
		[]profileMetricChart{
			{
				ID:         "a_tie_chart",
				Title:      "A tie chart",
				Context:    "snmp.trap.tie.a",
				Units:      "events/s",
				Algorithm:  "incremental",
				sourceFile: "test-profile.yaml",
			},
			{
				ID:         "z_tie_chart",
				Title:      "Z tie chart",
				Context:    "snmp.trap.tie.z",
				Units:      "events/s",
				Algorithm:  "incremental",
				sourceFile: "test-profile.yaml",
			},
		},
	); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntimeWithPolicy(t, idx, ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"a.tie_z_chart", "z.tie_a_chart"},
	}, func(cfg *normalizedProfileMetricsConfig) {
		cfg.limits.MaxInstancesPerJob = 1
	})

	rt.update(ciscoConfigTrapEntry("profile-job"))

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_tie_a_chart_events", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_tie_a_chart_events = %v/%v, want 1/true", v, ok)
	}
	if _, ok := store.Read().Value("snmp_trap_tie_z_chart_events", labels); ok {
		t.Fatalf("snmp_trap_tie_z_chart_events was emitted despite max_instances_per_job=1")
	}
}

func TestProfileMetricRuntimeReleasesSourceCapAfterLifecycleExpiry(t *testing.T) {
	idx := testProfileMetricIndex(t)
	idx.metricChartsByID["cisco_config_changes"].Lifecycle = &charttpl.Lifecycle{MaxInstances: 10, ExpireAfterCycles: 1}
	rt := sourceRuntimeWithLimits(t, idx, profileMetricLimitsPolicy{MaxSources: 1})
	first := ciscoConfigTrapEntry(testProfileMetricJobName)
	second := ciscoConfigTrapEntryFromSource(testProfileMetricJobName, "192.0.2.11")
	store := metrix.NewCollectorStore()

	rt.update(first)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	rt.update(second)
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	assertProfileMetricValue(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.11"), 1)
	assertProfileMetricOverflow(t, store, 0)
}

func TestProfileMetricRuntimePrunesExpiredSourceRoutes(t *testing.T) {
	idx := testProfileMetricIndex(t)
	idx.metricChartsByID["cisco_config_changes"].Lifecycle = &charttpl.Lifecycle{MaxInstances: 10, ExpireAfterCycles: 1}
	rt := newTestProfileMetricRuntimeWithPolicy(t, idx, ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	}, func(cfg *normalizedProfileMetricsConfig) {
		cfg.identity.Device = profileMetricIdentityListener
		cfg.limits.MaxSources = 1
	})
	first := ciscoConfigTrapEntry(testProfileMetricJobName)
	second := ciscoConfigTrapEntryFromSource(testProfileMetricJobName, "192.0.2.11")
	rt.update(first)
	rt.update(second)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	rt.mu.Lock()
	routeCount := len(rt.sourceRoutes)
	rt.mu.Unlock()
	if routeCount > 1 {
		t.Fatalf("sourceRoutes = %d, want <= 1 after lifecycle expiry and pruning", routeCount)
	}
}

func TestProfileMetricRuntimeResourceCapSkipsOnlyNewResource(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics([]profileMetricRule{{
		Name:       "cisco.port_security.ifindex_cap",
		Type:       profileMetricTypeCounter,
		OnTrap:     testPortSecurityTrapOID,
		Identity:   profileMetricIdentity{Resource: &profileMetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 1}},
		Output:     profileMetricOutputForTest("snmp_trap_cisco_port_security_capped_violations", "violations", "port_security_violations"),
		sourceFile: "test-profile.yaml",
	}}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.port_security.ifindex_cap"})
	first := portSecurityTrapEntry(7)
	second := portSecurityTrapEntry(8)

	rt.update(first)
	rt.update(second)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_port_security_capped_violations", portSecurityResourceLabels("7"), 1)
	assertProfileMetricAbsent(t, store, "snmp_trap_cisco_port_security_capped_violations", portSecurityResourceLabels("8"))
	assertProfileMetricOverflow(t, store, 1)
}

func TestProfileMetricRuntimeReleasesResourceCapAfterLifecycleExpiry(t *testing.T) {
	idx := testProfileMetricIndex(t)
	idx.metricChartsByID["port_security_violations"].Lifecycle = &charttpl.Lifecycle{MaxInstances: 10, ExpireAfterCycles: 1}
	if err := idx.addProfileMetrics([]profileMetricRule{{
		Name:       "cisco.port_security.ifindex_lifecycle_cap",
		Type:       profileMetricTypeCounter,
		OnTrap:     testPortSecurityTrapOID,
		Identity:   profileMetricIdentity{Resource: &profileMetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 1}},
		Output:     profileMetricOutputForTest("snmp_trap_cisco_port_security_lifecycle_capped_violations", "violations", "port_security_violations"),
		sourceFile: "test-profile.yaml",
	}}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.port_security.ifindex_lifecycle_cap"})
	first := portSecurityTrapEntry(7)
	second := portSecurityTrapEntry(8)
	store := metrix.NewCollectorStore()

	rt.update(first)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	rt.update(second)
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	assertProfileMetricValue(t, store, "snmp_trap_cisco_port_security_lifecycle_capped_violations", portSecurityResourceLabels("8"), 1)
	assertProfileMetricOverflow(t, store, 0)
}

func TestProfileMetricRuntimeResourceCapUsesJobDefault(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics([]profileMetricRule{{
		Name:       "cisco.port_security.ifindex_job_cap",
		Type:       profileMetricTypeCounter,
		OnTrap:     testPortSecurityTrapOID,
		Identity:   profileMetricIdentity{Resource: &profileMetricResource{Class: "interface", KeyFromVarbind: "ifIndex"}},
		Output:     profileMetricOutputForTest("snmp_trap_cisco_port_security_job_capped_violations", "violations", "port_security_violations"),
		sourceFile: "test-profile.yaml",
	}}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntimeWithPolicy(t, idx, ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.port_security.ifindex_job_cap"},
	}, func(cfg *normalizedProfileMetricsConfig) {
		cfg.limits.MaxResourcesPerSource = 1
	})
	first := portSecurityTrapEntry(7)
	second := portSecurityTrapEntry(8)

	rt.update(first)
	rt.update(second)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_port_security_job_capped_violations", portSecurityResourceLabels("7"), 1)
	assertProfileMetricAbsent(t, store, "snmp_trap_cisco_port_security_job_capped_violations", portSecurityResourceLabels("8"))
	assertProfileMetricOverflow(t, store, 1)
}

func TestProfileMetricRuntimeMissingResourceUnknownDimension(t *testing.T) {
	idx := testProfileMetricIndex(t)
	if err := idx.addProfileMetrics([]profileMetricRule{{
		Name:     "cisco.port_security.ifindex_unknown",
		Type:     profileMetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Missing:  profileMetricMissingUnknownDimension,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 2}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_port_security_unknown_violations",
			Dimension: "violations",
			Chart:     "port_security_violations",
		},
		sourceFile: "test-profile.yaml",
	}}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.port_security.ifindex_unknown"})
	entry := &TrapEntry{
		JobName:       "profile-job",
		TrapOID:       testPortSecurityTrapOID,
		TrapName:      "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment:    &TrapEnrichmentAudit{Source: &TrapSourceAudit{Selected: "192.0.2.10", Method: "udp_peer"}},
	}

	rt.update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := portSecurityResourceLabels("unknown")
	if v, ok := store.Read().Value("snmp_trap_cisco_port_security_unknown_violations", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_cisco_port_security_unknown_violations = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricsUpdateAfterCommittedTrapOnly(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	idx := &ProfileIndex{trapsByOID: make(map[string]*TrapDef), namesByTrapName: make(map[string]*TrapDef)}
	coldStart := testColdStartTrap("state_change", "warning", "coldStart")
	if err := idx.addTraps([]*TrapDef{coldStart}); err != nil {
		t.Fatalf("addTraps failed: %v", err)
	}
	if err := idx.addProfileMetrics(
		[]profileMetricRule{{
			Name:   "snmp.cold_start",
			Type:   profileMetricTypeCounter,
			OnTrap: coldStart.OID,
			Output: profileMetricOutput{
				Metric:    "snmp_trap_cold_start_events",
				Dimension: "events",
				Chart:     "cold_start",
			},
			sourceFile: "test-profile.yaml",
		}},
		[]profileMetricChart{{
			ID:         "cold_start",
			Title:      "SNMP cold start",
			Context:    "snmp.trap.cold.start",
			Units:      "events/s",
			Algorithm:  "incremental",
			sourceFile: "test-profile.yaml",
		}},
	); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"snmp.cold_start"})
	globalProfileCache.current.Store(idx)
	t.Cleanup(func() { globalProfileCache.current.Store(nil) })

	failedWriter := &mockTrapWriter{err: errors.New("write failed")}
	c := newDefaultTestV2Collector(failedWriter)
	c.profileMetrics = rt
	c.handlePacket(packet.payload, packet.peer, nil, nil)

	rt.mu.Lock()
	seriesAfterFailedWrite := len(rt.series)
	rt.mu.Unlock()
	if seriesAfterFailedWrite != 0 {
		t.Fatalf("profile metric series after failed write = %d, want 0", seriesAfterFailedWrite)
	}

	store := metrix.NewCollectorStore()
	successWriter := &mockTrapWriter{}
	c.trapWriter = successWriter
	c.handlePacket(packet.payload, packet.peer, nil, nil)
	if len(successWriter.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(successWriter.entries))
	}
	sourceID, sourceKind := rt.fallbackSourceIdentity(successWriter.entries[0])
	collectProfileMetricsOnce(t, rt, store, "test")
	if v, ok := store.Read().Value("snmp_trap_cold_start_events", metrix.Labels{"job_name": "test", "source_id": sourceID, "source_kind": sourceKind}); !ok || v != 1 {
		t.Fatalf("snmp_trap_cold_start_events = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricRuntimeLoadsMetricBearingStockProfiles(t *testing.T) {
	stockDir := t.TempDir()
	writeProfileYAML(t, stockDir, "stock.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.1
    name: SNMPv2-MIB::coldStart
    category: state_change
    severity: warning
charts:
  - id: stock_cold_start
    title: Stock cold start
    context: snmp.trap.stock.cold.start
    units: events/s
    algorithm: incremental
metrics:
  - name: stock.cold_start
    type: counter
    on_trap: SNMPv2-MIB::coldStart
    output:
      metric: snmp_trap_stock_cold_start_events
      dimension: events
      chart: stock_cold_start
`)
	idx := &ProfileIndex{
		trapsByOID:        make(map[string]*TrapDef),
		namesByTrapName:   make(map[string]*TrapDef),
		metricRulesByName: make(map[string]*profileMetricRule),
		metricChartsByID:  make(map[string]*profileMetricChart),
	}
	store, err := buildStockProfileStore(stockDir, multipath.New(stockDir), nil, idx)
	if err != nil {
		t.Fatalf("buildStockProfileStore failed: %v", err)
	}
	idx.stock = store
	if len(idx.metricRulesByName) != 0 {
		t.Fatalf("stock metric rule was loaded before profile_metrics runtime creation")
	}

	rt := newTestProfileMetricRuntimeWithConfig(t, idx, ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"stock.cold_start"},
	})
	if len(rt.rules) != 1 {
		t.Fatalf("runtime rules = %d, want 1 stock metric rule", len(rt.rules))
	}
	if !idx.stock.metricsLoaded {
		t.Fatalf("stock metric scan was not marked loaded")
	}
	if idx.metricRulesByName["stock.cold_start"] == nil {
		t.Fatalf("stock metric rule was not added to profile catalog")
	}
}

func TestProfileMetricUserRuleCanReferenceStockTrapName(t *testing.T) {
	userDir := t.TempDir()
	stockDir := t.TempDir()
	writeProfileYAML(t, stockDir, "stock.yaml", `
traps:
  - oid: 1.3.6.1.6.3.1.1.5.1
    name: SNMPv2-MIB::coldStart
    category: state_change
    severity: warning
`)
	writeProfileYAML(t, userDir, "site.yaml", `
charts:
  - id: site_cold_start
    title: Site cold start
    context: snmp.trap.site.cold_start
    units: events/s
    algorithm: incremental
metrics:
  - name: site.cold_start
    type: counter
    on_trap: SNMPv2-MIB::coldStart
    output:
      metric: snmp_trap_site_cold_start_events
      dimension: events
      chart: site_cold_start
`)

	paths := profileLoadPaths{
		eagerDirs: []string{userDir},
		stockDir:  stockDir,
		all:       multipath.New(userDir, stockDir),
	}
	idx, seen, accepted, err := loadUserProfileTraps(paths)
	if err != nil {
		t.Fatalf("loadUserProfileTraps failed: %v", err)
	}
	store, err := buildStockProfileStore(stockDir, paths.all, seen, idx)
	if err != nil {
		t.Fatalf("buildStockProfileStore failed: %v", err)
	}
	idx.stock = store
	if source := idx.loadedTrapNameSource("SNMPv2-MIB::coldStart"); source != "" {
		t.Fatalf("stock trap name was loaded before profile metric validation: %q", source)
	}
	if err := idx.loadStockProfileMetrics(); err != nil {
		t.Fatalf("loadStockProfileMetrics failed: %v", err)
	}
	if err := addLoadedProfileMetrics(idx, accepted); err != nil {
		t.Fatalf("addLoadedProfileMetrics failed: %v", err)
	}

	rt := newTestProfileMetricRuntimeWithConfig(t, idx, ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"site.cold_start"},
	})
	entry := &TrapEntry{
		JobName:       "profile-job",
		TrapOID:       "1.3.6.1.6.3.1.1.5.1",
		TrapName:      "SNMPv2-MIB::coldStart",
		SourceIP:      "192.0.2.30",
		SourceUDPPeer: "192.0.2.30",
		Enrichment: &TrapEnrichmentAudit{Source: &TrapSourceAudit{
			Selected: "192.0.2.30",
			Method:   "udp_peer",
		}},
	}

	rt.update(entry)

	metricStore := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, metricStore, "profile-job")
	labels := profileMetricSourceLabels("192.0.2.30")
	if v, ok := metricStore.Read().Value("snmp_trap_site_cold_start_events", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_site_cold_start_events = %v/%v, want 1/true", v, ok)
	}
}

func TestLoadProfileMergesMetricRulesAndChartsFromExtends(t *testing.T) {
	dir := t.TempDir()
	base := `
varbinds:
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
traps:
  - oid: 1.3.6.1.4.1.9.9.46.2.0.1
    name: BASE-MIB::baseTrap
    category: security
    severity: warning
    varbinds:
      - ifIndex
charts:
  - id: base_chart
    title: Base chart
    context: snmp.trap.base.chart
    units: events/s
    algorithm: incremental
metrics:
  - name: base.metric
    type: counter
    on_trap: BASE-MIB::baseTrap
    output:
      metric: snmp_trap_base_events
      dimension: events
      chart: base_chart
`
	child := `
extends:
  - base.yaml
charts:
  - id: base_chart
    title: Base chart overridden
    context: snmp.trap.base.chart.override
    units: events/s
    algorithm: incremental
  - id: child_chart
    title: Child chart
    context: snmp.trap.child.chart
    units: events/s
    algorithm: incremental
metrics:
  - name: base.metric
    type: counter
    on_trap: BASE-MIB::baseTrap
    output:
      metric: snmp_trap_child_events
      dimension: events
      chart: child_chart
`
	writeProfileYAML(t, dir, "base.yaml", base)
	writeProfileYAML(t, dir, "child.yaml", child)

	bundle, err := loadProfileBundle(filepath.Join(dir, "child.yaml"), multipath.New(dir), nil)
	if err != nil {
		t.Fatalf("loadProfileBundle failed: %v", err)
	}
	if len(bundle.metrics) != 1 {
		t.Fatalf("metrics = %d, want 1 inherited rule overridden by child", len(bundle.metrics))
	}
	if got := bundle.metrics[0].Output.Metric; got != "snmp_trap_child_events" {
		t.Fatalf("overridden metric output = %q, want snmp_trap_child_events", got)
	}
	if len(bundle.charts) != 2 {
		t.Fatalf("charts = %d, want child override of base chart plus child chart", len(bundle.charts))
	}
	byID := make(map[string]profileMetricChart, len(bundle.charts))
	for _, chart := range bundle.charts {
		byID[chart.ID] = chart
	}
	if got := byID["base_chart"].Title; got != "Base chart overridden" {
		t.Fatalf("base_chart title = %q, want child override", got)
	}
}

func TestLoadProfileAcceptsCanonicalMetricSyntax(t *testing.T) {
	dir := t.TempDir()
	profile := `
varbinds:
  ccmHistoryEventTerminalType:
    oid: 1.3.6.1.4.1.9.9.43.1.1.1.2
    type: INTEGER
    enum:
      "2": console
      "3": virtual
  ifIndex:
    oid: 1.3.6.1.2.1.2.2.1.1
    type: INTEGER
traps:
  - oid: 1.3.6.1.4.1.9.9.43.2.0.1
    name: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    category: config_change
    severity: notice
    varbinds:
      - ccmHistoryEventTerminalType
  - oid: 1.3.6.1.4.1.9.9.46.2.0.1
    name: CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation
    category: security
    severity: warning
    varbinds:
      - ifIndex
metrics:
  - name: cisco.config.changed
    type: counter
    on_trap: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    where:
      - varbind: ccmHistoryEventTerminalType
        equals: console
    output:
      metric: snmp_trap_cisco_config_events
      dimension: events
      chart: cisco_config_changes
  - name: cisco.config.changed.by_terminal
    type: counter
    on_trap: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    where:
      - varbind: ccmHistoryEventTerminalType
        in:
          - console
          - virtual
    output:
      metric: snmp_trap_cisco_config_terminal_events
      dimension: terminal_events
      chart: cisco_config_changes
  - name: cisco.config.console_state
    type: state
    on_trap: CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged
    state:
      set_when:
        varbind: ccmHistoryEventTerminalType
        equals: console
      clear_when:
        varbind: ccmHistoryEventTerminalType
        equals: virtual
    output:
      metric: snmp_trap_cisco_console_state
      dimension: active
      chart: cisco_console_state
  - name: cisco.port_security.ifindex
    type: counter
    on_trap: CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation
    identity:
      resource:
        class: interface
        key_from_varbind: ifIndex
        max_per_source: 48
    output:
      metric: snmp_trap_cisco_port_security_violations
      dimension: violations
      chart: port_security_violations
charts:
  - id: cisco_config_changes
    title: Cisco configuration changes
    context: snmp.trap.cisco.config.changes
    units: events/s
    algorithm: incremental
  - id: cisco_console_state
    title: Cisco console state
    context: snmp.trap.cisco.console.state
    units: state
    algorithm: absolute
  - id: port_security_violations
    title: Port security violations
    context: snmp.trap.cisco.port.security.violations
    units: events/s
    algorithm: incremental
`
	writeProfileYAML(t, dir, "profile.yaml", profile)

	bundle, err := loadProfileBundle(filepath.Join(dir, "profile.yaml"), multipath.New(dir), nil)
	if err != nil {
		t.Fatalf("loadProfileBundle failed: %v", err)
	}
	idx := &ProfileIndex{trapsByOID: make(map[string]*TrapDef), namesByTrapName: make(map[string]*TrapDef)}
	if err := idx.addTraps(bundle.traps); err != nil {
		t.Fatalf("addTraps failed: %v", err)
	}
	if err := idx.addProfileMetrics(bundle.metrics, bundle.charts); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	if idx.metricRulesByName["cisco.config.changed"].Output.Dimension != "events" {
		t.Fatalf("canonical counter dimension = %q, want events", idx.metricRulesByName["cisco.config.changed"].Output.Dimension)
	}
	if got := idx.metricRulesByName["cisco.config.changed"].Where; len(got) != 1 || got[0].Varbind != "ccmHistoryEventTerminalType" || got[0].Equals != "console" {
		t.Fatalf("canonical where list = %#v, want ccmHistoryEventTerminalType equals console", got)
	}
	if got := idx.metricRulesByName["cisco.config.changed.by_terminal"].Where; len(got) != 1 || got[0].Varbind != "ccmHistoryEventTerminalType" || !reflect.DeepEqual(got[0].In, []any{"console", "virtual"}) {
		t.Fatalf("canonical where in list = %#v, want ccmHistoryEventTerminalType in console,virtual", got)
	}
	if idx.metricRulesByName["cisco.port_security.ifindex"].Identity.Resource == nil {
		t.Fatalf("canonical identity.resource syntax did not populate identity.resource")
	}
	if idx.metricChartsByID["cisco_config_changes"] == nil {
		t.Fatalf("canonical chart did not create cisco_config_changes chart")
	}
	if got := idx.metricChartsByID["cisco_config_changes"].Type; got != "line" {
		t.Fatalf("canonical chart default type = %q, want line", got)
	}
	if got := idx.metricChartsByID["port_security_violations"].Type; got != "line" {
		t.Fatalf("canonical chart default type = %q, want line", got)
	}
}

func TestLoadProfileRejectsRemovedProfileMetricSyntax(t *testing.T) {
	tests := map[string]string{
		"auto_safe": `    auto_safe: true
`,
		"compact_metric": `    metric: snmp_trap_test_events
`,
		"compact_dimension": `    dimension: events
`,
		"compact_chart_id": `    chart_id: test_events
`,
		"compact_value": `    value: testValue
`,
		"top_level_resource": `    resource:
      class: interface
      key: ifIndex
`,
		"resource_key": `    identity:
      resource:
        class: interface
        key: ifIndex
`,
		"resource_max": `    identity:
      resource:
        class: interface
        key_from_varbind: ifIndex
        max: 48
`,
		"state_varbind": `    state:
      varbind: testValue
`,
		"state_set": `    state:
      set: 1
`,
		"state_clear": `    state:
      clear: 0
`,
		"state_ttl_behavior": `    state:
      ttl_behavior: clear_and_keep
`,
		"chart_meta": `    chart_meta:
      title: Test events
`,
		"map_where": `    where:
      testValue: 1
`,
	}

	for name, removed := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			profile := "metrics:\n  - name: test.removed\n    type: counter\n" + removed
			writeProfileYAML(t, dir, "profile.yaml", profile)
			if _, err := loadProfileBundle(filepath.Join(dir, "profile.yaml"), multipath.New(dir), nil); err == nil {
				t.Fatalf("loadProfileBundle accepted removed metric syntax %s", name)
			}
		})
	}
}

func TestLoadProfileRejectsUnknownProfileMetricChartKeys(t *testing.T) {
	tests := map[string]string{
		"dimensions": `
charts:
  - id: bad_chart
    title: Bad chart
    context: snmp.trap.bad.chart
    units: events/s
    dimensions:
      - selector: snmp_trap_bad
        name: bad
`,
		"lifecycle": `
charts:
  - id: bad_chart
    title: Bad chart
    context: snmp.trap.bad.chart
    units: events/s
    lifecycle:
      typo: bad
`,
	}

	for name, profile := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeProfileYAML(t, dir, "profile.yaml", profile)
			if _, err := loadProfileBundle(filepath.Join(dir, "profile.yaml"), multipath.New(dir), nil); err == nil {
				t.Fatalf("loadProfileBundle accepted unknown chart %s key", name)
			}
		})
	}
}

func TestProfileMetricDiagnosticChartUsesTemplateLocalContext(t *testing.T) {
	chart := profileMetricDiagnosticChart()
	if chart.Context != "profile_metric_diagnostics" {
		t.Fatalf("diagnostic chart context = %q, want profile_metric_diagnostics", chart.Context)
	}
}

func TestProfileMetricValidationResourceClassPolicy(t *testing.T) {
	idx := testProfileMetricIndex(t)
	err := idx.addProfileMetrics([]profileMetricRule{{
		Name:     "cisco.port_security.custom_resource_class",
		Type:     profileMetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "custom", KeyFromVarbind: "ifIndex", MaxPerSource: 48}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_custom_resource_class",
			Dimension: "violations",
			Chart:     "custom_resource_class",
		},
		sourceFile: "test-profile.yaml",
	}}, []profileMetricChart{{
		ID:         "custom_resource_class",
		Title:      "Custom resource class",
		Context:    "snmp.trap.cisco.custom.resource.class",
		Units:      "events/s",
		Algorithm:  "incremental",
		sourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted non-stock resource class without site_ prefix")
	}

	idx = testProfileMetricIndex(t)
	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:     "cisco.port_security.site_resource_class",
		Type:     profileMetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "site_lab_port", KeyFromVarbind: "ifIndex", MaxPerSource: 48}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_site_resource_class",
			Dimension: "violations",
			Chart:     "site_resource_class",
		},
		sourceFile: "test-profile.yaml",
	}}, []profileMetricChart{{
		ID:         "site_resource_class",
		Title:      "Site resource class",
		Context:    "snmp.trap.cisco.site.resource.class",
		Units:      "events/s",
		Algorithm:  "incremental",
		sourceFile: "test-profile.yaml",
	}})
	if err != nil {
		t.Fatalf("addProfileMetrics rejected site-prefixed resource class: %v", err)
	}
}

func TestProfileMetricValidationRejectsNonNumericPredicateBounds(t *testing.T) {
	tests := map[string]struct {
		pred      profileMetricPredicate
		wantError string
	}{
		"greater_than": {
			pred: profileMetricPredicate{
				Varbind:     testCiscoCommandSourceVarbind,
				GreaterThan: "not-a-number",
			},
			wantError: "must be a finite number",
		},
		"less_than": {
			pred: profileMetricPredicate{
				Varbind:  testCiscoCommandSourceVarbind,
				LessThan: "not-a-number",
			},
			wantError: "must be a finite number",
		},
		"range_lower": {
			pred: profileMetricPredicate{
				Varbind: testCiscoCommandSourceVarbind,
				Range:   []any{"not-a-number", 4},
			},
			wantError: "must be a finite number",
		},
		"range_upper": {
			pred: profileMetricPredicate{
				Varbind: testCiscoCommandSourceVarbind,
				Range:   []any{1, "not-a-number"},
			},
			wantError: "must be a finite number",
		},
		"range_reversed": {
			pred: profileMetricPredicate{
				Varbind: testCiscoCommandSourceVarbind,
				Range:   []any{4, 1},
			},
			wantError: "range[0] must be less than or equal to range[1]",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			idx := testProfileMetricIndex(t)
			err := idx.addProfileMetrics([]profileMetricRule{{
				Name:   "cisco.config.bad_" + name,
				Type:   profileMetricTypeCounter,
				OnTrap: testCiscoConfigTrapOID,
				Where:  profileMetricPredicates{tc.pred},
				Output: profileMetricOutput{
					Metric:    "snmp_trap_cisco_bad_" + name,
					Dimension: "bad_" + name,
					Chart:     "cisco_config_changes",
				},
				sourceFile: "test-profile.yaml",
			}}, nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("addProfileMetrics error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

func TestProfileMetricValidationRejectsDuplicateChartDimensions(t *testing.T) {
	idx := testProfileMetricIndex(t)
	err := idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.duplicate_dimension",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_config_duplicate_dimension_events",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		sourceFile: "site-profile.yaml",
	}}, nil)
	if err != nil {
		t.Fatalf("addProfileMetrics rejected alternate same-dimension rule before selection: %v", err)
	}
	cfg, err := normalizeProfileMetricsConfig(ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed", "cisco.config.duplicate_dimension"},
	})
	if err != nil {
		t.Fatalf("normalizeProfileMetricsConfig failed: %v", err)
	}
	_, _, err = newProfileMetricRuntime(cfg, idx, "test")
	if err == nil ||
		!strings.Contains(err.Error(), "reuses output.dimension") ||
		!strings.Contains(err.Error(), "cisco.config.changed") {
		t.Fatalf("newProfileMetricRuntime duplicate dimension error = %v, want rule-specific duplicate dimension error", err)
	}
}

func TestProfileMetricValidationRejectsUnsupportedPublicConfig(t *testing.T) {
	if _, err := normalizeProfileMetricsConfig(ProfileMetricsConfig{
		Enabled: true,
		Include: []string{""},
	}); err == nil {
		t.Fatalf("normalizeProfileMetricsConfig accepted empty include entry")
	}
	if _, err := normalizeProfileMetricsConfig(ProfileMetricsConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed", "cisco.config.changed"},
	}); err == nil {
		t.Fatalf("normalizeProfileMetricsConfig accepted duplicate include entry")
	}

	idx := testProfileMetricIndex(t)
	err := idx.addProfileMetrics([]profileMetricRule{{
		Name:             "cisco.config.bad_missing",
		Type:             profileMetricTypeSample,
		OnTrap:           testCiscoConfigTrapOID,
		ValueFromVarbind: testCiscoTerminalTypeVarbind,
		Missing:          "unknown_dimension",
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_missing",
			Dimension: "value",
			Chart:     "cisco_terminal_type",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted sample missing=unknown_dimension without resource identity")
	}

	err = idx.addProfileMetrics(nil, []profileMetricChart{{
		ID:         "profile_metric_diagnostics",
		Title:      "Reserved diagnostics",
		Context:    "snmp.trap.site.diagnostics",
		Units:      "events/s",
		Algorithm:  "incremental",
		sourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted reserved diagnostics chart id")
	}

	err = idx.addProfileMetrics(nil, []profileMetricChart{{
		ID:         "site_events",
		Title:      "Reserved events context",
		Context:    "snmp.trap.events",
		Units:      "events/s",
		Algorithm:  "incremental",
		sourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted reserved built-in chart context")
	}

	err = idx.addProfileMetrics(nil, []profileMetricChart{{
		ID:         "site_profile_metric_diagnostics",
		Title:      "Reserved profile metric diagnostics context",
		Context:    "snmp.trap.profile_metric_diagnostics",
		Units:      "events/s",
		Algorithm:  "incremental",
		sourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted reserved profile metric diagnostics chart context")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.bad_diagnostic_prefix",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Output: profileMetricOutput{
			Metric:    "snmp_trap_profile_metrics_custom",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted reserved profile metric diagnostics prefix")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.bad_not_absent",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{{
			Varbind: testCiscoTerminalTypeVarbind,
			Absent:  new(true),
			Not:     true,
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_not_absent",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted not with absent predicate")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.bad_range",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{{
			Varbind: testCiscoCommandSourceVarbind,
			Range:   []any{1},
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_range",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted one-sided range predicate")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.bad_empty_predicate",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{{
			Varbind: testCiscoTerminalTypeVarbind,
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_empty_predicate",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted predicate without condition")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.bad_where_varbind",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{{
			Varbind: "missingVarbind",
			Equals:  1,
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_where_varbind",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted where predicate with unknown varbind")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.bad_where_field",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Where: profileMetricPredicates{{
			Field:  "missing_field",
			Equals: "value",
		}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_where_field",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted where predicate with unknown synthetic field")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.bad_state_set_varbind",
		Type:   profileMetricTypeState,
		OnTrap: testCiscoConfigTrapOID,
		State: profileMetricState{
			SetWhen:   &profileMetricPredicate{Varbind: "missingVarbind", Equals: 1},
			ClearWhen: &profileMetricPredicate{Varbind: testCiscoCommandSourceVarbind, Equals: 2},
		},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_state_set_varbind",
			Dimension: "state",
			Chart:     "cisco_terminal_type",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted state.set_when predicate with unknown varbind")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.bad_ttl",
		Type:   profileMetricTypeState,
		OnTrap: testCiscoConfigTrapOID,
		State: profileMetricState{
			SetWhen:   &profileMetricPredicate{Varbind: testCiscoCommandSourceVarbind, Equals: 2},
			ClearWhen: &profileMetricPredicate{Varbind: testCiscoCommandSourceVarbind, Equals: 3},
			TTL:       "not-a-duration",
		},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_ttl",
			Dimension: "state",
			Chart:     "cisco_terminal_type",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted invalid state.ttl")
	}

	for name, ttl := range map[string]string{
		"negative_ttl": "-1h",
		"zero_ttl":     "0s",
	} {
		t.Run(name, func(t *testing.T) {
			idx := testProfileMetricIndex(t)
			err := idx.addProfileMetrics([]profileMetricRule{{
				Name:   "cisco.config." + name,
				Type:   profileMetricTypeState,
				OnTrap: testCiscoConfigTrapOID,
				State: profileMetricState{
					SetWhen:   &profileMetricPredicate{Varbind: testCiscoCommandSourceVarbind, Equals: 2},
					ClearWhen: &profileMetricPredicate{Varbind: testCiscoCommandSourceVarbind, Equals: 3},
					TTL:       ttl,
				},
				Output: profileMetricOutput{
					Metric:    "snmp_trap_cisco_" + name,
					Dimension: "state",
					Chart:     "cisco_terminal_type",
				},
				sourceFile: "test-profile.yaml",
			}}, nil)
			if err == nil || !strings.Contains(err.Error(), "must be greater than zero") {
				t.Fatalf("addProfileMetrics error = %v, want positive state.ttl validation error", err)
			}
		})
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:             "cisco.config.bad_multiplier",
		Type:             profileMetricTypeSample,
		OnTrap:           testCiscoConfigTrapOID,
		ValueFromVarbind: testCiscoTerminalTypeVarbind,
		Scale:            profileMetricScale{Multiplier: -1, Divisor: 1},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_multiplier",
			Dimension: "value",
			Chart:     "cisco_terminal_type",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted negative scale multiplier")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.bad_chart_algorithm",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_chart_algorithm",
			Dimension: "events",
			Chart:     "bad_chart_algorithm",
		},
		sourceFile: "test-profile.yaml",
	}}, []profileMetricChart{{
		ID:         "bad_chart_algorithm",
		Title:      "Bad chart algorithm",
		Context:    "snmp.trap.cisco.bad.chart.algorithm",
		Units:      "events/s",
		Algorithm:  "percentage-of-incremental-row",
		sourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted framework-unsupported chart algorithm")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.bad_chart_type",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_chart_type",
			Dimension: "events",
			Chart:     "bad_chart_type",
		},
		sourceFile: "test-profile.yaml",
	}}, []profileMetricChart{{
		ID:         "bad_chart_type",
		Title:      "Bad chart type",
		Context:    "snmp.trap.cisco.bad.chart.type",
		Units:      "events/s",
		Algorithm:  "incremental",
		Type:       "pie",
		sourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted framework-unsupported chart type")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:   "cisco.config.duplicate_output_metric",
		Type:   profileMetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_config_events",
			Dimension: "duplicate",
			Chart:     "cisco_config_changes",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted duplicate output.metric")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:     "cisco.port_security.bad_negative_resource_cap",
		Type:     profileMetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: -1}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_negative_resource_cap",
			Dimension: "violations",
			Chart:     "port_security_violations",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted negative resource max_per_source")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:     "cisco.config.resource_on_non_resource_chart",
		Type:     profileMetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 48}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_resource_shape",
			Dimension: "events",
			Chart:     "cisco_config_changes",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted mixed resource and non-resource rules on one chart")
	}

	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:     "cisco.port_security.bad_resource_class",
		Type:     profileMetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "peer", KeyFromVarbind: "ifIndex", MaxPerSource: 48}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_resource_class",
			Dimension: "events",
			Chart:     "port_security_violations",
		},
		sourceFile: "test-profile.yaml",
	}}, nil)
	if err == nil {
		t.Fatalf("addProfileMetrics accepted mixed resource classes on one chart")
	}

	const testCiscoUsernameOID = "1.3.6.1.4.1.9.9.43.1.1.1.99"
	configTrap := idx.namesByTrapName["CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged"]
	configTrap.sharedVarbinds[testCiscoUsernameOID] = &VarbindDef{
		OID:     testCiscoUsernameOID,
		Type:    "DisplayString",
		rawName: "ccmHistoryEventUser",
	}
	err = idx.addProfileMetrics([]profileMetricRule{{
		Name:     "cisco.config.bad_string_resource",
		Type:     profileMetricTypeCounter,
		OnTrap:   testCiscoConfigTrapOID,
		Identity: profileMetricIdentity{Resource: &profileMetricResource{Class: "user", KeyFromVarbind: "ccmHistoryEventUser", MaxPerSource: 48}},
		Output: profileMetricOutput{
			Metric:    "snmp_trap_cisco_bad_string_resource",
			Dimension: "events",
			Chart:     "bad_string_resource",
		},
		sourceFile: "test-profile.yaml",
	}}, []profileMetricChart{{
		ID:         "bad_string_resource",
		Title:      "Bad string resource",
		Context:    "snmp.trap.cisco.bad.string.resource",
		Units:      "events/s",
		Algorithm:  "incremental",
		sourceFile: "test-profile.yaml",
	}})
	if err == nil {
		t.Fatalf("addProfileMetrics accepted non-integer resource key varbind")
	}
}
