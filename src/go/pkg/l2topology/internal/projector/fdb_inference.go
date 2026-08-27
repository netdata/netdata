// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

func inferFDBPairwiseBridgeLinks(
	attachments []model.Attachment,
	ifaceByDeviceIndex map[string]model.Interface,
	reporterAliases map[string][]string,
) []bridgeBridgeLinkRecord {
	return inferFDBPairwiseBridgeLinksWithWork(nil, attachments, ifaceByDeviceIndex, reporterAliases)
}

func inferFDBPairwiseBridgeLinksWithWork(
	work *projectionWork,
	attachments []model.Attachment,
	ifaceByDeviceIndex map[string]model.Interface,
	reporterAliases map[string][]string,
) []bridgeBridgeLinkRecord {
	if len(attachments) == 0 || len(reporterAliases) == 0 {
		return nil
	}
	if !work.chargeProduct(uint64(len(attachments)), uint64(len(reporterAliases))) {
		return nil
	}

	aliasOwnerIDs := buildFDBAliasOwnerMapWithWork(work, reporterAliases)
	if len(aliasOwnerIDs) == 0 {
		return nil
	}

	macLinks := collectBridgeMacLinkRecordsWithWork(work, attachments, ifaceByDeviceIndex, nil)
	vlanAliases := buildBridgeVLANAliasIndex(macLinks)

	// reporterA -> reporterB -> compatible scope -> unique canonical reporter
	// ports where A learns aliases of B.
	pairs := make(map[string]map[string]map[string]map[string]bridgePortRef)
	for _, link := range macLinks {
		if !strings.EqualFold(strings.TrimSpace(link.method), "fdb") {
			continue
		}
		reporterID := strings.TrimSpace(link.port.deviceID)
		if reporterID == "" {
			continue
		}
		endpointID := normalizeFDBEndpointID(link.endpointID)
		if endpointID == "" {
			continue
		}
		owners := aliasOwnerIDs[endpointID]
		if len(owners) == 0 {
			continue
		}
		port := link.port
		portKey := bridgePortObservationKey(port)
		scope := pairwiseCorrelationScope(port, vlanAliases)
		if portKey == "" || scope == "" {
			continue
		}
		for ownerID := range owners {
			ownerID = strings.TrimSpace(ownerID)
			if ownerID == "" || strings.EqualFold(ownerID, reporterID) {
				continue
			}
			byPeer := pairs[reporterID]
			if byPeer == nil {
				byPeer = make(map[string]map[string]map[string]bridgePortRef)
				pairs[reporterID] = byPeer
			}
			byScope := byPeer[ownerID]
			if byScope == nil {
				byScope = make(map[string]map[string]bridgePortRef)
				byPeer[ownerID] = byScope
			}
			ports := byScope[scope]
			if ports == nil {
				ports = make(map[string]bridgePortRef)
				byScope[scope] = ports
			}
			ports[portKey] = port
		}
	}
	if len(pairs) == 0 {
		return nil
	}

	records := make([]bridgeBridgeLinkRecord, 0)
	seen := make(map[string]struct{})
	var leftIDs []string
	if work == nil {
		leftIDs = make([]string, 0, len(pairs))
	}
	leftIDs = sortedProjectionKeys(work, pairs, leftIDs)
	for _, leftID := range leftIDs {
		neighbors := pairs[leftID]
		if len(neighbors) == 0 {
			continue
		}
		var rightIDs []string
		if work == nil {
			rightIDs = make([]string, 0, len(neighbors))
		}
		rightIDs = sortedProjectionKeys(work, neighbors, rightIDs)
		for _, rightID := range rightIDs {
			if leftID >= rightID {
				continue
			}
			leftByScope := pairs[leftID][rightID]
			rightByScope := pairs[rightID][leftID]
			for _, scope := range compatiblePairwiseScopes(work, leftByScope, rightByScope) {
				leftPorts := leftByScope[scope]
				rightPorts := rightByScope[scope]
				if len(leftPorts) != 1 || len(rightPorts) != 1 {
					// Conservative rule: infer a bridge segment only when each side reports
					// exactly one reciprocal managed-alias learning port in this scope.
					continue
				}
				leftPort := onlyBridgePort(leftPorts)
				rightPort := onlyBridgePort(rightPorts)
				if bridgePortObservationKey(leftPort) == "" || bridgePortObservationKey(rightPort) == "" {
					continue
				}
				key := bridgeScopedPairKey(leftPort, rightPort)
				if key == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}

				leftSortKey, leftOK := bridgePortRefSortKeyWithWork(work, leftPort)
				rightSortKey, rightOK := bridgePortRefSortKeyWithWork(work, rightPort)
				if !leftOK || !rightOK {
					return nil
				}
				designated := leftPort
				other := rightPort
				if leftSortKey > rightSortKey {
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
	}
	if len(records) == 0 {
		return nil
	}
	if !sortProjectionByPreparedStringKeyStable(work, records, bridgeBridgeLinkRecordSortKeyWithWork) {
		return nil
	}
	return records
}

func pairwiseCorrelationScope(port bridgePortRef, aliases bridgeVLANAliasIndex) string {
	rawScope := bridgePortForwardingDomain(port)
	vlanScope := bridgePortVLANScope(port)
	if rawScope == "" && vlanScope == "" {
		return "domainless"
	}
	if vlanScope == "" {
		return ""
	}
	if rawScope == vlanScope || aliases.uniqueAliasKey(port) != "" {
		return vlanScope
	}
	return ""
}

func compatiblePairwiseScopes(
	work *projectionWork,
	left, right map[string]map[string]bridgePortRef,
) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	if !work.charge(uint64(len(left))) {
		return nil
	}
	scopes := make([]string, 0, len(left))
	for scope := range left {
		if _, ok := right[scope]; ok {
			scopes = append(scopes, scope)
		}
	}
	if !sortProjectionStrings(work, scopes) {
		return nil
	}
	return scopes
}

func onlyBridgePort(ports map[string]bridgePortRef) bridgePortRef {
	if len(ports) != 1 {
		return bridgePortRef{}
	}
	for _, port := range ports {
		return port
	}
	return bridgePortRef{}
}
