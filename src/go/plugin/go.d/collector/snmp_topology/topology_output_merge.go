// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"
)

func mergeTopologyMatch(base, other topologyMatch) topologyMatch {
	base.ChassisIDs = appendUniqueTopologyStrings(base.ChassisIDs, other.ChassisIDs...)
	base.MacAddresses = appendUniqueTopologyStrings(base.MacAddresses, other.MacAddresses...)
	base.IPAddresses = appendUniqueTopologyStrings(base.IPAddresses, other.IPAddresses...)
	base.Hostnames = appendUniqueTopologyStrings(base.Hostnames, other.Hostnames...)
	base.DNSNames = appendUniqueTopologyStrings(base.DNSNames, other.DNSNames...)
	if strings.TrimSpace(base.SysName) == "" {
		base.SysName = strings.TrimSpace(other.SysName)
	}
	if strings.TrimSpace(base.SysObjectID) == "" {
		base.SysObjectID = strings.TrimSpace(other.SysObjectID)
	}
	return base
}

func mergeTopologyStringMap(base, other map[string]string) map[string]string {
	if len(other) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]string, len(other))
	}
	for key, value := range other {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		if _, exists := base[key]; exists {
			continue
		}
		base[key] = value
	}
	return base
}

func mergeTopologyAnyMap(base, other map[string]any) map[string]any {
	if len(other) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]any, len(other))
	}
	for key, value := range other {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := base[key]; exists {
			continue
		}
		base[key] = value
	}
	return base
}

func appendUniqueTopologyStrings(base []string, values ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(values))
	out := make([]string, 0, len(base)+len(values))
	for _, value := range append(base, values...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
