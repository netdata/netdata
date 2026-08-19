// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"fmt"
	"maps"
	"net/netip"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

func (s *l2BuildState) registerObservations(observations []model.L2Observation) error {
	for _, obs := range observations {
		if err := s.registerObservation(obs); err != nil {
			return err
		}
	}
	s.finalizeDirectIPIndex()
	return nil
}

func (s *l2BuildState) registerObservation(obs model.L2Observation) error {
	deviceID := strings.TrimSpace(obs.DeviceID)
	if deviceID == "" {
		return fmt.Errorf("observation with empty device id")
	}

	device := model.Device{
		ID:        deviceID,
		Hostname:  strings.TrimSpace(obs.Hostname),
		SysObject: strings.TrimSpace(obs.SysObjectID),
		ChassisID: strings.TrimSpace(obs.ChassisID),
		Labels:    mergeObservedDeviceLabels(nil, obs.Labels),
	}
	if primaryMAC := primaryL2MACIdentity(obs.ChassisID, obs.BaseBridgeAddress); primaryMAC != "" {
		device.ChassisID = primaryMAC
	}
	incomingManaged := !obs.Inferred
	existingManagementIPDirect := s.directManagementIPByDeviceID[deviceID]
	if device.Hostname == "" {
		device.Hostname = device.ID
	}
	managementAddr := canonicalUsableIPAddress(obs.ManagementIP)
	managementAliases := canonicalManagementAliases(obs.ManagementAliases)
	if incomingManaged {
		addresses := make(map[string]netip.Addr, 1+len(managementAliases))
		for _, addr := range managementAliases {
			addresses[addr.String()] = addr
		}
		if managementAddr.IsValid() {
			device.ManagementIP = managementAddr
			addresses[managementAddr.String()] = managementAddr
		}
		device.Addresses = sortedAddrValues(addresses)
	}
	selectedManagementIPDirect := incomingManaged && device.ManagementIP.IsValid()
	if len(device.Labels) == 0 {
		device.Labels = make(map[string]string)
	}
	observedProtocols := observationProtocolsUsed(obs)
	if existing, ok := s.devices[device.ID]; ok {
		selectedManagementIP, selectedDirect := selectObservedManagementIP(
			existing.ManagementIP,
			device.ManagementIP,
			existingManagementIPDirect,
			selectedManagementIPDirect,
		)
		device = mergeObservedDevice(existing, device)
		device.ManagementIP = selectedManagementIP
		selectedManagementIPDirect = selectedDirect
		if device.Labels == nil {
			device.Labels = make(map[string]string)
		}
		for protocol := range csvToTopologySet(existing.Labels["protocols_observed"]) {
			observedProtocols[protocol] = struct{}{}
		}
	}
	if incomingManaged {
		s.managedObservationByDeviceID[deviceID] = true
	}
	if selectedManagementIPDirect {
		s.directManagementIPByDeviceID[deviceID] = true
	} else {
		delete(s.directManagementIPByDeviceID, deviceID)
	}
	if len(observedProtocols) > 0 {
		device.Labels["protocols_observed"] = setToCSV(observedProtocols)
	}
	s.devices[device.ID] = device
	if incomingManaged {
		if managementAddr.IsValid() {
			s.recordDirectManagementAddress(device.ID, managementAddr, true)
		}
		for _, addr := range managementAliases {
			s.recordDirectManagementAddress(device.ID, addr, false)
		}
	} else {
		if managementAddr.IsValid() {
			s.recordRemoteManagementAddress(device.ID, managementAddr.String())
		}
		for _, addr := range managementAliases {
			s.recordRemoteManagementAddress(device.ID, addr.String())
		}
	}

	if host := canonicalHost(device.Hostname); host != "" {
		s.hostToID[host] = device.ID
	}
	if mac := primaryL2MACIdentity(device.ChassisID, ""); mac != "" {
		if _, exists := s.macToID[mac]; !exists {
			s.macToID[mac] = device.ID
		}
		s.chassisToID[canonicalToken(mac)] = device.ID
	} else if chassis := canonicalToken(device.ChassisID); chassis != "" {
		s.chassisToID[chassis] = device.ID
	}
	if bridgeAddr := canonicalBridgeAddr(obs.BaseBridgeAddress, device.ChassisID); bridgeAddr != "" {
		if _, exists := s.bridgeAddrToID[bridgeAddr]; !exists {
			s.bridgeAddrToID[bridgeAddr] = device.ID
		}
	}

	for _, iface := range obs.Interfaces {
		if iface.IfIndex <= 0 {
			continue
		}
		ifName := strings.TrimSpace(iface.IfName)
		ifDescr := strings.TrimSpace(iface.IfDescr)
		if ifName == "" {
			ifName = ifDescr
		}
		if ifDescr == "" {
			ifDescr = ifName
		}
		if ifName == "" {
			continue
		}
		engIface := model.Interface{
			DeviceID: device.ID,
			IfIndex:  iface.IfIndex,
			IfName:   ifName,
			IfDescr:  ifDescr,
			MAC:      normalizeMAC(iface.MAC),
		}
		if ifType := strings.TrimSpace(iface.InterfaceType); ifType != "" {
			if engIface.Labels == nil {
				engIface.Labels = make(map[string]string)
			}
			engIface.Labels["if_type"] = ifType
		}
		if admin := strings.TrimSpace(iface.AdminStatus); admin != "" {
			if engIface.Labels == nil {
				engIface.Labels = make(map[string]string)
			}
			engIface.Labels["admin_status"] = admin
		}
		if oper := strings.TrimSpace(iface.OperStatus); oper != "" {
			if engIface.Labels == nil {
				engIface.Labels = make(map[string]string)
			}
			engIface.Labels["oper_status"] = oper
		}
		if ifAlias := strings.TrimSpace(iface.IfAlias); ifAlias != "" {
			if engIface.Labels == nil {
				engIface.Labels = make(map[string]string)
			}
			engIface.Labels["if_alias"] = ifAlias
		}
		if iface.SpeedBps > 0 {
			if engIface.Labels == nil {
				engIface.Labels = make(map[string]string)
			}
			engIface.Labels["speed_bps"] = strconv.FormatInt(iface.SpeedBps, 10)
		}
		if iface.LastChange > 0 {
			if engIface.Labels == nil {
				engIface.Labels = make(map[string]string)
			}
			engIface.Labels["last_change"] = strconv.FormatInt(iface.LastChange, 10)
		}
		if duplex := strings.TrimSpace(iface.Duplex); duplex != "" {
			if engIface.Labels == nil {
				engIface.Labels = make(map[string]string)
			}
			engIface.Labels["duplex"] = duplex
		}
		if engIface.MAC != "" {
			if engIface.Labels == nil {
				engIface.Labels = make(map[string]string)
			}
			engIface.Labels["mac"] = engIface.MAC
		}
		s.interfaces[ifaceKey(engIface)] = engIface
		s.ifNameByDeviceIfIndex[deviceIfIndexKey(device.ID, iface.IfIndex)] = ifName
	}

	return nil
}

