// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strconv"
	"strings"
)

func topologyMetricString(metrics map[string]any, key string) string {
	if len(metrics) == 0 {
		return ""
	}
	value, ok := metrics[key]
	if !ok || value == nil {
		return ""
	}
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

func bridgeDomainSegmentID(segment *bridgeDomainSegment) string {
	if segment == nil {
		return ""
	}
	portKeys := sortedBridgePortSet(segment.ports)
	sig := strings.Join(portKeys, "<->")
	if sig == "" {
		sig = portSortKey(segment.designatedPort)
	}
	return "bridge-domain:" + sig
}

func bridgePortFromAdjacencySide(deviceID, port string, ifIndexByDeviceName map[string]int) bridgePortRef {
	deviceID = strings.TrimSpace(deviceID)
	port = strings.TrimSpace(port)
	if deviceID == "" || port == "" {
		return bridgePortRef{}
	}
	ifIndex := resolveIfIndexByPortName(deviceID, port, ifIndexByDeviceName)
	return bridgePortRef{
		deviceID:   deviceID,
		ifIndex:    ifIndex,
		ifName:     port,
		bridgePort: port,
	}
}

func bridgePortFromAttachment(attachment Attachment, ifaceByDeviceIndex map[string]Interface) bridgePortRef {
	deviceID := strings.TrimSpace(attachment.DeviceID)
	if deviceID == "" {
		return bridgePortRef{}
	}
	ifIndex := attachment.IfIndex
	ifName := strings.TrimSpace(attachment.Labels["if_name"])
	if ifName == "" && ifIndex > 0 {
		if iface, ok := ifaceByDeviceIndex[deviceIfIndexKey(deviceID, ifIndex)]; ok {
			ifName = strings.TrimSpace(iface.IfName)
		}
	}
	bridgePort := strings.TrimSpace(attachment.Labels["bridge_port"])
	if bridgePort == "" {
		if ifIndex > 0 {
			bridgePort = strconv.Itoa(ifIndex)
		} else {
			bridgePort = ifName
		}
	}
	vlanID := strings.TrimSpace(attachment.Labels["vlan"])
	if vlanID == "" {
		vlanID = strings.TrimSpace(attachment.Labels["vlan_id"])
	}
	return bridgePortRef{
		deviceID:   deviceID,
		ifIndex:    ifIndex,
		ifName:     ifName,
		bridgePort: bridgePort,
		vlanID:     vlanID,
	}
}

func bridgeAttachmentSortKey(attachment Attachment) string {
	vlanID := strings.TrimSpace(attachment.Labels["vlan"])
	if vlanID == "" {
		vlanID = strings.TrimSpace(attachment.Labels["vlan_id"])
	}
	parts := []string{
		strings.TrimSpace(attachment.DeviceID),
		strconv.Itoa(attachment.IfIndex),
		strings.TrimSpace(attachment.Labels["if_name"]),
		strings.TrimSpace(attachment.Labels["bridge_port"]),
		strings.ToLower(vlanID),
		strings.ToLower(strings.TrimSpace(attachment.Method)),
		strings.TrimSpace(attachment.EndpointID),
	}
	return strings.Join(parts, keySep)
}

func bridgePairKey(left, right bridgePortRef) string {
	leftKey := bridgePortRefKey(left, false, false)
	rightKey := bridgePortRefKey(right, false, false)
	if leftKey == "" || rightKey == "" {
		return ""
	}
	if leftKey > rightKey {
		leftKey, rightKey = rightKey, leftKey
	}
	return leftKey + "<->" + rightKey
}

func bridgePortRefKey(port bridgePortRef, includeBridgePort bool, includeVLAN bool) string {
	deviceID := strings.TrimSpace(port.deviceID)
	if deviceID == "" {
		return ""
	}
	bridgePort := strings.TrimSpace(port.bridgePort)
	if bridgePort == "" && port.ifIndex > 0 {
		bridgePort = strconv.Itoa(port.ifIndex)
	}
	ifName := strings.TrimSpace(port.ifName)
	vlanID := strings.TrimSpace(port.vlanID)
	if !includeVLAN {
		vlanID = ""
	}

	parts := []string{
		deviceID,
		"if:" + strconv.Itoa(port.ifIndex),
		"name:" + strings.ToLower(ifName),
	}
	if includeBridgePort {
		parts = append(parts, "bp:"+strings.ToLower(bridgePort))
	}
	parts = append(parts, "vlan:"+strings.ToLower(vlanID))
	return strings.Join(parts, keySep)
}

func bridgePortRefSortKey(port bridgePortRef) string {
	return bridgePortRefKey(port, true, true)
}

func bridgePortDisplay(port bridgePortRef) string {
	if name := strings.TrimSpace(port.ifName); name != "" {
		return name
	}
	if port.ifIndex > 0 {
		return strconv.Itoa(port.ifIndex)
	}
	return strings.TrimSpace(port.bridgePort)
}
