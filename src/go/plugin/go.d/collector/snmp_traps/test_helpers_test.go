// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"net"
	"net/netip"
	"testing"
)

func readSinglePcapUDPPacket(t *testing.T, fixture string) pcapUDPPacket {
	t.Helper()

	packets := readPcapUDPPackets(t, fixture)
	if len(packets) != 1 {
		t.Fatalf("expected one packet in %s, got %d", fixture, len(packets))
	}
	return packets[0]
}

func readColdStartUDPPacket(t *testing.T) pcapUDPPacket {
	t.Helper()
	return readSinglePcapUDPPacket(t, "testdata/v2c_coldstart.pcap.hex")
}

func testColdStartTrap(category, severity, description string) *TrapDef {
	return &TrapDef{
		OID:         "1.3.6.1.6.3.1.1.5.1",
		Name:        "TEST-MIB::coldStartSecurity",
		Category:    category,
		Severity:    severity,
		Description: description,
	}
}

func setSingleTestTrap(t *testing.T, trap *TrapDef) {
	t.Helper()
	setTestProfileIndex(t, map[string]*TrapDef{trap.OID: trap})
}

func newTestV2Collector(jobName string, writer TrapWriter, prefixes []netip.Prefix, communities []string) *Collector {
	return &Collector{
		jobName:    jobName,
		trapWriter: writer,
		versions:   map[SnmpVersion]struct{}{SnmpVersionV2c: {}},
		allowlist:  NewAllowlist(prefixes, communities),
	}
}

func newDefaultTestV2Collector(writer TrapWriter) *Collector {
	return newTestV2Collector("test", writer, nil, []string{"public"})
}

func defaultDynamicUser() USMUserConfig {
	return USMUserConfig{Username: "testuser", AuthProto: "none", PrivProto: "none"}
}

func newDynamicEngineIDTestCollector(
	t *testing.T,
	jobName string,
	data []byte,
	configure func(*Collector),
) (*Collector, *mockTrapWriter, *net.UDPAddr) {
	t.Helper()

	removeJobMetrics(jobName)
	t.Cleanup(func() { removeJobMetrics(jobName) })

	user := defaultDynamicUser()
	secTable, err := buildSnmpV3SecurityTable([]USMUserConfig{user}, true)
	if err != nil {
		t.Fatalf("buildSnmpV3SecurityTable failed: %v", err)
	}
	writer := &mockTrapWriter{}
	c := &Collector{
		jobName:            jobName,
		trapWriter:         writer,
		Config:             Config{USMUsers: []USMUserConfig{user}},
		versions:           map[SnmpVersion]struct{}{SnmpVersionV3: {}},
		allowlist:          NewAllowlist(nil, nil),
		v3SecTable:         secTable,
		dynamicEngineID:    true,
		dynamicEngineIDMax: defaultDynamicEngineIDMax,
	}
	if configure != nil {
		configure(c)
	}
	c.dynamicEngineIDReg = newDynamicEngineIDRegistry(secTable, c.dynamicEngineIDMax, nil, []USMUserConfig{user})

	peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}
	return c, writer, peer
}
