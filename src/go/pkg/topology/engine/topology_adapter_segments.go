// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func projectSegmentTopology(
	attachments []Attachment,
	adjacencies []Adjacency,
	layer string,
	source string,
	collectedAt time.Time,
	deviceByID map[string]Device,
	ifaceByDeviceIndex map[string]Interface,
	ifIndexByDeviceName map[string]int,
	bridgeLinks []bridgeBridgeLinkRecord,
	reporterAliases map[string][]string,
	endpointMatchByID map[string]topology.Match,
	endpointLabelsByID map[string]map[string]string,
	actorIndex map[string]struct{},
	probabilisticConnectivity bool,
	strategyConfig topologyInferenceStrategyConfig,
) projectedSegments {
	return newSegmentProjectionBuilder(
		attachments,
		adjacencies,
		layer,
		source,
		collectedAt,
		deviceByID,
		ifaceByDeviceIndex,
		ifIndexByDeviceName,
		bridgeLinks,
		reporterAliases,
		endpointMatchByID,
		endpointLabelsByID,
		actorIndex,
		probabilisticConnectivity,
		strategyConfig,
	).build()
}

func pickProbableSegmentAnchorPortID(
	segment *bridgeDomainSegment,
	probableEndpoints map[string]struct{},
	fdbOwners map[string]fdbEndpointOwner,
	managedDeviceIDs map[string]struct{},
) string {
	if segment == nil || len(segment.ports) == 0 {
		return ""
	}

	portIDs := make([]string, 0, len(segment.ports))
	portIDByObservation := make(map[string]string, len(segment.ports)*2)
	designatedPortID := ""
	managedPortIDs := make(map[string]struct{})
	for portID, port := range segment.ports {
		portIDs = append(portIDs, portID)
		if key := bridgePortObservationKey(port); key != "" {
			portIDByObservation[key] = portID
		}
		if key := bridgePortObservationVLANKey(port); key != "" {
			portIDByObservation[key] = portID
		}
		if segment.portIdentityKey(port) == segment.portIdentityKey(segment.designatedPort) {
			designatedPortID = portID
		}
		if len(managedDeviceIDs) == 0 {
			managedPortIDs[portID] = struct{}{}
			continue
		}
		if _, ok := managedDeviceIDs[strings.TrimSpace(port.deviceID)]; ok {
			managedPortIDs[portID] = struct{}{}
		}
	}
	sort.Strings(portIDs)
	preferManaged := len(managedPortIDs) > 0
	allowPortID := func(portID string) bool {
		if !preferManaged {
			return true
		}
		_, ok := managedPortIDs[portID]
		return ok
	}

	endpointIDs := sortedTopologySet(probableEndpoints)
	for _, endpointID := range endpointIDs {
		owner, ok := fdbOwners[endpointID]
		if !ok {
			continue
		}
		if portID, ok := portIDByObservation[owner.portVLANKey]; ok {
			if allowPortID(portID) {
				return portID
			}
		}
		if portID, ok := portIDByObservation[owner.portKey]; ok {
			if allowPortID(portID) {
				return portID
			}
		}
	}

	if designatedPortID != "" && allowPortID(designatedPortID) {
		return designatedPortID
	}
	if preferManaged {
		managedPortIDList := make([]string, 0, len(managedPortIDs))
		for portID := range managedPortIDs {
			managedPortIDList = append(managedPortIDList, portID)
		}
		sort.Strings(managedPortIDList)
		if len(managedPortIDList) > 0 {
			return managedPortIDList[0]
		}
	}
	if designatedPortID != "" {
		return designatedPortID
	}
	return portIDs[0]
}

func segmentHasManagedPort(segment *bridgeDomainSegment, managedDeviceIDs map[string]struct{}) bool {
	if segment == nil || len(segment.ports) == 0 {
		return false
	}
	if len(managedDeviceIDs) == 0 {
		return true
	}
	for _, port := range segment.ports {
		if _, ok := managedDeviceIDs[strings.TrimSpace(port.deviceID)]; ok {
			return true
		}
	}
	return false
}

func pickMostProbableSegment(
	candidates []string,
	endpointLabels map[string]string,
	segmentIfIndexes map[string]map[string]struct{},
	segmentIfNames map[string]map[string]struct{},
) string {
	if len(candidates) == 0 {
		return ""
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	ifIndexes := make(map[string]struct{})
	for _, ifIndex := range labelsCSVToSlice(endpointLabels, "learned_if_indexes") {
		ifIndex = strings.TrimSpace(ifIndex)
		if ifIndex == "" {
			continue
		}
		ifIndexes[ifIndex] = struct{}{}
	}
	ifNames := make(map[string]struct{})
	for _, ifName := range labelsCSVToSlice(endpointLabels, "learned_if_names") {
		ifName = strings.ToLower(strings.TrimSpace(ifName))
		if ifName == "" {
			continue
		}
		ifNames[ifName] = struct{}{}
	}

	bestID := ""
	bestScore := -1
	for _, segmentID := range candidates {
		score := 0
		for ifIndex := range ifIndexes {
			if indexes := segmentIfIndexes[segmentID]; indexes != nil {
				if _, ok := indexes[ifIndex]; ok {
					score += 2
				}
			}
		}
		for ifName := range ifNames {
			if names := segmentIfNames[segmentID]; names != nil {
				if _, ok := names[ifName]; ok {
					score++
				}
			}
		}
		if score > bestScore {
			bestScore = score
			bestID = segmentID
			continue
		}
		if score == bestScore && (bestID == "" || segmentID < bestID) {
			bestID = segmentID
		}
	}
	if bestID != "" {
		return bestID
	}
	return candidates[0]
}
