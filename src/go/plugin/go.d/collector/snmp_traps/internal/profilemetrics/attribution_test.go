// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"encoding/hex"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

func TestProfileMetricRuntimeUsesVnodeHostScopeWhenAvailable(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.SourceVnodeID = "vnode-1"
	entry.DeviceHostname = "switch-1"

	rt.Update(entry)

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
	idx := newPopulatedTestCatalog(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.SourceVnodeID = "vnode-1"
	entry.DeviceHostname = "switch-1"
	entry.Enrichment.Topology = &model.TrapEnrichmentLookup{
		Status: "conflict",
		Reason: "vnode_mismatch",
	}

	rt.Update(entry)

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
	idx := newPopulatedTestCatalog(t)
	cfg, err := normalizeTestRuntimeConfig(testRuntimeConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	})
	if err != nil {
		t.Fatalf("normalizeTestRuntimeConfig failed: %v", err)
	}
	rt, _, err := newTestRuntime(cfg, idx, "test")
	if err != nil {
		t.Fatalf("newTestRuntime failed: %v", err)
	}
	entry := ciscoConfigTrapEntry("profile-job")

	rt.Update(entry)

	sourceID, sourceKind := fallbackTrapSourceIdentity(entry, entry.JobName, rt.sourceHashSalt)
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

func TestProfileMetricRuntimeSourceLabelIgnoresVnodeScope(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	rt := newTestProfileMetricRuntimeWithPolicy(t, idx, testRuntimeConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	}, func(cfg *Policy) {
		cfg.identity.Device = catalog.MetricIdentitySourceLabel
	})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.SourceVnodeID = "vnode-1"
	entry.DeviceHostname = "switch-1"

	rt.Update(entry)

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
	idx := newPopulatedTestCatalog(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")
	entry.SourceIP = ""
	entry.SourceUDPPeer = ""
	entry.Enrichment = nil

	rt.Update(entry)

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
	idx := newPopulatedTestCatalog(t)
	rt := newTestProfileMetricRuntimeWithPolicy(t, idx, testRuntimeConfig{
		Enabled: true,
		Include: []string{"cisco.config.changed"},
	}, func(cfg *Policy) {
		cfg.identity.Device = catalog.MetricIdentityListener
	})
	first := ciscoConfigTrapEntry("profile-job")
	second := ciscoConfigTrapEntry("profile-job")
	second.SourceIP = "192.0.2.11"
	second.SourceUDPPeer = "192.0.2.11"
	second.Enrichment.Source.Selected = "192.0.2.11"

	rt.Update(first)
	rt.Update(second)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := metrix.Labels{"job_name": "profile-job", "source_id": "profile-job", "source_kind": "listener"}
	if v, ok := store.Read().Value("snmp_trap_cisco_config_events", labels); !ok || v != 2 {
		t.Fatalf("listener-scoped snmp_trap_cisco_config_events = %v/%v, want 2/true", v, ok)
	}
}

func TestProfileMetricRuntimeCountsSourceRouteTransitions(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")

	rt.Update(entry)
	entry.SourceVnodeID = "vnode-1"
	entry.DeviceHostname = "switch-1"
	rt.Update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	if v, ok := store.Read().Value("snmp_trap_profile_metrics_source_transitions", metrix.Labels{"job_name": "profile-job"}); !ok || v != 1 {
		t.Fatalf("snmp_trap_profile_metrics_source_transitions = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricRuntimeResourceIdentity(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.port_security.ifindex"})
	entry := &model.TrapEntry{
		JobName:       "profile-job",
		TrapOID:       testPortSecurityTrapOID,
		TrapName:      "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment: &model.TrapEnrichmentAudit{Source: &model.TrapSourceAudit{
			Selected: "192.0.2.10",
			Method:   "udp_peer",
		}},
		Varbinds: []model.VarbindValue{
			{OID: testIfIndexOID, Type: "INTEGER", Value: 7},
		},
	}

	rt.Update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := portSecurityResourceLabels("7")
	if v, ok := store.Read().Value("snmp_trap_cisco_port_security_violations", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_cisco_port_security_violations = %v/%v, want 1/true", v, ok)
	}
}

func TestProfileMetricChartTemplateUsesResourceLabels(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	cfg, err := normalizeTestRuntimeConfig(testRuntimeConfig{
		Enabled: true,
		Include: []string{"cisco.port_security.ifindex"},
	})
	if err != nil {
		t.Fatalf("normalizeTestRuntimeConfig failed: %v", err)
	}
	_, tmpl, err := newTestRuntime(cfg, idx, "test")
	if err != nil {
		t.Fatalf("newTestRuntime failed: %v", err)
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
