// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

// TopologyDataOptions controls conversion from Result to topology.Data.
type TopologyDataOptions struct {
	SchemaVersion string
	Source        string
	Layer         string
	View          string
	AgentID       string
	LocalDeviceID string
	CollectedAt   time.Time
}

type endpointActorAccumulator struct {
	endpointID string
	mac        string
	ips        map[string]netip.Addr
	sources    map[string]struct{}
	ifIndexes  map[string]struct{}
	ifNames    map[string]struct{}
}

type builtAdjacencyLink struct {
	adj      Adjacency
	protocol string
	link     topology.Link
}

type pairedLinkAccumulator struct {
	source *builtAdjacencyLink
	target *builtAdjacencyLink
}

type projectedLinks struct {
	links               []topology.Link
	lldp                int
	cdp                 int
	bidirectionalCount  int
	unidirectionalCount int
}

type bridgePortRef struct {
	deviceID   string
	ifIndex    int
	ifName     string
	bridgePort string
	vlanID     string
}

type segmentAccumulator struct {
	id               string
	designatedPortID string
	ports            map[string]bridgePortRef
	portByLooseKey   map[string]string
	endpointIDs      map[string]struct{}
	deviceIDs        map[string]struct{}
	ifNames          map[string]struct{}
	ifIndexes        map[string]struct{}
	bridgePorts      map[string]struct{}
	vlanIDs          map[string]struct{}
	methods          map[string]struct{}
}

type projectedSegments struct {
	actors             []topology.Actor
	links              []topology.Link
	linksFdb           int
	bidirectionalCount int
}

// ToTopologyData converts an engine result to the shared topology schema.
func ToTopologyData(result Result, opts TopologyDataOptions) topology.Data {
	schemaVersion := strings.TrimSpace(opts.SchemaVersion)
	if schemaVersion == "" {
		schemaVersion = "2.0"
	}

	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = "snmp"
	}

	layer := strings.TrimSpace(opts.Layer)
	if layer == "" {
		layer = "2"
	}

	view := strings.TrimSpace(opts.View)
	if view == "" {
		view = "summary"
	}

	collectedAt := opts.CollectedAt
	if collectedAt.IsZero() {
		collectedAt = result.CollectedAt
	}
	if collectedAt.IsZero() {
		collectedAt = time.Now().UTC()
	}

	deviceByID := make(map[string]Device, len(result.Devices))
	ifaceByDeviceIndex := make(map[string]Interface, len(result.Interfaces))
	ifIndexByDeviceName := make(map[string]int, len(result.Interfaces))

	for _, dev := range result.Devices {
		deviceByID[dev.ID] = dev
	}

	for _, iface := range result.Interfaces {
		if iface.IfIndex <= 0 {
			continue
		}
		ifaceByDeviceIndex[deviceIfIndexKey(iface.DeviceID, iface.IfIndex)] = iface
		ifName := strings.TrimSpace(iface.IfName)
		if ifName != "" {
			ifIndexByDeviceName[deviceIfNameKey(iface.DeviceID, ifName)] = iface.IfIndex
		}
	}

	actors := make([]topology.Actor, 0, len(result.Devices))
	actorIndex := make(map[string]struct{}, len(result.Devices)*2)
	for _, dev := range result.Devices {
		actor := deviceToTopologyActor(dev, source, layer, opts.LocalDeviceID)
		keys := topologyMatchIdentityKeys(actor.Match)
		if len(keys) == 0 {
			continue
		}
		if topologyIdentityIndexOverlaps(actorIndex, keys) {
			continue
		}
		addTopologyIdentityKeys(actorIndex, keys)
		actors = append(actors, actor)
	}

	projected := projectAdjacencyLinks(result.Adjacencies, layer, collectedAt, deviceByID, ifIndexByDeviceName)

	endpointActors := buildEndpointActors(result.Attachments, result.Enrichments, ifaceByDeviceIndex, source, layer, actorIndex)
	actors = append(actors, endpointActors.actors...)

	segmentProjection := projectSegmentTopology(
		result.Attachments,
		result.Adjacencies,
		layer,
		source,
		collectedAt,
		deviceByID,
		ifaceByDeviceIndex,
		ifIndexByDeviceName,
		endpointActors.matchByEndpointID,
		actorIndex,
	)
	actors = append(actors, segmentProjection.actors...)
	sortTopologyActors(actors)

	links := make([]topology.Link, 0, len(projected.links)+len(segmentProjection.links))
	links = append(links, projected.links...)
	links = append(links, segmentProjection.links...)
	sortTopologyLinks(links)

	stats := cloneAnyMap(result.Stats)
	if stats == nil {
		stats = make(map[string]any)
	}
	stats["devices_total"] = len(result.Devices)
	stats["devices_discovered"] = discoveredDeviceCount(result.Devices, opts.LocalDeviceID)
	stats["links_total"] = len(links)
	stats["links_lldp"] = projected.lldp
	stats["links_cdp"] = projected.cdp
	stats["links_bidirectional"] = projected.bidirectionalCount + segmentProjection.bidirectionalCount
	stats["links_unidirectional"] = projected.unidirectionalCount
	stats["links_fdb"] = segmentProjection.linksFdb
	stats["links_arp"] = 0
	stats["actors_total"] = len(actors)
	stats["endpoints_total"] = endpointActors.count

	return topology.Data{
		SchemaVersion: schemaVersion,
		Source:        source,
		Layer:         layer,
		AgentID:       opts.AgentID,
		CollectedAt:   collectedAt,
		View:          view,
		Actors:        actors,
		Links:         links,
		Stats:         stats,
	}
}

