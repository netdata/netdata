// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

func topologyNeighborCapabilitiesFromLabels(labels map[string]string) []string {
	return topologyNeighborCapabilitiesFromLabelsWithWork(nil, labels)
}

func topologyNeighborCapabilitiesFromLabelsWithWork(work *projectionWork, labels map[string]string) []string {
	if len(labels) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	for _, key := range []string{"capabilities_enabled", "capabilities_supported", "capabilities"} {
		for _, capability := range labelsCSVToSlice(labels, key) {
			capability = strings.TrimSpace(capability)
			if capability == "" {
				continue
			}
			seen[capability] = struct{}{}
		}
	}
	return sortedTopologySetWithWork(work, seen)
}

func buildTopologyPortNeighborStatus(protocol string, adj model.Adjacency, deviceByID map[string]model.Device) topologyPortNeighborStatus {
	return buildTopologyPortNeighborStatusWithWork(nil, protocol, adj, deviceByID)
}

func buildTopologyPortNeighborStatusWithWork(work *projectionWork, protocol string, adj model.Adjacency, deviceByID map[string]model.Device) topologyPortNeighborStatus {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	targetID := strings.TrimSpace(adj.TargetID)
	remotePort := strings.TrimSpace(adj.TargetPort)
	switch {
	case strings.TrimSpace(adj.TargetPortEvidence.IfName) != "":
		remotePort = strings.TrimSpace(adj.TargetPortEvidence.IfName)
	case remotePort != "":
	case strings.TrimSpace(adj.TargetPortEvidence.BridgePort) != "":
		remotePort = strings.TrimSpace(adj.TargetPortEvidence.BridgePort)
	case adj.TargetPortEvidence.IfIndex > 0:
		remotePort = strconv.Itoa(adj.TargetPortEvidence.IfIndex)
	}

	neighbor := topologyPortNeighborStatus{
		Protocol:     protocol,
		RemoteDevice: targetID,
		RemotePort:   remotePort,
	}
	if targetID == "" {
		return neighbor
	}

	remote, ok := deviceByID[targetID]
	if !ok {
		return neighbor
	}

	if remoteName := strings.TrimSpace(remote.Hostname); remoteName != "" {
		neighbor.RemoteDevice = remoteName
	}
	neighbor.RemoteIP = selectedDeviceManagementIP(remote)
	neighbor.RemoteChassisID = strings.TrimSpace(remote.ChassisID)
	neighbor.RemoteCapabilities = topologyNeighborCapabilitiesFromLabelsWithWork(work, remote.Labels)
	return neighbor
}

func topologyPortNeighborStatusKey(status topologyPortNeighborStatus) string {
	protocol := strings.ToLower(strings.TrimSpace(status.Protocol))
	remoteDevice := strings.ToLower(strings.TrimSpace(status.RemoteDevice))
	remotePort := strings.ToLower(strings.TrimSpace(status.RemotePort))
	remoteIP := strings.ToLower(strings.TrimSpace(status.RemoteIP))
	remoteChassisID := normalizeMAC(status.RemoteChassisID)
	if remoteChassisID == "" {
		remoteChassisID = strings.ToLower(strings.TrimSpace(status.RemoteChassisID))
	}

	if protocol == "" && remoteDevice == "" && remotePort == "" && remoteIP == "" && remoteChassisID == "" {
		return ""
	}
	return strings.Join([]string{
		protocol,
		remoteDevice,
		remotePort,
		remoteIP,
		remoteChassisID,
	}, keySep)
}

func sortedTopologyPortNeighbors(neighbors map[string]topologyPortNeighborStatus) []topologyPortNeighborStatus {
	return sortedTopologyPortNeighborsWithWork(nil, neighbors)
}

func sortedTopologyPortNeighborsWithWork(work *projectionWork, neighbors map[string]topologyPortNeighborStatus) []topologyPortNeighborStatus {
	if len(neighbors) == 0 {
		return nil
	}
	if !work.charge(uint64(len(neighbors))) {
		return nil
	}
	out := make([]topologyPortNeighborStatus, 0, len(neighbors))
	var maxStringBytes uint64
	for _, neighbor := range neighbors {
		if work != nil && !work.chargeStrings([]string{
			neighbor.Protocol,
			neighbor.RemoteDevice,
			neighbor.RemotePort,
			neighbor.RemoteIP,
			neighbor.RemoteChassisID,
		}) {
			return nil
		}
		neighbor.Protocol = strings.ToLower(strings.TrimSpace(neighbor.Protocol))
		neighbor.RemoteDevice = strings.TrimSpace(neighbor.RemoteDevice)
		neighbor.RemotePort = strings.TrimSpace(neighbor.RemotePort)
		neighbor.RemoteIP = strings.TrimSpace(neighbor.RemoteIP)
		neighbor.RemoteChassisID = strings.TrimSpace(neighbor.RemoteChassisID)
		neighbor.RemoteCapabilities = uniqueTopologyStringsWithWork(work, neighbor.RemoteCapabilities)
		if topologyPortNeighborStatusKey(neighbor) == "" {
			continue
		}
		if bytes := uint64(len(neighbor.Protocol) + len(neighbor.RemoteDevice) + len(neighbor.RemotePort) + len(neighbor.RemoteIP) + len(neighbor.RemoteChassisID)); bytes > maxStringBytes {
			maxStringBytes = bytes
		}
		out = append(out, neighbor)
	}
	if !sortProjectionSliceStableWithStringWork(work, out, maxStringBytes, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.Protocol != right.Protocol {
			return left.Protocol < right.Protocol
		}
		if left.RemoteDevice != right.RemoteDevice {
			return left.RemoteDevice < right.RemoteDevice
		}
		if left.RemotePort != right.RemotePort {
			return left.RemotePort < right.RemotePort
		}
		if left.RemoteIP != right.RemoteIP {
			return left.RemoteIP < right.RemoteIP
		}
		return left.RemoteChassisID < right.RemoteChassisID
	}) {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
