// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/multipath"
)

// ---------------------------------------------------------------------------
// M1 benchmark harness
// ---------------------------------------------------------------------------

// buildBenchV2cTrap generates a synthetic SNMPv2c trap packet.
func buildBenchV2cTrap(b testing.TB, community, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	b.Helper()
	x := &gosnmp.GoSNMP{Version: gosnmp.Version2c, Community: community}
	pdus := []gosnmp.SnmpPDU{
		{Name: sysUpTimeOID, Type: gosnmp.TimeTicks, Value: uint32(10)},
		{Name: snmpTrapOIDOID, Type: gosnmp.ObjectIdentifier, Value: trapOID},
	}
	pdus = append(pdus, extra...)
	data, err := x.MkSnmpPacket(gosnmp.SNMPv2Trap, pdus, 0, 0).MarshalMsg()
	if err != nil {
		b.Fatalf("marshal benchmark v2c trap: %v", err)
	}
	return data
}

// setBenchProfileIndex seeds the global profile index for benchmarks.
func setBenchProfileIndex(b *testing.B, traps map[string]*TrapDef) {
	b.Helper()
	// Benchmark-only shortcut: swap the atomic current index without touching
	// the lazy-load/refcount state used by production job creation.
	prev := globalProfileCache.current.Load()
	globalProfileCache.current.Store(&ProfileIndex{trapsByOID: traps})
	b.Cleanup(func() { globalProfileCache.current.Store(prev) })
}

// countingWriter is an in-memory sink that counts trap entries without
// unbounded storage growth, suitable for high-iteration benchmarks.
type countingWriter struct {
	count  int64
	closed int32
}

func (w *countingWriter) Write(entry *TrapEntry) error {
	if atomic.LoadInt32(&w.closed) != 0 {
		return errWriterClosed
	}
	atomic.AddInt64(&w.count, 1)
	return nil
}

func (w *countingWriter) Flush() error   { return nil }
func (w *countingWriter) Close() error   { atomic.StoreInt32(&w.closed, 1); return nil }
func (w *countingWriter) Written() int64 { return atomic.LoadInt64(&w.count) }

var _ TrapWriter = (*countingWriter)(nil)

// benchMakePDUs creates n synthetic PDU values with integer types.
func benchMakePDUs(n int) []gosnmp.SnmpPDU {
	out := make([]gosnmp.SnmpPDU, n)
	for i := range out {
		out[i] = gosnmp.SnmpPDU{
			Name:  fmt.Sprintf("1.3.6.1.2.1.2.2.1.%d", i+1),
			Type:  gosnmp.Integer,
			Value: int(i),
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// 1. Decode-only throughput benchmarks (varying varbind count)
// ---------------------------------------------------------------------------

func BenchmarkDecodeTrap(b *testing.B) {
	cases := map[string]int{
		"Varbinds=2":   0,  // baseline: 2 total varbinds (sysUpTime + trapOID)
		"Varbinds=5":   3,  // 3 extra (5 total)
		"Varbinds=10":  8,  // 8 extra (10 total)
		"Varbinds=20":  18, // 18 extra (20 total)
		"Varbinds=50":  48, // 48 extra (50 total)
		"Varbinds=256": 254,
	}
	for name, n := range cases {
		b.Run(name, func(b *testing.B) {
			data := buildBenchV2cTrap(b, "public", "1.3.6.1.6.3.1.1.5.1", benchMakePDUs(n)...)
			peer := net.ParseIP("10.1.2.3")
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = DecodeTrap(data, peer, nil)
			}
			b.StopTimer()
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "traps/s")
		})
	}
}

// ---------------------------------------------------------------------------
// 2. End-to-end packet path through Collector.handlePacket
// ---------------------------------------------------------------------------

func BenchmarkPacketTrap(b *testing.B) {
	data := buildBenchV2cTrap(b, "public", "1.3.6.1.6.3.1.1.5.1",
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: "test-device"},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(123456)},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.1.1", Type: gosnmp.Integer, Value: 1},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.1.2", Type: gosnmp.Integer, Value: 2},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.2.1", Type: gosnmp.OctetString, Value: "Gi0/1"},
	)
	peer := net.ParseIP("10.1.2.3")

	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "coldStart from {TRAP_SOURCE_IP}",
	}
	setBenchProfileIndex(b, map[string]*TrapDef{trap.OID: trap})

	const jobName = "bench-pkt"

	writer := &countingWriter{}
	c := &Collector{
		jobName:    jobName,
		trapWriter: writer,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:  NewAllowlist(nil, []string{"public"}),
		metrics:    &perJobMetrics{},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkt := make([]byte, len(data))
		copy(pkt, data)
		c.handlePacket(pkt, peer, nil, nil)
	}
	b.StopTimer()
	written := writer.Written()
	// Zero writes means the synthetic packet/profile setup is broken; non-zero
	// drops are reported as metrics because decode-budget drops depend on load.
	if written == 0 {
		b.Fatal("expected at least one written entry, got 0")
	}
	reportDrops(b, int64(b.N), written)
	b.ReportMetric(float64(written)/b.Elapsed().Seconds(), "entries/s")
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "packets/s")
}

