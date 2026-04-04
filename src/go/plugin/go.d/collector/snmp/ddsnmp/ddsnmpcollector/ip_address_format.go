// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gosnmp/gosnmp"
)

func convPduToIPAddress(pdu gosnmp.SnmpPDU) (string, error) {
	if pdu.Type == gosnmp.IPAddress {
		return convPduToString(pdu)
	}

	switch v := pdu.Value.(type) {
	case []byte:
		if s, ok := ipAddressFromOctetBytes(v); ok {
			return s, nil
		}
	case string:
		if s, ok := ipAddressFromText(v); ok {
			return s, nil
		}
	}

	return "", fmt.Errorf("cannot convert %T to IP address", pdu.Value)
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

	if _, err := netip.ParseAddr(raw); err == nil {
		return raw, true
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
	if _, err := netip.ParseAddr(text); err == nil {
		return text, true
	}

	return "", false
}

func ipAddressFromRawBytes(bs []byte) (string, bool) {
	switch len(bs) {
	case 4:
		return renderIPv4Bytes(bs), true
	case 8:
		return renderIPv4Bytes(bs[:4]) + "%" + renderZoneIndex(bs[4:8]), true
	case 16:
		return renderIPv6Bytes(bs), true
	case 20:
		return renderIPv6Bytes(bs[:16]) + "%" + renderZoneIndex(bs[16:20]), true
	default:
		return "", false
	}
}

func renderIPv4Bytes(bs []byte) string {
	return fmt.Sprintf("%d.%d.%d.%d", bs[0], bs[1], bs[2], bs[3])
}

func renderIPv6Bytes(bs []byte) string {
	parts := make([]string, 0, 8)
	for i := 0; i < 16; i += 2 {
		parts = append(parts, fmt.Sprintf("%02x%02x", bs[i], bs[i+1]))
	}
	return strings.Join(parts, ":")
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
