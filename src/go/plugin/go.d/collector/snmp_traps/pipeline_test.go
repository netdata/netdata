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

func TestCollectorHandlePacketWritesProfileResolvedTrapEntry(t *testing.T) {
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "security coldStart from {TRAP_SOURCE_IP}",
	}
	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:      "test",
		profileIndex: &ProfileIndex{trapsByOID: map[string]*TrapDef{trap.OID: trap}},
		trapWriter:   writer,
		versions:     map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:    NewAllowlist(nil, []string{"public"}),
	}

	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

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
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	regKey := "test:198.51.100.10:162"
	ddsnmp.DeviceRegistry.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname: "198.51.100.10",
		SysName:  "core-sw-01",
		Vendor:   "cisco",
	})
	defer ddsnmp.DeviceRegistry.Unregister(regKey)

	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "security coldStart on {_HOSTNAME} from {TRAP_DEVICE_VENDOR}",
	}
	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:      "test",
		profileIndex: &ProfileIndex{trapsByOID: map[string]*TrapDef{trap.OID: trap}},
		trapWriter:   writer,
		versions:     map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:    NewAllowlist(nil, []string{"public"}),
	}

	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	if got := writer.entries[0].Message; got != "security coldStart on core-sw-01 from cisco" {
		t.Fatalf("Message = %q", got)
	}
}

func TestCollectorHandlePacketDoesNotUseListenerVnodeAsSourceNode(t *testing.T) {
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "coldStart from {TRAP_SOURCE_IP}",
	}
	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:      "test",
		vnode:        "listener-vnode-id",
		profileIndex: &ProfileIndex{trapsByOID: map[string]*TrapDef{trap.OID: trap}},
		trapWriter:   writer,
		versions:     map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:    NewAllowlist(nil, []string{"public"}),
	}

	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	if got := writer.entries[0].SourceVnodeID; got != "" {
		t.Fatalf("SourceVnodeID = %q, want empty without source device match", got)
	}
}

func TestCollectorHandlePacketRendersTopologyEnrichmentBeforeReverseDNS(t *testing.T) {
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

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

	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "trap on {_HOSTNAME} vendor {TRAP_DEVICE_VENDOR} iface {TRAP_INTERFACE} neighbors {TRAP_NEIGHBORS}",
	}
	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:           "test",
		profileIndex:      &ProfileIndex{trapsByOID: map[string]*TrapDef{trap.OID: trap}},
		trapWriter:        writer,
		versions:          map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:         NewAllowlist(nil, []string{"public"}),
		reverseDNSEnabled: true,
		reverseDNS:        dns,
	}

	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

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
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "coldStart from {TRAP_SOURCE_IP}",
	}
	writer := &mockTrapWriter{}
	metrics := getJobMetrics(jobName)
	metrics.setDedupEnabled(true)
	c := &Collector{
		Config:       Config{Dedup: DedupConfig{Enabled: true}},
		jobName:      jobName,
		profileIndex: &ProfileIndex{trapsByOID: map[string]*TrapDef{trap.OID: trap}},
		trapWriter:   writer,
		versions:     map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:    NewAllowlist(nil, []string{"public"}),
		metrics:      metrics,
	}
	c.deduper = newTrapDeduper(jobName, c.Dedup, writer, metrics, c.profileIndex)
	c.deduper.start()
	defer c.deduper.Close()

	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)
	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries = %d, want 1", len(writer.entries))
	}
	if got := atomic.LoadUint64(&metrics.dedup.suppressed); got != 1 {
		t.Fatalf("dedup suppressed = %d, want 1", got)
	}
	if got := atomic.LoadUint64(&metrics.events.security); got != 1 {
		t.Fatalf("security events = %d, want 1", got)
	}
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
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	t.Run("unknown OID", func(t *testing.T) {
		const jobName = "test-dedup-unknown-oid"
		removeJobMetrics(jobName)
		defer removeJobMetrics(jobName)

		writer := &mockTrapWriter{}
		metrics := getJobMetrics(jobName)
		metrics.setDedupEnabled(true)
		c := &Collector{
			Config:       Config{Dedup: DedupConfig{Enabled: true}},
			jobName:      jobName,
			profileIndex: &ProfileIndex{trapsByOID: map[string]*TrapDef{}},
			trapWriter:   writer,
			versions:     map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
			allowlist:    NewAllowlist(nil, []string{"public"}),
			metrics:      metrics,
		}
		c.deduper = newTrapDeduper(jobName, c.Dedup, writer, metrics, c.profileIndex)

		c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)
		c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

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
	})

	t.Run("template unresolved", func(t *testing.T) {
		const jobName = "test-dedup-template-unresolved"
		removeJobMetrics(jobName)
		defer removeJobMetrics(jobName)

		trap := &TrapDef{
			OID:         "1.3.6.1.6.3.1.1.5.1",
			Name:        "TEST-MIB::coldStartTemplate",
			Category:    "security",
			Severity:    "warning",
			Description: "coldStart from {DOES_NOT_EXIST}",
		}
		writer := &mockTrapWriter{}
		metrics := getJobMetrics(jobName)
		metrics.setDedupEnabled(true)
		c := &Collector{
			Config:       Config{Dedup: DedupConfig{Enabled: true}},
			jobName:      jobName,
			profileIndex: &ProfileIndex{trapsByOID: map[string]*TrapDef{trap.OID: trap}},
			trapWriter:   writer,
			versions:     map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
			allowlist:    NewAllowlist(nil, []string{"public"}),
			metrics:      metrics,
		}
		c.deduper = newTrapDeduper(jobName, c.Dedup, writer, metrics, c.profileIndex)

		c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)
		c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

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
	})
}