// ---------------------------------------------------------------------------
// 3. Multi-job scale shape (independent collectors, shared source distribution)
// ---------------------------------------------------------------------------

func BenchmarkMultiJob(b *testing.B) {
	for _, numJobs := range []int{1, 4, 10} {
		b.Run(fmt.Sprintf("N=%d", numJobs), func(b *testing.B) {
			// Keep the packet minimal so this benchmark isolates per-job
			// partition shape; BenchmarkPacketTrap carries extra varbind cost.
			data := buildBenchV2cTrap(b, "public", "1.3.6.1.6.3.1.1.5.1")

			trap := &TrapDef{
				OID:         "1.3.6.1.6.3.1.1.5.1",
				Name:        "TEST-MIB::coldStart",
				Category:    "security",
				Severity:    "warning",
				Description: "coldStart from {TRAP_SOURCE_IP}",
			}
			setBenchProfileIndex(b, map[string]*TrapDef{trap.OID: trap})

			writers := make([]*countingWriter, numJobs)
			collectors := make([]*Collector, numJobs)
			peers := make([]net.IP, numJobs)
			for i := range numJobs {
				jn := fmt.Sprintf("bm-multi-%d", i)
				peers[i] = net.ParseIP(fmt.Sprintf("10.1.2.%d", i+1))
				writers[i] = &countingWriter{}
				collectors[i] = &Collector{
					jobName:    jn,
					trapWriter: writers[i],
					versions:   map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
					allowlist:  NewAllowlist(nil, []string{"public"}),
					metrics:    &perJobMetrics{},
				}
			}

			b.ReportAllocs()
			b.ResetTimer()
			var wg sync.WaitGroup
			perJob := b.N / numJobs
			remainder := b.N % numJobs
			for i := range numJobs {
				count := perJob
				if i < remainder {
					count++
				}
				wg.Add(1)
				go func(jobIdx, iterations int) {
					defer wg.Done()
					for range iterations {
						pkt := make([]byte, len(data))
						copy(pkt, data)
						collectors[jobIdx].handlePacket(pkt, peers[jobIdx], nil, nil)
					}
				}(i, count)
			}
			wg.Wait()
			b.StopTimer()

			var total int64
			for _, w := range writers {
				total += w.Written()
			}
			// Zero writes means the synthetic packet/profile setup is broken;
			// normal decode-budget drops are reported instead of hard-failed.
			if total == 0 {
				b.Fatal("expected at least one written entry, got 0")
			}
			reportDrops(b, int64(b.N), total)
			elapsed := b.Elapsed().Seconds()
			if elapsed > 0 {
				b.ReportMetric(float64(total)/elapsed, "entries/s")
				b.ReportMetric(float64(b.N)/elapsed, "packets/s")
			}
		})
	}
}

