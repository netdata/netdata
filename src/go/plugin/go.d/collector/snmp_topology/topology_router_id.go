// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "strings"

func normalizeTopologyRouterID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := normalizeIPAddress(value); ip != "" {
		return normalizeNonUnspecifiedIPAddress(ip)
	}
	return value
}

func normalizeBGPRouterID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if ip := normalizeNonUnspecifiedIPAddress(value); ip != "" {
		return ip
	}
	return value
}
