// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import "strings"

func ensureTopologyObservationDeviceID(device topologyDevice, baseBridgeAddress string) string {
	if mac := topologyPrimaryIdentityMAC(device.ChassisID, baseBridgeAddress); mac != "" {
		return "macAddress:" + mac
	}
	if key := strings.TrimSpace(topologyDeviceKey(device)); key != "" {
		return key
	}
	if sysName := strings.TrimSpace(device.SysName); sysName != "" {
		return "sysname:" + strings.ToLower(sysName)
	}
	if ip := normalizeIPAddress(device.ManagementIP); ip != "" {
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

func topologyPrimaryIdentityMAC(chassisID, baseBridgeAddress string) string {
	for _, candidate := range []string{chassisID, baseBridgeAddress} {
		if mac := normalizeMAC(candidate); mac != "" && mac != "00:00:00:00:00:00" {
			return mac
		}
	}
	return ""
}
