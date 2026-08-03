// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
)

var currentTestProfileIndex *ProfileIndex

func setTestProfileIndex(t *testing.T, traps map[string]*TrapDef) {
	t.Helper()
	for _, trap := range traps {
		if err := prepareTrapDefinition(trap); err != nil {
			t.Fatalf("compile test trap templates: %v", err)
		}
	}
	idx := newProfileIndex()
	trapDefs := make([]*TrapDef, 0, len(traps))
	for _, trap := range traps {
		trapDefs = append(trapDefs, trap)
	}
	if err := idx.AddTraps(trapDefs); err != nil {
		t.Fatalf("build test profile index: %v", err)
	}
	currentTestProfileIndex = idx
	t.Cleanup(func() { currentTestProfileIndex = nil })
}

func assertSeverityCounters(t *testing.T, metrics *perJobMetrics, want map[string]uint64) {
	t.Helper()

	got := map[string]uint64{
		"emerg":   metrics.severities.emerg.Load(),
		"alert":   metrics.severities.alert.Load(),
		"crit":    metrics.severities.crit.Load(),
		"err":     metrics.severities.err.Load(),
		"warning": metrics.severities.warning.Load(),
		"notice":  metrics.severities.notice.Load(),
		"info":    metrics.severities.info.Load(),
		"debug":   metrics.severities.debug.Load(),
	}

	for name, value := range got {
		if value != want[name] {
			t.Errorf("%s severity = %d, want %d", name, value, want[name])
		}
	}
}

type panicTrapWriter struct{}

func (panicTrapWriter) Write(*TrapEntry) error { panic("trap writer panic") }
func (panicTrapWriter) Flush() error           { return nil }
func (panicTrapWriter) Close() error           { return nil }

func TestCollectorHandlePacketWritesProfileResolvedTrapEntry(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "security coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

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
	if entry.PacketSequence != 1 {
		t.Fatalf("PacketSequence = %d, want 1", entry.PacketSequence)
	}
}

func TestCollectorHandlePacketAssignsReceiveSequencePerPacket(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)
	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 2 {
		t.Fatalf("written entries = %d, want 2", len(writer.entries))
	}
	for i, entry := range writer.entries {
		want := uint64(i + 1)
		if entry.PacketSequence != want {
			t.Fatalf("entry %d PacketSequence = %d, want %d", i, entry.PacketSequence, want)
		}
	}
}

func TestCollectorHandlePacketRecoversFromPanic(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "security coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	metrics := withCleanJobMetrics(t, "panic-recover")
	c := newTestV2Collector("panic-recover", panicTrapWriter{}, nil, []string{"public"})
	c.metrics = metrics

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if got := metrics.errors.decodeFailed.Load(); got != 1 {
		t.Fatalf("decode_failed = %d, want 1", got)
	}
}

func TestCollectorHandlePacketRendersTemplatesAfterEnrichment(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	deviceStore := ddsnmp.NewDeviceStore()
	regKey := "test:198.51.100.10:162"
	deviceStore.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname: "198.51.100.10",
		SysName:  "core-sw-01",
		Vendor:   "cisco",
	})

	trap := testColdStartTrap("security", "warning", "security coldStart on {{hostname}} from {{vendor}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)
	c.enricher = newTestTrapEnricher(deviceStore, nil, nil)

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	if got := writer.entries[0].Message; got != "security coldStart on core-sw-01 from cisco" {
		t.Fatalf("Message = %q", got)
	}
}

func TestCollectorHandlePacketDoesNotUseListenerVnodeAsSourceNode(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)
	c.vnode = "listener-vnode-id"

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	if got := writer.entries[0].SourceVnodeID; got != "" {
		t.Fatalf("SourceVnodeID = %q, want empty without source device match", got)
	}
}

