// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"sort"
	"strconv"
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
		li := portSortKey(records[i].designatedPort) + "|" + portSortKey(records[i].port)
		lj := portSortKey(records[j].designatedPort) + "|" + portSortKey(records[j].port)
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
		li := portSortKey(out[i].designatedPort) + "|" + portSortKey(out[i].port)
		lj := portSortKey(out[j].designatedPort) + "|" + portSortKey(out[j].port)
		return li < lj
	})
	return out
}

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
		// Keep direct discovery links as the authoritative source.
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

func mergeBridgePortObservationKeySets(left, right map[string]struct{}) map[string]struct{} {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}

	out := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	for key := range right {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
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

		key := portKey + "|" + endpointID + "|" + method
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

func buildSwitchFacingPortKeySet(bridgeLinks []bridgeBridgeLinkRecord) map[string]struct{} {
	if len(bridgeLinks) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(bridgeLinks)*4)
	for _, link := range bridgeLinks {
		addBridgePortObservationKeys(out, link.designatedPort)
		addBridgePortObservationKeys(out, link.port)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addBridgePortObservationKeys(out map[string]struct{}, port bridgePortRef) {
	if out == nil {
		return
	}
	if key := bridgePortObservationKey(port); key != "" {
		out[key] = struct{}{}
	}
	if key := bridgePortObservationVLANKey(port); key != "" {
		out[key] = struct{}{}
	}
}

func augmentSwitchFacingPortKeySetFromManagedAliases(
	switchFacingPortKeys map[string]struct{},
	observations fdbReporterObservation,
	reporterAliases map[string][]string,
) map[string]struct{} {
	if len(observations.byReporter) == 0 || len(reporterAliases) == 0 {
		return switchFacingPortKeys
	}

	aliasOwnerIDs := buildFDBAliasOwnerMap(reporterAliases)
	if len(aliasOwnerIDs) == 0 {
		return switchFacingPortKeys
	}

	updated := switchFacingPortKeys
	for reporterID, endpoints := range observations.byReporter {
		reporterID = strings.TrimSpace(reporterID)
		if reporterID == "" {
			continue
		}
		for endpointID, ports := range endpoints {
			owners := aliasOwnerIDs[normalizeFDBEndpointID(endpointID)]
			if len(owners) == 0 {
				continue
			}
			otherManagedOwner := false
			for ownerID := range owners {
				if !strings.EqualFold(ownerID, reporterID) {
					otherManagedOwner = true
					break
				}
			}
			if !otherManagedOwner {
				continue
			}
			for portKey := range ports {
				portKey = strings.TrimSpace(portKey)
				if portKey == "" {
					continue
				}
				if updated == nil {
					updated = make(map[string]struct{})
				}
				updated[portKey] = struct{}{}
			}
		}
	}
	return updated
}

func augmentSwitchFacingPortKeySetFromSTPManagedAliasCorrelation(
	switchFacingPortKeys map[string]struct{},
	adjacencies []Adjacency,
	ifIndexByDeviceName map[string]int,
	observations fdbReporterObservation,
	reporterAliases map[string][]string,
) map[string]struct{} {
	if len(adjacencies) == 0 || len(observations.byReporter) == 0 || len(reporterAliases) == 0 {
		return switchFacingPortKeys
	}

	updated := switchFacingPortKeys
	for _, adj := range adjacencies {
		if !strings.EqualFold(strings.TrimSpace(adj.Protocol), "stp") {
			continue
		}

		srcReporterID := strings.TrimSpace(adj.SourceID)
		dstReporterID := strings.TrimSpace(adj.TargetID)
		if srcReporterID == "" || dstReporterID == "" || strings.EqualFold(srcReporterID, dstReporterID) {
			continue
		}

		srcPort := bridgePortFromAdjacencySide(srcReporterID, adj.SourcePort, ifIndexByDeviceName)
		dstPort := bridgePortFromAdjacencySide(dstReporterID, adj.TargetPort, ifIndexByDeviceName)

		if stpPortSeesManagedAlias(srcReporterID, srcPort, dstReporterID, observations, reporterAliases) {
			if updated == nil {
				updated = make(map[string]struct{})
			}
			addBridgePortObservationKeys(updated, srcPort)
		}
		if stpPortSeesManagedAlias(dstReporterID, dstPort, srcReporterID, observations, reporterAliases) {
			if updated == nil {
				updated = make(map[string]struct{})
			}
			addBridgePortObservationKeys(updated, dstPort)
		}
	}
	return updated
}

func stpPortSeesManagedAlias(
	reporterID string,
	port bridgePortRef,
	peerDeviceID string,
	observations fdbReporterObservation,
	reporterAliases map[string][]string,
) bool {
	reporterID = strings.TrimSpace(reporterID)
	peerDeviceID = strings.TrimSpace(peerDeviceID)
	if reporterID == "" || peerDeviceID == "" {
		return false
	}
	portKey := bridgePortObservationKey(port)
	if portKey == "" {
		return false
	}
	endpointsByReporter := observations.byReporter[reporterID]
	if len(endpointsByReporter) == 0 {
		return false
	}
	aliases := reporterAliases[peerDeviceID]
	if len(aliases) == 0 {
		return false
	}
	for _, alias := range aliases {
		alias = normalizeFDBEndpointID(alias)
		if alias == "" {
			continue
		}
		ports := endpointsByReporter[alias]
		if len(ports) == 0 {
			continue
		}
		if _, ok := ports[portKey]; ok {
			return true
		}
	}
	return false
}

func bridgePortObservationKey(port bridgePortRef) string {
	base := bridgePortObservationBaseKey(port)
	if base == "" {
		return ""
	}
	return base + "|vlan:"
}

func bridgePortObservationVLANKey(port bridgePortRef) string {
	base := bridgePortObservationBaseKey(port)
	if base == "" {
		return ""
	}
	return base + "|vlan:" + strings.ToLower(strings.TrimSpace(port.vlanID))
}

func bridgePortObservationBaseKey(port bridgePortRef) string {
	deviceID := strings.TrimSpace(port.deviceID)
	if deviceID == "" {
		return ""
	}
	if port.ifIndex > 0 {
		return deviceID + "|if:" + strconv.Itoa(port.ifIndex)
	}
	name := firstNonEmpty(port.ifName, port.bridgePort)
	name = normalizeInterfaceNameForLookup(name)
	if name == "" {
		return ""
	}
	return deviceID + "|name:" + name
}
