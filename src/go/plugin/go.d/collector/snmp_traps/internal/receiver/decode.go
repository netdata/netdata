// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/netdata/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
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

	maxVarbinds           = 256
	maxNestingDepth       = 8
	maxOIDEncodedLen      = 128
	maxOctetStringLen     = 1024
	maxBERLengthOctets    = 4
	maxSNMPv1SpecificTrap = 2147483647
)

var (
	errBERTruncatedTLV    = errors.New("BER: truncated tag/length")
	errBERMissingLength   = errors.New("BER: missing length byte")
	errBERIndefinite      = errors.New("BER: indefinite length is unsupported")
	errBERTruncatedLength = errors.New("BER: truncated length")
)

type packetContext struct {
	Notification *gosnmp.Notification
	PDU          *model.TrapPDU
}

type decodeOptions struct {
	trustedRelay bool
}

// decodeTrap decodes one datagram as Process does, without its policy checks.
func decodeTrap(data []byte, udpPeer net.IP, receiver *gosnmp.Receiver) (*packetContext, []byte, error) {
	envelope, err := gosnmp.ReadEnvelope(data)
	v3 := err == nil && envelope.Version == gosnmp.Version3
	return decodeTrapWithOptions(data, v3, udpPeer, receiver, decodeOptions{})
}

// decodeTrapWithOptions decodes a datagram whose header says it is SNMPv3 when
// v3 is set. It returns the response the receiver made for its sender, if any:
// an inform's Response with the notification, or a Report with the error.
func decodeTrapWithOptions(data []byte, v3 bool, udpPeer net.IP, receiver *gosnmp.Receiver, opts decodeOptions) (*packetContext, []byte, error) {
	n, response, err := decodePacket(data, v3, receiver)
	if err != nil {
		return nil, response, err
	}
	pkt := &n.Message

	varbinds, err := packetVarbinds(pkt)
	if err != nil {
		return nil, nil, err
	}

	version, err := snmpVersion(pkt.Version)
	if err != nil {
		return nil, nil, err
	}

	pduType, err := snmpPDUType(pkt.PDUType)
	if err != nil {
		return nil, nil, err
	}

	trapOID := trapOIDFromVarbinds(varbinds)
	if trapOID == "" {
		return nil, nil, errors.New("missing snmpTrapOID.0")
	}

	peerIP := ""
	if udpPeer != nil {
		peerIP = udpPeer.String()
	}

	source := selectTrapSource(varbinds, udpPeer, opts.trustedRelay)

	tpdu := &model.TrapPDU{
		OID:         trapOID,
		SourceIP:    source.sourceIP,
		PeerIP:      peerIP,
		Community:   pkt.Community,
		Version:     version,
		PduType:     pduType,
		Varbinds:    varbinds,
		RequestID:   pkt.RequestID,
		SourceAudit: source.audit,
	}

	return &packetContext{
		Notification: n,
		PDU:          tpdu,
	}, response, nil
}

// classifyDecodeError labels a decode failure: the library's errors by their
// category, Netdata's decode budget and trap-shape errors by their own text.
func classifyDecodeError(err error) ErrorKind {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, gosnmp.ErrASNParse), errors.Is(err, gosnmp.ErrBadVersion), errors.Is(err, gosnmp.ErrInvalidMsgs):
		return ErrorMalformedPDU
	case errors.Is(err, gosnmp.ErrWrongDigest), errors.Is(err, gosnmp.ErrDecryption):
		return ErrorAuthFailure
	case errors.Is(err, gosnmp.ErrUnknownUsername), errors.Is(err, gosnmp.ErrUnknownSecurityLevel),
		errors.Is(err, gosnmp.ErrUnknownSecurityModels), errors.Is(err, gosnmp.ErrNotInTimeWindow):
		return ErrorUSMFailure
	case errors.Is(err, gosnmp.ErrUnknownEngineID):
		return ErrorUnknownEngine
	}

	s := err.Error()
	switch {
	case strings.Contains(s, "BER:"),
		strings.Contains(s, "datagram too large"),
		strings.Contains(s, "too many varbinds"),
		strings.Contains(s, "OctetString too long"),
		strings.Contains(s, "missing snmpTrapOID.0"),
		strings.Contains(s, "invalid SNMPv1"):
		return ErrorMalformedPDU
	default:
		return ErrorDecodeFailed
	}
}

