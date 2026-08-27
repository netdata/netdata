// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
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

	type orderedInterface struct {
		key   string
		index int
	}
	interfaces := make([]orderedInterface, 0, len(c.ifStatusByIndex))
	if !c.chargeWork(uint64(len(c.ifStatusByIndex))) {
		return ""
	}
	for key := range c.ifStatusByIndex {
		interfaces = append(interfaces, orderedInterface{key: key, index: topologyutil.ParseIndex(key)})
	}
	maxKeyBytes, err := worklimit.ChargeStringValues(c.workLimiter, interfaces, func(value orderedInterface) (uint64, error) {
		return uint64(len(value.key)), nil
	})
	if err != nil {
		c.workErr = err
		return ""
	}
	sortBuilderSliceWithStringWork(c, interfaces, maxKeyBytes, func(i, j int) bool {
		left := interfaces[i].index
		right := interfaces[j].index
		if left > 0 && right > 0 && left != right {
			return left < right
		}
		if left > 0 && right <= 0 {
			return true
		}
		if left <= 0 && right > 0 {
			return false
		}
		return interfaces[i].key < interfaces[j].key
	})
	if !c.chargeWork(uint64(len(interfaces))) {
		return ""
	}

	for _, iface := range interfaces {
		key := iface.key
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
