// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

func buildDeterministicDiscoveryDevicePairSet(adjacencies []model.Adjacency) map[string]struct{} {
	return buildDeterministicDiscoveryDevicePairSetWithWork(nil, adjacencies)
}

func buildDeterministicDiscoveryDevicePairSetWithWork(work *projectionWork, adjacencies []model.Adjacency) map[string]struct{} {
	if len(adjacencies) == 0 {
		return nil
	}
	if !work.chargeProduct(uint64(len(adjacencies)), 2) {
		return nil
	}

	out := make(map[string]struct{}, len(adjacencies))
	for _, adj := range adjacencies {
		if !chargeDeterministicAdjacencyWork(work, adj) {
			return nil
		}
		protocol := strings.ToLower(strings.TrimSpace(adj.Protocol))
		if protocol != "lldp" && protocol != "cdp" {
			continue
		}

		left := strings.TrimSpace(adj.SourceID)
		right := strings.TrimSpace(adj.TargetID)
		if left == "" || right == "" {
			continue
		}
		if pair := topologyUndirectedPairKey(left, right); pair != "" {
			out[pair] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func suppressInferredBridgeLinksOnDeterministicDiscovery(
	bridgeLinks []bridgeBridgeLinkRecord,
	deterministicTransitPortKeys map[string]struct{},
	discoveryDevicePairs map[string]struct{},
) []bridgeBridgeLinkRecord {
	return suppressInferredBridgeLinksOnDeterministicDiscoveryWithWork(
		nil,
		bridgeLinks,
		deterministicTransitPortKeys,
		discoveryDevicePairs,
	)
}

func suppressInferredBridgeLinksOnDeterministicDiscoveryWithWork(
	work *projectionWork,
	bridgeLinks []bridgeBridgeLinkRecord,
	deterministicTransitPortKeys map[string]struct{},
	discoveryDevicePairs map[string]struct{},
) []bridgeBridgeLinkRecord {
	if len(bridgeLinks) == 0 {
		return bridgeLinks
	}
	if !work.chargeProduct(uint64(len(bridgeLinks)), 2) {
		return nil
	}

	filtered := make([]bridgeBridgeLinkRecord, 0, len(bridgeLinks))
	for _, link := range bridgeLinks {
		if work != nil && !work.chargeStrings([]string{
			link.method,
			link.designatedPort.deviceID, link.designatedPort.ifName, link.designatedPort.bridgePort, link.designatedPort.vlanID,
			link.port.deviceID, link.port.ifName, link.port.bridgePort, link.port.vlanID,
		}) {
			return nil
		}
		method := strings.ToLower(strings.TrimSpace(link.method))
		if method == "lldp" || method == "cdp" {
			filtered = append(filtered, link)
			continue
		}

		if len(deterministicTransitPortKeys) > 0 {
			if _, blocked := deterministicTransitPortKeys[bridgePortObservationKey(link.designatedPort)]; blocked {
				continue
			}
			if _, blocked := deterministicTransitPortKeys[bridgePortObservationVLANKey(link.designatedPort)]; blocked {
				continue
			}
			if _, blocked := deterministicTransitPortKeys[bridgePortObservationKey(link.port)]; blocked {
				continue
			}
			if _, blocked := deterministicTransitPortKeys[bridgePortObservationVLANKey(link.port)]; blocked {
				continue
			}
		}

		if len(discoveryDevicePairs) > 0 {
			left := strings.TrimSpace(link.designatedPort.deviceID)
			right := strings.TrimSpace(link.port.deviceID)
			if pair := topologyUndirectedPairKey(left, right); pair != "" {
				if _, blocked := discoveryDevicePairs[pair]; blocked {
					continue
				}
			}
		}

		filtered = append(filtered, link)
	}
	return filtered
}

func buildDeterministicTransitPortKeySet(
	adjacencies []model.Adjacency,
	ifIndexByDeviceName map[string]int,
	ifaceByDeviceIndex map[string]model.Interface,
	aliases bridgePortAliasIndex,
) map[string]struct{} {
	return buildDeterministicTransitPortKeySetWithWork(
		nil,
		adjacencies,
		ifIndexByDeviceName,
		ifaceByDeviceIndex,
		aliases,
	)
}

func buildDeterministicTransitPortKeySetWithWork(
	work *projectionWork,
	adjacencies []model.Adjacency,
	ifIndexByDeviceName map[string]int,
	ifaceByDeviceIndex map[string]model.Interface,
	aliases bridgePortAliasIndex,
) map[string]struct{} {
	if len(adjacencies) == 0 {
		return nil
	}
	if !work.chargeProduct(uint64(len(adjacencies)), 5) {
		return nil
	}

	out := make(map[string]struct{}, len(adjacencies)*4)
	for _, adj := range adjacencies {
		if !chargeDeterministicAdjacencyWork(work, adj) {
			return nil
		}
		protocol := strings.ToLower(strings.TrimSpace(adj.Protocol))
		if protocol != "lldp" && protocol != "cdp" {
			continue
		}

		src, dst := bridgePortsFromAdjacency(adj, ifIndexByDeviceName, ifaceByDeviceIndex, aliases)
		addBridgePortObservationKeys(out, src)
		addBridgePortObservationKeys(out, dst)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func chargeDeterministicAdjacencyWork(work *projectionWork, adj model.Adjacency) bool {
	return work == nil || work.chargeStrings([]string{
		adj.Protocol,
		adj.SourceID,
		adj.SourcePort,
		adj.SourcePortEvidence.IfName,
		adj.SourcePortEvidence.BridgePort,
		adj.TargetID,
		adj.TargetPort,
		adj.TargetPortEvidence.IfName,
		adj.TargetPortEvidence.BridgePort,
	}) && chargeProjectionStringMapWork(work, adj.Labels)
}
