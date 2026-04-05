// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strings"
)

func collectBridgeLinkRecords(
	adjacencies []Adjacency,
	ifIndexByDeviceName map[string]int,
	strategy topologyInferenceStrategyConfig,
) []bridgeBridgeLinkRecord {
	records := make([]bridgeBridgeLinkRecord, 0)
	seen := make(map[string]struct{})

	for _, adj := range adjacencies {
		protocol := strings.ToLower(strings.TrimSpace(adj.Protocol))
		if !strategy.acceptsBridgeProtocol(protocol) {
			continue
		}

		src := bridgePortFromAdjacencySide(adj.SourceID, adj.SourcePort, ifIndexByDeviceName)
		dst := bridgePortFromAdjacencySide(adj.TargetID, adj.TargetPort, ifIndexByDeviceName)
		srcKey := bridgePortRefKey(src, false, false)
		dstKey := bridgePortRefKey(dst, false, false)
		if srcKey == "" || dstKey == "" {
			continue
		}

		pairKey := bridgePairKey(src, dst)
		if pairKey == "" {
			continue
		}
		if _, ok := seen[pairKey]; ok {
			continue
		}
		seen[pairKey] = struct{}{}

		designated := src
		other := dst
		if protocol == "stp" && strategy.useSTPDesignatedParent {
			designated = dst
			other = src
			if bridgePortRefKey(designated, false, false) == "" {
				designated = src
				other = dst
			}
		} else {
			if bridgePortRefSortKey(src) > bridgePortRefSortKey(dst) {
				designated = dst
				other = src
			}
		}
		records = append(records, bridgeBridgeLinkRecord{
			port:           other,
			designatedPort: designated,
			method:         protocol,
		})
	}

	sort.SliceStable(records, func(i, j int) bool {
		li := portSortKey(records[i].designatedPort) + keySep + portSortKey(records[i].port)
		lj := portSortKey(records[j].designatedPort) + keySep + portSortKey(records[j].port)
		return li < lj
	})
	return records
}

func (s topologyInferenceStrategyConfig) acceptsBridgeProtocol(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "lldp":
		return s.includeLLDPBridgeLinks
	case "cdp":
		return s.includeCDPBridgeLinks
	case "stp":
		return s.includeSTPBridgeLinks
	default:
		return false
	}
}

func mergeBridgeLinkRecordSets(base, extra []bridgeBridgeLinkRecord) []bridgeBridgeLinkRecord {
	if len(extra) == 0 {
		return base
	}
	out := make([]bridgeBridgeLinkRecord, 0, len(base)+len(extra))
	out = append(out, base...)
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, link := range out {
		if key := bridgePairKey(link.designatedPort, link.port); key != "" {
			seen[key] = struct{}{}
		}
	}
	for _, link := range extra {
		key := bridgePairKey(link.designatedPort, link.port)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, link)
	}
	sort.SliceStable(out, func(i, j int) bool {
		li := portSortKey(out[i].designatedPort) + keySep + portSortKey(out[i].port)
		lj := portSortKey(out[j].designatedPort) + keySep + portSortKey(out[j].port)
		return li < lj
	})
	return out
}

func collectBridgeMacLinkRecords(
	attachments []Attachment,
	ifaceByDeviceIndex map[string]Interface,
	switchFacingPortKeys map[string]struct{},
) []bridgeMacLinkRecord {
	records := make([]bridgeMacLinkRecord, 0, len(attachments))
	seen := make(map[string]struct{}, len(attachments))

	attachmentsSorted := append([]Attachment(nil), attachments...)
	sort.SliceStable(attachmentsSorted, func(i, j int) bool {
		return bridgeAttachmentSortKey(attachmentsSorted[i]) < bridgeAttachmentSortKey(attachmentsSorted[j])
	})

	for _, attachment := range attachmentsSorted {
		port := bridgePortFromAttachment(attachment, ifaceByDeviceIndex)
		portKey := bridgePortRefKey(port, false, false)
		endpointID := strings.TrimSpace(attachment.EndpointID)
		if portKey == "" || endpointID == "" {
			continue
		}
		method := strings.ToLower(strings.TrimSpace(attachment.Method))
		if method == "" {
			method = "fdb"
		}
		if method == "fdb" {
			if _, isSwitchFacingPort := switchFacingPortKeys[bridgePortObservationKey(port)]; isSwitchFacingPort {
				continue
			}
			if _, isSwitchFacingPort := switchFacingPortKeys[bridgePortObservationVLANKey(port)]; isSwitchFacingPort {
				continue
			}
		}

		key := portKey + keySep + endpointID + keySep + method
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		records = append(records, bridgeMacLinkRecord{
			port:       port,
			endpointID: endpointID,
			method:     method,
		})
	}

	return records
}
