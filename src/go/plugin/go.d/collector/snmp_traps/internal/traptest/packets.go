// SPDX-License-Identifier: GPL-3.0-or-later

package traptest

import (
	"encoding/hex"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

var logger = gosnmp.NewLogger(log.New(io.Discard, "", 0))

type V3Spec struct {
	User        string
	EngineIDHex string
	AuthProto   string
	PrivProto   string
	AuthKey     string
	PrivKey     string
	TrapOID     string
	Extra       []gosnmp.SnmpPDU
}

func Marshal(t testing.TB, packet *gosnmp.SnmpPacket) []byte {
	t.Helper()
	data, err := packet.MarshalMsg()
	if err != nil {
		t.Fatalf("marshal SNMP test packet: %v", err)
	}
	return data
}

func BuildV2cTrap(t testing.TB, community, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	t.Helper()
	return BuildV2cPDU(t, gosnmp.SNMPv2Trap, community, trapOID, extra...)
}

func BuildV2cPDU(t testing.TB, pduType gosnmp.PDUType, community, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	t.Helper()
	client := &gosnmp.GoSNMP{Version: gosnmp.Version2c, Community: community}
	pdus := []gosnmp.SnmpPDU{
		{Name: model.SysUpTimeOID, Type: gosnmp.TimeTicks, Value: uint32(10)},
		{Name: model.SNMPTrapOID, Type: gosnmp.ObjectIdentifier, Value: trapOID},
	}
	pdus = append(pdus, extra...)
	return Marshal(t, client.MkSnmpPacket(pduType, pdus, 0, 0))
}

func BuildV1Trap(t testing.TB, community, agentAddress string, genericTrap, specificTrap int, extra ...gosnmp.SnmpPDU) []byte {
	t.Helper()
	return Marshal(t, &gosnmp.SnmpPacket{
		Version:   gosnmp.Version1,
		Community: community,
		PDUType:   gosnmp.Trap,
		SnmpTrap: gosnmp.SnmpTrap{
			Enterprise:   "1.3.6.1.4.1.9",
			AgentAddress: agentAddress,
			GenericTrap:  genericTrap,
			SpecificTrap: specificTrap,
			Timestamp:    10,
			Variables:    extra,
		},
	})
}

func BuildV3Trap(t testing.TB, user, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	t.Helper()
	client := &gosnmp.GoSNMP{
		Version:            gosnmp.Version3,
		SecurityModel:      gosnmp.UserSecurityModel,
		MsgFlags:           gosnmp.NoAuthNoPriv,
		SecurityParameters: &gosnmp.UsmSecurityParameters{UserName: user},
		Logger:             logger,
	}
	pdus := []gosnmp.SnmpPDU{
		{Name: model.SysUpTimeOID, Type: gosnmp.TimeTicks, Value: uint32(10)},
		{Name: model.SNMPTrapOID, Type: gosnmp.ObjectIdentifier, Value: trapOID},
	}
	pdus = append(pdus, extra...)
	data, err := client.SnmpEncodePacket(gosnmp.SNMPv2Trap, pdus, 0, 0)
	if err != nil {
		t.Fatalf("marshal v3 test packet: %v", err)
	}
	return data
}

func BuildV3TrapWithEngineID(t testing.TB, user, engineIDHex, trapOID string, extra ...gosnmp.SnmpPDU) []byte {
	t.Helper()
	return BuildV3SecuredTrap(t, V3Spec{
		User: user, EngineIDHex: engineIDHex, AuthProto: "none", PrivProto: "none", TrapOID: trapOID, Extra: extra,
	})
}

func BuildV3SecuredTrap(t testing.TB, spec V3Spec) []byte {
	t.Helper()
	return buildV3SecuredPDU(t, gosnmp.SNMPv2Trap, spec)
}

func BuildV3SecuredInform(t testing.TB, spec V3Spec) []byte {
	t.Helper()
	return buildV3SecuredPDU(t, gosnmp.InformRequest, spec)
}

func BuildV3SecuredTrapWithFlags(t testing.TB, spec V3Spec, flags gosnmp.SnmpV3MsgFlags) []byte {
	t.Helper()
	return buildV3SecuredPDUWithFlags(t, gosnmp.SNMPv2Trap, spec, flags)
}

func BuildV3SecuredInformWithFlags(t testing.TB, spec V3Spec, flags gosnmp.SnmpV3MsgFlags) []byte {
	t.Helper()
	return buildV3SecuredPDUWithFlags(t, gosnmp.InformRequest, spec, flags)
}

func buildV3SecuredPDUWithFlags(t testing.TB, pduType gosnmp.PDUType, spec V3Spec, flags gosnmp.SnmpV3MsgFlags) []byte {
	t.Helper()
	client, pdus, engineID := newV3PacketParts(t, spec)
	client.MsgFlags = flags
	packet := client.MkSnmpPacket(pduType, pdus, 0, 0)
	packet.MsgID = 1
	packet.MsgMaxSize = 65535
	packet.RequestID = 1
	packet.ContextEngineID = string(engineID)
	packet.Logger = logger
	return Marshal(t, packet)
}

func buildV3SecuredPDU(t testing.TB, pduType gosnmp.PDUType, spec V3Spec) []byte {
	t.Helper()
	client, pdus, _ := newV3PacketParts(t, spec)
	data, err := client.SnmpEncodePacket(pduType, pdus, 0, 0)
	if err != nil {
		t.Fatalf("marshal v3 %s test packet: %v", pduType, err)
	}
	return data
}

func newV3PacketParts(t testing.TB, spec V3Spec) (*gosnmp.GoSNMP, []gosnmp.SnmpPDU, []byte) {
	t.Helper()
	engineID, err := hex.DecodeString(spec.EngineIDHex)
	if err != nil {
		t.Fatalf("invalid test engine ID: %v", err)
	}
	authProto := snmputils.ParseSNMPv3AuthProtocol(strings.ToLower(spec.AuthProto))
	privProto := snmputils.ParseSNMPv3PrivProtocol(strings.ToLower(spec.PrivProto))
	security := &gosnmp.UsmSecurityParameters{
		UserName:                 spec.User,
		AuthenticationProtocol:   authProto,
		AuthenticationPassphrase: spec.AuthKey,
		PrivacyProtocol:          privProto,
		PrivacyPassphrase:        spec.PrivKey,
		AuthoritativeEngineID:    string(engineID),
		AuthoritativeEngineBoots: 1,
		AuthoritativeEngineTime:  1,
	}
	if err := security.InitSecurityKeys(); err != nil {
		t.Fatalf("initialize v3 test security keys: %v", err)
	}
	client := &gosnmp.GoSNMP{
		Version:            gosnmp.Version3,
		SecurityModel:      gosnmp.UserSecurityModel,
		MsgFlags:           securityLevel(authProto, privProto),
		SecurityParameters: security,
		Logger:             logger,
	}
	pdus := []gosnmp.SnmpPDU{
		{Name: model.SysUpTimeOID, Type: gosnmp.TimeTicks, Value: uint32(10)},
		{Name: model.SNMPTrapOID, Type: gosnmp.ObjectIdentifier, Value: spec.TrapOID},
	}
	pdus = append(pdus, spec.Extra...)
	return client, pdus, engineID
}

func securityLevel(authProto gosnmp.SnmpV3AuthProtocol, privProto gosnmp.SnmpV3PrivProtocol) gosnmp.SnmpV3MsgFlags {
	switch {
	case authProto == gosnmp.NoAuth:
		return gosnmp.NoAuthNoPriv
	case privProto == gosnmp.NoPriv:
		return gosnmp.AuthNoPriv
	default:
		return gosnmp.AuthPriv
	}
}
