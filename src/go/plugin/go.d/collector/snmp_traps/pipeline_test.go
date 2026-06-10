// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"gopkg.in/yaml.v2"
)

func setTestProfileIndex(t *testing.T, traps map[string]*TrapDef) {
	t.Helper()
	// Test-only shortcut: direct packet-path tests do not run Collector.Init(),
	// so they seed the immutable shared index without touching refcounts.
	globalProfileCache.current.Store(&ProfileIndex{trapsByOID: traps})
	t.Cleanup(func() { globalProfileCache.current.Store(nil) })
}

func assertSeverityCounters(t *testing.T, metrics *perJobMetrics, want map[string]uint64) {
	t.Helper()

	got := map[string]uint64{
		"emerg":   atomic.LoadUint64(&metrics.severities.emerg),
		"alert":   atomic.LoadUint64(&metrics.severities.alert),
		"crit":    atomic.LoadUint64(&metrics.severities.crit),
		"err":     atomic.LoadUint64(&metrics.severities.err),
		"warning": atomic.LoadUint64(&metrics.severities.warning),
		"notice":  atomic.LoadUint64(&metrics.severities.notice),
		"info":    atomic.LoadUint64(&metrics.severities.info),
		"debug":   atomic.LoadUint64(&metrics.severities.debug),
	}

	for name, value := range got {
		if value != want[name] {
			t.Errorf("%s severity = %d, want %d", name, value, want[name])
		}
	}
}

func TestCollectorHandlePacketWritesProfileResolvedTrapEntry(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "security coldStart from {TRAP_SOURCE_IP}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)

	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	entry := writer.entries[0]
	if entry.TrapName != trap.Name {
		t.Fatalf("TrapName = %q, want %q", entry.TrapName, trap.Name)
	}
	if entry.Category != "security" || entry.Severity != "warning" {
		t.Fatalf("category/severity = %q/%q, want security/warning", entry.Category, entry.Severity)
	}
	if entry.Message != "security coldStart from 198.51.100.10" {
		t.Fatalf("Message = %q", entry.Message)
	}
}

func TestCollectorHandlePacketRendersTemplatesAfterEnrichment(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	regKey := "test:198.51.100.10:162"
	ddsnmp.DeviceRegistry.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname: "198.51.100.10",
		SysName:  "core-sw-01",
		Vendor:   "cisco",
	})
	defer ddsnmp.DeviceRegistry.Unregister(regKey)

	trap := testColdStartTrap("security", "warning", "security coldStart on {_HOSTNAME} from {TRAP_DEVICE_VENDOR}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)

	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	if got := writer.entries[0].Message; got != "security coldStart on core-sw-01 from cisco" {
		t.Fatalf("Message = %q", got)
	}
}

func TestCollectorHandlePacketDoesNotUseListenerVnodeAsSourceNode(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "coldStart from {TRAP_SOURCE_IP}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)
	c.vnode = "listener-vnode-id"

	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	if got := writer.entries[0].SourceVnodeID; got != "" {
		t.Fatalf("SourceVnodeID = %q, want empty without source device match", got)
	}
}

func TestCollectorHandlePacketRendersTopologyEnrichmentBeforeReverseDNS(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	prev := trapTopologyEnrichmentForIP
	trapTopologyEnrichmentForIP = func(ip string) *snmptopology.TrapTopologyEnrichment {
		if ip != "198.51.100.10" {
			t.Fatalf("topology enrichment looked up IP %q, want 198.51.100.10", ip)
		}
		return &snmptopology.TrapTopologyEnrichment{
			DeviceHostname: "topo-sw-01",
			DeviceVendor:   "arista",
			SourceVnodeID:  "topo-vnode-id",
			Interface:      "Gi0/1",
			Neighbors:      []string{"dist-a", "dist-b"},
		}
	}
	t.Cleanup(func() { trapTopologyEnrichmentForIP = prev })

	dns := newReverseDNSResolver()
	dns.cache["198.51.100.10"] = reverseDNSCacheEntry{
		name:      "dns-sw-01.example.com",
		expiresAt: farFuture(),
	}
	t.Cleanup(dns.Close)

	trap := testColdStartTrap(
		"security",
		"warning",
		"trap on {_HOSTNAME} vendor {TRAP_DEVICE_VENDOR} iface {TRAP_INTERFACE} neighbors {TRAP_NEIGHBORS}",
	)
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)
	c.reverseDNSEnabled = true
	c.reverseDNS = dns

	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	entry := writer.entries[0]
	if entry.Message != "trap on topo-sw-01 vendor arista iface Gi0/1 neighbors dist-a,dist-b" {
		t.Fatalf("Message = %q", entry.Message)
	}
	if entry.DeviceHostname != "topo-sw-01" {
		t.Fatalf("DeviceHostname = %q, want topology hostname", entry.DeviceHostname)
	}
	if entry.SourceVnodeID != "topo-vnode-id" {
		t.Fatalf("SourceVnodeID = %q, want topology vnode", entry.SourceVnodeID)
	}
}

