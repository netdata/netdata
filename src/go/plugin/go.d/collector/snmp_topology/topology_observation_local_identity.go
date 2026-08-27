// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func (c *topologyBuilder) resolveLocalBaseBridgeAddress(localManagementIP string) string {
	baseBridgeAddress := strings.TrimSpace(c.bridgeBaseAddress)
	if baseBridgeAddress == "" {
		baseBridgeAddress = c.deriveLocalBridgeMACFromFDBSelfEntries()
	}
	if baseBridgeAddress == "" {
		baseBridgeAddress = c.deriveLocalBridgeMACFromInterfacePhysAddress(localManagementIP)
	}
	return baseBridgeAddress
}

func (c *topologyBuilder) deriveLocalBridgeMACFromFDBSelfEntries() string {
	if len(c.fdbEntries) == 0 {
		return ""
	}

	var keys []string
	if c.workLimiter == nil {
		keys = make([]string, 0, len(c.fdbEntries))
	}
	keys = sortedBuilderKeys(c, c.fdbEntries, keys)
	if !c.chargeWork(uint64(len(keys))) {
		return ""
	}

	for _, key := range keys {
		entry := c.fdbEntries[key]
		if entry == nil || !isFDBSelfStatus(entry.status) {
			continue
		}
		mac := topologyutil.NormalizeMAC(entry.mac)
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		return mac
	}

	return ""
}

func (c *topologyBuilder) deriveLocalBridgeMACFromInterfacePhysAddress(localManagementIP string) string {
	if len(c.ifStatusByIndex) == 0 {
		return ""
	}

	localManagementIP = topologyutil.NormalizeIPAddress(localManagementIP)
	if localManagementIP != "" {
		ifIndex := strings.TrimSpace(c.ifIndexByIP[localManagementIP])
		if ifIndex != "" {
			if status, ok := c.ifStatusByIndex[ifIndex]; ok {
				if mac := topologyutil.NormalizeMAC(status.mac); mac != "" && mac != "00:00:00:00:00:00" {
					return mac
				}
			}
		}
	}

	keys := make([]string, 0, len(c.ifStatusByIndex))
	if !c.chargeWork(uint64(len(c.ifStatusByIndex))) {
		return ""
	}
	for key := range c.ifStatusByIndex {
		keys = append(keys, key)
	}
	sortBuilderSlice(c, keys, func(i, j int) bool {
		left := topologyutil.ParseIndex(keys[i])
		right := topologyutil.ParseIndex(keys[j])
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
	if !c.chargeWork(uint64(len(keys))) {
		return ""
	}

	for _, key := range keys {
		mac := topologyutil.NormalizeMAC(c.ifStatusByIndex[key].mac)
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
