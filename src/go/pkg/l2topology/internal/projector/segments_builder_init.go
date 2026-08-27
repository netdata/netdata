// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func (b *segmentProjectionBuilder) initializeSegments() bool {
	// Hard deterministic rule: LLDP/CDP-adjacent ports are transit ports.
	// FDB data learned on those ports belongs to the neighbor domain and must
	// not create parallel inferred segment paths on top of direct discovery.
	//
	// NOTE:
	// switch-facing/trunk classification must not suppress endpoint ownership.
	// It is a topology correlation/confidence signal, not a hard endpoint
	// placement filter.
	deterministicTransitPortKeys := buildDeterministicTransitPortKeySet(
		b.adjacencies,
		b.ifIndexByDeviceName,
		b.ifaceByDeviceIndex,
		b.bridgePortAliases,
	)
	seedMacLinks := collectBridgeMacLinkRecordsWithWork(b.work, b.attachments, b.ifaceByDeviceIndex, deterministicTransitPortKeys)
	b.rawFDBObservations = buildFDBReporterObservations(seedMacLinks)
	model := buildBridgeDomainModelWithWork(b.work, b.bridgeLinks, seedMacLinks)
	if len(model.domains) == 0 {
		return false
	}

	b.segmentMatchByID = make(map[string]graph.Match)
	b.segmentByID = make(map[string]*bridgeDomainSegment)
	for _, domain := range model.domains {
		if domain == nil {
			continue
		}
		for _, segment := range domain.segments {
			if segment == nil || len(segment.endpointIDs) == 0 {
				continue
			}
			segmentID := bridgeDomainSegmentIDWithWork(b.work, segment)
			if _, exists := b.segmentByID[segmentID]; exists {
				continue
			}
			b.segmentByID[segmentID] = segment
			b.segmentIDs = append(b.segmentIDs, segmentID)
		}
	}
	if !sortProjectionStrings(b.work, b.segmentIDs) {
		return false
	}
	if len(b.segmentIDs) == 0 {
		return false
	}

	for _, segmentID := range b.segmentIDs {
		segment := b.segmentByID[segmentID]
		if segment == nil {
			continue
		}
		match, actor := buildBridgeSegmentActorWithWork(b.work, segmentID, segment, b.layer, b.source)
		keys := topologyMatchIdentityKeysWithWork(b.work, actor.Actor.Match)
		if len(keys) > 0 && !topologyIdentityIndexOverlaps(b.actorIndex, keys) {
			addTopologyIdentityKeys(b.actorIndex, keys)
		}
		b.out.actors = append(b.out.actors, actor)
		b.segmentMatchByID[segmentID] = match
	}

	b.deviceSegmentEdgeSeen = make(map[string]struct{})
	b.endpointSegmentEdgeSeen = make(map[string]struct{})
	b.endpointSegmentCandidates = make(map[string][]string)
	b.segmentPortKeys = make(map[string]map[string]struct{}, len(b.segmentIDs))
	b.segmentIfIndexes = make(map[string]map[string]struct{}, len(b.segmentIDs))
	b.segmentIfNames = make(map[string]map[string]struct{}, len(b.segmentIDs))
	for _, segmentID := range b.segmentIDs {
		segment := b.segmentByID[segmentID]
		if segment == nil {
			continue
		}
		portKeys := make(map[string]struct{}, len(segment.ports))
		ifIndexes := make(map[string]struct{}, len(segment.ports))
		ifNames := make(map[string]struct{}, len(segment.ports))
		for _, port := range segment.ports {
			if portKey := bridgePortObservationKey(port); portKey != "" {
				portKeys[portKey] = struct{}{}
			}
			if portVLANKey := bridgePortObservationVLANKey(port); portVLANKey != "" {
				portKeys[portVLANKey] = struct{}{}
			}
			if port.ifIndex > 0 {
				ifIndexes[strconv.Itoa(port.ifIndex)] = struct{}{}
			}
			if ifName := strings.TrimSpace(port.ifName); ifName != "" {
				ifNames[strings.ToLower(ifName)] = struct{}{}
			}
		}
		b.segmentPortKeys[segmentID] = portKeys
		b.segmentIfIndexes[segmentID] = ifIndexes
		b.segmentIfNames[segmentID] = ifNames
		for endpointID := range segment.endpointIDs {
			endpointID = strings.TrimSpace(endpointID)
			if endpointID == "" {
				continue
			}
			b.endpointSegmentCandidates[endpointID] = append(b.endpointSegmentCandidates[endpointID], segmentID)
		}
	}

	b.rawFDBReporterHints = buildFDBEndpointReporterHintsWithWork(b.work, seedMacLinks)
	b.fdbObservations = buildFDBReporterObservations(seedMacLinks)
	b.fdbOwners = inferFDBEndpointOwnersWithWork(b.work, b.fdbObservations, b.reporterAliases, deterministicTransitPortKeys)
	for endpointID, owner := range inferSinglePortEndpointOwnersWithWork(b.work, seedMacLinks, deterministicTransitPortKeys) {
		if strings.TrimSpace(endpointID) == "" {
			continue
		}
		if b.fdbOwners == nil {
			b.fdbOwners = make(map[string]fdbEndpointOwner)
		}
		// Port-centric ownership has precedence: if a port carries a single learned
		// MAC in the same snapshot/VLAN scope, use it to resolve ambiguous placement.
		b.fdbOwners[endpointID] = owner
		if b.out.endpointDirectOwners == nil {
			b.out.endpointDirectOwners = make(map[string]fdbEndpointOwner)
		}
		b.out.endpointDirectOwners[endpointID] = owner
	}

	b.deviceIdentityByID = buildDeviceIdentityKeySetByID(b.work, b.deviceByID, b.adjacencies, b.ifaceByDeviceIndex)
	b.reporterSegmentIndex = buildSegmentReporterIndexWithWork(b.work, b.segmentIDs, b.segmentByID)
	b.aliasOwnerIDs = buildFDBAliasOwnerMapWithWork(b.work, b.reporterAliases)
	b.managedDeviceIDs = make(map[string]struct{}, len(b.deviceByID))
	for deviceID := range b.deviceByID {
		deviceID = strings.TrimSpace(deviceID)
		if deviceID == "" {
			continue
		}
		b.managedDeviceIDs[deviceID] = struct{}{}
	}
	b.managedDeviceIDList = sortedTopologySetWithWork(b.work, b.managedDeviceIDs)
	b.allowedEndpointBySegment = make(map[string]map[string]struct{})
	b.strictEndpointBySegment = make(map[string]map[string]struct{})
	b.probableEndpointBySegment = make(map[string]map[string]struct{})
	b.probableAttachmentModes = make(map[string]map[string]string)
	b.assignedEndpoints = make(map[string]struct{}, len(b.endpointMatchByID))

	return true
}