func TestCollectorHandlePacketDedupSuppressesDuplicates(t *testing.T) {
	const jobName = "test-dedup-packet"

	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "coldStart from {TRAP_SOURCE_IP}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c, metrics := newDedupTestV2Collector(t, jobName, writer)
	c.deduper.start()
	defer c.deduper.Close()

	c.handlePacket(packet.payload, packet.peer, nil, nil)
	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	if got := atomic.LoadUint64(&metrics.dedup.suppressed); got != 1 {
		t.Fatalf("dedup suppressed = %d, want 1", got)
	}
	if got := atomic.LoadUint64(&metrics.events.security); got != 1 {
		t.Fatalf("security events = %d, want 1", got)
	}
	assertSeverityCounters(t, metrics, map[string]uint64{"warning": 1})
	if got := atomic.LoadUint64(&metrics.errors.unknownOID); got != 0 {
		t.Fatalf("unknown OID errors = %d, want 0", got)
	}
	if got := atomic.LoadUint64(&metrics.errors.templateUnresolved); got != 0 {
		t.Fatalf("template unresolved errors = %d, want 0", got)
	}
	if got := atomic.LoadUint64(&metrics.errors.journalWriteFailed); got != 0 {
		t.Fatalf("journal write failures = %d, want 0", got)
	}
}

func TestCollectorHandlePacketDedupPreservesHealthErrorCounters(t *testing.T) {
	packet := readColdStartUDPPacket(t)

	t.Run("unknown OID", func(t *testing.T) {
		const jobName = "test-dedup-unknown-oid"

		setTestProfileIndex(t, map[string]*TrapDef{})
		writer := &mockTrapWriter{}
		c, metrics := newDedupTestV2Collector(t, jobName, writer)

		c.handlePacket(packet.payload, packet.peer, nil, nil)
		c.handlePacket(packet.payload, packet.peer, nil, nil)

		if len(writer.entries) != 1 {
			t.Fatalf("written entries = %d, want 1", len(writer.entries))
		}
		if got := atomic.LoadUint64(&metrics.dedup.suppressed); got != 1 {
			t.Fatalf("dedup suppressed = %d, want 1", got)
		}
		if got := atomic.LoadUint64(&metrics.errors.unknownOID); got != 2 {
			t.Fatalf("unknown OID errors = %d, want 2", got)
		}
		if got := atomic.LoadUint64(&metrics.events.unknown); got != 1 {
			t.Fatalf("unknown events = %d, want 1", got)
		}
		assertSeverityCounters(t, metrics, map[string]uint64{"notice": 1})
	})

	t.Run("template unresolved", func(t *testing.T) {
		const jobName = "test-dedup-template-unresolved"

		trap := testColdStartTrap("security", "warning", "coldStart from {DOES_NOT_EXIST}")
		trap.Name = "TEST-MIB::coldStartTemplate"
		setSingleTestTrap(t, trap)
		writer := &mockTrapWriter{}
		c, metrics := newDedupTestV2Collector(t, jobName, writer)

		c.handlePacket(packet.payload, packet.peer, nil, nil)
		c.handlePacket(packet.payload, packet.peer, nil, nil)

		if len(writer.entries) != 1 {
			t.Fatalf("written entries = %d, want 1", len(writer.entries))
		}
		if got := atomic.LoadUint64(&metrics.dedup.suppressed); got != 1 {
			t.Fatalf("dedup suppressed = %d, want 1", got)
		}
		if got := atomic.LoadUint64(&metrics.errors.templateUnresolved); got != 2 {
			t.Fatalf("template unresolved errors = %d, want 2", got)
		}
		if got := atomic.LoadUint64(&metrics.events.security); got != 1 {
			t.Fatalf("security events = %d, want 1", got)
		}
		assertSeverityCounters(t, metrics, map[string]uint64{"warning": 1})
	})
}

func TestCollectorHandlePacketDedupRollsBackFingerprintAfterWriteFailure(t *testing.T) {
	const jobName = "test-dedup-write-rollback"

	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "coldStart from {TRAP_SOURCE_IP}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{err: errors.New("write failed")}
	c, metrics := newDedupTestV2Collector(t, jobName, writer)

	c.handlePacket(packet.payload, packet.peer, nil, nil)
	if got := atomic.LoadUint64(&metrics.errors.journalWriteFailed); got != 1 {
		t.Fatalf("journal write failures = %d, want 1", got)
	}
	if got := atomic.LoadUint64(&metrics.dedup.suppressed); got != 0 {
		t.Fatalf("dedup suppressed after failed first write = %d, want 0", got)
	}

	writer.err = nil
	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries after rollback = %d, want 1", len(writer.entries))
	}
	if got := atomic.LoadUint64(&metrics.dedup.suppressed); got != 0 {
		t.Fatalf("dedup suppressed after rollback retry = %d, want 0", got)
	}
	if got := atomic.LoadUint64(&metrics.events.security); got != 1 {
		t.Fatalf("security events = %d, want 1", got)
	}
	assertSeverityCounters(t, metrics, map[string]uint64{"warning": 1})
}

func TestCollectorHandlePacketDropsDisallowedVersion(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:    "test",
		trapWriter: writer,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV3: {}},
		allowlist:  NewAllowlist(nil, nil),
	}

	removeJobMetrics("test")
	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if len(writer.entries) != 0 {
		t.Fatalf("expected 0 entries for disallowed version, got %d", len(writer.entries))
	}
	m := getJobMetrics("test")
	if dr := atomic.LoadUint64(&m.errors.droppedAllowlist); dr != 1 {
		t.Errorf("expected 1 dropped_allowlist, got %d", dr)
	}
	removeJobMetrics("test")
}

