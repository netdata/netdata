// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

// BenchmarkProfileMetricRuntimeUpdateAndCollect exercises the profile-metric
// hot path near configured caps: rule evaluation, hash-mode source identity,
// resource cap checks, state updates, metric collection, and TTL sweep.
func BenchmarkProfileMetricRuntimeUpdateAndCollect(b *testing.B) {
	idx := benchmarkProfileMetricCatalog()
	cfg, err := normalizeTestRuntimeConfig(testRuntimeConfig{
		Enabled: true,
		Include: []string{
			"bench.config.changed",
			"bench.config.terminal_type",
			"bench.config.console_state",
			"bench.port_security.ifindex",
		},
	})
	if err != nil {
		b.Fatalf("normalizeTestRuntimeConfig: %v", err)
	}
	cfg.limits = profileMetricLimitsPolicy{
		MaxRules:              4,
		MaxSources:            64,
		MaxResourcesPerSource: 32,
		MaxInstancesPerJob:    4096,
	}
	rt, _, err := newTestRuntime(cfg, idx, "benchmark")
	if err != nil {
		b.Fatalf("newTestRuntime: %v", err)
	}
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		b.Fatal("metrix.AsCycleManagedStore returned false")
	}

	const (
		nearMaxSources   = 63
		nearMaxResources = 31
	)
	for i := range nearMaxSources {
		rt.Update(benchmarkProfileMetricConfigTrapEntry("bench-profile", benchmarkSourceIP(i), 2))
	}
	for i := range nearMaxResources {
		rt.Update(benchmarkProfileMetricPortTrapEntry("bench-profile", "10.254.0.1", i+1))
	}

	entries := make([]*model.TrapEntry, 0, nearMaxSources+nearMaxResources)
	for i := range nearMaxSources {
		terminalType := 2
		if i%2 == 1 {
			terminalType = 3
		}
		entries = append(entries, benchmarkProfileMetricConfigTrapEntry("bench-profile", benchmarkSourceIP(i), terminalType))
	}
	for i := range nearMaxResources {
		entries = append(entries, benchmarkProfileMetricPortTrapEntry("bench-profile", "10.254.0.1", i+1))
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.Update(entries[i%len(entries)])
		managed.CycleController().BeginCycle()
		rt.Collect(store, "bench-profile")
		if err := managed.CycleController().CommitCycleSuccess(); err != nil {
			b.Fatalf("CommitCycleSuccess: %v", err)
		}
	}
	b.StopTimer()

	rt.mu.Lock()
	seriesCount := len(rt.series)
	sourceCount := len(rt.sources)
	resourceGroupCount := len(rt.resources)
	rt.mu.Unlock()
	b.ReportMetric(float64(seriesCount), "series")
	b.ReportMetric(float64(sourceCount), "sources")
	b.ReportMetric(float64(resourceGroupCount), "resource_groups")
	if elapsed := b.Elapsed().Seconds(); elapsed > 0 {
		b.ReportMetric(float64(b.N)/elapsed, "cycles/s")
	}
}

