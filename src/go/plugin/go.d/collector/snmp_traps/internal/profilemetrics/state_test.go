// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

func TestProfileMetricRuntimeSameOIDStateRule(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = addProfileMetricRuleWithChart(
		idx,
		catalog.MetricRule{
			Name:   "cisco.config.console_state",
			Type:   catalog.MetricTypeState,
			OnTrap: testCiscoConfigTrapOID,
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			State: catalog.MetricState{
				SetWhen:   &catalog.MetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "console"},
				ClearWhen: &catalog.MetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "virtual"},
			},
			Output:  profileMetricOutputForTest("snmp_trap_cisco_console_session_state", "active", "cisco_console_state"),
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
		profileMetricChartForTest("cisco_console_state", "Cisco console configuration state", "snmp.trap.cisco.console.state", "state", "absolute"),
	)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.console_state"})
	entry := ciscoConfigTrapEntry("profile-job")

	rt.Update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_session_state", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_cisco_console_session_state after set = %v/%v, want 1/true", v, ok)
	}

	entry.Varbinds[1].Value = 3
	rt.Update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_session_state", labels); !ok || v != 0 {
		t.Fatalf("snmp_trap_cisco_console_session_state after clear = %v/%v, want 0/true", v, ok)
	}
}

func TestProfileMetricRuntimeSameOIDStateCustomValuesAndWhere(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = addProfileMetricRuleWithChart(
		idx,
		catalog.MetricRule{
			Name:   "cisco.config.console_custom_state",
			Type:   catalog.MetricTypeState,
			OnTrap: testCiscoConfigTrapOID,
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			Where: catalog.MetricPredicates{{
				Field:  "category",
				Equals: "config_change",
			}},
			State: catalog.MetricState{
				SetWhen:      &catalog.MetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "console"},
				ClearWhen:    &catalog.MetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "virtual"},
				ProblemValue: new(float64(5)),
				ClearValue:   2,
			},
			Output:  profileMetricOutputForTest("snmp_trap_cisco_console_custom_state", "active", "cisco_console_custom_state"),
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
		profileMetricChartForTest("cisco_console_custom_state", "Cisco console custom state", "snmp.trap.cisco.console.custom.state", "state", "absolute"),
	)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.console_custom_state"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.Category = model.Category("config_change")
	store := metrix.NewCollectorStore()
	labels := profileMetricSourceLabels("192.0.2.10")

	rt.Update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_custom_state", labels); !ok || v != 5 {
		t.Fatalf("snmp_trap_cisco_console_custom_state after set = %v/%v, want 5/true", v, ok)
	}

	filtered := ciscoConfigTrapEntry("profile-job")
	filtered.Category = "security"
	filtered.Varbinds[1].Value = 3
	rt.Update(filtered)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_custom_state", labels); !ok || v != 5 {
		t.Fatalf("snmp_trap_cisco_console_custom_state after filtered clear = %v/%v, want 5/true", v, ok)
	}
	if v, ok := store.Read().Value("snmp_trap_profile_metrics_rule_missed", metrix.Labels{"job_name": "profile-job"}); !ok || v != 1 {
		t.Fatalf("snmp_trap_profile_metrics_rule_missed = %v/%v, want 1/true", v, ok)
	}

	entry.Varbinds[1].Value = 3
	rt.Update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_custom_state", labels); !ok || v != 2 {
		t.Fatalf("snmp_trap_cisco_console_custom_state after clear = %v/%v, want 2/true", v, ok)
	}
}

func TestProfileMetricRuntimeSeparateOIDStateRuleSupportsZeroProblemValue(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withTraps(
		&catalog.TrapDef{OID: testLinkUpTrapOID, Name: "IF-MIB::linkUp", Category: "state_change", Severity: "notice"},
	)
	idx = addProfileMetricRuleWithChart(
		idx,
		catalog.MetricRule{
			Name:        "if.link_state",
			Type:        catalog.MetricTypeState,
			ProblemTrap: "IF-MIB::linkDown",
			ClearTrap:   "IF-MIB::linkUp",
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			State: catalog.MetricState{
				ProblemValue: new(float64(0)),
				ClearValue:   1,
			},
			Output:  profileMetricOutputForTest("snmp_trap_if_link_state", "up", "if_link_state"),
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
		profileMetricChartForTest("if_link_state", "Interface link state", "snmp.trap.if.link.state", "state", "absolute"),
	)
	rt := newTestProfileMetricRuntime(t, idx, []string{"if.link_state"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.Varbinds = nil

	entry.TrapOID = testLinkUpTrapOID
	entry.TrapName = "IF-MIB::linkUp"
	rt.Update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_if_link_state", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_if_link_state after clear-before-problem = %v/%v, want 1/true", v, ok)
	}

	entry.TrapOID = testLinkDownTrapOID
	entry.TrapName = "IF-MIB::linkDown"
	rt.Update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_if_link_state", labels); !ok || v != 0 {
		t.Fatalf("snmp_trap_if_link_state after problem = %v/%v, want 0/true", v, ok)
	}

	entry.TrapOID = testLinkUpTrapOID
	entry.TrapName = "IF-MIB::linkUp"
	rt.Update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_if_link_state", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_if_link_state after clear = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricRuntimeStateTTLClearsAndExpires(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = addProfileMetricRuleWithChart(
		idx,
		catalog.MetricRule{
			Name:   "cisco.config.console_state_ttl",
			Type:   catalog.MetricTypeState,
			OnTrap: testCiscoConfigTrapOID,
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			State: catalog.MetricState{
				SetWhen:   &catalog.MetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "console"},
				ClearWhen: &catalog.MetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "virtual"},
				TTL:       "100ms",
			},
			Output:  profileMetricOutputForTest("snmp_trap_cisco_console_ttl_state", "active", "cisco_console_ttl_state"),
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
		profileMetricChartForTest("cisco_console_ttl_state", "Cisco console TTL state", "snmp.trap.cisco.console.ttl.state", "state", "absolute"),
	)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.console_state_ttl"})
	entry := ciscoConfigTrapEntry("profile-job")
	store := metrix.NewCollectorStore()
	labels := profileMetricSourceLabels("192.0.2.10")

	rt.Update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_cisco_console_ttl_state", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_cisco_console_ttl_state after set = %v/%v, want 1/true", v, ok)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		collectProfileMetricsOnce(t, rt, store, "profile-job")
		if v, ok := store.Read().Value("snmp_trap_cisco_console_ttl_state", labels); ok && v == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("snmp_trap_cisco_console_ttl_state did not clear before deadline")
		}
		time.Sleep(10 * time.Millisecond)
	}
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if _, ok := store.Read().Value("snmp_trap_cisco_console_ttl_state", labels); ok {
		t.Fatalf("snmp_trap_cisco_console_ttl_state remained after clear-and-expire")
	}
}
