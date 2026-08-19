// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func CanonicalMatchKey(match Match) string {
	if key := CanonicalPrimaryMACListKey(match); key != "" {
		return "mac:" + key
	}
	if key := CanonicalHardwareListKey(match.ChassisIDs); key != "" {
		return "chassis:" + key
	}
	if key := CanonicalIPListKey(match.IPAddresses); key != "" {
		return "ip:" + key
	}
	if key := CanonicalStringListKey(match.Hostnames); key != "" {
		return "hostname:" + key
	}
	if key := CanonicalStringListKey(match.DNSNames); key != "" {
		return "dns:" + key
	}
	if sysName := strings.ToLower(strings.TrimSpace(match.SysName)); sysName != "" {
		return "sysname:" + sysName
	}
	if match.SysObjectID != "" {
		return "sysobjectid:" + match.SysObjectID
	}
	return ""
}

func LinkSortKey(link Link) string {
	return strings.Join([]string{
		link.Protocol,
		link.Direction,
		CanonicalMatchKey(link.Src.Match),
		CanonicalMatchKey(link.Dst.Match),
		EndpointKey(link.Src, "if_index"),
		EndpointKey(link.Src, "if_name"),
		EndpointKey(link.Src, "port_id"),
		EndpointKey(link.Dst, "if_index"),
		EndpointKey(link.Dst, "if_name"),
		EndpointKey(link.Dst, "port_id"),
		link.State,
	}, "|")
}

func EndpointKey(endpoint LinkEndpoint, key string) string {
	switch key {
	case "if_index":
		if endpoint.IfIndex <= 0 {
			return ""
		}
		return strconv.Itoa(endpoint.IfIndex)
	case "if_name":
		return strings.TrimSpace(endpoint.IfName)
	case "port_id":
		return strings.TrimSpace(endpoint.PortID)
	case "port_name":
		return strings.TrimSpace(endpoint.PortName)
	default:
		return ""
	}
}

func MatchIdentityKeys(match Match) []string {
	seen := make(map[string]struct{}, 8)
	add := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		seen[kind+":"+value] = struct{}{}
	}

	for _, value := range match.ChassisIDs {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mac := topologyutil.NormalizeMAC(value); mac != "" {
			add("hw", mac)
			continue
		}
		if ip := topologyutil.NormalizeIPAddress(value); ip != "" {
			add("ip", ip)
			continue
		}
		add("chassis", strings.ToLower(value))
	}
	for _, value := range match.MacAddresses {
		if mac := topologyutil.NormalizeMAC(value); mac != "" {
			add("hw", mac)
		}
	}
	for _, value := range match.IPAddresses {
		if ip := topologyutil.NormalizeIPAddress(value); ip != "" {
			add("ip", ip)
			continue
		}
		add("ipraw", strings.ToLower(strings.TrimSpace(value)))
	}
	for _, value := range match.Hostnames {
		add("hostname", strings.ToLower(strings.TrimSpace(value)))
	}
	for _, value := range match.DNSNames {
		add("dns", strings.ToLower(strings.TrimSpace(value)))
	}
	if sysName := strings.TrimSpace(match.SysName); sysName != "" {
		add("sysname", strings.ToLower(sysName))
	}

	if len(seen) == 0 {
		return nil
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func CanonicalPrimaryMACListKey(match Match) string {
	seen := make(map[string]struct{}, len(match.MacAddresses)+len(match.ChassisIDs))
	for _, value := range match.MacAddresses {
		if mac := topologyutil.NormalizeMAC(value); mac != "" {
			seen[mac] = struct{}{}
		}
	}
	for _, value := range match.ChassisIDs {
		if mac := topologyutil.NormalizeMAC(value); mac != "" {
			seen[mac] = struct{}{}
		}
	}
	if len(seen) == 0 {
		return ""
	}
	values := make([]string, 0, len(seen))
	for value := range seen {
		values = append(values, value)
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

func CanonicalHardwareListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mac := topologyutil.NormalizeMAC(value); mac != "" {
			out = append(out, mac)
			continue
		}
		if ip := topologyutil.NormalizeIPAddress(value); ip != "" {
			out = append(out, ip)
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueStrings(out)
	return strings.Join(out, ",")
}

func CanonicalMACListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if mac := topologyutil.NormalizeMAC(value); mac != "" {
			out = append(out, mac)
		}
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueStrings(out)
	return strings.Join(out, ",")
}

func CanonicalIPListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if ip := topologyutil.NormalizeIPAddress(value); ip != "" {
			out = append(out, ip)
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueStrings(out)
	return strings.Join(out, ",")
}

func CanonicalStringListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueStrings(out)
	return strings.Join(out, ",")
}

func uniqueStrings(values []string) []string {
	if len(values) <= 1 {
		return values
	}
	out := values[:0]
	var prev string
	for i, value := range values {
		if i == 0 || value != prev {
			out = append(out, value)
			prev = value
		}
	}
	return out
}
