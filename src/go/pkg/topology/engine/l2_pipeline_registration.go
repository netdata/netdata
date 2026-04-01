// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

func (s *l2BuildState) registerObservations(observations []L2Observation) error {
	for _, obs := range observations {
		if err := s.registerObservation(obs); err != nil {
			return err
		}
	}
	return nil
}

func (s *l2BuildState) registerObservation(obs L2Observation) error {
	deviceID := strings.TrimSpace(obs.DeviceID)
	if deviceID == "" {
		return fmt.Errorf("observation with empty device id")
	}

	device := Device{
		ID:        deviceID,
		Hostname:  strings.TrimSpace(obs.Hostname),
		SysObject: strings.TrimSpace(obs.SysObjectID),
		ChassisID: strings.TrimSpace(obs.ChassisID),
	}
	if primaryMAC := primaryL2MACIdentity(obs.ChassisID, obs.BaseBridgeAddress); primaryMAC != "" {
		device.ChassisID = primaryMAC
	}
	if !obs.Inferred {
		s.managedObservationByDeviceID[deviceID] = true
	}
	if device.Hostname == "" {
		device.Hostname = device.ID
	}
	if addr := parseAddr(obs.ManagementIP); addr.IsValid() {
		device.Addresses = []netip.Addr{addr}
	}
	if len(device.Labels) == 0 {
		device.Labels = make(map[string]string)
	}
	observedProtocols := observationProtocolsUsed(obs)
	if existing, ok := s.devices[device.ID]; ok {
		for protocol := range csvToTopologySet(existing.Labels["protocols_observed"]) {
			observedProtocols[protocol] = struct{}{}
		}
	}
	if len(observedProtocols) > 0 {
		device.Labels["protocols_observed"] = setToCSV(observedProtocols)
	}
	s.devices[device.ID] = device

	if host := canonicalHost(device.Hostname); host != "" {
		s.hostToID[host] = device.ID
	}
	if ip := canonicalIP(obs.ManagementIP); ip != "" {
		s.ipToID[ip] = device.ID
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
		engIface := Interface{
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
