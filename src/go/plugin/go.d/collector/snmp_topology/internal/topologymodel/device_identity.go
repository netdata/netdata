// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyutil"
)

func ObservationDeviceID(device Device, baseBridgeAddress string) string {
	if mac := primaryIdentityMAC(device.ChassisID, baseBridgeAddress); mac != "" {
		return "macAddress:" + mac
	}
	if key := deviceKey(device); key != "" {
		return key
	}
	if sysName := strings.TrimSpace(device.SysName); sysName != "" {
		return "sysname:" + strings.ToLower(sysName)
	}
	if ip := topologyutil.NormalizeIPAddress(device.ManagementIP); ip != "" {
		return "management_ip:" + ip
	}
	if managementIP := strings.TrimSpace(device.ManagementIP); managementIP != "" {
		return "management_addr:" + strings.ToLower(managementIP)
	}
	if jobID := strings.TrimSpace(device.AgentJobID); jobID != "" {
		return "agent_job:" + strings.ToLower(jobID)
	}
	if hostID := strings.TrimSpace(device.NetdataHostID); hostID != "" {
		return "agent:" + strings.ToLower(hostID)
	}
	if agentID := strings.TrimSpace(device.AgentID); agentID != "" {
		return "agent:" + strings.ToLower(agentID)
	}
	return "local-device"
}

func primaryIdentityMAC(chassisID, baseBridgeAddress string) string {
	for _, candidate := range []string{chassisID, baseBridgeAddress} {
		if mac := topologyutil.NormalizeMAC(candidate); mac != "" && mac != "00:00:00:00:00:00" {
			return mac
		}
	}
	return ""
}

func deviceKey(dev Device) string {
	chassisID := strings.TrimSpace(dev.ChassisID)
	if chassisID == "" {
		return ""
	}
	return strings.TrimSpace(dev.ChassisIDType) + ":" + chassisID
}

// LocalActorID applies the same fallback used by live local actor construction.
func LocalActorID(localDeviceID string, device Device) string {
	if id := strings.TrimSpace(localDeviceID); id != "" {
		return id
	}
	return ObservationDeviceID(device, "")
}