func reportDrops(b *testing.B, packets, entries int64) {
	b.Helper()
	drops := max(packets-entries, 0)
	b.ReportMetric(float64(drops), "drops")
	if packets > 0 {
		b.ReportMetric(100*float64(drops)/float64(packets), "drop_pct")
	}
}

// BenchmarkProfileMetricRuntimeUpdateAndCollect exercises the profile-metric
// hot path near configured caps: rule evaluation, hash-mode source identity,
// resource cap checks, state updates, metric collection, and TTL sweep.
func BenchmarkProfileMetricRuntimeUpdateAndCollect(b *testing.B) {
	idx := benchmarkProfileMetricIndex(b)
	cfg, err := normalizeProfileMetricsConfig(ProfileMetricsConfig{
		Enabled: true,
		Mode:    profileMetricModeExact,
		Include: []string{
			"bench.config.changed",
			"bench.config.terminal_type",
			"bench.config.console_state",
			"bench.port_security.ifindex",
		},
		Identity: ProfileMetricIdentityConfig{
			SourceIDPrivacy: profileMetricSourceIDHash,
		},
		Limits: ProfileMetricLimitsConfig{
			MaxRules:              4,
			MaxSources:            64,
			MaxResourcesPerSource: 32,
			MaxInstancesPerJob:    4096,
		},
	})
	if err != nil {
		b.Fatalf("normalizeProfileMetricsConfig: %v", err)
	}
	rt, _, err := newProfileMetricRuntime(cfg, idx)
	if err != nil {
		b.Fatalf("newProfileMetricRuntime: %v", err)
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
		rt.update(benchmarkProfileMetricConfigTrapEntry("bench-profile", benchmarkSourceIP(i), 2))
	}
	for i := range nearMaxResources {
		rt.update(benchmarkProfileMetricPortTrapEntry("bench-profile", "10.254.0.1", i+1))
	}

	entries := make([]*TrapEntry, 0, nearMaxSources+nearMaxResources)
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
		rt.update(entries[i%len(entries)])
		managed.CycleController().BeginCycle()
		rt.collect(store, "bench-profile")
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

// BenchmarkPipelineSourceMetricsUpdateAndCollect exercises the built-in
// receiver/pipeline source-metric hot path near the internal source cap.
func BenchmarkPipelineSourceMetricsUpdateAndCollect(b *testing.B) {
	const jobName = "bench-pipeline"
	metrics := &perJobMetrics{}
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		b.Fatal("metrix.AsCycleManagedStore returned false")
	}

	entries := make([]*TrapEntry, 0, defaultPipelineMetricMaxSources)
	for i := range defaultPipelineMetricMaxSources {
		entries = append(entries, &TrapEntry{
			JobName:  jobName,
			SourceIP: benchmarkSourceIP(i),
			Severity: "warning",
		})
	}
	for i := range defaultPipelineMetricMaxSources - 1 {
		metrics.recordSourceAccepted(entries[i])
		metrics.recordSourceCommitted(entries[i])
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := entries[i%len(entries)]
		metrics.recordSourceAccepted(entry)
		metrics.recordSourceCommitted(entry)
		managed.CycleController().BeginCycle()
		collectSourceMetrics(store, jobName, metrics)
		if err := managed.CycleController().CommitCycleSuccess(); err != nil {
			b.Fatalf("CommitCycleSuccess: %v", err)
		}
	}
	b.StopTimer()

	metrics.sourceMu.Lock()
	sourceCount := len(metrics.sources)
	overflowDropped := metrics.sourceDiagnostics.overflowDropped
	metrics.sourceMu.Unlock()
	b.ReportMetric(float64(sourceCount), "sources")
	b.ReportMetric(float64(overflowDropped), "overflow_dropped")
	if elapsed := b.Elapsed().Seconds(); elapsed > 0 {
		b.ReportMetric(float64(b.N)/elapsed, "cycles/s")
	}
}

