// SPDX-License-Identifier: GPL-3.0-or-later

package projector

import (
	"sort"
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

func normalizeBridgeMacLinkRecords(records []bridgeMacLinkRecord) []bridgeMacLinkRecord {
	if len(records) == 0 {
		return nil
	}
	if !bridgeMacLinksNeedNormalization(records) {
		return records
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

	sort.SliceStable(out, func(i, j int) bool {
		left := portSortKey(out[i].port) + keySep + out[i].endpointID + keySep + out[i].method
		right := portSortKey(out[j].port) + keySep + out[j].endpointID + keySep + out[j].method
		return left < right
	})
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

func mergeBridgeLinkRecordSets(base, extra []bridgeBridgeLinkRecord) []bridgeBridgeLinkRecord {
	if len(extra) == 0 {
		return base
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
	sort.SliceStable(out, func(i, j int) bool {
		li := portSortKey(out[i].designatedPort) + keySep + portSortKey(out[i].port)
		lj := portSortKey(out[j].designatedPort) + keySep + portSortKey(out[j].port)
		return li < lj
	})
	return out
}

func collectBridgeMacLinkRecords(
	attachments []model.Attachment,
	ifaceByDeviceIndex map[string]model.Interface,
	switchFacingPortKeys map[string]struct{},
) []bridgeMacLinkRecord {
	records := make([]bridgeMacLinkRecord, 0, len(attachments))
	seen := make(map[string]struct{}, len(attachments))

	attachmentsSorted := append([]model.Attachment(nil), attachments...)
	sort.SliceStable(attachmentsSorted, func(i, j int) bool {
		return bridgeAttachmentSortKey(attachmentsSorted[i]) < bridgeAttachmentSortKey(attachmentsSorted[j])
	})

	for _, attachment := range attachmentsSorted {
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

	return normalizeBridgeMacLinkRecords(records)
}
