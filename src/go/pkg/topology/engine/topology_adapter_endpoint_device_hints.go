// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"
)

func filterManagedDeviceHints(hints []string, managedDeviceIDs map[string]struct{}) []string {
	if len(hints) == 0 || len(managedDeviceIDs) == 0 {
		return hints
	}
	managed := make([]string, 0, len(hints))
	for _, hint := range hints {
		hint = strings.TrimSpace(hint)
		if hint == "" {
			continue
		}
		if _, ok := managedDeviceIDs[hint]; ok {
			managed = append(managed, hint)
		}
	}
	if len(managed) == 0 {
		return nil
	}
	managed = uniqueTopologyStrings(managed)
	sort.Strings(managed)
	return managed
}

func topologyEndpointLabelDeviceIDs(labels map[string]string) []string {
	out := labelsCSVToSlice(labels, "learned_device_ids")
	if len(out) == 0 {
		out = labelsCSVToSlice(labels, "device_ids")
	}
	out = uniqueTopologyStrings(out)
	sort.Strings(out)
	return out
}

func resolveTopologyEndpointDeviceHints(
	hints []string,
	aliasOwnerIDs map[string]map[string]struct{},
) []string {
	set := make(map[string]struct{})
	for _, hint := range hints {
		hint = strings.TrimSpace(hint)
		if hint == "" {
			continue
		}
		if alias := normalizeTopologyEndpointDeviceAlias(hint); alias != "" {
			if owners := aliasOwnerIDs[alias]; len(owners) > 0 {
				for ownerID := range owners {
					ownerID = strings.TrimSpace(ownerID)
					if ownerID == "" {
						continue
					}
					set[ownerID] = struct{}{}
				}
				continue
			}
		}
		set[hint] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	return sortedTopologySet(set)
}

func normalizeTopologyEndpointDeviceAlias(hint string) string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return ""
	}
	if alias := normalizeFDBEndpointID(hint); alias != "" {
		return alias
	}
	lower := strings.ToLower(hint)
	if strings.HasPrefix(lower, "macaddress:") {
		if mac := normalizeMAC(hint[len("macAddress:"):]); mac != "" {
			return "mac:" + mac
		}
	}
	return ""
}
