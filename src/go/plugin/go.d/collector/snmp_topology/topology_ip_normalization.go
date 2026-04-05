// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"net"
	"strings"
)

func normalizeIPAddress(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}

	if bs, err := decodeHexString(value); err == nil {
		if ip := parseIPFromDecodedBytes(bs); ip != nil {
			return ip.String()
		}
	}

	return ""
}

func parseIPFromDecodedBytes(bs []byte) net.IP {
	if len(bs) == net.IPv4len || len(bs) == net.IPv6len {
		ip := net.IP(bs)
		if ip.To16() != nil {
			return ip
		}
	}

	ascii := decodePrintableASCII(bs)
	if ascii == "" {
		return nil
	}

	if ip := net.ParseIP(ascii); ip != nil {
		return ip
	}
	return nil
}

func decodePrintableASCII(bs []byte) string {
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