func TestCollectorHandlePacketRendersTopologyEnrichmentBeforeReverseDNS(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	topologyEnricher := testTrapTopologyEnricher(func(ip, ifIndex string) *snmptopology.TrapTopologyEnrichment {
		if ip != "198.51.100.10" {
			t.Fatalf("topology enrichment looked up IP %q, want 198.51.100.10", ip)
		}
		if ifIndex != "" {
			t.Fatalf("topology enrichment got trap ifIndex %q, want empty for coldStart", ifIndex)
		}
		return &snmptopology.TrapTopologyEnrichment{
			DeviceStatus:    "matched",
			DeviceMethod:    "management_ip",
			DeviceMatches:   1,
			DeviceHostname:  "topo-sw-01",
			DeviceVendor:    "arista",
			SourceVnodeID:   "topo-vnode-id",
			InterfaceStatus: "skipped",
			NeighborStatus:  "skipped",
		}
	})

	dns := newTestReverseDNS(map[string]string{"198.51.100.10": "dns-sw-01.example.com"})

	trap := testColdStartTrap(
		"security",
		"warning",
		"trap on {{hostname}} vendor {{vendor}}",
	)
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)
	c.enricher = newTestTrapEnricher(ddsnmp.NewDeviceStore(), topologyEnricher, dns)
	c.reverseDNSEnabled = true

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	entry := writer.entries[0]
	if entry.Message != "trap on topo-sw-01 vendor arista" {
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
	trap := testColdStartTrap("security", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c, metrics := newDedupTestV2Collector(t, jobName, writer)
	c.deduper.start()
	defer c.deduper.Close()

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)
	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	if got := writer.entries[0].PacketSequence; got != 1 {
		t.Fatalf("written PacketSequence = %d, want 1", got)
	}
	if got := c.packetSequence.Load(); got != 2 {
		t.Fatalf("collector packetSequence = %d, want 2", got)
	}
	if got := metrics.dedup.suppressed.Load(); got != 1 {
		t.Fatalf("dedup suppressed = %d, want 1", got)
	}
	if got := metrics.events.security.Load(); got != 1 {
		t.Fatalf("security events = %d, want 1", got)
	}
	assertSeverityCounters(t, metrics, map[string]uint64{"warning": 1})
	if got := metrics.errors.unknownOID.Load(); got != 0 {
		t.Fatalf("unknown OID errors = %d, want 0", got)
	}
	if got := metrics.errors.templateUnresolved.Load(); got != 0 {
		t.Fatalf("template unresolved errors = %d, want 0", got)
	}
	if got := metrics.errors.journalWriteFailed.Load(); got != 0 {
		t.Fatalf("journal write failures = %d, want 0", got)
	}

	store := collectJobMetricsForTest(t, jobName)
	jobLabels := metrix.Labels{"job_name": jobName}
	for name, expected := range map[string]float64{
		"snmp_trap_pipeline_accepted":         2,
		"snmp_trap_pipeline_committed":        1,
		"snmp_trap_pipeline_dedup_suppressed": 1,
	} {
		if v, ok := store.Read().Value(name, jobLabels); !ok || v != expected {
			t.Fatalf("%s = %v/%v, want %v/true", name, v, ok, expected)
		}
	}
}

