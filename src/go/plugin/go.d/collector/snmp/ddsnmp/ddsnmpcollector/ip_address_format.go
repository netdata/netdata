// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/netdata/gosnmp"
)

func convPduToIPAddress(pdu gosnmp.VarBind) (string, error) {
	if pdu.Value.Type() == gosnmp.IPAddress {
		return convPduToString(pdu)
	}

	if bs, ok := pdu.Value.Bytes(); ok {
		if s, ok := ipAddressFromOctetBytes(bs); ok {
			return s, nil
		}
	} else if oid, ok := pdu.Value.OID(); ok {
		if s, ok := ipAddressFromText(oid); ok {
			return s, nil
		}
	}

	return "", fmt.Errorf("cannot convert %v to IP address", pdu.Value.Type())
}

func ipAddressFromOctetBytes(bs []byte) (string, bool) {
	if s, ok := ipAddressFromRawBytes(bs); ok {
		return s, true
	}

	if !utf8.Valid(bs) {
		return "", false
	}

	return ipAddressFromText(string(bs))
}

func ipAddressFromText(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}

	if s, ok := canonicalIPAddressText(raw); ok {
		return s, true
	}

	decoded, ok := parseDecimalOctets(raw)
	if !ok {
		return "", false
	}

	if s, ok := ipAddressFromRawBytes(decoded); ok {
		return s, true
	}

	if !utf8.Valid(decoded) {
		return "", false
	}

	text := strings.TrimSpace(string(decoded))
	if s, ok := canonicalIPAddressText(text); ok {
		return s, true
	}

	return "", false
}

func canonicalIPAddressText(raw string) (string, bool) {
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return "", false
	}
	return addr.Unmap().String(), true
}

func ipAddressFromRawBytes(bs []byte) (string, bool) {
	switch len(bs) {
	case 4:
		return renderIPv4Bytes(bs), true
	case 8:
		return renderIPv4Bytes(bs[:4]) + "%" + renderZoneIndex(bs[4:8]), true
	case 16:
		return canonicalIPAddressBytes(bs)
	case 20:
		if s, ok := canonicalIPAddressBytes(bs[:16]); ok {
			return s + "%" + renderZoneIndex(bs[16:20]), true
		}
		return "", false
	default:
		return "", false
	}
}

func renderIPv4Bytes(bs []byte) string {
	return fmt.Sprintf("%d.%d.%d.%d", bs[0], bs[1], bs[2], bs[3])
}

func canonicalIPAddressBytes(bs []byte) (string, bool) {
	addr, ok := netip.AddrFromSlice(bs)
	if !ok {
		return "", false
	}
	return addr.Unmap().String(), true
}

func renderZoneIndex(bs []byte) string {
	parts := make([]string, 0, len(bs))
	for _, b := range bs {
		parts = append(parts, strconv.Itoa(int(b)))
	}
	return strings.Join(parts, ".")
}

func parseDecimalOctets(raw string) ([]byte, bool) {
	parts := strings.Split(raw, ".")
	if len(parts) < 2 {
		return nil, false
	}

	out := make([]byte, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			return nil, false
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 255 {
			return nil, false
		}
		out = append(out, byte(n))
	}

	return out, true
}