func canonicalManagementAliases(values []string) []netip.Addr {
	aliases := make(map[string]netip.Addr, len(values))
	for _, value := range values {
		addr := canonicalUsableIPAddress(value)
		if !addr.IsValid() {
			continue
		}
		aliases[addr.String()] = addr
	}
	return sortedAddrValues(aliases)
}

func (s *l2BuildState) recordDirectManagementAddress(deviceID string, addr netip.Addr, selectedPrimary bool) {
	deviceID = strings.TrimSpace(deviceID)
	addr = addr.Unmap()
	if deviceID == "" || !isUsableAliasIPAddress(addr) {
		return
	}

	claims := s.directAddressClaimsByIP[addr.String()]
	if claims == nil {
		claims = &directAddressClaims{
			primaryOwners: make(map[string]struct{}),
			aliasOwners:   make(map[string]struct{}),
		}
		s.directAddressClaimsByIP[addr.String()] = claims
	}
	if selectedPrimary {
		claims.primaryOwners[deviceID] = struct{}{}
	} else {
		claims.aliasOwners[deviceID] = struct{}{}
	}
}

func (s *l2BuildState) finalizeDirectIPIndex() {
	s.directIPToID = make(map[string]string, len(s.directAddressClaimsByIP))
	s.directOwnersByIP = make(map[string]map[string]struct{}, len(s.directAddressClaimsByIP))
	for ip, claims := range s.directAddressClaimsByIP {
		owners := make(map[string]struct{}, len(claims.primaryOwners)+len(claims.aliasOwners))
		maps.Copy(owners, claims.primaryOwners)
		maps.Copy(owners, claims.aliasOwners)
		s.directOwnersByIP[ip] = owners

		if len(claims.primaryOwners) == 1 {
			for deviceID := range claims.primaryOwners {
				s.directIPToID[ip] = deviceID
			}
		} else if len(claims.primaryOwners) == 0 && len(claims.aliasOwners) == 1 {
			for deviceID := range claims.aliasOwners {
				s.directIPToID[ip] = deviceID
			}
		}
	}

	for deviceID, device := range s.devices {
		addresses := make(map[string]netip.Addr, len(device.Addresses))
		for _, addr := range device.Addresses {
			addr = addr.Unmap()
			if !addr.IsValid() || !s.keepDirectManagementAddress(deviceID, addr.String()) {
				continue
			}
			addresses[addr.String()] = addr
		}
		device.Addresses = sortedAddrValues(addresses)
		s.devices[deviceID] = device
	}
	s.directAddressClaimsByIP = nil
}