func TestCollectorHandlePacketDedupPreservesHealthErrorCounters(t *testing.T) {
	packet := readColdStartUDPPacket(t)

	t.Run("unknown OID", func(t *testing.T) {
		const jobName = "test-dedup-unknown-oid"

		setTestProfileIndex(t, map[string]*TrapDef{})
		writer := &mockTrapWriter{}
		c, metrics := newDedupTestV2Collector(t, jobName, writer)

		c.handlePacket(packet.Payload, packet.Peer, nil, nil)
		c.handlePacket(packet.Payload, packet.Peer, nil, nil)

		if len(writer.entries) != 1 {
			t.Fatalf("written entries = %d, want 1", len(writer.entries))
		}
		if got := metrics.dedup.suppressed.Load(); got != 1 {
			t.Fatalf("dedup suppressed = %d, want 1", got)
		}
		if got := metrics.errors.unknownOID.Load(); got != 2 {
			t.Fatalf("unknown OID errors = %d, want 2", got)
		}
		if got := metrics.events.unknown.Load(); got != 1 {
			t.Fatalf("unknown events = %d, want 1", got)
		}
		assertSeverityCounters(t, metrics, map[string]uint64{"notice": 1})
	})

	t.Run("template unresolved", func(t *testing.T) {
		const jobName = "test-dedup-template-unresolved"

		trap := testColdStartTrap("security", "warning", `coldStart from {{with value "missing_var"}}{{.}}{{else}}<missing>{{end}}`)
		trap.VarbindRefs = []any{map[any]any{
			"name": "missing_var",
			"oid":  "1.3.6.1.4.1.99999.1",
			"type": "OctetString",
		}}
		trap.Name = "TEST-MIB::coldStartTemplate"
		setSingleTestTrap(t, trap)
		writer := &mockTrapWriter{}
		c, metrics := newDedupTestV2Collector(t, jobName, writer)

		c.handlePacket(packet.Payload, packet.Peer, nil, nil)
		c.handlePacket(packet.Payload, packet.Peer, nil, nil)

		if len(writer.entries) != 1 {
			t.Fatalf("written entries = %d, want 1", len(writer.entries))
		}
		if got := metrics.dedup.suppressed.Load(); got != 1 {
			t.Fatalf("dedup suppressed = %d, want 1", got)
		}
		if got := metrics.errors.templateUnresolved.Load(); got != 2 {
			t.Fatalf("template unresolved errors = %d, want 2", got)
		}
		if got := metrics.events.security.Load(); got != 1 {
			t.Fatalf("security events = %d, want 1", got)
		}
		assertSeverityCounters(t, metrics, map[string]uint64{"warning": 1})
	})
}

func TestCollectorHandlePacketDedupRollsBackFingerprintAfterWriteFailure(t *testing.T) {
	const jobName = "test-dedup-write-rollback"

	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{err: errors.New("write failed")}
	c, metrics := newDedupTestV2Collector(t, jobName, writer)

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)
	if got := metrics.errors.journalWriteFailed.Load(); got != 1 {
		t.Fatalf("journal write failures = %d, want 1", got)
	}
	store := collectJobMetricsForTest(t, jobName)
	jobLabels := metrix.Labels{"job_name": jobName}
	if v, ok := store.Read().Value("snmp_trap_pipeline_write_failed", jobLabels); !ok || v != 1 {
		t.Fatalf("snmp_trap_pipeline_write_failed = %v/%v, want 1/true", v, ok)
	}
	if got := metrics.dedup.suppressed.Load(); got != 0 {
		t.Fatalf("dedup suppressed after failed first write = %d, want 0", got)
	}

	writer.err = nil
	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries after rollback = %d, want 1", len(writer.entries))
	}
	if got := metrics.dedup.suppressed.Load(); got != 0 {
		t.Fatalf("dedup suppressed after rollback retry = %d, want 0", got)
	}
	if got := metrics.events.security.Load(); got != 1 {
		t.Fatalf("security events = %d, want 1", got)
	}
	assertSeverityCounters(t, metrics, map[string]uint64{"warning": 1})
}

func TestCollectorHandlePacketDropsDisallowedVersion(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	writer := &mockTrapWriter{}
	c := newTestV2CollectorWithPolicy("test", writer, receiver.PolicyConfig{Versions: []string{"v3"}})

	removeJobMetrics("test")
	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 0 {
		t.Fatalf("expected 0 entries for disallowed version, got %d", len(writer.entries))
	}
	m := getJobMetrics("test")
	if dr := m.errors.droppedAllowlist.Load(); dr != 1 {
		t.Errorf("expected 1 dropped_allowlist, got %d", dr)
	}
	store := collectJobMetricsForTest(t, "test")
	labels := metrix.Labels{"job_name": "test"}
	for name, expected := range map[string]float64{
		"snmp_trap_pipeline_received": 1,
		"snmp_trap_pipeline_dropped":  1,
		"snmp_trap_pipeline_accepted": 0,
	} {
		if v, ok := store.Read().Value(name, labels); !ok || v != expected {
			t.Fatalf("%s = %v/%v, want %v/true", name, v, ok, expected)
		}
	}
	removeJobMetrics("test")
}

