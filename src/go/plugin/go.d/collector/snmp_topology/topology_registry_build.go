// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"strings"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyenrich"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyshape"
)

func buildSNMPTopologySnapshot(aggregate topologymodel.ObservationAggregate, options topologyoptions.QueryOptions) (topologymodel.Data, bool) {
	if len(aggregate.L2Observations) == 0 {
		return topologymodel.Data{}, false
	}

	if options.MapType != topologyoptions.MapTypeAllDevicesLowConfidence {
		return buildSingleMapTopologySnapshot(aggregate, options)
	}

	return buildProbableTopologySnapshot(aggregate, options)
}

func buildSingleMapTopologySnapshot(aggregate topologymodel.ObservationAggregate, options topologyoptions.QueryOptions) (topologymodel.Data, bool) {
	data, ok := buildSNMPL2TopologyData(
		aggregate.L2Observations,
		aggregate.AgentID,
		aggregate.LocalDeviceID,
		aggregate.CollectedAt,
		options,
	)
	if !ok {
		return topologymodel.Data{}, false
	}
	augmentTopologySnapshotLocals(&data, aggregate.Snapshots)
	topologyshape.ApplyPolicies(&data, options)
	topologyenrich.ApplyL3Subnet(&data, aggregate)
	topologyenrich.ApplyOSPFAdjacency(&data, aggregate)
	topologyenrich.ApplyBGPAdjacency(&data, aggregate)
	topologyshape.ApplyDepthFocusFilter(&data, options)
	return data, true
}

func buildProbableTopologySnapshot(aggregate topologymodel.ObservationAggregate, options topologyoptions.QueryOptions) (topologymodel.Data, bool) {
	strictOptions := options
	strictOptions.MapType = topologyoptions.MapTypeHighConfidenceInferred
	strictData, strictOK := buildSNMPL2TopologyData(
		aggregate.L2Observations,
		aggregate.AgentID,
		aggregate.LocalDeviceID,
		aggregate.CollectedAt,
		strictOptions,
	)
	if !strictOK {
		return topologymodel.Data{}, false
	}
	augmentTopologySnapshotLocals(&strictData, aggregate.Snapshots)
	topologyshape.ApplyPolicies(&strictData, strictOptions)

	probableOptions := options
	probableOptions.MapType = topologyoptions.MapTypeAllDevicesLowConfidence
	probableData, probableOK := buildSNMPL2TopologyData(
		aggregate.L2Observations,
		aggregate.AgentID,
		aggregate.LocalDeviceID,
		aggregate.CollectedAt,
		probableOptions,
	)
	if !probableOK {
		return topologymodel.Data{}, false
	}
	augmentTopologySnapshotLocals(&probableData, aggregate.Snapshots)
	topologyshape.ApplyPolicies(&probableData, probableOptions)
	topologyshape.MarkProbableDeltaLinks(&strictData, &probableData)
	topologyenrich.ApplyL3Subnet(&probableData, aggregate)
	topologyenrich.ApplyOSPFAdjacency(&probableData, aggregate)
	topologyenrich.ApplyBGPAdjacency(&probableData, aggregate)
	topologyshape.ApplyDepthFocusFilter(&probableData, options)
	return probableData, true
}

func augmentTopologySnapshotLocals(data *topologymodel.Data, snapshots []topologymodel.ObservationSnapshot) {
	for _, snapshot := range snapshots {
		augmentLocalActorFromCache(data, snapshot.LocalDevice)
	}
}

func buildSNMPL2TopologyData(
	observations []topologyengine.L2Observation,
	agentID string,
	localDeviceID string,
	collectedAt time.Time,
	options topologyoptions.QueryOptions,
) (topologymodel.Data, bool) {
	if len(observations) == 0 {
		return topologymodel.Data{}, false
	}

	result, err := topologyengine.BuildL2ResultFromObservations(observations, topologyengine.DiscoverOptions{
		EnableLLDP:   true,
		EnableCDP:    true,
		EnableBridge: true,
		EnableARP:    true,
		EnableSTP:    true,
	})
	if err != nil {
		return topologymodel.Data{}, false
	}

	projection := topologyengine.ToGraph(result, topologyengine.GraphOptions{
		SchemaVersion:             topologymodel.SchemaVersion,
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
	data := topologymodel.Data{
		SchemaVersion: graphData.SchemaVersion,
		Source:        graphData.Source,
		Layer:         graphData.Layer,
		AgentID:       graphData.AgentID,
		CollectedAt:   graphData.CollectedAt,
		View:          graphData.View,
		Actors:        topologyActorsFromProjection(graphData.Actors, projection.ActorDetails),
		Links:         topologyLinksFromGraph(graphData.Links),
		Stats: topologymodel.Stats{
			L2:    projection.Stats,
			HasL2: true,
		},
	}
	return data, true
}

func topologyActorsFromProjection(actors []graph.Actor, details map[string]topologyengine.ProjectionActorDetail) []topologymodel.Actor {
	if len(actors) == 0 {
		return nil
	}
	out := make([]topologymodel.Actor, len(actors))
	for i, actor := range actors {
		out[i] = topologyActorFromGraph(actor, details[strings.TrimSpace(actor.ActorID)])
	}
	return out
}
