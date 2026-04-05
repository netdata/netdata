// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"net"
	"strconv"
	"strings"
)

type stpBridgeIDStatus uint8

const (
	stpBridgeIDInvalid stpBridgeIDStatus = iota
	stpBridgeIDEmpty
	stpBridgeIDValid
)

func stpBridgeAddressToMAC(value string) string {
	mac, status := parseSTPBridgeID(value, 0)
	if status != stpBridgeIDValid {
		return ""
	}
	return mac
}

func parseSTPBridgeID(value string, depth int) (string, stpBridgeIDStatus) {
	if depth > 2 {
		return "", stpBridgeIDInvalid
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return "", stpBridgeIDEmpty
	}

	if mac := normalizeMAC(value); mac != "" && strings.Count(mac, ":") == 5 {
		if mac == "00:00:00:00:00:00" {
			return "", stpBridgeIDEmpty
		}
		return mac, stpBridgeIDValid
	}

	if priority, bridgeID, ok := splitSTPBridgeIDWithPriority(value); ok {
		if priority == "0" && isSTPAllZeroBridgeID(bridgeID) {
			return "", stpBridgeIDEmpty
		}
		return parseSTPBridgeID(bridgeID, depth+1)
	}

	bs, err := decodeHexString(value)
	if err != nil || len(bs) == 0 {
		return "", stpBridgeIDInvalid
	}
	if allBytesZero(bs) {
		return "", stpBridgeIDEmpty
	}
	if ascii := decodePrintableASCII(bs); ascii != "" && depth < 2 {
		return parseSTPBridgeID(ascii, depth+1)
	}
	switch len(bs) {
	case 6:
		mac := strings.ToLower(net.HardwareAddr(bs).String())
		if mac == "00:00:00:00:00:00" {
			return "", stpBridgeIDEmpty
		}
		return mac, stpBridgeIDValid
	case 8:
		mac := strings.ToLower(net.HardwareAddr(bs[len(bs)-6:]).String())
		if mac == "00:00:00:00:00:00" {
			return "", stpBridgeIDEmpty
		}
		return mac, stpBridgeIDValid
	default:
		return "", stpBridgeIDInvalid
	}
}

func splitSTPBridgeIDWithPriority(value string) (string, string, bool) {
	parts := strings.SplitN(value, "-", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	priority := strings.TrimSpace(parts[0])
	bridgeID := strings.TrimSpace(parts[1])
	if priority == "" || bridgeID == "" {
		return "", "", false
	}
	if _, err := strconv.Atoi(priority); err != nil {
		return "", "", false
	}
	return priority, bridgeID, true
}

func isSTPAllZeroBridgeID(value string) bool {
	mac := normalizeMAC(value)
	if mac == "00:00:00:00:00:00" {
		return true
	}
	if mac != "" {
		return false
	}
	clean := normalizeHexIdentifier(value)
	if clean == "" {
		return false
	}
	for _, r := range clean {
		if r != '0' {
			return false
		}
	}
	return true
}

func allBytesZero(bs []byte) bool {
	for _, b := range bs {
		if b != 0 {
			return false
		}
	}
	return true
}

func stpDesignatedPortString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if _, err := strconv.Atoi(value); err == nil {
		return value
	}
	return normalizeHexIdentifier(value)
}
