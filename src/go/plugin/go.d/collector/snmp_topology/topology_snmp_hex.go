// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"encoding/hex"
	"fmt"
	"strings"
)

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
