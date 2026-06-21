// SPDX-License-Identifier: GPL-3.0-or-later

package topologyutil

import (
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
)

func CanonicalSNMPEnumValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if open := strings.IndexByte(value, '('); open > 0 && strings.HasSuffix(value, ")") {
		value = strings.TrimSpace(value[:open])
	}
	return value
}

func DecodeHexString(value string) ([]byte, error) {
	clean := strings.TrimPrefix(strings.ToLower(NormalizeSNMPHexText(value)), "0x")
	clean = strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(clean)
	if clean == "" {
		return nil, fmt.Errorf("empty hex string")
	}
	if len(clean)%2 == 1 {
		clean = "0" + clean
	}
	return hex.DecodeString(clean)
}

func NormalizeSNMPHexText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	trimQuotes := func(v string) string {
		return strings.TrimSpace(strings.Trim(v, "\"'"))
	}
	value = trimQuotes(value)
	lower := strings.ToLower(value)
	for _, prefix := range []string{
		"hex-string:",
		"hex string:",
		"octet-string:",
		"octet string:",
		"string:",
	} {
		if strings.HasPrefix(lower, prefix) {
			value = trimQuotes(value[len(prefix):])
			lower = strings.ToLower(value)
		}
	}
	return value
}

func NormalizeIPAddress(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := net.ParseIP(value); ip != nil {
		return canonicalIPAddress(ip)
	}

	if bs, err := DecodeHexString(value); err == nil {
		if ip := ParseIPFromDecodedBytes(bs); ip != nil {
			return canonicalIPAddress(ip)
		}
	}

	return ""
}

func canonicalIPAddress(ip net.IP) string {
	if ip4 := ip.To4(); ip4 != nil {
		return net.IP(ip4).String()
	}
	return ip.String()
}

func NormalizeNonUnspecifiedIPAddress(value string) string {
	ip := NormalizeIPAddress(value)
	if ip == "" {
		return ""
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil || addr.IsUnspecified() {
		return ""
	}
	return addr.Unmap().String()
}

func ParseIPFromDecodedBytes(bs []byte) net.IP {
	if len(bs) == net.IPv4len || len(bs) == net.IPv6len {
		ip := net.IP(bs)
		if ip.To16() != nil {
			return ip
		}
	}

	ascii := DecodePrintableASCII(bs)
	if ascii == "" {
		return nil
	}

	if ip := net.ParseIP(ascii); ip != nil {
		return ip
	}
	return nil
}

func DecodePrintableASCII(bs []byte) string {
	if len(bs) == 0 {
		return ""
	}

	for _, b := range bs {
		if b == 0 {
			continue
		}
		if b < 32 || b > 126 {
			return ""
		}
	}

	s := strings.TrimRight(string(bs), "\x00")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return s
}

func NormalizeMAC(value string) string {
	value = NormalizeSNMPHexText(value)
	if value == "" {
		return ""
	}

	if hw, err := net.ParseMAC(value); err == nil {
		return strings.ToLower(hw.String())
	}

	clean := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(value))
	if clean == "" {
		return ""
	}

	bs, err := DecodeHexString(clean)
	if err != nil || len(bs) != 6 {
		return ""
	}

	return strings.ToLower(net.HardwareAddr(bs).String())
}

func NormalizeHexToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if mac := NormalizeMAC(value); mac != "" {
		return mac
	}
	if ip := NormalizeIPAddress(value); ip != "" {
		return ip
	}
	return strings.TrimSpace(value)
}

func NormalizeHexIdentifier(value string) string {
	value = NormalizeSNMPHexText(value)
	if value == "" {
		return ""
	}

	bs, err := DecodeHexString(value)
	if err == nil && len(bs) > 0 {
		return strings.ToLower(hex.EncodeToString(bs))
	}

	clean := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(value))
	if clean == "" {
		return ""
	}
	return clean
}

func NormalizeTopologyRouterID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := NormalizeIPAddress(value); ip != "" {
		return NormalizeNonUnspecifiedIPAddress(ip)
	}
	return value
}

func NormalizeBGPRouterID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := NormalizeNonUnspecifiedIPAddress(value); ip != "" {
		return ip
	}
	return value
}

func ParsePositiveInt64(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0
	}
	return parsed
}

func ParseIndex(value string) int {
	if value == "" {
		return 0
	}
	v, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return v
}
