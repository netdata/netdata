// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/telemetry"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

// ---------------------------------------------------------------------------
// M1 benchmark harness
// ---------------------------------------------------------------------------

// buildBenchV2cTrap generates a synthetic SNMPv2c trap packet.
func buildBenchV2cTrap(b testing.TB, community, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	b.Helper()
	x := &gosnmp.GoSNMP{Version: gosnmp.Version2c, Community: community}
	pdus := []gosnmp.SnmpPDU{
		{Name: model.SysUpTimeOID, Type: gosnmp.TimeTicks, Value: uint32(10)},
		{Name: model.SNMPTrapOID, Type: gosnmp.ObjectIdentifier, Value: trapOID},
	}
	pdus = append(pdus, extra...)
	data, err := x.MkSnmpPacket(gosnmp.SNMPv2Trap, pdus, 0, 0).MarshalMsg()
	if err != nil {
		b.Fatalf("marshal benchmark v2c trap: %v", err)
	}
	return data
}

func setBenchProfileIndex(b *testing.B, traps map[string]*TrapDef) *catalog.Epoch {
	b.Helper()
	for _, trap := range traps {
		if err := catalog.PrepareTrap(trap); err != nil {
			b.Fatalf("compile benchmark trap templates: %v", err)
		}
	}
	idx := catalog.NewEpoch()
	trapDefs := make([]*TrapDef, 0, len(traps))
	for _, trap := range traps {
		trapDefs = append(trapDefs, trap)
	}
	if err := idx.AddTraps(trapDefs); err != nil {
		b.Fatalf("build benchmark profile index: %v", err)
	}
	return idx
}

// countingWriter is an in-memory sink that counts trap entries without
// unbounded storage growth, suitable for high-iteration benchmarks.
type countingWriter struct {
	count  int64
	closed int32
}

func (w *countingWriter) Write(entry *TrapEntry) error {
	if atomic.LoadInt32(&w.closed) != 0 {
		return output.ErrClosed
	}
	atomic.AddInt64(&w.count, 1)
	return nil
}

func (w *countingWriter) Flush() error   { return nil }
func (w *countingWriter) Close() error   { atomic.StoreInt32(&w.closed, 1); return nil }
func (w *countingWriter) Written() int64 { return atomic.LoadInt64(&w.count) }

var _ output.Writer = (*countingWriter)(nil)

func newBenchmarkTelemetry(jobName string) *telemetry.Job {
	return telemetry.NewRegistry().Attach(jobName, telemetry.Options{})
}

// ---------------------------------------------------------------------------
// End-to-end packet path through Collector.handlePacket
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
		Description: "coldStart from {{source_ip}}",
	}
	idx := setBenchProfileIndex(b, map[string]*TrapDef{trap.OID: trap})

	const jobName = "bench-pkt"

	writer := &countingWriter{}
	c := &Collector{
		Config:       Config{Name: jobName},
		trapWriter:   writer,
		telemetry:    newBenchmarkTelemetry(jobName),
		profileIndex: idx,
	}
	c.receiver = newTestReceiver(c, receiver.PolicyConfig{Versions: []string{"v2c"}, Communities: []string{"public"}})

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

var (
	benchmarkEnrichmentStringSink string
	benchmarkEnrichmentIntSink    int
)

// BenchmarkTrapEnrichmentMatchingRegistryTopology measures the affected
// per-packet enrichment path with registry, topology, and cached PTR hits.
func BenchmarkTrapEnrichmentMatchingRegistryTopology(b *testing.B) {
	const sourceIP = "192.0.2.10"

	topology := testTrapTopologyEnricher(func(ip, trapIfIndex string) *snmptopology.TrapTopologyEnrichment {
		if ip != sourceIP || trapIfIndex != "7" {
			b.Fatalf("unexpected topology lookup: ip=%q ifIndex=%q", ip, trapIfIndex)
		}
		return &snmptopology.TrapTopologyEnrichment{
			SourceIP:        sourceIP,
			DeviceStatus:    "matched",
			DeviceMethod:    "management_ip",
			DeviceMatches:   1,
			DeviceHostname:  "topology-device.example.test",
			DeviceVendor:    "topology-vendor",
			SourceVnodeID:   "vnode-benchmark",
			InterfaceIndex:  "7",
			InterfaceStatus: "matched",
			Interface:       "Gi0/7",
			NeighborStatus:  "matched",
			Neighbors:       []string{"access-a", "access-b"},
		}
	})
	dns := reversedns.New(reversedns.Config{Lookup: func(context.Context, string) ([]string, error) {
		return []string{"ptr-device.example.test."}, nil
	}})
	_, err := dns.Resolve(context.Background(), netip.MustParseAddr(sourceIP))
	if err != nil {
		b.Fatalf("prewarm reverse DNS: %v", err)
	}
	c, store := newTestTrapEnrichmentCollector(topology, dns)
	c.reverseDNSEnabled = true
	store.Register("benchmark:"+sourceIP, ddsnmp.DeviceConnectionInfo{
		Hostname:      sourceIP,
		VnodeHostname: "registry-device.example.test",
		Vendor:        "registry-vendor",
		VnodeGUID:     "vnode-benchmark",
	})

	varbinds := []VarbindValue{{
		Name:  "ifIndex",
		OID:   "1.3.6.1.2.1.2.2.1.1.7",
		Type:  "INTEGER",
		Value: int64(7),
	}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := TrapEntry{SourceIP: sourceIP, Varbinds: varbinds}
		c.enrichTrapEntry(&entry)
		benchmarkEnrichmentStringSink = entry.ReverseDNS
		benchmarkEnrichmentIntSink = len(entry.Enrichment.Applied)
	}
}