func TestCollectorHandlePacketDropsDisallowedV3BeforeDecode(t *testing.T) {
	const jobName = "test-disallowed-v3"
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	data := buildV3Trap(t, "testuser", "1.3.6.1.6.3.1.1.5.1")
	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:    jobName,
		trapWriter: writer,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:  NewAllowlist(nil, nil),
	}

	c.handlePacket(data, net.ParseIP("10.1.2.3"), nil, &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162})

	if len(writer.entries) != 0 {
		t.Fatalf("expected 0 entries for disallowed v3 packet, got %d", len(writer.entries))
	}
	m := getJobMetrics(jobName)
	if v := atomic.LoadUint64(&m.errors.droppedAllowlist); v != 1 {
		t.Fatalf("dropped_allowlist = %d, want 1", v)
	}
	if v := atomic.LoadUint64(&m.errors.authFailures); v != 0 {
		t.Fatalf("auth_failures = %d, want 0", v)
	}
	if v := atomic.LoadUint64(&m.errors.decodeFailed); v != 0 {
		t.Fatalf("decode_failed = %d, want 0", v)
	}
}

func TestCollectorHandlePacketWritesDecodeErrorEntry(t *testing.T) {
	const jobName = "test-decode-error-entry"
	metrics := withCleanJobMetrics(t, jobName)
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, nil, []string{"public"})

	data := make([]byte, maxDatagramSize+1)
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	c.handlePacket(data, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	entry := writer.entries[0]
	if entry.ReportType != ReportTypeDecodeError {
		t.Fatalf("ReportType = %q, want decode_error", entry.ReportType)
	}
	if entry.DecodeError == nil {
		t.Fatal("DecodeError is nil")
	}
	if entry.DecodeError.Kind != "malformed_pdu" {
		t.Fatalf("DecodeError.Kind = %q, want malformed_pdu", entry.DecodeError.Kind)
	}
	if entry.DecodeError.PacketSize != maxDatagramSize+1 {
		t.Fatalf("DecodeError.PacketSize = %d, want %d", entry.DecodeError.PacketSize, maxDatagramSize+1)
	}
	if len(entry.DecodeError.PacketSHA256) != 64 {
		t.Fatalf("DecodeError.PacketSHA256 length = %d, want 64", len(entry.DecodeError.PacketSHA256))
	}
	if entry.SourceIP != "10.1.2.3" {
		t.Fatalf("SourceIP = %q, want 10.1.2.3", entry.SourceIP)
	}
	if entry.SourceUDPPeer != "10.1.2.3" {
		t.Fatalf("SourceUDPPeer = %q, want 10.1.2.3", entry.SourceUDPPeer)
	}
	if entry.DecodeError.SourceUDPPort != 9162 {
		t.Fatalf("SourceUDPPort = %d, want 9162", entry.DecodeError.SourceUDPPort)
	}
	if !strings.Contains(entry.Message, "malformed_pdu") {
		t.Fatalf("Message = %q, want malformed_pdu", entry.Message)
	}
	if got := atomic.LoadUint64(&metrics.errors.malformedPDU); got != 1 {
		t.Fatalf("malformed_pdu = %d, want 1", got)
	}
}

func TestCollectorHandlePacketDecodeErrorHonorsAllowlist(t *testing.T) {
	const jobName = "test-decode-error-allowlist"
	metrics := withCleanJobMetrics(t, jobName)
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, []string{"public"})

	peer := &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 9162}
	c.handlePacket([]byte{0xff, 0x00}, peer.IP, nil, peer)

	if len(writer.entries) != 0 {
		t.Fatalf("written entries = %d, want 0", len(writer.entries))
	}
	if got := atomic.LoadUint64(&metrics.errors.droppedAllowlist); got != 1 {
		t.Fatalf("dropped_allowlist = %d, want 1", got)
	}
	if got := atomic.LoadUint64(&metrics.errors.decodeFailed); got != 0 {
		t.Fatalf("decode_failed = %d, want 0", got)
	}
}

func TestCollectorHandlePacketDecodeErrorHonorsRateLimitDrop(t *testing.T) {
	const jobName = "test-decode-error-rate-limit"
	metrics := withCleanJobMetrics(t, jobName)
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, nil, []string{"public"})
	c.rateLimiter = newRateLimiter(true, 1, "drop")

	data := make([]byte, maxDatagramSize+1)
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	c.handlePacket(data, peer.IP, nil, peer)
	c.handlePacket(data, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	if got := atomic.LoadUint64(&metrics.errors.malformedPDU); got != 2 {
		t.Fatalf("malformed_pdu = %d, want 2", got)
	}
	if got := atomic.LoadUint64(&metrics.errors.rateLimited); got != 1 {
		t.Fatalf("rate_limited = %d, want 1", got)
	}
}

func TestCollectorHandlePacketDecodeErrorNormalizesIPv4MappedSource(t *testing.T) {
	const jobName = "test-decode-error-ipv4-mapped"
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, []string{"public"})

	data := make([]byte, maxDatagramSize+1)
	peer := &net.UDPAddr{IP: net.ParseIP("::ffff:10.1.2.3"), Port: 9162}
	c.handlePacket(data, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	entry := writer.entries[0]
	if entry.SourceIP != "10.1.2.3" {
		t.Fatalf("SourceIP = %q, want 10.1.2.3", entry.SourceIP)
	}
	if entry.SourceUDPPeer != "10.1.2.3" {
		t.Fatalf("SourceUDPPeer = %q, want 10.1.2.3", entry.SourceUDPPeer)
	}
}

