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
		li := portSortKey(records[i].designatedPort) + "|" + portSortKey(records[i].port)
		lj := portSortKey(records[j].designatedPort) + "|" + portSortKey(records[j].port)
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

type fdbPortEndpointSet struct {
	port      bridgePortRef
	endpoints map[string]struct{}
}

type fdbOverlapCandidate struct {
	leftKey   string
	rightKey  string
	leftPort  bridgePortRef
	rightPort bridgePortRef
	shared    int
	score     float64
}

func inferFDBOverlapWeightedBridgeLinks(
	attachments []Attachment,
	ifaceByDeviceIndex map[string]Interface,
	reporterAliases map[string][]string,
	minShared int,
) []bridgeBridgeLinkRecord {
	if len(attachments) == 0 {
		return nil
	}
	if minShared <= 0 {
		minShared = 1
	}

	managedAliasSet := make(map[string]struct{})
	for endpointID := range buildFDBAliasOwnerMap(reporterAliases) {
		managedAliasSet[endpointID] = struct{}{}
	}

	portEndpointsByDevice := make(map[string]map[string]*fdbPortEndpointSet)
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
		// Overlap mode focuses on host-side FDB overlap and excludes managed
		// aliases that are already covered by direct reciprocal pairwise rules.
		if _, isManagedAlias := managedAliasSet[endpointID]; isManagedAlias {
			continue
		}
		port := bridgePortFromAttachment(attachment, ifaceByDeviceIndex)
		portKey := bridgePortObservationKey(port)
		if portKey == "" {
			continue
		}

		byPort := portEndpointsByDevice[reporterID]
		if byPort == nil {
			byPort = make(map[string]*fdbPortEndpointSet)
			portEndpointsByDevice[reporterID] = byPort
		}
		set := byPort[portKey]
		if set == nil {
			set = &fdbPortEndpointSet{
				port:      port,
				endpoints: make(map[string]struct{}),
			}
			byPort[portKey] = set
		}
		set.endpoints[endpointID] = struct{}{}
	}
	if len(portEndpointsByDevice) < 2 {
		return nil
	}

	deviceIDs := make([]string, 0, len(portEndpointsByDevice))
	for deviceID := range portEndpointsByDevice {
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Strings(deviceIDs)

	records := make([]bridgeBridgeLinkRecord, 0)
	seen := make(map[string]struct{})
	for leftIdx := 0; leftIdx < len(deviceIDs); leftIdx++ {
		leftID := deviceIDs[leftIdx]
		leftPorts := portEndpointsByDevice[leftID]
		if len(leftPorts) == 0 {
			continue
		}
		for rightIdx := leftIdx + 1; rightIdx < len(deviceIDs); rightIdx++ {
			rightID := deviceIDs[rightIdx]
			rightPorts := portEndpointsByDevice[rightID]
			if len(rightPorts) == 0 {
				continue
			}

			candidates := make([]fdbOverlapCandidate, 0)
			for leftKey, leftSet := range leftPorts {
				if len(leftSet.endpoints) < minShared {
					continue
				}
				for rightKey, rightSet := range rightPorts {
					if len(rightSet.endpoints) < minShared {
						continue
					}
					shared := overlapSize(leftSet.endpoints, rightSet.endpoints)
					if shared < minShared {
						continue
					}
					union := len(leftSet.endpoints) + len(rightSet.endpoints) - shared
					if union <= 0 {
						continue
					}
					score := float64(shared) + (float64(shared) / float64(union))
					candidates = append(candidates, fdbOverlapCandidate{
						leftKey:   leftKey,
						rightKey:  rightKey,
						leftPort:  leftSet.port,
						rightPort: rightSet.port,
						shared:    shared,
						score:     score,
					})
				}
			}
			if len(candidates) == 0 {
				continue
			}

			sort.SliceStable(candidates, func(i, j int) bool {
				if candidates[i].score != candidates[j].score {
					return candidates[i].score > candidates[j].score
				}
				if candidates[i].shared != candidates[j].shared {
					return candidates[i].shared > candidates[j].shared
				}
				iKey := candidates[i].leftKey + "|" + candidates[i].rightKey
				jKey := candidates[j].leftKey + "|" + candidates[j].rightKey
				return iKey < jKey
			})

			usedLeft := make(map[string]struct{})
			usedRight := make(map[string]struct{})
			for _, candidate := range candidates {
				if _, ok := usedLeft[candidate.leftKey]; ok {
					continue
				}
				if _, ok := usedRight[candidate.rightKey]; ok {
					continue
				}
				usedLeft[candidate.leftKey] = struct{}{}
				usedRight[candidate.rightKey] = struct{}{}

				designated := candidate.leftPort
				other := candidate.rightPort
				if bridgePortRefSortKey(designated) > bridgePortRefSortKey(other) {
					designated = candidate.rightPort
					other = candidate.leftPort
				}
				key := bridgePairKey(designated, other)
				if key == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				records = append(records, bridgeBridgeLinkRecord{
					port:           other,
					designatedPort: designated,
					method:         topologyInferenceStrategyFDBOverlapWeighted,
				})

				// Conservative default: one overlap-inferred bridge pair per
				// device pair to avoid over-connecting noisy FDB domains.
				break
			}
		}
	}
	if len(records) == 0 {
		return nil
	}
	sort.SliceStable(records, func(i, j int) bool {
		li := portSortKey(records[i].designatedPort) + "|" + portSortKey(records[i].port)
		lj := portSortKey(records[j].designatedPort) + "|" + portSortKey(records[j].port)
		return li < lj
	})
	return records
}

func overlapSize(left, right map[string]struct{}) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	if len(left) > len(right) {
		left, right = right, left
	}
	count := 0
	for value := range left {
		if _, ok := right[value]; ok {
			count++
		}
	}
	return count
}
