// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

func parsePositiveInt64(value string) int64 {
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

func canonicalSNMPEnumValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if open := strings.IndexByte(value, '('); open > 0 && strings.HasSuffix(value, ")") {
		value = strings.TrimSpace(value[:open])
	}
	return value
}

func decodeHexString(value string) ([]byte, error) {
	clean := strings.TrimPrefix(strings.ToLower(normalizeSNMPHexText(value)), "0x")
	clean = strings.NewReplacer(":", "", "-", "", ".", "", " ", "").Replace(clean)
	if clean == "" {
		return nil, fmt.Errorf("empty hex string")
	}
	if len(clean)%2 == 1 {
		clean = "0" + clean
	}
	return hex.DecodeString(clean)
}

func normalizeSNMPHexText(value string) string {
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

func decodeLLDPCapabilities(value string) []string {
	bs, err := decodeHexString(value)
	if err != nil {
		return nil
	}

	names := []string{
		"other",
		"repeater",
		"bridge",
		"wlanAccessPoint",
		"router",
		"telephone",
		"docsisCableDevice",
		"stationOnly",
		"cVlanComponent",
		"sVlanComponent",
		"twoPortMacRelay",
	}

	caps := make([]string, 0, len(names))
	for bit, name := range names {
		if bitSet(bs, bit) {
			caps = append(caps, name)
		}
	}
	return caps
}

func inferCategoryFromCapabilities(caps []string) string {
	has := make(map[string]bool, len(caps))
	for _, c := range caps {
		has[c] = true
	}
	switch {
	case has["router"]:
		return "router"
	case has["wlanAccessPoint"]:
		return "access point"
	case has["telephone"]:
		return "voip"
	case has["bridge"]:
		return "switch"
	case has["repeater"]:
		return "switch"
	case has["docsisCableDevice"]:
		return "network device"
	default:
		return ""
	}
}

func bitSet(bs []byte, bit int) bool {
	idx := bit / 8
	if idx < 0 || idx >= len(bs) {
		return false
	}
	mask := byte(1 << uint(7-(bit%8)))
	return bs[idx]&mask != 0
}

func parseIndex(value string) int {
	if value == "" {
		return 0
	}
	v, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
