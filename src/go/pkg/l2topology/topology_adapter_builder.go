// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

type graphBuilder struct {
	result Result
	opts   GraphOptions

	schemaVersion string
	source        string
	layer         string
	view          string
	collectedAt   time.Time

	strategyConfig topologyInferenceStrategyConfig

	deviceByID           map[string]Device
	ifaceByDeviceIndex   map[string]Interface
	ifIndexByDeviceName  map[string]int
	bridgeLinks          []bridgeBridgeLinkRecord
	reporterAliases      map[string][]string
	ifaceSummaryByDevice map[string]topologyDeviceInterfaceSummary

	actors        []projectedActor
	actorIndex    map[string]struct{}
	actorMACIndex map[string]struct{}

	projectedAdjacencies projectedLinks
	endpointActors       builtEndpointActors
	segmentProjection    projectedSegments

	links              []graph.Link
	segmentSuppressed  int
	unlinkedSuppressed int
	linkCounts         topologyLinkCounts
	probableLinks      int
	stats              ProjectionStats
}

func newGraphBuilder(result Result, opts GraphOptions) *graphBuilder {
	builder := &graphBuilder{
		result: result,
		opts:   opts,
	}

	builder.schemaVersion = strings.TrimSpace(opts.SchemaVersion)
	if builder.schemaVersion == "" {
		builder.schemaVersion = "2.0"
	}

	builder.source = strings.TrimSpace(opts.Source)
	if builder.source == "" {
		builder.source = "snmp"
	}

	builder.layer = strings.TrimSpace(opts.Layer)
	if builder.layer == "" {
		builder.layer = "2"
	}

	builder.view = strings.TrimSpace(opts.View)
	if builder.view == "" {
		builder.view = "summary"
	}

	builder.collectedAt = opts.CollectedAt
	if builder.collectedAt.IsZero() {
		builder.collectedAt = result.CollectedAt
	}
	if builder.collectedAt.IsZero() {
		builder.collectedAt = time.Now().UTC()
	}

	builder.strategyConfig = topologyInferenceStrategyConfigFor(opts.InferenceStrategy)
	return builder
}

func (b *graphBuilder) prepareIndexes() {
	b.deviceByID = make(map[string]Device, len(b.result.Devices))
	b.ifaceByDeviceIndex = make(map[string]Interface, len(b.result.Interfaces))
	b.ifIndexByDeviceName = make(map[string]int, len(b.result.Interfaces))

	for _, dev := range b.result.Devices {
		b.deviceByID[dev.ID] = dev
	}

	for _, iface := range b.result.Interfaces {
		if iface.IfIndex <= 0 {
			continue
		}
		b.ifaceByDeviceIndex[deviceIfIndexKey(iface.DeviceID, iface.IfIndex)] = iface
		for _, alias := range interfaceNameLookupAliases(iface.IfName, iface.IfDescr) {
			b.ifIndexByDeviceName[deviceIfNameKey(iface.DeviceID, alias)] = iface.IfIndex
		}
	}
}

func (b *graphBuilder) collectBridgeTopologyInputs() {
	b.bridgeLinks = collectBridgeLinkRecords(b.result.Adjacencies, b.ifIndexByDeviceName, b.strategyConfig)
	b.reporterAliases = buildFDBReporterAliases(b.deviceByID, b.ifaceByDeviceIndex)
	if b.strategyConfig.enableFDBPairwiseLinks {
		b.bridgeLinks = mergeBridgeLinkRecordSets(
			b.bridgeLinks,
			inferFDBPairwiseBridgeLinks(b.result.Attachments, b.ifaceByDeviceIndex, b.reporterAliases),
		)
	}

	deterministicTransitPortKeys := buildDeterministicTransitPortKeySet(b.result.Adjacencies, b.ifIndexByDeviceName)
	discoveryDevicePairs := buildDeterministicDiscoveryDevicePairSet(b.result.Adjacencies)
	b.bridgeLinks = suppressInferredBridgeLinksOnDeterministicDiscovery(
		b.bridgeLinks,
		deterministicTransitPortKeys,
		discoveryDevicePairs,
	)

	b.ifaceSummaryByDevice = buildTopologyDeviceInterfaceSummaries(
		b.result.Interfaces,
		b.result.Attachments,
		b.result.Adjacencies,
		b.deviceByID,
		b.ifIndexByDeviceName,
		b.bridgeLinks,
		b.reporterAliases,
	)
}

