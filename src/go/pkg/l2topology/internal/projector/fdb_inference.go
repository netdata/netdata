// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

func inferFDBPairwiseBridgeLinks(
	attachments []model.Attachment,
	ifaceByDeviceIndex map[string]model.Interface,
	reporterAliases map[string][]string,
) []bridgeBridgeLinkRecord {
	if len(attachments) == 0 || len(reporterAliases) == 0 {
		return nil
	}

	aliasOwnerIDs := buildFDBAliasOwnerMap(reporterAliases)
	if len(aliasOwnerIDs) == 0 {
		return nil
	}

	macLinks := collectBridgeMacLinkRecords(attachments, ifaceByDeviceIndex, nil)
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
			leftByScope := pairs[leftID][rightID]
			rightByScope := pairs[rightID][leftID]
			for _, scope := range compatiblePairwiseScopes(leftByScope, rightByScope) {
				leftPorts := leftByScope[scope]
				rightPorts := rightByScope[scope]
				if len(leftPorts) != 1 || len(rightPorts) != 1 {
					// Conservative rule: infer a bridge segment only when each side reports
					// exactly one reciprocal managed-alias learning port in this scope.
					continue
				}
				leftPort := firstSortedBridgePort(leftPorts)
				rightPort := firstSortedBridgePort(rightPorts)
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
	left, right map[string]map[string]bridgePortRef,
) []string {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	scopes := make([]string, 0, len(left))
	for scope := range left {
		if _, ok := right[scope]; ok {
			scopes = append(scopes, scope)
		}
	}
	sort.Strings(scopes)
	return scopes
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
