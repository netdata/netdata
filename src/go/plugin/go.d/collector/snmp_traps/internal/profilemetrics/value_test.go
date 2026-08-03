// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

func TestProfileMetricRuntimeIncludedCounterBySource(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")

	rt.Update(entry)
	rt.Update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_config_events", labels); !ok || v != 2 {
		t.Fatalf("snmp_trap_cisco_config_events = %v/%v, want 2/true", v, ok)
	}
}

func TestProfileMetricRuntimeIncludedSampleUsesVarbindValue(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.terminal_type"})
	entry := ciscoConfigTrapEntry("profile-job")

	rt.Update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_terminal_type", labels); !ok || v != 2 {
		t.Fatalf("snmp_trap_cisco_terminal_type = %v/%v, want 2/true", v, ok)
	}
}

func TestProfileMetricRuntimeSampleWherePredicate(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	if err := idx.addDefinitions([]profileMetricRule{{
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
		SourceFile: "test-profile.yaml",
	}}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.console_terminal_type"})
	pass := ciscoConfigTrapEntry("profile-job")
	fail := ciscoConfigTrapEntry("profile-job")
	fail.Varbinds[1].Value = 3

	rt.Update(pass)
	rt.Update(fail)

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
	idx := newPopulatedTestCatalog(t)
	profileMetricChartFromIndex(t, idx, "cisco_terminal_type").Lifecycle = &charttpl.Lifecycle{MaxInstances: 10, ExpireAfterCycles: 3}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.terminal_type"})
	entry := ciscoConfigTrapEntry("profile-job")
	store := metrix.NewCollectorStore()
	labels := profileMetricSourceLabels("192.0.2.10")

	rt.Update(entry)
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
	idx := newPopulatedTestCatalog(t)
	if err := idx.addDefinitions([]profileMetricRule{{
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
		SourceFile: "test-profile.yaml",
	}}, nil); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.terminal_type_scaled"})
	entry := ciscoConfigTrapEntry("profile-job")

	rt.Update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_terminal_type_scaled", labels); !ok || v != 10 {
		t.Fatalf("snmp_trap_cisco_terminal_type_scaled = %v/%v, want 10/true", v, ok)
	}

	entry.Varbinds = entry.Varbinds[:1]
	rt.Update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_terminal_type_scaled", labels); !ok || v != 0 {
		t.Fatalf("snmp_trap_cisco_terminal_type_scaled after missing varbind = %v/%v, want 0/true", v, ok)
	}
}

func TestProfileMetricRuntimeConvertsTimeTicksSamplesToSeconds(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	if err := idx.addDefinitions([]profileMetricRule{{
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
		SourceFile: "test-profile.yaml",
	}}, []profileMetricChart{{
		ID:         "cisco_sysuptime_seconds",
		Title:      "Cisco sysUpTime seconds",
		Context:    "snmp.trap.cisco.sysuptime.seconds",
		Units:      "seconds",
		Algorithm:  "absolute",
		Type:       "line",
		SourceFile: "test-profile.yaml",
	}}); err != nil {
		t.Fatalf("addProfileMetrics failed: %v", err)
	}
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.sysuptime_seconds"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.Varbinds[2].Value = uint64(10000)

	rt.Update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_sysuptime_scaled_seconds", labels); !ok || v != 200 {
		t.Fatalf("snmp_trap_cisco_sysuptime_scaled_seconds = %v/%v, want 200/true", v, ok)
	}
}

func TestProfileMetricRuntimeMissingDropAndErrorDiagnostics(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	if err := idx.addDefinitions([]profileMetricRule{
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
			SourceFile: "test-profile.yaml",
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
			SourceFile: "test-profile.yaml",
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

	rt.Update(entry)

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
