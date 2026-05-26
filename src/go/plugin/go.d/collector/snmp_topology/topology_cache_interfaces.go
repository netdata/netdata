// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"math"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func init() {
	registerTopologyMetricHandler(ddsnmp.KindIfName, (*topologyCache).updateIfNameByIndex)
	registerTopologyMetricHandler(ddsnmp.KindIfStatus, (*topologyCache).updateIfNameByIndex)
	registerTopologyMetricHandler(ddsnmp.KindIfDuplex, (*topologyCache).updateIfNameByIndex)
	registerTopologyMetricHandler(ddsnmp.KindIpIfIndex, (*topologyCache).updateIfIndexByIP)
	registerTopologyMetricHandler(ddsnmp.KindBridgePortIfIndex, (*topologyCache).updateBridgePortMap)
}

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
