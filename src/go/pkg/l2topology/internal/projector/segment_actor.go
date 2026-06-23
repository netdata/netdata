// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func buildBridgeSegmentActor(segmentID string, segment *bridgeDomainSegment, layer string, source string) (graph.Match, projectedActor) {
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

	match := graph.Match{
		Hostnames: []string{"segment:" + segmentID},
	}

	detail := ProjectionSegmentActorDetail{
		SegmentID:     strings.TrimSpace(segmentID),
		SegmentType:   "broadcast_domain",
		ParentDevices: sortedTopologySet(parentDevices),
		IfNames:       sortedTopologySet(ifNames),
		IfIndexes:     sortedTopologySet(ifIndexes),
		BridgePorts:   sortedTopologySet(bridgePorts),
		VLANIDs:       sortedTopologySet(vlanIDs),
		SegmentKind:   "broadcast_domain",
	}
	if segment != nil {
		detail.LearnedSources = sortedTopologySet(segment.methods)
		detail.PortsTotal = OptionalValue[int]{Value: len(segment.ports), Has: true}
		detail.EndpointsTotal = OptionalValue[int]{Value: len(segment.endpointIDs), Has: true}
		if bridgePortRefKey(segment.designatedPort, false, false) != "" {
			detail.DesignatedPort = bridgePortRefSortKey(segment.designatedPort)
		}
	}

	actor := projectedActor{
		Actor: graph.Actor{
			ActorType: "segment",
			Layer:     layer,
			Source:    source,
			Match:     match,
		},
		Detail: ProjectionActorDetail{
			Segment: detail,
		},
	}

	return match, actor
}

func endpointMatchFromID(endpointID string) graph.Match {
	kind, value, ok := strings.Cut(strings.TrimSpace(endpointID), ":")
	if !ok {
		return graph.Match{}
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "mac":
		mac := normalizeMAC(value)
		if mac == "" {
			return graph.Match{}
		}
		return graph.Match{
			ChassisIDs:   []string{mac},
			MacAddresses: []string{mac},
		}
	case "ip":
		addr := normalizeTopologyIP(value)
		if addr == "" {
			return graph.Match{}
		}
		return graph.Match{
			IPAddresses: []string{addr},
		}
	}
	return graph.Match{}
}

func annotateEndpointActorsWithDirectOwners(
	actors []projectedActor,
	endpointMatchByID map[string]graph.Match,
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
		if !strings.EqualFold(strings.TrimSpace(actor.Actor.ActorType), "endpoint") {
			continue
		}
		key := canonicalTopologyMatchKey(actor.Actor.Match)
		if key == "" {
			continue
		}
		owner, ok := ownerByMatchKey[key]
		if !ok {
			continue
		}

		deviceID := strings.TrimSpace(owner.port.deviceID)
		port := bridgePortDisplay(owner.port)
		ifName := strings.TrimSpace(owner.port.ifName)
		bridgePort := strings.TrimSpace(owner.port.bridgePort)
		vlanID := strings.TrimSpace(owner.port.vlanID)

		detail := &actor.Detail.Endpoint
		detail.AttachmentSource = "single_port_mac"
		if deviceID != "" {
			detail.AttachedDeviceID = deviceID
		}
		if port != "" {
			detail.AttachedPort = port
		}
		if ifName != "" {
			detail.AttachedIfName = ifName
		}
		if owner.port.ifIndex > 0 {
			detail.AttachedIfIndex = owner.port.ifIndex
		}
		if bridgePort != "" {
			detail.AttachedBridgePort = bridgePort
		}
		if vlanID != "" {
			detail.AttachedVLAN = vlanID
			detail.AttachedVLANID = vlanID
		}
		if device, ok := deviceByID[deviceID]; ok {
			display := strings.TrimSpace(device.Hostname)
			if display == "" {
				display = deviceID
			}
			if display != "" {
				detail.AttachedDevice = display
			}
		}
		detail.AttachedBy = "single_port_mac"
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
