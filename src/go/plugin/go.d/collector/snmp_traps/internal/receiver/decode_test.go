// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/traptest"
)

const testEngineIDHex = "80001f888077dfe44faa700258"

// newTestV3Receiver is the receiver of a static job whose whitelist holds the
// users' engine IDs.
func newTestV3Receiver(t *testing.T, users ...USMUser) *gosnmp.Receiver {
	t.Helper()
	var engineIDs []string
	for _, u := range users {
		engineIDs = append(engineIDs, u.EngineID)
	}
	whitelist, err := buildEngineIDWhitelist(engineIDs)
	if err != nil {
		t.Fatalf("buildEngineIDWhitelist failed: %v", err)
	}
	registered, _, err := receiverUsers(users, whitelist, nil, false)
	if err != nil {
		t.Fatalf("receiverUsers failed: %v", err)
	}
	receiver, err := gosnmp.NewReceiver(gosnmp.ReceiverConfig{Users: registered})
	if err != nil {
		t.Fatalf("NewReceiver failed: %v", err)
	}
	return receiver
}

// newPlainReceiver decodes SNMPv1 and SNMPv2c messages.
func newPlainReceiver(t testing.TB) *gosnmp.Receiver {
	t.Helper()
	receiver, err := gosnmp.NewReceiver(gosnmp.ReceiverConfig{})
	if err != nil {
		t.Fatalf("NewReceiver failed: %v", err)
	}
	return receiver
}

func buildV3TrapWithEngineID(t *testing.T, user, engineIDHex, trapOID string, extra ...gosnmp.VarBind) []byte {
	return traptest.BuildV3TrapWithEngineID(t, user, engineIDHex, trapOID, extra...)
}

type v3SecuredTrapSpec struct {
	user        string
	engineIDHex string
	authProto   string
	privProto   string
	authKey     string
	privKey     string
	trapOID     string
	extra       []gosnmp.VarBind
}

func buildV3SecuredTrap(t *testing.T, spec v3SecuredTrapSpec) []byte {
	return traptest.BuildV3SecuredTrap(t, spec.traptest())
}

func buildV3SecuredTrapWithFlags(t *testing.T, spec v3SecuredTrapSpec, flags gosnmp.SecurityLevel) []byte {
	return traptest.BuildV3SecuredTrapWithFlags(t, spec.traptest(), flags)
}

func buildV3SecuredInform(t *testing.T, spec v3SecuredTrapSpec) []byte {
	return traptest.BuildV3SecuredInform(t, spec.traptest())
}

func buildV3SecuredInformWithFlags(t *testing.T, spec v3SecuredTrapSpec, flags gosnmp.SecurityLevel) []byte {
	return traptest.BuildV3SecuredInformWithFlags(t, spec.traptest(), flags)
}

func (s v3SecuredTrapSpec) traptest() traptest.V3Spec {
	return traptest.V3Spec{
		User: s.user, EngineIDHex: s.engineIDHex, AuthProto: s.authProto, PrivProto: s.privProto,
		AuthKey: s.authKey, PrivKey: s.privKey, TrapOID: s.trapOID, Extra: s.extra,
	}
}

func buildV2cTrap(t testing.TB, community, trapOID string, extra ...gosnmp.VarBind) []byte {
	return traptest.BuildV2cTrap(t, community, trapOID, extra...)
}

func buildV2cPDU(t testing.TB, pduType gosnmp.PDUType, community, trapOID string, extra ...gosnmp.VarBind) []byte {
	return traptest.BuildV2cPDU(t, pduType, community, trapOID, extra...)
}

func buildV1Trap(t *testing.T, community, agentAddr string, genericTrap, specificTrap int, extra ...gosnmp.VarBind) []byte {
	return traptest.BuildV1Trap(t, community, agentAddr, genericTrap, specificTrap, extra...)
}