func (s *l2BuildState) keepDirectManagementAddress(deviceID, ip string) bool {
	claims := s.directAddressClaimsByIP[ip]
	if claims == nil {
		return true
	}
	if _, primary := claims.primaryOwners[deviceID]; primary {
		return true
	}
	if len(claims.primaryOwners) > 0 {
		return false
	}
	if len(claims.aliasOwners) != 1 {
		return false
	}
	_, owned := claims.aliasOwners[deviceID]
	return owned
}

func mergeObservedDevice(existing, incoming model.Device) model.Device {
	out := existing
	if strings.TrimSpace(out.ID) == "" {
		out.ID = incoming.ID
	}
	if strings.TrimSpace(incoming.Hostname) != "" && (strings.TrimSpace(out.Hostname) == "" || out.Hostname == out.ID) {
		out.Hostname = incoming.Hostname
	}
	if strings.TrimSpace(out.SysObject) == "" {
		out.SysObject = incoming.SysObject
	}
	if strings.TrimSpace(out.ChassisID) == "" {
		out.ChassisID = incoming.ChassisID
	}
	out.Addresses = mergeObservedDeviceAddresses(existing.Addresses, incoming.Addresses)
	out.Labels = mergeObservedDeviceLabels(existing.Labels, incoming.Labels)
	if strings.TrimSpace(out.Hostname) == "" {
		out.Hostname = out.ID
	}
	return out
}

func selectObservedManagementIP(existing, incoming netip.Addr, existingDirect, incomingDirect bool) (netip.Addr, bool) {
	existing = existing.Unmap()
	incoming = incoming.Unmap()
	if !existing.IsValid() {
		return incoming, incoming.IsValid() && incomingDirect
	}
	if !incoming.IsValid() {
		return existing, existingDirect
	}
	if existingDirect != incomingDirect {
		if incomingDirect {
			return incoming, true
		}
		return existing, true
	}
	if !existingDirect && existing.IsPrivate() != incoming.IsPrivate() {
		if incoming.IsPrivate() {
			return incoming, false
		}
		return existing, false
	}
	if incoming.Compare(existing) < 0 {
		return incoming, incomingDirect
	}
	return existing, existingDirect
}

func mergeObservedDeviceAddresses(existing, incoming []netip.Addr) []netip.Addr {
	if len(existing) == 0 && len(incoming) == 0 {
		return nil
	}
	merged := make(map[string]netip.Addr, len(existing)+len(incoming))
	for _, addr := range existing {
		if addr.IsValid() {
			merged[addr.String()] = addr
		}
	}
	for _, addr := range incoming {
		if addr.IsValid() {
			merged[addr.String()] = addr
		}
	}
	return sortedAddrValues(merged)
}

func mergeObservedDeviceLabels(existing, incoming map[string]string) map[string]string {
	if len(existing) == 0 && len(incoming) == 0 {
		return nil
	}
	out := make(map[string]string, len(existing)+len(incoming))
	for key, value := range existing {
		if value != "" {
			out[key] = value
		}
	}
	for key, value := range incoming {
		if value == "" {
			continue
		}
		if strings.TrimSpace(out[key]) == "" {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