func TestCollectorHandlePacketDropsDisallowedV3BeforeDecode(t *testing.T) {
	const jobName = "test-disallowed-v3"
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	data := buildV3Trap(t, "testuser", "1.3.6.1.6.3.1.1.5.1")
	writer := &mockTrapWriter{}
	c := newTestV2CollectorWithPolicy(jobName, writer, receiver.PolicyConfig{Versions: []string{"v2c"}})

	c.handlePacket(data, net.ParseIP("10.1.2.3"), nil, &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162})

	if len(writer.entries) != 0 {
		t.Fatalf("expected 0 entries for disallowed v3 packet, got %d", len(writer.entries))
	}
	m := getJobMetrics(jobName)
	if v := m.errors.droppedAllowlist.Load(); v != 1 {
		t.Fatalf("dropped_allowlist = %d, want 1", v)
	}
	if v := m.errors.authFailures.Load(); v != 0 {
		t.Fatalf("auth_failures = %d, want 0", v)
	}
	if v := m.errors.decodeFailed.Load(); v != 0 {
		t.Fatalf("decode_failed = %d, want 0", v)
	}
}

func TestCollectorHandlePacketWritesDecodeErrorEntry(t *testing.T) {
	const jobName = "test-decode-error-entry"
	metrics := withCleanJobMetrics(t, jobName)
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, nil, []string{"public"})

	data := make([]byte, 64*1024)
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
	if entry.DecodeError.PacketSize != len(data) {
		t.Fatalf("DecodeError.PacketSize = %d, want %d", entry.DecodeError.PacketSize, len(data))
	}
	if entry.PacketSequence != 1 {
		t.Fatalf("PacketSequence = %d, want 1", entry.PacketSequence)
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
	if got := metrics.errors.malformedPDU.Load(); got != 1 {
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
	if got := metrics.errors.droppedAllowlist.Load(); got != 1 {
		t.Fatalf("dropped_allowlist = %d, want 1", got)
	}
	if got := metrics.errors.decodeFailed.Load(); got != 0 {
		t.Fatalf("decode_failed = %d, want 0", got)
	}
}

func TestCollectorHandlePacketDecodeErrorHonorsRateLimitDrop(t *testing.T) {
	const jobName = "test-decode-error-rate-limit"
	metrics := withCleanJobMetrics(t, jobName)
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, nil, []string{"public"})
	c.receiver = newTestReceiver(c, receiver.PolicyConfig{
		Versions:    []string{"v2c"},
		Communities: []string{"public"},
		RateLimit: receiver.RateLimitConfig{
			Enabled:      true,
			PerSourcePPS: 1,
			Mode:         "drop",
		},
	})

	data := make([]byte, 64*1024)
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	c.handlePacket(data, peer.IP, nil, peer)
	c.handlePacket(data, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	if got := metrics.errors.malformedPDU.Load(); got != 2 {
		t.Fatalf("malformed_pdu = %d, want 2", got)
	}
	if got := metrics.errors.rateLimited.Load(); got != 1 {
		t.Fatalf("rate_limited = %d, want 1", got)
	}
}

func TestCollectorHandlePacketDynamicDecodeFailureReusesRateLimitAdmission(t *testing.T) {
	const jobName = "test-dynamic-decode-error-rate-limit"
	metrics := withCleanJobMetrics(t, jobName)
	writer := &mockTrapWriter{}
	c := &Collector{
		Config:      Config{Name: jobName},
		trapWriter:  writer,
		journalHost: newTestJournalHostProvider(),
	}
	user := USMUserConfig{
		Username:  "testuser",
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	}
	c.receiver = newTestReceiver(c, receiver.PolicyConfig{
		Versions:        []string{"v3"},
		USMUsers:        toReceiverUSMUsers([]USMUserConfig{user}),
		DynamicEngineID: true,
		RateLimit: receiver.RateLimitConfig{
			Enabled:      true,
			PerSourcePPS: 1,
			Mode:         "drop",
		},
	})
	if err := c.receiver.PrepareV3(t.TempDir(), jobName); err != nil {
		t.Fatalf("prepare test v3 receiver: %v", err)
	}
	t.Cleanup(c.receiver.RollbackPreparedState)

	data := buildV3SecuredTrapWithFlags(t, v3SecuredTrapSpec{
		user:        "testuser",
		engineIDHex: testEngineIDHex,
		authProto:   "sha256",
		privProto:   "aes",
		authKey:     "wrongpassword",
		privKey:     "privpassword",
		trapOID:     "1.3.6.1.6.3.1.1.5.1",
	}, gosnmp.AuthPriv)
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	c.handlePacket(data, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want first admitted decode error", len(writer.entries))
	}
	if got := metrics.errors.rateLimited.Load(); got != 0 {
		t.Fatalf("rate_limited = %d, want 0", got)
	}
}

func TestCollectorHandlePacketDecodeErrorNormalizesIPv4MappedSource(t *testing.T) {
	const jobName = "test-decode-error-ipv4-mapped"
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, []string{"public"})

	data := make([]byte, 64*1024)
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
	if got := metrics.errors.droppedAllowlist.Load(); got != 1 {
		t.Fatalf("dropped_allowlist = %d, want 1", got)
	}
}