func benchmarkProfileMetricIndex(b testing.TB) *ProfileIndex {
	b.Helper()
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
			sharedVarbinds: map[string]*VarbindDef{
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
				sysUpTimeOID: {
					OID:     sysUpTimeOID,
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
		b.Fatalf("addTraps: %v", err)
	}

	rules := []profileMetricRule{
		{
			Name:       "bench.config.changed",
			Type:       profileMetricTypeCounter,
			OnTrap:     testCiscoConfigTrapOID,
			Output:     profileMetricOutput{Metric: "snmp_trap_bench_config_events", Dimension: "events", Chart: "bench_config_changes"},
			sourceFile: "benchmark-profile.yaml",
		},
		{
			Name:             "bench.config.terminal_type",
			Type:             profileMetricTypeSample,
			OnTrap:           testCiscoConfigTrapOID,
			ValueFromVarbind: testCiscoTerminalTypeVarbind,
			Output:           profileMetricOutput{Metric: "snmp_trap_bench_terminal_type", Dimension: "terminal_type", Chart: "bench_terminal_type"},
			sourceFile:       "benchmark-profile.yaml",
		},
		{
			Name:   "bench.config.console_state",
			Type:   profileMetricTypeState,
			OnTrap: testCiscoConfigTrapOID,
			State: profileMetricState{
				SetWhen:   &profileMetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "console"},
				ClearWhen: &profileMetricPredicate{Varbind: testCiscoTerminalTypeVarbind, Equals: "virtual"},
				TTL:       "1ns",
			},
			Output:     profileMetricOutput{Metric: "snmp_trap_bench_console_state", Dimension: "active", Chart: "bench_console_state"},
			sourceFile: "benchmark-profile.yaml",
		},
		{
			Name:       "bench.port_security.ifindex",
			Type:       profileMetricTypeCounter,
			OnTrap:     testPortSecurityTrapOID,
			Identity:   profileMetricIdentity{Resource: &profileMetricResource{Class: "interface", KeyFromVarbind: "ifIndex", MaxPerSource: 32}},
			Output:     profileMetricOutput{Metric: "snmp_trap_bench_port_security_violations", Dimension: "violations", Chart: "bench_port_security"},
			sourceFile: "benchmark-profile.yaml",
		},
	}
	charts := []profileMetricChart{
		{ID: "bench_config_changes", Title: "Benchmark config changes", Context: "snmp.trap.bench.config.changes", Units: "events/s", Algorithm: "incremental", sourceFile: "benchmark-profile.yaml"},
		{ID: "bench_terminal_type", Title: "Benchmark terminal type", Context: "snmp.trap.bench.terminal.type", Units: "type", Algorithm: "absolute", sourceFile: "benchmark-profile.yaml"},
		{ID: "bench_console_state", Title: "Benchmark console state", Context: "snmp.trap.bench.console.state", Units: "state", Algorithm: "absolute", sourceFile: "benchmark-profile.yaml"},
		{ID: "bench_port_security", Title: "Benchmark port security", Context: "snmp.trap.bench.port.security", Units: "events/s", Algorithm: "incremental", sourceFile: "benchmark-profile.yaml"},
	}
	if err := idx.addProfileMetrics(rules, charts); err != nil {
		b.Fatalf("addProfileMetrics: %v", err)
	}
	return idx
}

func benchmarkProfileMetricConfigTrapEntry(jobName, sourceIP string, terminalType int) *TrapEntry {
	return &TrapEntry{
		JobName:       jobName,
		TrapOID:       testCiscoConfigTrapOID,
		TrapName:      "CISCO-CONFIG-MAN-MIB::ccmCLIRunningConfigChanged",
		SourceIP:      sourceIP,
		SourceUDPPeer: sourceIP,
		Enrichment: &TrapEnrichmentAudit{Source: &TrapSourceAudit{
			Selected: sourceIP,
			Method:   "udp_peer",
		}},
		Varbinds: []VarbindValue{
			{OID: testCiscoTerminalTypeOID, Type: "INTEGER", Value: terminalType},
			{OID: sysUpTimeOID, Type: "TimeTicks", Value: uint64(12345)},
		},
	}
}

