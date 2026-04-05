// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// keySep is a NUL byte used as composite map-key separator.
// NUL cannot appear in SNMP string values, so field collisions are impossible.
const keySep = "\x00"

func adjacencyKey(adj Adjacency) string {
	protocol := strings.ToLower(strings.TrimSpace(adj.Protocol))
	sourceID := strings.TrimSpace(adj.SourceID)
	sourcePort := strings.TrimSpace(adj.SourcePort)
	targetID := strings.TrimSpace(adj.TargetID)
	targetPort := strings.TrimSpace(adj.TargetPort)

	return strings.Join([]string{protocol, sourceID, sourcePort, targetID, targetPort}, keySep)
}

func attachmentKey(attachment Attachment) string {
	deviceID := strings.TrimSpace(attachment.DeviceID)
	endpointID := strings.TrimSpace(attachment.EndpointID)
	method := strings.ToLower(strings.TrimSpace(attachment.Method))
	vlanID := ""
	if len(attachment.Labels) > 0 {
		vlanID = strings.TrimSpace(attachment.Labels["vlan_id"])
		if vlanID == "" {
			vlanID = strings.TrimSpace(attachment.Labels["vlan"])
		}
	}
	return strings.Join([]string{
		deviceID,
		strconv.Itoa(attachment.IfIndex),
		endpointID,
		method,
		strings.ToLower(vlanID),
	}, keySep)
}

func ifaceKey(iface Interface) string {
	return iface.DeviceID + keySep + strconv.Itoa(iface.IfIndex) + keySep + iface.IfName
}

func deviceIfIndexKey(deviceID string, ifIndex int) string {
	return deviceID + keySep + strconv.Itoa(ifIndex)
}

func deriveBridgeDomainFromIfIndex(deviceID string, ifIndex int) string {
	return fmt.Sprintf("bridge-domain:%s:if:%d", deviceID, ifIndex)
}

func deriveBridgeDomainFromBridgePort(deviceID, bridgePort string) string {
	return fmt.Sprintf("bridge-domain:%s:bp:%s", deviceID, bridgePort)
}

func attachmentDomain(attachment Attachment) string {
	if len(attachment.Labels) == 0 {
		return ""
	}
	return strings.TrimSpace(attachment.Labels["bridge_domain"])
}
