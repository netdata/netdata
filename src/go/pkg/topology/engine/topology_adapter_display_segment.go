// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

type topologySegmentPortRef struct {
	deviceID   string
	ifName     string
	ifIndex    string
	bridgePort string
}

func topologySegmentDisplayName(actor topology.Actor, deviceDisplayByID map[string]string) string {
	attrs := actor.Attributes
	if len(attrs) == 0 {
		return ""
	}

	ref := parseTopologySegmentPortRef(topologyAttrString(attrs, "designated_port"))
	parent := topologySegmentParentDisplayName(ref.deviceID, deviceDisplayByID)
	port := topologySegmentPortDisplay(ref)
	if parent != "" && port != "" {
		return parent + "." + port + ".segment"
	}

	parentCandidates := make(map[string]struct{})
	for _, candidate := range topologyAttrStringSlice(attrs, "parent_devices") {
		if value := topologySegmentParentDisplayName(candidate, deviceDisplayByID); value != "" {
			parentCandidates[value] = struct{}{}
		}
	}
	portCandidates := make(map[string]struct{})
	for _, candidate := range topologyAttrStringSlice(attrs, "if_names") {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			portCandidates[candidate] = struct{}{}
		}
	}
	if len(portCandidates) == 0 {
		for _, candidate := range topologyAttrStringSlice(attrs, "bridge_ports") {
			if candidate = strings.TrimSpace(candidate); candidate != "" {
				portCandidates[candidate] = struct{}{}
			}
		}
	}
	parents := sortedTopologySet(parentCandidates)
	ports := sortedTopologySet(portCandidates)
	if len(parents) > 0 && len(ports) > 0 {
		return parents[0] + "." + ports[0] + ".segment"
	}

	return topologyCompactSegmentID(topologyAttrString(attrs, "segment_id"))
}

func parseTopologySegmentPortRef(raw string) topologySegmentPortRef {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return topologySegmentPortRef{}
	}
	parts := strings.Split(raw, keySep)
	if len(parts) == 0 {
		return topologySegmentPortRef{}
	}
	ref := topologySegmentPortRef{
		deviceID: strings.TrimSpace(parts[0]),
	}
	for _, part := range parts[1:] {
		switch {
		case strings.HasPrefix(part, "name:"):
			ref.ifName = strings.TrimSpace(strings.TrimPrefix(part, "name:"))
		case strings.HasPrefix(part, "if:"):
			ref.ifIndex = strings.TrimSpace(strings.TrimPrefix(part, "if:"))
		case strings.HasPrefix(part, "bp:"):
			ref.bridgePort = strings.TrimSpace(strings.TrimPrefix(part, "bp:"))
		}
	}
	return ref
}

func topologySegmentPortDisplay(ref topologySegmentPortRef) string {
	if name := strings.TrimSpace(ref.ifName); name != "" {
		return name
	}
	if name := strings.TrimSpace(ref.bridgePort); name != "" {
		return name
	}
	if index := strings.TrimSpace(ref.ifIndex); index != "" && index != "0" {
		return index
	}
	return ""
}

func topologySegmentParentDisplayName(raw string, deviceDisplayByID map[string]string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if deviceDisplayByID != nil {
		if display := strings.TrimSpace(deviceDisplayByID[raw]); display != "" {
			return display
		}
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "management_ip:"):
		return strings.TrimSpace(raw[len("management_ip:"):])
	case strings.HasPrefix(lower, "macaddress:"):
		if mac := normalizeMAC(raw[len("macAddress:"):]); mac != "" {
			return mac
		}
		return strings.TrimSpace(raw[len("macAddress:"):])
	}
	return raw
}

func topologyCompactSegmentID(segmentID string) string {
	segmentID = strings.TrimSpace(segmentID)
	if segmentID == "" {
		return ""
	}
	const max = 48
	runes := []rune(segmentID)
	if len(runes) <= max {
		return segmentID
	}
	return string(runes[:max]) + "..."
}
