// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
)

const (
	tagInteger    = 0x02
	tagOID        = 0x06
	tagSequence   = 0x30
	tagTrapV1     = 0xa4
	tagInform     = 0xa6
	tagTrapV2     = 0xa7
	tagOctetStr   = 0x04
	constructedTL = 0x20

	snmpV1TrapTypeEnterpriseSpec = 6

	sysUpTimeOID          = "1.3.6.1.2.1.1.3.0"
	snmpTrapOIDOID        = "1.3.6.1.6.3.1.1.4.1.0"
	snmpTrapAddressOID    = "1.3.6.1.6.3.18.1.3.0"
	snmpTrapCommunityOID  = "1.3.6.1.6.3.18.1.4.0"
	snmpTrapEnterpriseOID = "1.3.6.1.6.3.1.1.4.3.0"
	maxVarbinds           = 256
	maxNestingDepth       = 8
	maxOIDEncodedLen      = 128
	maxOctetStringLen     = 1024
	maxBERLengthOctets    = 4
	maxSNMPv1SpecificTrap = 2147483647
)

var (
	decodeBudgetTarget      = 1 * time.Millisecond
	errDecodeBudgetExceeded = errors.New("decode budget exceeded")
	trapDecodeLogger        = gosnmp.NewLogger(log.New(io.Discard, "", 0))
)

type TrapPDU struct {
	OID       string
	SourceIP  string
	PeerIP    string
	Community string
	Version   SnmpVersion
	PduType   PduType
	Varbinds  []VarbindValue
	RequestID uint32
}

type TrapPacketContext struct {
	Packet *gosnmp.SnmpPacket
	PDU    *TrapPDU
}

func DecodeTrap(data []byte, udpPeer net.IP, secTable *gosnmp.SnmpV3SecurityParametersTable) (*TrapPacketContext, error) {
	start := time.Now()

	pkt, err := decodePacket(data, secTable)
	if err != nil {
		return nil, err
	}

	if time.Since(start) > decodeBudgetTarget {
		return nil, errDecodeBudgetExceeded
	}

	varbinds, err := packetVarbinds(pkt)
	if err != nil {
		return nil, err
	}
	if time.Since(start) > decodeBudgetTarget {
		return nil, errDecodeBudgetExceeded
	}

	version, err := snmpVersion(pkt.Version)
	if err != nil {
		return nil, err
	}

	pduType, err := snmpPDUType(pkt.PDUType)
	if err != nil {
		return nil, err
	}

	trapOID := trapOIDFromVarbinds(varbinds)
	if trapOID == "" {
		return nil, errors.New("missing snmpTrapOID.0")
	}

	peerIP := ""
	if udpPeer != nil {
		peerIP = udpPeer.String()
	}

	tpdu := &TrapPDU{
		OID:       trapOID,
		SourceIP:  identifySource(varbinds, udpPeer),
		PeerIP:    peerIP,
		Community: pkt.Community,
		Version:   version,
		PduType:   pduType,
		Varbinds:  varbinds,
		RequestID: pkt.RequestID,
	}

	return &TrapPacketContext{
		Packet: pkt,
		PDU:    tpdu,
	}, nil
}

