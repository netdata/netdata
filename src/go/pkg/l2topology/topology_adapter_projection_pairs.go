// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
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
		links: make([]graph.Link, 0, len(adjacencies)),
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
			merged.L2 = buildPairedLinkL2(left.adj.Labels, right.adj.Labels)
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

func endpointHasKnownCanonicalPort(endpoint graph.LinkEndpoint) bool {
	return strings.TrimSpace(topologyEndpointCanonicalPortName(endpoint)) != ""
}

func backfillEndpointPortFromPeer(endpoint graph.LinkEndpoint, peer graph.LinkEndpoint) graph.LinkEndpoint {
	if endpointHasKnownCanonicalPort(endpoint) || !endpointHasKnownCanonicalPort(peer) {
		return endpoint
	}
	if endpoint.IfIndex <= 0 && peer.IfIndex > 0 {
		endpoint.IfIndex = peer.IfIndex
	}
	if strings.TrimSpace(endpoint.IfName) == "" {
		endpoint.IfName = strings.TrimSpace(peer.IfName)
	}
	if strings.TrimSpace(endpoint.IfDescr) == "" {
		endpoint.IfDescr = strings.TrimSpace(peer.IfDescr)
	}
	if strings.TrimSpace(endpoint.IfAlias) == "" {
		endpoint.IfAlias = strings.TrimSpace(peer.IfAlias)
	}
	if strings.TrimSpace(endpoint.PortID) == "" {
		endpoint.PortID = strings.TrimSpace(peer.PortID)
	}
	if strings.TrimSpace(endpoint.PortName) == "" {
		endpoint.PortName = strings.TrimSpace(peer.PortName)
	}
	if strings.TrimSpace(endpoint.BridgePort) == "" {
		endpoint.BridgePort = strings.TrimSpace(peer.BridgePort)
	}
	if strings.TrimSpace(endpoint.AdminStatus) == "" {
		endpoint.AdminStatus = strings.TrimSpace(peer.AdminStatus)
	}
	if strings.TrimSpace(endpoint.OperStatus) == "" {
		endpoint.OperStatus = strings.TrimSpace(peer.OperStatus)
	}
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
) graph.Link {
	src := adjacencySideToEndpoint(deviceByID[adj.SourceID], adj.SourcePort, ifIndexByDeviceName, ifaceByDeviceIndex)
	dst := adjacencySideToEndpoint(deviceByID[adj.TargetID], adj.TargetPort, ifIndexByDeviceName, ifaceByDeviceIndex)
	if rawAddress := strings.TrimSpace(adj.Labels["remote_address_raw"]); rawAddress != "" {
		dst.Match.IPAddresses = uniqueTopologyStrings(append(dst.Match.IPAddresses, rawAddress))
	}

	link := graph.Link{
		Layer:        layer,
		Protocol:     protocol,
		LinkType:     protocol,
		Direction:    "unidirectional",
		Src:          src,
		Dst:          dst,
		DiscoveredAt: topologyTimePtr(collectedAt),
		LastSeen:     topologyTimePtr(collectedAt),
	}
	return link
}

func buildPairedLinkL2(sourceLabels, targetLabels map[string]string) *graph.LinkL2 {
	l2 := &graph.LinkL2{}
	pairID := strings.TrimSpace(sourceLabels[adjacencyLabelPairID])
	if pairID == "" {
		pairID = strings.TrimSpace(targetLabels[adjacencyLabelPairID])
	}
	if pairID != "" {
		l2.PairID = pairID
	}

	pairPass := strings.TrimSpace(sourceLabels[adjacencyLabelPairPass])
	if pairPass == "" {
		pairPass = strings.TrimSpace(targetLabels[adjacencyLabelPairPass])
	}
	if pairPass != "" {
		l2.PairPass = pairPass
	}
	l2.PairConsistent = true
	return l2
}

func mergeEndpointIPHints(base, extra graph.LinkEndpoint) graph.LinkEndpoint {
	if len(extra.Match.IPAddresses) == 0 {
		return base
	}
	base.Match.IPAddresses = uniqueTopologyStrings(append(base.Match.IPAddresses, extra.Match.IPAddresses...))
	return base
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
