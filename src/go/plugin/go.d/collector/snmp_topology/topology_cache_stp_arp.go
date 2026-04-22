// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "strings"

func (c *topologyCache) updateStpPortEntry(tags map[string]string) {
	port := strings.TrimSpace(tags[tagStpPort])
	if port == "" {
		return
	}

	contextVLANID := strings.TrimSpace(tags[tagTopologyContextVLANID])
	stpPortKey := port
	if contextVLANID != "" {
		stpPortKey = port + "|vlan:" + strings.ToLower(contextVLANID)
	}
	entry := c.stpPorts[stpPortKey]
	if entry == nil {
		entry = &stpPortEntry{port: port}
		c.stpPorts[stpPortKey] = entry
	}
	if contextVLANID != "" {
		entry.vlanID = contextVLANID
	}
	if v := strings.TrimSpace(tags[tagTopologyContextVLANName]); v != "" {
		entry.vlanName = v
	}
	if v := strings.TrimSpace(tags[tagStpPortPriority]); v != "" {
		entry.priority = v
	}
	if v := strings.TrimSpace(tags[tagStpPortState]); v != "" {
		entry.state = v
	}
	if v := strings.TrimSpace(tags[tagStpPortEnable]); v != "" {
		entry.enable = v
	}
	if v := strings.TrimSpace(tags[tagStpPortPathCost]); v != "" {
		entry.pathCost = v
	}
	if v := stpBridgeAddressToMAC(tags[tagStpPortDesignatedRoot]); v != "" {
		entry.designatedRoot = v
	}
	if v := strings.TrimSpace(tags[tagStpPortDesignatedCost]); v != "" {
		entry.designatedCost = v
	}
	if v := stpBridgeAddressToMAC(tags[tagStpPortDesignatedBridge]); v != "" {
		entry.designatedBridge = v
	}
	if v := stpDesignatedPortString(tags[tagStpPortDesignatedPort]); v != "" {
		entry.designatedPort = v
	}
}

func (c *topologyCache) updateArpEntry(tags map[string]string) {
	ip := normalizeIPAddress(tags[tagArpIP])
	mac := normalizeMAC(tags[tagArpMac])
	if ip == "" || mac == "" {
		return
	}

	ifIndex := strings.TrimSpace(tags[tagArpIfIndex])
	ifName := strings.TrimSpace(tags[tagArpIfName])
	if ifName == "" && ifIndex != "" {
		ifName = c.ifNamesByIndex[ifIndex]
	}

	if ifIndex != "" && ifName != "" {
		c.ifNamesByIndex[ifIndex] = ifName
	}

	key := strings.Join([]string{ifIndex, ip, mac}, "|")
	entry := c.arpEntries[key]
	if entry == nil {
		entry = &arpEntry{
			ifIndex: ifIndex,
			ifName:  ifName,
			ip:      ip,
			mac:     mac,
		}
		c.arpEntries[key] = entry
	}

	if v := strings.TrimSpace(tags[tagArpState]); v != "" {
		entry.state = v
	}
	if v := strings.TrimSpace(tags[tagArpType]); v != "" && entry.state == "" {
		entry.state = v
	}
	if v := strings.TrimSpace(tags[tagArpAddrType]); v != "" {
		entry.addrType = v
	}
}