func ClassifyDecodeError(err error) string {
	if err == nil {
		return ""
	}

	// GoSNMP returns plain errors for several v3 USM decode failures. Keep the
	// text mapping narrow and covered by tests until the fork exposes typed
	// errors for authentication, decryption, and engine-ID failures.
	s := err.Error()
	switch {
	case errors.Is(err, errDecodeBudgetExceeded):
		return "malformed_pdu"
	case strings.Contains(s, "BER:"):
		return "malformed_pdu"
	case strings.Contains(s, "datagram too large"):
		return "malformed_pdu"
	case strings.Contains(s, "too many varbinds"):
		return "malformed_pdu"
	case strings.Contains(s, "OctetString too long"):
		return "malformed_pdu"
	case strings.Contains(s, "missing snmpTrapOID.0"):
		return "malformed_pdu"
	case strings.Contains(s, "invalid SNMPv1"):
		return "malformed_pdu"
	case strings.Contains(s, "authentication"):
		return "auth_failures"
	case strings.Contains(s, "decrypt"):
		return "auth_failures"
	case strings.Contains(s, "USM"):
		return "usm_failures"
	case strings.Contains(s, "no security parameters"):
		return "usm_failures"
	case strings.Contains(s, "no credentials"):
		return "usm_failures"
	case strings.Contains(s, "unknown engine"):
		return "unknown_engine_id"
	case strings.Contains(s, "engine ID"):
		return "unknown_engine_id"
	default:
		return "decode_failed"
	}
}

func sniffSNMPVersion(data []byte) (SnmpVersion, bool) {
	tag, valueStart, valueEnd, _, err := readBERElement(data, 0)
	if err != nil || tag != tagSequence {
		return "", false
	}

	tag, intStart, intEnd, _, err := readBERElement(data[:valueEnd], valueStart)
	if err != nil || tag != tagInteger {
		return "", false
	}

	version, ok := parseBERVersion(data[intStart:intEnd])
	if !ok {
		return "", false
	}
	v, err := snmpVersion(gosnmp.SnmpVersion(version))
	if err != nil {
		return "", false
	}
	return v, true
}

func extractSNMPv3EngineIDHex(data []byte) (string, bool, error) {
	tag, valueStart, valueEnd, _, err := readBERElement(data, 0)
	if err != nil {
		return "", false, err
	}
	if tag != tagSequence {
		return "", false, nil
	}

	pos := valueStart
	tag, intStart, intEnd, next, err := readBERElement(data[:valueEnd], pos)
	if err != nil {
		return "", false, err
	}
	if tag != tagInteger {
		return "", false, nil
	}
	version, ok := parseBERVersion(data[intStart:intEnd])
	if !ok || version != int(gosnmp.Version3) {
		return "", false, nil
	}

	pos = next
	tag, _, _, next, err = readBERElement(data[:valueEnd], pos)
	if err != nil {
		return "", false, err
	}
	if tag != tagSequence {
		return "", false, fmt.Errorf("SNMPv3 header data is not a sequence")
	}

	pos = next
	tag, secStart, secEnd, _, err := readBERElement(data[:valueEnd], pos)
	if err != nil {
		return "", false, err
	}
	if tag != tagOctetStr {
		return "", false, fmt.Errorf("SNMPv3 security parameters are not an octet string")
	}

	secData := data[secStart:secEnd]
	tag, usmStart, usmEnd, _, err := readBERElement(secData, 0)
	if err != nil {
		return "", false, err
	}
	if tag != tagSequence {
		return "", false, fmt.Errorf("SNMPv3 USM parameters are not a sequence")
	}

	tag, engineStart, engineEnd, _, err := readBERElement(secData[:usmEnd], usmStart)
	if err != nil {
		return "", false, err
	}
	if tag != tagOctetStr {
		return "", false, fmt.Errorf("SNMPv3 authoritative engine ID is not an octet string")
	}

	return hex.EncodeToString(secData[engineStart:engineEnd]), true, nil
}