// decodePacket applies Netdata's decode budget, then the receiver's decode and
// security processing.
func decodePacket(data []byte, v3 bool, receiver *gosnmp.Receiver) (n *gosnmp.Notification, response []byte, err error) {
	if len(data) > maxDatagramSize {
		return nil, nil, fmt.Errorf("datagram too large: %d > %d", len(data), maxDatagramSize)
	}
	if err := validateBERLimits(data, validateBEROptions{AllowLongOctetStrings: v3}); err != nil {
		return nil, nil, err
	}

	defer func() {
		if v := recover(); v != nil {
			n, response = nil, nil
			err = fmt.Errorf("panic decoding SNMP trap: %v", v)
		}
	}()

	n, response, err = receiver.Receive(data)
	if err != nil {
		return nil, response, err
	}
	if len(n.Message.VarBinds) > maxVarbinds {
		return nil, nil, fmt.Errorf("too many varbinds: %d > %d", len(n.Message.VarBinds), maxVarbinds)
	}
	return n, response, nil
}

func packetVarbinds(pkt *gosnmp.Message) ([]model.VarbindValue, error) {
	vbs, err := convertVarbinds(pkt.VarBinds)
	if err != nil {
		return nil, err
	}
	if pkt.Version != gosnmp.Version1 {
		return vbs, nil
	}

	trapOID := v1TrapOID(pkt.Enterprise, int(pkt.GenericTrap), int(pkt.SpecificTrap))
	if trapOID == "" {
		if pkt.GenericTrap == snmpV1TrapTypeEnterpriseSpec {
			return nil, fmt.Errorf("invalid SNMPv1 enterprise-specific trap: specificTrap %d out of range", pkt.SpecificTrap)
		}
		return nil, fmt.Errorf("invalid SNMPv1 generic trap value: %d", pkt.GenericTrap)
	}

	synthetic := []model.VarbindValue{
		{Name: "sysUpTime.0", OID: model.SysUpTimeOID, Type: "TimeTicks", Value: uint64(pkt.Timestamp)},
		{Name: "snmpTrapOID.0", OID: model.SNMPTrapOID, Type: "ObjectIdentifier", Value: trapOID},
	}
	if pkt.AgentAddress.IsValid() {
		synthetic = append(synthetic, model.VarbindValue{
			Name:  "snmpTrapAddress.0",
			OID:   model.SNMPTrapAddressOID,
			Type:  "IPAddress",
			Value: pkt.AgentAddress.String(),
		})
	}
	if pkt.Community != "" {
		synthetic = append(synthetic, model.VarbindValue{
			Name:  "snmpTrapCommunity.0",
			OID:   model.SNMPTrapCommunityOID,
			Type:  "OctetString",
			Value: pkt.Community,
		})
	}
	if pkt.Enterprise != "" {
		synthetic = append(synthetic, model.VarbindValue{
			Name:  "snmpTrapEnterprise.0",
			OID:   model.SNMPTrapEnterpriseOID,
			Type:  "ObjectIdentifier",
			Value: model.NormalizeOID(pkt.Enterprise),
		})
	}

	return append(synthetic, vbs...), nil
}

func convertVarbinds(pdus []gosnmp.VarBind) ([]model.VarbindValue, error) {
	vbs := make([]model.VarbindValue, 0, len(pdus))
	for _, pdu := range pdus {
		val, err := normalizePDUValue(pdu)
		if err != nil {
			return nil, fmt.Errorf("varbind %s: %w", model.NormalizeOID(pdu.Name), err)
		}
		vbs = append(vbs, model.VarbindValue{
			OID:   model.NormalizeOID(pdu.Name),
			Type:  model.ASN1Type(pdu.Value.Type().String()),
			Value: val,
		})
	}
	return vbs, nil
}

