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
			for i := 0; i < numJobs; i++ {
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
			for i := 0; i < numJobs; i++ {
				count := perJob
				if i < remainder {
					count++
				}
				wg.Add(1)
				go func(jobIdx, iterations int) {
					defer wg.Done()
					for j := 0; j < iterations; j++ {
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
	drops := packets - entries
	if drops < 0 {
		drops = 0
	}
	b.ReportMetric(float64(drops), "drops")
	if packets > 0 {
		b.ReportMetric(100*float64(drops)/float64(packets), "drop_pct")
	}
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
	requireLinuxJournalBenchmark(b)

	dir := b.TempDir()
	w, err := NewJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
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
	requireLinuxJournalBenchmark(b)

	dir := b.TempDir()
	w, err := NewJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
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
	w, err := NewJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
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
// Existing helpers (preserved)
// ---------------------------------------------------------------------------

func requireLinuxJournalBenchmark(b *testing.B) {
	b.Helper()
	if runtime.GOOS != "linux" {
		b.Skip("SNMP trap journal backend requires Linux")
	}
}

func requireJournalctlBenchmark(b *testing.B) {
	b.Helper()
	requireLinuxJournalBenchmark(b)
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
