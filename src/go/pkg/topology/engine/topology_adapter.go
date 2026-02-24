// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"fmt"
	"math"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
)

// TopologyDataOptions controls conversion from Result to topology.Data.
type TopologyDataOptions struct {
	SchemaVersion  string
	Source         string
	Layer          string
	View           string
	AgentID        string
	LocalDeviceID  string
	CollectedAt    time.Time
	ResolveDNSName func(ip string) string
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
	actors                        []topology.Actor
	links                         []topology.Link
	linksFdb                      int
	bidirectionalCount            int
	endpointLinksCandidates       int
	endpointLinksEmitted          int
	endpointLinksSuppressed       int
	endpointsWithAmbiguousSegment int
	endpointDirectOwners          map[string]fdbEndpointOwner
}

type fdbReporterObservation struct {
	byEndpoint map[string]map[string]map[string]struct{}
	byReporter map[string]map[string]map[string]struct{}
}

type fdbEndpointOwner struct {
	portKey     string
	portVLANKey string
	port        bridgePortRef
	source      string
}

type topologyIdentityKeySet map[string]struct{}

type topologyDevicePortStatus struct {
	IfIndex        int
	IfName         string
	InterfaceType  string
	AdminStatus    string
	OperStatus     string
	LinkMode       string
	ModeConfidence string
	ModeSources    []string
	VLANIDs        []string
	TopologyRole   string
	RoleConfidence string
	RoleSources    []string
}

type topologyDeviceInterfaceSummary struct {
	portsTotal       int
	ifIndexes        []string
	ifNames          []string
	adminStatusCount map[string]any
	operStatusCount  map[string]any
	linkModeCount    map[string]any
	roleCount        map[string]any
	portStatuses     []map[string]any
}

