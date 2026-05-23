// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"
	"strings"
)

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