func benchmarkProfileMetricCatalog() *staticTestCatalog {
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
			OID:         testCiscoConfigTrapOID,
			Name:        "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
			Category:    "config_change",
			Severity:    "notice",
			VarbindRefs: []any{testCiscoTerminalTypeVarbind, "sysUpTime.0"},
			SharedVarbinds: map[string]*catalog.VarbindDef{
				testCiscoTerminalTypeOID: terminalType,
				model.SysUpTimeOID:       sysUpTime,
			},
		},
		{
			OID:            testPortSecurityTrapOID,
			Name:           "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
			Category:       "security",
			Severity:       "warning",
			VarbindRefs:    []any{"ifIndex"},
			SharedVarbinds: map[string]*catalog.VarbindDef{testIfIndexOID: ifIndex},
		},
	}

	rules := []catalog.MetricRule{
		{
			Name:   "bench.config.changed",
			Type:   catalog.MetricTypeCounter,
			OnTrap: testCiscoConfigTrapOID,
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			Output:  catalog.MetricOutput{Metric: "snmp_trap_bench_config_events", Dimension: "events", Chart: "bench_config_changes"},
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
		{
			Name:             "bench.config.terminal_type",
			Type:             catalog.MetricTypeSample,
			OnTrap:           testCiscoConfigTrapOID,
			ValueFromVarbind: testCiscoTerminalTypeVarbind,
			Identity:         catalog.MetricIdentity{Device: catalog.MetricIdentitySource},
			Output:           catalog.MetricOutput{Metric: "snmp_trap_bench_terminal_type", Dimension: "terminal_type", Chart: "bench_terminal_type"},
			Missing:          catalog.MetricMissingDrop,
			Scale:            catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
		{
			Name:   "bench.config.console_state",
			Type:   catalog.MetricTypeState,
			OnTrap: testCiscoConfigTrapOID,
			Identity: catalog.MetricIdentity{
				Device: catalog.MetricIdentitySource,
			},
			State: catalog.MetricState{
				SetWhen:   &catalog.MetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "console"},
				ClearWhen: &catalog.MetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "virtual"},
				TTL:       "1ns",
			},
			Output:  catalog.MetricOutput{Metric: "snmp_trap_bench_console_state", Dimension: "active", Chart: "bench_console_state"},
			Missing: catalog.MetricMissingDrop,
			Scale:   catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
		{
			Name:     "bench.port_security.ifindex",
			Type:     catalog.MetricTypeCounter,
			OnTrap:   testPortSecurityTrapOID,
			Identity: catalog.MetricIdentity{Device: catalog.MetricIdentitySource, Resource: &catalog.MetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 32}},
			Output:   catalog.MetricOutput{Metric: "snmp_trap_bench_port_security_violations", Dimension: "violations", Chart: "bench_port_security"},
			Missing:  catalog.MetricMissingDrop,
			Scale:    catalog.MetricScale{Multiplier: 1, Divisor: 1},
		},
	}
	charts := []catalog.MetricChart{
		{ID: "bench_config_changes", Title: "Benchmark config changes", Context: "snmp.trap.bench.config.changes", Units: "events/s", Algorithm: "incremental", Type: "line", Lifecycle: &charttpl.Lifecycle{MaxInstances: catalog.DefaultMetricChartMaxInstances, ExpireAfterCycles: 256}},
		{ID: "bench_terminal_type", Title: "Benchmark terminal type", Context: "snmp.trap.bench.terminal.type", Units: "type", Algorithm: "absolute", Type: "line", Lifecycle: &charttpl.Lifecycle{MaxInstances: catalog.DefaultMetricChartMaxInstances, ExpireAfterCycles: 256}},
		{ID: "bench_console_state", Title: "Benchmark console state", Context: "snmp.trap.bench.console.state", Units: "state", Algorithm: "absolute", Type: "line", Lifecycle: &charttpl.Lifecycle{MaxInstances: catalog.DefaultMetricChartMaxInstances, ExpireAfterCycles: 256}},
		{ID: "bench_port_security", Title: "Benchmark port security", Context: "snmp.trap.bench.port.security", Units: "events/s", Algorithm: "incremental", Type: "line", Lifecycle: &charttpl.Lifecycle{MaxInstances: catalog.DefaultMetricChartMaxInstances, ExpireAfterCycles: 256}},
	}
	return newStaticTestCatalog(traps, rules, charts)
}

func benchmarkProfileMetricConfigTrapEntry(jobName, sourceIP string, terminalType int) *model.TrapEntry {
	return &model.TrapEntry{
		JobName:       jobName,
		TrapOID:       testCiscoConfigTrapOID,
		TrapName:      "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
		SourceIP:      sourceIP,
		SourceUDPPeer: sourceIP,
		Enrichment: &model.TrapEnrichmentAudit{Source: &model.TrapSourceAudit{
			Selected: sourceIP,
			Method:   "udp_peer",
		}},
		Varbinds: []model.VarbindValue{
			{OID: testCiscoTerminalTypeOID, Type: "INTEGER", Value: terminalType},
			{OID: model.SysUpTimeOID, Type: "TimeTicks", Value: uint64(12345)},
		},
	}
}

func benchmarkProfileMetricPortTrapEntry(jobName, sourceIP string, ifIndex int) *model.TrapEntry {
	return &model.TrapEntry{
		JobName:       jobName,
		TrapOID:       testPortSecurityTrapOID,
		TrapName:      "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
		SourceIP:      sourceIP,
		SourceUDPPeer: sourceIP,
		Enrichment: &model.TrapEnrichmentAudit{Source: &model.TrapSourceAudit{
			Selected: sourceIP,
			Method:   "udp_peer",
		}},
		Varbinds: []model.VarbindValue{
			{OID: testIfIndexOID, Type: "INTEGER", Value: ifIndex},
		},
	}
}

func benchmarkSourceIP(i int) string {
	return fmt.Sprintf("10.%d.%d.%d", 100+(i/65025), (i/255)%255, i%255+1)
}
