// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"net/netip"
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
)

type topologyInferenceStrategyConfig struct {
	id                               string
	includeLLDPBridgeLinks           bool
	includeCDPBridgeLinks            bool
	includeSTPBridgeLinks            bool
	useSTPDesignatedParent           bool
	enableFDBPairwiseLinks           bool
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
	IfDescr        string
	IfAlias        string
	MAC            string
	SpeedBps       int64
	LastChange     int64
	Duplex         string
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
	FDBMACCount    int
	STPState       string
	VLANs          []map[string]any
	Neighbors      []topologyPortNeighborStatus
}

type topologyPortNeighborStatus struct {
	Protocol           string
	RemoteDevice       string
	RemotePort         string
	RemoteIP           string
	RemoteChassisID    string
	RemoteCapabilities []string
}

type topologyDeviceInterfaceSummary struct {
	portsTotal        int
	ifIndexes         []string
	ifNames           []string
	adminStatusCount  map[string]any
	operStatusCount   map[string]any
	linkModeCount     map[string]any
	roleCount         map[string]any
	portsUp           int
	portsDown         int
	portsAdminDown    int
	totalBandwidthBps int64
	fdbTotalMACs      int
	vlanCount         int
	lldpNeighborCount int
	cdpNeighborCount  int
	portStatuses      []map[string]any
}

type topologyDevicePortEvidence struct {
	vlanIDs            map[string]struct{}
	vlanNames          map[string]string
	fdbEndpointIDs     map[string]struct{}
	hasFDB             bool
	hasFDBManagedAlias bool
	hasSTP             bool
	hasPeer            bool
	hasBridgeLink      bool
	isLAG              bool
	stpStates          map[string]struct{}
	neighbors          map[string]topologyPortNeighborStatus
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
			enableSTPManagedAliasCorrelation: true,
			filterSwitchFacingAttachments:    true,
		}
	case topologyInferenceStrategyCDPFDBHybrid:
		return topologyInferenceStrategyConfig{
			id:                            topologyInferenceStrategyCDPFDBHybrid,
			includeCDPBridgeLinks:         true,
			enableFDBPairwiseLinks:        true,
			filterSwitchFacingAttachments: true,
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
		deviceByID,
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
	enrichTopologyPortTablesWithLinkCounts(actors, links)
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
