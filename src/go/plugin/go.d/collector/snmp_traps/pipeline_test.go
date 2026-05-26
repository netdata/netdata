// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import "testing"

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
		communities:  map[string]struct{}{"public": {}},
	}

	c.handlePacket(packets[0].payload, packets[0].peer)

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