func readBERElement(data []byte, pos int) (tag byte, valueStart, valueEnd, next int, err error) {
	if pos < 0 || pos+2 > len(data) {
		return 0, 0, 0, 0, fmt.Errorf("BER: truncated element at offset %d", pos)
	}

	tag = data[pos]
	pos++

	lengthByte := data[pos]
	pos++

	length := 0
	if lengthByte&0x80 == 0 {
		length = int(lengthByte)
	} else {
		n := int(lengthByte & 0x7f)
		if n == 0 {
			return 0, 0, 0, 0, errors.New("BER: indefinite length is not allowed")
		}
		if n > maxBERLengthOctets {
			return 0, 0, 0, 0, fmt.Errorf("BER: length uses %d octets, max %d", n, maxBERLengthOctets)
		}
		if pos+n > len(data) {
			return 0, 0, 0, 0, fmt.Errorf("BER: truncated length at offset %d", pos)
		}
		for i := range n {
			length = (length << 8) | int(data[pos+i])
		}
		pos += n
	}

	valueStart = pos
	valueEnd = pos + length
	if valueEnd > len(data) {
		return 0, 0, 0, 0, fmt.Errorf("BER: value length %d exceeds remaining bytes %d", length, len(data)-pos)
	}
	return tag, valueStart, valueEnd, valueEnd, nil
}

func parseBERVersion(data []byte) (int, bool) {
	if len(data) == 0 || len(data) > 4 {
		return 0, false
	}
	v := 0
	for _, b := range data {
		v = (v << 8) | int(b)
	}
	return v, true
}

func decodePacket(data []byte, secTable *gosnmp.SnmpV3SecurityParametersTable) (pkt *gosnmp.SnmpPacket, err error) {
	if len(data) > maxDatagramSize {
		return nil, fmt.Errorf("datagram too large: %d > %d", len(data), maxDatagramSize)
	}
	if err := validateBERLimits(data); err != nil {
		return nil, err
	}

	defer func() {
		if v := recover(); v != nil {
			pkt = nil
			err = fmt.Errorf("panic decoding SNMP trap: %v", v)
		}
	}()

	decoder := &gosnmp.GoSNMP{
		Logger:                      trapDecodeLogger,
		TrapSecurityParametersTable: secTable,
	}
	if version, ok := sniffSNMPVersion(data); ok && version == SnmpVersionV3 {
		decoder.Version = gosnmp.Version3
	}
	pkt, err = decoder.UnmarshalTrap(data, false)
	if err != nil {
		return nil, err
	}
	if len(pkt.Variables) > maxVarbinds {
		return nil, fmt.Errorf("too many varbinds: %d > %d", len(pkt.Variables), maxVarbinds)
	}
	return pkt, nil
}

func packetVarbinds(pkt *gosnmp.SnmpPacket) ([]VarbindValue, error) {
	vbs, err := convertVarbinds(pkt.Variables)
	if err != nil {
		return nil, err
	}
	if pkt.Version != gosnmp.Version1 {
		return vbs, nil
	}

	trapOID := v1TrapOID(pkt.Enterprise, pkt.GenericTrap, pkt.SpecificTrap)
	if trapOID == "" {
		if pkt.GenericTrap == snmpV1TrapTypeEnterpriseSpec {
			return nil, fmt.Errorf("invalid SNMPv1 enterprise-specific trap: specificTrap %d out of range", pkt.SpecificTrap)
		}
		return nil, fmt.Errorf("invalid SNMPv1 generic trap value: %d", pkt.GenericTrap)
	}

	synthetic := []VarbindValue{
		{Name: "sysUpTime.0", OID: sysUpTimeOID, Type: "TimeTicks", Value: uint64(pkt.Timestamp)},
		{Name: "snmpTrapOID.0", OID: snmpTrapOIDOID, Type: "ObjectIdentifier", Value: trapOID},
	}
	if pkt.AgentAddress != "" {
		synthetic = append(synthetic, VarbindValue{
			Name:  "snmpTrapAddress.0",
			OID:   snmpTrapAddressOID,
			Type:  "IPAddress",
			Value: pkt.AgentAddress,
		})
	}
	if pkt.Community != "" {
		synthetic = append(synthetic, VarbindValue{
			Name:  "snmpTrapCommunity.0",
			OID:   snmpTrapCommunityOID,
			Type:  "OctetString",
			Value: pkt.Community,
		})
	}
	if pkt.Enterprise != "" {
		synthetic = append(synthetic, VarbindValue{
			Name:  "snmpTrapEnterprise.0",
			OID:   snmpTrapEnterpriseOID,
			Type:  "ObjectIdentifier",
			Value: normalizeOID(pkt.Enterprise),
		})
	}

	return append(synthetic, vbs...), nil
}

