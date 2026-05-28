// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"maps"
	"net/netip"
	"sort"
	"strings"
)

type enrichmentAccumulator struct {
	EndpointID string
	MAC        string
	IPs        map[string]netip.Addr
	Protocols  map[string]struct{}
	DeviceIDs  map[string]struct{}
	IfIndexes  map[string]struct{}
	IfNames    map[string]struct{}
	States     map[string]struct{}
	AddrTypes  map[string]struct{}
}

func ensureEnrichmentAccumulator(enrichments map[string]*enrichmentAccumulator, endpointID string) *enrichmentAccumulator {
	acc := enrichments[endpointID]
	if acc != nil {
		return acc
	}
	acc = &enrichmentAccumulator{
		EndpointID: endpointID,
		IPs:        make(map[string]netip.Addr),
		Protocols:  make(map[string]struct{}),
		DeviceIDs:  make(map[string]struct{}),
		IfIndexes:  make(map[string]struct{}),
		IfNames:    make(map[string]struct{}),
		States:     make(map[string]struct{}),
		AddrTypes:  make(map[string]struct{}),
	}
	enrichments[endpointID] = acc
	return acc
}

func mergeEnrichmentAccumulator(target, source *enrichmentAccumulator) {
	if target == nil || source == nil || target == source {
		return
	}
	if target.MAC == "" {
		target.MAC = source.MAC
	}
	maps.Copy(target.IPs, source.IPs)
	for key := range source.Protocols {
		target.Protocols[key] = struct{}{}
	}
	for key := range source.DeviceIDs {
		target.DeviceIDs[key] = struct{}{}
	}
	for key := range source.IfIndexes {
		target.IfIndexes[key] = struct{}{}
	}
	for key := range source.IfNames {
		target.IfNames[key] = struct{}{}
	}
	for key := range source.States {
		target.States[key] = struct{}{}
	}
	for key := range source.AddrTypes {
		target.AddrTypes[key] = struct{}{}
	}
}

type identityAliasReconcileStats struct {
	endpointsMapped       int
	endpointsAmbiguousMAC int
	ipsMerged             int
	ipsConflictSkipped    int
}

func reconcileDeviceIdentityAliases(
	devices map[string]Device,
	interfaces map[string]Interface,
	enrichments map[string]*enrichmentAccumulator,
) identityAliasReconcileStats {
	stats := identityAliasReconcileStats{}
	if len(devices) == 0 || len(enrichments) == 0 {
		return stats
	}

	uniqueMACToDeviceID, ambiguousMACs := buildUniqueMACToDeviceIndex(devices, interfaces)
	if len(uniqueMACToDeviceID) == 0 {
		return stats
	}

	ipToMACs := make(map[string]map[string]struct{})
	enrichmentKeys := make([]string, 0, len(enrichments))
	for endpointID := range enrichments {
		enrichmentKeys = append(enrichmentKeys, endpointID)
	}
	sort.Strings(enrichmentKeys)

	for _, endpointID := range enrichmentKeys {
		acc := enrichments[endpointID]
		if acc == nil {
			continue
		}
		mac := normalizeMAC(acc.MAC)
		if mac == "" {
			continue
		}
		for _, ipKey := range sortedIPKeys(acc.IPs) {
			addr, ok := acc.IPs[ipKey]
			if !ok || !isUsableAliasIPAddress(addr) {
				continue
			}
			owners := ipToMACs[ipKey]
			if owners == nil {
				owners = make(map[string]struct{})
				ipToMACs[ipKey] = owners
			}
			owners[mac] = struct{}{}
		}
	}

	aliasIPsByDevice := make(map[string]map[string]netip.Addr)
	for _, endpointID := range enrichmentKeys {
		acc := enrichments[endpointID]
		if acc == nil {
			continue
		}
		mac := normalizeMAC(acc.MAC)
		if mac == "" {
			continue
		}
		if _, ambiguous := ambiguousMACs[mac]; ambiguous {
			stats.endpointsAmbiguousMAC++
			continue
		}

		deviceID := strings.TrimSpace(uniqueMACToDeviceID[mac])
		if deviceID == "" {
			continue
		}
		stats.endpointsMapped++

		if aliasIPsByDevice[deviceID] == nil {
			aliasIPsByDevice[deviceID] = make(map[string]netip.Addr)
		}
		for _, ipKey := range sortedIPKeys(acc.IPs) {
			addr, ok := acc.IPs[ipKey]
			if !ok || !isUsableAliasIPAddress(addr) {
				continue
			}
			if len(ipToMACs[ipKey]) > 1 {
				stats.ipsConflictSkipped++
				continue
			}
			aliasIPsByDevice[deviceID][addr.String()] = addr.Unmap()
		}
	}

	for deviceID, aliasIPs := range aliasIPsByDevice {
		device, ok := devices[deviceID]
		if !ok || len(aliasIPs) == 0 {
			continue
		}

		merged := make(map[string]netip.Addr, len(device.Addresses)+len(aliasIPs))
		for _, addr := range device.Addresses {
			if !isUsableAliasIPAddress(addr) {
				continue
			}
			normalized := addr.Unmap()
			merged[normalized.String()] = normalized
		}
		before := len(merged)
		maps.Copy(merged, aliasIPs)
		added := len(merged) - before
		if added <= 0 {
			continue
		}
		stats.ipsMerged += added

		keys := make([]string, 0, len(merged))
		for key := range merged {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		addresses := make([]netip.Addr, 0, len(keys))
		for _, key := range keys {
			addresses = append(addresses, merged[key])
		}
		device.Addresses = addresses
		devices[deviceID] = device
	}

	return stats
}

func buildUniqueMACToDeviceIndex(
	devices map[string]Device,
	interfaces map[string]Interface,
) (map[string]string, map[string]struct{}) {
	ownersByMAC := make(map[string]map[string]struct{})
	addOwner := func(mac, deviceID string) {
		mac = normalizeMAC(mac)
		deviceID = strings.TrimSpace(deviceID)
		if mac == "" || deviceID == "" {
			return
		}
		owners := ownersByMAC[mac]
		if owners == nil {
			owners = make(map[string]struct{})
			ownersByMAC[mac] = owners
		}
		owners[deviceID] = struct{}{}
	}

	for _, device := range devices {
		addOwner(primaryL2MACIdentity(device.ChassisID, ""), device.ID)
	}
	for _, iface := range interfaces {
		addOwner(iface.MAC, iface.DeviceID)
	}

	unique := make(map[string]string, len(ownersByMAC))
	ambiguous := make(map[string]struct{})
	for mac, owners := range ownersByMAC {
		if len(owners) == 1 {
			for deviceID := range owners {
				unique[mac] = deviceID
			}
			continue
		}
		ambiguous[mac] = struct{}{}
	}
	return unique, ambiguous
}

func isUsableAliasIPAddress(addr netip.Addr) bool {
	addr = addr.Unmap()
	if !addr.IsValid() {
		return false
	}
	return !addr.IsUnspecified()
}

func sortedIPKeys(in map[string]netip.Addr) []string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