func TestCollectorHandlePacketDropsWhenAllowlistCannotDetermineSource(t *testing.T) {
	const jobName = "test-allowlist-missing-source"
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, []string{"public"})
	metrics := withCleanJobMetrics(t, jobName)

	c.handlePacket([]byte{0x30, 0x00}, nil, nil, nil)

	if len(writer.entries) != 0 {
		t.Fatalf("written entries = %d, want 0", len(writer.entries))
	}
	if got := atomic.LoadUint64(&metrics.errors.droppedAllowlist); got != 1 {
		t.Fatalf("dropped_allowlist = %d, want 1", got)
	}
}

func TestCollectorHandlePacketDropsDisallowedCommunity(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	writer := &mockTrapWriter{}
	c := newTestV2Collector("test", writer, nil, []string{"secret"})

	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if len(writer.entries) != 0 {
		t.Fatalf("expected 0 entries for disallowed community, got %d", len(writer.entries))
	}
}

func TestCollectorHandlePacketAllowsAllowedCommunity(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("state_change", "warning", "coldStart from {TRAP_SOURCE_IP}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newTestV2Collector("test", writer, nil, []string{"public"})

	c.handlePacket(packet.payload, packet.peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
}

func TestCollectorHandlePacketIncrementsEventsMetric(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("state_change", "warning", "coldStart from {TRAP_SOURCE_IP}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)

	withCleanJobMetrics(t, "test")
	c.handlePacket(packet.payload, packet.peer, nil, nil)

	m := getJobMetrics("test")
	ev := atomic.LoadUint64(&m.events.stateChange)
	if ev != 1 {
		t.Errorf("expected 1 state_change event, got %d", ev)
	}
}

func TestCollectorHandlePacketIncrementsSeverityMetric(t *testing.T) {
	const jobName = "test-severity-event"
	withCleanJobMetrics(t, jobName)

	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("state_change", "warning", "coldStart from {TRAP_SOURCE_IP}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, nil, []string{"public"})

	c.handlePacket(packet.payload, packet.peer, nil, nil)

	m := getJobMetrics(jobName)
	assertSeverityCounters(t, m, map[string]uint64{"warning": 1})
}

func TestPerJobMetricsIncSeverityFallsBackToNotice(t *testing.T) {
	m := &perJobMetrics{}
	m.incSeverity("")
	m.incSeverity("not_a_severity")

	if v := atomic.LoadUint64(&m.severities.notice); v != 2 {
		t.Fatalf("notice severity = %d, want 2", v)
	}
	assertSeverityCounters(t, m, map[string]uint64{"notice": 2})
}

func TestCollectMetricsEmitsSeverityCounters(t *testing.T) {
	const jobName = "test-severity-metrics"
	withCleanJobMetrics(t, jobName)

	getJobMetrics(jobName).incSeverity("crit")
	getJobMetrics(jobName).incSeverity("warning")
	getJobMetrics(jobName).incSeverity("info")

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatal("collector store does not expose cycle control")
	}

	managed.CycleController().BeginCycle()
	collectMetrics(store, jobName)
	if err := managed.CycleController().CommitCycleSuccess(); err != nil {
		t.Fatalf("commit collect cycle: %v", err)
	}

	labels := metrix.Labels{"job_name": jobName}
	want := map[string]float64{
		"snmp_trap_severity_emerg":   0,
		"snmp_trap_severity_alert":   0,
		"snmp_trap_severity_crit":    1,
		"snmp_trap_severity_err":     0,
		"snmp_trap_severity_warning": 1,
		"snmp_trap_severity_notice":  0,
		"snmp_trap_severity_info":    1,
		"snmp_trap_severity_debug":   0,
	}
	for name, expected := range want {
		if v, ok := store.Read().Value(name, labels); !ok || v != expected {
			t.Fatalf("%s value = %v/%v, want %v/true", name, v, ok, expected)
		}
	}
}

func TestCollectorHandlePacketIncrementsTemplateUnresolved(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "security coldStart from {missing_var}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)

	withCleanJobMetrics(t, "test")
	c.handlePacket(packet.payload, packet.peer, nil, nil)

	m := getJobMetrics("test")
	if v := atomic.LoadUint64(&m.errors.templateUnresolved); v != 1 {
		t.Errorf("expected 1 template_unresolved, got %d", v)
	}
}

func TestCollectorHandlePacketIncrementsAllowlistDrop(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	writer := &mockTrapWriter{}
	c := newTestV2Collector("test", writer, nil, []string{"secret"})

	withCleanJobMetrics(t, "test")
	c.handlePacket(packet.payload, packet.peer, nil, nil)

	m := getJobMetrics("test")
	dr := atomic.LoadUint64(&m.errors.droppedAllowlist)
	if dr != 1 {
		t.Errorf("expected 1 dropped_allowlist, got %d", dr)
	}
}

func TestCollectorHandlePacketRejectsUnknownV3EngineID(t *testing.T) {
	const jobName = "test-v3-engine"
	withCleanJobMetrics(t, jobName)

	data := buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	secTable := newTestV3SecurityTable(t, testNoAuthV3User(testEngineIDHex))
	engineIDs, err := buildEngineIDWhitelist([]string{"80001f888077dfe44faa700259"})
	if err != nil {
		t.Fatalf("buildEngineIDWhitelist failed: %v", err)
	}

	writer := &mockTrapWriter{}
	c := newTestV3Collector(jobName, writer, secTable, engineIDs)

	c.handlePacket(data, net.ParseIP("10.1.2.3"), nil, &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162})

	if len(writer.entries) != 0 {
		t.Fatalf("expected unknown engine ID to drop trap, got %d entries", len(writer.entries))
	}
	m := getJobMetrics(jobName)
	if v := atomic.LoadUint64(&m.errors.unknownEngineID); v != 1 {
		t.Fatalf("unknown_engine_id = %d, want 1", v)
	}
}

func TestCollectorHandlePacketClassifiesAuthFailureUnknownV3EngineID(t *testing.T) {
	const jobName = "test-v3-auth-failure-engine"
	withCleanJobMetrics(t, jobName)

	otherEngineID := "80001f888077dfe44faa700259"
	data := buildV3SecuredTrap(t, v3SecuredTrapSpec{
		user:        "testuser",
		engineIDHex: testEngineIDHex,
		authProto:   "sha256",
		privProto:   "aes",
		authKey:     "authpassword",
		privKey:     "privpassword",
		trapOID:     "1.3.6.1.6.3.1.1.5.1",
	})
	secTable := newTestV3SecurityTable(t, USMUserConfig{
		Username:  "testuser",
		EngineID:  otherEngineID,
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	})
	engineIDs, err := buildEngineIDWhitelist([]string{otherEngineID})
	if err != nil {
		t.Fatalf("buildEngineIDWhitelist failed: %v", err)
	}

	writer := &mockTrapWriter{}
	c := newTestV3Collector(jobName, writer, secTable, engineIDs)

	c.handlePacket(data, net.ParseIP("10.1.2.3"), nil, &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162})

	m := getJobMetrics(jobName)
	if v := atomic.LoadUint64(&m.errors.unknownEngineID); v != 1 {
		t.Fatalf("unknown_engine_id = %d, want 1", v)
	}
	if v := atomic.LoadUint64(&m.errors.authFailures); v != 0 {
		t.Fatalf("auth_failures = %d, want 0", v)
	}
}

func TestCollectorHandlePacketAllowsIPv4MappedSourceCIDR(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "coldStart from {TRAP_SOURCE_IP}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newTestV2Collector("test", writer, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, []string{"public"})
	peer := &net.UDPAddr{IP: net.ParseIP("::ffff:10.1.2.3"), Port: 9162}

	c.handlePacket(packet.payload, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("expected IPv4-mapped peer to match IPv4 CIDR, got %d entries", len(writer.entries))
	}
	if got := writer.entries[0].SourceUDPPeer; got != "10.1.2.3" {
		t.Fatalf("source UDP peer = %q, want unmapped IPv4", got)
	}
}

func TestCollectorHandlePacketAllowsNativeIPv6SourceCIDR(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "coldStart from {TRAP_SOURCE_IP}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newTestV2Collector("test", writer, []netip.Prefix{netip.MustParsePrefix("2001:db8::/32")}, []string{"public"})
	peer := &net.UDPAddr{IP: net.ParseIP("2001:db8::1"), Port: 9162}

	c.handlePacket(packet.payload, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("expected native IPv6 peer to match IPv6 CIDR, got %d entries", len(writer.entries))
	}
}

func TestCollectorHandlePacketRateLimitSampleWritesTrap(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	const jobName = "test-rate-limit-sample"
	withCleanJobMetrics(t, jobName)

	trap := testColdStartTrap("security", "warning", "coldStart from {TRAP_SOURCE_IP}")
	setSingleTestTrap(t, trap)
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	rl := newRateLimiter(true, 1, "sample")
	srcAddr, ok := udpPeerAddr(peer)
	if !ok {
		t.Fatal("failed to convert UDP peer address")
	}
	if allowed, _ := rl.Allow(srcAddr); !allowed {
		t.Fatal("expected first token to be available")
	}

	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, nil, []string{"public"})
	c.rateLimiter = rl

	c.handlePacket(packet.payload, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("sample-mode rate-limited trap should be written, got %d entries", len(writer.entries))
	}
	m := getJobMetrics(jobName)
	if v := atomic.LoadUint64(&m.errors.rateLimited); v != 1 {
		t.Fatalf("rate_limited = %d, want 1", v)
	}
}

