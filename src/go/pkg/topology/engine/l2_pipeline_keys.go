// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"fmt"
	"strconv"
	"strings"
)

func adjacencyKey(adj Adjacency) string {
	return strings.Join([]string{adj.Protocol, adj.SourceID, adj.SourcePort, adj.TargetID, adj.TargetPort}, "|")
}

func attachmentKey(attachment Attachment) string {
	vlanID := ""
	if len(attachment.Labels) > 0 {
		vlanID = strings.TrimSpace(attachment.Labels["vlan_id"])
		if vlanID == "" {
			vlanID = strings.TrimSpace(attachment.Labels["vlan"])
		}
	}
	return strings.Join([]string{
		attachment.DeviceID,
		strconv.Itoa(attachment.IfIndex),
		attachment.EndpointID,
		attachment.Method,
		strings.ToLower(vlanID),
	}, "|")
}

func ifaceKey(iface Interface) string {
	return fmt.Sprintf("%s|%d|%s", iface.DeviceID, iface.IfIndex, iface.IfName)
}

func deviceIfIndexKey(deviceID string, ifIndex int) string {
	return fmt.Sprintf("%s|%d", deviceID, ifIndex)
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
