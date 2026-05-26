// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
)

func marshalPacket(t *testing.T, pkt *gosnmp.SnmpPacket) []byte {
	t.Helper()
	data, err := pkt.MarshalMsg()
	if err != nil {
		t.Fatalf("failed to marshal test packet: %v", err)
	}
	return data
}

func buildV2cTrap(t *testing.T, community, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	t.Helper()
	return buildV2cPDU(t, gosnmp.SNMPv2Trap, community, trapOID, extra...)
}

func buildV2cPDU(t *testing.T, pduType gosnmp.PDUType, community, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	t.Helper()
	x := &gosnmp.GoSNMP{Version: gosnmp.Version2c, Community: community}
	pdus := []gosnmp.SnmpPDU{
		{Name: sysUpTimeOID, Type: gosnmp.TimeTicks, Value: uint32(10)},
		{Name: snmpTrapOIDOID, Type: gosnmp.ObjectIdentifier, Value: trapOID},
	}
	pdus = append(pdus, extra...)
	return marshalPacket(t, x.MkSnmpPacket(pduType, pdus, 0, 0))
}

func buildV1Trap(t *testing.T, community, agentAddr string, genericTrap, specificTrap int, extra ...gosnmp.SnmpPDU) []byte {
	t.Helper()
	return marshalPacket(t, &gosnmp.SnmpPacket{
		Version:   gosnmp.Version1,
		Community: community,
		PDUType:   gosnmp.Trap,
		SnmpTrap: gosnmp.SnmpTrap{
			Enterprise:   "1.3.6.1.4.1.9",
			AgentAddress: agentAddr,
			GenericTrap:  genericTrap,
			SpecificTrap: specificTrap,
			Timestamp:    10,
			Variables:    extra,
		},
	})
}

func TestMinimalV2cDecode(t *testing.T) {
	pkt, err := decodePacket(buildV2cTrap(t, "c", "1.3.6.1.6.3.1.1.5.1"))
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	if pkt.Version != gosnmp.Version2c {
		t.Errorf("expected v2c, got %s", pkt.Version)
	}
	vbs, err := packetVarbinds(pkt)
	if err != nil {
		t.Fatalf("unexpected varbind conversion error: %v", err)
	}
	if trapOIDFromVarbinds(vbs) != "1.3.6.1.6.3.1.1.5.1" {
		t.Errorf("expected coldStart OID, got %s", trapOIDFromVarbinds(vbs))
	}
}

