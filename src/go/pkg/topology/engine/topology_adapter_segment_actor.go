// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func buildBridgeSegmentActor(segmentID string, segment *bridgeDomainSegment, layer string, source string) (topology.Match, topology.Actor) {
	parentDevices := make(map[string]struct{})
	ifNames := make(map[string]struct{})
	ifIndexes := make(map[string]struct{})
	bridgePorts := make(map[string]struct{})
	vlanIDs := make(map[string]struct{})
	if segment != nil {
		for _, port := range segment.ports {
			if strings.TrimSpace(port.deviceID) != "" {
				parentDevices[port.deviceID] = struct{}{}
			}
			if strings.TrimSpace(port.ifName) != "" {
				ifNames[port.ifName] = struct{}{}
			}
			if port.ifIndex > 0 {
				ifIndexes[strconv.Itoa(port.ifIndex)] = struct{}{}
			}
			if strings.TrimSpace(port.bridgePort) != "" {
				bridgePorts[port.bridgePort] = struct{}{}
			}
			if strings.TrimSpace(port.vlanID) != "" {
				vlanIDs[port.vlanID] = struct{}{}
			}
		}
	}

	match := topology.Match{
		Hostnames: []string{"segment:" + segmentID},
	}

	attrs := map[string]any{
		"segment_id":      segmentID,
		"segment_type":    "broadcast_domain",
		"parent_devices":  sortedTopologySet(parentDevices),
		"if_names":        sortedTopologySet(ifNames),
		"if_indexes":      sortedTopologySet(ifIndexes),
		"bridge_ports":    sortedTopologySet(bridgePorts),
		"vlan_ids":        sortedTopologySet(vlanIDs),
		"ports_total":     0,
		"endpoints_total": 0,
	}
	if segment != nil {
		attrs["learned_sources"] = sortedTopologySet(segment.methods)
		attrs["ports_total"] = len(segment.ports)
		attrs["endpoints_total"] = len(segment.endpointIDs)
		if bridgePortRefKey(segment.designatedPort, false, false) != "" {
			attrs["designated_port"] = bridgePortRefSortKey(segment.designatedPort)
		}
	}

	actor := topology.Actor{
		ActorType:  "segment",
		Layer:      layer,
		Source:     source,
		Match:      match,
		Attributes: pruneTopologyAttributes(attrs),
		Labels: map[string]string{
			"segment_kind": "broadcast_domain",
		},
	}

	return match, actor
}

func endpointMatchFromID(endpointID string) topology.Match {
	kind, value, ok := strings.Cut(strings.TrimSpace(endpointID), ":")
	if !ok {
		return topology.Match{}
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "mac":
		mac := normalizeMAC(value)
		if mac == "" {
			return topology.Match{}
		}
		return topology.Match{
			ChassisIDs:   []string{mac},
			MacAddresses: []string{mac},
		}
	case "ip":
		addr := normalizeTopologyIP(value)
		if addr == "" {
			return topology.Match{}
		}
		return topology.Match{
			IPAddresses: []string{addr},
		}
	}
	return topology.Match{}
}

func annotateEndpointActorsWithDirectOwners(
	actors []topology.Actor,
	endpointMatchByID map[string]topology.Match,
	owners map[string]fdbEndpointOwner,
	deviceByID map[string]Device,
) {
	if len(actors) == 0 || len(owners) == 0 {
		return
	}

	ownerByMatchKey := make(map[string]fdbEndpointOwner, len(owners))
	endpointIDs := make([]string, 0, len(owners))
	for endpointID := range owners {
		endpointIDs = append(endpointIDs, endpointID)
	}
	sort.Strings(endpointIDs)

	for _, endpointID := range endpointIDs {
		owner := owners[endpointID]
		if !strings.EqualFold(strings.TrimSpace(owner.source), "single_port_mac") {
			continue
		}
		match, ok := endpointMatchByID[endpointID]
		if !ok {
			match = endpointMatchFromID(endpointID)
		}
		key := canonicalTopologyMatchKey(match)
		if key == "" {
			continue
		}
		ownerByMatchKey[key] = owner
	}

	if len(ownerByMatchKey) == 0 {
		return
	}

	for i := range actors {
		actor := &actors[i]
		if !strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
			continue
		}
		key := canonicalTopologyMatchKey(actor.Match)
		if key == "" {
			continue
		}
		owner, ok := ownerByMatchKey[key]
		if !ok {
			continue
		}

		attrs := cloneAnyMap(actor.Attributes)
		if attrs == nil {
			attrs = make(map[string]any)
		}
		labels := cloneStringMap(actor.Labels)
		if labels == nil {
			labels = make(map[string]string)
		}

		deviceID := strings.TrimSpace(owner.port.deviceID)
		port := bridgePortDisplay(owner.port)
		ifName := strings.TrimSpace(owner.port.ifName)
		bridgePort := strings.TrimSpace(owner.port.bridgePort)
		vlanID := strings.TrimSpace(owner.port.vlanID)

		attrs["attachment_source"] = "single_port_mac"
		if deviceID != "" {
			attrs["attached_device_id"] = deviceID
			labels["attached_device_id"] = deviceID
		}
		if port != "" {
			attrs["attached_port"] = port
			labels["attached_port"] = port
		}
		if ifName != "" {
			attrs["attached_if_name"] = ifName
		}
		if owner.port.ifIndex > 0 {
			attrs["attached_if_index"] = owner.port.ifIndex
		}
		if bridgePort != "" {
			attrs["attached_bridge_port"] = bridgePort
		}
		if vlanID != "" {
			attrs["attached_vlan"] = vlanID
			attrs["attached_vlan_id"] = vlanID
		}
		if device, ok := deviceByID[deviceID]; ok {
			display := strings.TrimSpace(device.Hostname)
			if display == "" {
				display = deviceID
			}
			if display != "" {
				attrs["attached_device"] = display
				labels["attached_device"] = display
			}
		}
		labels["attached_by"] = "single_port_mac"

		actor.Attributes = pruneTopologyAttributes(attrs)
		if len(labels) > 0 {
			actor.Labels = labels
		}
	}
}

func segmentContainsDevice(segment *bridgeDomainSegment, deviceID string) bool {
	if segment == nil {
		return false
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return false
	}
	for _, port := range segment.ports {
		if strings.EqualFold(strings.TrimSpace(port.deviceID), deviceID) {
			return true
		}
	}
	return false
}