func (b *graphBuilder) buildDeviceActors() {
	b.actors = make([]projectedActor, 0, len(b.result.Devices))
	b.actorIndex = make(map[string]struct{}, len(b.result.Devices)*2)
	b.actorMACIndex = make(map[string]struct{}, len(b.result.Devices))

	for _, dev := range b.result.Devices {
		actor := deviceToTopologyActor(
			dev,
			b.source,
			b.layer,
			b.opts.LocalDeviceID,
			b.ifaceSummaryByDevice[dev.ID],
			b.reporterAliases[dev.ID],
		)
		keys := topologyMatchIdentityKeys(actor.Actor.Match)
		if len(keys) == 0 {
			continue
		}
		macKeys := topologyMatchHardwareIdentityKeys(actor.Actor.Match)
		if len(macKeys) > 0 {
			if topologyIdentityIndexOverlaps(b.actorMACIndex, macKeys) {
				continue
			}
			addTopologyIdentityKeys(b.actorMACIndex, macKeys)
		} else if topologyIdentityIndexOverlaps(b.actorIndex, keys) {
			continue
		}
		addTopologyIdentityKeys(b.actorIndex, keys)
		b.actors = append(b.actors, actor)
	}
}

func (b *graphBuilder) projectAdjacencyTopology() {
	b.projectedAdjacencies = projectAdjacencyLinks(
		b.result.Adjacencies,
		b.layer,
		b.collectedAt,
		b.deviceByID,
		b.ifIndexByDeviceName,
		b.ifaceByDeviceIndex,
	)
}

func (b *graphBuilder) buildEndpointTopology() {
	b.endpointActors = buildEndpointActors(
		b.result.Attachments,
		b.result.Enrichments,
		b.ifaceByDeviceIndex,
		b.source,
		b.layer,
		b.actorIndex,
		b.actorMACIndex,
	)
	b.actors = append(b.actors, b.endpointActors.actors...)
}

func (b *graphBuilder) buildSegmentTopology() {
	b.segmentProjection = projectSegmentTopology(
		b.result.Attachments,
		b.result.Adjacencies,
		b.layer,
		b.source,
		b.collectedAt,
		b.deviceByID,
		b.ifaceByDeviceIndex,
		b.ifIndexByDeviceName,
		b.bridgeLinks,
		b.reporterAliases,
		b.endpointActors.matchByEndpointID,
		b.endpointActors.labelsByEndpointID,
		b.actorIndex,
		b.opts.ProbabilisticConnectivity,
		b.strategyConfig,
	)
	annotateEndpointActorsWithDirectOwners(
		b.actors,
		b.endpointActors.matchByEndpointID,
		b.segmentProjection.endpointDirectOwners,
		b.deviceByID,
	)
	b.actors = append(b.actors, b.segmentProjection.actors...)
}

