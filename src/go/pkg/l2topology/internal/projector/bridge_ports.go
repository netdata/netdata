// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func topologyLinkBridgeDomain(link graph.Link) string {
	if link.L2 == nil {
		return ""
	}
	return strings.TrimSpace(link.L2.BridgeDomain)
}

func topologyLinkInference(link graph.Link) string {
	if link.Inference == nil {
		return ""
	}
	return strings.TrimSpace(link.Inference.Inference)
}

func topologyLinkAttachmentMode(link graph.Link) string {
	if link.Inference == nil {
		return ""
	}
	return strings.TrimSpace(link.Inference.AttachmentMode)
}

func topologyLinkConfidence(link graph.Link) string {
	if link.Inference == nil {
		return ""
	}
	return strings.TrimSpace(link.Inference.Confidence)
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

func bridgePortsFromAdjacency(
	adj model.Adjacency,
	ifIndexByDeviceName map[string]int,
	ifaceByDeviceIndex map[string]model.Interface,
	aliases bridgePortAliasIndex,
) (bridgePortRef, bridgePortRef) {
	vlanID := adjacencyVLANID(adj)
	return bridgePortFromAdjacencyEvidence(
			adj.SourceID,
			adj.SourcePortEvidence,
			vlanID,
			ifIndexByDeviceName,
			ifaceByDeviceIndex,
			aliases,
		), bridgePortFromAdjacencyEvidence(
			adj.TargetID,
			adj.TargetPortEvidence,
			vlanID,
			ifIndexByDeviceName,
			ifaceByDeviceIndex,
			aliases,
		)
}

func bridgePortFromAdjacencyEvidence(
	deviceID string,
	evidence model.AdjacencyPortEvidence,
	vlanID string,
	ifIndexByDeviceName map[string]int,
	ifaceByDeviceIndex map[string]model.Interface,
	aliases bridgePortAliasIndex,
) bridgePortRef {
	deviceID = strings.TrimSpace(deviceID)
	ifName := strings.TrimSpace(evidence.IfName)
	bridgePort := strings.TrimSpace(evidence.BridgePort)
	if deviceID == "" || (evidence.IfIndex <= 0 && ifName == "" && bridgePort == "") {
		return bridgePortRef{}
	}

	ifIndex := evidence.IfIndex
	if ifIndex <= 0 && ifName != "" {
		ifIndex = resolveIfIndexByInterfaceName(deviceID, ifName, ifIndexByDeviceName)
	}
	if ifIndex <= 0 && bridgePort != "" {
		ifIndex = aliases.resolveIfIndex(deviceID, bridgePort)
	}

	if ifIndex > 0 {
		if iface, ok := ifaceByDeviceIndex[deviceIfIndexKey(deviceID, ifIndex)]; ok {
			if name := strings.TrimSpace(iface.IfName); name != "" {
				ifName = name
			}
		}
	}

	return bridgePortRef{
		deviceID:   deviceID,
		ifIndex:    ifIndex,
		ifName:     ifName,
		bridgePort: bridgePort,
		vlanID:     strings.TrimSpace(vlanID),
	}
}

func bridgePortFromAttachment(attachment model.Attachment, ifaceByDeviceIndex map[string]model.Interface) bridgePortRef {
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
		deviceID:    deviceID,
		ifIndex:     ifIndex,
		ifName:      ifName,
		bridgePort:  bridgePort,
		fdbDomainID: strings.TrimSpace(attachment.Labels["fdb_domain_id"]),
		vlanID:      vlanID,
	}
}

func bridgeAttachmentSortKey(attachment model.Attachment) string {
	vlanID := strings.TrimSpace(attachment.Labels["vlan"])
	if vlanID == "" {
		vlanID = strings.TrimSpace(attachment.Labels["vlan_id"])
	}
	parts := []string{
		strings.TrimSpace(attachment.DeviceID),
		strconv.Itoa(attachment.IfIndex),
		strings.TrimSpace(attachment.Labels["if_name"]),
		strings.TrimSpace(attachment.Labels["bridge_port"]),
		strings.ToLower(strings.TrimSpace(attachment.Labels["fdb_domain_id"])),
		strings.ToLower(vlanID),
		strings.ToLower(strings.TrimSpace(attachment.Method)),
		strings.TrimSpace(attachment.EndpointID),
	}
	return strings.Join(parts, keySep)
}