func convertVarbinds(pdus []gosnmp.SnmpPDU) ([]VarbindValue, error) {
	vbs := make([]VarbindValue, 0, len(pdus))
	for _, pdu := range pdus {
		val, err := normalizePDUValue(pdu)
		if err != nil {
			return nil, fmt.Errorf("varbind %s: %w", normalizeOID(pdu.Name), err)
		}
		vbs = append(vbs, VarbindValue{
			OID:   normalizeOID(pdu.Name),
			Type:  ASN1Type(pdu.Type.String()),
			Value: val,
		})
	}
	return vbs, nil
}

func normalizePDUValue(pdu gosnmp.SnmpPDU) (any, error) {
	switch v := pdu.Value.(type) {
	case nil:
		return nil, nil
	case string:
		if pdu.Type == gosnmp.ObjectIdentifier {
			return normalizeOID(v), nil
		}
		if pdu.Type == gosnmp.OctetString && len(v) > maxOctetStringLen {
			return nil, fmt.Errorf("OctetString too long: %d > %d", len(v), maxOctetStringLen)
		}
		return v, nil
	case []byte:
		if len(v) > maxOctetStringLen {
			return nil, fmt.Errorf("OctetString too long: %d > %d", len(v), maxOctetStringLen)
		}
		out := make([]byte, len(v))
		copy(out, v)
		return out, nil
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		return uint64(v), nil
	case uint8:
		return uint64(v), nil
	case uint16:
		return uint64(v), nil
	case uint32:
		return uint64(v), nil
	case uint64:
		return v, nil
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case bool:
		return v, nil
	default:
		return nil, fmt.Errorf("unsupported PDU value type %T", v)
	}
}

func trapOIDFromVarbinds(vbs []VarbindValue) string {
	for _, vb := range vbs {
		if vb.OID != snmpTrapOIDOID {
			continue
		}
		if oid, ok := vb.Value.(string); ok {
			return normalizeOID(oid)
		}
		return ""
	}
	return ""
}

func identifySource(vbs []VarbindValue, udpPeer net.IP) string {
	if addr := sourceFromVarbind(vbs, snmpTrapAddressOID); addr != "" {
		return addr
	}
	if udpPeer != nil {
		return udpPeer.String()
	}
	return ""
}

func sourceFromVarbind(vbs []VarbindValue, oid string) string {
	for _, vb := range vbs {
		if vb.OID != oid {
			continue
		}
		switch v := vb.Value.(type) {
		case string:
			return validIPString(v)
		case []byte:
			if len(v) == net.IPv4len || len(v) == net.IPv6len {
				return net.IP(v).String()
			}
			return validIPString(string(v))
		case net.IP:
			if v == nil {
				return ""
			}
			return v.String()
		default:
			return ""
		}
	}
	return ""
}

func validIPString(s string) string {
	ip := net.ParseIP(s)
	if ip == nil {
		return ""
	}
	return ip.String()
}

func snmpVersion(v gosnmp.SnmpVersion) (SnmpVersion, error) {
	switch v {
	case gosnmp.Version1:
		return SnmpVersionV1, nil
	case gosnmp.Version2c:
		return SnmpVersionV2c, nil
	case gosnmp.Version3:
		return SnmpVersionV3, nil
	default:
		return "", fmt.Errorf("unknown SNMP version: %d", v)
	}
}

func snmpPDUType(t gosnmp.PDUType) (PduType, error) {
	switch t {
	case gosnmp.Trap, gosnmp.SNMPv2Trap:
		return PduTypeTrap, nil
	case gosnmp.InformRequest:
		return PduTypeInform, nil
	default:
		return "", fmt.Errorf("unsupported PDU type: 0x%02x", byte(t))
	}
}

