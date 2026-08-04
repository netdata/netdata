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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/receiver"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/telemetry"
)

var currentTestProfileIndex *catalog.Epoch

func setTestProfileIndex(t *testing.T, traps map[string]*TrapDef) {
	t.Helper()
	for _, trap := range traps {
		if err := catalog.PrepareTrap(trap); err != nil {
			t.Fatalf("compile test trap templates: %v", err)
		}
	}
	idx := catalog.NewEpoch()
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

func assertSeverityCounters(t *testing.T, job *telemetry.Job, jobName string, want map[string]uint64) {
	t.Helper()
	store := collectJobMetricsForTest(t, job)
	labels := metrix.Labels{"job_name": jobName}
	for _, name := range []string{"emerg", "alert", "crit", "err", "warning", "notice", "info", "debug"} {
		metric := "snmp_trap_severity_" + name
		value, ok := store.Read().Value(metric, labels)
		if !ok || value != float64(want[name]) {
			t.Errorf("%s = %v/%v, want %d/true", metric, value, ok, want[name])
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
	c := newTestV2Collector("panic-recover", panicTrapWriter{}, nil, []string{"public"})

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)
	assertJobMetric(t, c.telemetry, "panic-recover", "snmp_trap_errors_decode_failed", 1)
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
	c.deduper.Start()
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
	assertJobMetric(t, metrics, jobName, "snmp_trap_dedup_suppressed", 1)
	assertJobMetric(t, metrics, jobName, "snmp_trap_events_security", 1)
	assertSeverityCounters(t, metrics, jobName, map[string]uint64{"warning": 1})
	assertJobMetric(t, metrics, jobName, "snmp_trap_errors_unknown_oid", 0)
	assertJobMetric(t, metrics, jobName, "snmp_trap_errors_template_unresolved", 0)
	assertJobMetric(t, metrics, jobName, "snmp_trap_errors_journal_write_failed", 0)

	store := collectJobMetricsForTest(t, metrics)
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

func TestSelectDedupKeyVarbindsPrefersProfileKeys(t *testing.T) {
	jobKeys := []string{"jobKey"}
	assert := func(got, want []string) {
		t.Helper()
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("selected keys = %v, want %v", got, want)
		}
	}

	assert(selectDedupKeyVarbinds(nil, jobKeys), jobKeys)
	assert(selectDedupKeyVarbinds(&TrapDef{}, jobKeys), jobKeys)
	assert(selectDedupKeyVarbinds(&TrapDef{DedupKeyVarbinds: []string{"profileKey"}}, jobKeys), []string{"profileKey"})
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
		assertJobMetric(t, metrics, jobName, "snmp_trap_dedup_suppressed", 1)
		assertJobMetric(t, metrics, jobName, "snmp_trap_errors_unknown_oid", 2)
		assertJobMetric(t, metrics, jobName, "snmp_trap_events_unknown", 1)
		assertSeverityCounters(t, metrics, jobName, map[string]uint64{"notice": 1})
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
		assertJobMetric(t, metrics, jobName, "snmp_trap_dedup_suppressed", 1)
		assertJobMetric(t, metrics, jobName, "snmp_trap_errors_template_unresolved", 2)
		assertJobMetric(t, metrics, jobName, "snmp_trap_events_security", 1)
		assertSeverityCounters(t, metrics, jobName, map[string]uint64{"warning": 1})
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
	assertJobMetric(t, metrics, jobName, "snmp_trap_errors_journal_write_failed", 1)
	store := collectJobMetricsForTest(t, metrics)
	jobLabels := metrix.Labels{"job_name": jobName}
	if v, ok := store.Read().Value("snmp_trap_pipeline_write_failed", jobLabels); !ok || v != 1 {
		t.Fatalf("snmp_trap_pipeline_write_failed = %v/%v, want 1/true", v, ok)
	}
	assertJobMetric(t, metrics, jobName, "snmp_trap_dedup_suppressed", 0)

	writer.err = nil
	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries after rollback = %d, want 1", len(writer.entries))
	}
	assertJobMetric(t, metrics, jobName, "snmp_trap_dedup_suppressed", 0)
	assertJobMetric(t, metrics, jobName, "snmp_trap_events_security", 1)
	assertSeverityCounters(t, metrics, jobName, map[string]uint64{"warning": 1})
}

func TestCollectorHandlePacketDropsDisallowedVersion(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	writer := &mockTrapWriter{}
	c := newTestV2CollectorWithPolicy("test", writer, receiver.PolicyConfig{Versions: []string{"v3"}})

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 0 {
		t.Fatalf("expected 0 entries for disallowed version, got %d", len(writer.entries))
	}
	assertJobMetric(t, c.telemetry, "test", "snmp_trap_errors_dropped_allowlist", 1)
	store := collectJobMetricsForTest(t, c.telemetry)
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
}

func TestCollectorHandlePacketDropsDisallowedV3BeforeDecode(t *testing.T) {
	const jobName = "test-disallowed-v3"
	data := buildV3Trap(t, "testuser", "1.3.6.1.6.3.1.1.5.1")
	writer := &mockTrapWriter{}
	c := newTestV2CollectorWithPolicy(jobName, writer, receiver.PolicyConfig{Versions: []string{"v2c"}})

	c.handlePacket(data, net.ParseIP("10.1.2.3"), nil, &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162})

	if len(writer.entries) != 0 {
		t.Fatalf("expected 0 entries for disallowed v3 packet, got %d", len(writer.entries))
	}
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_dropped_allowlist", 1)
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_auth_failures", 0)
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_decode_failed", 0)
}