// BenchmarkPacketTrapEnrichedJobs measures concurrent packet handling across
// multiple jobs with registry, topology, and cached PTR enrichment enabled.
func BenchmarkPacketTrapEnrichedJobs(b *testing.B) {
	for _, numJobs := range []int{1, 4, 10} {
		b.Run(fmt.Sprintf("jobs=%d", numJobs), func(b *testing.B) {
			data := buildBenchV2cTrap(b, "public", "1.3.6.1.6.3.1.1.5.1")
			trap := &TrapDef{
				OID:         "1.3.6.1.6.3.1.1.5.1",
				Name:        "TEST-MIB::coldStart",
				Category:    "security",
				Severity:    "warning",
				Description: "coldStart from {{source_ip}}",
			}
			idx := setBenchProfileIndex(b, map[string]*TrapDef{trap.OID: trap})

			store := ddsnmp.NewDeviceStore()
			dns := reversedns.New(reversedns.Config{Lookup: func(context.Context, string) ([]string, error) {
				return []string{"ptr-device.example.test."}, nil
			}})
			topologyByIP := make(map[string]*snmptopology.TrapTopologyEnrichment, numJobs)
			writers := make([]*countingWriter, numJobs)
			collectors := make([]*Collector, numJobs)
			peers := make([]net.IP, numJobs)
			for i := range numJobs {
				sourceIP := fmt.Sprintf("192.0.2.%d", i+1)
				vnodeID := fmt.Sprintf("vnode-benchmark-%d", i+1)
				peers[i] = net.ParseIP(sourceIP)
				store.Register("benchmark:"+sourceIP, ddsnmp.DeviceConnectionInfo{
					Hostname:  sourceIP,
					SysName:   fmt.Sprintf("device-%d.example.test", i+1),
					Vendor:    "benchmark-vendor",
					VnodeGUID: vnodeID,
				})
				topologyByIP[sourceIP] = &snmptopology.TrapTopologyEnrichment{
					SourceIP:       sourceIP,
					DeviceStatus:   "matched",
					DeviceMethod:   "management_ip",
					DeviceMatches:  1,
					DeviceHostname: fmt.Sprintf("device-%d.example.test", i+1),
					DeviceVendor:   "benchmark-vendor",
					SourceVnodeID:  vnodeID,
				}

				if _, err := dns.Resolve(context.Background(), netip.MustParseAddr(sourceIP)); err != nil {
					b.Fatalf("prewarm reverse DNS for %s: %v", sourceIP, err)
				}

				writers[i] = &countingWriter{}
				collectors[i] = &Collector{
					Config:            Config{Name: fmt.Sprintf("bench-enriched-%d", i+1)},
					trapWriter:        writers[i],
					telemetry:         newBenchmarkTelemetry(fmt.Sprintf("bench-enriched-%d", i+1)),
					profileIndex:      idx,
					enricher:          newTestTrapEnricher(store, testTrapTopologyEnricher(func(ip, _ string) *snmptopology.TrapTopologyEnrichment { return topologyByIP[ip] }), dns),
					reverseDNSEnabled: true,
				}
				collectors[i].receiver = newTestReceiver(collectors[i], receiver.PolicyConfig{
					Versions:    []string{"v2c"},
					Communities: []string{"public"},
				})
			}

			var nextJob atomic.Uint64
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				job := int(nextJob.Add(1)-1) % numJobs
				collector := collectors[job]
				peer := peers[job]
				for pb.Next() {
					pkt := make([]byte, len(data))
					copy(pkt, data)
					collector.handlePacket(pkt, peer, nil, nil)
				}
			})
			b.StopTimer()

			var written int64
			for _, writer := range writers {
				written += writer.Written()
			}
			reportDrops(b, int64(b.N), written)
			b.ReportMetric(float64(written)/b.Elapsed().Seconds(), "entries/s")
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "packets/s")
		})
	}
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
				Description: "coldStart from {{source_ip}}",
			}
			idx := setBenchProfileIndex(b, map[string]*TrapDef{trap.OID: trap})

			writers := make([]*countingWriter, numJobs)
			collectors := make([]*Collector, numJobs)
			peers := make([]net.IP, numJobs)
			for i := range numJobs {
				jn := fmt.Sprintf("bm-multi-%d", i)
				peers[i] = net.ParseIP(fmt.Sprintf("10.1.2.%d", i+1))
				writers[i] = &countingWriter{}
				collectors[i] = &Collector{
					Config:       Config{Name: jn},
					trapWriter:   writers[i],
					telemetry:    newBenchmarkTelemetry(jn),
					profileIndex: idx,
				}
				collectors[i].receiver = newTestReceiver(collectors[i], receiver.PolicyConfig{Versions: []string{"v2c"}, Communities: []string{"public"}})
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

// BenchmarkFullPacketToJournal measures the combined path:
// synthetic SNMPv2c packet -> handlePacket -> journal writer queue ->
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
		Description: "coldStart from {{source_ip}}",
	}
	idx := setBenchProfileIndex(b, map[string]*TrapDef{trap.OID: trap})

	dir := b.TempDir()
	tw := newBenchmarkJournalWriter(b, dir, 1<<20)
	c := &Collector{
		Config:       Config{Name: "bench-full"},
		trapWriter:   tw,
		telemetry:    newBenchmarkTelemetry("bench-full"),
		profileIndex: idx,
	}
	c.receiver = newTestReceiver(c, receiver.PolicyConfig{Versions: []string{"v2c"}, Communities: []string{"public"}})

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

	journalDir := tw.Directory()
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

