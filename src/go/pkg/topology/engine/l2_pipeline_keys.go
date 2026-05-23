// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// keySep separates delimiter-based identifiers that some topology paths still split later.
// Opaque keys built from observation data should use opaqueCompositeKey() instead.
const keySep = "\x00"

func opaqueCompositeKey(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}

	var b strings.Builder
	for _, part := range parts {
		b.WriteString(strconv.Itoa(len(part)))
		b.WriteByte(':')
		b.WriteString(part)
	}
	return b.String()
}

func topologyMatchCompositeKey(parts ...string) string {
	return opaqueCompositeKey(parts...)
}

func adjacencyKey(adj Adjacency) string {
	protocol := strings.ToLower(strings.TrimSpace(adj.Protocol))
	sourceID := strings.TrimSpace(adj.SourceID)
	sourcePort := strings.TrimSpace(adj.SourcePort)
	targetID := strings.TrimSpace(adj.TargetID)
	targetPort := strings.TrimSpace(adj.TargetPort)

	return opaqueCompositeKey(protocol, sourceID, sourcePort, targetID, targetPort)
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
	return opaqueCompositeKey(
		deviceID,
		strconv.Itoa(attachment.IfIndex),
		endpointID,
		method,
		strings.ToLower(vlanID),
	)
}

func ifaceKey(iface Interface) string {
	return opaqueCompositeKey(iface.DeviceID, strconv.Itoa(iface.IfIndex), iface.IfName)
}

func deviceIfIndexKey(deviceID string, ifIndex int) string {
	return opaqueCompositeKey(deviceID, strconv.Itoa(ifIndex))
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
