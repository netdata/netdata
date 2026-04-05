// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

func projectAdjacencyLinks(
	adjacencies []Adjacency,
	layer string,
	collectedAt time.Time,
	deviceByID map[string]Device,
	ifIndexByDeviceName map[string]int,
	ifaceByDeviceIndex map[string]Interface,
) projectedLinks {
	out := projectedLinks{
		links: make([]topology.Link, 0, len(adjacencies)),
	}
	if len(adjacencies) == 0 {
		return out
	}

	pairs := make(map[string]*pairedLinkAccumulator)
	pairOrder := make([]string, 0)

	for _, adj := range adjacencies {
		protocol := strings.ToLower(strings.TrimSpace(adj.Protocol))
		link := adjacencyToTopologyLink(adj, protocol, layer, collectedAt, deviceByID, ifIndexByDeviceName, ifaceByDeviceIndex)

		pairID := strings.TrimSpace(adj.Labels[adjacencyLabelPairID])
		if pairID != "" {
			acc := pairs[pairID]
			if acc == nil {
				acc = &pairedLinkAccumulator{}
				pairs[pairID] = acc
				pairOrder = append(pairOrder, pairID)
			}

			entry := &builtAdjacencyLink{
				adj:      adj,
				protocol: protocol,
				link:     link,
			}
			acc.all = append(acc.all, entry)
			continue
		}

		out.links = append(out.links, link)
		incrementProjectedProtocolCounters(&out, protocol, false)
	}

	for _, pairID := range pairOrder {
		acc := pairs[pairID]
		if acc == nil {
			continue
		}

		if left, right, ok := reversePairEntriesForBidirectionalMerge(acc.all); ok {
			merged := left.link
			merged.Direction = "bidirectional"
			merged.Src = mergeEndpointIPHints(left.link.Src, right.link.Dst)
			merged.Dst = mergeEndpointIPHints(right.link.Src, left.link.Dst)
			merged.Metrics = buildPairedLinkMetrics(left.adj.Labels, right.adj.Labels)
			out.links = append(out.links, merged)
			incrementProjectedProtocolCounters(&out, left.protocol, true)
			continue
		}

		backfillPairGroupMissingEndpointPorts(acc.all)
		for _, entry := range acc.all {
			if entry == nil {
				continue
			}
			out.links = append(out.links, entry.link)
			incrementProjectedProtocolCounters(&out, entry.protocol, false)
		}
	}

	sortTopologyLinks(out.links)
	return out
}

func reversePairEntriesForBidirectionalMerge(entries []*builtAdjacencyLink) (left, right *builtAdjacencyLink, ok bool) {
	if len(entries) != 2 || entries[0] == nil || entries[1] == nil {
		return nil, nil, false
	}

	a := entries[0]
	b := entries[1]
	aSrc := strings.TrimSpace(a.adj.SourceID)
	aDst := strings.TrimSpace(a.adj.TargetID)
	bSrc := strings.TrimSpace(b.adj.SourceID)
	bDst := strings.TrimSpace(b.adj.TargetID)
	if aSrc == "" || aDst == "" || bSrc == "" || bDst == "" {
		return nil, nil, false
	}
	if aSrc != bDst || aDst != bSrc {
		return nil, nil, false
	}

	if pairedEntryDeterministicKey(b) < pairedEntryDeterministicKey(a) {
		a, b = b, a
	}
	return a, b, true
}

func pairedEntryDeterministicKey(entry *builtAdjacencyLink) string {
	if entry == nil {
		return ""
	}
	return strings.Join([]string{
		strings.TrimSpace(entry.protocol),
		strings.TrimSpace(entry.adj.SourceID),
		strings.TrimSpace(entry.adj.SourcePort),
		strings.TrimSpace(entry.adj.TargetID),
		strings.TrimSpace(entry.adj.TargetPort),
	}, keySep)
}

func backfillPairGroupMissingEndpointPorts(entries []*builtAdjacencyLink) {
	if len(entries) < 2 {
		return
	}

	directionToIndexes := make(map[string][]int, len(entries))
	for i, entry := range entries {
		if entry == nil {
			continue
		}
		src := strings.TrimSpace(entry.adj.SourceID)
		dst := strings.TrimSpace(entry.adj.TargetID)
		if src == "" || dst == "" {
			continue
		}
		key := src + keySep + dst
		directionToIndexes[key] = append(directionToIndexes[key], i)
	}

	for i, entry := range entries {
		if entry == nil {
			continue
		}
		src := strings.TrimSpace(entry.adj.SourceID)
		dst := strings.TrimSpace(entry.adj.TargetID)
		if src == "" || dst == "" {
			continue
		}

		reverseKey := dst + keySep + src
		candidates := directionToIndexes[reverseKey]
		if len(candidates) != 1 {
			continue
		}

		reverseEntry := entries[candidates[0]]
		if reverseEntry == nil || candidates[0] == i {
			continue
		}

		entry.link.Src = backfillEndpointPortFromPeer(entry.link.Src, reverseEntry.link.Dst)
		entry.link.Dst = backfillEndpointPortFromPeer(entry.link.Dst, reverseEntry.link.Src)
	}
}