func benchmarkProfileMetricPortTrapEntry(jobName, sourceIP string, ifIndex int) *TrapEntry {
	return &TrapEntry{
		JobName:       jobName,
		TrapOID:       testPortSecurityTrapOID,
		TrapName:      "CISCO-PORT-SECURITY-MIB::cpsSecureMacAddrViolation",
		SourceIP:      sourceIP,
		SourceUDPPeer: sourceIP,
		Enrichment: &TrapEnrichmentAudit{Source: &TrapSourceAudit{
			Selected: sourceIP,
			Method:   "udp_peer",
		}},
		Varbinds: []VarbindValue{
			{OID: testIfIndexOID, Type: "INTEGER", Value: ifIndex},
		},
	}
}

func benchmarkSourceIP(i int) string {
	return fmt.Sprintf("10.%d.%d.%d", 100+(i/65025), (i/255)%255, i%255+1)
}

// ---------------------------------------------------------------------------
// 4. SDK-backed journal writer/drain throughput (enhanced existing benchmarks)
// ---------------------------------------------------------------------------

func BenchmarkTrapWriterWrite(b *testing.B) {
	tw := newJournalTrapWriter(nil, 1<<20)
	entry := benchmarkTrapEntry()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writeTrapEntryWithBackpressure(b, tw, entry)
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "entries/s")
	_ = tw.Close()
}

func BenchmarkJournalTrapWriterDrain(b *testing.B) {
	dir := b.TempDir()
	w, err := newTestJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
	if err != nil {
		b.Fatalf("NewJournalWriter: %v", err)
	}
	tw := newJournalTrapWriter(w, 1<<20)
	entry := benchmarkTrapEntry()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writeTrapEntryWithBackpressure(b, tw, entry)
	}
	if err := tw.Flush(); err != nil {
		b.Fatalf("Flush: %v", err)
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "entries/s")
	if err := tw.Close(); err != nil {
		b.Fatalf("Close: %v", err)
	}
}

func writeTrapEntryWithBackpressure(b *testing.B, tw *journalTrapWriter, entry *TrapEntry) {
	b.Helper()
	for {
		err := tw.Write(entry)
		if err == nil {
			return
		}
		if err != errQueueFull {
			b.Fatalf("Write: %v", err)
		}
		runtime.Gosched()
	}
}

func BenchmarkJournalWriterWriteEntry(b *testing.B) {
	dir := b.TempDir()
	w, err := newTestJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
	if err != nil {
		b.Fatalf("NewJournalWriter: %v", err)
	}
	defer w.Close()

	fields, err := serializeToJournalFields(benchmarkTrapEntry())
	if err != nil {
		b.Fatalf("serializeToJournalFields: %v", err)
	}
	now := time.Now().UnixMicro()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := w.WriteEntry(fields, now+int64(i), now+int64(i)); err != nil {
			b.Fatalf("WriteEntry: %v", err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "entries/s")
}

// BenchmarkFullPacketToJournal measures the combined path:
// synthetic SNMPv2c packet -> handlePacket -> journalTrapWriter queue ->
// SDK-backed journal append/sync. journalctl row counting runs after the timed
// section so the throughput metric reflects ingestion and persistence only.
func BenchmarkFullPacketToJournal(b *testing.B) {
	requireJournalctlBenchmark(b)

	data := buildBenchV2cTrap(b, "public", "1.3.6.1.6.3.1.1.5.1",
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: "test-device"},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(123456)},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.1.1", Type: gosnmp.Integer, Value: 1},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.1.2", Type: gosnmp.Integer, Value: 2},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.2.1", Type: gosnmp.OctetString, Value: "Gi0/1"},
	)
	peer := net.ParseIP("10.1.2.3")
	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "coldStart from {TRAP_SOURCE_IP}",
	}
	setBenchProfileIndex(b, map[string]*TrapDef{trap.OID: trap})

	dir := b.TempDir()
	w, err := newTestJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
	if err != nil {
		b.Fatalf("NewJournalWriter: %v", err)
	}
	tw := newJournalTrapWriter(w, 1<<20)
	c := &Collector{
		jobName:    "bench-full",
		trapWriter: tw,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:  NewAllowlist(nil, []string{"public"}),
		metrics:    &perJobMetrics{},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pkt := make([]byte, len(data))
		copy(pkt, data)
		c.handlePacket(pkt, peer, nil, nil)
	}
	if err := tw.Flush(); err != nil {
		b.Fatalf("Flush: %v", err)
	}
	b.StopTimer()

	journalDir := w.JournalDirectory()
	if err := tw.Close(); err != nil {
		b.Fatalf("Close: %v", err)
	}
	rows := countJournalRowsBenchmark(b, journalDir, "TRAP_CATEGORY=security")
	if rows == 0 {
		b.Fatal("expected queryable journal rows, got 0")
	}
	reportDrops(b, int64(b.N), rows)
	elapsed := b.Elapsed().Seconds()
	if elapsed > 0 {
		b.ReportMetric(float64(rows)/elapsed, "persisted_entries/s")
		b.ReportMetric(float64(b.N)/elapsed, "packets/s")
	}
}

