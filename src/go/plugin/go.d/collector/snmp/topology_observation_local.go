// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"sort"
	"strconv"
	"strings"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

func (c *topologyCache) buildEngineObservation(local topologyDevice) topologyengine.L2Observation {
	localManagementIP := normalizeIPAddress(local.ManagementIP)
	if localManagementIP == "" {
		localManagementIP = pickManagementIP(local.ManagementAddresses)
	}

	baseBridgeAddress := c.resolveLocalBaseBridgeAddress(localManagementIP)
	if baseBridgeAddress != "" && normalizeMAC(local.ChassisID) == "" {
		local.ChassisID = baseBridgeAddress
		local.ChassisIDType = "macAddress"
	}

	observation := topologyengine.L2Observation{
		DeviceID:          ensureTopologyObservationDeviceID(local, baseBridgeAddress),
		Hostname:          strings.TrimSpace(local.SysName),
		ManagementIP:      localManagementIP,
		SysObjectID:       strings.TrimSpace(local.SysObjectID),
		ChassisID:         strings.TrimSpace(local.ChassisID),
		BaseBridgeAddress: baseBridgeAddress,
	}
	if observation.BaseBridgeAddress == "" {
		observation.BaseBridgeAddress = stpBridgeAddressToMAC(observation.ChassisID)
	}

	c.appendObservedInterfaces(&observation)
	c.appendObservedBridgePorts(&observation)
	c.appendObservedFDBEntries(&observation)
	c.appendObservedSTPPorts(&observation)
	c.appendObservedARPNDEntries(&observation)
	c.appendObservedLLDPRemotes(&observation)
	c.appendObservedCDPRemotes(&observation)

	return observation
}

func (c *topologyCache) resolveLocalBaseBridgeAddress(localManagementIP string) string {
	baseBridgeAddress := strings.TrimSpace(c.stpBaseBridgeAddress)
	if baseBridgeAddress == "" {
		baseBridgeAddress = c.deriveLocalBridgeMACFromFDBSelfEntries()
	}
	if baseBridgeAddress == "" {
		baseBridgeAddress = c.deriveLocalBridgeMACFromInterfacePhysAddress(localManagementIP)
	}
	return baseBridgeAddress
}

func (c *topologyCache) appendObservedInterfaces(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	ifaceKeys := make(map[string]struct{}, len(c.ifNamesByIndex)+len(c.ifStatusByIndex))
	for key := range c.ifNamesByIndex {
		ifaceKeys[key] = struct{}{}
	}
	for key := range c.ifStatusByIndex {
		ifaceKeys[key] = struct{}{}
	}

	ifaceKeyList := make([]string, 0, len(ifaceKeys))
	for key := range ifaceKeys {
		ifaceKeyList = append(ifaceKeyList, key)
	}
	sort.Strings(ifaceKeyList)

	for _, ifIndex := range ifaceKeyList {
		idx := parseIndex(ifIndex)
		if idx <= 0 {
			continue
		}
		ifName := strings.TrimSpace(c.ifNamesByIndex[ifIndex])
		if ifName == "" {
			ifName = ifIndex
		}
		status := c.ifStatusByIndex[ifIndex]
		ifDescr := strings.TrimSpace(status.ifDescr)
		if ifDescr == "" {
			ifDescr = ifName
		}
		observation.Interfaces = append(observation.Interfaces, topologyengine.ObservedInterface{
			IfIndex:       idx,
			IfName:        ifName,
			IfDescr:       ifDescr,
			IfAlias:       strings.TrimSpace(status.ifAlias),
			MAC:           strings.TrimSpace(status.mac),
			SpeedBps:      status.speedBps,
			LastChange:    status.lastChange,
			Duplex:        strings.TrimSpace(status.duplex),
			InterfaceType: strings.TrimSpace(status.ifType),
			AdminStatus:   strings.TrimSpace(status.admin),
			OperStatus:    strings.TrimSpace(status.oper),
		})
	}
}