type topologyDevicePortEvidence struct {
	vlanIDs            map[string]struct{}
	fdbEndpointIDs     map[string]struct{}
	hasFDB             bool
	hasFDBManagedAlias bool
	hasSTP             bool
	hasPeer            bool
	hasBridgeLink      bool
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
	bridgeLinks := collectBridgeLinkRecords(result.Adjacencies, ifIndexByDeviceName)
	reporterAliases := buildFDBReporterAliases(deviceByID, ifaceByDeviceIndex)
	ifaceSummaryByDevice := buildTopologyDeviceInterfaceSummaries(
		result.Interfaces,
		result.Attachments,
		result.Adjacencies,
		ifIndexByDeviceName,
		bridgeLinks,
		reporterAliases,
	)

	actors := make([]topology.Actor, 0, len(result.Devices))
	actorIndex := make(map[string]struct{}, len(result.Devices)*2)
	actorMACIndex := make(map[string]struct{}, len(result.Devices))
	for _, dev := range result.Devices {
		actor := deviceToTopologyActor(
			dev,
			source,
			layer,
			opts.LocalDeviceID,
			ifaceSummaryByDevice[dev.ID],
			reporterAliases[dev.ID],
		)
		keys := topologyMatchIdentityKeys(actor.Match)
		if len(keys) == 0 {
			continue
		}
		macKeys := topologyMatchHardwareIdentityKeys(actor.Match)
		if len(macKeys) > 0 {
			if topologyIdentityIndexOverlaps(actorMACIndex, macKeys) {
				continue
			}
			addTopologyIdentityKeys(actorMACIndex, macKeys)
		} else if topologyIdentityIndexOverlaps(actorIndex, keys) {
			continue
		}
		addTopologyIdentityKeys(actorIndex, keys)
		actors = append(actors, actor)
	}

	projected := projectAdjacencyLinks(result.Adjacencies, layer, collectedAt, deviceByID, ifIndexByDeviceName, ifaceByDeviceIndex)

	endpointActors := buildEndpointActors(result.Attachments, result.Enrichments, ifaceByDeviceIndex, source, layer, actorIndex, actorMACIndex)
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
		bridgeLinks,
		reporterAliases,
		endpointActors.matchByEndpointID,
		actorIndex,
	)
	annotateEndpointActorsWithDirectOwners(actors, endpointActors.matchByEndpointID, segmentProjection.endpointDirectOwners, deviceByID)
	actors = append(actors, segmentProjection.actors...)
	sortTopologyActors(actors)

	links := make([]topology.Link, 0, len(projected.links)+len(segmentProjection.links))
	links = append(links, projected.links...)
	links = append(links, segmentProjection.links...)
	sortTopologyLinks(links)
	segmentSuppressed := 0
	actors, links, segmentSuppressed = pruneSegmentArtifacts(actors, links)
	sortTopologyActors(actors)
	sortTopologyLinks(links)
	applyTopologyDisplayNames(actors, links, opts.ResolveDNSName)
	assignTopologyActorIDsAndLinkEndpoints(actors, links)
	linkCounts := summarizeTopologyLinks(links)

	stats := cloneAnyMap(result.Stats)
	if stats == nil {
		stats = make(map[string]any)
	}
	stats["devices_total"] = len(result.Devices)
	stats["devices_discovered"] = discoveredDeviceCount(result.Devices, opts.LocalDeviceID)
	stats["links_total"] = len(links)
	stats["links_lldp"] = linkCounts.lldp
	stats["links_cdp"] = linkCounts.cdp
	stats["links_bidirectional"] = linkCounts.bidirectional
	stats["links_unidirectional"] = linkCounts.unidirectional
	stats["links_fdb"] = linkCounts.fdb
	stats["links_fdb_endpoint_candidates"] = segmentProjection.endpointLinksCandidates
	stats["links_fdb_endpoint_emitted"] = segmentProjection.endpointLinksEmitted
	stats["links_fdb_endpoint_suppressed"] = segmentProjection.endpointLinksSuppressed
	stats["endpoints_ambiguous_segments"] = segmentProjection.endpointsWithAmbiguousSegment
	stats["links_arp"] = linkCounts.arp
	stats["segments_suppressed"] = segmentSuppressed
	stats["actors_total"] = len(actors)
	stats["actors_unlinked_suppressed"] = 0
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
	bridgeLinks []bridgeBridgeLinkRecord,
	reporterAliases map[string][]string,
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

	switchFacingPortKeys := buildSwitchFacingPortKeySet(bridgeLinks)
	seedMacLinks := collectBridgeMacLinkRecords(attachments, ifaceByDeviceIndex, switchFacingPortKeys)
	managedAliasEndpointIDs := buildManagedAliasEndpointIDSet(reporterAliases)
	switchFacingPortKeys = augmentSwitchFacingPortKeySetFromManagedAliases(
		switchFacingPortKeys,
		buildFDBReporterObservations(seedMacLinks),
		reporterAliases,
	)
	macLinks := filterBridgeMacLinkRecordsBySwitchFacing(seedMacLinks, switchFacingPortKeys, managedAliasEndpointIDs)
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
	endpointSegmentCandidates := make(map[string][]string)
	segmentPortKeys := make(map[string]map[string]struct{}, len(segmentIDs))
	for _, segmentID := range segmentIDs {
		segment := segmentByID[segmentID]
		if segment == nil {
			continue
		}
		portKeys := make(map[string]struct{}, len(segment.ports))
		for _, port := range segment.ports {
			if portKey := bridgePortObservationKey(port); portKey != "" {
				portKeys[portKey] = struct{}{}
			}
			if portVLANKey := bridgePortObservationVLANKey(port); portVLANKey != "" {
				portKeys[portVLANKey] = struct{}{}
			}
		}
		segmentPortKeys[segmentID] = portKeys
		for endpointID := range segment.endpointIDs {
			endpointID = strings.TrimSpace(endpointID)
			if endpointID == "" {
				continue
			}
			endpointSegmentCandidates[endpointID] = append(endpointSegmentCandidates[endpointID], segmentID)
		}
	}
	fdbObservations := buildFDBReporterObservations(macLinks)
	fdbOwners := inferFDBEndpointOwners(fdbObservations, reporterAliases, switchFacingPortKeys)
	for endpointID, owner := range inferSinglePortEndpointOwners(macLinks, switchFacingPortKeys) {
		if strings.TrimSpace(endpointID) == "" {
			continue
		}
		if fdbOwners == nil {
			fdbOwners = make(map[string]fdbEndpointOwner)
		}
		// Port-centric ownership has precedence: if a port carries a single learned
		// MAC in the same snapshot/VLAN scope, use it to resolve ambiguous placement.
		fdbOwners[endpointID] = owner
		if out.endpointDirectOwners == nil {
			out.endpointDirectOwners = make(map[string]fdbEndpointOwner)
		}
		out.endpointDirectOwners[endpointID] = owner
	}
	deviceIdentityByID := buildDeviceIdentityKeySetByID(deviceByID, adjacencies, ifaceByDeviceIndex)
	allowedEndpointBySegment := make(map[string]map[string]struct{})
	for endpointID, candidates := range endpointSegmentCandidates {
		candidateSet := make(map[string]struct{}, len(candidates))
		for _, candidate := range candidates {
			candidate = strings.TrimSpace(candidate)
			if candidate == "" {
				continue
			}
			candidateSet[candidate] = struct{}{}
		}
		sortedCandidates := sortedTopologySet(candidateSet)
		out.endpointLinksCandidates += len(sortedCandidates)
		if len(sortedCandidates) == 1 {
			segmentID := sortedCandidates[0]
			allowed := allowedEndpointBySegment[segmentID]
			if allowed == nil {
				allowed = make(map[string]struct{})
				allowedEndpointBySegment[segmentID] = allowed
			}
			allowed[endpointID] = struct{}{}
			continue
		}

		if owner, ok := fdbOwners[endpointID]; ok {
			filtered := make([]string, 0, len(sortedCandidates))
			for _, segmentID := range sortedCandidates {
				portKeys := segmentPortKeys[segmentID]
				if len(portKeys) == 0 {
					continue
				}
				if _, matchesOwnerPort := portKeys[owner.portVLANKey]; matchesOwnerPort {
					filtered = append(filtered, segmentID)
					continue
				}
				if _, matchesOwnerPort := portKeys[owner.portKey]; matchesOwnerPort {
					filtered = append(filtered, segmentID)
				}
			}
			if len(filtered) == 1 {
				segmentID := filtered[0]
				allowed := allowedEndpointBySegment[segmentID]
				if allowed == nil {
					allowed = make(map[string]struct{})
					allowedEndpointBySegment[segmentID] = allowed
				}
				allowed[endpointID] = struct{}{}
				continue
			}
		}

		if len(sortedCandidates) > 1 {
			out.endpointsWithAmbiguousSegment++
			out.endpointLinksSuppressed += len(sortedCandidates)
		}
	}

	segmentsWithAnyLinks := make(map[string]struct{})
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
				Src:          adjacencySideToEndpoint(device, localPort, ifIndexByDeviceName, ifaceByDeviceIndex),
				Dst:          segmentEndpoint,
				DiscoveredAt: topologyTimePtr(collectedAt),
				LastSeen:     topologyTimePtr(collectedAt),
				Metrics:      metrics,
			})
			out.linksFdb++
			out.bidirectionalCount++
			segmentsWithAnyLinks[segmentID] = struct{}{}
		}

		endpointIDs := make([]string, 0, len(segment.endpointIDs))
		for endpointID := range segment.endpointIDs {
			endpointIDs = append(endpointIDs, endpointID)
		}
		sort.Strings(endpointIDs)
		allowedEndpoints := allowedEndpointBySegment[segmentID]
		for _, endpointID := range endpointIDs {
			if len(allowedEndpoints) == 0 {
				continue
			}
			if _, ok := allowedEndpoints[endpointID]; !ok {
				continue
			}

			endpointMatch, ok := endpointMatchByID[endpointID]
			if !ok {
				endpointMatch = endpointMatchFromID(endpointID)
				if len(topologyMatchIdentityKeys(endpointMatch)) == 0 {
					continue
				}
			}
			overlappingDeviceIDs := endpointMatchOverlappingKnownDeviceIDs(endpointMatch, deviceIdentityByID)
			if len(overlappingDeviceIDs) > 0 {
				if len(overlappingDeviceIDs) == 1 {
					matchedDeviceID := overlappingDeviceIDs[0]
					if segmentContainsDevice(segment, matchedDeviceID) {
						out.endpointLinksSuppressed++
						continue
					}
					if matchedDevice, ok := deviceByID[matchedDeviceID]; ok {
						edgeKey := segmentID + "|managed-device|" + matchedDeviceID
						if _, seen := endpointSegmentEdgeSeen[edgeKey]; !seen {
							endpointSegmentEdgeSeen[edgeKey] = struct{}{}
							out.links = append(out.links, topology.Link{
								Layer:        layer,
								Protocol:     "fdb",
								Direction:    "bidirectional",
								Src:          segmentEndpoint,
								Dst:          adjacencySideToEndpoint(matchedDevice, "", ifIndexByDeviceName, ifaceByDeviceIndex),
								DiscoveredAt: topologyTimePtr(collectedAt),
								LastSeen:     topologyTimePtr(collectedAt),
								Metrics: map[string]any{
									"bridge_domain":   segmentID,
									"attachment_mode": "managed_device_overlap",
								},
							})
							out.linksFdb++
							out.bidirectionalCount++
							out.endpointLinksEmitted++
							segmentsWithAnyLinks[segmentID] = struct{}{}
						}
						continue
					}
				}
				out.endpointLinksSuppressed++
				continue
			}

			if owner, hasOwner := out.endpointDirectOwners[endpointID]; hasOwner &&
				strings.EqualFold(strings.TrimSpace(owner.source), "single_port_mac") {
				device, ok := deviceByID[owner.port.deviceID]
				if ok {
					localPort := bridgePortDisplay(owner.port)
					if localPort != "" {
						edgeKey := "direct|" + bridgePortObservationVLANKey(owner.port) + "|" + endpointID
						if _, seen := endpointSegmentEdgeSeen[edgeKey]; !seen {
							endpointSegmentEdgeSeen[edgeKey] = struct{}{}
							out.links = append(out.links, topology.Link{
								Layer:        layer,
								Protocol:     "fdb",
								Direction:    "bidirectional",
								Src:          adjacencySideToEndpoint(device, localPort, ifIndexByDeviceName, ifaceByDeviceIndex),
								Dst:          topology.LinkEndpoint{Match: endpointMatch},
								DiscoveredAt: topologyTimePtr(collectedAt),
								LastSeen:     topologyTimePtr(collectedAt),
								Metrics: map[string]any{
									"attachment_mode": "direct",
								},
							})
							out.linksFdb++
							out.bidirectionalCount++
							out.endpointLinksEmitted++
							continue
						}
					}
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
			out.endpointLinksEmitted++
			segmentsWithAnyLinks[segmentID] = struct{}{}
		}
	}

	if len(segmentsWithAnyLinks) < len(segmentIDs) {
		filteredActors := make([]topology.Actor, 0, len(out.actors))
		for _, actor := range out.actors {
			segmentID := topologyAttrString(actor.Attributes, "segment_id")
			if segmentID == "" {
				continue
			}
			if _, ok := segmentsWithAnyLinks[segmentID]; ok {
				filteredActors = append(filteredActors, actor)
			}
		}
		out.actors = filteredActors

		filteredLinks := make([]topology.Link, 0, len(out.links))
		out.linksFdb = 0
		out.bidirectionalCount = 0
		out.endpointLinksEmitted = 0
		for _, link := range out.links {
			segmentID := topologyMetricString(link.Metrics, "bridge_domain")
			if segmentID != "" {
				if _, ok := segmentsWithAnyLinks[segmentID]; !ok {
					continue
				}
			}
			filteredLinks = append(filteredLinks, link)
			out.linksFdb++
			if strings.EqualFold(strings.TrimSpace(link.Direction), "bidirectional") {
				out.bidirectionalCount++
			}
			if strings.EqualFold(strings.TrimSpace(link.Protocol), "fdb") {
				out.endpointLinksEmitted++
			}
		}
		out.links = filteredLinks
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

func annotateEndpointActorsWithDirectOwners(
	actors []topology.Actor,
	endpointMatchByID map[string]topology.Match,
	owners map[string]fdbEndpointOwner,
	deviceByID map[string]Device,
) {
	if len(actors) == 0 || len(owners) == 0 {
		return
	}

	ownerByMatchKey := make(map[string]fdbEndpointOwner, len(owners))
	endpointIDs := make([]string, 0, len(owners))
	for endpointID := range owners {
		endpointIDs = append(endpointIDs, endpointID)
	}
	sort.Strings(endpointIDs)

	for _, endpointID := range endpointIDs {
		owner := owners[endpointID]
		if !strings.EqualFold(strings.TrimSpace(owner.source), "single_port_mac") {
			continue
		}
		match, ok := endpointMatchByID[endpointID]
		if !ok {
			match = endpointMatchFromID(endpointID)
		}
		key := canonicalTopologyMatchKey(match)
		if key == "" {
			continue
		}
		ownerByMatchKey[key] = owner
	}

	if len(ownerByMatchKey) == 0 {
		return
	}

	for i := range actors {
		actor := &actors[i]
		if !strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
			continue
		}
		key := canonicalTopologyMatchKey(actor.Match)
		if key == "" {
			continue
		}
		owner, ok := ownerByMatchKey[key]
		if !ok {
			continue
		}

		attrs := cloneAnyMap(actor.Attributes)
		if attrs == nil {
			attrs = make(map[string]any)
		}
		labels := cloneStringMap(actor.Labels)
		if labels == nil {
			labels = make(map[string]string)
		}

		deviceID := strings.TrimSpace(owner.port.deviceID)
		port := bridgePortDisplay(owner.port)
		ifName := strings.TrimSpace(owner.port.ifName)
		bridgePort := strings.TrimSpace(owner.port.bridgePort)
		vlanID := strings.TrimSpace(owner.port.vlanID)

		attrs["attachment_source"] = "single_port_mac"
		if deviceID != "" {
			attrs["attached_device_id"] = deviceID
			labels["attached_device_id"] = deviceID
		}
		if port != "" {
			attrs["attached_port"] = port
			labels["attached_port"] = port
		}
		if ifName != "" {
			attrs["attached_if_name"] = ifName
		}
		if owner.port.ifIndex > 0 {
			attrs["attached_if_index"] = owner.port.ifIndex
		}
		if bridgePort != "" {
			attrs["attached_bridge_port"] = bridgePort
		}
		if vlanID != "" {
			attrs["attached_vlan"] = vlanID
			attrs["attached_vlan_id"] = vlanID
		}
		if device, ok := deviceByID[deviceID]; ok {
			display := strings.TrimSpace(device.Hostname)
			if display == "" {
				display = deviceID
			}
			if display != "" {
				attrs["attached_device"] = display
				labels["attached_device"] = display
			}
		}
		labels["attached_by"] = "single_port_mac"

		actor.Attributes = pruneTopologyAttributes(attrs)
		if len(labels) > 0 {
			actor.Labels = labels
		}
	}
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

func buildFDBReporterAliases(
	deviceByID map[string]Device,
	ifaceByDeviceIndex map[string]Interface,
) map[string][]string {
	aliases := make(map[string]map[string]struct{}, len(deviceByID))

	for _, device := range deviceByID {
		deviceID := strings.TrimSpace(device.ID)
		if deviceID == "" {
			continue
		}
		chassisMAC := normalizeMAC(device.ChassisID)
		if chassisMAC == "" {
			continue
		}
		if aliases[deviceID] == nil {
			aliases[deviceID] = make(map[string]struct{})
		}
		aliases[deviceID]["mac:"+chassisMAC] = struct{}{}
	}

	for _, iface := range ifaceByDeviceIndex {
		deviceID := strings.TrimSpace(iface.DeviceID)
		if deviceID == "" {
			continue
		}
		ifaceMAC := normalizeMAC(iface.MAC)
		if ifaceMAC == "" {
			continue
		}
		if aliases[deviceID] == nil {
			aliases[deviceID] = make(map[string]struct{})
		}
		aliases[deviceID]["mac:"+ifaceMAC] = struct{}{}
	}

	out := make(map[string][]string, len(aliases))
	for deviceID, set := range aliases {
		values := sortedTopologySet(set)
		if len(values) == 0 {
			continue
		}
		out[deviceID] = values
	}

	return out
}

func buildDeviceIdentityKeySetByID(
	deviceByID map[string]Device,
	adjacencies []Adjacency,
	ifaceByDeviceIndex map[string]Interface,
) map[string]topologyIdentityKeySet {
	if len(deviceByID) == 0 {
		return nil
	}
	out := make(map[string]topologyIdentityKeySet, len(deviceByID))
	for _, device := range deviceByID {
		deviceID := strings.TrimSpace(device.ID)
		if deviceID == "" {
			continue
		}
		keys := topologyMatchIdentityKeys(
			deviceToTopologyActor(device, "", "", "", topologyDeviceInterfaceSummary{}, nil).Match,
		)
		if len(keys) == 0 {
			continue
		}
		set := make(topologyIdentityKeySet, len(keys))
		for _, key := range keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			set[key] = struct{}{}
		}
		if len(set) == 0 {
			continue
		}
		out[deviceID] = set
	}
	for _, adjacency := range adjacencies {
		protocol := strings.ToLower(strings.TrimSpace(adjacency.Protocol))
		if protocol != "lldp" && protocol != "cdp" {
			continue
		}
		if mac := normalizeMAC(adjacency.SourcePort); mac != "" {
			deviceID := strings.TrimSpace(adjacency.SourceID)
			if deviceID != "" {
				if out[deviceID] == nil {
					out[deviceID] = make(topologyIdentityKeySet)
				}
				out[deviceID]["hw:"+mac] = struct{}{}
			}
		}
		if mac := normalizeMAC(adjacency.TargetPort); mac != "" {
			deviceID := strings.TrimSpace(adjacency.TargetID)
			if deviceID != "" {
				if out[deviceID] == nil {
					out[deviceID] = make(topologyIdentityKeySet)
				}
				out[deviceID]["hw:"+mac] = struct{}{}
			}
		}
	}
	for _, iface := range ifaceByDeviceIndex {
		deviceID := strings.TrimSpace(iface.DeviceID)
		if deviceID == "" {
			continue
		}
		ifaceMAC := normalizeMAC(iface.MAC)
		if ifaceMAC == "" {
			continue
		}
		if out[deviceID] == nil {
			out[deviceID] = make(topologyIdentityKeySet)
		}
		out[deviceID]["hw:"+ifaceMAC] = struct{}{}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildFDBReporterObservations(macLinks []bridgeMacLinkRecord) fdbReporterObservation {
	obs := fdbReporterObservation{
		byEndpoint: make(map[string]map[string]map[string]struct{}),
		byReporter: make(map[string]map[string]map[string]struct{}),
	}
	for _, link := range macLinks {
		if strings.ToLower(strings.TrimSpace(link.method)) != "fdb" {
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
		portKey := bridgePortObservationKey(link.port)
		if portKey == "" {
			continue
		}

		byEndpointReporter := obs.byEndpoint[endpointID]
		if byEndpointReporter == nil {
			byEndpointReporter = make(map[string]map[string]struct{})
			obs.byEndpoint[endpointID] = byEndpointReporter
		}
		if byEndpointReporter[reporterID] == nil {
			byEndpointReporter[reporterID] = make(map[string]struct{})
		}
		byEndpointReporter[reporterID][portKey] = struct{}{}

		byReporterEndpoint := obs.byReporter[reporterID]
		if byReporterEndpoint == nil {
			byReporterEndpoint = make(map[string]map[string]struct{})
			obs.byReporter[reporterID] = byReporterEndpoint
		}
		if byReporterEndpoint[endpointID] == nil {
			byReporterEndpoint[endpointID] = make(map[string]struct{})
		}
		byReporterEndpoint[endpointID][portKey] = struct{}{}
	}
	return obs
}

func normalizeFDBEndpointID(endpointID string) string {
	kind, value, ok := strings.Cut(strings.TrimSpace(endpointID), ":")
	if !ok {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "mac":
		if mac := normalizeMAC(value); mac != "" {
			return "mac:" + mac
		}
	}
	return ""
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

func buildFDBAliasOwnerMap(reporterAliases map[string][]string) map[string]map[string]struct{} {
	if len(reporterAliases) == 0 {
		return nil
	}
	aliasOwners := make(map[string]map[string]struct{})
	reporterIDs := make([]string, 0, len(reporterAliases))
	for reporterID := range reporterAliases {
		reporterID = strings.TrimSpace(reporterID)
		if reporterID == "" {
			continue
		}
		reporterIDs = append(reporterIDs, reporterID)
	}
	sort.Strings(reporterIDs)
	for _, reporterID := range reporterIDs {
		aliases := uniqueTopologyStrings(reporterAliases[reporterID])
		for _, alias := range aliases {
			alias = normalizeFDBEndpointID(alias)
			if alias == "" {
				continue
			}
			owners := aliasOwners[alias]
			if owners == nil {
				owners = make(map[string]struct{})
				aliasOwners[alias] = owners
			}
			owners[reporterID] = struct{}{}
		}
	}
	if len(aliasOwners) == 0 {
		return nil
	}
	return aliasOwners
}

func buildManagedAliasEndpointIDSet(reporterAliases map[string][]string) map[string]struct{} {
	if len(reporterAliases) == 0 {
		return nil
	}
	out := make(map[string]struct{})
	for _, aliases := range reporterAliases {
		for _, alias := range aliases {
			alias = normalizeFDBEndpointID(alias)
			if alias == "" {
				continue
			}
			out[alias] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func filterBridgeMacLinkRecordsBySwitchFacing(
	macLinks []bridgeMacLinkRecord,
	switchFacingPortKeys map[string]struct{},
	managedAliasEndpointIDs map[string]struct{},
) []bridgeMacLinkRecord {
	if len(macLinks) == 0 || len(switchFacingPortKeys) == 0 {
		return macLinks
	}
	out := make([]bridgeMacLinkRecord, 0, len(macLinks))
	for _, link := range macLinks {
		if strings.ToLower(strings.TrimSpace(link.method)) == "fdb" {
			endpointID := normalizeFDBEndpointID(link.endpointID)
			if endpointID != "" {
				if _, keepManagedAlias := managedAliasEndpointIDs[endpointID]; keepManagedAlias {
					out = append(out, link)
					continue
				}
			}
			if _, isSwitchFacing := switchFacingPortKeys[bridgePortObservationKey(link.port)]; isSwitchFacing {
				continue
			}
			if _, isSwitchFacing := switchFacingPortKeys[bridgePortObservationVLANKey(link.port)]; isSwitchFacing {
				continue
			}
		}
		out = append(out, link)
	}
	return out
}

func inferFDBEndpointOwners(
	observations fdbReporterObservation,
	reporterAliases map[string][]string,
	switchFacingPortKeys map[string]struct{},
) map[string]fdbEndpointOwner {
	if len(observations.byEndpoint) == 0 {
		return nil
	}

	owners := make(map[string]fdbEndpointOwner, len(observations.byEndpoint))
	endpointIDs := make([]string, 0, len(observations.byEndpoint))
	for endpointID := range observations.byEndpoint {
		endpointIDs = append(endpointIDs, endpointID)
	}
	sort.Strings(endpointIDs)

	for _, endpointID := range endpointIDs {
		reportersMap := observations.byEndpoint[endpointID]
		if len(reportersMap) < 2 {
			continue
		}

		reporterIDs := make([]string, 0, len(reportersMap))
		for reporterID := range reportersMap {
			reporterIDs = append(reporterIDs, reporterID)
		}
		sort.Strings(reporterIDs)

		validPortsByReporter := make(map[string][]string)
		for _, reporterID := range reporterIDs {
			ports := sortedTopologySet(reportersMap[reporterID])
			if len(ports) == 0 {
				continue
			}

			for _, endpointPort := range ports {
				if _, isSwitchFacingPort := switchFacingPortKeys[endpointPort]; isSwitchFacingPort {
					continue
				}
				if !reporterSatisfiesFDBOwnerRule(endpointPort, reporterID, reporterIDs, observations.byReporter, reporterAliases) {
					continue
				}
				validPortsByReporter[reporterID] = append(validPortsByReporter[reporterID], endpointPort)
			}
		}

		if len(validPortsByReporter) != 1 {
			continue
		}
		for _, ports := range validPortsByReporter {
			ports = uniqueTopologyStrings(ports)
			if len(ports) == 0 {
				continue
			}
			owners[endpointID] = fdbEndpointOwner{
				portKey: ports[0],
				source:  "reporter_matrix",
			}
		}
	}

	if len(owners) == 0 {
		return nil
	}
	return owners
}

func reporterSatisfiesFDBOwnerRule(
	endpointPort string,
	reporterID string,
	reporterIDs []string,
	reporterObservations map[string]map[string]map[string]struct{},
	reporterAliases map[string][]string,
) bool {
	reporterEndpoints := reporterObservations[reporterID]
	if len(reporterEndpoints) == 0 {
		return false
	}

	for _, otherReporterID := range reporterIDs {
		if otherReporterID == reporterID {
			continue
		}
		aliases := reporterAliases[otherReporterID]
		if len(aliases) == 0 {
			return false
		}

		seenOtherOnDifferentPort := false
		for _, alias := range aliases {
			ports := reporterEndpoints[alias]
			if len(ports) == 0 {
				continue
			}
			for observedPort := range ports {
				if observedPort == endpointPort {
					return false
				}
				seenOtherOnDifferentPort = true
			}
		}
		if !seenOtherOnDifferentPort {
			return false
		}
	}

	return true
}

func bridgePortObservationKey(port bridgePortRef) string {
	return bridgePortRefKey(port, false, false)
}

func bridgePortObservationVLANKey(port bridgePortRef) string {
	return bridgePortRefKey(port, false, true)
}

func inferSinglePortEndpointOwners(
	macLinks []bridgeMacLinkRecord,
	switchFacingPortKeys map[string]struct{},
) map[string]fdbEndpointOwner {
	if len(macLinks) == 0 {
		return nil
	}

	type portScope struct {
		portKey     string
		portVLANKey string
		port        bridgePortRef
		endpointIDs map[string]struct{}
	}

	byPortScope := make(map[string]*portScope)
	for _, link := range macLinks {
		if strings.ToLower(strings.TrimSpace(link.method)) != "fdb" {
			continue
		}
		endpointID := normalizeFDBEndpointID(link.endpointID)
		if endpointID == "" {
			continue
		}

		portKey := bridgePortObservationKey(link.port)
		if portKey == "" {
			continue
		}
		if _, isSwitchFacingPort := switchFacingPortKeys[portKey]; isSwitchFacingPort {
			continue
		}
		portVLANKey := bridgePortObservationVLANKey(link.port)
		if portVLANKey == "" {
			portVLANKey = portKey
		}
		if _, isSwitchFacingPort := switchFacingPortKeys[portVLANKey]; isSwitchFacingPort {
			continue
		}

		scope := byPortScope[portVLANKey]
		if scope == nil {
			scope = &portScope{
				portKey:     portKey,
				portVLANKey: portVLANKey,
				port:        link.port,
				endpointIDs: make(map[string]struct{}),
			}
			byPortScope[portVLANKey] = scope
		}
		scope.endpointIDs[endpointID] = struct{}{}
	}

	if len(byPortScope) == 0 {
		return nil
	}

	candidatesByEndpoint := make(map[string]map[string]fdbEndpointOwner)
	scopeKeys := make([]string, 0, len(byPortScope))
	for key := range byPortScope {
		scopeKeys = append(scopeKeys, key)
	}
	sort.Strings(scopeKeys)

	for _, scopeKey := range scopeKeys {
		scope := byPortScope[scopeKey]
		if scope == nil || len(scope.endpointIDs) != 1 {
			continue
		}
		endpointID := ""
		for id := range scope.endpointIDs {
			endpointID = id
			break
		}
		if endpointID == "" {
			continue
		}
		candidates := candidatesByEndpoint[endpointID]
		if candidates == nil {
			candidates = make(map[string]fdbEndpointOwner)
			candidatesByEndpoint[endpointID] = candidates
		}
		candidates[scope.portVLANKey] = fdbEndpointOwner{
			portKey:     scope.portKey,
			portVLANKey: scope.portVLANKey,
			port:        scope.port,
			source:      "single_port_mac",
		}
	}

	if len(candidatesByEndpoint) == 0 {
		return nil
	}

	owners := make(map[string]fdbEndpointOwner)
	endpointIDs := make([]string, 0, len(candidatesByEndpoint))
	for endpointID := range candidatesByEndpoint {
		endpointIDs = append(endpointIDs, endpointID)
	}
	sort.Strings(endpointIDs)

	for _, endpointID := range endpointIDs {
		candidates := candidatesByEndpoint[endpointID]
		if len(candidates) != 1 {
			continue
		}
		for _, owner := range candidates {
			owners[endpointID] = owner
		}
	}

	if len(owners) == 0 {
		return nil
	}
	return owners
}

func endpointMatchOverlappingKnownDeviceIDs(
	endpointMatch topology.Match,
	deviceIdentityByID map[string]topologyIdentityKeySet,
) []string {
	if len(deviceIdentityByID) == 0 {
		return nil
	}

	endpointKeys := topologyMatchHardwareIdentityKeys(endpointMatch)
	if len(endpointKeys) == 0 {
		endpointKeys = topologyMatchIdentityKeys(endpointMatch)
	}
	if len(endpointKeys) == 0 {
		return nil
	}

	deviceIDs := make([]string, 0, len(deviceIdentityByID))
	for deviceID := range deviceIdentityByID {
		deviceID = strings.TrimSpace(deviceID)
		if deviceID == "" {
			continue
		}
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Strings(deviceIDs)
	if len(deviceIDs) == 0 {
		return nil
	}

	matches := make([]string, 0, 2)
	for _, deviceID := range deviceIDs {
		deviceKeys := deviceIdentityByID[deviceID]
		if len(deviceKeys) == 0 {
			continue
		}
		for _, endpointKey := range endpointKeys {
			if _, ok := deviceKeys[endpointKey]; ok {
				matches = append(matches, deviceID)
				break
			}
		}
	}
	if len(matches) == 0 {
		return nil
	}
	return matches
}

func segmentContainsDevice(segment *bridgeDomainSegment, deviceID string) bool {
	if segment == nil {
		return false
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return false
	}
	for _, port := range segment.ports {
		if strings.EqualFold(strings.TrimSpace(port.deviceID), deviceID) {
			return true
		}
	}
	return false
}

func topologyMetricString(metrics map[string]any, key string) string {
	if len(metrics) == 0 {
		return ""
	}
	value, ok := metrics[key]
	if !ok || value == nil {
		return ""
	}
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

func pruneSegmentArtifacts(actors []topology.Actor, links []topology.Link) ([]topology.Actor, []topology.Link, int) {
	if len(actors) == 0 || len(links) == 0 {
		return actors, links, 0
	}

	segmentKeys := make(map[string]struct{})
	segmentOrder := make([]string, 0)
	for _, actor := range actors {
		if !strings.EqualFold(strings.TrimSpace(actor.ActorType), "segment") {
			continue
		}
		key := canonicalTopologyMatchKey(actor.Match)
		if key == "" {
			continue
		}
		if _, seen := segmentKeys[key]; seen {
			continue
		}
		segmentKeys[key] = struct{}{}
		segmentOrder = append(segmentOrder, key)
	}
	if len(segmentKeys) == 0 {
		return actors, links, 0
	}
	sort.Strings(segmentOrder)

	lldpPairs := make(map[string]struct{})
	for _, link := range links {
		if !strings.EqualFold(strings.TrimSpace(link.Protocol), "lldp") {
			continue
		}
		src := canonicalTopologyMatchKey(link.Src.Match)
		dst := canonicalTopologyMatchKey(link.Dst.Match)
		if src == "" || dst == "" {
			continue
		}
		if _, srcSegment := segmentKeys[src]; srcSegment {
			continue
		}
		if _, dstSegment := segmentKeys[dst]; dstSegment {
			continue
		}
		if pair := topologyUndirectedPairKey(src, dst); pair != "" {
			lldpPairs[pair] = struct{}{}
		}
	}

	suppressed := make(map[string]struct{})
	for {
		changed := false
		neighborsBySegment := make(map[string]map[string]struct{})
		for _, link := range links {
			src := canonicalTopologyMatchKey(link.Src.Match)
			dst := canonicalTopologyMatchKey(link.Dst.Match)
			if src == "" || dst == "" {
				continue
			}
			if _, srcSuppressed := suppressed[src]; srcSuppressed {
				continue
			}
			if _, dstSuppressed := suppressed[dst]; dstSuppressed {
				continue
			}

			_, srcSegment := segmentKeys[src]
			_, dstSegment := segmentKeys[dst]

			if srcSegment && !dstSegment {
				neighbors := neighborsBySegment[src]
				if neighbors == nil {
					neighbors = make(map[string]struct{})
					neighborsBySegment[src] = neighbors
				}
				neighbors[dst] = struct{}{}
			}
			if dstSegment && !srcSegment {
				neighbors := neighborsBySegment[dst]
				if neighbors == nil {
					neighbors = make(map[string]struct{})
					neighborsBySegment[dst] = neighbors
				}
				neighbors[src] = struct{}{}
			}
		}

		for _, segmentKey := range segmentOrder {
			if _, alreadySuppressed := suppressed[segmentKey]; alreadySuppressed {
				continue
			}

			neighbors := neighborsBySegment[segmentKey]
			if len(neighbors) < 2 {
				suppressed[segmentKey] = struct{}{}
				changed = true
				continue
			}

			if len(neighbors) == 2 {
				pairValues := make([]string, 0, 2)
				for neighbor := range neighbors {
					pairValues = append(pairValues, neighbor)
				}
				if len(pairValues) == 2 {
					if pair := topologyUndirectedPairKey(pairValues[0], pairValues[1]); pair != "" {
						if _, found := lldpPairs[pair]; found {
							suppressed[segmentKey] = struct{}{}
							changed = true
						}
					}
				}
			}
		}

		if !changed {
			break
		}
	}

	if len(suppressed) == 0 {
		return actors, links, 0
	}

	filteredActors := make([]topology.Actor, 0, len(actors))
	for _, actor := range actors {
		key := canonicalTopologyMatchKey(actor.Match)
		if key == "" {
			filteredActors = append(filteredActors, actor)
			continue
		}
		if _, isSuppressed := suppressed[key]; isSuppressed && strings.EqualFold(strings.TrimSpace(actor.ActorType), "segment") {
			continue
		}
		filteredActors = append(filteredActors, actor)
	}

	filteredLinks := make([]topology.Link, 0, len(links))
	for _, link := range links {
		src := canonicalTopologyMatchKey(link.Src.Match)
		dst := canonicalTopologyMatchKey(link.Dst.Match)
		if src != "" {
			if _, srcSuppressed := suppressed[src]; srcSuppressed {
				continue
			}
		}
		if dst != "" {
			if _, dstSuppressed := suppressed[dst]; dstSuppressed {
				continue
			}
		}
		filteredLinks = append(filteredLinks, link)
	}

	return filteredActors, filteredLinks, len(suppressed)
}

func topologyUndirectedPairKey(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" {
		return ""
	}
	if left <= right {
		return left + "|" + right
	}
	return right + "|" + left
}

type topologyLinkCounts struct {
	lldp           int
	cdp            int
	fdb            int
	arp            int
	bidirectional  int
	unidirectional int
}

func summarizeTopologyLinks(links []topology.Link) topologyLinkCounts {
	var counts topologyLinkCounts
	for _, link := range links {
		switch strings.ToLower(strings.TrimSpace(link.Protocol)) {
		case "lldp":
			counts.lldp++
		case "cdp":
			counts.cdp++
		case "bridge", "fdb":
			counts.fdb++
		case "arp":
			counts.arp++
		}

		switch strings.ToLower(strings.TrimSpace(link.Direction)) {
		case "bidirectional":
			counts.bidirectional++
		case "unidirectional":
			counts.unidirectional++
		}
	}
	return counts
}

func pruneUnlinkedEndpointActors(actors []topology.Actor, links []topology.Link) []topology.Actor {
	if len(actors) == 0 {
		return actors
	}
	linkedByActorID := make(map[string]struct{}, len(links)*2)
	for _, link := range links {
		if srcActorID := strings.TrimSpace(link.SrcActorID); srcActorID != "" {
			linkedByActorID[srcActorID] = struct{}{}
		}
		if dstActorID := strings.TrimSpace(link.DstActorID); dstActorID != "" {
			linkedByActorID[dstActorID] = struct{}{}
		}
	}
	filtered := make([]topology.Actor, 0, len(actors))
	for _, actor := range actors {
		if !strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
			filtered = append(filtered, actor)
			continue
		}
		actorID := strings.TrimSpace(actor.ActorID)
		if actorID == "" {
			continue
		}
		if _, ok := linkedByActorID[actorID]; !ok {
			continue
		}
		filtered = append(filtered, actor)
	}
	return filtered
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

func buildTopologyDeviceInterfaceSummaries(
	interfaces []Interface,
	attachments []Attachment,
	adjacencies []Adjacency,
	ifIndexByDeviceName map[string]int,
	bridgeLinks []bridgeBridgeLinkRecord,
	reporterAliases map[string][]string,
) map[string]topologyDeviceInterfaceSummary {
	if len(interfaces) == 0 {
		return nil
	}

	type interfaceCollector struct {
		ifIndexes    map[string]struct{}
		ifNames      map[string]struct{}
		ifTypes      map[string]int
		adminCounts  map[string]int
		operCounts   map[string]int
		portStatuses []topologyDevicePortStatus
		portEvidence map[int]*topologyDevicePortEvidence
	}

	collectors := make(map[string]*interfaceCollector)
	for _, iface := range interfaces {
		deviceID := strings.TrimSpace(iface.DeviceID)
		if deviceID == "" || iface.IfIndex <= 0 {
			continue
		}
		col := collectors[deviceID]
		if col == nil {
			col = &interfaceCollector{
				ifIndexes:    make(map[string]struct{}),
				ifNames:      make(map[string]struct{}),
				ifTypes:      make(map[string]int),
				adminCounts:  make(map[string]int),
				operCounts:   make(map[string]int),
				portEvidence: make(map[int]*topologyDevicePortEvidence),
			}
			collectors[deviceID] = col
		}

		ifIndex := strconv.Itoa(iface.IfIndex)
		col.ifIndexes[ifIndex] = struct{}{}
		if ifName := strings.TrimSpace(iface.IfName); ifName != "" {
			col.ifNames[ifName] = struct{}{}
		}

		admin := strings.TrimSpace(iface.Labels["admin_status"])
		oper := strings.TrimSpace(iface.Labels["oper_status"])
		ifType := strings.TrimSpace(iface.Labels["if_type"])
		if ifType != "" {
			col.ifTypes[ifType]++
		}
		if admin != "" {
			col.adminCounts[admin]++
		}
		if oper != "" {
			col.operCounts[oper]++
		}

		col.portStatuses = append(col.portStatuses, topologyDevicePortStatus{
			IfIndex:        iface.IfIndex,
			IfName:         strings.TrimSpace(iface.IfName),
			InterfaceType:  ifType,
			AdminStatus:    admin,
			OperStatus:     oper,
			LinkMode:       "unknown",
			ModeConfidence: "low",
			TopologyRole:   "unknown",
			RoleConfidence: "low",
		})
	}

	managedAliasOwners := buildFDBAliasOwnerMap(reporterAliases)
	for _, attachment := range attachments {
		deviceID := strings.TrimSpace(attachment.DeviceID)
		if deviceID == "" || attachment.IfIndex <= 0 {
			continue
		}
		col := collectors[deviceID]
		if col == nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(attachment.Method), "fdb") {
			continue
		}
		fdbStatus := strings.ToLower(strings.TrimSpace(attachment.Labels["fdb_status"]))
		if fdbStatus == "ignored" {
			continue
		}

		evidence := ensureTopologyPortEvidence(col.portEvidence, attachment.IfIndex)
		evidence.hasFDB = true
		endpointID := normalizeFDBEndpointID(attachment.EndpointID)
		if endpointID == "" {
			endpointID = strings.TrimSpace(attachment.EndpointID)
		}
		if endpointID != "" {
			evidence.fdbEndpointIDs[endpointID] = struct{}{}
			if aliasOwners, ok := managedAliasOwners[endpointID]; ok {
				for aliasOwnerID := range aliasOwners {
					if !strings.EqualFold(strings.TrimSpace(aliasOwnerID), deviceID) {
						evidence.hasFDBManagedAlias = true
						break
					}
				}
			}
		}
		vlanID := normalizeTopologyVLANID(firstNonEmpty(attachment.Labels["vlan_id"], attachment.Labels["vlan"]))
		if vlanID != "" {
			evidence.vlanIDs[vlanID] = struct{}{}
		}
	}

	for _, adj := range adjacencies {
		deviceID := strings.TrimSpace(adj.SourceID)
		if deviceID == "" {
			continue
		}
		col := collectors[deviceID]
		if col == nil {
			continue
		}

		ifIndex := resolveAdjacencySourceIfIndex(adj, ifIndexByDeviceName)
		if ifIndex <= 0 {
			continue
		}
		evidence := ensureTopologyPortEvidence(col.portEvidence, ifIndex)
		switch strings.ToLower(strings.TrimSpace(adj.Protocol)) {
		case "stp":
			evidence.hasSTP = true
			vlanID := normalizeTopologyVLANID(firstNonEmpty(adj.Labels["vlan_id"], adj.Labels["vlan"]))
			if vlanID != "" {
				evidence.vlanIDs[vlanID] = struct{}{}
			}
		case "lldp", "cdp":
			evidence.hasPeer = true
		}
	}

	for _, link := range bridgeLinks {
		for _, port := range []bridgePortRef{link.designatedPort, link.port} {
			deviceID := strings.TrimSpace(port.deviceID)
			if deviceID == "" || port.ifIndex <= 0 {
				continue
			}
			col := collectors[deviceID]
			if col == nil {
				continue
			}
			evidence := ensureTopologyPortEvidence(col.portEvidence, port.ifIndex)
			evidence.hasBridgeLink = true
		}
	}

	if len(collectors) == 0 {
		return nil
	}

	out := make(map[string]topologyDeviceInterfaceSummary, len(collectors))
	for deviceID, col := range collectors {
		sort.Slice(col.portStatuses, func(i, j int) bool {
			left, right := col.portStatuses[i], col.portStatuses[j]
			if left.IfIndex != right.IfIndex {
				return left.IfIndex < right.IfIndex
			}
			return left.IfName < right.IfName
		})

		modeCounts := make(map[string]int)
		roleCounts := make(map[string]int)
		portStatuses := make([]map[string]any, 0, len(col.portStatuses))
		for _, st := range col.portStatuses {
			evidence := col.portEvidence[st.IfIndex]
			mode, confidence, sources, vlans := classifyTopologyPortLinkMode(evidence)
			role, roleConfidence, roleSources := classifyTopologyPortRole(evidence)
			st.LinkMode = mode
			st.ModeConfidence = confidence
			st.ModeSources = sources
			st.VLANIDs = vlans
			st.TopologyRole = role
			st.RoleConfidence = roleConfidence
			st.RoleSources = roleSources
			modeCounts[mode]++
			roleCounts[role]++

			portStatus := map[string]any{
				"if_index":                 st.IfIndex,
				"if_name":                  strings.TrimSpace(st.IfName),
				"link_mode":                st.LinkMode,
				"link_mode_confidence":     st.ModeConfidence,
				"topology_role":            st.TopologyRole,
				"topology_role_confidence": st.RoleConfidence,
			}
			if len(st.ModeSources) > 0 {
				portStatus["link_mode_sources"] = st.ModeSources
			}
			if len(st.RoleSources) > 0 {
				portStatus["topology_role_sources"] = st.RoleSources
			}
			if len(st.VLANIDs) > 0 {
				portStatus["vlan_ids"] = st.VLANIDs
			}
			if st.AdminStatus != "" {
				portStatus["admin_status"] = st.AdminStatus
			}
			if st.OperStatus != "" {
				portStatus["oper_status"] = st.OperStatus
			}
			if st.InterfaceType != "" {
				portStatus["if_type"] = st.InterfaceType
			}
			portStatuses = append(portStatuses, pruneTopologyAttributes(portStatus))
		}

		out[deviceID] = topologyDeviceInterfaceSummary{
			portsTotal:       len(col.ifIndexes),
			ifIndexes:        sortedTopologySet(col.ifIndexes),
			ifNames:          sortedTopologySet(col.ifNames),
			adminStatusCount: intCountMapToAny(col.adminCounts),
			operStatusCount:  intCountMapToAny(col.operCounts),
			linkModeCount:    intCountMapToAny(modeCounts),
			roleCount:        intCountMapToAny(roleCounts),
			portStatuses:     portStatuses,
		}
	}
	return out
}

func normalizeTopologyVLANID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.ToLower(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func ensureTopologyPortEvidence(
	evidenceByIfIndex map[int]*topologyDevicePortEvidence,
	ifIndex int,
) *topologyDevicePortEvidence {
	if ifIndex <= 0 {
		return nil
	}
	evidence := evidenceByIfIndex[ifIndex]
	if evidence == nil {
		evidence = &topologyDevicePortEvidence{
			vlanIDs:        make(map[string]struct{}),
			fdbEndpointIDs: make(map[string]struct{}),
		}
		evidenceByIfIndex[ifIndex] = evidence
	}
	return evidence
}

func resolveAdjacencySourceIfIndex(adj Adjacency, ifIndexByDeviceName map[string]int) int {
	ifIndex := 0
	if ifName := strings.TrimSpace(adj.SourcePort); ifName != "" {
		if idx, ok := ifIndexByDeviceName[deviceIfNameKey(adj.SourceID, ifName)]; ok {
			ifIndex = idx
		} else if parsed, err := strconv.Atoi(ifName); err == nil && parsed > 0 {
			ifIndex = parsed
		}
	}
	return ifIndex
}

func classifyTopologyPortLinkMode(evidence *topologyDevicePortEvidence) (mode string, confidence string, sources []string, vlans []string) {
	mode = "unknown"
	confidence = "low"
	if evidence == nil {
		return mode, confidence, nil, nil
	}

	if len(evidence.vlanIDs) > 0 {
		vlans = sortedTopologySet(evidence.vlanIDs)
	}
	if evidence.hasFDB {
		sources = append(sources, "fdb")
	}
	if evidence.hasSTP {
		sources = append(sources, "stp")
	}
	if evidence.hasPeer {
		sources = append(sources, "peer_link")
	}

	switch vlanCount := len(evidence.vlanIDs); {
	case vlanCount >= 2:
		mode = "trunk"
		if evidence.hasFDB && evidence.hasSTP {
			confidence = "high"
		} else {
			confidence = "medium"
		}
	case vlanCount == 1 && !evidence.hasPeer:
		mode = "access"
		confidence = "medium"
	default:
		mode = "unknown"
		confidence = "low"
	}
	return mode, confidence, sources, vlans
}

func classifyTopologyPortRole(evidence *topologyDevicePortEvidence) (role string, confidence string, sources []string) {
	role = "unknown"
	confidence = "low"
	if evidence == nil {
		return role, confidence, nil
	}

	if evidence.hasPeer {
		sources = append(sources, "peer_link")
	}
	if evidence.hasBridgeLink {
		sources = append(sources, "bridge_link")
	}
	if evidence.hasSTP {
		sources = append(sources, "stp")
	}
	if evidence.hasFDB {
		sources = append(sources, "fdb")
	}
	if evidence.hasFDBManagedAlias {
		sources = append(sources, "fdb_managed_alias")
	}

	switch {
	case evidence.hasPeer || evidence.hasBridgeLink:
		role = "switch_facing"
		confidence = "high"
	case evidence.hasSTP && evidence.hasFDBManagedAlias:
		// STP alone is not sufficient to mark switch-facing; require corroborating
		// managed-alias FDB evidence on the same port.
		role = "switch_facing"
		confidence = "medium"
	case evidence.hasFDB && len(evidence.fdbEndpointIDs) == 1 && !evidence.hasSTP:
		role = "host_facing"
		confidence = "medium"
	case evidence.hasFDB && !evidence.hasSTP:
		role = "host_candidate"
		confidence = "low"
	default:
		role = "unknown"
		confidence = "low"
	}
	return role, confidence, sources
}

func intCountMapToAny(in map[string]int) map[string]any {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		keys = append(keys, key)
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		out[key] = in[key]
	}
	return out
}

func topologyDeviceInferred(dev Device) bool {
	if len(dev.Labels) == 0 {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(dev.Labels["inferred"])) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func deviceToTopologyActor(
	dev Device,
	source, layer, localDeviceID string,
	ifaceSummary topologyDeviceInterfaceSummary,
	reporterAliases []string,
) topology.Actor {
	match := topology.Match{
		SysObjectID: strings.TrimSpace(dev.SysObject),
		SysName:     strings.TrimSpace(dev.Hostname),
	}

	macSet := make(map[string]struct{}, 1+len(reporterAliases))
	chassis := strings.TrimSpace(dev.ChassisID)
	if chassis != "" {
		match.ChassisIDs = []string{chassis}
		if mac := normalizeMAC(chassis); mac != "" {
			macSet[mac] = struct{}{}
		}
	}
	for _, alias := range reporterAliases {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if strings.HasPrefix(alias, "mac:") {
			if mac := normalizeMAC(strings.TrimPrefix(alias, "mac:")); mac != "" {
				macSet[mac] = struct{}{}
			}
			continue
		}
		if mac := normalizeMAC(alias); mac != "" {
			macSet[mac] = struct{}{}
		}
	}
	if len(macSet) > 0 {
		match.MacAddresses = sortedTopologySet(macSet)
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
		"inferred":               topologyDeviceInferred(dev),
		"management_ip":          firstAddress(dev.Addresses),
		"management_addresses":   addressStrings(dev.Addresses),
		"protocols":              labelsCSVToSlice(dev.Labels, "protocols_observed"),
		"protocols_collected":    labelsCSVToSlice(dev.Labels, "protocols_observed"),
		"capabilities":           labelsCSVToSlice(dev.Labels, "capabilities"),
		"capabilities_supported": labelsCSVToSlice(dev.Labels, "capabilities_supported"),
		"capabilities_enabled":   labelsCSVToSlice(dev.Labels, "capabilities_enabled"),
	}
	if ifaceSummary.portsTotal > 0 {
		attrs["ports_total"] = ifaceSummary.portsTotal
	}
	if len(ifaceSummary.ifIndexes) > 0 {
		attrs["if_indexes"] = ifaceSummary.ifIndexes
	}
	if len(ifaceSummary.ifNames) > 0 {
		attrs["if_names"] = ifaceSummary.ifNames
	}
	if len(ifaceSummary.adminStatusCount) > 0 {
		attrs["if_admin_status_counts"] = ifaceSummary.adminStatusCount
	}
	if len(ifaceSummary.operStatusCount) > 0 {
		attrs["if_oper_status_counts"] = ifaceSummary.operStatusCount
	}
	if len(ifaceSummary.linkModeCount) > 0 {
		attrs["if_link_mode_counts"] = ifaceSummary.linkModeCount
	}
	if len(ifaceSummary.roleCount) > 0 {
		attrs["if_topology_role_counts"] = ifaceSummary.roleCount
	}
	if len(ifaceSummary.portStatuses) > 0 {
		attrs["if_statuses"] = ifaceSummary.portStatuses
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

func adjacencySideToEndpoint(dev Device, port string, ifIndexByDeviceName map[string]int, ifaceByDeviceIndex map[string]Interface) topology.LinkEndpoint {
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
	if ifIndex > 0 {
		if iface, ok := ifaceByDeviceIndex[deviceIfIndexKey(dev.ID, ifIndex)]; ok {
			if admin := strings.TrimSpace(iface.Labels["admin_status"]); admin != "" {
				attrs["if_admin_status"] = admin
			}
			if oper := strings.TrimSpace(iface.Labels["oper_status"]); oper != "" {
				attrs["if_oper_status"] = oper
			}
		}
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
	actorMACIndex map[string]struct{},
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
		macKeys := topologyMatchHardwareIdentityKeys(actor.Match)
		if len(macKeys) > 0 {
			if topologyIdentityIndexOverlaps(actorMACIndex, macKeys) {
				continue
			}
			addTopologyIdentityKeys(actorMACIndex, macKeys)
		} else if topologyIdentityIndexOverlaps(actorIndex, keys) {
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

func topologyMatchHardwareIdentityKeys(match topology.Match) []string {
	seen := make(map[string]struct{}, len(match.MacAddresses)+len(match.ChassisIDs))
	add := func(value string) {
		if mac := normalizeMAC(value); mac != "" {
			seen["hw:"+mac] = struct{}{}
		}
	}

	for _, value := range match.MacAddresses {
		add(value)
	}
	for _, value := range match.ChassisIDs {
		add(value)
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
	if key := canonicalTopologyPrimaryMACKey(match); key != "" {
		return "mac:" + key
	}
	if key := canonicalTopologyHardwareKey(match.ChassisIDs); key != "" {
		return "chassis:" + key
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

func assignTopologyActorIDsAndLinkEndpoints(actors []topology.Actor, links []topology.Link) {
	if len(actors) == 0 {
		return
	}

	usedActorIDs := make(map[string]int, len(actors))
	actorIDByCanonicalMatch := make(map[string]string, len(actors))
	actorIDByIdentityKey := make(map[string]string, len(actors)*4)

	for i := range actors {
		baseID := canonicalTopologyMatchKey(actors[i].Match)
		if baseID == "" {
			actorType := strings.ToLower(strings.TrimSpace(actors[i].ActorType))
			if actorType == "" {
				actorType = "actor"
			}
			baseID = "generated:" + actorType
		}

		actorID := responseScopedActorID(baseID, usedActorIDs)
		actors[i].ActorID = actorID

		if canonical := canonicalTopologyMatchKey(actors[i].Match); canonical != "" {
			if _, exists := actorIDByCanonicalMatch[canonical]; !exists {
				actorIDByCanonicalMatch[canonical] = actorID
			}
		}
		for _, key := range topologyMatchIdentityKeys(actors[i].Match) {
			if _, exists := actorIDByIdentityKey[key]; !exists {
				actorIDByIdentityKey[key] = actorID
			}
		}
	}

	for i := range links {
		links[i].SrcActorID = resolveTopologyEndpointActorID(links[i].Src.Match, actorIDByCanonicalMatch, actorIDByIdentityKey)
		links[i].DstActorID = resolveTopologyEndpointActorID(links[i].Dst.Match, actorIDByCanonicalMatch, actorIDByIdentityKey)
	}
}

func responseScopedActorID(base string, used map[string]int) string {
	base = strings.ToLower(strings.TrimSpace(base))
	if base == "" {
		base = "generated:actor"
	}

	count := used[base]
	count++
	used[base] = count
	if count == 1 {
		return base
	}
	return fmt.Sprintf("%s#%d", base, count)
}

func resolveTopologyEndpointActorID(match topology.Match, byCanonicalMatch map[string]string, byIdentityKey map[string]string) string {
	if canonical := canonicalTopologyMatchKey(match); canonical != "" {
		if actorID := strings.TrimSpace(byCanonicalMatch[canonical]); actorID != "" {
			return actorID
		}
	}
	for _, key := range topologyMatchIdentityKeys(match) {
		if actorID := strings.TrimSpace(byIdentityKey[key]); actorID != "" {
			return actorID
		}
	}
	return ""
}

func canonicalTopologyPrimaryMACKey(match topology.Match) string {
	set := make(map[string]struct{}, len(match.MacAddresses)+len(match.ChassisIDs))
	for _, value := range match.MacAddresses {
		if mac := normalizeMAC(value); mac != "" {
			set[mac] = struct{}{}
		}
	}
	for _, value := range match.ChassisIDs {
		if mac := normalizeMAC(value); mac != "" {
			set[mac] = struct{}{}
		}
	}
	if len(set) == 0 {
		return ""
	}
	keys := sortedTopologySet(set)
	if len(keys) == 0 {
		return ""
	}
	return strings.Join(keys, ",")
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

type topologyDisplayNameResolver struct {
	lookup func(ip string) string
	cache  map[string]string
}

type topologyDisplayName struct {
	name   string
	source string
}

func applyTopologyDisplayNames(actors []topology.Actor, links []topology.Link, lookup func(ip string) string) {
	resolver := topologyDisplayNameResolver{
		lookup: lookup,
		cache:  make(map[string]string),
	}

	deviceDisplayByID := make(map[string]string, len(actors))
	displayByMatchKey := make(map[string]string, len(actors))

	// First pass: materialize display names for non-segment actors so segment naming can reuse them.
	for i := range actors {
		if actors[i].ActorType == "segment" {
			continue
		}
		display := topologyActorDisplayName(actors[i], nil, &resolver)
		if display.name == "" {
			display = topologyFallbackActorDisplayName(actors[i])
		}
		topologySetActorDisplay(&actors[i], display)
		if matchKey := canonicalTopologyMatchKey(actors[i].Match); matchKey != "" {
			displayByMatchKey[matchKey] = display.name
		}
		if actors[i].ActorType == "device" {
			if deviceID := topologyActorDeviceID(actors[i]); deviceID != "" {
				deviceDisplayByID[deviceID] = display.name
			}
		}
	}

	// Second pass: segment display names depend on finalized device display names.
	for i := range actors {
		if actors[i].ActorType != "segment" {
			continue
		}
		display := topologyActorDisplayName(actors[i], deviceDisplayByID, &resolver)
		if display.name == "" {
			display = topologyFallbackActorDisplayName(actors[i])
		}
		topologySetActorDisplay(&actors[i], display)
		if matchKey := canonicalTopologyMatchKey(actors[i].Match); matchKey != "" {
			displayByMatchKey[matchKey] = display.name
		}
	}

	for i := range links {
		src := topologyEndpointDisplayName(links[i].Src, displayByMatchKey, &resolver)
		if src.name == "" {
			src = topologyDisplayName{name: "[unset]", source: "fallback"}
		}
		topologySetEndpointDisplay(&links[i].Src, src)
		srcPortName := topologySetEndpointCanonicalPortName(&links[i].Src)

		dst := topologyEndpointDisplayName(links[i].Dst, displayByMatchKey, &resolver)
		if dst.name == "" {
			dst = topologyDisplayName{name: "[unset]", source: "fallback"}
		}
		topologySetEndpointDisplay(&links[i].Dst, dst)
		dstPortName := topologySetEndpointCanonicalPortName(&links[i].Dst)

		linkName := topologyCanonicalLinkName(src.name, srcPortName, dst.name, dstPortName)
		if links[i].Metrics == nil {
			links[i].Metrics = make(map[string]any)
		}
		links[i].Metrics["display_name"] = linkName
		links[i].Metrics["src_port_name"] = srcPortName
		links[i].Metrics["dst_port_name"] = dstPortName
	}
}

func topologySetActorDisplay(actor *topology.Actor, display topologyDisplayName) {
	if actor == nil {
		return
	}
	labels := cloneStringMap(actor.Labels)
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["display_name"] = display.name
	if display.source != "" {
		labels["display_source"] = display.source
	}
	actor.Labels = labels

	attrs := cloneAnyMap(actor.Attributes)
	if attrs == nil {
		attrs = make(map[string]any)
	}
	attrs["display_name"] = display.name
	if display.source != "" {
		attrs["display_source"] = display.source
	}
	actor.Attributes = pruneTopologyAttributes(attrs)
}

func topologySetEndpointDisplay(endpoint *topology.LinkEndpoint, display topologyDisplayName) {
	if endpoint == nil {
		return
	}
	attrs := cloneAnyMap(endpoint.Attributes)
	if attrs == nil {
		attrs = make(map[string]any)
	}
	attrs["display_name"] = display.name
	if display.source != "" {
		attrs["display_source"] = display.source
	}
	endpoint.Attributes = pruneTopologyAttributes(attrs)
}

func topologySetEndpointCanonicalPortName(endpoint *topology.LinkEndpoint) string {
	if endpoint == nil {
		return "0"
	}
	attrs := cloneAnyMap(endpoint.Attributes)
	if attrs == nil {
		attrs = make(map[string]any)
	}
	name := topologyCanonicalPortName(attrs)
	attrs["port_name"] = name
	endpoint.Attributes = pruneTopologyAttributes(attrs)
	return name
}

func topologyEndpointDisplayName(endpoint topology.LinkEndpoint, actorDisplayByMatch map[string]string, resolver *topologyDisplayNameResolver) topologyDisplayName {
	if key := canonicalTopologyMatchKey(endpoint.Match); key != "" {
		if name := strings.TrimSpace(actorDisplayByMatch[key]); name != "" {
			return topologyDisplayName{name: name, source: "actor"}
		}
	}
	return topologyDisplayNameFromMatch(endpoint.Match, resolver)
}

func topologyActorDisplayName(actor topology.Actor, deviceDisplayByID map[string]string, resolver *topologyDisplayNameResolver) topologyDisplayName {
	if actor.ActorType == "segment" {
		if name := topologySegmentDisplayName(actor, deviceDisplayByID); name != "" {
			return topologyDisplayName{name: name, source: "segment"}
		}
	}

	display := topologyDisplayNameFromMatch(actor.Match, resolver)
	if display.name != "" {
		return display
	}

	if segmentID := topologyAttrString(actor.Attributes, "segment_id"); segmentID != "" {
		return topologyDisplayName{name: topologyCompactSegmentID(segmentID), source: "segment_id"}
	}
	return topologyDisplayName{}
}

func topologyFallbackActorDisplayName(actor topology.Actor) topologyDisplayName {
	if matchKey := canonicalTopologyMatchKey(actor.Match); matchKey != "" {
		return topologyDisplayName{name: matchKey, source: "fallback_match"}
	}
	if segmentID := topologyAttrString(actor.Attributes, "segment_id"); segmentID != "" {
		return topologyDisplayName{name: topologyCompactSegmentID(segmentID), source: "segment_id"}
	}
	actorType := strings.TrimSpace(actor.ActorType)
	if actorType == "" {
		actorType = "actor"
	}
	return topologyDisplayName{name: actorType + ":[unset]", source: "fallback"}
}

func topologyActorDeviceID(actor topology.Actor) string {
	return topologyAttrString(actor.Attributes, "device_id")
}

func topologyDisplayNameFromMatch(match topology.Match, resolver *topologyDisplayNameResolver) topologyDisplayName {
	if dns := topologyMatchPreferredDNSName(match, resolver); dns != "" {
		return topologyDisplayName{name: dns, source: "dns"}
	}
	if sysName := topologyMatchPreferredSysName(match); sysName != "" {
		return topologyDisplayName{name: sysName, source: "sys_name"}
	}
	if hostname := topologyMatchPreferredHostname(match); hostname != "" {
		return topologyDisplayName{name: hostname, source: "hostname"}
	}
	if ip := topologyMatchPreferredIP(match); ip != "" {
		return topologyDisplayName{name: ip, source: "ip"}
	}
	if mac := topologyMatchPreferredMAC(match); mac != "" {
		return topologyDisplayName{name: mac, source: "mac"}
	}
	return topologyDisplayName{}
}

func topologyMatchPreferredDNSName(match topology.Match, resolver *topologyDisplayNameResolver) string {
	candidates := make(map[string]struct{})
	for _, value := range match.DNSNames {
		if normalized := normalizeDNSName(value); normalized != "" {
			candidates[normalized] = struct{}{}
		}
	}
	for _, value := range match.IPAddresses {
		if resolver == nil {
			continue
		}
		if ip := normalizeTopologyIP(value); ip != "" {
			if resolved := resolver.resolve(ip); resolved != "" {
				candidates[resolved] = struct{}{}
			}
		}
	}
	names := sortedTopologySet(candidates)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func topologyMatchPreferredSysName(match topology.Match) string {
	return strings.TrimSpace(match.SysName)
}

func topologyMatchPreferredHostname(match topology.Match) string {
	hostnames := uniqueTopologyStrings(match.Hostnames)
	if len(hostnames) == 0 {
		return ""
	}
	return hostnames[0]
}

func topologyMatchPreferredIP(match topology.Match) string {
	ips := make([]string, 0, len(match.IPAddresses))
	for _, value := range match.IPAddresses {
		if ip := normalizeTopologyIP(value); ip != "" {
			ips = append(ips, ip)
		}
	}
	ips = uniqueTopologyStrings(ips)
	if len(ips) == 0 {
		return ""
	}
	return ips[0]
}

func topologyMatchPreferredMAC(match topology.Match) string {
	macs := make([]string, 0, len(match.MacAddresses)+len(match.ChassisIDs))
	for _, value := range match.MacAddresses {
		if mac := normalizeMAC(value); mac != "" {
			macs = append(macs, mac)
		}
	}
	for _, value := range match.ChassisIDs {
		if mac := normalizeMAC(value); mac != "" {
			macs = append(macs, mac)
		}
	}
	macs = uniqueTopologyStrings(macs)
	if len(macs) == 0 {
		return ""
	}
	return macs[0]
}

func topologyCanonicalPortName(attrs map[string]any) string {
	if name := topologyAttrString(attrs, "port_name"); name != "" {
		return name
	}
	if name := topologyAttrString(attrs, "if_name"); name != "" {
		return name
	}
	if name := topologyAttrString(attrs, "if_descr"); name != "" {
		return name
	}
	if name := topologyAttrString(attrs, "if_alias"); name != "" {
		return name
	}

	if ifIndex := topologyAttrInt(attrs, "if_index"); ifIndex > 0 {
		return strconv.Itoa(ifIndex)
	}

	if portID := topologyAttrString(attrs, "port_id"); portID != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(portID)); err == nil && n > 0 {
			return strconv.Itoa(n)
		}
		return portID
	}
	if bridgePort := topologyAttrString(attrs, "bridge_port"); bridgePort != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(bridgePort)); err == nil && n > 0 {
			return strconv.Itoa(n)
		}
		return bridgePort
	}
	return "0"
}

func topologyCanonicalLinkName(srcName, srcPortName, dstName, dstPortName string) string {
	srcName = strings.TrimSpace(srcName)
	if srcName == "" {
		srcName = "[unset]"
	}
	dstName = strings.TrimSpace(dstName)
	if dstName == "" {
		dstName = "[unset]"
	}
	srcPortName = strings.TrimSpace(srcPortName)
	if srcPortName == "" {
		srcPortName = "0"
	}
	dstPortName = strings.TrimSpace(dstPortName)
	if dstPortName == "" {
		dstPortName = "0"
	}
	return srcName + ":" + srcPortName + " -> " + dstName + ":" + dstPortName
}

func normalizeDNSName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".")
	if name == "" {
		return ""
	}
	return strings.ToLower(name)
}

func (r *topologyDisplayNameResolver) resolve(ip string) string {
	if r == nil || r.lookup == nil {
		return ""
	}
	ip = normalizeTopologyIP(ip)
	if ip == "" {
		return ""
	}
	if name, ok := r.cache[ip]; ok {
		return name
	}
	name := normalizeDNSName(r.lookup(ip))
	r.cache[ip] = name
	return name
}

type topologySegmentPortRef struct {
	deviceID   string
	ifName     string
	ifIndex    string
	bridgePort string
}

func topologySegmentDisplayName(actor topology.Actor, deviceDisplayByID map[string]string) string {
	attrs := actor.Attributes
	if len(attrs) == 0 {
		return ""
	}

	ref := parseTopologySegmentPortRef(topologyAttrString(attrs, "designated_port"))
	parent := topologySegmentParentDisplayName(ref.deviceID, deviceDisplayByID)
	port := topologySegmentPortDisplay(ref)
	if parent != "" && port != "" {
		return parent + "." + port + ".segment"
	}

	parentCandidates := make(map[string]struct{})
	for _, candidate := range topologyAttrStringSlice(attrs, "parent_devices") {
		if value := topologySegmentParentDisplayName(candidate, deviceDisplayByID); value != "" {
			parentCandidates[value] = struct{}{}
		}
	}
	portCandidates := make(map[string]struct{})
	for _, candidate := range topologyAttrStringSlice(attrs, "if_names") {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			portCandidates[candidate] = struct{}{}
		}
	}
	if len(portCandidates) == 0 {
		for _, candidate := range topologyAttrStringSlice(attrs, "bridge_ports") {
			if candidate = strings.TrimSpace(candidate); candidate != "" {
				portCandidates[candidate] = struct{}{}
			}
		}
	}
	parents := sortedTopologySet(parentCandidates)
	ports := sortedTopologySet(portCandidates)
	if len(parents) > 0 && len(ports) > 0 {
		return parents[0] + "." + ports[0] + ".segment"
	}

	return topologyCompactSegmentID(topologyAttrString(attrs, "segment_id"))
}

func parseTopologySegmentPortRef(raw string) topologySegmentPortRef {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return topologySegmentPortRef{}
	}
	parts := strings.Split(raw, "|")
	if len(parts) == 0 {
		return topologySegmentPortRef{}
	}
	ref := topologySegmentPortRef{
		deviceID: strings.TrimSpace(parts[0]),
	}
	for _, part := range parts[1:] {
		switch {
		case strings.HasPrefix(part, "name:"):
			ref.ifName = strings.TrimSpace(strings.TrimPrefix(part, "name:"))
		case strings.HasPrefix(part, "if:"):
			ref.ifIndex = strings.TrimSpace(strings.TrimPrefix(part, "if:"))
		case strings.HasPrefix(part, "bp:"):
			ref.bridgePort = strings.TrimSpace(strings.TrimPrefix(part, "bp:"))
		}
	}
	return ref
}

func topologySegmentPortDisplay(ref topologySegmentPortRef) string {
	if name := strings.TrimSpace(ref.ifName); name != "" {
		return name
	}
	if name := strings.TrimSpace(ref.bridgePort); name != "" {
		return name
	}
	if index := strings.TrimSpace(ref.ifIndex); index != "" && index != "0" {
		return index
	}
	return ""
}

func topologySegmentParentDisplayName(raw string, deviceDisplayByID map[string]string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if deviceDisplayByID != nil {
		if display := strings.TrimSpace(deviceDisplayByID[raw]); display != "" {
			return display
		}
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "management_ip:"):
		return strings.TrimSpace(raw[len("management_ip:"):])
	case strings.HasPrefix(lower, "macaddress:"):
		if mac := normalizeMAC(raw[len("macAddress:"):]); mac != "" {
			return mac
		}
		return strings.TrimSpace(raw[len("macAddress:"):])
	}
	return raw
}

func topologyCompactSegmentID(segmentID string) string {
	segmentID = strings.TrimSpace(segmentID)
	if segmentID == "" {
		return ""
	}
	const max = 48
	if len(segmentID) <= max {
		return segmentID
	}
	return segmentID[:max] + "..."
}

func topologyAttrString(attrs map[string]any, key string) string {
	if len(attrs) == 0 {
		return ""
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return ""
	}
	str, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(str)
}

func topologyAttrInt(attrs map[string]any, key string) int {
	if len(attrs) == 0 {
		return 0
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		if typed < 0 {
			return 0
		}
		if typed > math.MaxInt {
			return math.MaxInt
		}
		return int(typed)
	case float64:
		if typed <= 0 {
			return 0
		}
		return int(typed)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil || parsed <= 0 {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

func topologyAttrStringSlice(attrs map[string]any, key string) []string {
	if len(attrs) == 0 {
		return nil
	}
	value, ok := attrs[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			str, ok := item.(string)
			if !ok {
				continue
			}
			if str = strings.TrimSpace(str); str != "" {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

func topologyTimePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	out := t
	return &out
}