func projectAdjacencyLinks(
	adjacencies []Adjacency,
	layer string,
	collectedAt time.Time,
	deviceByID map[string]Device,
	ifIndexByDeviceName map[string]int,
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
		link := adjacencyToTopologyLink(adj, protocol, layer, collectedAt, deviceByID, ifIndexByDeviceName)

		pairID := strings.TrimSpace(adj.Labels[adjacencyLabelPairID])
		pairSide := strings.TrimSpace(adj.Labels[adjacencyLabelPairSide])
		if pairID != "" && (pairSide == adjacencyPairSideSource || pairSide == adjacencyPairSideTarget) {
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
			switch pairSide {
			case adjacencyPairSideSource:
				if acc.source == nil {
					acc.source = entry
					continue
				}
			case adjacencyPairSideTarget:
				if acc.target == nil {
					acc.target = entry
					continue
				}
			}
		}

		out.links = append(out.links, link)
		incrementProjectedProtocolCounters(&out, protocol, false)
	}

	for _, pairID := range pairOrder {
		acc := pairs[pairID]
		if acc == nil {
			continue
		}

		if acc.source != nil && acc.target != nil {
			merged := acc.source.link
			merged.Direction = "bidirectional"
			merged.Src = mergeEndpointIPHints(acc.source.link.Src, acc.target.link.Dst)
			merged.Dst = mergeEndpointIPHints(acc.target.link.Src, acc.source.link.Dst)
			merged.Metrics = buildPairedLinkMetrics(acc.source.adj.Labels, acc.target.adj.Labels)
			out.links = append(out.links, merged)
			incrementProjectedProtocolCounters(&out, acc.source.protocol, true)
			continue
		}

		if acc.source != nil {
			out.links = append(out.links, acc.source.link)
			incrementProjectedProtocolCounters(&out, acc.source.protocol, false)
		}
		if acc.target != nil {
			out.links = append(out.links, acc.target.link)
			incrementProjectedProtocolCounters(&out, acc.target.protocol, false)
		}
	}

	sortTopologyLinks(out.links)
	return out
}

