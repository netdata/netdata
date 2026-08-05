// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"errors"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

func TestProfileMetricRuntimeConcurrentUpdates(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.config.changed"})
	entry := ciscoConfigTrapEntry("profile-job")

	var wg sync.WaitGroup
	for range 25 {
		wg.Go(func() {
			rt.Update(entry)
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
	idx := newPopulatedTestCatalog(t)
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
			rt.Collect(store, "profile-job")
			if err := managed.CycleController().CommitCycleSuccess(); err != nil {
				errCh <- err
				return
			}
		}
	})

	for range 1000 {
		rt.Update(entry)
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
	idx := newPopulatedTestCatalog(t)
	rt := sourceRuntimeWithLimits(t, idx, profileMetricLimitsPolicy{MaxSources: 1})
	first := ciscoConfigTrapEntry(testProfileMetricJobName)
	second := ciscoConfigTrapEntryFromSource(testProfileMetricJobName, "192.0.2.11")

	rt.Update(first)
	rt.Update(second)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.10"), 1)
	assertProfileMetricAbsent(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.11"))
	assertProfileMetricOverflow(t, store, 1)
}

func TestProfileMetricRuntimeMaxInstancesSkipsOnlyNewMetricInstance(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	rt := sourceRuntimeWithLimits(t, idx, profileMetricLimitsPolicy{MaxSources: 10, MaxInstancesPerJob: 1})
	first := ciscoConfigTrapEntry(testProfileMetricJobName)
	second := ciscoConfigTrapEntryFromSource(testProfileMetricJobName, "192.0.2.11")

	rt.Update(first)
	rt.Update(second)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.10"), 1)
	assertProfileMetricAbsent(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.11"))
	assertProfileMetricOverflow(t, store, 1)
}

func TestProfileMetricRuntimeChartMaxInstancesSkipsOnlyNewChartInstance(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withChartLifecycle("cisco_config_changes", charttpl.Lifecycle{MaxInstances: 1, ExpireAfterCycles: 60})
	rt := sourceRuntimeWithLimits(t, idx, profileMetricLimitsPolicy{MaxSources: 10, MaxInstancesPerJob: 10})
	first := ciscoConfigTrapEntry(testProfileMetricJobName)
	second := ciscoConfigTrapEntryFromSource(testProfileMetricJobName, "192.0.2.11")

	rt.Update(first)
	rt.Update(second)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.10"), 1)
	assertProfileMetricAbsent(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.11"))
	assertProfileMetricOverflow(t, store, 1)
}

func TestProfileMetricRuntimeReleasesChartMaxInstancesAfterLifecycleExpiry(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withChartLifecycle("cisco_config_changes", charttpl.Lifecycle{MaxInstances: 1, ExpireAfterCycles: 1})
	rt := sourceRuntimeWithLimits(t, idx, profileMetricLimitsPolicy{MaxSources: 10, MaxInstancesPerJob: 10})
	first := ciscoConfigTrapEntry(testProfileMetricJobName)
	second := ciscoConfigTrapEntryFromSource(testProfileMetricJobName, "192.0.2.11")
	store := metrix.NewCollectorStore()

	rt.Update(first)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	rt.Update(second)
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	assertProfileMetricValue(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.11"), 1)
	assertProfileMetricOverflow(t, store, 0)
}

func TestProfileMetricRuntimeMaxInstancesUsesDeterministicRuleOrder(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withDefinitions(
		[]catalog.MetricRule{
			{
				Name:   "z.tie_a_chart",
				Type:   catalog.MetricTypeCounter,
				OnTrap: testCiscoConfigTrapOID,
				Identity: catalog.MetricIdentity{
					Device: catalog.MetricIdentitySource,
				},
				Output: catalog.MetricOutput{
					Metric:    "snmp_trap_tie_a_chart_events",
					Dimension: "events",
					Chart:     "a_tie_chart",
				},
				Missing: catalog.MetricMissingDrop,
				Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
			},
			{
				Name:   "a.tie_z_chart",
				Type:   catalog.MetricTypeCounter,
				OnTrap: testCiscoConfigTrapOID,
				Identity: catalog.MetricIdentity{
					Device: catalog.MetricIdentitySource,
				},
				Output: catalog.MetricOutput{
					Metric:    "snmp_trap_tie_z_chart_events",
					Dimension: "events",
					Chart:     "z_tie_chart",
				},
				Missing: catalog.MetricMissingDrop,
				Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
			},
		},
		[]catalog.MetricChart{
			{
				ID:        "a_tie_chart",
				Title:     "A tie chart",
				Context:   "snmp.trap.tie.a",
				Units:     "events/s",
				Algorithm: "incremental",
				Type:      "line",
				Lifecycle: &charttpl.Lifecycle{
					MaxInstances:      catalog.DefaultMetricChartMaxInstances,
					ExpireAfterCycles: catalog.DefaultMetricExpireAfterCycles,
				},
			},
			{
				ID:        "z_tie_chart",
				Title:     "Z tie chart",
				Context:   "snmp.trap.tie.z",
				Units:     "events/s",
				Algorithm: "incremental",
				Type:      "line",
				Lifecycle: &charttpl.Lifecycle{
					MaxInstances:      catalog.DefaultMetricChartMaxInstances,
					ExpireAfterCycles: catalog.DefaultMetricExpireAfterCycles,
				},
			},
		},
	)
	rt := newTestProfileMetricRuntimeWithPolicy(t, idx, testRuntimeConfig{
		Enabled: true,
		Include: []string{"a.tie_z_chart", "z.tie_a_chart"},
	}, func(cfg *Policy) {
		cfg.limits.MaxInstancesPerJob = 1
	})

	rt.Update(ciscoConfigTrapEntry("profile-job"))

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
	idx := newPopulatedTestCatalog(t)
	idx = idx.withChartLifecycle("cisco_config_changes", charttpl.Lifecycle{MaxInstances: 10, ExpireAfterCycles: 1})
	rt := sourceRuntimeWithLimits(t, idx, profileMetricLimitsPolicy{MaxSources: 1})
	first := ciscoConfigTrapEntry(testProfileMetricJobName)
	second := ciscoConfigTrapEntryFromSource(testProfileMetricJobName, "192.0.2.11")
	store := metrix.NewCollectorStore()

	rt.Update(first)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	rt.Update(second)
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	assertProfileMetricValue(t, store, "snmp_trap_cisco_config_events", profileMetricSourceLabels("192.0.2.11"), 1)
	assertProfileMetricOverflow(t, store, 0)
}

func TestProfileMetricRuntimeResourceCapSkipsOnlyNewResource(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withDefinitions([]catalog.MetricRule{{
		Name:     "cisco.port_security.ifindex_cap",
		Type:     catalog.MetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: catalog.MetricIdentity{Device: catalog.MetricIdentitySource, Resource: &catalog.MetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 1}},
		Output:   profileMetricOutputForTest("snmp_trap_cisco_port_security_capped_violations", "violations", "port_security_violations"),
		Missing:  catalog.MetricMissingDrop,
		Scale:    catalog.MetricScale{Multiplier: 1, Divisor: 1},
	}}, nil)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.port_security.ifindex_cap"})
	first := portSecurityTrapEntry(7)
	second := portSecurityTrapEntry(8)

	rt.Update(first)
	rt.Update(second)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_port_security_capped_violations", portSecurityResourceLabels("7"), 1)
	assertProfileMetricAbsent(t, store, "snmp_trap_cisco_port_security_capped_violations", portSecurityResourceLabels("8"))
	assertProfileMetricOverflow(t, store, 1)
}

func TestProfileMetricRuntimeReleasesResourceCapAfterLifecycleExpiry(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withChartLifecycle("port_security_violations", charttpl.Lifecycle{MaxInstances: 10, ExpireAfterCycles: 1})
	idx = idx.withDefinitions([]catalog.MetricRule{{
		Name:     "cisco.port_security.ifindex_lifecycle_cap",
		Type:     catalog.MetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: catalog.MetricIdentity{Device: catalog.MetricIdentitySource, Resource: &catalog.MetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 1}},
		Output:   profileMetricOutputForTest("snmp_trap_cisco_port_security_lifecycle_capped_violations", "violations", "port_security_violations"),
		Missing:  catalog.MetricMissingDrop,
		Scale:    catalog.MetricScale{Multiplier: 1, Divisor: 1},
	}}, nil)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.port_security.ifindex_lifecycle_cap"})
	first := portSecurityTrapEntry(7)
	second := portSecurityTrapEntry(8)
	store := metrix.NewCollectorStore()

	rt.Update(first)
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	collectProfileMetricsOnce(t, rt, store, "profile-job")
	rt.Update(second)
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	assertProfileMetricValue(t, store, "snmp_trap_cisco_port_security_lifecycle_capped_violations", portSecurityResourceLabels("8"), 1)
	assertProfileMetricOverflow(t, store, 0)
}

func TestProfileMetricRuntimeResourceCapUsesJobDefault(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withDefinitions([]catalog.MetricRule{{
		Name:     "cisco.port_security.ifindex_job_cap",
		Type:     catalog.MetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Identity: catalog.MetricIdentity{Device: catalog.MetricIdentitySource, Resource: &catalog.MetricResource{Class: "interface", KeyFromVarbind: "ifIndex"}},
		Output:   profileMetricOutputForTest("snmp_trap_cisco_port_security_job_capped_violations", "violations", "port_security_violations"),
		Missing:  catalog.MetricMissingDrop,
		Scale:    catalog.MetricScale{Multiplier: 1, Divisor: 1},
	}}, nil)
	rt := newTestProfileMetricRuntimeWithPolicy(t, idx, testRuntimeConfig{
		Enabled: true,
		Include: []string{"cisco.port_security.ifindex_job_cap"},
	}, func(cfg *Policy) {
		cfg.limits.MaxResourcesPerSource = 1
	})
	first := portSecurityTrapEntry(7)
	second := portSecurityTrapEntry(8)

	rt.Update(first)
	rt.Update(second)

	store := collectProfileMetricStore(t, rt)

	assertProfileMetricValue(t, store, "snmp_trap_cisco_port_security_job_capped_violations", portSecurityResourceLabels("7"), 1)
	assertProfileMetricAbsent(t, store, "snmp_trap_cisco_port_security_job_capped_violations", portSecurityResourceLabels("8"))
	assertProfileMetricOverflow(t, store, 1)
}

func TestProfileMetricRuntimeMissingResourceUnknownDimension(t *testing.T) {
	idx := newPopulatedTestCatalog(t)
	idx = idx.withDefinitions([]catalog.MetricRule{{
		Name:     "cisco.port_security.ifindex_unknown",
		Type:     catalog.MetricTypeCounter,
		OnTrap:   testPortSecurityTrapOID,
		Missing:  catalog.MetricMissingUnknownDimension,
		Identity: catalog.MetricIdentity{Device: catalog.MetricIdentitySource, Resource: &catalog.MetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 2}},
		Output: catalog.MetricOutput{
			Metric:    "snmp_trap_cisco_port_security_unknown_violations",
			Dimension: "violations",
			Chart:     "port_security_violations",
		},
		Scale: catalog.MetricScale{Multiplier: 1, Divisor: 1},
	}}, nil)
	rt := newTestProfileMetricRuntime(t, idx, []string{"cisco.port_security.ifindex_unknown"})
	entry := &model.TrapEntry{
		JobName:       "profile-job",
		TrapOID:       testPortSecurityTrapOID,
		TrapName:      "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
		SourceIP:      "192.0.2.10",
		SourceUDPPeer: "192.0.2.10",
		Enrichment:    &model.TrapEnrichmentAudit{Source: &model.TrapSourceAudit{Selected: "192.0.2.10", Method: "udp_peer"}},
	}

	rt.Update(entry)

	store := metrix.NewCollectorStore()
	collectProfileMetricsOnce(t, rt, store, "profile-job")

	labels := portSecurityResourceLabels("unknown")
	if v, ok := store.Read().Value("snmp_trap_cisco_port_security_unknown_violations", labels); !ok || v != 1 {
		t.Fatalf("snmp_trap_cisco_port_security_unknown_violations = %v/%v, want 1/true", v, ok)
	}
}