func newBenchmarkJournalWriter(b *testing.B, dir string, queueCapacity int) *journal.Writer {
	b.Helper()
	writer, err := journal.Prepare(
		dir,
		journal.Config{RotateSize: 200 * 1024 * 1024},
		newTestJournalHostProvider(),
		journal.Options{QueueCapacity: queueCapacity},
	)
	if err != nil {
		b.Fatalf("prepare journal writer: %v", err)
	}
	if err := writer.Start(); err != nil {
		_ = writer.Close()
		b.Fatalf("start journal writer: %v", err)
	}
	return writer
}

// BenchmarkUDPPacketToJournal measures the real local UDP receive path:
// UDP socket -> receiver read loop -> Collector.handlePacket ->
// journal writer queue -> SDK-backed journal append/sync.
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
	writer     *journal.Writer
	receiver   *receiver.Receiver
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
		Description: "coldStart from {{source_ip}}",
	}
	idx := setBenchProfileIndex(b, map[string]*TrapDef{trap.OID: trap})

	tw := newBenchmarkJournalWriter(b, b.TempDir(), journal.DefaultQueueCapacity)
	b.Cleanup(func() {
		_ = tw.Close()
	})

	c := &Collector{
		Config:       Config{Name: "bench-udp"},
		trapWriter:   tw,
		telemetry:    newBenchmarkTelemetry("bench-udp"),
		profileIndex: idx,
	}
	port := freeUDPPort(b)
	recv := newTestReceiver(c, receiver.PolicyConfig{
		Listen:      receiver.ListenConfig{Endpoints: []receiver.Endpoint{{Protocol: "udp", Address: "127.0.0.1", Port: port}}},
		Versions:    []string{"v2c"},
		Communities: []string{"public"},
	})
	c.receiver = recv
	_, err := recv.Bind()
	if err != nil {
		b.Fatalf("bind receiver: %v", err)
	}
	b.Cleanup(recv.Close)

	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if err != nil {
		b.Fatalf("DialUDP: %v", err)
	}
	b.Cleanup(func() { _ = conn.Close() })

	h := &udpPacketToJournalBenchmark{
		data:       data,
		writer:     tw,
		receiver:   recv,
		conn:       conn,
		journalDir: tw.Directory(),
	}
	recv.Start(func(datagram receiver.Datagram) {
		c.handlePacket(datagram.Data, datagram.PeerIP, datagram.Conn, datagram.Peer)
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
	h.receiver.Close()
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
// Profile catalog and dedup hot-path benchmarks
// ---------------------------------------------------------------------------

func BenchmarkAcquireProfileCatalogDefaultProfiles(b *testing.B) {
	dir := filepath.Clean("../../config/go.d/snmp.trap-profiles/default")

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		lease, err := catalog.NewManager(catalog.Paths{StockDir: dir}).Acquire()
		if err != nil {
			b.Fatalf("acquire profile catalog: %v", err)
		}
		if len(lease.Epoch().Profiles()) == 0 {
			b.Fatal("expected non-empty profile catalog")
		}
		lease.Close()
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