func (b *graphBuilder) finalizeGraph() {
	sortProjectedTopologyActors(b.actors)

	b.links = make([]graph.Link, 0, len(b.projectedAdjacencies.links)+len(b.segmentProjection.links))
	b.links = append(b.links, b.projectedAdjacencies.links...)
	b.links = append(b.links, b.segmentProjection.links...)
	sortTopologyLinks(b.links)

	b.actors, b.links, b.segmentSuppressed = pruneSegmentArtifacts(b.actors, b.links)
	if b.opts.CollapseActorsByIP {
		b.actors = collapseActorsByIP(b.actors)
	}
	if b.opts.EliminateNonIPInferred {
		b.actors, b.links = eliminateNonIPInferredActors(b.actors, b.links)
	}
	if b.opts.CollapseActorsByIP {
		b.actors, b.unlinkedSuppressed = pruneManagedOverlapUnlinkedEndpointActors(
			b.actors,
			b.links,
			b.segmentProjection.suppressedManagedOverlapIDs,
		)
	}
	var additionalSegmentSuppressed int
	b.actors, b.links, additionalSegmentSuppressed = pruneSegmentArtifacts(b.actors, b.links)
	b.segmentSuppressed += additionalSegmentSuppressed
	sortProjectedTopologyActors(b.actors)
	sortTopologyLinks(b.links)
	applyTopologyDisplayNames(b.actors, b.links, b.opts.ResolveDNSName)
	assignTopologyActorIDsAndLinkEndpoints(b.actors, b.links)
	enrichTopologyPortDetailsWithLinkCounts(b.actors, b.links)

	b.linkCounts = summarizeTopologyLinks(b.links)
	b.probableLinks = 0
	for _, link := range b.links {
		if strings.EqualFold(strings.TrimSpace(link.State), "probable") {
			b.probableLinks++
			continue
		}
		if strings.EqualFold(topologyLinkInference(link), "probable") {
			b.probableLinks++
		}
	}
}

func (b *graphBuilder) buildStats() {
	b.stats = ProjectionStats{
		ResultStats: b.result.Stats,

		DevicesDiscovered:          discoveredDeviceCount(b.result.Devices, b.opts.LocalDeviceID),
		LinksBidirectional:         b.linkCounts.bidirectional,
		LinksUnidirectional:        b.linkCounts.unidirectional,
		LinksFDB:                   b.linkCounts.fdb,
		LinksFDBEndpointCandidates: b.segmentProjection.endpointLinksCandidates,
		LinksFDBEndpointEmitted:    b.segmentProjection.endpointLinksEmitted,
		LinksFDBEndpointSuppressed: b.segmentProjection.endpointLinksSuppressed,
		EndpointsAmbiguousSegments: b.segmentProjection.endpointsWithAmbiguousSegment,
		LinksARP:                   b.linkCounts.arp,
		LinksProbable:              b.probableLinks,
		SegmentsSuppressed:         b.segmentSuppressed,
		ActorsTotal:                len(b.actors),
		ActorsUnlinkedSuppressed:   b.unlinkedSuppressed,
		InferenceStrategy:          b.strategyConfig.id,
	}
	b.stats.DevicesTotal = len(b.result.Devices)
	b.stats.LinksTotal = len(b.links)
	b.stats.LinksLLDP = b.linkCounts.lldp
	b.stats.LinksCDP = b.linkCounts.cdp
	b.stats.EndpointsTotal = b.endpointActors.count
}

func (b *graphBuilder) graph() graph.Graph {
	return graph.Graph{
		SchemaVersion: b.schemaVersion,
		Source:        b.source,
		Layer:         b.layer,
		AgentID:       b.opts.AgentID,
		CollectedAt:   b.collectedAt,
		View:          b.view,
		Actors:        graphActorsFromProjected(b.actors),
		Links:         b.links,
	}
}

func (b *graphBuilder) actorDetails() map[string]ProjectionActorDetail {
	if len(b.actors) == 0 {
		return nil
	}
	out := make(map[string]ProjectionActorDetail, len(b.actors))
	for _, actor := range b.actors {
		actorID := strings.TrimSpace(actor.Actor.ActorID)
		if actorID == "" {
			continue
		}
		out[actorID] = actor.Detail
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func graphActorsFromProjected(actors []projectedActor) []graph.Actor {
	if len(actors) == 0 {
		return nil
	}
	out := make([]graph.Actor, len(actors))
	for i, actor := range actors {
		out[i] = actor.Actor
	}
	return out
}
