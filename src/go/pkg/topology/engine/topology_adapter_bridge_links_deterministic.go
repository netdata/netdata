// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import "strings"

func buildDeterministicDiscoveryDevicePairSet(adjacencies []Adjacency) map[string]struct{} {
	if len(adjacencies) == 0 {
		return nil
	}

	out := make(map[string]struct{}, len(adjacencies))
	for _, adj := range adjacencies {
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
	if len(bridgeLinks) == 0 {
		return bridgeLinks
	}

	filtered := make([]bridgeBridgeLinkRecord, 0, len(bridgeLinks))
	for _, link := range bridgeLinks {
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
	adjacencies []Adjacency,
	ifIndexByDeviceName map[string]int,
) map[string]struct{} {
	if len(adjacencies) == 0 {
		return nil
	}

	out := make(map[string]struct{}, len(adjacencies)*4)
	for _, adj := range adjacencies {
		protocol := strings.ToLower(strings.TrimSpace(adj.Protocol))
		if protocol != "lldp" && protocol != "cdp" {
			continue
		}

		src := bridgePortFromAdjacencySide(adj.SourceID, adj.SourcePort, ifIndexByDeviceName)
		dst := bridgePortFromAdjacencySide(adj.TargetID, adj.TargetPort, ifIndexByDeviceName)
		addBridgePortObservationKeys(out, src)
		addBridgePortObservationKeys(out, dst)
	}

	if len(out) == 0 {
		return nil
	}
	return out
}
