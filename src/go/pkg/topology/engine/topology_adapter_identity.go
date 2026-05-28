// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

var interfaceNameLookupSanitizer = strings.NewReplacer(
	" ", "",
	"-", "",
	"_", "",
	".", "",
	"\t", "",
	"\n", "",
	"\r", "",
)

func deviceIfNameKey(deviceID, ifName string) string {
	return strings.TrimSpace(deviceID) + keySep + strings.ToLower(strings.TrimSpace(ifName))
}

func interfaceNameLookupAliases(values ...string) []string {
	set := make(map[string]struct{}, len(values)*2)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
		if normalized := normalizeInterfaceNameForLookup(trimmed); normalized != "" && normalized != strings.ToLower(trimmed) {
			set[normalized] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeInterfaceNameForLookup(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	return interfaceNameLookupSanitizer.Replace(value)
}

func resolveIfIndexByPortName(deviceID, port string, ifIndexByDeviceName map[string]int) int {
	deviceID = strings.TrimSpace(deviceID)
	port = strings.TrimSpace(port)
	if deviceID == "" || port == "" {
		return 0
	}
	if idx, ok := ifIndexByDeviceName[deviceIfNameKey(deviceID, port)]; ok && idx > 0 {
		return idx
	}
	if normalized := normalizeInterfaceNameForLookup(port); normalized != "" {
		if idx, ok := ifIndexByDeviceName[deviceIfNameKey(deviceID, normalized)]; ok && idx > 0 {
			return idx
		}
	}
	if parsed, err := strconv.Atoi(port); err == nil && parsed > 0 {
		return parsed
	}
	return 0
}

func topologyIdentityIndexOverlaps(index map[string]struct{}, keys []string) bool {
	if len(index) == 0 || len(keys) == 0 {
		return false
	}
	for _, key := range keys {
		if _, ok := index[key]; ok {
			return true
		}
	}
	return false
}

func addTopologyIdentityKeys(index map[string]struct{}, keys []string) {
	if index == nil || len(keys) == 0 {
		return
	}
	for _, key := range keys {
		index[key] = struct{}{}
	}
}

func buildDeviceIdentityKeySetByID(
	deviceByID map[string]Device,
	adjacencies []Adjacency,
	ifaceByDeviceIndex map[string]Interface,
) map[string]topologyIdentityKeySet {
	if len(deviceByID) == 0 {
		return nil
	}
	out := make(map[string]topologyIdentityKeySet, len(deviceByID))
	for _, device := range deviceByID {
		deviceID := strings.TrimSpace(device.ID)
		if deviceID == "" {
			continue
		}
		keys := topologyMatchIdentityKeys(
			deviceToTopologyActor(device, "", "", "", topologyDeviceInterfaceSummary{}, nil).Match,
		)
		if len(keys) == 0 {
			continue
		}
		set := make(topologyIdentityKeySet, len(keys))
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			set[key] = struct{}{}
		}
		if len(set) == 0 {
			continue
		}
		out[deviceID] = set
	}
	for _, adjacency := range adjacencies {
		protocol := strings.ToLower(strings.TrimSpace(adjacency.Protocol))
		if protocol != "lldp" && protocol != "cdp" {
			continue
		}
		if mac := normalizeMAC(adjacency.SourcePort); mac != "" {
			deviceID := strings.TrimSpace(adjacency.SourceID)
			if deviceID != "" {
				if out[deviceID] == nil {
					out[deviceID] = make(topologyIdentityKeySet)
				}
				out[deviceID]["hw:"+mac] = struct{}{}
			}
		}
		if mac := normalizeMAC(adjacency.TargetPort); mac != "" {
			deviceID := strings.TrimSpace(adjacency.TargetID)
			if deviceID != "" {
				if out[deviceID] == nil {
					out[deviceID] = make(topologyIdentityKeySet)
				}
				out[deviceID]["hw:"+mac] = struct{}{}
			}
		}
	}
	for _, iface := range ifaceByDeviceIndex {
		deviceID := strings.TrimSpace(iface.DeviceID)
		if deviceID == "" {
			continue
		}
		ifaceMAC := normalizeMAC(iface.MAC)
		if ifaceMAC == "" {
			continue
		}
		if out[deviceID] == nil {
			out[deviceID] = make(topologyIdentityKeySet)
		}
		out[deviceID]["hw:"+ifaceMAC] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func topologyMatchIdentityKeys(match topology.Match) []string {
	seen := make(map[string]struct{}, 8)
	add := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := kind + ":" + value
		seen[key] = struct{}{}
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
		if ip := normalizeTopologyIP(value); ip != "" {
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
		if ip := normalizeTopologyIP(value); ip != "" {
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

func topologyMatchHardwareIdentityKeys(match topology.Match) []string {
	seen := make(map[string]struct{}, len(match.MacAddresses)+len(match.ChassisIDs))
	add := func(value string) {
		if mac := normalizeMAC(value); mac != "" {
			seen["hw:"+mac] = struct{}{}
		}
	}

	for _, value := range match.MacAddresses {
		add(value)
	}
	for _, value := range match.ChassisIDs {
		add(value)
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

func endpointMatchOverlappingKnownDeviceIDs(
	endpointMatch topology.Match,
	deviceIdentityByID map[string]topologyIdentityKeySet,
) []string {
	if len(deviceIdentityByID) == 0 {
		return nil
	}

	endpointKeys := topologyMatchHardwareIdentityKeys(endpointMatch)
	if len(endpointKeys) == 0 {
		endpointKeys = topologyMatchIdentityKeys(endpointMatch)
	}
	if len(endpointKeys) == 0 {
		return nil
	}

	deviceIDs := make([]string, 0, len(deviceIdentityByID))
	for deviceID := range deviceIdentityByID {
		deviceID = strings.TrimSpace(deviceID)
		if deviceID == "" {
			continue
		}
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Strings(deviceIDs)
	if len(deviceIDs) == 0 {
		return nil
	}

	matches := make([]string, 0, 2)
	for _, deviceID := range deviceIDs {
		deviceKeys := deviceIdentityByID[deviceID]
		if len(deviceKeys) == 0 {
			continue
		}
		for _, endpointKey := range endpointKeys {
			if _, ok := deviceKeys[endpointKey]; ok {
				matches = append(matches, deviceID)
				break
			}
		}
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}

func normalizeTopologyIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	addr := parseAddr(value)
	if !addr.IsValid() {
		return ""
	}
	return addr.Unmap().String()
}
