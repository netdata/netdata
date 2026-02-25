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
	SchemaVersion             string
	Source                    string
	Layer                     string
	View                      string
	AgentID                   string
	LocalDeviceID             string
	CollectedAt               time.Time
	ResolveDNSName            func(ip string) string
	CollapseActorsByIP        bool
	EliminateNonIPInferred    bool
	ProbabilisticConnectivity bool
	InferenceStrategy         string
}

const (
	topologyInferenceStrategyFDBMinimumKnowledge = "fdb_minimum_knowledge"
	topologyInferenceStrategySTPParentTree       = "stp_parent_tree"
	topologyInferenceStrategyFDBPairwise         = "fdb_pairwise_minimum_knowledge"
	topologyInferenceStrategySTPFDBCorrelated    = "stp_fdb_correlated"
	topologyInferenceStrategyCDPFDBHybrid        = "cdp_fdb_hybrid"
	topologyInferenceStrategyFDBOverlapWeighted  = "fdb_overlap_weighted"
	topologyInferenceStrategyExperimentalFull    = "experimental_full"
)

type topologyInferenceStrategyConfig struct {
	id                               string
	includeLLDPBridgeLinks           bool
	includeCDPBridgeLinks            bool
	includeSTPBridgeLinks            bool
	useSTPDesignatedParent           bool
	enableFDBPairwiseLinks           bool
	enableFDBOverlapLinks            bool
	fdbOverlapMinShared              int
	enableSTPManagedAliasCorrelation bool
	filterSwitchFacingAttachments    bool
}

type endpointActorAccumulator struct {
	endpointID string
	mac        string
	ips        map[string]netip.Addr
	sources    map[string]struct{}
	deviceIDs  map[string]struct{}
	ifIndexes  map[string]struct{}
	ifNames    map[string]struct{}
}

type builtAdjacencyLink struct {
	adj      Adjacency
	protocol string
	link     topology.Link
}

type pairedLinkAccumulator struct {
	all []*builtAdjacencyLink
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
	suppressedManagedOverlapIDs   map[string]struct{}
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

type probableEndpointReporterHint struct {
	deviceID string
	ifIndex  int
	ifName   string
}

type segmentReporterIndex struct {
	byDevice        map[string]map[string]struct{}
	byDeviceIfIndex map[string]map[string]struct{}
	byDeviceIfName  map[string]map[string]struct{}
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

func normalizeTopologyInferenceStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", topologyInferenceStrategyFDBMinimumKnowledge:
		return topologyInferenceStrategyFDBMinimumKnowledge
	case topologyInferenceStrategySTPParentTree:
		return topologyInferenceStrategySTPParentTree
	case topologyInferenceStrategyFDBPairwise:
		return topologyInferenceStrategyFDBPairwise
	case topologyInferenceStrategySTPFDBCorrelated:
		return topologyInferenceStrategySTPFDBCorrelated
	case topologyInferenceStrategyCDPFDBHybrid:
		return topologyInferenceStrategyCDPFDBHybrid
	case topologyInferenceStrategyFDBOverlapWeighted:
		return topologyInferenceStrategyFDBOverlapWeighted
	case topologyInferenceStrategyExperimentalFull:
		return topologyInferenceStrategyExperimentalFull
	default:
		return topologyInferenceStrategyFDBMinimumKnowledge
	}
}

