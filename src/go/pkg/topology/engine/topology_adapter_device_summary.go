// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"math"
	"strconv"
	"strings"
)

func normalizeTopologyVLANID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToLower(value)
}

func safeTopologyInt64Add(base, add int64) int64 {
	if add <= 0 {
		return base
	}
	if base > math.MaxInt64-add {
		return math.MaxInt64
	}
	return base + add
}

func parseTopologyLabelInt64(value string) int64 {
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

func normalizeTopologyDuplex(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "full", "3":
		return "full"
	case "half", "2":
		return "half"
	case "unknown", "1":
		return "unknown"
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
