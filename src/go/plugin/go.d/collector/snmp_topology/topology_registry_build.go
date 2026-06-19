// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
)

func buildSNMPTopologySnapshot(aggregate topologyObservationAggregate, options topologyQueryOptions) (topologyData, bool) {
	if len(aggregate.l2Observations) == 0 {
		return topologyData{}, false
	}

	if options.MapType != topologyMapTypeAllDevicesLowConfidence {
		return buildSingleMapTopologySnapshot(aggregate, options)
	}

	return buildProbableTopologySnapshot(aggregate, options)
}

func buildSingleMapTopologySnapshot(aggregate topologyObservationAggregate, options topologyQueryOptions) (topologyData, bool) {
	data, ok := buildSNMPL2TopologyData(
		aggregate.l2Observations,
		aggregate.agentID,
		aggregate.localDeviceID,
		aggregate.collectedAt,
		options,
	)
	if !ok {
		return topologyData{}, false
	}
	augmentTopologySnapshotLocals(&data, aggregate.snapshots)
	applySNMPTopologyShapePolicies(&data, options)
	applyTopologyL3SubnetEnrichment(&data, aggregate)
	applyTopologyOSPFAdjacencyEnrichment(&data, aggregate)
	applyTopologyDepthFocusFilter(&data, options)
	return data, true
}

func buildProbableTopologySnapshot(aggregate topologyObservationAggregate, options topologyQueryOptions) (topologyData, bool) {
	strictOptions := options
	strictOptions.MapType = topologyMapTypeHighConfidenceInferred
	strictData, strictOK := buildSNMPL2TopologyData(
		aggregate.l2Observations,
		aggregate.agentID,
		aggregate.localDeviceID,
		aggregate.collectedAt,
		strictOptions,
	)
	if !strictOK {
		return topologyData{}, false
	}
	augmentTopologySnapshotLocals(&strictData, aggregate.snapshots)
	applySNMPTopologyShapePolicies(&strictData, strictOptions)

	probableOptions := options
	probableOptions.MapType = topologyMapTypeAllDevicesLowConfidence
	probableData, probableOK := buildSNMPL2TopologyData(
		aggregate.l2Observations,
		aggregate.agentID,
		aggregate.localDeviceID,
		aggregate.collectedAt,
		probableOptions,
	)
	if !probableOK {
		return topologyData{}, false
	}
	augmentTopologySnapshotLocals(&probableData, aggregate.snapshots)
	applySNMPTopologyShapePolicies(&probableData, probableOptions)
	markProbableDeltaLinks(&strictData, &probableData)
	applyTopologyL3SubnetEnrichment(&probableData, aggregate)
	applyTopologyOSPFAdjacencyEnrichment(&probableData, aggregate)
	applyTopologyDepthFocusFilter(&probableData, options)
	return probableData, true
}

func augmentTopologySnapshotLocals(data *topologyData, snapshots []topologyObservationSnapshot) {
	for _, snapshot := range snapshots {
		augmentLocalActorFromCache(data, snapshot.localDevice)
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

	data := topologyengine.ToGraph(result, topologyengine.GraphOptions{
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
		ProbabilisticConnectivity: isTopologyMapTypeProbable(options.MapType),
		InferenceStrategy:         options.InferenceStrategy,
	})
	return data, true
}