func TestMinimalV2cDecode(t *testing.T) {
	n, _, err := decodePacket(buildV2cTrap(t, "c", "1.3.6.1.6.3.1.1.5.1"), false, newPlainReceiver(t))
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
	pkt := &n.Message
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
	ctx, _, err := decodeTrap(buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.3"), net.ParseIP("10.1.2.3"), newPlainReceiver(t))
	if err != nil {
		t.Fatalf("decodeTrap failed: %v", err)
	}
	if ctx.PDU.OID != "1.3.6.1.6.3.1.1.5.3" {
		t.Errorf("expected linkDown OID, got %s", ctx.PDU.OID)
	}
	if ctx.PDU.PduType != model.PduTypeTrap {
		t.Errorf("expected trap PDU type, got %s", ctx.PDU.PduType)
	}
}

func TestInformDecode(t *testing.T) {
	ctx, _, err := decodeTrap(buildV2cPDU(t, gosnmp.InformRequest, "public", "1.3.6.1.6.3.1.1.5.1"), net.ParseIP("10.1.2.3"), newPlainReceiver(t))
	if err != nil {
		t.Fatalf("decodeTrap failed: %v", err)
	}
	if ctx.PDU.PduType != model.PduTypeInform {
		t.Errorf("expected inform PDU type, got %s", ctx.PDU.PduType)
	}
}

func TestDecodeOversized(t *testing.T) {
	data := make([]byte, maxDatagramSize+1)
	data[0] = 0x30
	if _, _, err := decodePacket(data, false, newPlainReceiver(t)); err == nil {
		t.Fatal("expected error for oversized datagram")
	}
}

func TestDecodeMalformed(t *testing.T) {
	data := []byte{0x30, 0x01, 0x02}
	if _, _, err := decodePacket(data, false, newPlainReceiver(t)); err == nil {
		t.Fatal("expected error for truncated packet")
	}
}

func TestDecodeRejectsOctetStringOverLimit(t *testing.T) {
	longValue := make([]byte, maxOctetStringLen+1)
	data := buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.1", gosnmp.VarBind{
		Name:  "1.3.6.1.2.1.1.1.0",
		Value: gosnmp.OctetStringValue([]byte(longValue)),
	})
	if _, _, err := decodePacket(data, false, newPlainReceiver(t)); err == nil {
		t.Fatal("expected error for oversized OctetString")
	}
}

func TestDecodeV3AuthPrivAllowsEncryptedScopedPDUOverOctetStringLimit(t *testing.T) {
	var extra []gosnmp.VarBind
	for i := range 40 {
		extra = append(extra, gosnmp.VarBind{
			Name:  "1.3.6.1.4.1.999.1." + strconv.Itoa(i+1),
			Value: gosnmp.OctetStringValue([]byte(strings.Repeat("x", 48))),
		})
	}
	data := buildV3SecuredTrap(t, v3SecuredTrapSpec{
		user:        "testuser",
		engineIDHex: testEngineIDHex,
		authProto:   "sha256",
		privProto:   "aes",
		authKey:     "authpassword",
		privKey:     "privpassword",
		trapOID:     "1.3.6.1.6.3.1.1.5.1",
		extra:       extra,
	})
	if len(data) <= maxOctetStringLen {
		t.Fatalf("test packet length = %d, want encrypted ScopedPDU coverage over %d", len(data), maxOctetStringLen)
	}
	tbl := newTestV3Receiver(t, USMUser{
		Username:  "testuser",
		EngineID:  testEngineIDHex,
		AuthProto: "sha256",
		AuthKey:   "authpassword",
		PrivProto: "aes",
		PrivKey:   "privpassword",
	})

	ctx, _, err := decodeTrap(data, net.ParseIP("10.1.2.3"), tbl)
	if err != nil {
		t.Fatalf("decodeTrap failed for valid authPriv packet: %v", err)
	}
	if got := len(ctx.PDU.Varbinds); got < len(extra)+2 {
		t.Fatalf("decoded varbinds = %d, want at least %d", got, len(extra)+2)
	}
}

func TestNormalizePDUValueSupportsOpaqueFloats(t *testing.T) {
	tests := map[string]struct {
		pdu  gosnmp.VarBind
		want float64
	}{
		"float32": {
			pdu:  gosnmp.VarBind{Name: "1.3.6.1.4.1.9.1", Value: gosnmp.OpaqueFloatValue(1.25)},
			want: 1.25,
		},
		"float64": {
			pdu:  gosnmp.VarBind{Name: "1.3.6.1.4.1.9.2", Value: gosnmp.OpaqueDoubleValue(2.5)},
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
			if _, _, err := decodePacket(tc.data, false, newPlainReceiver(t)); err == nil {
				t.Fatal("expected BER limit error")
			}
		})
	}
}

