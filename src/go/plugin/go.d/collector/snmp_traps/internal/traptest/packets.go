// SPDX-License-Identifier: GPL-3.0-or-later

package traptest

import (
	"encoding/hex"
	"math/rand/v2"
	"net/netip"
	"strings"
	"testing"

	"github.com/netdata/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

// V3Spec describes an SNMPv3 test message: its user and the engine its keys
// are localized to. EngineBoots and EngineTime default to 1.
type V3Spec struct {
	User        string
	EngineIDHex string
	AuthProto   string
	PrivProto   string
	AuthKey     string
	PrivKey     string
	TrapOID     string
	Extra       []gosnmp.VarBind
	EngineBoots uint32
	EngineTime  uint32
}

// Marshal encodes an SNMPv1 or SNMPv2c test message.
func Marshal(t testing.TB, packet *gosnmp.Message) []byte {
	t.Helper()
	data, err := packet.Encode(nil)
	if err != nil {
		t.Fatalf("marshal SNMP test packet: %v", err)
	}
	return data
}

func BuildV2cTrap(t testing.TB, community, trapOID string, extra ...gosnmp.VarBind) []byte {
	t.Helper()
	return BuildV2cPDU(t, gosnmp.SNMPv2Trap, community, trapOID, extra...)
}

func BuildV2cPDU(t testing.TB, pduType gosnmp.PDUType, community, trapOID string, extra ...gosnmp.VarBind) []byte {
	t.Helper()
	return Marshal(t, &gosnmp.Message{
		Version:   gosnmp.Version2c,
		Community: community,
		PDUType:   pduType,
		VarBinds:  trapVarBinds(trapOID, extra),
	})
}

// BuildV1Trap builds an SNMPv1 trap; agentAddress is an IPv4 address.
func BuildV1Trap(t testing.TB, community, agentAddress string, genericTrap, specificTrap int, extra ...gosnmp.VarBind) []byte {
	t.Helper()
	addr, err := netip.ParseAddr(agentAddress)
	if err != nil {
		t.Fatalf("invalid test agent address %q: %v", agentAddress, err)
	}
	return Marshal(t, &gosnmp.Message{
		Version:   gosnmp.Version1,
		Community: community,
		PDUType:   gosnmp.Trap,
		VarBinds:  extra,
		TrapV1Header: gosnmp.TrapV1Header{
			Enterprise:   "1.3.6.1.4.1.9",
			AgentAddress: addr,
			GenericTrap:  int32(genericTrap),
			SpecificTrap: int32(specificTrap),
			Timestamp:    10,
		},
	})
}

// BuildV3Trap builds an unauthenticated SNMPv3 trap that names no engine ID.
func BuildV3Trap(t testing.TB, user, trapOID string, extra ...gosnmp.VarBind) []byte {
	t.Helper()
	data, err := (&gosnmp.Message{
		Version:            gosnmp.Version3,
		SecurityLevel:      gosnmp.NoAuthNoPriv,
		SecurityParameters: gosnmp.SecurityParameters{UserName: user},
		PDUType:            gosnmp.SNMPv2Trap,
		MsgID:              rand.Uint32N(1 << 31),
		RequestID:          rand.Uint32(),
		VarBinds:           trapVarBinds(trapOID, extra),
	}).Encode(nil)
	if err != nil {
		t.Fatalf("marshal v3 test packet: %v", err)
	}
	return data
}

func BuildV3TrapWithEngineID(t testing.TB, user, engineIDHex, trapOID string, extra ...gosnmp.VarBind) []byte {
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

func BuildV3SecuredTrapWithFlags(t testing.TB, spec V3Spec, flags gosnmp.SecurityLevel) []byte {
	t.Helper()
	return buildV3SecuredPDUWithFlags(t, gosnmp.SNMPv2Trap, spec, flags)
}

func BuildV3SecuredInformWithFlags(t testing.TB, spec V3Spec, flags gosnmp.SecurityLevel) []byte {
	t.Helper()
	return buildV3SecuredPDUWithFlags(t, gosnmp.InformRequest, spec, flags)
}

// buildV3SecuredPDUWithFlags builds the message at the security level flags,
// whatever the user's protocols allow, with fixed msgID and request ID 1.
func buildV3SecuredPDUWithFlags(t testing.TB, pduType gosnmp.PDUType, spec V3Spec, flags gosnmp.SecurityLevel) []byte {
	t.Helper()
	msg, user := newV3Message(t, pduType, spec)
	msg.SecurityLevel = flags
	msg.MsgID, msg.RequestID, msg.MsgMaxSize = 1, 1, 65535
	msg.ContextEngineID = msg.SecurityParameters.AuthoritativeEngineID
	return encodeV3(t, msg, user)
}

func buildV3SecuredPDU(t testing.TB, pduType gosnmp.PDUType, spec V3Spec) []byte {
	t.Helper()
	msg, user := newV3Message(t, pduType, spec)
	msg.MsgID, msg.RequestID = rand.Uint32N(1<<31), rand.Uint32()
	return encodeV3(t, msg, user)
}

func newV3Message(t testing.TB, pduType gosnmp.PDUType, spec V3Spec) (*gosnmp.Message, *gosnmp.User) {
	t.Helper()
	engineID, err := hex.DecodeString(spec.EngineIDHex)
	if err != nil {
		t.Fatalf("invalid test engine ID: %v", err)
	}
	user := &gosnmp.User{
		Name:           spec.User,
		AuthProtocol:   snmputils.ParseSNMPv3AuthProtocol(strings.ToLower(spec.AuthProto)),
		AuthPassphrase: spec.AuthKey,
		PrivProtocol:   snmputils.ParseSNMPv3PrivProtocol(strings.ToLower(spec.PrivProto)),
		PrivPassphrase: spec.PrivKey,
	}
	boots, engineTime := spec.EngineBoots, spec.EngineTime
	if boots == 0 {
		boots = 1
	}
	if engineTime == 0 {
		engineTime = 1
	}
	return &gosnmp.Message{
		Version:       gosnmp.Version3,
		SecurityLevel: snmputils.SNMPv3SecurityLevel(user.AuthProtocol, user.PrivProtocol),
		SecurityParameters: gosnmp.SecurityParameters{
			AuthoritativeEngineID:    engineID,
			AuthoritativeEngineBoots: boots,
			AuthoritativeEngineTime:  engineTime,
			UserName:                 spec.User,
		},
		PDUType:  pduType,
		VarBinds: trapVarBinds(spec.TrapOID, spec.Extra),
	}, user
}

func encodeV3(t testing.TB, msg *gosnmp.Message, user *gosnmp.User) []byte {
	t.Helper()
	data, err := msg.Encode(user)
	if err != nil {
		t.Fatalf("marshal v3 %s test packet: %v", msg.PDUType, err)
	}
	return data
}

func trapVarBinds(trapOID string, extra []gosnmp.VarBind) []gosnmp.VarBind {
	vbs := []gosnmp.VarBind{
		{Name: model.SysUpTimeOID, Value: gosnmp.TimeTicksValue(10)},
		{Name: model.SNMPTrapOID, Value: gosnmp.ObjectIdentifierValue(trapOID)},
	}
	return append(vbs, extra...)
}