func TestCollectorHandlePacketDropsDisallowedCommunity(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	writer := &mockTrapWriter{}
	c := newTestV2Collector("test", writer, nil, []string{"secret"})

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 0 {
		t.Fatalf("expected 0 entries for disallowed community, got %d", len(writer.entries))
	}
}

func TestCollectorHandlePacketAllowsAllowedCommunity(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("state_change", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newTestV2Collector("test", writer, nil, []string{"public"})

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
}

func TestCollectorHandlePacketIncrementsEventsMetric(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("state_change", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)

	withCleanJobMetrics(t, "test")
	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	m := getJobMetrics("test")
	ev := m.events.stateChange.Load()
	if ev != 1 {
		t.Errorf("expected 1 state_change event, got %d", ev)
	}
}

func TestCollectorHandlePacketIncrementsSeverityMetric(t *testing.T) {
	const jobName = "test-severity-event"
	withCleanJobMetrics(t, jobName)

	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("state_change", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, nil, []string{"public"})

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	m := getJobMetrics(jobName)
	assertSeverityCounters(t, m, map[string]uint64{"warning": 1})
}

func TestPerJobMetricsIncSeverityFallsBackToNotice(t *testing.T) {
	m := &perJobMetrics{}
	m.incSeverity("")
	m.incSeverity("not_a_severity")

	if v := m.severities.notice.Load(); v != 2 {
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
	trap := testColdStartTrap("security", "warning", `security coldStart from {{with value "missing_var"}}{{.}}{{else}}<missing>{{end}}`)
	trap.VarbindRefs = []any{map[any]any{
		"name": "missing_var",
		"oid":  "1.3.6.1.4.1.99999.1",
		"type": "OctetString",
	}}
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newDefaultTestV2Collector(writer)

	withCleanJobMetrics(t, "test")
	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	m := getJobMetrics("test")
	if v := m.errors.templateUnresolved.Load(); v != 1 {
		t.Errorf("expected 1 template_unresolved, got %d", v)
	}
}

func TestCollectorHandlePacketIncrementsAllowlistDrop(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	writer := &mockTrapWriter{}
	c := newTestV2Collector("test", writer, nil, []string{"secret"})

	withCleanJobMetrics(t, "test")
	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	m := getJobMetrics("test")
	dr := m.errors.droppedAllowlist.Load()
	if dr != 1 {
		t.Errorf("expected 1 dropped_allowlist, got %d", dr)
	}
}

func TestCollectorHandlePacketRejectsUnknownV3EngineID(t *testing.T) {
	const jobName = "test-v3-engine"
	withCleanJobMetrics(t, jobName)

	data := buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")

	writer := &mockTrapWriter{}
	c := newTestV3Collector(t, jobName, writer, []USMUserConfig{testNoAuthV3User(testEngineIDHex)}, []string{"80001f888077dfe44faa700259"})

	c.handlePacket(data, net.ParseIP("10.1.2.3"), nil, &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162})

	if len(writer.entries) != 0 {
		t.Fatalf("expected unknown engine ID to drop trap, got %d entries", len(writer.entries))
	}
	m := getJobMetrics(jobName)
	if v := m.errors.unknownEngineID.Load(); v != 1 {
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
	user := USMUserConfig{
		Username:  "testuser",
		EngineID:  otherEngineID,
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	}

	writer := &mockTrapWriter{}
	c := newTestV3Collector(t, jobName, writer, []USMUserConfig{user}, []string{otherEngineID})

	c.handlePacket(data, net.ParseIP("10.1.2.3"), nil, &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162})

	m := getJobMetrics(jobName)
	if v := m.errors.unknownEngineID.Load(); v != 1 {
		t.Fatalf("unknown_engine_id = %d, want 1", v)
	}
	if v := m.errors.authFailures.Load(); v != 0 {
		t.Fatalf("auth_failures = %d, want 0", v)
	}
}

func TestCollectorHandlePacketAllowsIPv4MappedSourceCIDR(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newTestV2Collector("test", writer, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, []string{"public"})
	peer := &net.UDPAddr{IP: net.ParseIP("::ffff:10.1.2.3"), Port: 9162}

	c.handlePacket(packet.Payload, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("expected IPv4-mapped peer to match IPv4 CIDR, got %d entries", len(writer.entries))
	}
	if got := writer.entries[0].SourceUDPPeer; got != "10.1.2.3" {
		t.Fatalf("source UDP peer = %q, want unmapped IPv4", got)
	}
}

func TestCollectorHandlePacketAllowsNativeIPv6SourceCIDR(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("security", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newTestV2Collector("test", writer, []netip.Prefix{netip.MustParsePrefix("2001:db8::/32")}, []string{"public"})
	peer := &net.UDPAddr{IP: net.ParseIP("2001:db8::1"), Port: 9162}

	c.handlePacket(packet.Payload, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("expected native IPv6 peer to match IPv6 CIDR, got %d entries", len(writer.entries))
	}
}

func TestCollectorHandlePacketUsesSnmpTrapAddressOnlyForTrustedRelay(t *testing.T) {
	trap := testColdStartTrap("security", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	data := buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.1", gosnmp.SnmpPDU{
		Name:  model.SNMPTrapAddressOID,
		Type:  gosnmp.IPAddress,
		Value: "192.0.2.20",
	})
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}

	t.Run("untrusted_peer_uses_udp_peer", func(t *testing.T) {
		writer := &mockTrapWriter{}
		c := newTestV2Collector("test-untrusted-relay", writer, nil, []string{"public"})

		c.handlePacket(data, peer.IP, nil, peer)

		if len(writer.entries) != 1 {
			t.Fatalf("written entries = %d, want 1", len(writer.entries))
		}
		entry := writer.entries[0]
		if entry.SourceIP != "10.1.2.3" {
			t.Fatalf("SourceIP = %q, want UDP peer", entry.SourceIP)
		}
		if entry.Enrichment == nil || entry.Enrichment.Source == nil {
			t.Fatal("missing source audit")
		}
		if entry.Enrichment.Source.Method != "udp_peer" {
			t.Fatalf("source method = %q, want udp_peer", entry.Enrichment.Source.Method)
		}
	})

	t.Run("trusted_peer_uses_snmpTrapAddress", func(t *testing.T) {
		writer := &mockTrapWriter{}
		c := newTestV2CollectorWithPolicy("test-trusted-relay", writer, receiver.PolicyConfig{
			Versions:      []string{"v2c"},
			Communities:   []string{"public"},
			TrustedRelays: []netip.Prefix{netip.MustParsePrefix("10.1.2.0/24")},
		})

		c.handlePacket(data, peer.IP, nil, peer)

		if len(writer.entries) != 1 {
			t.Fatalf("written entries = %d, want 1", len(writer.entries))
		}
		entry := writer.entries[0]
		if entry.SourceIP != "192.0.2.20" {
			t.Fatalf("SourceIP = %q, want relayed snmpTrapAddress.0", entry.SourceIP)
		}
		if entry.SourceUDPPeer != "10.1.2.3" {
			t.Fatalf("SourceUDPPeer = %q, want UDP peer", entry.SourceUDPPeer)
		}
		if entry.Enrichment == nil || entry.Enrichment.Source == nil {
			t.Fatal("missing source audit")
		}
		if entry.Enrichment.Source.Method != "trusted_relay_snmpTrapAddress.0" || !entry.Enrichment.Source.TrustedRelay {
			t.Fatalf("source audit = %+v, want trusted relay source", entry.Enrichment.Source)
		}
	})

	t.Run("ipv4_mapped_trusted_peer_matches_ipv4_cidr", func(t *testing.T) {
		writer := &mockTrapWriter{}
		c := newTestV2CollectorWithPolicy("test-trusted-relay-mapped", writer, receiver.PolicyConfig{
			Versions:      []string{"v2c"},
			Communities:   []string{"public"},
			TrustedRelays: []netip.Prefix{netip.MustParsePrefix("10.1.2.0/24")},
		})
		mappedPeer := &net.UDPAddr{IP: net.ParseIP("::ffff:10.1.2.3"), Port: 9162}

		c.handlePacket(data, mappedPeer.IP, nil, mappedPeer)

		if len(writer.entries) != 1 {
			t.Fatalf("written entries = %d, want 1", len(writer.entries))
		}
		entry := writer.entries[0]
		if entry.SourceIP != "192.0.2.20" {
			t.Fatalf("SourceIP = %q, want relayed snmpTrapAddress.0", entry.SourceIP)
		}
		if entry.SourceUDPPeer != "10.1.2.3" {
			t.Fatalf("SourceUDPPeer = %q, want normalized UDP peer", entry.SourceUDPPeer)
		}
		if entry.Enrichment == nil || entry.Enrichment.Source == nil {
			t.Fatal("missing source audit")
		}
		if entry.Enrichment.Source.Method != "trusted_relay_snmpTrapAddress.0" || !entry.Enrichment.Source.TrustedRelay {
			t.Fatalf("source audit = %+v, want trusted relay source", entry.Enrichment.Source)
		}
	})

	t.Run("missing_peer_never_trusts_snmpTrapAddress", func(t *testing.T) {
		writer := &mockTrapWriter{}
		c := newTestV2CollectorWithPolicy("test-trusted-relay-missing-peer", writer, receiver.PolicyConfig{
			Versions:      []string{"v2c"},
			Communities:   []string{"public"},
			TrustedRelays: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")},
		})

		c.handlePacket(data, nil, nil, nil)

		if len(writer.entries) != 0 {
			t.Fatalf("written entries = %d, want 0 without UDP peer", len(writer.entries))
		}
	})

	t.Run("catch_all_trusted_peer_uses_snmpTrapAddress", func(t *testing.T) {
		writer := &mockTrapWriter{}
		c := newTestV2CollectorWithPolicy("test-trusted-relay-catch-all", writer, receiver.PolicyConfig{
			Versions:      []string{"v2c"},
			Communities:   []string{"public"},
			TrustedRelays: []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")},
		})

		c.handlePacket(data, peer.IP, nil, peer)

		if len(writer.entries) != 1 {
			t.Fatalf("written entries = %d, want 1", len(writer.entries))
		}
		entry := writer.entries[0]
		if entry.SourceIP != "192.0.2.20" {
			t.Fatalf("SourceIP = %q, want relayed snmpTrapAddress.0", entry.SourceIP)
		}
		if entry.Enrichment == nil || entry.Enrichment.Source == nil {
			t.Fatal("missing source audit")
		}
		if entry.Enrichment.Source.Method != "trusted_relay_snmpTrapAddress.0" || !entry.Enrichment.Source.TrustedRelay {
			t.Fatalf("source audit = %+v, want catch-all trusted relay source", entry.Enrichment.Source)
		}
	})
}

