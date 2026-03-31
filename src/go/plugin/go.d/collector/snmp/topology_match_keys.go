// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func canonicalMatchKey(match topology.Match) string {
	if key := canonicalPrimaryMACListKey(match); key != "" {
		return "mac:" + key
	}
	if key := canonicalHardwareListKey(match.ChassisIDs); key != "" {
		return "chassis:" + key
	}
	if key := canonicalIPListKey(match.IPAddresses); key != "" {
		return "ip:" + key
	}
	if key := canonicalStringListKey(match.Hostnames); key != "" {
		return "hostname:" + key
	}
	if key := canonicalStringListKey(match.DNSNames); key != "" {
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

func canonicalPrimaryMACListKey(match topology.Match) string {
	seen := make(map[string]struct{}, len(match.MacAddresses)+len(match.ChassisIDs))
	for _, value := range match.MacAddresses {
		if mac := normalizeMAC(value); mac != "" {
			seen[mac] = struct{}{}
		}
	}
	for _, value := range match.ChassisIDs {
		if mac := normalizeMAC(value); mac != "" {
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

func topologyMatchIdentityKeys(match topology.Match) []string {
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
		if mac := normalizeMAC(value); mac != "" {
			add("hw", mac)
			continue
		}
		if ip := normalizeIPAddress(value); ip != "" {
			add("ip", ip)
			continue
		}
		add("chassis", strings.ToLower(value))
	}
	for _, value := range match.MacAddresses {
		if mac := normalizeMAC(value); mac != "" {
			add("hw", mac)
		}
	}
	for _, value := range match.IPAddresses {
		if ip := normalizeIPAddress(value); ip != "" {
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

func canonicalHardwareListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mac := normalizeMAC(value); mac != "" {
			out = append(out, mac)
			continue
		}
		if ip := normalizeIPAddress(value); ip != "" {
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

func canonicalMACListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if mac := normalizeMAC(value); mac != "" {
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

func canonicalIPListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if ip := normalizeIPAddress(value); ip != "" {
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

func canonicalStringListKey(values []string) string {
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

func topologyLinkSortKey(link topology.Link) string {
	return strings.Join([]string{
		link.Protocol,
		link.Direction,
		canonicalMatchKey(link.Src.Match),
		canonicalMatchKey(link.Dst.Match),
		attrKey(link.Src.Attributes, "if_index"),
		attrKey(link.Src.Attributes, "if_name"),
		attrKey(link.Src.Attributes, "port_id"),
		attrKey(link.Dst.Attributes, "if_index"),
		attrKey(link.Dst.Attributes, "if_name"),
		attrKey(link.Dst.Attributes, "port_id"),
		link.State,
	}, "|")
}

func attrKey(attrs map[string]any, key string) string {
	if len(attrs) == 0 {
		return ""
	}
	v, ok := attrs[key]
	if !ok || v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
