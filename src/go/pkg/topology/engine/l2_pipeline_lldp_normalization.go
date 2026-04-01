// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"encoding/hex"
	"strings"
)

func normalizeLLDPPortIDForMatch(portID, subtype string) string {
	portID = strings.TrimSpace(portID)
	if portID == "" {
		return ""
	}

	switch normalizeLLDPPortSubtypeForMatch(subtype) {
	case "mac":
		if mac := canonicalLLDPMACToken(portID); mac != "" {
			return mac
		}
	case "network":
		if ip := canonicalIP(portID); ip != "" {
			return ip
		}
	}

	return portID
}

func normalizeLLDPPortSubtypeForMatch(subtype string) string {
	switch strings.ToLower(strings.TrimSpace(subtype)) {
	case "3", "macaddress":
		return "mac"
	case "4", "networkaddress":
		return "network"
	default:
		return strings.ToLower(strings.TrimSpace(subtype))
	}
}

func normalizeLLDPChassisForMatch(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if ip := canonicalIP(v); ip != "" {
		return ip
	}
	if mac := canonicalLLDPMACToken(v); mac != "" {
		return mac
	}
	return v
}

func canonicalLLDPMACToken(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" {
		return ""
	}

	clean := strings.TrimPrefix(v, "0x")
	clean = strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(clean)
	if len(clean) != 12 {
		return ""
	}
	if _, err := hex.DecodeString(clean); err != nil {
		return ""
	}
	return clean
}