func TestCollectorHandlePacketDedupRollsBackFingerprintAfterWriteFailure(t *testing.T) {
	const jobName = "test-dedup-write-rollback"
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "coldStart from {TRAP_SOURCE_IP}",
	}
	writer := &mockTrapWriter{err: errors.New("write failed")}
	metrics := getJobMetrics(jobName)
	metrics.setDedupEnabled(true)
	c := &Collector{
		Config:       Config{Dedup: DedupConfig{Enabled: true}},
		jobName:      jobName,
		profileIndex: &ProfileIndex{trapsByOID: map[string]*TrapDef{trap.OID: trap}},
		trapWriter:   writer,
		versions:     map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:    NewAllowlist(nil, []string{"public"}),
		metrics:      metrics,
	}
	c.deduper = newTrapDeduper(jobName, c.Dedup, writer, metrics, c.profileIndex)

	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)
	if got := atomic.LoadUint64(&metrics.errors.journalWriteFailed); got != 1 {
		t.Fatalf("journal write failures = %d, want 1", got)
	}
	if got := atomic.LoadUint64(&metrics.dedup.suppressed); got != 0 {
		t.Fatalf("dedup suppressed after failed first write = %d, want 0", got)
	}

	writer.err = nil
	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

	if len(writer.entries) != 1 {
		t.Fatalf("written entries after rollback = %d, want 1", len(writer.entries))
	}
	if got := atomic.LoadUint64(&metrics.dedup.suppressed); got != 0 {
		t.Fatalf("dedup suppressed after rollback retry = %d, want 0", got)
	}
	if got := atomic.LoadUint64(&metrics.events.security); got != 1 {
		t.Fatalf("security events = %d, want 1", got)
	}
}

func TestCollectorHandlePacketDropsDisallowedVersion(t *testing.T) {
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:    "test",
		trapWriter: writer,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV3: {}},
		allowlist:  NewAllowlist(nil, nil),
	}

	removeJobMetrics("test")
	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

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

func TestCollectorHandlePacketDropsDisallowedCommunity(t *testing.T) {
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:    "test",
		trapWriter: writer,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:  NewAllowlist(nil, []string{"secret"}),
	}

	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

	if len(writer.entries) != 0 {
		t.Fatalf("expected 0 entries for disallowed community, got %d", len(writer.entries))
	}
}