// BenchmarkUDPPacketToJournal measures the real local UDP receive path:
// UDP socket -> Listener.readLoop -> Collector.handlePacket ->
// journalTrapWriter queue -> SDK-backed journal append/sync.
func BenchmarkUDPPacketToJournal(b *testing.B) {
	requireJournalctlBenchmark(b)

	h := newUDPPacketToJournalBenchmark(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.writePacket(b)
	}
	h.finish(b, int64(b.N))
}

func BenchmarkUDPPacketToJournalPaced(b *testing.B) {
	for _, rate := range []int{25000, 50000, 75000, 100000} {
		b.Run(fmt.Sprintf("%dpps", rate), func(b *testing.B) {
			h := newUDPPacketToJournalBenchmark(b)
			perTick := rate / 1000
			remainder := rate % 1000
			sent := int64(0)
			start := time.Now()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				target := start.Add(time.Duration(i) * time.Millisecond)
				if delay := time.Until(target); delay > 0 {
					time.Sleep(delay)
				}
				n := perTick
				if (i*remainder)%1000 < remainder {
					n++
				}
				for range n {
					h.writePacket(b)
				}
				sent += int64(n)
			}
			h.finish(b, sent)
		})
	}
}

type udpPacketToJournalBenchmark struct {
	data       []byte
	writer     *journalTrapWriter
	listener   *Listener
	conn       *net.UDPConn
	delivered  atomic.Int64
	journalDir string
}

func newUDPPacketToJournalBenchmark(b *testing.B) *udpPacketToJournalBenchmark {
	b.Helper()

	data := buildBenchV2cTrap(b, "public", "1.3.6.1.6.3.1.1.5.1",
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: "test-device"},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(123456)},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.1.1", Type: gosnmp.Integer, Value: 1},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.1.2", Type: gosnmp.Integer, Value: 2},
		gosnmp.SnmpPDU{Name: "1.3.6.1.2.1.2.2.1.2.1", Type: gosnmp.OctetString, Value: "Gi0/1"},
	)
	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "coldStart from {TRAP_SOURCE_IP}",
	}
	setBenchProfileIndex(b, map[string]*TrapDef{trap.OID: trap})

	w, err := newTestJournalWriter(b.TempDir(), JournalConfig{RotateSize: 200 * bytesPerMB})
	if err != nil {
		b.Fatalf("NewJournalWriter: %v", err)
	}
	tw := newJournalTrapWriter(w, defaultQueueCapacity)
	b.Cleanup(func() {
		_ = tw.Close()
	})

	c := &Collector{
		jobName:    "bench-udp",
		trapWriter: tw,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:  NewAllowlist(nil, []string{"public"}),
		metrics:    &perJobMetrics{},
	}

	listener, err := newListener("bench-udp", ListenConfig{
		Endpoints: []EndpointConfig{{Protocol: "udp4", Address: "127.0.0.1", Port: 0}},
	})
	if err != nil {
		b.Fatalf("newListener: %v", err)
	}
	b.Cleanup(listener.close)

	udpAddr, ok := listener.endpoints[0].conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		b.Fatalf("unexpected listener address: %T", listener.endpoints[0].conn.LocalAddr())
	}
	conn, err := net.DialUDP("udp4", nil, udpAddr)
	if err != nil {
		b.Fatalf("DialUDP: %v", err)
	}
	b.Cleanup(func() { _ = conn.Close() })

	h := &udpPacketToJournalBenchmark{
		data:       data,
		writer:     tw,
		listener:   listener,
		conn:       conn,
		journalDir: w.JournalDirectory(),
	}
	listener.start(func(pkt []byte, peerIP net.IP, conn *net.UDPConn, peer *net.UDPAddr) {
		c.handlePacket(pkt, peerIP, conn, peer)
		h.delivered.Add(1)
	})

	return h
}

