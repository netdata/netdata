// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"sort"
	"strings"
	"time"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

type topologyObservationSnapshot struct {
	l2Observations []topologyengine.L2Observation
	localDevice    topologyDevice
	localDeviceID  string
	agentID        string
	collectedAt    time.Time
}

type topologyObservationAggregate struct {
	snapshots      []topologyObservationSnapshot
	l2Observations []topologyengine.L2Observation
	localDeviceID  string
	agentID        string
	collectedAt    time.Time
}

func (c *topologyCache) snapshotEngineObservations() (topologyObservationSnapshot, bool) {
	if c == nil {
		return topologyObservationSnapshot{}, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.hasFreshSnapshotAt(time.Now()) {
		return topologyObservationSnapshot{}, false
	}

	local := normalizeTopologyDevice(c.localDevice)
	localObservation := c.buildEngineObservation(local)
	localObservation.DeviceID = strings.TrimSpace(localObservation.DeviceID)
	if localObservation.DeviceID == "" {
		return topologyObservationSnapshot{}, false
	}
	if normalizeMAC(local.ChassisID) == "" {
		if mac := normalizeMAC(localObservation.BaseBridgeAddress); mac != "" {
			local.ChassisID = mac
			local.ChassisIDType = "macAddress"
		}
	}

	return topologyObservationSnapshot{
		l2Observations: []topologyengine.L2Observation{localObservation},
		localDevice:    local,
		localDeviceID:  localObservation.DeviceID,
		agentID:        strings.TrimSpace(c.agentID),
		collectedAt:    c.lastUpdate,
	}, true
}

func (r *topologyRegistry) activeCaches() []*topologyCache {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	caches := make([]*topologyCache, 0, len(r.caches))
	for cache := range r.caches {
		caches = append(caches, cache)
	}
	r.mu.RUnlock()
	return caches
}

func (r *topologyRegistry) observationSnapshots() []topologyObservationSnapshot {
	caches := r.activeCaches()
	if len(caches) == 0 {
		return nil
	}

	snapshots := make([]topologyObservationSnapshot, 0, len(caches))
	for _, cache := range caches {
		snapshot, ok := cache.snapshotEngineObservations()
		if !ok {
			continue
		}
		snapshots = append(snapshots, snapshot)
	}
	if len(snapshots) == 0 {
		return nil
	}

	sortTopologyObservationSnapshots(snapshots)
	return snapshots
}

func sortTopologyObservationSnapshots(snapshots []topologyObservationSnapshot) {
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].localDeviceID != snapshots[j].localDeviceID {
			return snapshots[i].localDeviceID < snapshots[j].localDeviceID
		}
		leftMgmt, leftHost := topologyObservationSnapshotIdentity(snapshots[i])
		rightMgmt, rightHost := topologyObservationSnapshotIdentity(snapshots[j])
		if leftMgmt != rightMgmt {
			return leftMgmt < rightMgmt
		}
		if leftHost != rightHost {
			return leftHost < rightHost
		}
		return snapshots[i].collectedAt.Before(snapshots[j].collectedAt)
	})
}

func topologyObservationSnapshotIdentity(snapshot topologyObservationSnapshot) (managementIP, hostname string) {
	if len(snapshot.l2Observations) == 0 {
		return "", ""
	}
	return snapshot.l2Observations[0].ManagementIP, snapshot.l2Observations[0].Hostname
}

func aggregateTopologyObservationSnapshots(snapshots []topologyObservationSnapshot) (topologyObservationAggregate, bool) {
	if len(snapshots) == 0 {
		return topologyObservationAggregate{}, false
	}

	totalObservations := 0
	for _, snapshot := range snapshots {
		totalObservations += len(snapshot.l2Observations)
	}

	aggregate := topologyObservationAggregate{
		snapshots:      snapshots,
		l2Observations: make([]topologyengine.L2Observation, 0, totalObservations),
	}
	for _, snapshot := range snapshots {
		aggregate.l2Observations = append(aggregate.l2Observations, snapshot.l2Observations...)
		if aggregate.localDeviceID == "" {
			aggregate.localDeviceID = snapshot.localDeviceID
		}
		if aggregate.agentID == "" && snapshot.agentID != "" {
			aggregate.agentID = snapshot.agentID
		}
		if snapshot.collectedAt.After(aggregate.collectedAt) {
			aggregate.collectedAt = snapshot.collectedAt
		}
	}

	return aggregate, len(aggregate.l2Observations) > 0
}