func bridgePairKey(left, right bridgePortRef) string {
	leftKey := bridgePortRefKey(left, false, false)
	rightKey := bridgePortRefKey(right, false, false)
	return canonicalBridgePairKey(leftKey, rightKey)
}

func bridgeScopedPairKey(left, right bridgePortRef) string {
	leftKey := bridgePortRefKey(left, false, true)
	rightKey := bridgePortRefKey(right, false, true)
	return canonicalBridgePairKey(leftKey, rightKey)
}

func canonicalBridgePairKey(leftKey, rightKey string) string {
	if leftKey == "" || rightKey == "" {
		return ""
	}
	if leftKey > rightKey {
		leftKey, rightKey = rightKey, leftKey
	}
	return leftKey + "<->" + rightKey
}

func bridgePortRefKey(port bridgePortRef, includeBridgePort bool, includeVLAN bool) string {
	identity := bridgePortCanonicalIdentity(port)
	if identity == "" {
		return ""
	}
	parts := []string{identity}
	if includeVLAN {
		parts = append(parts, "scope:"+bridgePortForwardingDomain(port))
	}
	if includeBridgePort {
		parts = append(parts,
			"name:"+normalizeInterfaceNameForLookup(port.ifName),
			"bp:"+strings.ToLower(strings.TrimSpace(port.bridgePort)),
		)
	}
	return strings.Join(parts, keySep)
}

func bridgePortForwardingDomain(port bridgePortRef) string {
	if domain := strings.TrimSpace(port.fdbDomainID); domain != "" {
		return strings.ToLower(domain)
	}
	if vlanID := strings.TrimSpace(port.vlanID); vlanID != "" {
		return "vlan:" + strings.ToLower(vlanID)
	}
	return ""
}

func bridgePortVLANScope(port bridgePortRef) string {
	if vlanID := strings.TrimSpace(port.vlanID); vlanID != "" {
		return "vlan:" + strings.ToLower(vlanID)
	}
	return ""
}

func bridgePortCanonicalIdentity(port bridgePortRef) string {
	deviceID := strings.TrimSpace(port.deviceID)
	if deviceID == "" {
		return ""
	}
	if port.ifIndex > 0 {
		return strings.Join([]string{deviceID, "if:" + strconv.Itoa(port.ifIndex)}, keySep)
	}
	if name := normalizeInterfaceNameForLookup(port.ifName); name != "" {
		return strings.Join([]string{deviceID, "name:" + name}, keySep)
	}
	if bridgePort := strings.ToLower(strings.TrimSpace(port.bridgePort)); bridgePort != "" {
		return strings.Join([]string{deviceID, "bp:" + bridgePort}, keySep)
	}
	return ""
}

func bridgePortRefSortKey(port bridgePortRef) string {
	return bridgePortRefKey(port, true, true)
}

func bridgePortRefDisplayKey(port bridgePortRef) string {
	identity := bridgePortCanonicalIdentity(port)
	if identity == "" {
		return ""
	}
	return strings.Join([]string{
		identity,
		"name:" + strings.ToLower(strings.TrimSpace(port.ifName)),
		"bp:" + strings.ToLower(strings.TrimSpace(port.bridgePort)),
		"scope:" + bridgePortForwardingDomain(port),
	}, keySep)
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

func adjacencyVLANID(adj model.Adjacency) string {
	vlanID := strings.TrimSpace(adj.Labels["vlan_id"])
	if vlanID == "" {
		vlanID = strings.TrimSpace(adj.Labels["vlan"])
	}
	return vlanID
}
