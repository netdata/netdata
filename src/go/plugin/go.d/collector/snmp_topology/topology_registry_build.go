// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
)

func buildSNMPTopologySnapshot(aggregate topologyObservationAggregate, options topologyQueryOptions) (topologyData, bool) {
	if len(aggregate.L2Observations) == 0 {
		return topologyData{}, false
	}

	if options.MapType != topologyMapTypeAllDevicesLowConfidence {
		return buildSingleMapTopologySnapshot(aggregate, options)
	}

	return buildProbableTopologySnapshot(aggregate, options)
}

func buildSingleMapTopologySnapshot(aggregate topologyObservationAggregate, options topologyQueryOptions) (topologyData, bool) {
	data, ok := buildSNMPL2TopologyData(
		aggregate.L2Observations,
		aggregate.AgentID,
		aggregate.LocalDeviceID,
		aggregate.CollectedAt,
		options,
	)
	if !ok {
		return topologyData{}, false
	}
	augmentTopologySnapshotLocals(&data, aggregate.Snapshots)
	applySNMPTopologyShapePolicies(&data, options)
	applyTopologyL3SubnetEnrichment(&data, aggregate)
	applyTopologyOSPFAdjacencyEnrichment(&data, aggregate)
	applyTopologyBGPAdjacencyEnrichment(&data, aggregate)
	applyTopologyDepthFocusFilter(&data, options)
	return data, true
}

func buildProbableTopologySnapshot(aggregate topologyObservationAggregate, options topologyQueryOptions) (topologyData, bool) {
	strictOptions := options
	strictOptions.MapType = topologyMapTypeHighConfidenceInferred
	strictData, strictOK := buildSNMPL2TopologyData(
		aggregate.L2Observations,
		aggregate.AgentID,
		aggregate.LocalDeviceID,
		aggregate.CollectedAt,
		strictOptions,
	)
	if !strictOK {
		return topologyData{}, false
	}
	augmentTopologySnapshotLocals(&strictData, aggregate.Snapshots)
	applySNMPTopologyShapePolicies(&strictData, strictOptions)

	probableOptions := options
	probableOptions.MapType = topologyMapTypeAllDevicesLowConfidence
	probableData, probableOK := buildSNMPL2TopologyData(
		aggregate.L2Observations,
		aggregate.AgentID,
		aggregate.LocalDeviceID,
		aggregate.CollectedAt,
		probableOptions,
	)
	if !probableOK {
		return topologyData{}, false
	}
	augmentTopologySnapshotLocals(&probableData, aggregate.Snapshots)
	applySNMPTopologyShapePolicies(&probableData, probableOptions)
	markProbableDeltaLinks(&strictData, &probableData)
	applyTopologyL3SubnetEnrichment(&probableData, aggregate)
	applyTopologyOSPFAdjacencyEnrichment(&probableData, aggregate)
	applyTopologyBGPAdjacencyEnrichment(&probableData, aggregate)
	applyTopologyDepthFocusFilter(&probableData, options)
	return probableData, true
}

func augmentTopologySnapshotLocals(data *topologyData, snapshots []topologyObservationSnapshot) {
	for _, snapshot := range snapshots {
		augmentLocalActorFromCache(data, snapshot.LocalDevice)
	}
}

func buildSNMPL2TopologyData(
	observations []topologyengine.L2Observation,
	agentID string,
	localDeviceID string,
	collectedAt time.Time,
	options topologyQueryOptions,
) (topologyData, bool) {
	if len(observations) == 0 {
		return topologyData{}, false
	}

	result, err := topologyengine.BuildL2ResultFromObservations(observations, topologyengine.DiscoverOptions{
		EnableLLDP:   true,
		EnableCDP:    true,
		EnableBridge: true,
		EnableARP:    true,
		EnableSTP:    true,
	})
	if err != nil {
		return topologyData{}, false
	}

	projection := topologyengine.ToGraph(result, topologyengine.GraphOptions{
		SchemaVersion:             topologySchemaVersion,
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		AgentID:                   agentID,
		LocalDeviceID:             localDeviceID,
		CollectedAt:               collectedAt,
		ResolveDNSName:            options.ResolveDNSName,
		CollapseActorsByIP:        options.CollapseActorsByIP,
		EliminateNonIPInferred:    options.EliminateNonIPInferred,
		ProbabilisticConnectivity: topologyoptions.IsMapTypeProbable(options.MapType),
		InferenceStrategy:         options.InferenceStrategy,
	})
	graphData := projection.Graph
	data := topologyData{
		SchemaVersion: graphData.SchemaVersion,
		Source:        graphData.Source,
		Layer:         graphData.Layer,
		AgentID:       graphData.AgentID,
		CollectedAt:   graphData.CollectedAt,
		View:          graphData.View,
		Actors:        topologyActorsFromProjection(graphData.Actors, projection.ActorDetails),
		Links:         topologyLinksFromGraph(graphData.Links),
		Stats: topologyStats{
			L2:    projection.Stats,
			HasL2: true,
		},
	}
	return data, true
}

func topologyActorsFromProjection(actors []graph.Actor, details map[string]topologyengine.ProjectionActorDetail) []topologyActor {
	if len(actors) == 0 {
		return nil
	}
	out := make([]topologyActor, len(actors))
	for i, actor := range actors {
		out[i] = topologyActorFromGraph(actor, details[strings.TrimSpace(actor.ActorID)])
	}
	return out
}
