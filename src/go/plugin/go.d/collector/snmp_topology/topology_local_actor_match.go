// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func matchLocalTopologyActor(match topology.Match, local topologyDevice) bool {
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

	localIP := normalizeIPAddress(local.ManagementIP)
	if localIP != "" {
		for _, ip := range match.IPAddresses {
			if normalizeIPAddress(ip) == localIP {
				return true
			}
		}
	}

	return false
}