func TestCollectMetricsEmitsCounters(t *testing.T) {
	const jobName = "test-metrics"
	withCleanJobMetrics(t, jobName)

	incTrapEvents(jobName, "security")
	incTrapError(jobName, "decode_failed")
	getJobMetrics(jobName).addError("otlp_export_failed", 3)
	getJobMetrics(jobName).addError("listener_read_failed", 2)

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatal("collector store does not expose cycle control")
	}

	managed.CycleController().BeginCycle()
	collectMetrics(store, jobName)
	if err := managed.CycleController().CommitCycleSuccess(); err != nil {
		t.Fatalf("commit collect cycle: %v", err)
	}

	labels := metrix.Labels{"job_name": jobName}
	if v, ok := store.Read().Value("snmp_trap_events_security", labels); !ok || v != 1 {
		t.Fatalf("events security value = %v/%v, want 1/true", v, ok)
	}
	if v, ok := store.Read().Value("snmp_trap_errors_decode_failed", labels); !ok || v != 1 {
		t.Fatalf("errors decode_failed value = %v/%v, want 1/true", v, ok)
	}
	if v, ok := store.Read().Value("snmp_trap_errors_otlp_export_failed", labels); !ok || v != 3 {
		t.Fatalf("errors otlp_export_failed value = %v/%v, want 3/true", v, ok)
	}
	if v, ok := store.Read().Value("snmp_trap_errors_listener_read_failed", labels); !ok || v != 2 {
		t.Fatalf("errors listener_read_failed value = %v/%v, want 2/true", v, ok)
	}
}

