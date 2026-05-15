// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import "strings"

func canonicalARPProtocol(protocol string) string {
	protocol = strings.TrimSpace(strings.ToLower(protocol))
	switch protocol {
	case "", "arp":
		return "arp"
	case "nd":
		return "nd"
	default:
		return "arp"
	}
}

func canonicalAddrType(addrType, ip string) string {
	addrType = strings.TrimSpace(strings.ToLower(addrType))
	if ipAddr := parseAddr(ip); ipAddr.IsValid() {
		if ipAddr.Is4() {
			return "ipv4"
		}
		return "ipv6"
	}
	if addrType == "" {
		return ""
	}
	return addrType
}

func deriveRemoteDeviceID(hostname, chassisID, mgmtIP, fallback string) string {
	if host := canonicalHost(hostname); host != "" {
		return host
	}
	if ch := canonicalToken(chassisID); ch != "" {
		return "chassis-" + ch
	}
	if ip := canonicalIP(mgmtIP); ip != "" {
		return "ip-" + strings.ReplaceAll(ip, ":", "-")
	}
	if fb := canonicalHost(fallback); fb != "" {
		return fb
	}
	return "discovered-unknown"
}

func canonicalHost(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimSuffix(v, ".")
	return v
}

func canonicalToken(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	v = strings.TrimPrefix(v, "0x")
	v = strings.ReplaceAll(v, ":", "")
	v = strings.ReplaceAll(v, "-", "")
	v = strings.ReplaceAll(v, ".", "")
	v = strings.ReplaceAll(v, " ", "")
	return v
}