func TestCollectorHandlePacketIncrementsEventsMetric(t *testing.T) {
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "state_change",
		Severity:    "warning",
		Description: "coldStart from {TRAP_SOURCE_IP}",
	}
	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:      "test",
		profileIndex: &ProfileIndex{trapsByOID: map[string]*TrapDef{trap.OID: trap}},
		trapWriter:   writer,
		versions:     map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:    NewAllowlist(nil, []string{"public"}),
	}

	removeJobMetrics("test")
	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

	m := getJobMetrics("test")
	ev := atomic.LoadUint64(&m.events.stateChange)
	if ev != 1 {
		t.Errorf("expected 1 state_change event, got %d", ev)
	}
	removeJobMetrics("test")
}

func TestCollectorHandlePacketIncrementsTemplateUnresolved(t *testing.T) {
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "security coldStart from {missing_var}",
	}
	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:      "test",
		profileIndex: &ProfileIndex{trapsByOID: map[string]*TrapDef{trap.OID: trap}},
		trapWriter:   writer,
		versions:     map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:    NewAllowlist(nil, []string{"public"}),
	}

	removeJobMetrics("test")
	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

	m := getJobMetrics("test")
	if v := atomic.LoadUint64(&m.errors.templateUnresolved); v != 1 {
		t.Errorf("expected 1 template_unresolved, got %d", v)
	}
	removeJobMetrics("test")
}

func TestCollectorHandlePacketIncrementsAllowlistDrop(t *testing.T) {
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:    "test",
		trapWriter: writer,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:  NewAllowlist(nil, []string{"secret"}),
	}

	removeJobMetrics("test")
	c.handlePacket(packets[0].payload, packets[0].peer, nil, nil)

	m := getJobMetrics("test")
	dr := atomic.LoadUint64(&m.errors.droppedAllowlist)
	if dr != 1 {
		t.Errorf("expected 1 dropped_allowlist, got %d", dr)
	}
	removeJobMetrics("test")
}

func TestCollectorHandlePacketRejectsUnknownV3EngineID(t *testing.T) {
	const jobName = "test-v3-engine"
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	data := buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	secTable, err := buildSnmpV3SecurityTable([]USMUserConfig{{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "none",
		PrivProto: "none",
	}})
	if err != nil {
		t.Fatalf("buildSnmpV3SecurityTable failed: %v", err)
	}
	engineIDs, err := buildEngineIDWhitelist([]string{"80001f888077dfe44faa700259"})
	if err != nil {
		t.Fatalf("buildEngineIDWhitelist failed: %v", err)
	}

	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:    jobName,
		trapWriter: writer,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV3: {}},
		allowlist:  NewAllowlist(nil, nil),
		v3SecTable: secTable,
		engineIDs:  engineIDs,
	}

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
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	otherEngineID := "80001f888077dfe44faa700259"
	data := buildV3SecuredTrap(t, "testuser", testEngineIDHex, "sha256", "aes", "authpassword", "privpassword", "1.3.6.1.6.3.1.1.5.1")
	secTable, err := buildSnmpV3SecurityTable([]USMUserConfig{{
		Username:  "testuser",
		EngineID:  otherEngineID,
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	}})
	if err != nil {
		t.Fatalf("buildSnmpV3SecurityTable failed: %v", err)
	}
	engineIDs, err := buildEngineIDWhitelist([]string{otherEngineID})
	if err != nil {
		t.Fatalf("buildEngineIDWhitelist failed: %v", err)
	}

	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:    jobName,
		trapWriter: writer,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV3: {}},
		allowlist:  NewAllowlist(nil, nil),
		v3SecTable: secTable,
		engineIDs:  engineIDs,
	}

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
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "coldStart from {TRAP_SOURCE_IP}",
	}
	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:      "test",
		profileIndex: &ProfileIndex{trapsByOID: map[string]*TrapDef{trap.OID: trap}},
		trapWriter:   writer,
		versions:     map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:    NewAllowlist([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}, []string{"public"}),
	}
	peer := &net.UDPAddr{IP: net.ParseIP("::ffff:10.1.2.3"), Port: 9162}

	c.handlePacket(packets[0].payload, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("expected IPv4-mapped peer to match IPv4 CIDR, got %d entries", len(writer.entries))
	}
	if got := writer.entries[0].SourceUDPPeer; got != "10.1.2.3" {
		t.Fatalf("source UDP peer = %q, want unmapped IPv4", got)
	}
}