// normalizePDUValue gives each SMI type the Go type the trap pipeline stores:
// int64 for Integer, uint64 for the unsigned types, the OID and address as
// text (an empty IpAddress, which some devices send, as nil), a copy of the
// octets, float64 for the Opaque floats, nil for the types without a value.
func normalizePDUValue(pdu gosnmp.VarBind) (any, error) {
	v := pdu.Value
	switch v.Type() {
	case gosnmp.Null, gosnmp.NoSuchObject, gosnmp.NoSuchInstance, gosnmp.EndOfMibView, gosnmp.UnknownType:
		return nil, nil
	case gosnmp.ObjectIdentifier:
		oid, _ := v.OID()
		return model.NormalizeOID(oid), nil
	case gosnmp.IPAddress:
		ip, _ := v.IP()
		if !ip.IsValid() {
			return nil, nil
		}
		return ip.String(), nil
	case gosnmp.OctetString, gosnmp.Opaque:
		bs, _ := v.Bytes()
		if len(bs) > maxOctetStringLen {
			return nil, fmt.Errorf("OctetString too long: %d > %d", len(bs), maxOctetStringLen)
		}
		out := make([]byte, len(bs))
		copy(out, bs)
		return out, nil
	case gosnmp.Integer:
		n, _ := v.Int32()
		return int64(n), nil
	case gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks, gosnmp.Uinteger32, gosnmp.Counter64:
		n, _ := v.Uint64()
		return n, nil
	case gosnmp.OpaqueFloat, gosnmp.OpaqueDouble:
		f, _ := v.Float64()
		return f, nil
	default:
		return nil, fmt.Errorf("unsupported PDU value type %v", v.Type())
	}
}

func trapOIDFromVarbinds(vbs []model.VarbindValue) string {
	for _, vb := range vbs {
		if vb.OID != model.SNMPTrapOID {
			continue
		}
		if oid, ok := vb.Value.(string); ok {
			return model.NormalizeOID(oid)
		}
		return ""
	}
	return ""
}

type trapSourceSelection struct {
	sourceIP string
	audit    *model.TrapSourceAudit
}

func selectTrapSource(vbs []model.VarbindValue, udpPeer net.IP, trustedRelay bool) trapSourceSelection {
	peer := validIPString(ipString(udpPeer))
	trapAddr, trapAddrReject := sourceFromVarbindWithRejectReason(vbs, model.SNMPTrapAddressOID)

	audit := &model.TrapSourceAudit{
		UDPPeer:         peer,
		SnmpTrapAddress: trapAddr,
		TrustedRelay:    trustedRelay,
	}

	if peer != "" {
		if trustedRelay && trapAddr != "" {
			audit.Selected = trapAddr
			audit.Method = "trusted_relay_snmpTrapAddress.0"
			return trapSourceSelection{sourceIP: trapAddr, audit: audit}
		}
		audit.Selected = peer
		audit.Method = "udp_peer"
		if trapAddr != "" {
			audit.RejectedCandidates = append(audit.RejectedCandidates, "snmpTrapAddress.0:untrusted_relay_uses_udp_peer")
		} else if trapAddrReject != "" {
			audit.RejectedCandidates = append(audit.RejectedCandidates, "snmpTrapAddress.0:"+trapAddrReject)
		}
		return trapSourceSelection{sourceIP: peer, audit: audit}
	}

	if trustedRelay && trapAddr != "" {
		audit.Selected = trapAddr
		audit.Method = "trusted_relay_snmpTrapAddress.0"
		return trapSourceSelection{sourceIP: trapAddr, audit: audit}
	}
	if trapAddr != "" {
		audit.RejectedCandidates = append(audit.RejectedCandidates, "snmpTrapAddress.0:missing_udp_peer")
	}
	if trapAddrReject != "" {
		audit.RejectedCandidates = append(audit.RejectedCandidates, "snmpTrapAddress.0:"+trapAddrReject)
	}
	return trapSourceSelection{audit: audit}
}

func sourceFromVarbindWithRejectReason(vbs []model.VarbindValue, oid string) (string, string) {
	for _, vb := range vbs {
		if vb.OID != oid {
			continue
		}
		switch v := vb.Value.(type) {
		case string:
			return validIPStringWithRejectReason(v)
		case []byte:
			if len(v) == net.IPv4len || len(v) == net.IPv6len {
				return validIPStringWithRejectReason(net.IP(v).String())
			}
			return validIPStringWithRejectReason(string(v))
		case net.IP:
			if v == nil {
				return "", "invalid_ip"
			}
			return validIPStringWithRejectReason(v.String())
		default:
			return "", "invalid_type"
		}
	}
	return "", ""
}