func projectSegmentTopology(
	attachments []Attachment,
	adjacencies []Adjacency,
	layer string,
	source string,
	collectedAt time.Time,
	deviceByID map[string]Device,
	ifaceByDeviceIndex map[string]Interface,
	ifIndexByDeviceName map[string]int,
	endpointMatchByID map[string]topology.Match,
	actorIndex map[string]struct{},
) projectedSegments {
	out := projectedSegments{
		actors: make([]topology.Actor, 0),
		links:  make([]topology.Link, 0),
	}
	if len(attachments) == 0 && len(adjacencies) == 0 {
		return out
	}

	bridgeLinks := collectBridgeLinkRecords(adjacencies, ifIndexByDeviceName)
	macLinks := collectBridgeMacLinkRecords(attachments, ifaceByDeviceIndex)
	model := buildBridgeDomainModel(bridgeLinks, macLinks)
	if len(model.domains) == 0 {
		return out
	}

	segmentIDs := make([]string, 0)
	segmentMatchByID := make(map[string]topology.Match)
	segmentByID := make(map[string]*bridgeDomainSegment)

	for _, domain := range model.domains {
		if domain == nil {
			continue
		}
		for _, segment := range domain.segments {
			if segment == nil {
				continue
			}
			if len(segment.endpointIDs) == 0 {
				continue
			}
			segmentID := bridgeDomainSegmentID(segment)
			if _, exists := segmentByID[segmentID]; exists {
				continue
			}
			segmentByID[segmentID] = segment
			segmentIDs = append(segmentIDs, segmentID)
		}
	}
	sort.Strings(segmentIDs)
	if len(segmentIDs) == 0 {
		return out
	}

	for _, segmentID := range segmentIDs {
		segment := segmentByID[segmentID]
		if segment == nil {
			continue
		}

		parentDevices := make(map[string]struct{})
		ifNames := make(map[string]struct{})
		ifIndexes := make(map[string]struct{})
		bridgePorts := make(map[string]struct{})
		vlanIDs := make(map[string]struct{})
		for _, port := range segment.ports {
			if strings.TrimSpace(port.deviceID) != "" {
				parentDevices[port.deviceID] = struct{}{}
			}
			if strings.TrimSpace(port.ifName) != "" {
				ifNames[port.ifName] = struct{}{}
			}
			if port.ifIndex > 0 {
				ifIndexes[strconv.Itoa(port.ifIndex)] = struct{}{}
			}
			if strings.TrimSpace(port.bridgePort) != "" {
				bridgePorts[port.bridgePort] = struct{}{}
			}
			if strings.TrimSpace(port.vlanID) != "" {
				vlanIDs[port.vlanID] = struct{}{}
			}
		}

		match := topology.Match{
			Hostnames: []string{"segment:" + segmentID},
		}

		attrs := map[string]any{
			"segment_id":      segmentID,
			"segment_type":    "broadcast_domain",
			"parent_devices":  sortedTopologySet(parentDevices),
			"if_names":        sortedTopologySet(ifNames),
			"if_indexes":      sortedTopologySet(ifIndexes),
			"bridge_ports":    sortedTopologySet(bridgePorts),
			"vlan_ids":        sortedTopologySet(vlanIDs),
			"learned_sources": sortedTopologySet(segment.methods),
			"ports_total":     len(segment.ports),
			"endpoints_total": len(segment.endpointIDs),
		}
		if bridgePortRefKey(segment.designatedPort, false, false) != "" {
			attrs["designated_port"] = bridgePortRefSortKey(segment.designatedPort)
		}

		actor := topology.Actor{
			ActorType:  "segment",
			Layer:      layer,
			Source:     source,
			Match:      match,
			Attributes: pruneTopologyAttributes(attrs),
			Labels: map[string]string{
				"segment_kind": "broadcast_domain",
			},
		}
		keys := topologyMatchIdentityKeys(actor.Match)
		if len(keys) > 0 && !topologyIdentityIndexOverlaps(actorIndex, keys) {
			addTopologyIdentityKeys(actorIndex, keys)
		}
		out.actors = append(out.actors, actor)
		segmentMatchByID[segmentID] = match
	}

	deviceSegmentEdgeSeen := make(map[string]struct{})
	endpointSegmentEdgeSeen := make(map[string]struct{})
	for _, segmentID := range segmentIDs {
		segment := segmentByID[segmentID]
		if segment == nil {
			continue
		}
		segmentEndpoint := topology.LinkEndpoint{
			Match: segmentMatchByID[segmentID],
			Attributes: map[string]any{
				"segment_id": segmentID,
			},
		}

		portIDs := make([]string, 0, len(segment.ports))
		for portID := range segment.ports {
			portIDs = append(portIDs, portID)
		}
		sort.Strings(portIDs)
		for _, portID := range portIDs {
			port := segment.ports[portID]
			device, ok := deviceByID[port.deviceID]
			if !ok {
				continue
			}
			localPort := bridgePortDisplay(port)
			if localPort == "" {
				continue
			}
			edgeKey := segmentID + "|" + portID
			if _, seen := deviceSegmentEdgeSeen[edgeKey]; seen {
				continue
			}
			deviceSegmentEdgeSeen[edgeKey] = struct{}{}

			metrics := map[string]any{
				"bridge_domain": segmentID,
			}
			if segment.portIdentityKey(port) == segment.portIdentityKey(segment.designatedPort) {
				metrics["designated"] = true
			}
			out.links = append(out.links, topology.Link{
				Layer:        layer,
				Protocol:     "bridge",
				Direction:    "bidirectional",
				Src:          adjacencySideToEndpoint(device, localPort, ifIndexByDeviceName),
				Dst:          segmentEndpoint,
				DiscoveredAt: topologyTimePtr(collectedAt),
				LastSeen:     topologyTimePtr(collectedAt),
				Metrics:      metrics,
			})
			out.linksFdb++
			out.bidirectionalCount++
		}

		endpointIDs := make([]string, 0, len(segment.endpointIDs))
		for endpointID := range segment.endpointIDs {
			endpointIDs = append(endpointIDs, endpointID)
		}
		sort.Strings(endpointIDs)
		for _, endpointID := range endpointIDs {
			endpointMatch, ok := endpointMatchByID[endpointID]
			if !ok {
				endpointMatch = endpointMatchFromID(endpointID)
				if len(topologyMatchIdentityKeys(endpointMatch)) == 0 {
					continue
				}
			}
			edgeKey := segmentID + "|" + endpointID
			if _, seen := endpointSegmentEdgeSeen[edgeKey]; seen {
				continue
			}
			endpointSegmentEdgeSeen[edgeKey] = struct{}{}

			out.links = append(out.links, topology.Link{
				Layer:        layer,
				Protocol:     "fdb",
				Direction:    "bidirectional",
				Src:          segmentEndpoint,
				Dst:          topology.LinkEndpoint{Match: endpointMatch},
				DiscoveredAt: topologyTimePtr(collectedAt),
				LastSeen:     topologyTimePtr(collectedAt),
				Metrics: map[string]any{
					"bridge_domain": segmentID,
				},
			})
			out.linksFdb++
			out.bidirectionalCount++
		}
	}

	sortTopologyLinks(out.links)
	return out
}