func endpointHasKnownCanonicalPort(endpoint topology.LinkEndpoint) bool {
	return strings.TrimSpace(topologyCanonicalPortName(endpoint.Attributes)) != ""
}

func backfillEndpointPortFromPeer(endpoint topology.LinkEndpoint, peer topology.LinkEndpoint) topology.LinkEndpoint {
	if endpointHasKnownCanonicalPort(endpoint) || !endpointHasKnownCanonicalPort(peer) {
		return endpoint
	}

	attrs := cloneAnyMap(endpoint.Attributes)
	if attrs == nil {
		attrs = make(map[string]any)
	}
	peerAttrs := peer.Attributes
	if len(peerAttrs) == 0 {
		return endpoint
	}

	if topologyAttrInt(attrs, "if_index") <= 0 {
		if ifIndex := topologyAttrInt(peerAttrs, "if_index"); ifIndex > 0 {
			attrs["if_index"] = ifIndex
		}
	}

	copyIfMissing := func(key string) {
		if topologyAttrString(attrs, key) != "" {
			return
		}
		if value := topologyAttrString(peerAttrs, key); value != "" {
			attrs[key] = value
		}
	}

	copyIfMissing("if_name")
	copyIfMissing("if_descr")
	copyIfMissing("if_alias")
	copyIfMissing("port_id")
	copyIfMissing("port_name")
	copyIfMissing("bridge_port")
	copyIfMissing("if_admin_status")
	copyIfMissing("if_oper_status")

	endpoint.Attributes = pruneTopologyAttributes(attrs)
	return endpoint
}

func adjacencyToTopologyLink(
	adj Adjacency,
	protocol string,
	layer string,
	collectedAt time.Time,
	deviceByID map[string]Device,
	ifIndexByDeviceName map[string]int,
	ifaceByDeviceIndex map[string]Interface,
) topology.Link {
	src := adjacencySideToEndpoint(deviceByID[adj.SourceID], adj.SourcePort, ifIndexByDeviceName, ifaceByDeviceIndex)
	dst := adjacencySideToEndpoint(deviceByID[adj.TargetID], adj.TargetPort, ifIndexByDeviceName, ifaceByDeviceIndex)
	if rawAddress := strings.TrimSpace(adj.Labels["remote_address_raw"]); rawAddress != "" {
		dst.Match.IPAddresses = uniqueTopologyStrings(append(dst.Match.IPAddresses, rawAddress))
	}

	link := topology.Link{
		Layer:        layer,
		Protocol:     protocol,
		LinkType:     protocol,
		Direction:    "unidirectional",
		Src:          src,
		Dst:          dst,
		DiscoveredAt: topologyTimePtr(collectedAt),
		LastSeen:     topologyTimePtr(collectedAt),
	}
	if len(adj.Labels) > 0 {
		link.Metrics = mapStringStringToAny(adj.Labels)
	}
	return link
}

func buildPairedLinkMetrics(sourceLabels, targetLabels map[string]string) map[string]any {
	metrics := make(map[string]any)

	pairID := strings.TrimSpace(sourceLabels[adjacencyLabelPairID])
	if pairID == "" {
		pairID = strings.TrimSpace(targetLabels[adjacencyLabelPairID])
	}
	if pairID != "" {
		metrics[adjacencyLabelPairID] = pairID
	}

	pairPass := strings.TrimSpace(sourceLabels[adjacencyLabelPairPass])
	if pairPass == "" {
		pairPass = strings.TrimSpace(targetLabels[adjacencyLabelPairPass])
	}
	if pairPass != "" {
		metrics[adjacencyLabelPairPass] = pairPass
	}
	metrics["pair_consistent"] = true

	for key, value := range sourceLabels {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || isPairLabelKey(key) {
			continue
		}
		metrics["src_"+key] = value
	}
	for key, value := range targetLabels {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" || isPairLabelKey(key) {
			continue
		}
		metrics["dst_"+key] = value
	}

	if len(metrics) == 0 {
		return nil
	}
	return metrics
}

func mergeEndpointIPHints(base, extra topology.LinkEndpoint) topology.LinkEndpoint {
	if len(extra.Match.IPAddresses) == 0 {
		return base
	}
	base.Match.IPAddresses = uniqueTopologyStrings(append(base.Match.IPAddresses, extra.Match.IPAddresses...))
	return base
}

func isPairLabelKey(key string) bool {
	return key == adjacencyLabelPairID || key == adjacencyLabelPairPass
}

func incrementProjectedProtocolCounters(out *projectedLinks, protocol string, bidirectional bool) {
	if out == nil {
		return
	}
	switch protocol {
	case "lldp":
		out.lldp++
	case "cdp":
		out.cdp++
	}
	if bidirectional {
		out.bidirectionalCount++
		return
	}
	out.unidirectionalCount++
}
