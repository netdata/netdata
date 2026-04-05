// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"
)

func topologyNeighborCapabilitiesFromLabels(labels map[string]string) []string {
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
	return sortedTopologySet(seen)
}

func buildTopologyPortNeighborStatus(protocol string, adj Adjacency, deviceByID map[string]Device) topologyPortNeighborStatus {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	targetID := strings.TrimSpace(adj.TargetID)

	neighbor := topologyPortNeighborStatus{
		Protocol:     protocol,
		RemoteDevice: targetID,
		RemotePort:   strings.TrimSpace(adj.TargetPort),
	}
	if targetID == "" {
		return neighbor
	}

	remote, ok := deviceByID[targetID]
	if !ok {
		if protocol == "cdp" {
			neighbor.RemoteIP = strings.TrimSpace(adj.Labels["remote_address_raw"])
		}
		return neighbor
	}

	if remoteName := strings.TrimSpace(remote.Hostname); remoteName != "" {
		neighbor.RemoteDevice = remoteName
	}
	neighbor.RemoteIP = firstAddress(remote.Addresses)
	neighbor.RemoteChassisID = strings.TrimSpace(remote.ChassisID)
	neighbor.RemoteCapabilities = topologyNeighborCapabilitiesFromLabels(remote.Labels)
	if neighbor.RemoteIP == "" && protocol == "cdp" {
		neighbor.RemoteIP = strings.TrimSpace(adj.Labels["remote_address_raw"])
	}
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
	if len(neighbors) == 0 {
		return nil
	}
	out := make([]topologyPortNeighborStatus, 0, len(neighbors))
	for _, neighbor := range neighbors {
		neighbor.Protocol = strings.ToLower(strings.TrimSpace(neighbor.Protocol))
		neighbor.RemoteDevice = strings.TrimSpace(neighbor.RemoteDevice)
		neighbor.RemotePort = strings.TrimSpace(neighbor.RemotePort)
		neighbor.RemoteIP = strings.TrimSpace(neighbor.RemoteIP)
		neighbor.RemoteChassisID = strings.TrimSpace(neighbor.RemoteChassisID)
		neighbor.RemoteCapabilities = uniqueTopologyStrings(neighbor.RemoteCapabilities)
		if topologyPortNeighborStatusKey(neighbor) == "" {
			continue
		}
		out = append(out, neighbor)
	}
	sort.Slice(out, func(i, j int) bool {
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
	})
	if len(out) == 0 {
		return nil
	}
	return out
}

func topologyPortNeighborStatusToAttributes(status topologyPortNeighborStatus) map[string]any {
	attrs := map[string]any{
		"protocol":            strings.ToLower(strings.TrimSpace(status.Protocol)),
		"remote_device":       strings.TrimSpace(status.RemoteDevice),
		"remote_port":         strings.TrimSpace(status.RemotePort),
		"remote_ip":           strings.TrimSpace(status.RemoteIP),
		"remote_chassis_id":   strings.TrimSpace(status.RemoteChassisID),
		"remote_capabilities": uniqueTopologyStrings(status.RemoteCapabilities),
	}
	return pruneTopologyAttributes(attrs)
}