func normalizeOID(oid string) string {
	return strings.TrimPrefix(oid, ".")
}

func v1TrapOID(enterprise string, genericTrap, specificTrap int) string {
	if genericTrap == snmpV1TrapTypeEnterpriseSpec {
		enterprise = normalizeOID(enterprise)
		if enterprise == "" || specificTrap < 0 || specificTrap > maxSNMPv1SpecificTrap {
			return ""
		}
		return enterprise + ".0." + strconv.Itoa(specificTrap)
	}

	switch genericTrap {
	case 0:
		return "1.3.6.1.6.3.1.1.5.1"
	case 1:
		return "1.3.6.1.6.3.1.1.5.2"
	case 2:
		return "1.3.6.1.6.3.1.1.5.3"
	case 3:
		return "1.3.6.1.6.3.1.1.5.4"
	case 4:
		return "1.3.6.1.6.3.1.1.5.5"
	case 5:
		return "1.3.6.1.6.3.1.1.5.6"
	default:
		return ""
	}
}

func validateBERLimits(data []byte) error {
	if len(data) == 0 {
		return errors.New("BER: empty packet")
	}
	consumed, err := walkBER(data, 1)
	if err != nil {
		return err
	}
	if consumed != len(data) {
		return fmt.Errorf("BER: trailing data: %d bytes", len(data)-consumed)
	}
	return nil
}

func walkBER(data []byte, depth int) (int, error) {
	if depth > maxNestingDepth {
		return 0, fmt.Errorf("BER: nesting depth %d > %d", depth, maxNestingDepth)
	}

	pos := 0
	for pos < len(data) {
		tag, content, consumed, err := readTLV(data[pos:])
		if err != nil {
			return 0, err
		}
		if tag == tagOID && len(content) > maxOIDEncodedLen {
			return 0, fmt.Errorf("BER: OID too long: %d > %d", len(content), maxOIDEncodedLen)
		}
		if tag == tagOctetStr && len(content) > maxOctetStringLen {
			return 0, fmt.Errorf("BER: OctetString too long: %d > %d", len(content), maxOctetStringLen)
		}
		if isConstructedBER(tag) && len(content) > 0 {
			if _, err := walkBER(content, depth+1); err != nil {
				return 0, err
			}
		}
		pos += consumed
	}
	return pos, nil
}

func readTLV(data []byte) (tag byte, content []byte, consumed int, err error) {
	if len(data) < 2 {
		return 0, nil, 0, errors.New("BER: truncated tag/length")
	}
	tag = data[0]
	length, lengthBytes, err := decodeBERLength(data[1:])
	if err != nil {
		return 0, nil, 0, err
	}
	start := 1 + lengthBytes
	end := start + length
	if length < 0 || end < start || end > len(data) {
		return 0, nil, 0, fmt.Errorf("BER: invalid length %d", length)
	}
	return tag, data[start:end], end, nil
}

func decodeBERLength(data []byte) (length int, consumed int, err error) {
	if len(data) == 0 {
		return 0, 0, errors.New("BER: missing length byte")
	}
	b := data[0]
	if b&0x80 == 0 {
		return int(b), 1, nil
	}

	n := int(b & 0x7f)
	if n == 0 {
		return 0, 0, errors.New("BER: indefinite length is unsupported")
	}
	if n > maxBERLengthOctets {
		return 0, 0, fmt.Errorf("BER: length uses %d octets, max %d", n, maxBERLengthOctets)
	}
	if len(data) < 1+n {
		return 0, 0, errors.New("BER: truncated length")
	}

	for _, lb := range data[1 : 1+n] {
		length = (length << 8) | int(lb)
	}
	return length, 1 + n, nil
}

func isConstructedBER(tag byte) bool {
	return tag&constructedTL != 0
}