func validIPString(s string) string {
	ip, _ := validIPStringWithRejectReason(s)
	return ip
}

func validIPStringWithRejectReason(s string) (string, string) {
	s = strings.TrimSpace(s)
	addr, err := netip.ParseAddr(s)
	if err != nil {
		ip := net.ParseIP(s)
		if ip == nil {
			return "", "invalid_ip"
		}
		parsed, ok := netip.AddrFromSlice(ip)
		if !ok {
			return "", "invalid_ip"
		}
		addr = parsed
	}
	addr = addr.Unmap()
	if addr.IsUnspecified() {
		return "", "unspecified_ip"
	}
	if addr.IsMulticast() {
		return "", "multicast_ip"
	}
	return addr.String(), ""
}

func ipString(ip net.IP) string {
	if ip == nil {
		return ""
	}
	return ip.String()
}

func snmpVersion(v gosnmp.Version) (model.SnmpVersion, error) {
	switch v {
	case gosnmp.Version1:
		return model.SnmpVersionV1, nil
	case gosnmp.Version2c:
		return model.SnmpVersionV2c, nil
	case gosnmp.Version3:
		return model.SnmpVersionV3, nil
	default:
		return "", fmt.Errorf("unknown SNMP version: %d", v)
	}
}

func snmpPDUType(t gosnmp.PDUType) (model.PduType, error) {
	switch t {
	case gosnmp.Trap, gosnmp.SNMPv2Trap:
		return model.PduTypeTrap, nil
	case gosnmp.InformRequest:
		return model.PduTypeInform, nil
	default:
		return "", fmt.Errorf("unsupported PDU type: 0x%02x", byte(t))
	}
}

func v1TrapOID(enterprise string, genericTrap, specificTrap int) string {
	if genericTrap == snmpV1TrapTypeEnterpriseSpec {
		enterprise = model.NormalizeOID(enterprise)
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

type validateBEROptions struct {
	AllowLongOctetStrings bool
}

func validateBERLimits(data []byte, opts validateBEROptions) error {
	if len(data) == 0 {
		return errors.New("BER: empty packet")
	}
	consumed, err := walkBER(data, 1, opts)
	if err != nil {
		return err
	}
	if consumed != len(data) {
		return fmt.Errorf("BER: trailing data: %d bytes", len(data)-consumed)
	}
	return nil
}

func walkBER(data []byte, depth int, opts validateBEROptions) (int, error) {
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
		if tag == tagOctetStr && !opts.AllowLongOctetStrings && len(content) > maxOctetStringLen {
			return 0, fmt.Errorf("BER: OctetString too long: %d > %d", len(content), maxOctetStringLen)
		}
		if isConstructedBER(tag) && len(content) > 0 {
			if _, err := walkBER(content, depth+1, opts); err != nil {
				return 0, err
			}
		}
		pos += consumed
	}
	return pos, nil
}

func readTLV(data []byte) (tag byte, content []byte, consumed int, err error) {
	if len(data) < 2 {
		return 0, nil, 0, errBERTruncatedTLV
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
		return 0, 0, errBERMissingLength
	}
	b := data[0]
	if b&0x80 == 0 {
		return int(b), 1, nil
	}

	n := int(b & 0x7f)
	if n == 0 {
		return 0, 0, errBERIndefinite
	}
	if n > maxBERLengthOctets {
		return 0, 0, fmt.Errorf("BER: length uses %d octets, max %d", n, maxBERLengthOctets)
	}
	if len(data) < 1+n {
		return 0, 0, errBERTruncatedLength
	}

	var length64 uint64
	for _, lb := range data[1 : 1+n] {
		length64 = (length64 << 8) | uint64(lb)
		if length64 > uint64(maxDatagramSize) {
			return 0, 0, fmt.Errorf("BER: value length %d exceeds maximum datagram size %d", length64, maxDatagramSize)
		}
	}
	return int(length64), 1 + n, nil
}

func isConstructedBER(tag byte) bool {
	return tag&constructedTL != 0
}