func (c *topologyCache) appendObservedBridgePorts(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	keys := make([]string, 0, len(c.bridgePortToIf))
	for key := range c.bridgePortToIf {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, basePort := range keys {
		ifIndex := parseIndex(c.bridgePortToIf[basePort])
		if ifIndex <= 0 {
			continue
		}
		observation.BridgePorts = append(observation.BridgePorts, topologyengine.BridgePortObservation{
			BasePort: strings.TrimSpace(basePort),
			IfIndex:  ifIndex,
		})
	}
}

func (c *topologyCache) appendObservedFDBEntries(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	keys := make([]string, 0, len(c.fdbEntries))
	for key := range c.fdbEntries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		entry := c.fdbEntries[key]
		if entry == nil || strings.TrimSpace(entry.mac) == "" {
			continue
		}
		ifIndex := parseIndex(c.bridgePortToIf[strings.TrimSpace(entry.bridgePort)])
		vlanID := strings.TrimSpace(entry.vlanID)
		if vlanID == "" && strings.TrimSpace(entry.fdbID) != "" {
			vlanID = strings.TrimSpace(c.fdbIDToVlanID[strings.TrimSpace(entry.fdbID)])
		}
		observation.FDBEntries = append(observation.FDBEntries, topologyengine.FDBObservation{
			MAC:        strings.TrimSpace(entry.mac),
			BridgePort: strings.TrimSpace(entry.bridgePort),
			IfIndex:    ifIndex,
			Status:     strings.TrimSpace(entry.status),
			VLANID:     vlanID,
			VLANName:   strings.TrimSpace(entry.vlanName),
		})
	}
}

func (c *topologyCache) appendObservedSTPPorts(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	keys := make([]string, 0, len(c.stpPorts))
	for key := range c.stpPorts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		entry := c.stpPorts[key]
		if entry == nil {
			continue
		}
		port := strings.TrimSpace(entry.port)
		if port == "" {
			continue
		}
		ifIndex := parseIndex(c.bridgePortToIf[port])
		ifName := ""
		if ifIndex > 0 {
			ifName = strings.TrimSpace(c.ifNamesByIndex[strconv.Itoa(ifIndex)])
		}
		observation.STPPorts = append(observation.STPPorts, topologyengine.STPPortObservation{
			Port:             port,
			IfIndex:          ifIndex,
			IfName:           ifName,
			VLANID:           strings.TrimSpace(entry.vlanID),
			VLANName:         strings.TrimSpace(entry.vlanName),
			State:            strings.TrimSpace(entry.state),
			Enable:           strings.TrimSpace(entry.enable),
			PathCost:         strings.TrimSpace(entry.pathCost),
			DesignatedRoot:   strings.TrimSpace(entry.designatedRoot),
			DesignatedBridge: strings.TrimSpace(entry.designatedBridge),
			DesignatedPort:   strings.TrimSpace(entry.designatedPort),
		})
	}
}

func (c *topologyCache) appendObservedARPNDEntries(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	keys := make([]string, 0, len(c.arpEntries))
	for key := range c.arpEntries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		entry := c.arpEntries[key]
		if entry == nil {
			continue
		}
		ifName := strings.TrimSpace(entry.ifName)
		if ifName == "" && strings.TrimSpace(entry.ifIndex) != "" {
			ifName = strings.TrimSpace(c.ifNamesByIndex[entry.ifIndex])
		}
		observation.ARPNDEntries = append(observation.ARPNDEntries, topologyengine.ARPNDObservation{
			Protocol: "arp",
			IfIndex:  parseIndex(entry.ifIndex),
			IfName:   ifName,
			IP:       strings.TrimSpace(entry.ip),
			MAC:      strings.TrimSpace(entry.mac),
			State:    strings.TrimSpace(entry.state),
			AddrType: strings.TrimSpace(entry.addrType),
		})
	}
}

func (c *topologyCache) appendObservedLLDPRemotes(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	keys := make([]string, 0, len(c.lldpRemotes))
	for key := range c.lldpRemotes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		remote := c.lldpRemotes[key]
		if remote == nil {
			continue
		}

		managementIP := normalizeIPAddress(remote.managementAddr)
		if managementIP == "" {
			managementIP = pickManagementIP(remote.managementAddrs)
		}

		localPort := c.lldpLocPorts[remote.localPortNum]
		localPortID := ""
		localPortIDSubtype := ""
		localPortDesc := ""
		if localPort != nil {
			localPortID = strings.TrimSpace(localPort.portID)
			localPortIDSubtype = strings.TrimSpace(localPort.portIDSubtype)
			localPortDesc = strings.TrimSpace(localPort.portDesc)
		}

		observation.LLDPRemotes = append(observation.LLDPRemotes, topologyengine.LLDPRemoteObservation{
			LocalPortNum:       strings.TrimSpace(remote.localPortNum),
			RemoteIndex:        strings.TrimSpace(remote.remIndex),
			LocalPortID:        localPortID,
			LocalPortIDSubtype: localPortIDSubtype,
			LocalPortDesc:      localPortDesc,
			ChassisID:          strings.TrimSpace(remote.chassisID),
			SysName:            strings.TrimSpace(remote.sysName),
			PortID:             strings.TrimSpace(remote.portID),
			PortIDSubtype:      strings.TrimSpace(remote.portIDSubtype),
			PortDesc:           strings.TrimSpace(remote.portDesc),
			ManagementIP:       managementIP,
		})
	}
}

