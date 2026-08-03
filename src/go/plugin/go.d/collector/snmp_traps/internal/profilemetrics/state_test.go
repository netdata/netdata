// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func TestProfileMetricRuntimeSameOIDStateRule(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
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
			SourceFile: "test-profile.yaml",
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
			SourceFile: "test-profile.yaml",
		},
		profileMetricChartForTest("cisco_console_custom_state", "Cisco console custom state", "snmp.trap.cisco.console.custom.state", "state", "absolute"),
	)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.console_custom_state"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.Category = testCategory("config_change")
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
	if err := idx.addTraps([]*testTrapDef{
		{OID: testLinkDownTrapOID, Name: "SNMPv2-MIB::linkDown", Category: "state_change", Severity: "warning", SourceFile: "test-profile.yaml"},
		{OID: testLinkUpTrapOID, Name: "SNMPv2-MIB::linkUp", Category: "state_change", Severity: "notice", SourceFile: "test-profile.yaml"},
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
			SourceFile: "test-profile.yaml",
		},
		profileMetricChartForTest("if_link_state", "Interface link state", "snmp.trap.if.link.state", "state", "absolute"),
	)
	rt := newTestProfileMetricRuntime(t, idx, []string{"if.link_state"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.Varbinds = nil

	entry.TrapOID = testLinkUpTrapOID
	entry.TrapName = "SNMPv2-MIB::linkUp"
	rt.Update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := profileMetricSourceLabels("192.0.2.10")
	if v, ok := store.Read().Value("snmp_trap_if_link_state", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_if_link_state after clear-before-problem = %v/%v, want 1/true", v, ok)
	}

	entry.TrapOID = testLinkDownTrapOID
	entry.TrapName = "SNMPv2-MIB::linkDown"
	rt.Update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_if_link_state", labels); !ok || v != 0 {
		t.Fatalf("snmp_trap_if_link_state after problem = %v/%v, want 0/true", v, ok)
	}

	entry.TrapOID = testLinkUpTrapOID
	entry.TrapName = "SNMPv2-MIB::linkUp"
	rt.Update(entry)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	if v, ok := store.Read().Value("snmp_trap_if_link_state", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_if_link_state after clear = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricRuntimeStateTTLClearsAndExpires(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
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
				TTL:       "100ms",
			},
			Output:     profileMetricOutputForTest("snmp_trap_cisco_console_ttl_state", "active", "cisco_console_ttl_state"),
			SourceFile: "test-profile.yaml",
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
