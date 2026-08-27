// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

func collectBridgeLinkRecords(
	adjacencies []model.Adjacency,
	ifIndexByDeviceName map[string]int,
	ifaceByDeviceIndex map[string]model.Interface,
	aliases bridgePortAliasIndex,
	strategy topologyInferenceStrategyConfig,
) []bridgeBridgeLinkRecord {
	return collectBridgeLinkRecordsWithWork(nil, adjacencies, ifIndexByDeviceName, ifaceByDeviceIndex, aliases, strategy)
}

func collectBridgeLinkRecordsWithWork(
	work *projectionWork,
	adjacencies []model.Adjacency,
	ifIndexByDeviceName map[string]int,
	ifaceByDeviceIndex map[string]model.Interface,
	aliases bridgePortAliasIndex,
	strategy topologyInferenceStrategyConfig,
) []bridgeBridgeLinkRecord {
	if !work.charge(uint64(len(adjacencies))) {
		return nil
	}
	records := make([]bridgeBridgeLinkRecord, 0)
	seen := make(map[string]struct{})

	for _, adj := range adjacencies {
		protocol := strings.ToLower(strings.TrimSpace(adj.Protocol))
		if !strategy.acceptsBridgeProtocol(protocol) {
			continue
		}

		src, dst := bridgePortsFromAdjacency(adj, ifIndexByDeviceName, ifaceByDeviceIndex, aliases)
		srcKey := bridgePortRefKey(src, false, false)
		dstKey := bridgePortRefKey(dst, false, false)
		if srcKey == "" || dstKey == "" {
			continue
		}

		pairKey := bridgeScopedPairKey(src, dst)
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
			srcSortKey, srcOK := bridgePortRefSortKeyWithWork(work, src)
			dstSortKey, dstOK := bridgePortRefSortKeyWithWork(work, dst)
			if !srcOK || !dstOK {
				return nil
			}
			if srcSortKey > dstSortKey {
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

	if !sortProjectionByPreparedStringKeyStable(work, records, bridgeBridgeLinkRecordSortKeyWithWork) {
		return nil
	}
	return records
}

func normalizeBridgeMacLinkRecords(records []bridgeMacLinkRecord) []bridgeMacLinkRecord {
	return normalizeBridgeMacLinkRecordsWithWork(nil, records)
}

func normalizeBridgeMacLinkRecordsWithWork(work *projectionWork, records []bridgeMacLinkRecord) []bridgeMacLinkRecord {
	if len(records) == 0 {
		return nil
	}
	if !bridgeMacLinksNeedNormalization(records) {
		return records
	}
	if !work.charge(uint64(len(records))) {
		return nil
	}

	aliases := buildBridgeVLANAliasIndex(records)
	normalized := make([]bridgeMacLinkRecord, 0, len(records))
	domainful := make(map[string]struct{}, len(records))
	for _, record := range records {
		record.port = aliases.canonicalizeVLANScopedPort(record.port)
		identity := bridgePortCanonicalIdentity(record.port)
		endpointID := strings.TrimSpace(record.endpointID)
		method := strings.ToLower(strings.TrimSpace(record.method))
		if identity == "" || endpointID == "" {
			continue
		}
		if method == "" {
			method = "fdb"
		}
		record.endpointID = endpointID
		record.method = method
		normalized = append(normalized, record)
		if method == "fdb" && bridgePortForwardingDomain(record.port) != "" {
			domainful[identity+keySep+endpointID] = struct{}{}
		}
	}

	out := make([]bridgeMacLinkRecord, 0, len(normalized))
	position := make(map[string]int, len(normalized))
	for _, record := range normalized {
		identity := bridgePortCanonicalIdentity(record.port)
		if record.method == "fdb" && bridgePortForwardingDomain(record.port) == "" {
			if _, superseded := domainful[identity+keySep+record.endpointID]; superseded {
				continue
			}
		}

		key := bridgePortRefKey(record.port, false, true) + keySep + record.endpointID + keySep + record.method
		if index, exists := position[key]; exists {
			mergeBridgeMacLinkPortEvidence(&out[index].port, record.port)
			continue
		}
		position[key] = len(out)
		out = append(out, record)
	}

	if !sortProjectionByPreparedStringKeyStable(work, out, bridgeMacLinkRecordSortKeyWithWork) {
		return nil
	}
	return out
}

func bridgeMacLinksNeedNormalization(records []bridgeMacLinkRecord) bool {
	hasDomainless := false
	hasRawDomain := false
	hasVLANScope := false
	for _, record := range records {
		method := strings.ToLower(strings.TrimSpace(record.method))
		if method != "" && method != "fdb" {
			continue
		}
		domain := bridgePortForwardingDomain(record.port)
		vlanScope := bridgePortVLANScope(record.port)
		switch {
		case domain == "":
			hasDomainless = true
		case domain == vlanScope:
			hasVLANScope = true
		default:
			hasRawDomain = true
		}
	}
	return hasRawDomain && (hasDomainless || hasVLANScope) || hasDomainless && hasVLANScope
}

func mergeBridgeMacLinkPortEvidence(dst *bridgePortRef, src bridgePortRef) {
	if dst == nil {
		return
	}
	if dst.ifIndex <= 0 && src.ifIndex > 0 {
		dst.ifIndex = src.ifIndex
	}
	if strings.TrimSpace(dst.ifName) == "" {
		dst.ifName = strings.TrimSpace(src.ifName)
	}
	if strings.TrimSpace(dst.bridgePort) == "" {
		dst.bridgePort = strings.TrimSpace(src.bridgePort)
	}
	if strings.TrimSpace(dst.fdbDomainID) == "" {
		dst.fdbDomainID = strings.TrimSpace(src.fdbDomainID)
	}
	if strings.TrimSpace(dst.vlanID) == "" {
		dst.vlanID = strings.TrimSpace(src.vlanID)
	}
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

func mergeBridgeLinkRecordSets(
	base, extra []bridgeBridgeLinkRecord,
) []bridgeBridgeLinkRecord {
	return mergeBridgeLinkRecordSetsWithWork(nil, base, extra)
}

func mergeBridgeLinkRecordSetsWithWork(
	work *projectionWork,
	base, extra []bridgeBridgeLinkRecord,
) []bridgeBridgeLinkRecord {
	if len(extra) == 0 {
		return base
	}
	if !work.charge(uint64(len(base)) + uint64(len(extra))) {
		return nil
	}
	out := make([]bridgeBridgeLinkRecord, 0, len(base)+len(extra))
	out = append(out, base...)
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, link := range out {
		if key := bridgeScopedPairKey(link.designatedPort, link.port); key != "" {
			seen[key] = struct{}{}
		}
	}
	for _, link := range extra {
		key := bridgeScopedPairKey(link.designatedPort, link.port)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, link)
	}
	if !sortProjectionByPreparedStringKeyStable(work, out, bridgeBridgeLinkRecordSortKeyWithWork) {
		return nil
	}
	return out
}

func bridgeBridgeLinkRecordSortKeyWithWork(work *projectionWork, record bridgeBridgeLinkRecord) (string, bool) {
	designated, ok := portSortKeyWithWork(work, record.designatedPort)
	if !ok {
		return "", false
	}
	port, ok := portSortKeyWithWork(work, record.port)
	if !ok {
		return "", false
	}
	return designated + keySep + port, true
}

func bridgeMacLinkRecordSortKeyWithWork(work *projectionWork, record bridgeMacLinkRecord) (string, bool) {
	port, ok := portSortKeyWithWork(work, record.port)
	if !ok || work != nil && !work.chargeStrings([]string{record.endpointID, record.method}) {
		return "", false
	}
	return port + keySep + record.endpointID + keySep + record.method, true
}

func collectBridgeMacLinkRecords(
	attachments []model.Attachment,
	ifaceByDeviceIndex map[string]model.Interface,
	switchFacingPortKeys map[string]struct{},
) []bridgeMacLinkRecord {
	return collectBridgeMacLinkRecordsWithWork(nil, attachments, ifaceByDeviceIndex, switchFacingPortKeys)
}

func collectBridgeMacLinkRecordsWithWork(
	work *projectionWork,
	attachments []model.Attachment,
	ifaceByDeviceIndex map[string]model.Interface,
	switchFacingPortKeys map[string]struct{},
) []bridgeMacLinkRecord {
	if !work.charge(uint64(len(attachments))) {
		return nil
	}
	records := make([]bridgeMacLinkRecord, 0, len(attachments))
	seen := make(map[string]struct{}, len(attachments))

	for _, attachment := range attachments {
		port := bridgePortFromAttachment(attachment, ifaceByDeviceIndex)
		portKey := bridgePortRefKey(port, false, true)
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

	return normalizeBridgeMacLinkRecordsWithWork(work, records)
}
