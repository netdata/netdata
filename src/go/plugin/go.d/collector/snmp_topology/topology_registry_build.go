// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyenrich"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyshape"
)

func buildSNMPTopologySnapshot(aggregate topologymodel.ObservationAggregate, options topologyoptions.QueryOptions) (topologymodel.Data, bool, error) {
	if len(aggregate.L2Observations) == 0 {
		return topologymodel.Data{}, false, nil
	}

	if options.MapType != topologyoptions.MapTypeAllDevicesLowConfidence {
		return buildSingleMapTopologySnapshot(aggregate, options)
	}

	return buildProbableTopologySnapshot(aggregate, options)
}

func buildSingleMapTopologySnapshot(aggregate topologymodel.ObservationAggregate, options topologyoptions.QueryOptions) (topologymodel.Data, bool, error) {
	result, ok, err := buildSNMPL2TopologyResult(aggregate.L2Observations)
	if err != nil || !ok {
		return topologymodel.Data{}, false, err
	}
	data, err := projectSNMPL2TopologyData(
		result,
		aggregate.AgentID,
		aggregate.CollectedAt,
		options,
	)
	if err != nil {
		return topologymodel.Data{}, false, err
	}
	augmentTopologySnapshotLocals(&data, aggregate.Snapshots)
	topologyshape.ApplyPolicies(&data, options)
	topologyenrich.ApplyLayer3(&data, aggregate)
	topologyshape.ApplyDepthFocusFilter(&data, options)
	return data, true, nil
}

func buildProbableTopologySnapshot(aggregate topologymodel.ObservationAggregate, options topologyoptions.QueryOptions) (topologymodel.Data, bool, error) {
	result, ok, err := buildSNMPL2TopologyResult(aggregate.L2Observations)
	if err != nil {
		return topologymodel.Data{}, false, fmt.Errorf("build strict topology: %w", err)
	}
	if !ok {
		return topologymodel.Data{}, false, nil
	}

	strictOptions := options
	strictOptions.MapType = topologyoptions.MapTypeHighConfidenceInferred
	strictData, err := projectSNMPL2TopologyData(
		result,
		aggregate.AgentID,
		aggregate.CollectedAt,
		strictOptions,
	)
	if err != nil {
		return topologymodel.Data{}, false, fmt.Errorf("build strict topology: %w", err)
	}
	augmentTopologySnapshotLocals(&strictData, aggregate.Snapshots)
	topologyshape.ApplyPolicies(&strictData, strictOptions)

	probableOptions := options
	probableOptions.MapType = topologyoptions.MapTypeAllDevicesLowConfidence
	probableData, err := projectSNMPL2TopologyData(
		result,
		aggregate.AgentID,
		aggregate.CollectedAt,
		probableOptions,
	)
	if err != nil {
		return topologymodel.Data{}, false, fmt.Errorf("build probable topology: %w", err)
	}
	augmentTopologySnapshotLocals(&probableData, aggregate.Snapshots)
	topologyshape.ApplyPolicies(&probableData, probableOptions)
	topologyshape.MarkProbableDeltaLinks(&strictData, &probableData)
	topologyenrich.ApplyLayer3(&probableData, aggregate)
	topologyshape.ApplyDepthFocusFilter(&probableData, options)
	return probableData, true, nil
}

func augmentTopologySnapshotLocals(data *topologymodel.Data, snapshots []topologymodel.ObservationSnapshot) {
	if data == nil || len(snapshots) == 0 {
		return
	}
	index := topologymodel.NewLocalActorMatchIndex()
	for i := range data.Actors {
		actor := data.Actors[i]
		index.AddActorID(actor.ActorID)
		if topologyengine.IsDeviceActorType(actor.ActorType) {
			index.AddMatch(i, actor.Match)
		}
	}

	for _, snapshot := range snapshots {
		if actorIndex, ok := index.FirstMatch(snapshot.LocalDevice); ok {
			augmentLocalActor(&data.Actors[actorIndex], snapshot.LocalDevice)
			continue
		}
		// The default map is a managed-device map: show polled SNMP devices even
		// when no L2 relationship emitted a local actor yet.
		actor, ok := topologyLocalActorFromCache(snapshot.LocalDeviceID, snapshot.LocalDevice)
		if !ok || index.ContainsActorID(actor.ActorID) {
			continue
		}
		actor.ActorHandle = data.NextActorHandle()
		data.Actors = append(data.Actors, actor)
		actorIndex := len(data.Actors) - 1
		index.AddActorID(actor.ActorID)
		index.AddMatch(actorIndex, actor.Match)
	}
}

func buildSNMPL2TopologyResult(observations []topologyengine.L2Observation) (topologyengine.Result, bool, error) {
	if len(observations) == 0 {
		return topologyengine.Result{}, false, nil
	}

	result, err := topologyengine.BuildL2ResultFromObservations(observations, topologyengine.DiscoverOptions{
		EnableLLDP:   true,
		EnableCDP:    true,
		EnableBridge: true,
		EnableARP:    true,
		EnableSTP:    true,
	})
	if err != nil {
		return topologyengine.Result{}, false, fmt.Errorf("build L2 topology result: %w", err)
	}
	return result, true, nil
}

func projectSNMPL2TopologyData(
	result topologyengine.Result,
	agentID string,
	collectedAt time.Time,
	options topologyoptions.QueryOptions,
) (topologymodel.Data, error) {
	projection := topologyengine.ToGraph(result, topologyengine.GraphOptions{
		SchemaVersion:             topologymodel.SchemaVersion,
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		AgentID:                   agentID,
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
	if err := data.InitializeActorHandles(); err != nil {
		return topologymodel.Data{}, fmt.Errorf("initialize topology actor handles: %w", err)
	}
	return data, nil
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