func topologyInferenceStrategyConfigFor(strategy string) topologyInferenceStrategyConfig {
	switch normalizeTopologyInferenceStrategy(strategy) {
	case topologyInferenceStrategySTPParentTree:
		return topologyInferenceStrategyConfig{
			id:                            topologyInferenceStrategySTPParentTree,
			includeSTPBridgeLinks:         true,
			useSTPDesignatedParent:        true,
			filterSwitchFacingAttachments: true,
		}
	case topologyInferenceStrategyFDBPairwise:
		return topologyInferenceStrategyConfig{
			id:                            topologyInferenceStrategyFDBPairwise,
			enableFDBPairwiseLinks:        true,
			filterSwitchFacingAttachments: true,
		}
	case topologyInferenceStrategySTPFDBCorrelated:
		return topologyInferenceStrategyConfig{
			id:                               topologyInferenceStrategySTPFDBCorrelated,
			includeLLDPBridgeLinks:           true,
			includeCDPBridgeLinks:            true,
			includeSTPBridgeLinks:            true,
			useSTPDesignatedParent:           true,
			enableFDBPairwiseLinks:           true,
			enableFDBOverlapLinks:            true,
			fdbOverlapMinShared:              2,
			enableSTPManagedAliasCorrelation: true,
			filterSwitchFacingAttachments:    true,
		}
	case topologyInferenceStrategyCDPFDBHybrid:
		return topologyInferenceStrategyConfig{
			id:                            topologyInferenceStrategyCDPFDBHybrid,
			includeCDPBridgeLinks:         true,
			enableFDBPairwiseLinks:        true,
			enableFDBOverlapLinks:         true,
			fdbOverlapMinShared:           2,
			filterSwitchFacingAttachments: true,
		}
	case topologyInferenceStrategyFDBOverlapWeighted:
		return topologyInferenceStrategyConfig{
			id:                            topologyInferenceStrategyFDBOverlapWeighted,
			enableFDBPairwiseLinks:        true,
			enableFDBOverlapLinks:         true,
			fdbOverlapMinShared:           2,
			filterSwitchFacingAttachments: true,
		}
	case topologyInferenceStrategyExperimentalFull:
		return topologyInferenceStrategyConfig{
			id:                               topologyInferenceStrategyExperimentalFull,
			includeLLDPBridgeLinks:           true,
			includeCDPBridgeLinks:            true,
			includeSTPBridgeLinks:            true,
			useSTPDesignatedParent:           true,
			enableFDBPairwiseLinks:           true,
			enableFDBOverlapLinks:            true,
			fdbOverlapMinShared:              1,
			enableSTPManagedAliasCorrelation: true,
			filterSwitchFacingAttachments:    false,
		}
	default:
		return topologyInferenceStrategyConfig{
			id:                            topologyInferenceStrategyFDBMinimumKnowledge,
			includeLLDPBridgeLinks:        true,
			includeCDPBridgeLinks:         true,
			filterSwitchFacingAttachments: true,
		}
	}
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
	strategyConfig := topologyInferenceStrategyConfigFor(opts.InferenceStrategy)

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
		for _, alias := range interfaceNameLookupAliases(iface.IfName, iface.IfDescr) {
			ifIndexByDeviceName[deviceIfNameKey(iface.DeviceID, alias)] = iface.IfIndex
		}
	}
	bridgeLinks := collectBridgeLinkRecords(result.Adjacencies, ifIndexByDeviceName, strategyConfig)
	reporterAliases := buildFDBReporterAliases(deviceByID, ifaceByDeviceIndex)
	if strategyConfig.enableFDBPairwiseLinks {
		bridgeLinks = mergeBridgeLinkRecordSets(
			bridgeLinks,
			inferFDBPairwiseBridgeLinks(result.Attachments, ifaceByDeviceIndex, reporterAliases),
		)
	}
	if strategyConfig.enableFDBOverlapLinks {
		bridgeLinks = mergeBridgeLinkRecordSets(
			bridgeLinks,
			inferFDBOverlapWeightedBridgeLinks(
				result.Attachments,
				ifaceByDeviceIndex,
				reporterAliases,
				strategyConfig.fdbOverlapMinShared,
			),
		)
	}
	// LLDP/CDP ports and device pairs are deterministic for direct adjacency.
	// Never allow inferred bridge links (STP/FDB-derived) to run in parallel
	// on top of those deterministic paths.
	deterministicTransitPortKeys := buildDeterministicTransitPortKeySet(result.Adjacencies, ifIndexByDeviceName)
	discoveryDevicePairs := buildDeterministicDiscoveryDevicePairSet(result.Adjacencies)
	bridgeLinks = suppressInferredBridgeLinksOnDeterministicDiscovery(
		bridgeLinks,
		deterministicTransitPortKeys,
		discoveryDevicePairs,
	)
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
		endpointActors.labelsByEndpointID,
		actorIndex,
		opts.ProbabilisticConnectivity,
		strategyConfig,
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
	if opts.CollapseActorsByIP {
		actors = collapseActorsByIP(actors)
	}
	if opts.EliminateNonIPInferred {
		actors, links = eliminateNonIPInferredActors(actors, links)
	}
	unlinkedSuppressed := 0
	if opts.CollapseActorsByIP {
		actors, unlinkedSuppressed = pruneManagedOverlapUnlinkedEndpointActors(
			actors,
			links,
			segmentProjection.suppressedManagedOverlapIDs,
		)
	}
	actors, links, _ = pruneSegmentArtifacts(actors, links)
	sortTopologyActors(actors)
	sortTopologyLinks(links)
	applyTopologyDisplayNames(actors, links, opts.ResolveDNSName)
	assignTopologyActorIDsAndLinkEndpoints(actors, links)
	linkCounts := summarizeTopologyLinks(links)
	probableLinks := 0
	for _, link := range links {
		if strings.EqualFold(strings.TrimSpace(link.State), "probable") {
			probableLinks++
			continue
		}
		if strings.EqualFold(topologyMetricString(link.Metrics, "inference"), "probable") {
			probableLinks++
		}
	}

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
	stats["links_probable"] = probableLinks
	stats["segments_suppressed"] = segmentSuppressed
	stats["actors_total"] = len(actors)
	stats["actors_unlinked_suppressed"] = unlinkedSuppressed
	stats["endpoints_total"] = endpointActors.count
	stats["inference_strategy"] = strategyConfig.id

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
	}, "|")
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
		key := src + "|" + dst
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

		reverseKey := dst + "|" + src
		candidates := directionToIndexes[reverseKey]
		if len(candidates) != 1 {
			continue
		}

		reverseEntry := entries[candidates[0]]
		if reverseEntry == nil {
			continue
		}
		if candidates[0] == i {
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
	endpointLabelsByID map[string]map[string]string,
	actorIndex map[string]struct{},
	probabilisticConnectivity bool,
	strategyConfig topologyInferenceStrategyConfig,
) projectedSegments {
	out := projectedSegments{
		actors: make([]topology.Actor, 0),
		links:  make([]topology.Link, 0),
	}
	if len(attachments) == 0 && len(adjacencies) == 0 {
		return out
	}

	// Hard deterministic rule: LLDP/CDP-adjacent ports are transit ports.
	// FDB data learned on those ports belongs to the neighbor domain and must
	// not create parallel inferred segment paths on top of direct discovery.
	//
	// NOTE:
	// switch-facing/trunk classification must not suppress endpoint ownership.
	// It is a topology correlation/confidence signal, not a hard endpoint
	// placement filter.
	deterministicTransitPortKeys := buildDeterministicTransitPortKeySet(adjacencies, ifIndexByDeviceName)
	seedMacLinks := collectBridgeMacLinkRecords(attachments, ifaceByDeviceIndex, deterministicTransitPortKeys)
	rawFDBObservations := buildFDBReporterObservations(seedMacLinks)
	macLinks := seedMacLinks
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
		match, actor := buildBridgeSegmentActor(segmentID, segment, layer, source)
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
	segmentIfIndexes := make(map[string]map[string]struct{}, len(segmentIDs))
	segmentIfNames := make(map[string]map[string]struct{}, len(segmentIDs))
	for _, segmentID := range segmentIDs {
		segment := segmentByID[segmentID]
		if segment == nil {
			continue
		}
		portKeys := make(map[string]struct{}, len(segment.ports))
		ifIndexes := make(map[string]struct{}, len(segment.ports))
		ifNames := make(map[string]struct{}, len(segment.ports))
		for _, port := range segment.ports {
			if portKey := bridgePortObservationKey(port); portKey != "" {
				portKeys[portKey] = struct{}{}
			}
			if portVLANKey := bridgePortObservationVLANKey(port); portVLANKey != "" {
				portKeys[portVLANKey] = struct{}{}
			}
			if port.ifIndex > 0 {
				ifIndexes[strconv.Itoa(port.ifIndex)] = struct{}{}
			}
			if ifName := strings.TrimSpace(port.ifName); ifName != "" {
				ifNames[strings.ToLower(ifName)] = struct{}{}
			}
		}
		segmentPortKeys[segmentID] = portKeys
		segmentIfIndexes[segmentID] = ifIndexes
		segmentIfNames[segmentID] = ifNames
		for endpointID := range segment.endpointIDs {
			endpointID = strings.TrimSpace(endpointID)
			if endpointID == "" {
				continue
			}
			endpointSegmentCandidates[endpointID] = append(endpointSegmentCandidates[endpointID], segmentID)
		}
	}
	rawFDBReporterHints := buildFDBEndpointReporterHints(seedMacLinks)
	fdbObservations := buildFDBReporterObservations(macLinks)
	fdbOwners := inferFDBEndpointOwners(fdbObservations, reporterAliases, deterministicTransitPortKeys)
	for endpointID, owner := range inferSinglePortEndpointOwners(macLinks, deterministicTransitPortKeys) {
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
	reporterSegmentIndex := buildSegmentReporterIndex(segmentIDs, segmentByID)
	aliasOwnerIDs := buildFDBAliasOwnerMap(reporterAliases)
	managedDeviceIDs := make(map[string]struct{}, len(deviceByID))
	for deviceID := range deviceByID {
		deviceID = strings.TrimSpace(deviceID)
		if deviceID == "" {
			continue
		}
		managedDeviceIDs[deviceID] = struct{}{}
	}
	managedDeviceIDList := sortedTopologySet(managedDeviceIDs)
	allowedEndpointBySegment := make(map[string]map[string]struct{})
	strictEndpointBySegment := make(map[string]map[string]struct{})
	probableEndpointBySegment := make(map[string]map[string]struct{})
	probableAttachmentModeBySegment := make(map[string]map[string]string)
	assignedEndpoints := make(map[string]struct{}, len(endpointMatchByID))
	allowEndpoint := func(segmentID, endpointID string, probable bool, probableMode string) {
		if strings.TrimSpace(segmentID) == "" || strings.TrimSpace(endpointID) == "" {
			return
		}

		allowed := allowedEndpointBySegment[segmentID]
		if allowed == nil {
			allowed = make(map[string]struct{})
			allowedEndpointBySegment[segmentID] = allowed
		}
		allowed[endpointID] = struct{}{}
		assignedEndpoints[endpointID] = struct{}{}

		if !probable {
			strictSet := strictEndpointBySegment[segmentID]
			if strictSet == nil {
				strictSet = make(map[string]struct{})
				strictEndpointBySegment[segmentID] = strictSet
			}
			strictSet[endpointID] = struct{}{}
			return
		}

		probableSet := probableEndpointBySegment[segmentID]
		if probableSet == nil {
			probableSet = make(map[string]struct{})
			probableEndpointBySegment[segmentID] = probableSet
		}
		probableSet[endpointID] = struct{}{}
		if strings.TrimSpace(probableMode) == "" {
			probableMode = "probable_segment"
		}
		modes := probableAttachmentModeBySegment[segmentID]
		if modes == nil {
			modes = make(map[string]string)
			probableAttachmentModeBySegment[segmentID] = modes
		}
		modes[endpointID] = probableMode
	}
	endpointIDs := collectTopologyEndpointIDs(endpointMatchByID, endpointLabelsByID, endpointSegmentCandidates, rawFDBObservations, fdbObservations)
	baseCandidatesByEndpoint := make(map[string][]string, len(endpointIDs))
	probableBaseCandidatesByEndpoint := make(map[string][]string, len(endpointIDs))
	strictLinkedEndpoints := make(map[string]struct{}, len(endpointIDs))

	for _, endpointID := range endpointIDs {
		candidates := endpointSegmentCandidates[endpointID]
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
		baseCandidatesByEndpoint[endpointID] = sortedCandidates
		strictSegmentID := ""
		probableCandidates := sortedCandidates
		if len(sortedCandidates) == 1 {
			strictSegmentID = sortedCandidates[0]
		} else if owner, ok := fdbOwners[endpointID]; ok {
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
				strictSegmentID = filtered[0]
			}
			if len(filtered) > 0 {
				probableCandidates = filtered
			}
		}
		probableBaseCandidatesByEndpoint[endpointID] = probableCandidates

		if strictSegmentID != "" {
			allowEndpoint(strictSegmentID, endpointID, false, "")
			strictLinkedEndpoints[endpointID] = struct{}{}
		}
	}

	if probabilisticConnectivity {
		for _, endpointID := range endpointIDs {
			if _, strictLinked := strictLinkedEndpoints[endpointID]; strictLinked {
				continue
			}

			baseCandidates := baseCandidatesByEndpoint[endpointID]
			probableCandidates := append([]string(nil), probableBaseCandidatesByEndpoint[endpointID]...)
			if len(probableCandidates) == 0 {
				probableCandidates = probableCandidateSegmentsFromReporterHints(
					endpointLabelsByID[endpointID],
					rawFDBObservations.byEndpoint[normalizeFDBEndpointID(endpointID)],
					reporterSegmentIndex,
					aliasOwnerIDs,
					managedDeviceIDs,
				)
			}

			segmentID := pickMostProbableSegment(
				probableCandidates,
				endpointLabelsByID[endpointID],
				segmentIfIndexes,
				segmentIfNames,
			)
			if segmentID == "" && len(probableCandidates) > 0 {
				segmentID = probableCandidates[0]
			}
			if segmentID != "" && !segmentHasManagedPort(segmentByID[segmentID], managedDeviceIDs) {
				hint := selectProbableEndpointReporterHint(
					endpointLabelsByID[endpointID],
					rawFDBReporterHints[normalizeFDBEndpointID(endpointID)],
					fdbOwners[endpointID],
					aliasOwnerIDs,
					managedDeviceIDs,
				)
				hint = ensureManagedProbableReporterHint(
					hint,
					endpointLabelsByID[endpointID],
					rawFDBReporterHints[normalizeFDBEndpointID(endpointID)],
					aliasOwnerIDs,
					managedDeviceIDs,
					managedDeviceIDList,
				)
				if strings.TrimSpace(hint.deviceID) != "" {
					created := false
					segmentID, created = ensureProbablePortlessSegment(segmentByID, hint)
					if created {
						segmentIDs = append(segmentIDs, segmentID)
						segmentIfIndexes[segmentID] = make(map[string]struct{})
						segmentIfNames[segmentID] = make(map[string]struct{})
						if hint.ifIndex > 0 {
							segmentIfIndexes[segmentID][strconv.Itoa(hint.ifIndex)] = struct{}{}
						}
						if ifName := strings.ToLower(strings.TrimSpace(hint.ifName)); ifName != "" {
							segmentIfNames[segmentID][ifName] = struct{}{}
						}
						match, actor := buildBridgeSegmentActor(segmentID, segmentByID[segmentID], layer, source)
						keys := topologyMatchIdentityKeys(actor.Match)
						if len(keys) > 0 && !topologyIdentityIndexOverlaps(actorIndex, keys) {
							addTopologyIdentityKeys(actorIndex, keys)
						}
						out.actors = append(out.actors, actor)
						segmentMatchByID[segmentID] = match
					}
					if seg := segmentByID[segmentID]; seg != nil {
						seg.addEndpoint(endpointID, "probable")
					}
				}
			}

			if segmentID == "" {
				hint := selectProbableEndpointReporterHint(
					endpointLabelsByID[endpointID],
					rawFDBReporterHints[normalizeFDBEndpointID(endpointID)],
					fdbOwners[endpointID],
					aliasOwnerIDs,
					managedDeviceIDs,
				)
				hint = ensureManagedProbableReporterHint(
					hint,
					endpointLabelsByID[endpointID],
					rawFDBReporterHints[normalizeFDBEndpointID(endpointID)],
					aliasOwnerIDs,
					managedDeviceIDs,
					managedDeviceIDList,
				)
				if strings.TrimSpace(hint.deviceID) != "" {
					created := false
					segmentID, created = ensureProbablePortlessSegment(segmentByID, hint)
					if created {
						segmentIDs = append(segmentIDs, segmentID)
						segmentIfIndexes[segmentID] = make(map[string]struct{})
						segmentIfNames[segmentID] = make(map[string]struct{})
						if hint.ifIndex > 0 {
							segmentIfIndexes[segmentID][strconv.Itoa(hint.ifIndex)] = struct{}{}
						}
						if ifName := strings.ToLower(strings.TrimSpace(hint.ifName)); ifName != "" {
							segmentIfNames[segmentID][ifName] = struct{}{}
						}
						match, actor := buildBridgeSegmentActor(segmentID, segmentByID[segmentID], layer, source)
						keys := topologyMatchIdentityKeys(actor.Match)
						if len(keys) > 0 && !topologyIdentityIndexOverlaps(actorIndex, keys) {
							addTopologyIdentityKeys(actorIndex, keys)
						}
						out.actors = append(out.actors, actor)
						segmentMatchByID[segmentID] = match
					}
					if seg := segmentByID[segmentID]; seg != nil {
						seg.addEndpoint(endpointID, "probable")
					}
				}
			}

			if segmentID != "" {
				probableMode := "probable_segment"
				if strings.HasPrefix(segmentID, "bridge-domain:probable:") {
					probableMode = "probable_portless"
				}
				allowEndpoint(segmentID, endpointID, true, probableMode)
				if len(baseCandidates) > 1 {
					out.endpointLinksSuppressed += len(baseCandidates) - 1
				}
				continue
			}

			if len(baseCandidates) > 1 {
				out.endpointsWithAmbiguousSegment++
				out.endpointLinksSuppressed += len(baseCandidates)
			}
		}
	} else {
		for _, endpointID := range endpointIDs {
			if _, strictLinked := strictLinkedEndpoints[endpointID]; strictLinked {
				continue
			}
			baseCandidates := baseCandidatesByEndpoint[endpointID]
			if len(baseCandidates) > 1 {
				out.endpointsWithAmbiguousSegment++
				out.endpointLinksSuppressed += len(baseCandidates)
			}
		}
	}
	if probabilisticConnectivity && len(managedDeviceIDs) > 0 {
		for _, endpointID := range endpointIDs {
			if _, alreadyAssigned := assignedEndpoints[endpointID]; alreadyAssigned {
				continue
			}
			hint := selectProbableEndpointReporterHint(
				endpointLabelsByID[endpointID],
				rawFDBReporterHints[normalizeFDBEndpointID(endpointID)],
				fdbOwners[endpointID],
				aliasOwnerIDs,
				managedDeviceIDs,
			)
			hint = ensureManagedProbableReporterHint(
				hint,
				endpointLabelsByID[endpointID],
				rawFDBReporterHints[normalizeFDBEndpointID(endpointID)],
				aliasOwnerIDs,
				managedDeviceIDs,
				managedDeviceIDList,
			)
			if strings.TrimSpace(hint.deviceID) == "" {
				continue
			}
			created := false
			segmentID, created := ensureProbablePortlessSegment(segmentByID, hint)
			if strings.TrimSpace(segmentID) == "" {
				continue
			}
			if created {
				segmentIDs = append(segmentIDs, segmentID)
				segmentIfIndexes[segmentID] = make(map[string]struct{})
				segmentIfNames[segmentID] = make(map[string]struct{})
				if hint.ifIndex > 0 {
					segmentIfIndexes[segmentID][strconv.Itoa(hint.ifIndex)] = struct{}{}
				}
				if ifName := strings.ToLower(strings.TrimSpace(hint.ifName)); ifName != "" {
					segmentIfNames[segmentID][ifName] = struct{}{}
				}
				match, actor := buildBridgeSegmentActor(segmentID, segmentByID[segmentID], layer, source)
				keys := topologyMatchIdentityKeys(actor.Match)
				if len(keys) > 0 && !topologyIdentityIndexOverlaps(actorIndex, keys) {
					addTopologyIdentityKeys(actorIndex, keys)
				}
				out.actors = append(out.actors, actor)
				segmentMatchByID[segmentID] = match
			}
			if seg := segmentByID[segmentID]; seg != nil {
				seg.addEndpoint(endpointID, "probable")
			}
			allowEndpoint(segmentID, endpointID, true, "probable_portless")
		}
	}
	sort.Strings(segmentIDs)

	probableOnlyAnchorPortIDBySegment := make(map[string]string)
	for _, segmentID := range segmentIDs {
		if len(probableEndpointBySegment[segmentID]) == 0 {
			continue
		}
		if len(strictEndpointBySegment[segmentID]) > 0 {
			continue
		}
		segment := segmentByID[segmentID]
		if segment == nil {
			continue
		}
		if portID := pickProbableSegmentAnchorPortID(segment, probableEndpointBySegment[segmentID], fdbOwners, managedDeviceIDs); portID != "" {
			probableOnlyAnchorPortIDBySegment[segmentID] = portID
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
		probableOnlyAnchorPortID := probableOnlyAnchorPortIDBySegment[segmentID]
		for _, portID := range portIDs {
			if probableOnlyAnchorPortID != "" && portID != probableOnlyAnchorPortID {
				continue
			}
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

		allowedEndpoints := allowedEndpointBySegment[segmentID]
		if len(allowedEndpoints) == 0 {
			continue
		}
		endpointSet := make(map[string]struct{}, len(segment.endpointIDs)+len(allowedEndpoints))
		for endpointID := range segment.endpointIDs {
			endpointSet[endpointID] = struct{}{}
		}
		for endpointID := range allowedEndpoints {
			endpointSet[endpointID] = struct{}{}
		}
		endpointIDs := sortedTopologySet(endpointSet)
		for _, endpointID := range endpointIDs {
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
				matchedManagedDeviceIDs := make([]string, 0, len(overlappingDeviceIDs))
				for _, overlapID := range overlappingDeviceIDs {
					if _, ok := deviceByID[overlapID]; ok {
						matchedManagedDeviceIDs = append(matchedManagedDeviceIDs, overlapID)
					}
				}
				if len(matchedManagedDeviceIDs) > 0 {
					if len(matchedManagedDeviceIDs) == 1 {
						matchedDeviceID := matchedManagedDeviceIDs[0]
						if segmentContainsDevice(segment, matchedDeviceID) {
							if out.suppressedManagedOverlapIDs == nil {
								out.suppressedManagedOverlapIDs = make(map[string]struct{})
							}
							out.suppressedManagedOverlapIDs[normalizeFDBEndpointID(endpointID)] = struct{}{}
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
							if out.suppressedManagedOverlapIDs == nil {
								out.suppressedManagedOverlapIDs = make(map[string]struct{})
							}
							out.suppressedManagedOverlapIDs[normalizeFDBEndpointID(endpointID)] = struct{}{}
							continue
						}
					}
					if out.suppressedManagedOverlapIDs == nil {
						out.suppressedManagedOverlapIDs = make(map[string]struct{})
					}
					out.suppressedManagedOverlapIDs[normalizeFDBEndpointID(endpointID)] = struct{}{}
					out.endpointLinksSuppressed++
					continue
				}
				if !probabilisticConnectivity {
					out.endpointLinksSuppressed++
					continue
				}
				allowEndpoint(segmentID, endpointID, true, "probable_segment")
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
							metrics := map[string]any{
								"attachment_mode": "direct",
							}
							linkState := ""
							if probableSet := probableEndpointBySegment[segmentID]; len(probableSet) > 0 {
								if _, isProbable := probableSet[endpointID]; isProbable {
									metrics["attachment_mode"] = "probable_direct"
									metrics["inference"] = "probable"
									metrics["confidence"] = "low"
									linkState = "probable"
								}
							}
							out.links = append(out.links, topology.Link{
								Layer:        layer,
								Protocol:     "fdb",
								Direction:    "bidirectional",
								Src:          adjacencySideToEndpoint(device, localPort, ifIndexByDeviceName, ifaceByDeviceIndex),
								Dst:          topology.LinkEndpoint{Match: endpointMatch},
								DiscoveredAt: topologyTimePtr(collectedAt),
								LastSeen:     topologyTimePtr(collectedAt),
								State:        linkState,
								Metrics:      metrics,
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

			metrics := map[string]any{
				"bridge_domain": segmentID,
			}
			linkState := ""
			if probableSet := probableEndpointBySegment[segmentID]; len(probableSet) > 0 {
				if _, isProbable := probableSet[endpointID]; isProbable {
					probableMode := ""
					if modes := probableAttachmentModeBySegment[segmentID]; len(modes) > 0 {
						probableMode = strings.TrimSpace(modes[endpointID])
					}
					if probableMode == "" {
						probableMode = "probable_segment"
					}
					metrics["attachment_mode"] = probableMode
					metrics["inference"] = "probable"
					metrics["confidence"] = "low"
					linkState = "probable"
				}
			}

			out.links = append(out.links, topology.Link{
				Layer:        layer,
				Protocol:     "fdb",
				Direction:    "bidirectional",
				Src:          segmentEndpoint,
				Dst:          topology.LinkEndpoint{Match: endpointMatch},
				DiscoveredAt: topologyTimePtr(collectedAt),
				LastSeen:     topologyTimePtr(collectedAt),
				State:        linkState,
				Metrics:      metrics,
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

func pickProbableSegmentAnchorPortID(
	segment *bridgeDomainSegment,
	probableEndpoints map[string]struct{},
	fdbOwners map[string]fdbEndpointOwner,
	managedDeviceIDs map[string]struct{},
) string {
	if segment == nil || len(segment.ports) == 0 {
		return ""
	}

	portIDs := make([]string, 0, len(segment.ports))
	portIDByObservation := make(map[string]string, len(segment.ports)*2)
	designatedPortID := ""
	managedPortIDs := make(map[string]struct{})
	for portID, port := range segment.ports {
		portIDs = append(portIDs, portID)
		if key := bridgePortObservationKey(port); key != "" {
			portIDByObservation[key] = portID
		}
		if key := bridgePortObservationVLANKey(port); key != "" {
			portIDByObservation[key] = portID
		}
		if segment.portIdentityKey(port) == segment.portIdentityKey(segment.designatedPort) {
			designatedPortID = portID
		}
		if len(managedDeviceIDs) == 0 {
			managedPortIDs[portID] = struct{}{}
			continue
		}
		if _, ok := managedDeviceIDs[strings.TrimSpace(port.deviceID)]; ok {
			managedPortIDs[portID] = struct{}{}
		}
	}
	sort.Strings(portIDs)
	preferManaged := len(managedPortIDs) > 0
	allowPortID := func(portID string) bool {
		if !preferManaged {
			return true
		}
		_, ok := managedPortIDs[portID]
		return ok
	}

	endpointIDs := sortedTopologySet(probableEndpoints)
	for _, endpointID := range endpointIDs {
		owner, ok := fdbOwners[endpointID]
		if !ok {
			continue
		}
		if portID, ok := portIDByObservation[owner.portVLANKey]; ok {
			if allowPortID(portID) {
				return portID
			}
		}
		if portID, ok := portIDByObservation[owner.portKey]; ok {
			if allowPortID(portID) {
				return portID
			}
		}
	}

	if designatedPortID != "" && allowPortID(designatedPortID) {
		return designatedPortID
	}
	if preferManaged {
		managedPortIDList := make([]string, 0, len(managedPortIDs))
		for portID := range managedPortIDs {
			managedPortIDList = append(managedPortIDList, portID)
		}
		sort.Strings(managedPortIDList)
		if len(managedPortIDList) > 0 {
			return managedPortIDList[0]
		}
	}
	if designatedPortID != "" {
		return designatedPortID
	}
	return portIDs[0]
}

func segmentHasManagedPort(segment *bridgeDomainSegment, managedDeviceIDs map[string]struct{}) bool {
	if segment == nil || len(segment.ports) == 0 {
		return false
	}
	if len(managedDeviceIDs) == 0 {
		return true
	}
	for _, port := range segment.ports {
		if _, ok := managedDeviceIDs[strings.TrimSpace(port.deviceID)]; ok {
			return true
		}
	}
	return false
}

func pickMostProbableSegment(
	candidates []string,
	endpointLabels map[string]string,
	segmentIfIndexes map[string]map[string]struct{},
	segmentIfNames map[string]map[string]struct{},
) string {
	if len(candidates) == 0 {
		return ""
	}
	if len(candidates) == 1 {
		return candidates[0]
	}

	ifIndexes := make(map[string]struct{})
	for _, ifIndex := range labelsCSVToSlice(endpointLabels, "learned_if_indexes") {
		ifIndex = strings.TrimSpace(ifIndex)
		if ifIndex == "" {
			continue
		}
		ifIndexes[ifIndex] = struct{}{}
	}
	ifNames := make(map[string]struct{})
	for _, ifName := range labelsCSVToSlice(endpointLabels, "learned_if_names") {
		ifName = strings.ToLower(strings.TrimSpace(ifName))
		if ifName == "" {
			continue
		}
		ifNames[ifName] = struct{}{}
	}

	bestID := ""
	bestScore := -1
	for _, segmentID := range candidates {
		score := 0
		for ifIndex := range ifIndexes {
			if indexes := segmentIfIndexes[segmentID]; indexes != nil {
				if _, ok := indexes[ifIndex]; ok {
					score += 2
				}
			}
		}
		for ifName := range ifNames {
			if names := segmentIfNames[segmentID]; names != nil {
				if _, ok := names[ifName]; ok {
					score++
				}
			}
		}
		if score > bestScore {
			bestScore = score
			bestID = segmentID
			continue
		}
		if score == bestScore && (bestID == "" || segmentID < bestID) {
			bestID = segmentID
		}
	}
	if bestID != "" {
		return bestID
	}
	return candidates[0]
}

func buildBridgeSegmentActor(segmentID string, segment *bridgeDomainSegment, layer string, source string) (topology.Match, topology.Actor) {
	parentDevices := make(map[string]struct{})
	ifNames := make(map[string]struct{})
	ifIndexes := make(map[string]struct{})
	bridgePorts := make(map[string]struct{})
	vlanIDs := make(map[string]struct{})
	if segment != nil {
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
		"ports_total":     0,
		"endpoints_total": 0,
	}
	if segment != nil {
		attrs["learned_sources"] = sortedTopologySet(segment.methods)
		attrs["ports_total"] = len(segment.ports)
		attrs["endpoints_total"] = len(segment.endpointIDs)
		if bridgePortRefKey(segment.designatedPort, false, false) != "" {
			attrs["designated_port"] = bridgePortRefSortKey(segment.designatedPort)
		}
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

	return match, actor
}

func collectTopologyEndpointIDs(
	endpointMatchByID map[string]topology.Match,
	endpointLabelsByID map[string]map[string]string,
	endpointSegmentCandidates map[string][]string,
	rawFDBObservations fdbReporterObservation,
	filteredFDBObservations fdbReporterObservation,
) []string {
	set := make(map[string]struct{})
	for endpointID := range endpointMatchByID {
		endpointID = strings.TrimSpace(endpointID)
		if endpointID != "" {
			set[endpointID] = struct{}{}
		}
	}
	for endpointID := range endpointLabelsByID {
		endpointID = strings.TrimSpace(endpointID)
		if endpointID != "" {
			set[endpointID] = struct{}{}
		}
	}
	for endpointID := range endpointSegmentCandidates {
		endpointID = strings.TrimSpace(endpointID)
		if endpointID != "" {
			set[endpointID] = struct{}{}
		}
	}
	for endpointID := range rawFDBObservations.byEndpoint {
		endpointID = strings.TrimSpace(endpointID)
		if endpointID != "" {
			set[endpointID] = struct{}{}
		}
	}
	for endpointID := range filteredFDBObservations.byEndpoint {
		endpointID = strings.TrimSpace(endpointID)
		if endpointID != "" {
			set[endpointID] = struct{}{}
		}
	}
	return sortedTopologySet(set)
}

func buildFDBEndpointReporterHints(macLinks []bridgeMacLinkRecord) map[string]map[string][]bridgePortRef {
	if len(macLinks) == 0 {
		return nil
	}

	byEndpointReporterPorts := make(map[string]map[string]map[string]bridgePortRef)
	for _, link := range macLinks {
		if strings.ToLower(strings.TrimSpace(link.method)) != "fdb" {
			continue
		}
		endpointID := normalizeFDBEndpointID(link.endpointID)
		reporterID := strings.TrimSpace(link.port.deviceID)
		portKey := bridgePortObservationVLANKey(link.port)
		if endpointID == "" || reporterID == "" || portKey == "" {
			continue
		}
		reporters := byEndpointReporterPorts[endpointID]
		if reporters == nil {
			reporters = make(map[string]map[string]bridgePortRef)
			byEndpointReporterPorts[endpointID] = reporters
		}
		ports := reporters[reporterID]
		if ports == nil {
			ports = make(map[string]bridgePortRef)
			reporters[reporterID] = ports
		}
		ports[portKey] = link.port
	}

	out := make(map[string]map[string][]bridgePortRef, len(byEndpointReporterPorts))
	for endpointID, reporters := range byEndpointReporterPorts {
		reporterHints := make(map[string][]bridgePortRef, len(reporters))
		reporterIDs := make([]string, 0, len(reporters))
		for reporterID := range reporters {
			reporterIDs = append(reporterIDs, reporterID)
		}
		sort.Strings(reporterIDs)
		for _, reporterID := range reporterIDs {
			portsMap := reporters[reporterID]
			if len(portsMap) == 0 {
				continue
			}
			portKeys := make([]string, 0, len(portsMap))
			for key := range portsMap {
				portKeys = append(portKeys, key)
			}
			sort.Strings(portKeys)
			ports := make([]bridgePortRef, 0, len(portKeys))
			for _, key := range portKeys {
				ports = append(ports, portsMap[key])
			}
			reporterHints[reporterID] = ports
		}
		if len(reporterHints) > 0 {
			out[endpointID] = reporterHints
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildSegmentReporterIndex(
	segmentIDs []string,
	segmentByID map[string]*bridgeDomainSegment,
) segmentReporterIndex {
	index := segmentReporterIndex{
		byDevice:        make(map[string]map[string]struct{}),
		byDeviceIfIndex: make(map[string]map[string]struct{}),
		byDeviceIfName:  make(map[string]map[string]struct{}),
	}
	for _, segmentID := range segmentIDs {
		segment := segmentByID[segmentID]
		if segment == nil {
			continue
		}
		for _, port := range segment.ports {
			deviceID := strings.TrimSpace(port.deviceID)
			if deviceID == "" {
				continue
			}
			addStringSet(index.byDevice, deviceID, segmentID)
			if port.ifIndex > 0 {
				addStringSet(index.byDeviceIfIndex, deviceID+"|"+strconv.Itoa(port.ifIndex), segmentID)
			}
			if ifName := strings.ToLower(strings.TrimSpace(port.ifName)); ifName != "" {
				addStringSet(index.byDeviceIfName, deviceID+"|"+ifName, segmentID)
			}
		}
	}
	return index
}

func addStringSet(out map[string]map[string]struct{}, key string, value string) {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	set := out[key]
	if set == nil {
		set = make(map[string]struct{})
		out[key] = set
	}
	set[value] = struct{}{}
}

func probableCandidateSegmentsFromReporterHints(
	endpointLabels map[string]string,
	fdbReporters map[string]map[string]struct{},
	reporterSegmentIndex segmentReporterIndex,
	aliasOwnerIDs map[string]map[string]struct{},
	managedDeviceIDs map[string]struct{},
) []string {
	deviceIDs := resolveTopologyEndpointDeviceHints(
		topologyEndpointLabelDeviceIDs(endpointLabels),
		aliasOwnerIDs,
	)
	if len(deviceIDs) == 0 {
		for reporterID := range fdbReporters {
			reporterID = strings.TrimSpace(reporterID)
			if reporterID == "" {
				continue
			}
			deviceIDs = append(deviceIDs, reporterID)
		}
		deviceIDs = resolveTopologyEndpointDeviceHints(deviceIDs, aliasOwnerIDs)
	}
	deviceIDs = filterManagedDeviceHints(deviceIDs, managedDeviceIDs)
	if len(deviceIDs) == 0 {
		return nil
	}

	ifIndexes := labelsCSVToSlice(endpointLabels, "learned_if_indexes")
	ifNames := labelsCSVToSlice(endpointLabels, "learned_if_names")
	hasPortHints := false
	for _, ifIndex := range ifIndexes {
		if strings.TrimSpace(ifIndex) != "" {
			hasPortHints = true
			break
		}
	}
	if !hasPortHints {
		for _, ifName := range ifNames {
			if strings.TrimSpace(ifName) != "" {
				hasPortHints = true
				break
			}
		}
	}
	candidateSet := make(map[string]struct{})

	for _, deviceID := range deviceIDs {
		for _, ifIndex := range ifIndexes {
			ifIndex = strings.TrimSpace(ifIndex)
			if ifIndex == "" {
				continue
			}
			for segmentID := range reporterSegmentIndex.byDeviceIfIndex[deviceID+"|"+ifIndex] {
				candidateSet[segmentID] = struct{}{}
			}
		}
		for _, ifName := range ifNames {
			ifName = strings.ToLower(strings.TrimSpace(ifName))
			if ifName == "" {
				continue
			}
			for segmentID := range reporterSegmentIndex.byDeviceIfName[deviceID+"|"+ifName] {
				candidateSet[segmentID] = struct{}{}
			}
		}
	}

	if len(candidateSet) == 0 && !hasPortHints {
		for _, deviceID := range deviceIDs {
			for segmentID := range reporterSegmentIndex.byDevice[deviceID] {
				candidateSet[segmentID] = struct{}{}
			}
		}
	}
	if len(candidateSet) == 0 {
		return nil
	}
	return sortedTopologySet(candidateSet)
}

func selectProbableEndpointReporterHint(
	endpointLabels map[string]string,
	reporterHints map[string][]bridgePortRef,
	owner fdbEndpointOwner,
	aliasOwnerIDs map[string]map[string]struct{},
	managedDeviceIDs map[string]struct{},
) probableEndpointReporterHint {
	ownerDeviceID := strings.TrimSpace(owner.port.deviceID)
	if ownerDeviceID != "" {
		if len(managedDeviceIDs) == 0 {
			return probableEndpointReporterHint{
				deviceID: ownerDeviceID,
				ifIndex:  owner.port.ifIndex,
				ifName:   strings.TrimSpace(owner.port.ifName),
			}
		}
		if _, ok := managedDeviceIDs[ownerDeviceID]; ok {
			return probableEndpointReporterHint{
				deviceID: ownerDeviceID,
				ifIndex:  owner.port.ifIndex,
				ifName:   strings.TrimSpace(owner.port.ifName),
			}
		}
	}

	deviceIDs := resolveTopologyEndpointDeviceHints(
		topologyEndpointLabelDeviceIDs(endpointLabels),
		aliasOwnerIDs,
	)
	if len(deviceIDs) == 0 {
		for reporterID := range reporterHints {
			reporterID = strings.TrimSpace(reporterID)
			if reporterID == "" {
				continue
			}
			deviceIDs = append(deviceIDs, reporterID)
		}
		deviceIDs = resolveTopologyEndpointDeviceHints(deviceIDs, aliasOwnerIDs)
	}
	deviceIDs = filterManagedDeviceHints(deviceIDs, managedDeviceIDs)
	if len(deviceIDs) == 0 {
		return probableEndpointReporterHint{}
	}

	ifIndexes := labelsCSVToSlice(endpointLabels, "learned_if_indexes")
	ifNames := labelsCSVToSlice(endpointLabels, "learned_if_names")
	parsedIfIndex := 0
	for _, value := range ifIndexes {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			parsedIfIndex = n
			break
		}
	}
	parsedIfName := ""
	for _, value := range ifNames {
		value = strings.TrimSpace(value)
		if value != "" {
			parsedIfName = value
			break
		}
	}

	selectedDeviceID := deviceIDs[0]
	for _, deviceID := range deviceIDs {
		if len(reporterHints[deviceID]) > 0 {
			selectedDeviceID = deviceID
			break
		}
	}
	hint := probableEndpointReporterHint{
		deviceID: selectedDeviceID,
		ifIndex:  parsedIfIndex,
		ifName:   parsedIfName,
	}
	if ports := reporterHints[selectedDeviceID]; len(ports) > 0 {
		sort.SliceStable(ports, func(i, j int) bool {
			return bridgePortRefSortKey(ports[i]) < bridgePortRefSortKey(ports[j])
		})
		port := ports[0]
		if hint.ifIndex == 0 && port.ifIndex > 0 {
			hint.ifIndex = port.ifIndex
		}
		if strings.TrimSpace(hint.ifName) == "" && strings.TrimSpace(port.ifName) != "" {
			hint.ifName = strings.TrimSpace(port.ifName)
		}
	}

	if hint.ifIndex == 0 && strings.TrimSpace(hint.ifName) == "" {
		hint.ifName = "0"
	}
	return hint
}

func ensureManagedProbableReporterHint(
	hint probableEndpointReporterHint,
	endpointLabels map[string]string,
	reporterHints map[string][]bridgePortRef,
	aliasOwnerIDs map[string]map[string]struct{},
	managedDeviceIDs map[string]struct{},
	managedDeviceIDList []string,
) probableEndpointReporterHint {
	if len(managedDeviceIDs) == 0 {
		return hint
	}
	deviceID := strings.TrimSpace(hint.deviceID)
	if deviceID != "" {
		if _, ok := managedDeviceIDs[deviceID]; ok {
			return hint
		}
	}

	deviceIDs := resolveTopologyEndpointDeviceHints(
		topologyEndpointLabelDeviceIDs(endpointLabels),
		aliasOwnerIDs,
	)
	if len(deviceIDs) == 0 {
		for reporterID := range reporterHints {
			reporterID = strings.TrimSpace(reporterID)
			if reporterID == "" {
				continue
			}
			deviceIDs = append(deviceIDs, reporterID)
		}
		deviceIDs = resolveTopologyEndpointDeviceHints(deviceIDs, aliasOwnerIDs)
	}
	deviceIDs = filterManagedDeviceHints(deviceIDs, managedDeviceIDs)
	if len(deviceIDs) == 0 {
		deviceIDs = managedDeviceIDList
	}
	if len(deviceIDs) == 0 {
		return probableEndpointReporterHint{}
	}

	hint.deviceID = strings.TrimSpace(deviceIDs[0])

	ports := reporterHints[hint.deviceID]
	if len(ports) > 0 {
		sort.SliceStable(ports, func(i, j int) bool {
			return bridgePortRefSortKey(ports[i]) < bridgePortRefSortKey(ports[j])
		})
		port := ports[0]
		if hint.ifIndex == 0 && port.ifIndex > 0 {
			hint.ifIndex = port.ifIndex
		}
		if strings.TrimSpace(hint.ifName) == "" && strings.TrimSpace(port.ifName) != "" {
			hint.ifName = strings.TrimSpace(port.ifName)
		}
	}
	if hint.ifIndex == 0 {
		for _, value := range labelsCSVToSlice(endpointLabels, "learned_if_indexes") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if n, err := strconv.Atoi(value); err == nil && n > 0 {
				hint.ifIndex = n
				break
			}
		}
	}
	if strings.TrimSpace(hint.ifName) == "" {
		for _, value := range labelsCSVToSlice(endpointLabels, "learned_if_names") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			hint.ifName = value
			break
		}
	}
	if hint.ifIndex == 0 && strings.TrimSpace(hint.ifName) == "" {
		hint.ifName = "0"
	}
	return hint
}

func filterManagedDeviceHints(hints []string, managedDeviceIDs map[string]struct{}) []string {
	if len(hints) == 0 || len(managedDeviceIDs) == 0 {
		return hints
	}
	managed := make([]string, 0, len(hints))
	for _, hint := range hints {
		hint = strings.TrimSpace(hint)
		if hint == "" {
			continue
		}
		if _, ok := managedDeviceIDs[hint]; ok {
			managed = append(managed, hint)
		}
	}
	if len(managed) == 0 {
		return hints
	}
	managed = uniqueTopologyStrings(managed)
	sort.Strings(managed)
	return managed
}

func topologyEndpointLabelDeviceIDs(labels map[string]string) []string {
	out := labelsCSVToSlice(labels, "learned_device_ids")
	if len(out) == 0 {
		out = labelsCSVToSlice(labels, "device_ids")
	}
	out = uniqueTopologyStrings(out)
	sort.Strings(out)
	return out
}

func resolveTopologyEndpointDeviceHints(
	hints []string,
	aliasOwnerIDs map[string]map[string]struct{},
) []string {
	set := make(map[string]struct{})
	for _, hint := range hints {
		hint = strings.TrimSpace(hint)
		if hint == "" {
			continue
		}
		if alias := normalizeTopologyEndpointDeviceAlias(hint); alias != "" {
			if owners := aliasOwnerIDs[alias]; len(owners) > 0 {
				for ownerID := range owners {
					ownerID = strings.TrimSpace(ownerID)
					if ownerID == "" {
						continue
					}
					set[ownerID] = struct{}{}
				}
				continue
			}
		}
		set[hint] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := sortedTopologySet(set)
	return out
}

func normalizeTopologyEndpointDeviceAlias(hint string) string {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		return ""
	}
	if alias := normalizeFDBEndpointID(hint); alias != "" {
		return alias
	}
	lower := strings.ToLower(hint)
	if strings.HasPrefix(lower, "macaddress:") {
		if mac := normalizeMAC(hint[len("macAddress:"):]); mac != "" {
			return "mac:" + mac
		}
	}
	return ""
}

func ensureProbablePortlessSegment(
	segmentByID map[string]*bridgeDomainSegment,
	hint probableEndpointReporterHint,
) (string, bool) {
	deviceID := strings.TrimSpace(hint.deviceID)
	if deviceID == "" {
		return "", false
	}
	port := bridgePortRef{
		deviceID: deviceID,
		ifIndex:  hint.ifIndex,
		ifName:   strings.TrimSpace(hint.ifName),
	}
	if port.ifIndex == 0 && strings.TrimSpace(port.ifName) == "" {
		port.ifName = "0"
	}

	segmentID := "bridge-domain:probable:" + bridgePortRefSortKey(port)
	if _, ok := segmentByID[segmentID]; ok {
		return segmentID, false
	}
	segment := newBridgeDomainSegment(port)
	segment.methods["probable"] = struct{}{}
	segmentByID[segmentID] = segment
	return segmentID, true
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

func inferFDBPairwiseBridgeLinks(
	attachments []Attachment,
	ifaceByDeviceIndex map[string]Interface,
	reporterAliases map[string][]string,
) []bridgeBridgeLinkRecord {
	if len(attachments) == 0 || len(reporterAliases) == 0 {
		return nil
	}

	aliasOwnerIDs := buildFDBAliasOwnerMap(reporterAliases)
	if len(aliasOwnerIDs) == 0 {
		return nil
	}

	// reporterA -> reporterB -> unique reporter ports where A learns aliases of B.
	pairs := make(map[string]map[string]map[string]bridgePortRef)
	for _, attachment := range attachments {
		if !strings.EqualFold(strings.TrimSpace(attachment.Method), "fdb") {
			continue
		}
		reporterID := strings.TrimSpace(attachment.DeviceID)
		if reporterID == "" {
			continue
		}
		endpointID := normalizeFDBEndpointID(attachment.EndpointID)
		if endpointID == "" {
			continue
		}
		owners := aliasOwnerIDs[endpointID]
		if len(owners) == 0 {
			continue
		}
		port := bridgePortFromAttachment(attachment, ifaceByDeviceIndex)
		portKey := bridgePortObservationKey(port)
		if portKey == "" {
			continue
		}
		for ownerID := range owners {
			ownerID = strings.TrimSpace(ownerID)
			if ownerID == "" || strings.EqualFold(ownerID, reporterID) {
				continue
			}
			byPeer := pairs[reporterID]
			if byPeer == nil {
				byPeer = make(map[string]map[string]bridgePortRef)
				pairs[reporterID] = byPeer
			}
			ports := byPeer[ownerID]
			if ports == nil {
				ports = make(map[string]bridgePortRef)
				byPeer[ownerID] = ports
			}
			ports[portKey] = port
		}
	}
	if len(pairs) == 0 {
		return nil
	}

	records := make([]bridgeBridgeLinkRecord, 0)
	seen := make(map[string]struct{})
	leftIDs := make([]string, 0, len(pairs))
	for leftID := range pairs {
		leftIDs = append(leftIDs, leftID)
	}
	sort.Strings(leftIDs)
	for _, leftID := range leftIDs {
		neighbors := pairs[leftID]
		if len(neighbors) == 0 {
			continue
		}
		rightIDs := make([]string, 0, len(neighbors))
		for rightID := range neighbors {
			rightIDs = append(rightIDs, rightID)
		}
		sort.Strings(rightIDs)
		for _, rightID := range rightIDs {
			if leftID >= rightID {
				continue
			}
			leftPorts := pairs[leftID][rightID]
			rightPorts := pairs[rightID][leftID]
			if len(leftPorts) != 1 || len(rightPorts) != 1 {
				// Conservative rule: infer direct bridge link only when each side reports
				// exactly one reciprocal managed-alias learning port.
				continue
			}
			leftPort := firstSortedBridgePort(leftPorts)
			rightPort := firstSortedBridgePort(rightPorts)
			if bridgePortObservationKey(leftPort) == "" || bridgePortObservationKey(rightPort) == "" {
				continue
			}
			key := bridgePairKey(leftPort, rightPort)
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}

			designated := leftPort
			other := rightPort
			if bridgePortRefSortKey(leftPort) > bridgePortRefSortKey(rightPort) {
				designated = rightPort
				other = leftPort
			}
			records = append(records, bridgeBridgeLinkRecord{
				port:           other,
				designatedPort: designated,
				method:         "fdb_pairwise",
			})
		}
	}
	if len(records) == 0 {
		return nil
	}
	sort.SliceStable(records, func(i, j int) bool {
		li := portSortKey(records[i].designatedPort) + "|" + portSortKey(records[i].port)
		lj := portSortKey(records[j].designatedPort) + "|" + portSortKey(records[j].port)
		return li < lj
	})
	return records
}

func firstSortedBridgePort(ports map[string]bridgePortRef) bridgePortRef {
	if len(ports) == 0 {
		return bridgePortRef{}
	}
	keys := make([]string, 0, len(ports))
	for key := range ports {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return ports[keys[0]]
}

type fdbPortEndpointSet struct {
	port      bridgePortRef
	endpoints map[string]struct{}
}

type fdbOverlapCandidate struct {
	leftKey   string
	rightKey  string
	leftPort  bridgePortRef
	rightPort bridgePortRef
	shared    int
	score     float64
}

func inferFDBOverlapWeightedBridgeLinks(
	attachments []Attachment,
	ifaceByDeviceIndex map[string]Interface,
	reporterAliases map[string][]string,
	minShared int,
) []bridgeBridgeLinkRecord {
	if len(attachments) == 0 {
		return nil
	}
	if minShared <= 0 {
		minShared = 1
	}

	managedAliasSet := make(map[string]struct{})
	for endpointID := range buildFDBAliasOwnerMap(reporterAliases) {
		managedAliasSet[endpointID] = struct{}{}
	}

	portEndpointsByDevice := make(map[string]map[string]*fdbPortEndpointSet)
	for _, attachment := range attachments {
		if !strings.EqualFold(strings.TrimSpace(attachment.Method), "fdb") {
			continue
		}
		reporterID := strings.TrimSpace(attachment.DeviceID)
		if reporterID == "" {
			continue
		}
		endpointID := normalizeFDBEndpointID(attachment.EndpointID)
		if endpointID == "" {
			continue
		}
		// Overlap mode focuses on host-side FDB overlap and excludes managed
		// aliases that are already covered by direct reciprocal pairwise rules.
		if _, isManagedAlias := managedAliasSet[endpointID]; isManagedAlias {
			continue
		}
		port := bridgePortFromAttachment(attachment, ifaceByDeviceIndex)
		portKey := bridgePortObservationKey(port)
		if portKey == "" {
			continue
		}

		byPort := portEndpointsByDevice[reporterID]
		if byPort == nil {
			byPort = make(map[string]*fdbPortEndpointSet)
			portEndpointsByDevice[reporterID] = byPort
		}
		set := byPort[portKey]
		if set == nil {
			set = &fdbPortEndpointSet{
				port:      port,
				endpoints: make(map[string]struct{}),
			}
			byPort[portKey] = set
		}
		set.endpoints[endpointID] = struct{}{}
	}
	if len(portEndpointsByDevice) < 2 {
		return nil
	}

	deviceIDs := make([]string, 0, len(portEndpointsByDevice))
	for deviceID := range portEndpointsByDevice {
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Strings(deviceIDs)

	records := make([]bridgeBridgeLinkRecord, 0)
	seen := make(map[string]struct{})
	for leftIdx := 0; leftIdx < len(deviceIDs); leftIdx++ {
		leftID := deviceIDs[leftIdx]
		leftPorts := portEndpointsByDevice[leftID]
		if len(leftPorts) == 0 {
			continue
		}
		for rightIdx := leftIdx + 1; rightIdx < len(deviceIDs); rightIdx++ {
			rightID := deviceIDs[rightIdx]
			rightPorts := portEndpointsByDevice[rightID]
			if len(rightPorts) == 0 {
				continue
			}

			candidates := make([]fdbOverlapCandidate, 0)
			for leftKey, leftSet := range leftPorts {
				if len(leftSet.endpoints) < minShared {
					continue
				}
				for rightKey, rightSet := range rightPorts {
					if len(rightSet.endpoints) < minShared {
						continue
					}
					shared := overlapSize(leftSet.endpoints, rightSet.endpoints)
					if shared < minShared {
						continue
					}
					union := len(leftSet.endpoints) + len(rightSet.endpoints) - shared
					if union <= 0 {
						continue
					}
					score := float64(shared) + (float64(shared) / float64(union))
					candidates = append(candidates, fdbOverlapCandidate{
						leftKey:   leftKey,
						rightKey:  rightKey,
						leftPort:  leftSet.port,
						rightPort: rightSet.port,
						shared:    shared,
						score:     score,
					})
				}
			}
			if len(candidates) == 0 {
				continue
			}

			sort.SliceStable(candidates, func(i, j int) bool {
				if candidates[i].score != candidates[j].score {
					return candidates[i].score > candidates[j].score
				}
				if candidates[i].shared != candidates[j].shared {
					return candidates[i].shared > candidates[j].shared
				}
				iKey := candidates[i].leftKey + "|" + candidates[i].rightKey
				jKey := candidates[j].leftKey + "|" + candidates[j].rightKey
				return iKey < jKey
			})

			usedLeft := make(map[string]struct{})
			usedRight := make(map[string]struct{})
			for _, candidate := range candidates {
				if _, ok := usedLeft[candidate.leftKey]; ok {
					continue
				}
				if _, ok := usedRight[candidate.rightKey]; ok {
					continue
				}
				usedLeft[candidate.leftKey] = struct{}{}
				usedRight[candidate.rightKey] = struct{}{}

				designated := candidate.leftPort
				other := candidate.rightPort
				if bridgePortRefSortKey(designated) > bridgePortRefSortKey(other) {
					designated = candidate.rightPort
					other = candidate.leftPort
				}
				key := bridgePairKey(designated, other)
				if key == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				records = append(records, bridgeBridgeLinkRecord{
					port:           other,
					designatedPort: designated,
					method:         topologyInferenceStrategyFDBOverlapWeighted,
				})

				// Conservative default: one overlap-inferred bridge pair per
				// device pair to avoid over-connecting noisy FDB domains.
				break
			}
		}
	}
	if len(records) == 0 {
		return nil
	}
	sort.SliceStable(records, func(i, j int) bool {
		li := portSortKey(records[i].designatedPort) + "|" + portSortKey(records[i].port)
		lj := portSortKey(records[j].designatedPort) + "|" + portSortKey(records[j].port)
		return li < lj
	})
	return records
}

func overlapSize(left, right map[string]struct{}) int {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	if len(left) > len(right) {
		left, right = right, left
	}
	count := 0
	for value := range left {
		if _, ok := right[value]; ok {
			count++
		}
	}
	return count
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

func collapseActorsByIP(actors []topology.Actor) []topology.Actor {
	if len(actors) <= 1 {
		return actors
	}

	parent := make([]int, len(actors))
	for i := range parent {
		parent[i] = i
	}
	find := func(x int) int {
		for parent[x] != x {
			parent[x] = parent[parent[x]]
			x = parent[x]
		}
		return x
	}
	union := func(a, b int) {
		ra := find(a)
		rb := find(b)
		if ra == rb {
			return
		}
		if ra < rb {
			parent[rb] = ra
			return
		}
		parent[ra] = rb
	}

	ipOwner := make(map[string]int)
	for idx, actor := range actors {
		if strings.EqualFold(strings.TrimSpace(actor.ActorType), "segment") {
			continue
		}
		ips := normalizedTopologyActorIPs(actor)
		if len(ips) == 0 {
			continue
		}
		for _, ip := range ips {
			if owner, ok := ipOwner[ip]; ok {
				union(idx, owner)
				continue
			}
			ipOwner[ip] = idx
		}
	}

	groups := make(map[int][]int)
	for idx := range actors {
		root := find(idx)
		groups[root] = append(groups[root], idx)
	}

	keep := make([]bool, len(actors))
	for i := range keep {
		keep[i] = true
	}
	for _, members := range groups {
		if len(members) <= 1 {
			continue
		}
		rep := members[0]
		for _, idx := range members[1:] {
			if compareTopologyActorCollapsePriority(actors[idx], actors[rep]) < 0 {
				rep = idx
			}
		}
		merged := actors[rep]
		collapsedCount := 1
		for _, idx := range members {
			if idx == rep {
				continue
			}
			collapsedCount++
			merged.Match = mergeTopologyActorMatch(merged.Match, actors[idx].Match)
			merged.Labels = mergeTopologyActorLabels(merged.Labels, actors[idx].Labels)
			merged.Attributes = mergeTopologyActorAttributes(merged.Attributes, actors[idx].Attributes)
			keep[idx] = false
		}
		if collapsedCount > 1 {
			if merged.Attributes == nil {
				merged.Attributes = make(map[string]any)
			}
			merged.Attributes["collapsed_by_ip"] = true
			merged.Attributes["collapsed_count"] = collapsedCount
		}
		actors[rep] = merged
	}

	out := make([]topology.Actor, 0, len(actors))
	for idx, actor := range actors {
		if !keep[idx] {
			continue
		}
		out = append(out, actor)
	}
	return out
}

func eliminateNonIPInferredActors(actors []topology.Actor, links []topology.Link) ([]topology.Actor, []topology.Link) {
	if len(actors) == 0 {
		return actors, links
	}
	removedIdentityKeys := make(map[string]struct{})
	filteredActors := make([]topology.Actor, 0, len(actors))
	for _, actor := range actors {
		if topologyActorIsInferred(actor) && len(normalizedTopologyActorIPs(actor)) == 0 {
			for _, key := range topologyMatchIdentityKeys(actor.Match) {
				removedIdentityKeys[key] = struct{}{}
			}
			continue
		}
		filteredActors = append(filteredActors, actor)
	}
	if len(removedIdentityKeys) == 0 {
		return actors, links
	}

	filteredLinks := make([]topology.Link, 0, len(links))
	for _, link := range links {
		srcKeys := topologyMatchIdentityKeys(link.Src.Match)
		dstKeys := topologyMatchIdentityKeys(link.Dst.Match)
		if topologyIdentityKeysOverlap(srcKeys, removedIdentityKeys) {
			continue
		}
		if topologyIdentityKeysOverlap(dstKeys, removedIdentityKeys) {
			continue
		}
		filteredLinks = append(filteredLinks, link)
	}
	return filteredActors, filteredLinks
}

func topologyIdentityKeysOverlap(keys []string, set map[string]struct{}) bool {
	if len(keys) == 0 || len(set) == 0 {
		return false
	}
	for _, key := range keys {
		if _, ok := set[key]; ok {
			return true
		}
	}
	return false
}

func normalizedTopologyActorIPs(actor topology.Actor) []string {
	if len(actor.Match.IPAddresses) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(actor.Match.IPAddresses))
	out := make([]string, 0, len(actor.Match.IPAddresses))
	for _, value := range actor.Match.IPAddresses {
		ip := normalizeTopologyIP(value)
		if ip == "" {
			continue
		}
		if _, ok := seen[ip]; ok {
			continue
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}
	sort.Strings(out)
	return out
}

func compareTopologyActorCollapsePriority(left, right topology.Actor) int {
	leftDevice := strings.EqualFold(strings.TrimSpace(left.ActorType), "device")
	rightDevice := strings.EqualFold(strings.TrimSpace(right.ActorType), "device")
	if leftDevice != rightDevice {
		if leftDevice {
			return -1
		}
		return 1
	}
	leftInferred := topologyActorIsInferred(left)
	rightInferred := topologyActorIsInferred(right)
	if leftInferred != rightInferred {
		if !leftInferred {
			return -1
		}
		return 1
	}
	leftKey := canonicalTopologyMatchKey(left.Match)
	rightKey := canonicalTopologyMatchKey(right.Match)
	return strings.Compare(leftKey, rightKey)
}

func mergeTopologyActorMatch(base, other topology.Match) topology.Match {
	base.ChassisIDs = mergeTopologyStringLists(base.ChassisIDs, other.ChassisIDs)
	base.MacAddresses = mergeTopologyStringLists(base.MacAddresses, other.MacAddresses)
	base.IPAddresses = mergeTopologyStringLists(base.IPAddresses, other.IPAddresses)
	base.Hostnames = mergeTopologyStringLists(base.Hostnames, other.Hostnames)
	base.DNSNames = mergeTopologyStringLists(base.DNSNames, other.DNSNames)
	if strings.TrimSpace(base.SysName) == "" {
		base.SysName = strings.TrimSpace(other.SysName)
	}
	if strings.TrimSpace(base.SysObjectID) == "" {
		base.SysObjectID = strings.TrimSpace(other.SysObjectID)
	}
	return base
}

func mergeTopologyStringLists(base []string, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, value := range append(base, extra...) {
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

func mergeTopologyActorLabels(base, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]string, len(extra))
	}
	for key, value := range extra {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		if _, exists := base[key]; exists {
			continue
		}
		base[key] = value
	}
	return base
}

func mergeTopologyActorAttributes(base, extra map[string]any) map[string]any {
	if len(extra) == 0 {
		return base
	}
	if base == nil {
		base = make(map[string]any, len(extra))
	}
	for key, value := range extra {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := base[key]; exists {
			continue
		}
		base[key] = value
	}
	return base
}

func topologyActorIsInferred(actor topology.Actor) bool {
	if strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
		return true
	}
	if topologyAnyBoolValue(actor.Attributes["inferred"]) {
		return true
	}
	if len(actor.Labels) > 0 {
		if topologyAnyBoolValue(actor.Labels["inferred"]) {
			return true
		}
	}
	return false
}

func topologyAnyBoolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
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

	discoveryPairs := make(map[string]struct{})
	for _, link := range links {
		protocol := strings.ToLower(strings.TrimSpace(link.Protocol))
		if protocol != "lldp" && protocol != "cdp" {
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
			discoveryPairs[pair] = struct{}{}
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
						if _, found := discoveryPairs[pair]; found {
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

func pruneManagedOverlapUnlinkedEndpointActors(
	actors []topology.Actor,
	links []topology.Link,
	suppressedEndpointIDs map[string]struct{},
) ([]topology.Actor, int) {
	if len(actors) == 0 || len(suppressedEndpointIDs) == 0 {
		return actors, 0
	}

	suppressedIdentityKeys := make(map[string]struct{})
	for endpointID := range suppressedEndpointIDs {
		endpointID = normalizeFDBEndpointID(endpointID)
		if endpointID == "" {
			continue
		}
		match := endpointMatchFromID(endpointID)
		for _, key := range topologyMatchIdentityKeys(match) {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			suppressedIdentityKeys[key] = struct{}{}
		}
	}
	if len(suppressedIdentityKeys) == 0 {
		return actors, 0
	}

	linkedIdentityKeys := make(map[string]struct{}, len(links)*2)
	for _, link := range links {
		for _, key := range topologyMatchIdentityKeys(link.Src.Match) {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			linkedIdentityKeys[key] = struct{}{}
		}
		for _, key := range topologyMatchIdentityKeys(link.Dst.Match) {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			linkedIdentityKeys[key] = struct{}{}
		}
	}

	filtered := make([]topology.Actor, 0, len(actors))
	suppressedCount := 0
	for _, actor := range actors {
		if !strings.EqualFold(strings.TrimSpace(actor.ActorType), "endpoint") {
			filtered = append(filtered, actor)
			continue
		}

		actorKeys := topologyMatchIdentityKeys(actor.Match)
		if !topologyIdentityKeysOverlap(actorKeys, suppressedIdentityKeys) {
			filtered = append(filtered, actor)
			continue
		}
		// Keep endpoint actors that still participate in at least one emitted link.
		if topologyIdentityKeysOverlap(actorKeys, linkedIdentityKeys) {
			filtered = append(filtered, actor)
			continue
		}
		suppressedCount++
	}
	return filtered, suppressedCount
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
	ifIndex := resolveIfIndexByPortName(deviceID, port, ifIndexByDeviceName)
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
		ifIndex = resolveIfIndexByPortName(adj.SourceID, ifName, ifIndexByDeviceName)
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
	ifName := ""
	ifDescr := ""
	ifIndex := 0
	var iface Interface
	hasIface := false
	if port != "" {
		ifIndex = resolveIfIndexByPortName(dev.ID, port, ifIndexByDeviceName)
	}
	if ifIndex > 0 {
		if ifaceValue, ok := ifaceByDeviceIndex[deviceIfIndexKey(dev.ID, ifIndex)]; ok {
			iface = ifaceValue
			hasIface = true
			ifName = strings.TrimSpace(iface.IfName)
			ifDescr = strings.TrimSpace(iface.IfDescr)
		}
	}
	if ifName == "" {
		ifName = ifDescr
	}
	if ifName == "" {
		ifName = port
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
	if ifDescr != "" {
		attrs["if_descr"] = ifDescr
	}
	if ifIndex > 0 && hasIface {
		if admin := strings.TrimSpace(iface.Labels["admin_status"]); admin != "" {
			attrs["if_admin_status"] = admin
		}
		if oper := strings.TrimSpace(iface.Labels["oper_status"]); oper != "" {
			attrs["if_oper_status"] = oper
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
		if deviceID := strings.TrimSpace(attachment.DeviceID); deviceID != "" {
			acc.deviceIDs[deviceID] = struct{}{}
		}
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
		for _, deviceID := range csvToSet(enrichment.Labels["device_ids"]) {
			deviceID = strings.TrimSpace(deviceID)
			if deviceID == "" {
				continue
			}
			acc.deviceIDs[deviceID] = struct{}{}
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
			"learned_sources":    strings.Join(sortedTopologySet(acc.sources), ","),
			"learned_device_ids": strings.Join(sortedTopologySet(acc.deviceIDs), ","),
			"learned_if_indexes": strings.Join(sortedTopologySet(acc.ifIndexes), ","),
			"learned_if_names":   strings.Join(sortedTopologySet(acc.ifNames), ","),
		}

		attrs := map[string]any{
			"discovered":         true,
			"learned_sources":    sortedTopologySet(acc.sources),
			"learned_device_ids": sortedTopologySet(acc.deviceIDs),
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
		deviceIDs:  make(map[string]struct{}),
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

func interfaceNameLookupAliases(values ...string) []string {
	set := make(map[string]struct{}, len(values)*2)
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
		if normalized := normalizeInterfaceNameForLookup(trimmed); normalized != "" && normalized != strings.ToLower(trimmed) {
			set[normalized] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeInterfaceNameForLookup(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "", ".", "", "\t", "", "\n", "", "\r", "")
	return replacer.Replace(value)
}

func resolveIfIndexByPortName(deviceID, port string, ifIndexByDeviceName map[string]int) int {
	deviceID = strings.TrimSpace(deviceID)
	port = strings.TrimSpace(port)
	if deviceID == "" || port == "" {
		return 0
	}
	if idx, ok := ifIndexByDeviceName[deviceIfNameKey(deviceID, port)]; ok && idx > 0 {
		return idx
	}
	if normalized := normalizeInterfaceNameForLookup(port); normalized != "" {
		if idx, ok := ifIndexByDeviceName[deviceIfNameKey(deviceID, normalized)]; ok && idx > 0 {
			return idx
		}
	}
	if parsed, err := strconv.Atoi(port); err == nil && parsed > 0 {
		return parsed
	}
	return 0
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
		return ""
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
	return ""
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
		srcPortName = "[unset]"
	}
	dstPortName = strings.TrimSpace(dstPortName)
	if dstPortName == "" {
		dstPortName = "[unset]"
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
