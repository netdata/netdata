// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
	"strings"
)

func matchLocalTopologyActor(match topologyMatch, local topologyDevice) bool {
	localChassisID := strings.TrimSpace(local.ChassisID)
	if localChassisID != "" {
		for _, chassisID := range match.ChassisIDs {
			if strings.EqualFold(strings.TrimSpace(chassisID), localChassisID) {
				return true
			}
		}
	}

	localSysName := strings.TrimSpace(local.SysName)
	if localSysName != "" && strings.EqualFold(strings.TrimSpace(match.SysName), localSysName) {
		return true
	}

	localIP := topologyutil.NormalizeIPAddress(local.ManagementIP)
	if localIP != "" {
		for _, ip := range match.IPAddresses {
			if topologyutil.NormalizeIPAddress(ip) == localIP {
				return true
			}
		}
	}

	return false
}
