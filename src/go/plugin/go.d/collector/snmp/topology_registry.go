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
	CollapseActorsByIP        bool
	EliminateNonIPInferred    bool
	ProbabilisticConnectivity bool
}

func (c *topologyCache) snapshotEngineObservations() (topologyObservationSnapshot, bool) {
	if c == nil {
		return topologyObservationSnapshot{}, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.lastUpdate.IsZero() {
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
		CollapseActorsByIP:        true,
		EliminateNonIPInferred:    true,
		ProbabilisticConnectivity: true,
	})
}

func (r *topologyRegistry) snapshotWithOptions(options topologyQueryOptions) (topologyData, bool) {
	if r == nil {
		return topologyData{}, false
	}

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

	l2Data, l2OK := buildSNMPL2TopologyData(l2Observations, agentID, localDeviceID, collectedAt, options)
	if !l2OK {
		return topologyData{}, false
	}
	data := l2Data
	for _, snapshot := range snapshots {
		augmentLocalActorFromCache(&data, snapshot.localDevice)
	}
	applySNMPTopologyOutputPolicies(&data, options)

	return data, true
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
		ProbabilisticConnectivity: queryOptions.ProbabilisticConnectivity,
	}), true
}