func TestCollectorCollectPublishesBinaryEncodedMetric(t *testing.T) {
	const jobName = "test-binary-encoded"
	withCleanJobMetrics(t, jobName)

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatal("collector store does not expose cycle control")
	}

	c := &Collector{
		jobName:    jobName,
		listener:   &Listener{},
		trapWriter: &mockTrapWriter{binaryEncodedFields: 2},
		metrics:    getJobMetrics(jobName),
		store:      store,
	}

	managed.CycleController().BeginCycle()
	if err := c.collect(context.Background()); err != nil {
		t.Fatalf("collect failed: %v", err)
	}
	if err := managed.CycleController().CommitCycleSuccess(); err != nil {
		t.Fatalf("commit collect cycle: %v", err)
	}

	labels := metrix.Labels{"job_name": jobName}
	if v, ok := store.Read().Value("snmp_trap_errors_binary_encoded", labels); !ok || v != 2 {
		t.Fatalf("errors binary_encoded value = %v/%v, want 2/true", v, ok)
	}
}

func TestSnmpEngineBootsPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	prev := engineBootsDirBase
	engineBootsDirBase = tmpDir
	t.Cleanup(func() { engineBootsDirBase = prev })

	jobName := "test-job"

	eb, err := NewEngineBoots(jobName)
	if err != nil {
		t.Fatalf("NewEngineBoots failed: %v", err)
	}
	if v := eb.Value(); v != 1 {
		t.Errorf("expected first boot value 1, got %d", v)
	}

	eb, err = NewEngineBoots(jobName)
	if err != nil {
		t.Fatalf("second NewEngineBoots failed: %v", err)
	}
	if v := eb.Value(); v != 2 {
		t.Errorf("expected second boot value 2, got %d", v)
	}
}

func TestSnmpEngineBootsCorruptFileFailsCreation(t *testing.T) {
	tmpDir := t.TempDir()
	prev := engineBootsDirBase
	engineBootsDirBase = tmpDir
	t.Cleanup(func() { engineBootsDirBase = prev })

	jobName := "test-job"
	if err := os.MkdirAll(engineBootsDir(jobName), 0750); err != nil {
		t.Fatalf("mkdir engine boots dir: %v", err)
	}
	if err := os.WriteFile(engineBootsPath(jobName), []byte("not-a-number\n"), 0640); err != nil {
		t.Fatalf("write engine boots file: %v", err)
	}

	if _, err := NewEngineBoots(jobName); err == nil {
		t.Fatal("expected corrupt engine-boots state to fail job creation")
	}
}

func TestSnmpEngineBootsRejectsMaxValue(t *testing.T) {
	tmpDir := t.TempDir()
	prev := engineBootsDirBase
	engineBootsDirBase = tmpDir
	t.Cleanup(func() { engineBootsDirBase = prev })

	jobName := "test-job"
	if err := os.MkdirAll(engineBootsDir(jobName), 0750); err != nil {
		t.Fatalf("mkdir engine boots dir: %v", err)
	}
	if err := os.WriteFile(engineBootsPath(jobName), []byte("2147483647\n"), 0640); err != nil {
		t.Fatalf("write engine boots file: %v", err)
	}

	if _, err := NewEngineBoots(jobName); err == nil {
		t.Fatal("expected error for max engine boots value")
	}
}

func TestSnmpEngineBootsReadErrorFails(t *testing.T) {
	tmpDir := t.TempDir()
	prev := engineBootsDirBase
	engineBootsDirBase = tmpDir
	t.Cleanup(func() { engineBootsDirBase = prev })

	jobName := "test-job"
	if err := os.MkdirAll(engineBootsPath(jobName), 0750); err != nil {
		t.Fatalf("mkdir engine boots file path: %v", err)
	}

	if _, err := NewEngineBoots(jobName); err == nil {
		t.Fatal("expected read error when engine boots path is a directory")
	}
}

func TestSnmpEngineBootsEngineTimeCapsAtUint32(t *testing.T) {
	eb := &EngineBoots{
		startedAt: time.Now().Add(-time.Duration(uint64(maxSnmpEngineTime)+1) * time.Second),
	}

	if got := eb.EngineTime(); got != maxSnmpEngineTime {
		t.Fatalf("engine time = %d, want %d", got, maxSnmpEngineTime)
	}
}

func TestAllowlistCIDR(t *testing.T) {
	al := NewAllowlist([]netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.168.1.0/24"),
	}, nil)

	tests := map[string]struct {
		addr string
		want bool
	}{
		"inside 10.x":    {addr: "10.1.2.3", want: true},
		"inside 192.168": {addr: "192.168.1.42", want: true},
		"outside":        {addr: "172.16.0.1", want: false},
		"loopback":       {addr: "127.0.0.1", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			addr := netip.MustParseAddr(tc.addr)
			if got := al.AllowedSource(addr); got != tc.want {
				t.Errorf("AllowedSource(%s) = %v, want %v", tc.addr, got, tc.want)
			}
		})
	}
}

