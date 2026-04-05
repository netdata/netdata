// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"encoding/hex"
	"net"
	"strings"
)

func normalizeMAC(value string) string {
	value = normalizeSNMPHexText(value)
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

	bs, err := decodeHexString(clean)
	if err != nil || len(bs) != 6 {
		return ""
	}

	return strings.ToLower(net.HardwareAddr(bs).String())
}

func normalizeHexToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if mac := normalizeMAC(value); mac != "" {
		return mac
	}
	if ip := normalizeIPAddress(value); ip != "" {
		return ip
	}
	return strings.TrimSpace(value)
}

func normalizeHexIdentifier(value string) string {
	value = normalizeSNMPHexText(value)
	if value == "" {
		return ""
	}

	bs, err := decodeHexString(value)
	if err == nil && len(bs) > 0 {
		return strings.ToLower(hex.EncodeToString(bs))
	}

	clean := strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(strings.ToLower(value))
	if clean == "" {
		return ""
	}
	return clean
}
