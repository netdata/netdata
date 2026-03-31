// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

func (c *topologyCache) updateIfNameByIndex(tags map[string]string) {
	ifIndex := strings.TrimSpace(tags[tagTopoIfIndex])
	if ifIndex == "" {
		return
	}

	ifName := strings.TrimSpace(tags[tagTopoIfName])
	if ifName != "" {
		c.ifNamesByIndex[ifIndex] = ifName
	}

	status := c.ifStatusByIndex[ifIndex]
	if ifType := normalizeInterfaceType(tags[tagTopoIfType]); ifType != "" {
		status.ifType = ifType
	}
	if admin := normalizeInterfaceAdminStatus(tags[tagTopoIfAdmin]); admin != "" {
		status.admin = admin
	}
	if oper := normalizeInterfaceOperStatus(tags[tagTopoIfOper]); oper != "" {
		status.oper = oper
	}
	if ifDescr := strings.TrimSpace(tags[tagTopoIfDescr]); ifDescr != "" {
		status.ifDescr = ifDescr
	}
	if ifAlias := strings.TrimSpace(tags[tagTopoIfAlias]); ifAlias != "" {
		status.ifAlias = ifAlias
	}
	if mac := normalizeMAC(tags[tagTopoIfPhys]); mac != "" && mac != "00:00:00:00:00:00" {
		status.mac = mac
	}
	if ifHighSpeed := parsePositiveInt64(tags[tagTopoIfHigh]); ifHighSpeed > 0 {
		if ifHighSpeed > math.MaxInt64/topologyHighSpeedMultiplier {
			status.speedBps = math.MaxInt64
		} else {
			status.speedBps = ifHighSpeed * topologyHighSpeedMultiplier
		}
	} else if ifSpeed := parsePositiveInt64(tags[tagTopoIfSpeed]); ifSpeed > 0 {
		status.speedBps = ifSpeed
	}
	if lastChange := parsePositiveInt64(tags[tagTopoIfLast]); lastChange > 0 {
		status.lastChange = lastChange
	}
	if duplex := normalizeInterfaceDuplex(tags[tagTopoIfDuplex]); duplex != "" {
		status.duplex = duplex
	}
	if status.ifType != "" ||
		status.admin != "" ||
		status.oper != "" ||
		status.ifDescr != "" ||
		status.ifAlias != "" ||
		status.mac != "" ||
		status.speedBps > 0 ||
		status.lastChange > 0 ||
		status.duplex != "" {
		c.ifStatusByIndex[ifIndex] = status
	}
}

func (c *topologyCache) updateIfIndexByIP(tags map[string]string) {
	ifIndex := strings.TrimSpace(tags[tagTopoIfIndex])
	if ifIndex == "" {
		return
	}

	ip := normalizeIPAddress(tags[tagTopoIPAddr])
	if ip == "" {
		return
	}

	c.ifIndexByIP[ip] = ifIndex
	c.localDevice.ManagementAddresses = appendManagementAddress(c.localDevice.ManagementAddresses, topologyManagementAddress{
		Address:     ip,
		AddressType: managementAddressTypeFromIP(ip),
		Source:      "ip_mib",
	})
	if mask := normalizeIPAddress(tags[tagTopoIPMask]); mask != "" {
		c.ifNetmaskByIP[ip] = mask
	}
}

func (c *topologyCache) updateBridgePortMap(tags map[string]string) {
	c.updateLocalBridgeIdentityFromTags(tags)

	basePort := strings.TrimSpace(tags[tagBridgeBasePort])
	if basePort == "" {
		return
	}

	ifIndex := strings.TrimSpace(tags[tagBridgeIfIndex])
	if ifIndex == "" {
		return
	}

	c.bridgePortToIf[basePort] = ifIndex
}

func (c *topologyCache) vtpVLANContexts() []topologyVLANContext {
	c.mu.RLock()
	defer c.mu.RUnlock()

	contexts := make([]topologyVLANContext, 0, len(c.vlanIDToName))
	for vlanID, vlanName := range c.vlanIDToName {
		id := strings.TrimSpace(vlanID)
		if id == "" {
			continue
		}
		if _, err := strconv.Atoi(id); err != nil {
			continue
		}
		contexts = append(contexts, topologyVLANContext{
			vlanID:   id,
			vlanName: strings.TrimSpace(vlanName),
		})
	}

	sortTopologyVLANContexts(contexts)
	return contexts
}

func sortTopologyVLANContexts(contexts []topologyVLANContext) {
	sort.Slice(contexts, func(i, j int) bool {
		left, leftErr := strconv.Atoi(contexts[i].vlanID)
		right, rightErr := strconv.Atoi(contexts[j].vlanID)
		if leftErr == nil && rightErr == nil && left != right {
			return left < right
		}
		if contexts[i].vlanID != contexts[j].vlanID {
			return contexts[i].vlanID < contexts[j].vlanID
		}
		return contexts[i].vlanName < contexts[j].vlanName
	})
}