func TestCollectorHandlePacketRateLimitSampleWritesTrap(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	const jobName = "test-rate-limit-sample"
	withCleanJobMetrics(t, jobName)

	trap := testColdStartTrap("security", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	writer := &mockTrapWriter{}
	c := newTestV2CollectorWithPolicy(jobName, writer, receiver.PolicyConfig{
		Versions:    []string{"v2c"},
		Communities: []string{"public"},
		RateLimit: receiver.RateLimitConfig{
			Enabled:      true,
			PerSourcePPS: 1,
			Mode:         "sample",
		},
	})
	if result := c.receiver.Process(receiver.Datagram{
		Data: packet.Payload, PeerIP: peer.IP, Peer: peer,
	}); result.PDU == nil {
		t.Fatalf("first packet result = %+v, want accepted packet to consume the initial token", result)
	}

	c.handlePacket(packet.Payload, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("sample-mode rate-limited trap should be written, got %d entries", len(writer.entries))
	}
	m := getJobMetrics(jobName)
	if v := m.errors.rateLimited.Load(); v != 1 {
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
	getJobMetrics(jobName).addError("listener_buffer_degraded", 1)

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
	if v, ok := store.Read().Value("snmp_trap_errors_listener_buffer_degraded", labels); !ok || v != 1 {
		t.Fatalf("errors listener_buffer_degraded value = %v/%v, want 1/true", v, ok)
	}
}

func TestCollectorHandlePacketEmitsPipelineMetrics(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	const jobName = "test-pipeline-metrics"
	metrics := withCleanJobMetrics(t, jobName)

	trap := testColdStartTrap("security", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, nil, []string{"public"})
	c.metrics = metrics

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}

	store := collectJobMetricsForTest(t, jobName)
	jobLabels := metrix.Labels{"job_name": jobName}
	for name, expected := range map[string]float64{
		"snmp_trap_pipeline_received":  1,
		"snmp_trap_pipeline_decoded":   1,
		"snmp_trap_pipeline_accepted":  1,
		"snmp_trap_pipeline_committed": 1,
		"snmp_trap_pipeline_dropped":   0,
	} {
		if v, ok := store.Read().Value(name, jobLabels); !ok || v != expected {
			t.Fatalf("%s = %v/%v, want %v/true", name, v, ok, expected)
		}
	}
}

func TestCollectorCollectEmitsBuiltInAndProfileMetrics(t *testing.T) {
	const jobName = "test-built-in-and-profile-metrics"
	metrics := withCleanJobMetrics(t, jobName)
	metrics.incEvent("security")

	rt := newRootTestProfileMetricRuntime(t)
	entry := rootTestCiscoConfigTrapEntry(jobName)
	rt.Update(entry)

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatal("collector store does not expose cycle control")
	}

	c := &Collector{
		Config:         Config{Name: jobName},
		trapWriter:     &mockTrapWriter{},
		metrics:        metrics,
		profileMetrics: rt,
		store:          store,
	}
	c.receiver = bindTestReceiver(t, c)

	managed.CycleController().BeginCycle()
	if err := c.collect(context.Background()); err != nil {
		t.Fatalf("collect failed: %v", err)
	}
	if err := managed.CycleController().CommitCycleSuccess(); err != nil {
		t.Fatalf("commit collect cycle: %v", err)
	}

	jobLabels := metrix.Labels{"job_name": jobName}
	if v, ok := store.Read().Value("snmp_trap_events_security", jobLabels); !ok || v != 1 {
		t.Fatalf("snmp_trap_events_security = %v/%v, want 1/true", v, ok)
	}

	profileLabels := rootTestProfileMetricSourceLabels(entry, "test")
	if v, ok := store.Read().Value("snmp_trap_cisco_config_events", profileLabels); !ok || v != 1 {
		t.Fatalf("snmp_trap_cisco_config_events = %v/%v, want 1/true", v, ok)
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
		Config:     Config{Name: jobName},
		trapWriter: &mockTrapWriter{binaryEncodedFields: 2},
		metrics:    getJobMetrics(jobName),
		store:      store,
	}
	c.receiver = bindTestReceiver(t, c)

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
