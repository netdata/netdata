// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"sort"
	"strings"
	"sync"
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

type topologyQueryOptions struct {
	CollapseActorsByIP     bool
	EliminateNonIPInferred bool
	MapType                string
	InferenceStrategy      string
	ManagedDeviceFocus     string
	Depth                  int
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

type topologyRegistry struct {
	mu     sync.RWMutex
	caches map[*topologyCache]struct{}
}

type topologyManagedFocusTarget struct {
	Value string
	Name  string
}

func newTopologyRegistry() *topologyRegistry {
	return &topologyRegistry{
		caches: make(map[*topologyCache]struct{}),
	}
}

var snmpTopologyRegistry = newTopologyRegistry()

func (r *topologyRegistry) register(cache *topologyCache) {
	if r == nil || cache == nil {
		return
	}
	r.mu.Lock()
	r.caches[cache] = struct{}{}
	r.mu.Unlock()
}

func (r *topologyRegistry) unregister(cache *topologyCache) {
	if r == nil || cache == nil {
		return
	}
	r.mu.Lock()
	delete(r.caches, cache)
	r.mu.Unlock()
}

func (r *topologyRegistry) snapshot() (topologyData, bool) {
	return r.snapshotWithOptions(topologyQueryOptions{
		CollapseActorsByIP:     true,
		EliminateNonIPInferred: true,
		MapType:                topologyMapTypeLLDPCDPManaged,
		InferenceStrategy:      topologyInferenceStrategyFDBMinimumKnowledge,
		ManagedDeviceFocus:     topologyManagedFocusAllDevices,
		Depth:                  topologyDepthAllInternal,
	})
}

func (r *topologyRegistry) snapshotWithOptions(options topologyQueryOptions) (topologyData, bool) {
	if r == nil {
		return topologyData{}, false
	}
	options = normalizeTopologyQueryOptions(options)

	r.mu.RLock()
	caches := make([]*topologyCache, 0, len(r.caches))
	for cache := range r.caches {
		caches = append(caches, cache)
	}
	r.mu.RUnlock()

	if len(caches) == 0 {
		return topologyData{}, false
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
		return topologyData{}, false
	}

	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].localDeviceID != snapshots[j].localDeviceID {
			return snapshots[i].localDeviceID < snapshots[j].localDeviceID
		}
		leftMgmt := ""
		rightMgmt := ""
		if len(snapshots[i].l2Observations) > 0 {
			leftMgmt = snapshots[i].l2Observations[0].ManagementIP
		}
		if len(snapshots[j].l2Observations) > 0 {
			rightMgmt = snapshots[j].l2Observations[0].ManagementIP
		}
		if leftMgmt != rightMgmt {
			return leftMgmt < rightMgmt
		}
		leftHost := ""
		rightHost := ""
		if len(snapshots[i].l2Observations) > 0 {
			leftHost = snapshots[i].l2Observations[0].Hostname
		}
		if len(snapshots[j].l2Observations) > 0 {
			rightHost = snapshots[j].l2Observations[0].Hostname
		}
		if leftHost != rightHost {
			return leftHost < rightHost
		}
		return snapshots[i].collectedAt.Before(snapshots[j].collectedAt)
	})

	totalObservations := 0
	for _, snapshot := range snapshots {
		totalObservations += len(snapshot.l2Observations)
	}
	l2Observations := make([]topologyengine.L2Observation, 0, totalObservations)
	localDeviceID := ""
	agentID := ""
	collectedAt := time.Time{}
	for _, snapshot := range snapshots {
		l2Observations = append(l2Observations, snapshot.l2Observations...)
		if localDeviceID == "" {
			localDeviceID = snapshot.localDeviceID
		}
		if agentID == "" && snapshot.agentID != "" {
			agentID = snapshot.agentID
		}
		if snapshot.collectedAt.After(collectedAt) {
			collectedAt = snapshot.collectedAt
		}
	}

	if options.MapType != topologyMapTypeAllDevicesLowConfidence {
		data, ok := buildSNMPL2TopologyData(
			l2Observations,
			agentID,
			localDeviceID,
			collectedAt,
			options,
		)
		if !ok {
			return topologyData{}, false
		}
		for _, snapshot := range snapshots {
			augmentLocalActorFromCache(&data, snapshot.localDevice)
		}
		applySNMPTopologyOutputPolicies(&data, options)
		applyTopologyDepthFocusFilter(&data, options)
		return data, true
	}

	strictOptions := options
	strictOptions.MapType = topologyMapTypeHighConfidenceInferred
	strictData, strictOK := buildSNMPL2TopologyData(
		l2Observations,
		agentID,
		localDeviceID,
		collectedAt,
		strictOptions,
	)
	if !strictOK {
		return topologyData{}, false
	}
	for _, snapshot := range snapshots {
		augmentLocalActorFromCache(&strictData, snapshot.localDevice)
	}
	applySNMPTopologyOutputPolicies(&strictData, strictOptions)

	probableOptions := options
	probableOptions.MapType = topologyMapTypeAllDevicesLowConfidence
	probableData, probableOK := buildSNMPL2TopologyData(
		l2Observations,
		agentID,
		localDeviceID,
		collectedAt,
		probableOptions,
	)
	if !probableOK {
		return topologyData{}, false
	}
	for _, snapshot := range snapshots {
		augmentLocalActorFromCache(&probableData, snapshot.localDevice)
	}
	applySNMPTopologyOutputPolicies(&probableData, probableOptions)
	markProbableDeltaLinks(&strictData, &probableData)
	applyTopologyDepthFocusFilter(&probableData, options)
	return probableData, true
}

func normalizeTopologyQueryOptions(options topologyQueryOptions) topologyQueryOptions {
	options.MapType = normalizeTopologyMapType(options.MapType)
	if options.MapType == "" {
		options.MapType = topologyMapTypeLLDPCDPManaged
	}
	options.InferenceStrategy = normalizeTopologyInferenceStrategy(options.InferenceStrategy)
	if options.InferenceStrategy == "" {
		options.InferenceStrategy = topologyInferenceStrategyFDBMinimumKnowledge
	}
	options.ManagedDeviceFocus = formatTopologyManagedFocuses(parseTopologyManagedFocuses(options.ManagedDeviceFocus))
	if options.Depth != topologyDepthAllInternal {
		if options.Depth < topologyDepthMin {
			options.Depth = topologyDepthMin
		} else if options.Depth > topologyDepthMax {
			options.Depth = topologyDepthMax
		}
	}
	return options
}

func (r *topologyRegistry) managedDeviceFocusTargets() []topologyManagedFocusTarget {
	if r == nil {
		return nil
	}

	r.mu.RLock()
	caches := make([]*topologyCache, 0, len(r.caches))
	for cache := range r.caches {
		caches = append(caches, cache)
	}
	r.mu.RUnlock()

	if len(caches) == 0 {
		return nil
	}

	targetByValue := make(map[string]topologyManagedFocusTarget)
	for _, cache := range caches {
		snapshot, ok := cache.snapshotEngineObservations()
		if !ok {
			continue
		}
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
