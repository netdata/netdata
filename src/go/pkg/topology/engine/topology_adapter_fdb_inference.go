// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"
)

func inferFDBPairwiseBridgeLinks(
	attachments []Attachment,
	ifaceByDeviceIndex map[string]Interface,
	reporterAliases map[string][]string,
) []bridgeBridgeLinkRecord {
	if len(attachments) == 0 || len(reporterAliases) == 0 {
		return nil
	}

	aliasOwnerIDs := buildFDBAliasOwnerMap(reporterAliases)
	if len(aliasOwnerIDs) == 0 {
		return nil
	}

	// reporterA -> reporterB -> unique reporter ports where A learns aliases of B.
	pairs := make(map[string]map[string]map[string]bridgePortRef)
	for _, attachment := range attachments {
		if !strings.EqualFold(strings.TrimSpace(attachment.Method), "fdb") {
			continue
		}
		reporterID := strings.TrimSpace(attachment.DeviceID)
		if reporterID == "" {
			continue
		}
		endpointID := normalizeFDBEndpointID(attachment.EndpointID)
		if endpointID == "" {
			continue
		}
		owners := aliasOwnerIDs[endpointID]
		if len(owners) == 0 {
			continue
		}
		port := bridgePortFromAttachment(attachment, ifaceByDeviceIndex)
		portKey := bridgePortObservationKey(port)
		if portKey == "" {
			continue
		}
		for ownerID := range owners {
			ownerID = strings.TrimSpace(ownerID)
			if ownerID == "" || strings.EqualFold(ownerID, reporterID) {
				continue
			}
			byPeer := pairs[reporterID]
			if byPeer == nil {
				byPeer = make(map[string]map[string]bridgePortRef)
				pairs[reporterID] = byPeer
			}
			ports := byPeer[ownerID]
			if ports == nil {
				ports = make(map[string]bridgePortRef)
				byPeer[ownerID] = ports
			}
			ports[portKey] = port
		}
	}
	if len(pairs) == 0 {
		return nil
	}

	records := make([]bridgeBridgeLinkRecord, 0)
	seen := make(map[string]struct{})
	leftIDs := make([]string, 0, len(pairs))
	for leftID := range pairs {
		leftIDs = append(leftIDs, leftID)
	}
	sort.Strings(leftIDs)
	for _, leftID := range leftIDs {
		neighbors := pairs[leftID]
		if len(neighbors) == 0 {
			continue
		}
		rightIDs := make([]string, 0, len(neighbors))
		for rightID := range neighbors {
			rightIDs = append(rightIDs, rightID)
		}
		sort.Strings(rightIDs)
		for _, rightID := range rightIDs {
			if leftID >= rightID {
				continue
			}
			leftPorts := pairs[leftID][rightID]
			rightPorts := pairs[rightID][leftID]
			if len(leftPorts) != 1 || len(rightPorts) != 1 {
				// Conservative rule: infer direct bridge link only when each side reports
				// exactly one reciprocal managed-alias learning port.
				continue
			}
			leftPort := firstSortedBridgePort(leftPorts)
			rightPort := firstSortedBridgePort(rightPorts)
			if bridgePortObservationKey(leftPort) == "" || bridgePortObservationKey(rightPort) == "" {
				continue
			}
			key := bridgePairKey(leftPort, rightPort)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}

			designated := leftPort
			other := rightPort
			if bridgePortRefSortKey(leftPort) > bridgePortRefSortKey(rightPort) {
				designated = rightPort
				other = leftPort
			}
			records = append(records, bridgeBridgeLinkRecord{
				port:           other,
				designatedPort: designated,
				method:         "fdb_pairwise",
			})
		}
	}
	if len(records) == 0 {
		return nil
	}
	sort.SliceStable(records, func(i, j int) bool {
		li := portSortKey(records[i].designatedPort) + keySep + portSortKey(records[i].port)
		lj := portSortKey(records[j].designatedPort) + keySep + portSortKey(records[j].port)
		return li < lj
	})
	return records
}

func firstSortedBridgePort(ports map[string]bridgePortRef) bridgePortRef {
	if len(ports) == 0 {
		return bridgePortRef{}
	}
	keys := make([]string, 0, len(ports))
	for key := range ports {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return ports[keys[0]]
}
