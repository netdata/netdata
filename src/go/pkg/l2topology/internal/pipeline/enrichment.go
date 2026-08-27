// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"maps"
	"net/netip"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
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
	limiter worklimit.Limiter,
	devices map[string]model.Device,
	interfaces map[string]model.Interface,
	enrichments map[string]*enrichmentAccumulator,
	directOwnersByIP map[string]map[string]struct{},
	remoteManagementByDeviceID map[string]map[string]netip.Addr,
	directManagementIPByDeviceID map[string]bool,
) (identityAliasReconcileStats, error) {
	stats := identityAliasReconcileStats{}
	if len(devices) == 0 {
		return stats, nil
	}
	// Direct observation ownership is finalized during registration. Without remote
	// management claims or ARP/ND enrichment there is nothing left to reconcile.
	if len(remoteManagementByDeviceID) == 0 && len(enrichments) == 0 {
		return stats, nil
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
	enrichmentKeys, err := worklimit.SortedStringKeys(limiter, enrichments)
	if err != nil {
		return stats, err
	}

	for _, endpointID := range enrichmentKeys {
		acc := enrichments[endpointID]
		if acc == nil {
			continue
		}
		mac := normalizeMAC(acc.MAC)
		if mac == "" {
			continue
		}
		ipKeys, err := sortedIPKeys(limiter, acc.IPs)
		if err != nil {
			return stats, err
		}
		for _, ipKey := range ipKeys {
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

		ipKeys, err := sortedIPKeys(limiter, acc.IPs)
		if err != nil {
			return stats, err
		}
		for _, ipKey := range ipKeys {
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

	remoteDeviceIDs, err := sortedDeviceClaimIDs(limiter, remoteClaims)
	if err != nil {
		return stats, err
	}
	for _, deviceID := range remoteDeviceIDs {
		device := devices[deviceID]
		addresses := deviceIdentityAddressMap(device)
		ipKeys, err := sortedIPKeys(limiter, remoteClaims[deviceID])
		if err != nil {
			return stats, err
		}
		for _, ip := range ipKeys {
			addr := remoteClaims[deviceID][ip]
			if !claimIsExclusive(deviceID, ip) {
				continue
			}
			addresses[ip] = addr
			if !directManagementIPByDeviceID[deviceID] {
				device.ManagementIP, _ = selectObservedManagementIP(device.ManagementIP, addr, false, false)
			}
		}
		device.Addresses, err = sortedAddrValues(limiter, addresses)
		if err != nil {
			return stats, err
		}
		devices[deviceID] = device
	}

	arpDeviceIDs, err := sortedDeviceClaimIDs(limiter, arpClaims)
	if err != nil {
		return stats, err
	}
	for _, deviceID := range arpDeviceIDs {
		device := devices[deviceID]
		addresses := deviceIdentityAddressMap(device)
		ipKeys, err := sortedIPKeys(limiter, arpClaims[deviceID])
		if err != nil {
			return stats, err
		}
		for _, ip := range ipKeys {
			if !claimIsExclusive(deviceID, ip) {
				stats.ipsConflictSkipped++
				continue
			}
			if _, exists := addresses[ip]; !exists {
				addresses[ip] = arpClaims[deviceID][ip]
				stats.ipsMerged++
			}
		}
		device.Addresses, err = sortedAddrValues(limiter, addresses)
		if err != nil {
			return stats, err
		}
		devices[deviceID] = device
	}

	return stats, nil
}

func sortedDeviceClaimIDs(
	limiter worklimit.Limiter,
	claims map[string]map[string]netip.Addr,
) ([]string, error) {
	return worklimit.SortedStringKeys(limiter, claims)
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

func sortedIPKeys(limiter worklimit.Limiter, in map[string]netip.Addr) ([]string, error) {
	return worklimit.SortedStringKeys(limiter, in)
}
