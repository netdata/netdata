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
	var err error
	options, err = topologyoptions.PrepareQueryOptions(options)
	if err != nil {
		return topologymodel.Data{}, false, err
	}
	if len(aggregate.L2Observations) == 0 {
		return topologymodel.Data{}, false, nil
	}

	if options.MapType != topologyoptions.MapTypeAllDevicesLowConfidence {
		return buildSingleMapTopologySnapshot(aggregate, options)
	}

	return buildProbableTopologySnapshot(aggregate, options)
}

func buildSingleMapTopologySnapshot(aggregate topologymodel.ObservationAggregate, options topologyoptions.QueryOptions) (topologymodel.Data, bool, error) {
	result, ok, err := buildSNMPL2TopologyResult(aggregate.L2Observations, options.WorkLimiter)
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
	if err := augmentTopologySnapshotLocals(&data, aggregate.Snapshots, options.WorkLimiter); err != nil {
		return topologymodel.Data{}, false, err
	}
	if err := topologyshape.ApplyPolicies(&data, options); err != nil {
		return topologymodel.Data{}, false, err
	}
	if err := topologyenrich.ApplyLayer3(&data, aggregate, options.WorkLimiter); err != nil {
		return topologymodel.Data{}, false, err
	}
	if err := topologyshape.ApplyDepthFocusFilter(&data, options); err != nil {
		return topologymodel.Data{}, false, err
	}
	return data, true, nil
}

func buildProbableTopologySnapshot(aggregate topologymodel.ObservationAggregate, options topologyoptions.QueryOptions) (topologymodel.Data, bool, error) {
	result, ok, err := buildSNMPL2TopologyResult(aggregate.L2Observations, options.WorkLimiter)
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
	if err := augmentTopologySnapshotLocals(&strictData, aggregate.Snapshots, options.WorkLimiter); err != nil {
		return topologymodel.Data{}, false, fmt.Errorf("build strict topology: %w", err)
	}
	if err := topologyshape.ApplyPolicies(&strictData, strictOptions); err != nil {
		return topologymodel.Data{}, false, fmt.Errorf("build strict topology: %w", err)
	}

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
	if err := augmentTopologySnapshotLocals(&probableData, aggregate.Snapshots, options.WorkLimiter); err != nil {
		return topologymodel.Data{}, false, fmt.Errorf("build probable topology: %w", err)
	}
	if err := topologyshape.ApplyPolicies(&probableData, probableOptions); err != nil {
		return topologymodel.Data{}, false, fmt.Errorf("build probable topology: %w", err)
	}
	if err := topologyshape.MarkProbableDeltaLinksWithLimiter(&strictData, &probableData, options.WorkLimiter); err != nil {
		return topologymodel.Data{}, false, fmt.Errorf("mark probable topology delta: %w", err)
	}
	if err := topologyenrich.ApplyLayer3(&probableData, aggregate, options.WorkLimiter); err != nil {
		return topologymodel.Data{}, false, err
	}
	if err := topologyshape.ApplyDepthFocusFilter(&probableData, options); err != nil {
		return topologymodel.Data{}, false, err
	}
	return probableData, true, nil
}

func augmentTopologySnapshotLocals(
	data *topologymodel.Data,
	snapshots []topologymodel.ObservationSnapshot,
	limiter topologyengine.WorkLimiter,
) error {
	if data == nil || len(snapshots) == 0 {
		return nil
	}
	if err := limiter.Charge(uint64(len(data.Actors))); err != nil {
		return err
	}
	index := topologymodel.NewLocalActorMatchIndex()
	for i := range data.Actors {
		actor := data.Actors[i]
		if err := index.AddActorIDWithLimiter(actor.ActorID, limiter); err != nil {
			return err
		}
		if topologyengine.IsDeviceActorType(actor.ActorType) {
			if err := index.AddMatchWithLimiter(i, actor.Match, limiter); err != nil {
				return err
			}
		}
	}

	if err := limiter.Charge(uint64(len(snapshots))); err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		actorIndex, found, err := index.FirstMatchWithLimiter(snapshot.LocalDevice, limiter)
		if err != nil {
			return err
		}
		if found {
			if err := augmentLocalActor(&data.Actors[actorIndex], snapshot.LocalDevice, limiter); err != nil {
				return err
			}
			continue
		}
		// The default map is a managed-device map: show polled SNMP devices even
		// when no L2 relationship emitted a local actor yet.
		actor, ok, err := topologyLocalActorFromCacheWithLimiter(snapshot.LocalDeviceID, snapshot.LocalDevice, limiter)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		contains, err := index.ContainsActorIDWithLimiter(actor.ActorID, limiter)
		if err != nil {
			return err
		}
		if contains {
			continue
		}
		actor.ActorHandle = data.NextActorHandle()
		data.Actors = append(data.Actors, actor)
		actorIndex = len(data.Actors) - 1
		if err := index.AddActorIDWithLimiter(actor.ActorID, limiter); err != nil {
			return err
		}
		if err := index.AddMatchWithLimiter(actorIndex, actor.Match, limiter); err != nil {
			return err
		}
	}
	return nil
}

func buildSNMPL2TopologyResult(
	observations []topologyengine.L2Observation,
	limiter topologyengine.WorkLimiter,
) (topologyengine.Result, bool, error) {
	if len(observations) == 0 {
		return topologyengine.Result{}, false, nil
	}

	result, err := topologyengine.BuildL2ResultFromObservations(observations, topologyengine.DiscoverOptions{
		EnableLLDP:   true,
		EnableCDP:    true,
		EnableBridge: true,
		EnableARP:    true,
		EnableSTP:    true,
		WorkLimiter:  limiter,
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
	projection, err := topologyengine.ToGraph(result, topologyengine.GraphOptions{
		SchemaVersion:             topologymodel.SchemaVersion,
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		AgentID:                   agentID,
		CollectedAt:               collectedAt,
		ResolveDNSName:            options.ResolveDNSName,
		LookupVendorByMAC:         options.LookupVendorByMAC,
		CollapseActorsByIP:        options.CollapseActorsByIP,
		EliminateNonIPInferred:    options.EliminateNonIPInferred,
		ProbabilisticConnectivity: topologyoptions.IsMapTypeProbable(options.MapType),
		InferenceStrategy:         options.InferenceStrategy,
		WorkLimiter:               options.WorkLimiter,
	})
	if err != nil {
		return topologymodel.Data{}, err
	}
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