func TestAllowlistDefaultIncludesIPv6(t *testing.T) {
	prefixes, err := validateAllowlist(AllowlistConfig{})
	if err != nil {
		t.Fatalf("validateAllowlist failed: %v", err)
	}
	al := NewAllowlist(prefixes, nil)

	if !al.AllowedSource(netip.MustParseAddr("2001:db8::1")) {
		t.Fatal("default allowlist should accept IPv6")
	}
	if !al.AllowedSource(netip.MustParseAddr("192.0.2.1")) {
		t.Fatal("default allowlist should accept IPv4")
	}
}

func TestPacketSourceAddrFallsBackToPeerIP(t *testing.T) {
	addr, ok := packetSourceAddr(net.ParseIP("::ffff:10.1.2.3"), nil)
	if !ok {
		t.Fatal("expected source address from peerIP")
	}
	if got := addr.String(); got != "10.1.2.3" {
		t.Fatalf("source addr = %q, want 10.1.2.3", got)
	}
}

func TestAllowlistCommunity(t *testing.T) {
	al := NewAllowlist(nil, []string{"public", "private"})

	tests := map[string]struct {
		community string
		want      bool
	}{
		"public":  {community: "public", want: true},
		"private": {community: "private", want: true},
		"wrong":   {community: "wrong", want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := al.AllowedCommunity(tc.community); got != tc.want {
				t.Errorf("AllowedCommunity(%s) = %v, want %v", tc.community, got, tc.want)
			}
		})
	}
}

func TestRateLimiterAllow(t *testing.T) {
	rl := newRateLimiter(true, 100, "drop")
	addr := netip.MustParseAddr("10.1.2.3")

	for i := range 100 {
		if allowed, _ := rl.Allow(addr); !allowed && i < 100 {
			t.Fatalf("token bucket exhausted too early at iteration %d", i)
		}
	}

	allowed, mode := rl.Allow(addr)
	if allowed {
		t.Fatal("expected rate limiter to drop after bucket exhausted")
	}
	if mode != rateLimitModeDrop {
		t.Fatal("expected drop mode")
	}
}

func TestRateLimiterDefaults(t *testing.T) {
	if err := validateRateLimit(RateLimitConfig{Enabled: true}); err != nil {
		t.Fatalf("validateRateLimit should accept defaulted enabled config: %v", err)
	}
	rl := newRateLimiter(true, 0, "")
	if !rl.enabled {
		t.Fatal("expected enabled rate limiter")
	}
	if rl.burst != defaultRateLimitPerSourcePPS {
		t.Fatalf("burst = %d, want %d", rl.burst, defaultRateLimitPerSourcePPS)
	}
	if rl.mode != rateLimitModeDrop {
		t.Fatalf("mode = %v, want drop", rl.mode)
	}
}

func TestRateLimiterEvictsOldestTrackedSourceAtCapacity(t *testing.T) {
	rl := newRateLimiter(true, 100, "drop")
	rl.maxSources = 2

	first := netip.MustParseAddr("10.0.0.1")
	second := netip.MustParseAddr("10.0.0.2")
	third := netip.MustParseAddr("10.0.0.3")

	if allowed, _ := rl.Allow(first); !allowed {
		t.Fatal("expected first source to be allowed")
	}
	if allowed, _ := rl.Allow(second); !allowed {
		t.Fatal("expected second source to be allowed")
	}

	now := time.Now()
	rl.mu.Lock()
	rl.buckets[first].lastFill = now.Add(-time.Minute)
	rl.buckets[second].lastFill = now
	rl.mu.Unlock()

	allowed, mode := rl.Allow(third)
	if !allowed {
		t.Fatal("expected new source above cap to evict the oldest tracked source")
	}
	if mode != rateLimitModeDrop {
		t.Fatalf("mode = %v, want drop", mode)
	}
	if got := len(rl.buckets); got != 2 {
		t.Fatalf("tracked sources = %d, want 2", got)
	}
	if _, ok := rl.buckets[first]; ok {
		t.Fatal("expected oldest tracked source to be evicted")
	}
	if _, ok := rl.buckets[second]; !ok {
		t.Fatal("expected second tracked source to remain")
	}
	if _, ok := rl.buckets[third]; !ok {
		t.Fatal("expected new tracked source to be admitted")
	}
}