func TestValidateBERLimitsAcceptsMaxDepth(t *testing.T) {
	if err := validateBERLimits(nestedSequence(maxNestingDepth), validateBEROptions{}); err != nil {
		t.Fatalf("expected max depth to pass, got %v", err)
	}
}

func TestSourceFromVarbind(t *testing.T) {
	vbs := []model.VarbindValue{
		{OID: model.SNMPTrapAddressOID, Value: "10.0.0.1", Type: "IPAddress"},
	}
	addr, _ := sourceFromVarbindWithRejectReason(vbs, model.SNMPTrapAddressOID)
	if addr != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %q", addr)
	}

	addr, _ = sourceFromVarbindWithRejectReason(vbs, "1.3.6.1.4.9.0")
	if addr != "" {
		t.Errorf("expected empty for no-match, got %q", addr)
	}
}

func TestSourceFromVarbindNotString(t *testing.T) {
	vbs := []model.VarbindValue{
		{OID: model.SNMPTrapAddressOID, Value: int64(42), Type: "INTEGER"},
	}
	addr, _ := sourceFromVarbindWithRejectReason(vbs, model.SNMPTrapAddressOID)
	if addr != "" {
		t.Errorf("expected empty for non-IP value, got %q", addr)
	}
}

func TestSourceFromVarbindNetIP(t *testing.T) {
	vbs := []model.VarbindValue{
		{OID: model.SNMPTrapAddressOID, Value: net.ParseIP("192.0.2.1"), Type: "IPAddress"},
	}
	addr, _ := sourceFromVarbindWithRejectReason(vbs, model.SNMPTrapAddressOID)
	if addr != "192.0.2.1" {
		t.Errorf("expected 192.0.2.1, got %q", addr)
	}
}

func TestSourceFromVarbindNotIP(t *testing.T) {
	vbs := []model.VarbindValue{
		{OID: model.SNMPTrapAddressOID, Value: "not-an-ip", Type: "OctetString"},
	}
	addr, _ := sourceFromVarbindWithRejectReason(vbs, model.SNMPTrapAddressOID)
	if addr != "" {
		t.Errorf("expected empty for non-IP value, got %q", addr)
	}
}

func TestIdentifySourceCascade(t *testing.T) {
	peer := net.ParseIP("10.0.0.5")

	vbsWithSource := []model.VarbindValue{
		{OID: model.SNMPTrapAddressOID, Value: "192.168.1.1", Type: "IPAddress"},
	}
	addr := selectTrapSource(vbsWithSource, peer, false).sourceIP
	if addr != "10.0.0.5" {
		t.Errorf("expected UDP peer to win over snmpTrapAddress.0, got %q", addr)
	}

	vbsNoSource := []model.VarbindValue{
		{OID: model.SysUpTimeOID, Value: uint64(10), Type: "TimeTicks"},
	}
	addr = selectTrapSource(vbsNoSource, peer, false).sourceIP
	if addr != "10.0.0.5" {
		t.Errorf("expected UDP peer fallback, got %q", addr)
	}

	addr = selectTrapSource(vbsNoSource, nil, false).sourceIP
	if addr != "" {
		t.Errorf("expected empty for nil peer, got %q", addr)
	}

	addr = selectTrapSource(vbsWithSource, nil, false).sourceIP
	if addr != "" {
		t.Errorf("expected empty without trusted relay and UDP peer, got %q", addr)
	}
	selected := selectTrapSource(vbsWithSource, nil, false)
	if len(selected.audit.RejectedCandidates) != 1 || selected.audit.RejectedCandidates[0] != "snmpTrapAddress.0:missing_udp_peer" {
		t.Fatalf("source audit rejected candidates = %+v, want missing UDP peer rejection", selected.audit.RejectedCandidates)
	}

	vbsUnspecifiedSource := []model.VarbindValue{
		{OID: model.SNMPTrapAddressOID, Value: "0.0.0.0", Type: "IPAddress"},
	}
	addr = selectTrapSource(vbsUnspecifiedSource, peer, false).sourceIP
	if addr != "10.0.0.5" {
		t.Errorf("expected UDP peer when snmpTrapAddress.0 is unspecified, got %q", addr)
	}
	selected = selectTrapSource(vbsUnspecifiedSource, peer, false)
	if selected.audit == nil {
		t.Fatal("missing source audit")
	}
	if selected.audit.Method != "udp_peer" || selected.audit.Selected != "10.0.0.5" {
		t.Fatalf("source audit = %+v, want UDP peer selected", selected.audit)
	}
	if len(selected.audit.RejectedCandidates) != 1 || selected.audit.RejectedCandidates[0] != "snmpTrapAddress.0:unspecified_ip" {
		t.Fatalf("source audit rejected candidates = %+v, want unspecified snmpTrapAddress.0 rejection", selected.audit.RejectedCandidates)
	}

	addr = selectTrapSource(vbsUnspecifiedSource, nil, false).sourceIP
	if addr != "" {
		t.Errorf("expected empty without UDP peer when snmpTrapAddress.0 is unspecified, got %q", addr)
	}
}