func endpointMatchFromID(endpointID string) topology.Match {
	kind, value, ok := strings.Cut(strings.TrimSpace(endpointID), ":")
	if !ok {
		return topology.Match{}
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "mac":
		mac := normalizeMAC(value)
		if mac == "" {
			return topology.Match{}
		}
		return topology.Match{
			ChassisIDs:   []string{mac},
			MacAddresses: []string{mac},
		}
	case "ip":
		addr := normalizeTopologyIP(value)
		if addr == "" {
			return topology.Match{}
		}
		return topology.Match{
			IPAddresses: []string{addr},
		}
	}
	return topology.Match{}
}

func collectBridgeLinkRecords(adjacencies []Adjacency, ifIndexByDeviceName map[string]int) []bridgeBridgeLinkRecord {
	records := make([]bridgeBridgeLinkRecord, 0)
	seen := make(map[string]struct{})

	for _, adj := range adjacencies {
		protocol := strings.ToLower(strings.TrimSpace(adj.Protocol))
		if protocol != "lldp" && protocol != "cdp" {
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
		if bridgePortRefSortKey(src) > bridgePortRefSortKey(dst) {
			designated = dst
			other = src
		}
		records = append(records, bridgeBridgeLinkRecord{
			port:           other,
			designatedPort: designated,
		})
	}

	sort.SliceStable(records, func(i, j int) bool {
		li := portSortKey(records[i].designatedPort) + "|" + portSortKey(records[i].port)
		lj := portSortKey(records[j].designatedPort) + "|" + portSortKey(records[j].port)
		return li < lj
	})
	return records
}

func collectBridgeMacLinkRecords(attachments []Attachment, ifaceByDeviceIndex map[string]Interface) []bridgeMacLinkRecord {
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

func bridgeDomainSegmentID(segment *bridgeDomainSegment) string {
	if segment == nil {
		return ""
	}
	portKeys := sortedBridgePortSet(segment.ports)
	sig := strings.Join(portKeys, "<->")
	if sig == "" {
		sig = portSortKey(segment.designatedPort)
	}
	return "bridge-domain:" + sig
}

func bridgePortFromAdjacencySide(deviceID, port string, ifIndexByDeviceName map[string]int) bridgePortRef {
	deviceID = strings.TrimSpace(deviceID)
	port = strings.TrimSpace(port)
	if deviceID == "" || port == "" {
		return bridgePortRef{}
	}
	ifIndex := ifIndexByDeviceName[deviceIfNameKey(deviceID, port)]
	if ifIndex == 0 {
		if n, err := strconv.Atoi(port); err == nil && n > 0 {
			ifIndex = n
		}
	}
	return bridgePortRef{
		deviceID:   deviceID,
		ifIndex:    ifIndex,
		ifName:     port,
		bridgePort: port,
	}
}

func bridgePortFromAttachment(attachment Attachment, ifaceByDeviceIndex map[string]Interface) bridgePortRef {
	deviceID := strings.TrimSpace(attachment.DeviceID)
	if deviceID == "" {
		return bridgePortRef{}
	}
	ifIndex := attachment.IfIndex
	ifName := strings.TrimSpace(attachment.Labels["if_name"])
	if ifName == "" && ifIndex > 0 {
		if iface, ok := ifaceByDeviceIndex[deviceIfIndexKey(deviceID, ifIndex)]; ok {
			ifName = strings.TrimSpace(iface.IfName)
		}
	}
	bridgePort := strings.TrimSpace(attachment.Labels["bridge_port"])
	if bridgePort == "" {
		if ifIndex > 0 {
			bridgePort = strconv.Itoa(ifIndex)
		} else {
			bridgePort = ifName
		}
	}
	vlanID := strings.TrimSpace(attachment.Labels["vlan"])
	if vlanID == "" {
		vlanID = strings.TrimSpace(attachment.Labels["vlan_id"])
	}
	return bridgePortRef{
		deviceID:   deviceID,
		ifIndex:    ifIndex,
		ifName:     ifName,
		bridgePort: bridgePort,
		vlanID:     vlanID,
	}
}

func bridgeAttachmentSortKey(attachment Attachment) string {
	parts := []string{
		strings.TrimSpace(attachment.DeviceID),
		strconv.Itoa(attachment.IfIndex),
		strings.TrimSpace(attachment.Labels["if_name"]),
		strings.TrimSpace(attachment.Labels["bridge_port"]),
		strings.TrimSpace(attachment.EndpointID),
	}
	return strings.Join(parts, "|")
}

func bridgePairKey(left, right bridgePortRef) string {
	leftKey := bridgePortRefKey(left, false, false)
	rightKey := bridgePortRefKey(right, false, false)
	if leftKey == "" || rightKey == "" {
		return ""
	}
	if leftKey > rightKey {
		leftKey, rightKey = rightKey, leftKey
	}
	return leftKey + "<->" + rightKey
}

func bridgePortRefKey(port bridgePortRef, includeBridgePort bool, includeVLAN bool) string {
	deviceID := strings.TrimSpace(port.deviceID)
	if deviceID == "" {
		return ""
	}
	bridgePort := strings.TrimSpace(port.bridgePort)
	if bridgePort == "" && port.ifIndex > 0 {
		bridgePort = strconv.Itoa(port.ifIndex)
	}
	ifName := strings.TrimSpace(port.ifName)
	vlanID := strings.TrimSpace(port.vlanID)
	if !includeVLAN {
		vlanID = ""
	}
	parts := []string{
		deviceID,
		"if:" + strconv.Itoa(port.ifIndex),
		"name:" + strings.ToLower(ifName),
	}
	if includeBridgePort {
		parts = append(parts, "bp:"+strings.ToLower(bridgePort))
	}
	parts = append(parts, "vlan:"+strings.ToLower(vlanID))
	return strings.Join(parts, "|")
}

func bridgePortRefSortKey(port bridgePortRef) string {
	return bridgePortRefKey(port, true, true)
}

func bridgePortDisplay(port bridgePortRef) string {
	if name := strings.TrimSpace(port.ifName); name != "" {
		return name
	}
	if port.ifIndex > 0 {
		return strconv.Itoa(port.ifIndex)
	}
	return strings.TrimSpace(port.bridgePort)
}

func adjacencyToTopologyLink(
	adj Adjacency,
	protocol string,
	layer string,
	collectedAt time.Time,
	deviceByID map[string]Device,
	ifIndexByDeviceName map[string]int,
) topology.Link {
	src := adjacencySideToEndpoint(deviceByID[adj.SourceID], adj.SourcePort, ifIndexByDeviceName)
	dst := adjacencySideToEndpoint(deviceByID[adj.TargetID], adj.TargetPort, ifIndexByDeviceName)
	if rawAddress := strings.TrimSpace(adj.Labels["remote_address_raw"]); rawAddress != "" {
		dst.Match.IPAddresses = uniqueTopologyStrings(append(dst.Match.IPAddresses, rawAddress))
	}

	link := topology.Link{
		Layer:        layer,
		Protocol:     protocol,
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
	return key == adjacencyLabelPairID || key == adjacencyLabelPairSide || key == adjacencyLabelPairPass
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

func deviceToTopologyActor(dev Device, source, layer, localDeviceID string) topology.Actor {
	match := topology.Match{
		SysObjectID: strings.TrimSpace(dev.SysObject),
		SysName:     strings.TrimSpace(dev.Hostname),
	}

	chassis := strings.TrimSpace(dev.ChassisID)
	if chassis != "" {
		match.ChassisIDs = []string{chassis}
		if mac := normalizeMAC(chassis); mac != "" {
			match.MacAddresses = []string{mac}
		}
	}

	if len(dev.Addresses) > 0 {
		ips := make([]string, 0, len(dev.Addresses))
		for _, addr := range dev.Addresses {
			if !addr.IsValid() {
				continue
			}
			ips = append(ips, addr.String())
		}
		match.IPAddresses = uniqueTopologyStrings(ips)
	}

	discovered := true
	if strings.TrimSpace(localDeviceID) != "" && dev.ID == localDeviceID {
		discovered = false
	}

	attrs := map[string]any{
		"device_id":              dev.ID,
		"discovered":             discovered,
		"management_ip":          firstAddress(dev.Addresses),
		"management_addresses":   addressStrings(dev.Addresses),
		"capabilities":           labelsCSVToSlice(dev.Labels, "capabilities"),
		"capabilities_supported": labelsCSVToSlice(dev.Labels, "capabilities_supported"),
		"capabilities_enabled":   labelsCSVToSlice(dev.Labels, "capabilities_enabled"),
	}

	return topology.Actor{
		ActorType:  "device",
		Layer:      layer,
		Source:     source,
		Match:      match,
		Attributes: pruneTopologyAttributes(attrs),
		Labels:     cloneStringMap(dev.Labels),
	}
}

func adjacencySideToEndpoint(dev Device, port string, ifIndexByDeviceName map[string]int) topology.LinkEndpoint {
	match := topology.Match{
		SysObjectID: strings.TrimSpace(dev.SysObject),
		SysName:     strings.TrimSpace(dev.Hostname),
	}
	chassis := strings.TrimSpace(dev.ChassisID)
	if chassis != "" {
		match.ChassisIDs = []string{chassis}
		if mac := normalizeMAC(chassis); mac != "" {
			match.MacAddresses = []string{mac}
		}
	}
	for _, addr := range dev.Addresses {
		if !addr.IsValid() {
			continue
		}
		match.IPAddresses = append(match.IPAddresses, addr.String())
	}
	match.IPAddresses = uniqueTopologyStrings(match.IPAddresses)

	port = strings.TrimSpace(port)
	ifName := port
	ifIndex := 0
	if port != "" {
		ifIndex = ifIndexByDeviceName[deviceIfNameKey(dev.ID, port)]
		if ifIndex == 0 {
			if n, err := strconv.Atoi(port); err == nil && n > 0 {
				ifIndex = n
			}
		}
	}
	if ifIndex > 0 && ifName == "" {
		ifName = strconv.Itoa(ifIndex)
	}

	attrs := map[string]any{
		"if_index":      ifIndex,
		"if_name":       ifName,
		"port_id":       port,
		"sys_name":      strings.TrimSpace(dev.Hostname),
		"management_ip": firstAddress(dev.Addresses),
	}

	return topology.LinkEndpoint{
		Match:      match,
		Attributes: pruneTopologyAttributes(attrs),
	}
}

type builtEndpointActors struct {
	actors             []topology.Actor
	count              int
	matchByEndpointID  map[string]topology.Match
	labelsByEndpointID map[string]map[string]string
}

func buildEndpointActors(
	attachments []Attachment,
	enrichments []Enrichment,
	ifaceByDeviceIndex map[string]Interface,
	source string,
	layer string,
	actorIndex map[string]struct{},
) builtEndpointActors {
	accumulators := make(map[string]*endpointActorAccumulator)

	for _, attachment := range attachments {
		endpointID := strings.TrimSpace(attachment.EndpointID)
		if endpointID == "" {
			continue
		}
		acc := ensureEndpointActorAccumulator(accumulators, endpointID)
		addEndpointIDIdentity(acc, endpointID)
		if method := strings.TrimSpace(attachment.Method); method != "" {
			acc.sources[strings.ToLower(method)] = struct{}{}
		}
		if attachment.IfIndex > 0 {
			acc.ifIndexes[strconv.Itoa(attachment.IfIndex)] = struct{}{}
			iface, ok := ifaceByDeviceIndex[deviceIfIndexKey(strings.TrimSpace(attachment.DeviceID), attachment.IfIndex)]
			if ok {
				if ifName := strings.TrimSpace(iface.IfName); ifName != "" {
					acc.ifNames[ifName] = struct{}{}
				}
			}
		}
		if ifName := strings.TrimSpace(attachment.Labels["if_name"]); ifName != "" {
			acc.ifNames[ifName] = struct{}{}
		}
	}

	for _, enrichment := range enrichments {
		endpointID := strings.TrimSpace(enrichment.EndpointID)
		if endpointID == "" {
			continue
		}
		acc := ensureEndpointActorAccumulator(accumulators, endpointID)
		addEndpointIDIdentity(acc, endpointID)

		if mac := normalizeMAC(enrichment.MAC); mac != "" {
			acc.mac = mac
		}
		for _, ip := range enrichment.IPs {
			if ip.IsValid() {
				acc.ips[ip.String()] = ip.Unmap()
			}
		}
		for _, sourceName := range csvToSet(enrichment.Labels["sources"]) {
			acc.sources[sourceName] = struct{}{}
		}
		for _, ifIndex := range csvToSet(enrichment.Labels["if_indexes"]) {
			acc.ifIndexes[ifIndex] = struct{}{}
		}
		for _, ifName := range csvToSet(enrichment.Labels["if_names"]) {
			acc.ifNames[ifName] = struct{}{}
		}
	}

	if len(accumulators) == 0 {
		return builtEndpointActors{
			matchByEndpointID:  map[string]topology.Match{},
			labelsByEndpointID: map[string]map[string]string{},
		}
	}

	keys := make([]string, 0, len(accumulators))
	for endpointID := range accumulators {
		keys = append(keys, endpointID)
	}
	sort.Strings(keys)

	actors := make([]topology.Actor, 0, len(keys))
	endpointCount := 0
	matchByEndpointID := make(map[string]topology.Match, len(keys))
	labelsByEndpointID := make(map[string]map[string]string, len(keys))
	for _, endpointID := range keys {
		acc := accumulators[endpointID]
		if acc == nil {
			continue
		}

		match := topology.Match{}
		if acc.mac != "" {
			match.ChassisIDs = []string{acc.mac}
			match.MacAddresses = []string{acc.mac}
		}
		match.IPAddresses = sortedEndpointIPs(acc.ips)
		matchByEndpointID[endpointID] = match
		labelsByEndpointID[endpointID] = map[string]string{
			"learned_sources": strings.Join(sortedTopologySet(acc.sources), ","),
		}

		attrs := map[string]any{
			"discovered":         true,
			"learned_sources":    sortedTopologySet(acc.sources),
			"learned_if_indexes": sortedTopologySet(acc.ifIndexes),
			"learned_if_names":   sortedTopologySet(acc.ifNames),
		}
		actor := topology.Actor{
			ActorType:  "endpoint",
			Layer:      layer,
			Source:     source,
			Match:      match,
			Attributes: pruneTopologyAttributes(attrs),
		}

		keys := topologyMatchIdentityKeys(actor.Match)
		if len(keys) == 0 {
			continue
		}
		if topologyIdentityIndexOverlaps(actorIndex, keys) {
			continue
		}
		addTopologyIdentityKeys(actorIndex, keys)

		actors = append(actors, actor)
		endpointCount++
	}

	return builtEndpointActors{
		actors:             actors,
		count:              endpointCount,
		matchByEndpointID:  matchByEndpointID,
		labelsByEndpointID: labelsByEndpointID,
	}
}

func ensureEndpointActorAccumulator(accumulators map[string]*endpointActorAccumulator, endpointID string) *endpointActorAccumulator {
	acc := accumulators[endpointID]
	if acc != nil {
		return acc
	}
	acc = &endpointActorAccumulator{
		endpointID: endpointID,
		ips:        make(map[string]netip.Addr),
		sources:    make(map[string]struct{}),
		ifIndexes:  make(map[string]struct{}),
		ifNames:    make(map[string]struct{}),
	}
	accumulators[endpointID] = acc
	return acc
}

func addEndpointIDIdentity(acc *endpointActorAccumulator, endpointID string) {
	if acc == nil {
		return
	}
	kind, value, ok := strings.Cut(strings.TrimSpace(endpointID), ":")
	if !ok {
		return
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "mac":
		if mac := normalizeMAC(value); mac != "" {
			acc.mac = mac
		}
	case "ip":
		if addr := parseAddr(value); addr.IsValid() {
			acc.ips[addr.String()] = addr.Unmap()
		}
	}
}

func discoveredDeviceCount(devices []Device, localDeviceID string) int {
	if len(devices) == 0 {
		return 0
	}

	localDeviceID = strings.TrimSpace(localDeviceID)
	if localDeviceID == "" {
		return maxIntValue(len(devices)-1, 0)
	}

	count := 0
	for _, dev := range devices {
		if strings.TrimSpace(dev.ID) == "" {
			continue
		}
		if dev.ID == localDeviceID {
			continue
		}
		count++
	}
	return count
}

func maxIntValue(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func cloneAnyMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func addressStrings(addresses []netip.Addr) []string {
	if len(addresses) == 0 {
		return nil
	}
	out := make([]string, 0, len(addresses))
	for _, addr := range addresses {
		if !addr.IsValid() {
			continue
		}
		out = append(out, addr.String())
	}
	out = uniqueTopologyStrings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstAddress(addresses []netip.Addr) string {
	values := addressStrings(addresses)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func uniqueTopologyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedEndpointIPs(in map[string]netip.Addr) []string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, key := range keys {
		addr, ok := in[key]
		if !ok || !addr.IsValid() {
			continue
		}
		out = append(out, addr.String())
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sortedTopologySet(in map[string]struct{}) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}

func csvToSet(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func labelsCSVToSlice(labels map[string]string, key string) []string {
	if len(labels) == 0 {
		return nil
	}
	return csvToSet(labels[key])
}

func pruneTopologyAttributes(attrs map[string]any) map[string]any {
	for key, value := range attrs {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) == "" {
				delete(attrs, key)
			}
		case []string:
			if len(typed) == 0 {
				delete(attrs, key)
			}
		case map[string]string:
			if len(typed) == 0 {
				delete(attrs, key)
			}
		case map[string]any:
			if len(typed) == 0 {
				delete(attrs, key)
			}
		case int:
			if typed == 0 {
				delete(attrs, key)
			}
		case nil:
			delete(attrs, key)
		}
	}
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func mapStringStringToAny(in map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func deviceIfNameKey(deviceID, ifName string) string {
	return fmt.Sprintf("%s|%s", strings.TrimSpace(deviceID), strings.ToLower(strings.TrimSpace(ifName)))
}

func topologyIdentityIndexOverlaps(index map[string]struct{}, keys []string) bool {
	if len(index) == 0 || len(keys) == 0 {
		return false
	}
	for _, key := range keys {
		if _, ok := index[key]; ok {
			return true
		}
	}
	return false
}

func addTopologyIdentityKeys(index map[string]struct{}, keys []string) {
	if index == nil || len(keys) == 0 {
		return
	}
	for _, key := range keys {
		index[key] = struct{}{}
	}
}

func topologyMatchIdentityKeys(match topology.Match) []string {
	seen := make(map[string]struct{}, 8)
	add := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := kind + ":" + value
		seen[key] = struct{}{}
	}

	for _, value := range match.ChassisIDs {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mac := normalizeMAC(value); mac != "" {
			add("hw", mac)
			continue
		}
		if ip := normalizeTopologyIP(value); ip != "" {
			add("ip", ip)
			continue
		}
		add("chassis", strings.ToLower(value))
	}

	for _, value := range match.MacAddresses {
		if mac := normalizeMAC(value); mac != "" {
			add("hw", mac)
		}
	}
	for _, value := range match.IPAddresses {
		if ip := normalizeTopologyIP(value); ip != "" {
			add("ip", ip)
			continue
		}
		add("ipraw", strings.ToLower(strings.TrimSpace(value)))
	}
	for _, value := range match.Hostnames {
		add("hostname", strings.ToLower(strings.TrimSpace(value)))
	}
	for _, value := range match.DNSNames {
		add("dns", strings.ToLower(strings.TrimSpace(value)))
	}
	if sysName := strings.TrimSpace(match.SysName); sysName != "" {
		add("sysname", strings.ToLower(sysName))
	}

	if len(seen) == 0 {
		return nil
	}

	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizeTopologyIP(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	addr := parseAddr(value)
	if !addr.IsValid() {
		return ""
	}
	return addr.Unmap().String()
}

func canonicalTopologyMatchKey(match topology.Match) string {
	if key := canonicalTopologyHardwareKey(match.ChassisIDs); key != "" {
		return "chassis:" + key
	}
	if key := canonicalTopologyMACListKey(match.MacAddresses); key != "" {
		return "mac:" + key
	}
	if key := canonicalTopologyIPListKey(match.IPAddresses); key != "" {
		return "ip:" + key
	}
	if key := canonicalTopologyStringListKey(match.Hostnames); key != "" {
		return "hostname:" + key
	}
	if key := canonicalTopologyStringListKey(match.DNSNames); key != "" {
		return "dns:" + key
	}
	if sysName := strings.ToLower(strings.TrimSpace(match.SysName)); sysName != "" {
		return "sysname:" + sysName
	}
	if match.SysObjectID != "" {
		return "sysobjectid:" + match.SysObjectID
	}
	return ""
}

func canonicalTopologyHardwareKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if mac := normalizeMAC(value); mac != "" {
			out = append(out, mac)
			continue
		}
		if ip := normalizeTopologyIP(value); ip != "" {
			out = append(out, ip)
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueTopologyStrings(out)
	return strings.Join(out, ",")
}

func canonicalTopologyMACListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if mac := normalizeMAC(value); mac != "" {
			out = append(out, mac)
		}
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueTopologyStrings(out)
	return strings.Join(out, ",")
}

func canonicalTopologyIPListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if ip := normalizeTopologyIP(value); ip != "" {
			out = append(out, ip)
			continue
		}
		out = append(out, strings.ToLower(value))
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueTopologyStrings(out)
	return strings.Join(out, ",")
}

func canonicalTopologyStringListKey(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return ""
	}
	sort.Strings(out)
	out = uniqueTopologyStrings(out)
	return strings.Join(out, ",")
}

func topologyLinkSortKey(link topology.Link) string {
	return strings.Join([]string{
		link.Protocol,
		link.Direction,
		canonicalTopologyMatchKey(link.Src.Match),
		canonicalTopologyMatchKey(link.Dst.Match),
		topologyAttrKey(link.Src.Attributes, "if_index"),
		topologyAttrKey(link.Src.Attributes, "if_name"),
		topologyAttrKey(link.Src.Attributes, "port_id"),
		topologyAttrKey(link.Dst.Attributes, "if_index"),
		topologyAttrKey(link.Dst.Attributes, "if_name"),
		topologyAttrKey(link.Dst.Attributes, "port_id"),
		link.State,
	}, "|")
}

func topologyAttrKey(attrs map[string]any, key string) string {
	if len(attrs) == 0 {
		return ""
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return ""
	}
	return fmt.Sprint(value)
}

func sortTopologyActors(actors []topology.Actor) {
	sort.SliceStable(actors, func(i, j int) bool {
		a, b := actors[i], actors[j]
		if a.ActorType != b.ActorType {
			return a.ActorType < b.ActorType
		}
		ak := canonicalTopologyMatchKey(a.Match)
		bk := canonicalTopologyMatchKey(b.Match)
		if ak != bk {
			return ak < bk
		}
		if a.Source != b.Source {
			return a.Source < b.Source
		}
		return a.Layer < b.Layer
	})
}

func sortTopologyLinks(links []topology.Link) {
	sort.SliceStable(links, func(i, j int) bool {
		return topologyLinkSortKey(links[i]) < topologyLinkSortKey(links[j])
	})
}

func topologyTimePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	out := t
	return &out
}
