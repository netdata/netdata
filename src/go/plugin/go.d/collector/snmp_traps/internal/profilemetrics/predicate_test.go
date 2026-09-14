// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

func TestProfileMetricRuntimePredicateFiltersByEnumLabel(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withDefinitions([]catalog.MetricRule{{
		Name:   "cisco.config.console",
		Type:   catalog.MetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Identity: catalog.MetricIdentity{
			Device: catalog.MetricIdentitySource,
		},
		Where: catalog.MetricPredicates{{
			Varbind: testCiscoTerminalTypeVarbind,
			Equals:  "console",
		}},
		Output: catalog.MetricOutput{
			Metric:    "snmp_trap_cisco_console_config_events",
			Dimension: "console_events",
			Chart:     "cisco_config_changes",
		},
		Missing: catalog.MetricMissingDrop,
		Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
	}}, nil)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.console"})
	consoleEntry := ciscoConfigTrapEntry("profile-job")
	virtualEntry := ciscoConfigTrapEntry("profile-job")
	virtualEntry.Varbinds[1].Value = 3

	rt.Update(consoleEntry)
	rt.Update(virtualEntry)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_console_config_events", profileMetricSourceLabels("192.0.2.10"), 1)
	assertProfileMetricValue(t, store, "snmp_trap_profile_metrics_rule_missed", profileMetricJobLabels(), 1)
}

func TestProfileMetricRuntimePredicateFiltersBySyntheticFields(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withDefinitions([]catalog.MetricRule{{
		Name:   "cisco.config.synthetic_fields",
		Type:   catalog.MetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Identity: catalog.MetricIdentity{
			Device: catalog.MetricIdentitySource,
		},
		Where: catalog.MetricPredicates{
			{Field: "category", Equals: "config_change"},
			{Field: "severity", In: []any{"notice"}},
			{Field: "trap_name", Equals: "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged"},
			{Field: "trap_oid", Equals: testCiscoConfigTrapOID},
		},
		Output: catalog.MetricOutput{
			Metric:    "snmp_trap_cisco_config_synthetic_field_events",
			Dimension: "synthetic_field_events",
			Chart:     "cisco_config_changes",
		},
		Missing: catalog.MetricMissingDrop,
		Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
	}}, nil)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.synthetic_fields"})
	pass := ciscoConfigTrapEntry("profile-job")
	pass.Category = model.Category("config_change")
	pass.Severity = model.Severity("notice")
	fail := ciscoConfigTrapEntry("profile-job")
	fail.Category = model.Category("security")
	fail.Severity = model.Severity("notice")

	rt.Update(pass)
	rt.Update(fail)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_config_synthetic_field_events", profileMetricSourceLabels("192.0.2.10"), 1)
	assertProfileMetricValue(t, store, "snmp_trap_profile_metrics_rule_missed", profileMetricJobLabels(), 1)
}

func TestProfileMetricRuntimePredicateOperators(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withDefinitions([]catalog.MetricRule{
		{
			Name:   "cisco.config.rich_predicates",
			Type:   catalog.MetricTypeCounter,
			OnTrap: testCiscoConfigTrapOID,
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			Where: catalog.MetricPredicates{
				{Varbind: testCiscoTerminalTypeVarbind, Exists: new(true)},
				{Varbind: testCiscoTerminalTypeVarbind, In: []any{"console", "virtual"}},
				{Varbind: testCiscoCommandSourceVarbind, GreaterThan: 1},
				{Varbind: testCiscoCommandSourceVarbind, LessThan: 4},
				{Varbind: testCiscoCommandSourceVarbind, Range: []any{2, 3}},
				{Varbind: testCiscoTerminalTypeVarbind, Equals: "aux", Not: true},
			},
			Output: catalog.MetricOutput{
				Metric:    "snmp_trap_cisco_config_rich_predicate_events",
				Dimension: "events",
				Chart:     "cisco_config_changes",
			},
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
		{
			Name:   "cisco.config.absent_predicate",
			Type:   catalog.MetricTypeCounter,
			OnTrap: testCiscoConfigTrapOID,
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			Where: catalog.MetricPredicates{{
				Varbind: "sysUpTime.0",
				Absent:  new(true),
			}},
			Output: catalog.MetricOutput{
				Metric:    "snmp_trap_cisco_config_absent_predicate_events",
				Dimension: "absent_events",
				Chart:     "cisco_config_changes",
			},
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
	}, nil)
	rt := newTestProfileMetricRuntime(t, idx, []string{
		"cisco.config.rich_predicates",
		"cisco.config.absent_predicate",
	})
	pass := ciscoConfigTrapEntry("profile-job")
	pass.Varbinds = pass.Varbinds[:2]
	fail := ciscoConfigTrapEntry("profile-job")
	fail.Varbinds[1].Value = 4

	rt.Update(pass)
	rt.Update(fail)

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
	idx := newPopulatedTestCatalog(t)
	idx = idx.withDefinitions([]catalog.MetricRule{
		{
			Name:   "cisco.config.exists_false",
			Type:   catalog.MetricTypeCounter,
			OnTrap: testCiscoConfigTrapOID,
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			Where: catalog.MetricPredicates{{
				Varbind: "sysUpTime.0",
				Exists:  new(false),
			}},
			Output: catalog.MetricOutput{
				Metric:    "snmp_trap_cisco_config_exists_false_events",
				Dimension: "exists_false_events",
				Chart:     "cisco_config_changes",
			},
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
		{
			Name:   "cisco.config.numeric_in",
			Type:   catalog.MetricTypeCounter,
			OnTrap: testCiscoConfigTrapOID,
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			Where: catalog.MetricPredicates{{
				Varbind: testCiscoCommandSourceVarbind,
				In:      []any{2, 3},
			}},
			Output: catalog.MetricOutput{
				Metric:    "snmp_trap_cisco_config_numeric_in_events",
				Dimension: "numeric_in_events",
				Chart:     "cisco_config_changes",
			},
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
		{
			Name:   "cisco.config.synthetic_not",
			Type:   catalog.MetricTypeCounter,
			OnTrap: testCiscoConfigTrapOID,
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			Where: catalog.MetricPredicates{{
				Field:  "category",
				Equals: "security",
				Not:    true,
			}},
			Output: catalog.MetricOutput{
				Metric:    "snmp_trap_cisco_config_synthetic_not_events",
				Dimension: "synthetic_not_events",
				Chart:     "cisco_config_changes",
			},
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
	}, nil)
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

	rt.Update(pass)
	rt.Update(fail)

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
	idx := newPopulatedTestCatalog(t)
	idx = idx.withDefinitions([]catalog.MetricRule{{
		Name:   "cisco.config.finite_range",
		Type:   catalog.MetricTypeCounter,
		OnTrap: testCiscoConfigTrapOID,
		Identity: catalog.MetricIdentity{
			Device: catalog.MetricIdentitySource,
		},
		Where: catalog.MetricPredicates{{
			Varbind: testCiscoCommandSourceVarbind,
			Range:   []any{1, 4},
		}},
		Output: catalog.MetricOutput{
			Metric:    "snmp_trap_cisco_config_finite_range_events",
			Dimension: "finite_range_events",
			Chart:     "cisco_config_changes",
		},
		Missing: catalog.MetricMissingDrop,
		Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
	}}, nil)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.finite_range"})

	for _, value := range []any{"NaN", "+Inf", "-Inf"} {
		entry := ciscoConfigTrapEntry("profile-job")
		entry.Varbinds[0].Value = value
		rt.Update(entry)
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