func TestSelectTrapSourceTrustedRelay(t *testing.T) {
	peer := net.ParseIP("10.0.0.5")
	vbsWithSource := []model.VarbindValue{
		{OID: model.SNMPTrapAddressOID, Value: "192.168.1.1", Type: "IPAddress"},
	}

	selected := selectTrapSource(vbsWithSource, peer, true)
	if selected.sourceIP != "192.168.1.1" {
		t.Fatalf("sourceIP = %q, want relayed snmpTrapAddress.0", selected.sourceIP)
	}
	if selected.audit == nil || selected.audit.Method != "trusted_relay_snmpTrapAddress.0" || !selected.audit.TrustedRelay {
		t.Fatalf("source audit = %+v, want trusted relay snmpTrapAddress.0", selected.audit)
	}

	selected = selectTrapSource(vbsWithSource, peer, false)
	if selected.sourceIP != "10.0.0.5" {
		t.Fatalf("sourceIP = %q, want untrusted UDP peer", selected.sourceIP)
	}
	if len(selected.audit.RejectedCandidates) != 1 || selected.audit.RejectedCandidates[0] != "snmpTrapAddress.0:untrusted_relay_uses_udp_peer" {
		t.Fatalf("source audit rejected candidates = %+v, want untrusted relay rejection", selected.audit.RejectedCandidates)
	}

	selected = selectTrapSource(vbsWithSource, nil, true)
	if selected.sourceIP != "192.168.1.1" {
		t.Fatalf("sourceIP = %q, want relayed snmpTrapAddress.0 when caller marks peer as trusted", selected.sourceIP)
	}

	vbsNoSource := []model.VarbindValue{{OID: model.SysUpTimeOID, Value: uint64(10), Type: "TimeTicks"}}
	selected = selectTrapSource(vbsNoSource, peer, true)
	if selected.sourceIP != "10.0.0.5" {
		t.Fatalf("sourceIP = %q, want UDP peer when trusted relay has no snmpTrapAddress.0", selected.sourceIP)
	}

	vbsUnspecifiedSource := []model.VarbindValue{
		{OID: model.SNMPTrapAddressOID, Value: "0.0.0.0", Type: "IPAddress"},
	}
	selected = selectTrapSource(vbsUnspecifiedSource, peer, true)
	if selected.sourceIP != "10.0.0.5" {
		t.Fatalf("sourceIP = %q, want UDP peer when trusted relay snmpTrapAddress.0 is unspecified", selected.sourceIP)
	}
	if len(selected.audit.RejectedCandidates) != 1 || selected.audit.RejectedCandidates[0] != "snmpTrapAddress.0:unspecified_ip" {
		t.Fatalf("source audit rejected candidates = %+v, want unspecified snmpTrapAddress.0 rejection", selected.audit.RejectedCandidates)
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
		specificTrap64 := int64(maxSNMPv1SpecificTrap)
		specificTrap := int(specificTrap64 + 1)
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
			_, _, err := decodeTrap(data, net.ParseIP("10.1.2.3"), newPlainReceiver(t))
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
	_, _, err := decodeTrap(data, net.ParseIP("10.1.2.3"), newPlainReceiver(t))
	if err == nil {
		t.Fatal("expected error for invalid enterprise-specific trap")
	}
	if !strings.Contains(err.Error(), "specificTrap -1 out of range") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestV1DecodeConvertsAgentAddressAndSyntheticVarbinds(t *testing.T) {
	data := buildV1Trap(t, "public", "192.0.2.10", 6, 42)

	ctx, _, err := decodeTrap(data, net.ParseIP("10.1.2.3"), newPlainReceiver(t))
	if err != nil {
		t.Fatalf("decodeTrap failed: %v", err)
	}
	pdu := ctx.PDU
	if pdu.OID != "1.3.6.1.4.1.9.0.42" {
		t.Errorf("trap OID mismatch: %s", pdu.OID)
	}
	if pdu.SourceIP != "10.1.2.3" {
		t.Errorf("source IP mismatch: %s", pdu.SourceIP)
	}
	if len(pdu.Varbinds) < 5 {
		t.Fatalf("expected synthetic varbinds, got %d", len(pdu.Varbinds))
	}
	if pdu.Varbinds[2].OID != model.SNMPTrapAddressOID || pdu.Varbinds[2].Value != "192.0.2.10" {
		t.Errorf("snmpTrapAddress.0 mismatch: %+v", pdu.Varbinds[2])
	}
}

func TestDecodeTrapIntegration(t *testing.T) {
	data := buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.1")

	ctx, _, err := decodeTrap(data, net.ParseIP("10.1.2.3"), newPlainReceiver(t))
	if err != nil {
		t.Fatalf("decodeTrap failed: %v", err)
	}
	pdu := ctx.PDU
	if pdu.OID != "1.3.6.1.6.3.1.1.5.1" {
		t.Errorf("OID mismatch: %s", pdu.OID)
	}
	if pdu.PeerIP != "10.1.2.3" {
		t.Errorf("PeerIP mismatch: %s", pdu.PeerIP)
	}
	if pdu.Version != model.SnmpVersionV2c {
		t.Errorf("Version mismatch: %s", pdu.Version)
	}
}

func TestDecodeTrapNilPeer(t *testing.T) {
	data := buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.1")

	ctx, _, err := decodeTrap(data, nil, newPlainReceiver(t))
	if err != nil {
		t.Fatalf("decodeTrap failed: %v", err)
	}
	pdu := ctx.PDU
	if pdu.PeerIP != "" {
		t.Errorf("expected empty PeerIP, got %q", pdu.PeerIP)
	}
	if pdu.SourceIP != "" {
		t.Errorf("expected empty SourceIP without snmpTrapAddress.0 or peer, got %q", pdu.SourceIP)
	}
}

func TestDecodePacketSucceeds(t *testing.T) {
	data := buildV2cTrap(t, "c", "1.3.6.1.6.3.1.1.5.1")
	_, _, err := decodePacket(data, false, newPlainReceiver(t))
	if err != nil {
		t.Fatalf("unexpected decode error: %v", err)
	}
}

func TestDecodeTrapV2cWithV3Receiver(t *testing.T) {
	data := buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.1")
	tbl := newTestV3Receiver(t, USMUser{Username: "testuser", EngineID: testEngineIDHex})

	ctx, _, err := decodeTrap(data, net.ParseIP("10.1.2.3"), tbl)
	if err != nil {
		t.Fatalf("decodeTrap failed: %v", err)
	}
	if ctx.PDU.Version != model.SnmpVersionV2c {
		t.Fatalf("version = %s, want v2c", ctx.PDU.Version)
	}
}

func TestV3DecodeNoAuth(t *testing.T) {
	data := buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	tbl := newTestV3Receiver(t, USMUser{Username: "testuser", EngineID: testEngineIDHex})

	ctx, _, err := decodeTrap(data, net.ParseIP("10.1.2.3"), tbl)
	if err != nil {
		t.Fatalf("v3 decode failed: %v", err)
	}
	if ctx.PDU.Version != model.SnmpVersionV3 {
		t.Errorf("expected v3, got %s", ctx.PDU.Version)
	}
}

func TestV3DecodeAuthProtocols(t *testing.T) {
	tests := map[string]string{
		"sha224": "sha224",
		"sha256": "sha256",
		"sha384": "sha384",
		"sha512": "sha512",
	}

	for name, authProto := range tests {
		t.Run(name, func(t *testing.T) {
			data := buildV3SecuredTrap(t, v3SecuredTrapSpec{
				user:        "testuser",
				engineIDHex: testEngineIDHex,
				authProto:   authProto,
				privProto:   "none",
				authKey:     "authpassword",
				trapOID:     "1.3.6.1.6.3.1.1.5.1",
			})
			tbl := newTestV3Receiver(t, USMUser{
				Username:  "testuser",
				EngineID:  testEngineIDHex,
				AuthProto: authProto,
				AuthKey:   "authpassword",
				PrivProto: "none",
			})

			ctx, _, err := decodeTrap(data, net.ParseIP("10.1.2.3"), tbl)
			if err != nil {
				t.Fatalf("decodeTrap failed: %v", err)
			}
			if ctx.PDU.Version != model.SnmpVersionV3 {
				t.Fatalf("version = %s, want v3", ctx.PDU.Version)
			}
		})
	}
}

func TestV3DecodePrivacyProtocols(t *testing.T) {
	tests := map[string]string{
		"aes":    "aes",
		"aes192": "aes192",
		"aes256": "aes256",
	}

	for name, privProto := range tests {
		t.Run(name, func(t *testing.T) {
			data := buildV3SecuredTrap(t, v3SecuredTrapSpec{
				user:        "testuser",
				engineIDHex: testEngineIDHex,
				authProto:   "sha256",
				privProto:   privProto,
				authKey:     "authpassword",
				privKey:     "privpassword",
				trapOID:     "1.3.6.1.6.3.1.1.5.1",
			})
			tbl := newTestV3Receiver(t, USMUser{
				Username:  "testuser",
				EngineID:  testEngineIDHex,
				AuthProto: "sha256",
				AuthKey:   "authpassword",
				PrivProto: privProto,
				PrivKey:   "privpassword",
			})

			ctx, _, err := decodeTrap(data, net.ParseIP("10.1.2.3"), tbl)
			if err != nil {
				t.Fatalf("decodeTrap failed: %v", err)
			}
			if ctx.PDU.Version != model.SnmpVersionV3 {
				t.Fatalf("version = %s, want v3", ctx.PDU.Version)
			}
		})
	}
}

func TestV3DecodeWrongUser(t *testing.T) {
	data := buildV3TrapWithEngineID(t, "testuser", testEngineIDHex, "1.3.6.1.6.3.1.1.5.1")
	tbl := newTestV3Receiver(t, USMUser{Username: "otheruser", EngineID: testEngineIDHex})

	_, _, err := decodeTrap(data, net.ParseIP("10.1.2.3"), tbl)
	if err == nil {
		t.Fatal("expected error for wrong user")
	}
	dim := classifyDecodeError(err)
	if dim != "usm_failures" {
		t.Errorf("expected usm_failures, got %s", dim)
	}
}

func TestClassifyDecodeError(t *testing.T) {
	wrap := func(category error) error { return fmt.Errorf("decode: %w", category) }
	tests := map[string]struct {
		err  error
		want ErrorKind
	}{
		"none":                    {},
		"ASN.1 parse":             {err: wrap(gosnmp.ErrASNParse), want: "malformed_pdu"},
		"bad version":             {err: wrap(gosnmp.ErrBadVersion), want: "malformed_pdu"},
		"invalid messages":        {err: wrap(gosnmp.ErrInvalidMsgs), want: "malformed_pdu"},
		"wrong digest":            {err: wrap(gosnmp.ErrWrongDigest), want: "auth_failures"},
		"decryption":              {err: wrap(gosnmp.ErrDecryption), want: "auth_failures"},
		"unknown username":        {err: wrap(gosnmp.ErrUnknownUsername), want: "usm_failures"},
		"unknown security level":  {err: wrap(gosnmp.ErrUnknownSecurityLevel), want: "usm_failures"},
		"unknown security models": {err: wrap(gosnmp.ErrUnknownSecurityModels), want: "usm_failures"},
		"not in time window":      {err: wrap(gosnmp.ErrNotInTimeWindow), want: "usm_failures"},
		"unknown engine ID":       {err: wrap(gosnmp.ErrUnknownEngineID), want: "unknown_engine_id"},
		"BER budget":              {err: errors.New("BER: trailing data"), want: "malformed_pdu"},
		"datagram too large":      {err: errors.New("datagram too large: 2 > 1"), want: "malformed_pdu"},
		"too many varbinds":       {err: errors.New("too many varbinds: 2 > 1"), want: "malformed_pdu"},
		"long OctetString":        {err: errors.New("OctetString too long: 2 > 1"), want: "malformed_pdu"},
		"missing trap OID":        {err: errors.New("missing snmpTrapOID.0"), want: "malformed_pdu"},
		"invalid SNMPv1 trap":     {err: errors.New("invalid SNMPv1 generic trap value: 7"), want: "malformed_pdu"},
		"other":                   {err: errors.New("some other error"), want: "decode_failed"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := classifyDecodeError(tc.err)
			if got != tc.want {
				t.Errorf("expected %s, got %s", tc.want, got)
			}
		})
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

// A decode failure carries what the unauthenticated header says: the version
// and the engine ID of a header that parses, nothing of one that does not.
func TestDecodeFailureEnvelope(t *testing.T) {
	wrongKey := buildV3SecuredTrap(t, v3SecuredTrapSpec{
		user:        "testuser",
		engineIDHex: testEngineIDHex,
		authProto:   "sha256",
		privProto:   "aes",
		authKey:     "wrongpassword",
		privKey:     "privpassword",
		trapOID:     "1.3.6.1.6.3.1.1.5.1",
	})
	v2c := buildV2cTrap(t, "public", "1.3.6.1.6.3.1.1.5.1")
	badCommunity := append([]byte(nil), v2c...)
	badCommunity[berChildren(t, badCommunity, berElementAt(t, badCommunity, 0))[1].start] = tagInteger

	type envelope struct {
		kind         ErrorKind
		versionKnown bool
		version      model.SnmpVersion
		engineID     string
	}
	tests := map[string]struct {
		data []byte
		want envelope
	}{
		"SNMPv3 header that parses": {
			data: wrongKey,
			want: envelope{kind: ErrorAuthFailure, versionKnown: true, version: model.SnmpVersionV3, engineID: testEngineIDHex},
		},
		"SNMPv3 header field that does not parse": {
			data: corruptV3HeaderFieldTag(t, wrongKey, 0, tagOctetStr),
			want: envelope{kind: ErrorMalformedPDU},
		},
		"SNMPv2c community that does not parse": {
			data: badCommunity,
			want: envelope{kind: ErrorMalformedPDU},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			recv := New(NewPolicy(PolicyConfig{
				Versions:          []string{"v2c", "v3"},
				USMUsers:          []USMUser{securityLevelAuthUser(testEngineIDHex)},
				EngineIDWhitelist: []string{testEngineIDHex},
			}), nil)
			if err := recv.PrepareV3(t.TempDir(), "decode-failure-test"); err != nil {
				t.Fatalf("prepare v3 receiver: %v", err)
			}
			t.Cleanup(recv.RollbackPreparedState)
			peer := &net.UDPAddr{IP: net.ParseIP("10.1.2.3"), Port: 9162}

			failure := recv.Process(Datagram{Data: tc.data, PeerIP: peer.IP, Peer: peer}).DecodeFailure
			if failure == nil {
				t.Fatal("want a decode failure")
			}
			got := envelope{kind: failure.Kind, versionKnown: failure.VersionKnown, version: failure.SniffedVersion, engineID: failure.EngineID}
			if got != tc.want {
				t.Fatalf("decode failure = %+v, want %+v", got, tc.want)
			}
		})
	}
}
