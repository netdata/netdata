// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func buildBridgeSegmentActor(segmentID string, segment *bridgeDomainSegment, layer string, source string) (graph.Match, projectedActor) {
	return buildBridgeSegmentActorWithWork(nil, segmentID, segment, layer, source)
}

func buildBridgeSegmentActorWithWork(work *projectionWork, segmentID string, segment *bridgeDomainSegment, layer string, source string) (graph.Match, projectedActor) {
	parentDevices := make(map[string]struct{})
	ifNames := make(map[string]struct{})
	ifIndexes := make(map[string]struct{})
	bridgePorts := make(map[string]struct{})
	vlanIDs := make(map[string]struct{})
	if segment != nil {
		if !work.charge(uint64(len(segment.ports))) {
			return graph.Match{}, projectedActor{}
		}
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

	detail := model.ProjectionSegmentActorDetail{
		SegmentID:     strings.TrimSpace(segmentID),
		SegmentType:   "broadcast_domain",
		ParentDevices: sortedTopologySetWithWork(work, parentDevices),
		IfNames:       sortedTopologySetWithWork(work, ifNames),
		IfIndexes:     sortedTopologySetWithWork(work, ifIndexes),
		BridgePorts:   sortedTopologySetWithWork(work, bridgePorts),
		VLANIDs:       sortedTopologySetWithWork(work, vlanIDs),
		SegmentKind:   "broadcast_domain",
	}
	if segment != nil {
		detail.LearnedSources = sortedTopologySetWithWork(work, segment.methods)
		detail.PortsTotal = model.OptionalValue[int]{Value: len(segment.ports), Has: true}
		detail.EndpointsTotal = model.OptionalValue[int]{Value: len(segment.endpointIDs), Has: true}
		if bridgePortRefKey(segment.designatedPort, false, false) != "" {
			detail.DesignatedPort = bridgePortRefDisplayKey(segment.designatedPort)
		}
	}

	actor := projectedActor{
		Actor: graph.Actor{
			ActorType: "segment",
			Layer:     layer,
			Source:    source,
			Match:     match,
		},
		Detail: model.ProjectionActorDetail{
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
	work *projectionWork,
	actors []projectedActor,
	endpointMatchByID map[string]graph.Match,
	owners map[string]fdbEndpointOwner,
	deviceByID map[string]model.Device,
) {
	if len(actors) == 0 || len(owners) == 0 {
		return
	}

	ownerByMatchKey := make(map[string]fdbEndpointOwner, len(owners))
	var endpointIDs []string
	if work == nil {
		endpointIDs = make([]string, 0, len(owners))
	}
	endpointIDs = sortedProjectionKeys(work, owners, endpointIDs)

	for _, endpointID := range endpointIDs {
		owner := owners[endpointID]
		if !strings.EqualFold(strings.TrimSpace(owner.source), "single_port_mac") {
			continue
		}
		match, ok := endpointMatchByID[endpointID]
		if !ok {
			match = endpointMatchFromID(endpointID)
		}
		key := canonicalTopologyMatchKeyWithWork(work, match)
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
		key := canonicalTopologyMatchKeyWithWork(work, actor.Actor.Match)
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
