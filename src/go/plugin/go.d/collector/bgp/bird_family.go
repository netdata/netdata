// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import "strings"

func normalizeBIRDChannelName(value string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	return strings.Join(fields, " ")
}

func parseBIRDChannelFamily(name string) (string, string, bool) {
	switch normalizeBIRDChannelName(name) {
	case "ipv4":
		return "ipv4", "unicast", true
	case "ipv6":
		return "ipv6", "unicast", true
	case "ipv4 multicast", "ipv4-mc":
		return "ipv4", "multicast", true
	case "ipv6 multicast", "ipv6-mc":
		return "ipv6", "multicast", true
	case "ipv4 mpls", "ipv4-mpls":
		return "ipv4", "label", true
	case "ipv6 mpls", "ipv6-mpls":
		return "ipv6", "label", true
	case "vpn4 mpls", "vpn4-mpls":
		return "ipv4", "vpn", true
	case "vpn6 mpls", "vpn6-mpls":
		return "ipv6", "vpn", true
	case "vpn4 multicast", "vpn4-mc":
		return "ipv4", "multicast_vpn", true
	case "vpn6 multicast", "vpn6-mc":
		return "ipv6", "multicast_vpn", true
	case "flow4":
		return "ipv4", "flowspec", true
	case "flow6":
		return "ipv6", "flowspec", true
	default:
		return "", "", false
	}
}