func (c *topologyCache) appendObservedCDPRemotes(observation *topologyengine.L2Observation) {
	if observation == nil {
		return
	}

	keys := make([]string, 0, len(c.cdpRemotes))
	for key := range c.cdpRemotes {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		remote := c.cdpRemotes[key]
		if remote == nil {
			continue
		}

		deviceID := strings.TrimSpace(remote.deviceID)
		sysName := strings.TrimSpace(remote.sysName)
		if deviceID == "" {
			deviceID = sysName
		}
		if deviceID == "" {
			continue
		}

		ifName := strings.TrimSpace(remote.ifName)
		if ifName == "" && strings.TrimSpace(remote.ifIndex) != "" {
			ifName = strings.TrimSpace(c.ifNamesByIndex[remote.ifIndex])
		}

		address := strings.TrimSpace(remote.address)
		if address == "" {
			address = pickManagementIP(remote.managementAddrs)
		}

		observation.CDPRemotes = append(observation.CDPRemotes, topologyengine.CDPRemoteObservation{
			LocalIfIndex: parseIndex(remote.ifIndex),
			LocalIfName:  ifName,
			DeviceIndex:  strings.TrimSpace(remote.deviceIndex),
			DeviceID:     deviceID,
			SysName:      sysName,
			DevicePort:   strings.TrimSpace(remote.devicePort),
			Address:      address,
		})
	}
}

func (c *topologyCache) deriveLocalBridgeMACFromFDBSelfEntries() string {
	if len(c.fdbEntries) == 0 {
		return ""
	}

	keys := make([]string, 0, len(c.fdbEntries))
	for key := range c.fdbEntries {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		entry := c.fdbEntries[key]
		if entry == nil || !isFDBSelfStatus(entry.status) {
			continue
		}
		mac := normalizeMAC(entry.mac)
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		return mac
	}

	return ""
}

func (c *topologyCache) deriveLocalBridgeMACFromInterfacePhysAddress(localManagementIP string) string {
	if len(c.ifStatusByIndex) == 0 {
		return ""
	}

	localManagementIP = normalizeIPAddress(localManagementIP)
	if localManagementIP != "" {
		ifIndex := strings.TrimSpace(c.ifIndexByIP[localManagementIP])
		if ifIndex != "" {
			if status, ok := c.ifStatusByIndex[ifIndex]; ok {
				if mac := normalizeMAC(status.mac); mac != "" && mac != "00:00:00:00:00:00" {
					return mac
				}
			}
		}
	}

	keys := make([]string, 0, len(c.ifStatusByIndex))
	for key := range c.ifStatusByIndex {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		left := parseIndex(keys[i])
		right := parseIndex(keys[j])
		if left > 0 && right > 0 && left != right {
			return left < right
		}
		if left > 0 && right <= 0 {
			return true
		}
		if left <= 0 && right > 0 {
			return false
		}
		return keys[i] < keys[j]
	})

	for _, key := range keys {
		mac := normalizeMAC(c.ifStatusByIndex[key].mac)
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		return mac
	}

	return ""
}

func isFDBSelfStatus(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "4", "self", "dot1d_tp_fdb_status_self", "dot1dtpfdbstatusself", "dot1q_tp_fdb_status_self", "dot1qtpfdbstatusself":
		return true
	default:
		return false
	}
}

func ensureTopologyObservationDeviceID(local topologyDevice, baseBridgeAddress string) string {
	if mac := topologyPrimaryIdentityMAC(local.ChassisID, baseBridgeAddress); mac != "" {
		return "macAddress:" + mac
	}
	if key := strings.TrimSpace(topologyDeviceKey(local)); key != "" {
		return key
	}
	if sysName := strings.TrimSpace(local.SysName); sysName != "" {
		return "sysname:" + strings.ToLower(sysName)
	}
	if ip := normalizeIPAddress(local.ManagementIP); ip != "" {
		return "management_ip:" + ip
	}
	if managementIP := strings.TrimSpace(local.ManagementIP); managementIP != "" {
		return "management_addr:" + strings.ToLower(managementIP)
	}
	return "local-device"
}

func topologyPrimaryIdentityMAC(chassisID, baseBridgeAddress string) string {
	for _, candidate := range []string{chassisID, baseBridgeAddress} {
		if mac := normalizeMAC(candidate); mac != "" && mac != "00:00:00:00:00:00" {
			return mac
		}
	}
	return ""
}
