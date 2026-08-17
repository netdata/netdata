// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"maps"
	"net/netip"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
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
	devices map[string]model.Device,
	interfaces map[string]model.Interface,
	enrichments map[string]*enrichmentAccumulator,
	directOwnersByIP map[string]map[string]struct{},
	remoteManagementByDeviceID map[string]map[string]netip.Addr,
	directManagementIPByDeviceID map[string]bool,
) identityAliasReconcileStats {
	stats := identityAliasReconcileStats{}
	if len(devices) == 0 {
		return stats
	}

	ownersByIP := make(map[string]map[string]struct{}, len(directOwnersByIP))
	for ip, owners := range directOwnersByIP {
		copied := make(map[string]struct{}, len(owners))
		maps.Copy(copied, owners)
		ownersByIP[ip] = copied
	}
	addOwner := func(deviceID string, addr netip.Addr) {
		deviceID = strings.TrimSpace(deviceID)
		addr = addr.Unmap()
		if deviceID == "" || !addr.IsValid() {
			return
		}
		owners := ownersByIP[addr.String()]
		if owners == nil {
			owners = make(map[string]struct{})
			ownersByIP[addr.String()] = owners
		}
		owners[deviceID] = struct{}{}
	}
	for deviceID, device := range devices {
		addOwner(deviceID, device.ManagementIP)
		for _, addr := range device.Addresses {
			addOwner(deviceID, addr)
		}
	}

	remoteClaims := make(map[string]map[string]netip.Addr)
	for deviceID, candidates := range remoteManagementByDeviceID {
		if _, ok := devices[deviceID]; !ok {
			continue
		}
		for _, addr := range candidates {
			if !isUsableAliasIPAddress(addr) {
				continue
			}
			addr = addr.Unmap()
			claims := remoteClaims[deviceID]
			if claims == nil {
				claims = make(map[string]netip.Addr)
				remoteClaims[deviceID] = claims
			}
			claims[addr.String()] = addr
			addOwner(deviceID, addr)
		}
	}

	uniqueMACToDeviceID, ambiguousMACs := buildUniqueMACToDeviceIndex(devices, interfaces)
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

	conflictingIPs := make(map[string]struct{})
	for ip, macs := range ipToMACs {
		if len(macs) > 1 {
			conflictingIPs[ip] = struct{}{}
		}
	}

	arpClaims := make(map[string]map[string]netip.Addr)
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

		for _, ipKey := range sortedIPKeys(acc.IPs) {
			addr, ok := acc.IPs[ipKey]
			if !ok || !isUsableAliasIPAddress(addr) {
				continue
			}
			if len(ipToMACs[ipKey]) > 1 {
				stats.ipsConflictSkipped++
				continue
			}
			addr = addr.Unmap()
			claims := arpClaims[deviceID]
			if claims == nil {
				claims = make(map[string]netip.Addr)
				arpClaims[deviceID] = claims
			}
			claims[addr.String()] = addr
			addOwner(deviceID, addr)
		}
	}

	claimIsExclusive := func(deviceID, ip string) bool {
		if _, conflict := conflictingIPs[ip]; conflict {
			return false
		}
		for owner := range ownersByIP[ip] {
			if owner != deviceID {
				return false
			}
		}
		return true
	}

	for _, deviceID := range sortedDeviceClaimIDs(remoteClaims) {
		device := devices[deviceID]
		addresses := deviceIdentityAddressMap(device)
		for _, ip := range sortedIPKeys(remoteClaims[deviceID]) {
			addr := remoteClaims[deviceID][ip]
			if !claimIsExclusive(deviceID, ip) {
				continue
			}
			addresses[ip] = addr
			if !directManagementIPByDeviceID[deviceID] {
				device.ManagementIP, _ = selectObservedManagementIP(device.ManagementIP, addr, false, false)
			}
		}
		device.Addresses = sortedAddrValues(addresses)
		devices[deviceID] = device
	}

	for _, deviceID := range sortedDeviceClaimIDs(arpClaims) {
		device := devices[deviceID]
		addresses := deviceIdentityAddressMap(device)
		for _, ip := range sortedIPKeys(arpClaims[deviceID]) {
			if !claimIsExclusive(deviceID, ip) {
				stats.ipsConflictSkipped++
				continue
			}
			if _, exists := addresses[ip]; !exists {
				addresses[ip] = arpClaims[deviceID][ip]
				stats.ipsMerged++
			}
		}
		device.Addresses = sortedAddrValues(addresses)
		devices[deviceID] = device
	}

	return stats
}

func sortedDeviceClaimIDs(claims map[string]map[string]netip.Addr) []string {
	if len(claims) == 0 {
		return nil
	}
	ids := make([]string, 0, len(claims))
	for deviceID := range claims {
		ids = append(ids, deviceID)
	}
	sort.Strings(ids)
	return ids
}

func deviceIdentityAddressMap(device model.Device) map[string]netip.Addr {
	addresses := make(map[string]netip.Addr, len(device.Addresses))
	for _, existing := range device.Addresses {
		existing = existing.Unmap()
		if existing.IsValid() {
			addresses[existing.String()] = existing
		}
	}
	return addresses
}

func buildUniqueMACToDeviceIndex(
	devices map[string]model.Device,
	interfaces map[string]model.Interface,
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
	if !addr.IsValid() || addr.Zone() != "" {
		return false
	}
	if addr.IsUnspecified() || addr.IsLoopback() || addr.IsLinkLocalUnicast() || addr.IsMulticast() {
		return false
	}
	return !addr.Is4() || addr != netip.AddrFrom4([4]byte{255, 255, 255, 255})
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