func TestV2cLinkDownDecode(t *testing.T) {
	pdu, err := DecodeTrap(buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.3"), net.ParseIP("10.1.2.3"))
	if err != nil {
		t.Fatalf("DecodeTrap failed: %v", err)
	}
	if pdu.OID != "1.3.6.1.6.3.1.1.5.3" {
		t.Errorf("expected linkDown OID, got %s", pdu.OID)
	}
	if pdu.PduType != PduTypeTrap {
		t.Errorf("expected trap PDU type, got %s", pdu.PduType)
	}
}

func TestInformDecode(t *testing.T) {
	pdu, err := DecodeTrap(buildV2cPDU(t, gosnmp.InformRequest, "public", "1.3.6.1.6.3.1.1.5.1"), net.ParseIP("10.1.2.3"))
	if err != nil {
		t.Fatalf("DecodeTrap failed: %v", err)
	}
	if pdu.PduType != PduTypeInform {
		t.Errorf("expected inform PDU type, got %s", pdu.PduType)
	}
}

func TestDecodeOversized(t *testing.T) {
	data := make([]byte, maxDatagramSize+1)
	data[0] = 0x30
	if _, err := decodePacket(data); err == nil {
		t.Fatal("expected error for oversized datagram")
	}
}

func TestDecodeMalformed(t *testing.T) {
	data := []byte{0x30, 0x01, 0x02}
	if _, err := decodePacket(data); err == nil {
		t.Fatal("expected error for truncated packet")
	}
}

func TestDecodeWithBudgetPreservesDecodeError(t *testing.T) {
	data := []byte{0x30, 0x01, 0x02}
	_, err := decodeWithBudget(data, 0*time.Nanosecond)
	if err == nil {
		t.Fatal("expected decode error")
	}
	if errors.Is(err, errDecodeBudgetExceeded) {
		t.Fatalf("expected decode error before budget error, got %v", err)
	}
}

func TestDecodeWithBudgetReturnsBudgetOnSlowSuccess(t *testing.T) {
	data := buildV2cTrap(t, "c", "1.3.6.1.6.3.1.1.5.1")
	// Negative budget forces the post-success budget branch without depending
	// on wall-clock slowness.
	_, err := decodeWithBudget(data, -1*time.Nanosecond)
	if !errors.Is(err, errDecodeBudgetExceeded) {
		t.Fatalf("expected budget error after successful decode, got %v", err)
	}
}

func TestDecodeInvalidVersion(t *testing.T) {
	data := []byte{0x30, 0x06, 0x02, 0x01, 0x99, 0x04, 0x01, 'c'}
	if _, err := decodePacket(data); err == nil {
		t.Fatal("expected error for invalid version")
	}
}

func TestDecodeRejectsSNMPv2uVersion(t *testing.T) {
	data := []byte{0x30, 0x06, 0x02, 0x01, 0x02, 0x04, 0x01, 'c'}
	if _, err := decodePacket(data); err == nil {
		t.Fatal("expected error for unsupported version 2")
	}
}

func TestDecodeRejectsSNMPv3InM2(t *testing.T) {
	data := []byte{0x30, 0x06, 0x02, 0x01, 0x03, 0x04, 0x01, 'c'}
	_, err := decodePacket(data)
	if err == nil {
		t.Fatal("expected error for SNMPv3")
	}
	if got := err.Error(); got != "SNMPv3 not supported in M2" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestDecodeTrapRejectsSNMPv3InM2(t *testing.T) {
	data := []byte{0x30, 0x06, 0x02, 0x01, 0x03, 0x04, 0x01, 'c'}
	_, err := DecodeTrap(data, net.ParseIP("10.1.2.3"))
	if err == nil {
		t.Fatal("expected error for SNMPv3")
	}
	if got := err.Error(); got != "SNMPv3 not supported in M2" {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestDecodeRejectsOctetStringOverLimit(t *testing.T) {
	longValue := make([]byte, maxOctetStringLen+1)
	data := buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.1", gosnmp.SnmpPDU{
		Name:  "1.3.6.1.2.1.1.1.0",
		Type:  gosnmp.OctetString,
		Value: longValue,
	})
	if _, err := decodePacket(data); err == nil {
		t.Fatal("expected error for oversized OctetString")
	}
}

func TestNormalizePDUValueRejectsUnexpectedType(t *testing.T) {
	_, err := normalizePDUValue(gosnmp.SnmpPDU{
		Name:  "1.3.6.1.4.1.9.1",
		Type:  gosnmp.Opaque,
		Value: struct{ Unsupported bool }{Unsupported: true},
	})
	if err == nil {
		t.Fatal("expected unsupported value type error")
	}
	if !strings.Contains(err.Error(), "unsupported PDU value type") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizePDUValueSupportsOpaqueFloats(t *testing.T) {
	tests := map[string]struct {
		pdu  gosnmp.SnmpPDU
		want float64
	}{
		"float32": {
			pdu:  gosnmp.SnmpPDU{Name: "1.3.6.1.4.1.9.1", Type: gosnmp.OpaqueFloat, Value: float32(1.25)},
			want: 1.25,
		},
		"float64": {
			pdu:  gosnmp.SnmpPDU{Name: "1.3.6.1.4.1.9.2", Type: gosnmp.OpaqueDouble, Value: float64(2.5)},
			want: 2.5,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := normalizePDUValue(tc.pdu)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestDecodeRejectsBERLimits(t *testing.T) {
	tests := map[string]struct {
		data []byte
	}{
		"depth over limit": {
			data: nestedSequence(maxNestingDepth + 1),
		},
		"OID encoded length over limit": {
			data: berTLV(tagSequence, berTLV(tagOID, make([]byte, maxOIDEncodedLen+1))),
		},
		"trailing data": {
			data: append(buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.1"), 0x00),
		},
		"indefinite length": {
			data: []byte{tagSequence, 0x80, 0x00, 0x00},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodePacket(tc.data); err == nil {
				t.Fatal("expected BER limit error")
			}
		})
	}
}

func TestValidateBERLimitsAcceptsMaxDepth(t *testing.T) {
	if err := validateBERLimits(nestedSequence(maxNestingDepth)); err != nil {
		t.Fatalf("expected max depth to pass, got %v", err)
	}
}

func TestSourceFromVarbind(t *testing.T) {
	vbs := []VarbindValue{
		{OID: snmpTrapAddressOID, Value: "10.0.0.1", Type: "IPAddress"},
	}
	addr := sourceFromVarbind(vbs, snmpTrapAddressOID)
	if addr != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %q", addr)
	}

	addr = sourceFromVarbind(vbs, "1.3.6.1.4.9.0")
	if addr != "" {
		t.Errorf("expected empty for no-match, got %q", addr)
	}
}

func TestSourceFromVarbindNotString(t *testing.T) {
	vbs := []VarbindValue{
		{OID: snmpTrapAddressOID, Value: int64(42), Type: "INTEGER"},
	}
	addr := sourceFromVarbind(vbs, snmpTrapAddressOID)
	if addr != "" {
		t.Errorf("expected empty for non-IP value, got %q", addr)
	}
}

func TestSourceFromVarbindNetIP(t *testing.T) {
	vbs := []VarbindValue{
		{OID: snmpTrapAddressOID, Value: net.ParseIP("192.0.2.1"), Type: "IPAddress"},
	}
	addr := sourceFromVarbind(vbs, snmpTrapAddressOID)
	if addr != "192.0.2.1" {
		t.Errorf("expected 192.0.2.1, got %q", addr)
	}
}

func TestSourceFromVarbindNotIP(t *testing.T) {
	vbs := []VarbindValue{
		{OID: snmpTrapAddressOID, Value: "not-an-ip", Type: "OctetString"},
	}
	addr := sourceFromVarbind(vbs, snmpTrapAddressOID)
	if addr != "" {
		t.Errorf("expected empty for non-IP value, got %q", addr)
	}
}

func TestIdentifySourceCascade(t *testing.T) {
	peer := net.ParseIP("10.0.0.5")

	vbsWithSource := []VarbindValue{
		{OID: snmpTrapAddressOID, Value: "192.168.1.1", Type: "IPAddress"},
	}
	addr := identifySource(vbsWithSource, peer)
	if addr != "192.168.1.1" {
		t.Errorf("expected snmpTrapAddress.0 value, got %q", addr)
	}

	vbsNoSource := []VarbindValue{
		{OID: sysUpTimeOID, Value: uint64(10), Type: "TimeTicks"},
	}
	addr = identifySource(vbsNoSource, peer)
	if addr != "10.0.0.5" {
		t.Errorf("expected UDP peer fallback, got %q", addr)
	}

	addr = identifySource(vbsNoSource, nil)
	if addr != "" {
		t.Errorf("expected empty for nil peer, got %q", addr)
	}
}

func TestV1TrapOID(t *testing.T) {
	if got := v1TrapOID("1.3.6.1.2.1", 0, 0); got != "1.3.6.1.6.3.1.1.5.1" {
		t.Errorf("unexpected coldStart OID: %s", got)
	}

	if got := v1TrapOID("1.3.6.1.4.1.9", 6, 42); got != "1.3.6.1.4.1.9.0.42" {
		t.Errorf("unexpected enterprise OID: %s", got)
	}

	if got := v1TrapOID("1.3.6.1.4.1.9", 6, -1); got != "" {
		t.Errorf("expected empty OID for negative specificTrap, got %s", got)
	}

	if strconv.IntSize > 32 {
		specificTrap := int(int64(maxSNMPv1SpecificTrap) + 1)
		if got := v1TrapOID("1.3.6.1.4.1.9", 6, specificTrap); got != "" {
			t.Errorf("expected empty OID for out-of-range specificTrap, got %s", got)
		}
	}
}

func TestV1DecodeRejectsInvalidGenericTrap(t *testing.T) {
	tests := map[string]int{
		"negative": -1,
		"seven":    7,
		"eight":    8,
		"large":    255,
	}

	for name, genericTrap := range tests {
		t.Run(name, func(t *testing.T) {
			data := buildV1Trap(t, "public", "192.0.2.10", genericTrap, 0)
			_, err := DecodeTrap(data, net.ParseIP("10.1.2.3"))
			if err == nil {
				t.Fatal("expected error for invalid generic trap")
			}
			if !strings.Contains(err.Error(), "invalid SNMPv1 generic trap value") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestV1DecodeRejectsInvalidEnterpriseSpecificTrap(t *testing.T) {
	data := buildV1Trap(t, "public", "192.0.2.10", 6, -1)
	_, err := DecodeTrap(data, net.ParseIP("10.1.2.3"))
	if err == nil {
		t.Fatal("expected error for invalid enterprise-specific trap")
	}
	if !strings.Contains(err.Error(), "specificTrap -1 out of range") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV1DecodeConvertsAgentAddressAndSyntheticVarbinds(t *testing.T) {
	data := buildV1Trap(t, "public", "192.0.2.10", 6, 42)

	pdu, err := DecodeTrap(data, net.ParseIP("10.1.2.3"))
	if err != nil {
		t.Fatalf("DecodeTrap failed: %v", err)
	}
	if pdu.OID != "1.3.6.1.4.1.9.0.42" {
		t.Errorf("trap OID mismatch: %s", pdu.OID)
	}
	if pdu.SourceIP != "192.0.2.10" {
		t.Errorf("source IP mismatch: %s", pdu.SourceIP)
	}
	if len(pdu.Varbinds) < 5 {
		t.Fatalf("expected synthetic varbinds, got %d", len(pdu.Varbinds))
	}
	if pdu.Varbinds[2].OID != snmpTrapAddressOID || pdu.Varbinds[2].Value != "192.0.2.10" {
		t.Errorf("snmpTrapAddress.0 mismatch: %+v", pdu.Varbinds[2])
	}
}

func TestDecodeTrapIntegration(t *testing.T) {
	data := buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.1")

	pdu, err := DecodeTrap(data, net.ParseIP("10.1.2.3"))
	if err != nil {
		t.Fatalf("DecodeTrap failed: %v", err)
	}
	if pdu.OID != "1.3.6.1.6.3.1.1.5.1" {
		t.Errorf("OID mismatch: %s", pdu.OID)
	}
	if pdu.PeerIP != "10.1.2.3" {
		t.Errorf("PeerIP mismatch: %s", pdu.PeerIP)
	}
	if pdu.Version != SnmpVersionV2c {
		t.Errorf("Version mismatch: %s", pdu.Version)
	}
}

func TestDecodeTrapNilPeer(t *testing.T) {
	data := buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.1")

	pdu, err := DecodeTrap(data, nil)
	if err != nil {
		t.Fatalf("DecodeTrap failed: %v", err)
	}
	if pdu.PeerIP != "" {
		t.Errorf("expected empty PeerIP, got %q", pdu.PeerIP)
	}
	if pdu.SourceIP != "" {
		t.Errorf("expected empty SourceIP without snmpTrapAddress.0 or peer, got %q", pdu.SourceIP)
	}
}

func nestedSequence(levels int) []byte {
	if levels <= 0 {
		return nil
	}
	data := berTLV(tagSequence, nil)
	for i := 1; i < levels; i++ {
		data = berTLV(tagSequence, data)
	}
	return data
}

func berTLV(tag byte, content []byte) []byte {
	out := []byte{tag}
	length := len(content)
	switch {
	case length < 0x80:
		out = append(out, byte(length))
	case length <= 0xff:
		out = append(out, 0x81, byte(length))
	default:
		out = append(out, 0x82, byte(length>>8), byte(length))
	}
	return append(out, content...)
}