func TestConfigValidation(t *testing.T) {
	t.Run("valid v3 versions", func(t *testing.T) {
		_, err := validateVersions([]string{"v1", "v2c", "v3"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("valid USM users", func(t *testing.T) {
		err := validateUSMUsers([]USMUserConfig{
			{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "sha256", AuthKey: "authsecret", PrivProto: "aes", PrivKey: "privsecret"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("short auth key", func(t *testing.T) {
		err := validateUSMUsers([]USMUserConfig{
			{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "sha", AuthKey: "short"},
		})
		if err == nil {
			t.Fatal("expected error for short auth key")
		}
	})

	t.Run("short priv key", func(t *testing.T) {
		err := validateUSMUsers([]USMUserConfig{
			{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "sha", AuthKey: "authpass", PrivProto: "aes", PrivKey: "short"},
		})
		if err == nil {
			t.Fatal("expected error for short priv key")
		}
	})

	t.Run("invalid auth proto", func(t *testing.T) {
		err := validateUSMUsers([]USMUserConfig{
			{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "whirlpool"},
		})
		if err == nil {
			t.Fatal("expected error for invalid auth proto")
		}
	})

	t.Run("invalid priv proto", func(t *testing.T) {
		err := validateUSMUsers([]USMUserConfig{
			{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "sha", AuthKey: "authsecret", PrivProto: "3des"},
		})
		if err == nil {
			t.Fatal("expected error for invalid priv proto")
		}
	})

	t.Run("priv without auth", func(t *testing.T) {
		err := validateUSMUsers([]USMUserConfig{
			{Username: "testuser", EngineID: testEngineIDHex, AuthProto: "none", PrivProto: "aes", PrivKey: "privsecret"},
		})
		if err == nil {
			t.Fatal("expected error for priv without auth")
		}
	})

	t.Run("invalid engine ID hex", func(t *testing.T) {
		err := validateEngineIDWhitelist([]string{"nothex"})
		if err == nil {
			t.Fatal("expected error for invalid engine ID hex")
		}
	})

	t.Run("duplicate engine ID whitelist", func(t *testing.T) {
		err := validateEngineIDWhitelist([]string{testEngineIDHex, strings.ToUpper(testEngineIDHex)})
		if err == nil {
			t.Fatal("expected error for duplicate engine ID")
		}
	})

	t.Run("invalid CIDR", func(t *testing.T) {
		_, err := validateAllowlist(AllowlistConfig{SourceCIDRs: []string{"not-a-cidr"}})
		if err == nil {
			t.Fatal("expected error for invalid CIDR")
		}
	})

	t.Run("invalid rate limit mode", func(t *testing.T) {
		err := validateRateLimit(RateLimitConfig{Enabled: true, PerSourcePPS: 100, Mode: "invalid"})
		if err == nil {
			t.Fatal("expected error for invalid rate limit mode")
		}
	})

	t.Run("invalid label key", func(t *testing.T) {
		err := validateConfigLabelKey("INVALID")
		if err == nil {
			t.Fatal("expected error for uppercase label key")
		}
	})

	t.Run("prefixed label keys", func(t *testing.T) {
		for _, key := range []string{"trap_zone", "message_type", "priority_level"} {
			if err := validateConfigLabelKey(key); err != nil {
				t.Fatalf("expected %q to be valid: %v", key, err)
			}
		}
	})

	t.Run("unknown config key", func(t *testing.T) {
		var cfg Config
		err := yaml.Unmarshal([]byte(`
listen:
  endpoints:
    - protocol: udp
      address: "127.0.0.1"
      port: 9162
unexpected: true
`), &cfg)
		if err == nil {
			t.Fatal("expected error for unknown top-level key")
		}
	})

	t.Run("framework metadata keys", func(t *testing.T) {
		var cfg Config
		err := yaml.Unmarshal([]byte(`
name: local
module: snmp_traps
autodetection_retry: 0
priority: 70000
function_only: false
labels:
  role: edge
__provider__: file reader
__source__: discoverer=file_reader,file=snmp_traps.conf
__source_type__: user
listen:
  endpoints:
    - protocol: udp
      address: "127.0.0.1"
      port: 9162
versions: [v2c]
communities: [public]
`), &cfg)
		if err != nil {
			t.Fatalf("expected framework metadata keys to be accepted: %v", err)
		}
	})

	t.Run("unknown nested config key", func(t *testing.T) {
		var cfg Config
		err := yaml.Unmarshal([]byte(`
listen:
  endpoints:
    - protocol: udp
      address: "127.0.0.1"
      port: 9162
      unexpected: true
`), &cfg)
		if err == nil {
			t.Fatal("expected error for unknown endpoint key")
		}
	})

	t.Run("implemented dynamic engine discovery", func(t *testing.T) {
		err := validateDeferredConfig(Config{DynamicEngineID: true})
		if err != nil {
			t.Fatalf("dynamic engine ID discovery should no longer be rejected as deferred: %v", err)
		}
	})

	t.Run("dynamic engine discovery rejects static whitelist", func(t *testing.T) {
		err := validateDeferredConfig(Config{DynamicEngineID: true, EngineIDWhitelist: []string{testEngineIDHex}})
		if err == nil {
			t.Fatal("expected error for dynamic engine ID discovery with engine_id_whitelist")
		}
	})

	t.Run("dynamic engine discovery rejects negative max pairs", func(t *testing.T) {
		err := validateDeferredConfig(Config{DynamicEngineIDMax: -1})
		if err == nil {
			t.Fatal("expected error for negative dynamic_engine_id_max_pairs")
		}
	})

	t.Run("static USM user requires engine ID", func(t *testing.T) {
		err := validateUSMUsers([]USMUserConfig{{Username: "testuser"}})
		if err == nil {
			t.Fatal("expected error for static USM user without engine_id")
		}
	})

	t.Run("dynamic USM user may omit engine ID", func(t *testing.T) {
		err := validateUSMUsers([]USMUserConfig{{Username: "testuser"}}, true)
		if err != nil {
			t.Fatalf("dynamic USM user should allow omitted engine_id: %v", err)
		}
	})

	t.Run("implemented dedup", func(t *testing.T) {
		err := validateDeferredConfig(Config{Dedup: DedupConfig{Enabled: true}})
		if err != nil {
			t.Fatalf("dedup should no longer be rejected as deferred: %v", err)
		}
	})

	t.Run("implemented per-OID metrics", func(t *testing.T) {
		err := validateDeferredConfig(Config{Metrics: []MetricConfig{{OID: "1.3.6.1.4.1.9.0.1", Context: "snmp.trap.test"}}})
		if err != nil {
			t.Fatalf("metrics should no longer be rejected as deferred: %v", err)
		}
	})
}