func (h *udpPacketToJournalBenchmark) writePacket(b *testing.B) {
	b.Helper()
	if _, err := h.conn.Write(h.data); err != nil {
		b.Fatalf("Write UDP packet: %v", err)
	}
}

func (h *udpPacketToJournalBenchmark) finish(b *testing.B, sent int64) {
	b.Helper()

	deliveredCount := waitForBenchmarkDeliveries(b, &h.delivered, sent, 250*time.Millisecond)
	h.listener.close()
	if err := h.writer.Flush(); err != nil {
		b.Fatalf("Flush: %v", err)
	}
	b.StopTimer()

	if err := h.writer.Close(); err != nil {
		b.Fatalf("Close: %v", err)
	}
	rows := countJournalRowsBenchmark(b, h.journalDir, "TRAP_CATEGORY=security")
	reportDrops(b, sent, rows)
	elapsed := b.Elapsed().Seconds()
	if elapsed > 0 {
		b.ReportMetric(float64(sent)/elapsed, "sent_packets/s")
		b.ReportMetric(float64(deliveredCount)/elapsed, "udp_delivered/s")
		b.ReportMetric(float64(rows)/elapsed, "persisted_entries/s")
	}
}

func waitForBenchmarkDeliveries(b *testing.B, delivered *atomic.Int64, want int64, quietFor time.Duration) int64 {
	b.Helper()

	last := delivered.Load()
	quietSince := time.Now()
	for last < want {
		time.Sleep(time.Millisecond)
		cur := delivered.Load()
		if cur != last {
			last = cur
			quietSince = time.Now()
			continue
		}
		if time.Since(quietSince) >= quietFor {
			return cur
		}
	}
	return last
}

// ---------------------------------------------------------------------------
// 5. Malformed BER / limit rejection benchmark
// ---------------------------------------------------------------------------

func BenchmarkBERRejection(b *testing.B) {
	cases := map[string]struct {
		data []byte
	}{
		"Oversized": {
			data: make([]byte, maxDatagramSize+1),
		},
		"DepthOverLimit": {
			data: nestedSequence(maxNestingDepth + 1),
		},
		"OIDTooLong": {
			data: berTLV(tagSequence, berTLV(tagOID, make([]byte, maxOIDEncodedLen+1))),
		},
		"OctetStringTooLong": {
			data: berTLV(tagSequence, berTLV(tagOctetStr, make([]byte, maxOctetStringLen+1))),
		},
		"TrailingData": {
			data: append(buildBenchV2cTrap(b, "public", "1.3.6.1.6.3.1.1.5.1"), 0x00),
		},
		"IndefiniteLength": {
			data: []byte{tagSequence, 0x80, 0x00, 0x00},
		},
		"Truncated": {
			data: []byte{0x30, 0x01, 0x02},
		},
	}

	for name, tc := range cases {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = decodePacket(tc.data, nil)
			}
			b.StopTimer()
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "rejected/s")
		})
	}
}

// ---------------------------------------------------------------------------
// 6. Profile-cache and dedup hot-path benchmarks
// ---------------------------------------------------------------------------

