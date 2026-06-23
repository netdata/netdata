// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/keyutil"
)

const keySep = keyutil.Sep

func opaqueCompositeKey(parts ...string) string {
	return keyutil.OpaqueCompositeKey(parts...)
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
	return keyutil.DeviceIfIndexKey(deviceID, ifIndex)
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