func buildSNMPTopologySnapshot(aggregate topologyObservationAggregate, options topologyQueryOptions) (topologyData, bool) {
	if len(aggregate.l2Observations) == 0 {
		return topologyData{}, false
	}

	if options.MapType != topologyMapTypeAllDevicesLowConfidence {
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
		for _, snapshot := range aggregate.snapshots {
			augmentLocalActorFromCache(&data, snapshot.localDevice)
		}
		applySNMPTopologyOutputPolicies(&data, options)
		applyTopologyDepthFocusFilter(&data, options)
		return data, true
	}

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
	for _, snapshot := range aggregate.snapshots {
		augmentLocalActorFromCache(&strictData, snapshot.localDevice)
	}
	applySNMPTopologyOutputPolicies(&strictData, strictOptions)

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
	for _, snapshot := range aggregate.snapshots {
		augmentLocalActorFromCache(&probableData, snapshot.localDevice)
	}
	applySNMPTopologyOutputPolicies(&probableData, probableOptions)
	markProbableDeltaLinks(&strictData, &probableData)
	applyTopologyDepthFocusFilter(&probableData, options)
	return probableData, true
}

func buildTopologyManagedFocusTargets(snapshots []topologyObservationSnapshot) []topologyManagedFocusTarget {
	if len(snapshots) == 0 {
		return nil
	}

	targetByValue := make(map[string]topologyManagedFocusTarget)
	for _, snapshot := range snapshots {
		managementIP := normalizeIPAddress(snapshot.localDevice.ManagementIP)
		if managementIP == "" && len(snapshot.l2Observations) > 0 {
			managementIP = normalizeIPAddress(snapshot.l2Observations[0].ManagementIP)
		}
		if managementIP == "" {
			continue
		}
		value := topologyManagedFocusIPPrefix + managementIP

		displayName := strings.TrimSpace(snapshot.localDevice.SysName)
		if displayName == "" && len(snapshot.l2Observations) > 0 {
			displayName = strings.TrimSpace(snapshot.l2Observations[0].Hostname)
		}
		if displayName == "" {
			displayName = managementIP
		}
		label := displayName
		if !strings.EqualFold(displayName, managementIP) {
			label = displayName + " (" + managementIP + ")"
		}

		existing, exists := targetByValue[value]
		if !exists || label < existing.Name {
			targetByValue[value] = topologyManagedFocusTarget{
				Value: value,
				Name:  label,
			}
		}
	}
	if len(targetByValue) == 0 {
		return nil
	}

	out := make([]topologyManagedFocusTarget, 0, len(targetByValue))
	for _, target := range targetByValue {
		out = append(out, target)
	}
	sort.Slice(out, func(i, j int) bool {
		leftName := strings.ToLower(strings.TrimSpace(out[i].Name))
		rightName := strings.ToLower(strings.TrimSpace(out[j].Name))
		if leftName != rightName {
			return leftName < rightName
		}
		return out[i].Value < out[j].Value
	})
	return out
}

func buildSNMPL2TopologyData(
	observations []topologyengine.L2Observation,
	agentID, localDeviceID string,
	collectedAt time.Time,
	queryOptions topologyQueryOptions,
) (topologyData, bool) {
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
	return topologyengine.ToTopologyData(result, topologyengine.TopologyDataOptions{
		SchemaVersion:             topologySchemaVersion,
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		AgentID:                   agentID,
		LocalDeviceID:             localDeviceID,
		CollectedAt:               collectedAt,
		ResolveDNSName:            resolveTopologyReverseDNSName,
		CollapseActorsByIP:        queryOptions.CollapseActorsByIP,
		EliminateNonIPInferred:    queryOptions.EliminateNonIPInferred,
		ProbabilisticConnectivity: isTopologyMapTypeProbable(queryOptions.MapType),
		InferenceStrategy:         queryOptions.InferenceStrategy,
	}), true
}
