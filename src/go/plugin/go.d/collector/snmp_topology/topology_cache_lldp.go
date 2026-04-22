// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "strings"

func (c *topologyCache) updateLldpLocPort(tags map[string]string) {
	portNum := tags[tagLldpLocPortNum]
	if portNum == "" {
		return
	}

	entry := c.lldpLocPorts[portNum]
	if entry == nil {
		entry = &lldpLocPort{portNum: portNum}
		c.lldpLocPorts[portNum] = entry
	}

	if v := tags[tagLldpLocPortID]; v != "" {
		entry.portID = v
	}
	if v := tags[tagLldpLocPortIDSubtype]; v != "" {
		entry.portIDSubtype = normalizeLLDPSubtype(v, lldpPortIDSubtypeMap)
	}
	if v := tags[tagLldpLocPortDesc]; v != "" {
		entry.portDesc = v
	}
}

func (c *topologyCache) updateLldpLocManAddr(tags map[string]string) {
	addrHex := tags[tagLldpLocMgmtAddr]
	if addrHex == "" {
		return
	}

	addr, addrType := normalizeManagementAddress(addrHex, tags[tagLldpLocMgmtAddrSubtype])
	if addr == "" {
		return
	}

	mgmt := topologyManagementAddress{
		Address:     addr,
		AddressType: addrType,
		IfSubtype:   tags[tagLldpLocMgmtAddrIfSubtype],
		IfID:        tags[tagLldpLocMgmtAddrIfID],
		OID:         tags[tagLldpLocMgmtAddrOID],
		Source:      "lldp_local",
	}

	c.localDevice.ManagementAddresses = appendManagementAddress(c.localDevice.ManagementAddresses, mgmt)
}

func (c *topologyCache) updateLldpRemote(tags map[string]string) {
	localPort := tags[tagLldpLocPortNum]
	if localPort == "" {
		return
	}

	remIndex := tags[tagLldpRemIndex]
	if remIndex == "" {
		return
	}
	key := localPort + ":" + remIndex

	entry := c.lldpRemotes[key]
	if entry == nil {
		entry = &lldpRemote{
			localPortNum: localPort,
			remIndex:     remIndex,
		}
		c.lldpRemotes[key] = entry
	}

	if v := tags[tagLldpRemChassisID]; v != "" {
		entry.chassisID = v
	}
	if v := tags[tagLldpRemChassisIDSubtype]; v != "" {
		entry.chassisIDSubtype = normalizeLLDPSubtype(v, lldpChassisIDSubtypeMap)
	}
	if v := tags[tagLldpRemPortID]; v != "" {
		entry.portID = v
	}
	if v := tags[tagLldpRemPortIDSubtype]; v != "" {
		entry.portIDSubtype = normalizeLLDPSubtype(v, lldpPortIDSubtypeMap)
	}
	if v := tags[tagLldpRemPortDesc]; v != "" {
		entry.portDesc = v
	}
	if v := tags[tagLldpRemSysName]; v != "" {
		entry.sysName = v
	}
	if v := tags[tagLldpRemSysDesc]; v != "" {
		entry.sysDesc = v
	}
	if v := tags[tagLldpRemSysCapSupported]; v != "" {
		entry.sysCapSupported = v
	}
	if v := tags[tagLldpRemSysCapEnabled]; v != "" {
		entry.sysCapEnabled = v
	}
	if v := tags[tagLldpRemMgmtAddr]; v != "" {
		entry.managementAddr = v
		addr, addrType := normalizeManagementAddress(v, tags[tagLldpRemMgmtAddrSubtype])
		if addr != "" {
			entry.managementAddrs = appendManagementAddress(entry.managementAddrs, topologyManagementAddress{
				Address:     addr,
				AddressType: addrType,
				Source:      "lldp_remote",
			})
		}
	}
}

func (c *topologyCache) updateLldpRemManAddr(tags map[string]string) {
	localPort := tags[tagLldpLocPortNum]
	if localPort == "" {
		return
	}

	remIndex := tags[tagLldpRemIndex]
	if remIndex == "" {
		return
	}

	key := localPort + ":" + remIndex
	entry := c.lldpRemotes[key]
	if entry == nil {
		entry = &lldpRemote{
			localPortNum: localPort,
			remIndex:     remIndex,
		}
		c.lldpRemotes[key] = entry
	}

	addrHex := tags[tagLldpRemMgmtAddr]
	if strings.TrimSpace(addrHex) == "" {
		addrHex = reconstructLldpRemMgmtAddrHex(tags)
	}
	addr, addrType := normalizeManagementAddress(addrHex, tags[tagLldpRemMgmtAddrSubtype])
	if addr == "" {
		return
	}

	mgmt := topologyManagementAddress{
		Address:     addr,
		AddressType: addrType,
		IfSubtype:   tags[tagLldpRemMgmtAddrIfSubtype],
		IfID:        tags[tagLldpRemMgmtAddrIfID],
		OID:         tags[tagLldpRemMgmtAddrOID],
		Source:      "lldp_remote",
	}
	entry.managementAddrs = appendManagementAddress(entry.managementAddrs, mgmt)
}