func TestCollectorHandlePacketAllowsNativeIPv6SourceCIDR(t *testing.T) {
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "coldStart from {TRAP_SOURCE_IP}",
	}
	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:      "test",
		profileIndex: &ProfileIndex{trapsByOID: map[string]*TrapDef{trap.OID: trap}},
		trapWriter:   writer,
		versions:     map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:    NewAllowlist([]netip.Prefix{netip.MustParsePrefix("2001:db8::/32")}, []string{"public"}),
	}
	peer := &net.UDPAddr{IP: net.ParseIP("2001:db8::1"), Port: 9162}

	c.handlePacket(packets[0].payload, peer.IP, nil, peer)

	if len(writer.entries) != 1 {
		t.Fatalf("expected native IPv6 peer to match IPv6 CIDR, got %d entries", len(writer.entries))
	}
}

func TestCollectorHandlePacketRateLimitSampleWritesTrap(t *testing.T) {
	packets := readPcapUDPPackets(t, "testdata/v2c_coldstart.pcap.hex")
	if len(packets) != 1 {
		t.Fatalf("expected one packet, got %d", len(packets))
	}

	const jobName = "test-rate-limit-sample"
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	trap := &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    "security",
		Severity:    "warning",
		Description: "coldStart from {TRAP_SOURCE_IP}",
	}
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
	c := &Collector{
		jobName:      jobName,
		profileIndex: &ProfileIndex{trapsByOID: map[string]*TrapDef{trap.OID: trap}},
		trapWriter:   writer,
		versions:     map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:    NewAllowlist(nil, []string{"public"}),
		rateLimiter:  rl,
	}

	c.handlePacket(packets[0].payload, peer.IP, nil, peer)

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
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	incTrapEvents(jobName, "security")
	incTrapError(jobName, "decode_failed")

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
}

func TestCollectorCollectPublishesSanitizedMetric(t *testing.T) {
	const jobName = "test-sanitized"
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatal("collector store does not expose cycle control")
	}

	c := &Collector{
		jobName:    jobName,
		listener:   &Listener{},
		trapWriter: &mockTrapWriter{sanitizedFields: 2},
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
	if v, ok := store.Read().Value("snmp_trap_errors_sanitized", labels); !ok || v != 2 {
		t.Fatalf("errors sanitized value = %v/%v, want 2/true", v, ok)
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

	for i := 0; i < 100; i++ {
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

func TestRateLimiterCapsTrackedSources(t *testing.T) {
	rl := newRateLimiter(true, 100, "drop")
	rl.maxSources = 2

	if allowed, _ := rl.Allow(netip.MustParseAddr("10.0.0.1")); !allowed {
		t.Fatal("expected first source to be allowed")
	}
	if allowed, _ := rl.Allow(netip.MustParseAddr("10.0.0.2")); !allowed {
		t.Fatal("expected second source to be allowed")
	}

	allowed, mode := rl.Allow(netip.MustParseAddr("10.0.0.3"))
	if allowed {
		t.Fatal("expected new source above cap to be rate limited")
	}
	if mode != rateLimitModeDrop {
		t.Fatalf("mode = %v, want drop", mode)
	}
	if got := len(rl.buckets); got != 2 {
		t.Fatalf("tracked sources = %d, want 2", got)
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

	t.Run("deferred dynamic engine discovery", func(t *testing.T) {
		err := validateDeferredConfig(Config{DynamicEngineID: true})
		if err == nil {
			t.Fatal("expected error for dynamic engine ID discovery")
		}
	})

	t.Run("implemented dedup", func(t *testing.T) {
		err := validateDeferredConfig(Config{Dedup: DedupConfig{Enabled: true}})
		if err != nil {
			t.Fatalf("dedup should no longer be rejected as deferred: %v", err)
		}
	})

	t.Run("deferred per-OID metrics", func(t *testing.T) {
		err := validateDeferredConfig(Config{Metrics: []MetricConfig{{OID: "1.3.6.1.4.1.9.0.1", Context: "snmp.trap.test"}}})
		if err == nil {
			t.Fatal("expected error for per-OID metrics")
		}
	})
}