func TestCollectorHandlePacketWritesDecodeErrorEntry(t *testing.T) {
	const jobName = "test-decode-error-entry"
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
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_malformed_pdu", 1)
}

func TestCollectorHandlePacketDecodeErrorHonorsAllowlist(t *testing.T) {
	const jobName = "test-decode-error-allowlist"
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, []string{"public"})

	peer := &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 9162}
	c.handlePacket([]byte{0xff, 0x00}, peer.IP, nil, peer)

	if len(writer.entries) != 0 {
		t.Fatalf("written entries = %d, want 0", len(writer.entries))
	}
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_dropped_allowlist", 1)
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_decode_failed", 0)
}

func TestCollectorHandlePacketDecodeErrorHonorsRateLimitDrop(t *testing.T) {
	const jobName = "test-decode-error-rate-limit"
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
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_malformed_pdu", 2)
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_rate_limited", 1)
}

func TestCollectorHandlePacketDynamicDecodeFailureReusesRateLimitAdmission(t *testing.T) {
	const jobName = "test-dynamic-decode-error-rate-limit"
	writer := &mockTrapWriter{}
	registry := telemetry.NewRegistry()
	c := &Collector{
		Config:            Config{Name: jobName},
		trapWriter:        writer,
		journalHost:       newTestJournalHostProvider(),
		telemetryRegistry: registry,
		telemetry:         registry.Attach(jobName, telemetry.Options{}),
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
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_rate_limited", 0)
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
	c.handlePacket([]byte{0x30, 0x00}, nil, nil, nil)

	if len(writer.entries) != 0 {
		t.Fatalf("written entries = %d, want 0", len(writer.entries))
	}
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_dropped_allowlist", 1)
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

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)
	assertJobMetric(t, c.telemetry, "test", "snmp_trap_events_state_change", 1)
}

func TestCollectorHandlePacketIncrementsSeverityMetric(t *testing.T) {
	const jobName = "test-severity-event"
	packet := readColdStartUDPPacket(t)
	trap := testColdStartTrap("state_change", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, nil, []string{"public"})

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	assertSeverityCounters(t, c.telemetry, jobName, map[string]uint64{"warning": 1})
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

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)
	assertJobMetric(t, c.telemetry, "test", "snmp_trap_errors_template_unresolved", 1)
}

func TestCollectorHandlePacketIncrementsAllowlistDrop(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	writer := &mockTrapWriter{}
	c := newTestV2Collector("test", writer, nil, []string{"secret"})

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)
	assertJobMetric(t, c.telemetry, "test", "snmp_trap_errors_dropped_allowlist", 1)
}

func TestCollectorHandlePacketRejectsUnknownV3EngineID(t *testing.T) {
	const jobName = "test-v3-engine"
	data := buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")

	writer := &mockTrapWriter{}
	c := newTestV3Collector(t, jobName, writer, []USMUserConfig{testNoAuthV3User(testEngineIDHex)}, []string{"80001f888077dfe44faa700259"})

	c.handlePacket(data, net.ParseIP("10.1.2.3"), nil, &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162})

	if len(writer.entries) != 0 {
		t.Fatalf("expected unknown engine ID to drop trap, got %d entries", len(writer.entries))
	}
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_unknown_engine_id", 1)
}

func TestCollectorHandlePacketClassifiesAuthFailureUnknownV3EngineID(t *testing.T) {
	const jobName = "test-v3-auth-failure-engine"
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

	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_unknown_engine_id", 1)
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_auth_failures", 0)
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
	assertJobMetric(t, c.telemetry, jobName, "snmp_trap_errors_rate_limited", 1)
}

func TestCollectorHandlePacketEmitsPipelineMetrics(t *testing.T) {
	packet := readColdStartUDPPacket(t)
	const jobName = "test-pipeline-metrics"

	trap := testColdStartTrap("security", "warning", "coldStart from {{source_ip}}")
	setSingleTestTrap(t, trap)
	writer := &mockTrapWriter{}
	c := newTestV2Collector(jobName, writer, nil, []string{"public"})

	c.handlePacket(packet.Payload, packet.Peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}

	store := collectJobMetricsForTest(t, c.telemetry)
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
	metrics := newTestJobTelemetry(t, jobName, false)
	metrics.Event(model.Category("security"))

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
		telemetry:      metrics,
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
	metrics := newTestJobTelemetry(t, jobName, false)

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatal("collector store does not expose cycle control")
	}

	c := &Collector{
		Config:     Config{Name: jobName},
		trapWriter: &mockTrapWriter{binaryEncodedFields: 2},
		telemetry:  metrics,
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