func BenchmarkBuildStockProfileStoreDefaultProfiles(b *testing.B) {
	dir := trapProfilesDirFromThisFile()
	if dir == "" {
		b.Skip("default trap profile directory not found")
	}
	paths := multipath.New(dir)

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		idx := &ProfileIndex{
			trapsByOID:      make(map[string]*TrapDef),
			namesByTrapName: make(map[string]*TrapDef),
		}
		store, err := buildStockProfileStore(dir, paths, nil, idx)
		if err != nil {
			b.Fatalf("buildStockProfileStore: %v", err)
		}
		if store.empty() {
			b.Fatal("expected non-empty stock profile store")
		}
	}
}

func BenchmarkDedupFingerprint(b *testing.B) {
	entry := benchmarkDedupEntry()
	td := &TrapDef{DedupKeyVarbinds: []string{"ifIndex", "ifDescr"}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = dedupFingerprint(entry, td, nil)
	}
}

func BenchmarkDedupAdmitDuplicate(b *testing.B) {
	entry := benchmarkDedupEntry()
	td := &TrapDef{DedupKeyVarbinds: []string{"ifIndex", "ifDescr"}}
	d := newTrapDeduper("bench-dedup", DedupConfig{Enabled: true}, nil, nil, "")
	if _, suppressed := d.Admit(entry, td, nil); suppressed {
		b.Fatal("first dedup admission was unexpectedly suppressed")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.Admit(entry, td, nil)
	}
}

// ---------------------------------------------------------------------------
// Existing helpers (preserved)
// ---------------------------------------------------------------------------

func requireJournalctlBenchmark(b *testing.B) {
	b.Helper()
	if _, err := exec.LookPath("journalctl"); err != nil {
		b.Skip("journalctl not found")
	}
}

func countJournalRowsBenchmark(b *testing.B, dir, match string) int64 {
	b.Helper()
	cmd := exec.Command("journalctl", "--directory="+dir, match, "-o", "cat", "--no-pager")
	out, err := cmd.CombinedOutput()
	trimmed := bytes.TrimSpace(out)
	if err != nil && len(trimmed) != 0 {
		b.Fatalf("journalctl failed: %v\n%s", err, string(out))
	}
	if len(trimmed) == 0 {
		return 0
	}
	return int64(bytes.Count(trimmed, []byte{'\n'}) + 1)
}

func benchmarkTrapEntry() *TrapEntry {
	return &TrapEntry{
		JobName:               "bench",
		ReportType:            ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.1",
		TrapName:              "TEST-MIB::coldStart",
		Category:              "security",
		Severity:              "warning",
		Message:               "benchmark trap",
		SourceIP:              "192.0.2.10",
		SourceUDPPeer:         "192.0.2.10",
		PduType:               PduTypeTrap,
		SnmpVersion:           SnmpVersionV2c,
		Labels: map[string]string{
			"site": "lab",
		},
		Varbinds: []VarbindValue{
			{Name: "ifIndex", OID: "1.3.6.1.2.1.2.2.1.1", Type: "INTEGER", Value: int64(1)},
			{Name: "ifDescr", OID: "1.3.6.1.2.1.31.1.1.1.1", Type: "OctetString", Value: "Ethernet1"},
		},
	}
}

func benchmarkDedupEntry() *TrapEntry {
	return &TrapEntry{
		SourceIP: "192.0.2.10",
		TrapOID:  "1.3.6.1.6.3.1.1.5.3",
		Varbinds: []VarbindValue{
			{Name: "ifIndex", OID: "1.3.6.1.2.1.2.2.1.1.1", Type: "INTEGER", Value: int64(7)},
			{Name: "ifDescr", OID: "1.3.6.1.2.1.31.1.1.1.1.7", Type: "OctetString", Value: "Gi0/7"},
			{Name: "ifAdminStatus", OID: "1.3.6.1.2.1.2.2.1.7.7", Type: "INTEGER", Value: int64(1)},
			{Name: "ifOperStatus", OID: "1.3.6.1.2.1.2.2.1.8.7", Type: "INTEGER", Value: int64(2)},
		},
	}
}
