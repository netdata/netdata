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
	l3Observation  topologyengine.L3Observation
	localDevice    topologyDevice
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

	if c.lastUpdate.IsZero() {
		return topologyObservationSnapshot{}, false
	}

	local := normalizeTopologyDevice(c.localDevice)
	localObservation := c.buildEngineObservation(local)
	localObservation.DeviceID = strings.TrimSpace(localObservation.DeviceID)
	if localObservation.DeviceID == "" {
		return topologyObservationSnapshot{}, false
	}

	return topologyObservationSnapshot{
		l2Observations: []topologyengine.L2Observation{localObservation},
		l3Observation:  c.buildEngineL3Observation(localObservation),
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
	return r.snapshotForView(topologyViewL2)
}

func (r *topologyRegistry) snapshotForView(view string) (topologyData, bool) {
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
	l3Observations := make([]topologyengine.L3Observation, 0, len(snapshots))
	localDeviceID := ""
	agentID := ""
	collectedAt := time.Time{}
	for _, snapshot := range snapshots {
		l2Observations = append(l2Observations, snapshot.l2Observations...)
		if strings.TrimSpace(snapshot.l3Observation.DeviceID) != "" {
			l3Observations = append(l3Observations, snapshot.l3Observation)
		}
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

	l2Data, l2OK := buildSNMPL2TopologyData(l2Observations, agentID, localDeviceID, collectedAt)
	l3Data, l3OK := buildSNMPL3TopologyData(l3Observations, agentID, localDeviceID, collectedAt)

	selectedView := normalizeTopologyView(view)
	if selectedView == "" {
		selectedView = topologyViewL2
	}
	switch selectedView {
	case topologyViewL3:
		if !l3OK {
			return topologyData{}, false
		}
		return l3Data, true
	case topologyViewMerged:
		data, ok := mergeSNMPTopologyData(l2Data, l2OK, l3Data, l3OK, agentID, collectedAt)
		if !ok {
			return topologyData{}, false
		}
		return data, true
	}
	if !l2OK {
		return topologyData{}, false
	}
	data := l2Data
	for _, snapshot := range snapshots {
		augmentLocalActorFromCache(&data, snapshot.localDevice)
	}

	return data, true
}

func buildSNMPL2TopologyData(
	observations []topologyengine.L2Observation,
	agentID, localDeviceID string,
	collectedAt time.Time,
) (topologyData, bool) {
	result, err := topologyengine.BuildL2ResultFromObservations(observations, topologyengine.DiscoverOptions{
		EnableLLDP:   true,
		EnableCDP:    true,
		EnableBridge: true,
		EnableARP:    true,
	})
	if err != nil {
		return topologyData{}, false
	}
	return topologyengine.ToTopologyData(result, topologyengine.TopologyDataOptions{
		SchemaVersion: topologySchemaVersion,
		Source:        "snmp",
		Layer:         "2",
		View:          "summary",
		AgentID:       agentID,
		LocalDeviceID: localDeviceID,
		CollectedAt:   collectedAt,
	}), true
}

func buildSNMPL3TopologyData(
	observations []topologyengine.L3Observation,
	agentID, localDeviceID string,
	collectedAt time.Time,
) (topologyData, bool) {
	result, err := topologyengine.BuildL3ResultFromObservations(observations)
	if err != nil {
		return topologyData{}, false
	}
	return topologyengine.ToTopologyData(result, topologyengine.TopologyDataOptions{
		SchemaVersion: topologySchemaVersion,
		Source:        "snmp",
		Layer:         "3",
		View:          "summary",
		AgentID:       agentID,
		LocalDeviceID: localDeviceID,
		CollectedAt:   collectedAt,
	}), true
}

func mergeSNMPTopologyData(
	l2 topologyData,
	l2OK bool,
	l3 topologyData,
	l3OK bool,
	agentID string,
	collectedAt time.Time,
) (topologyData, bool) {
	if !l2OK && !l3OK {
		return topologyData{}, false
	}
	if !l2OK {
		onlyL3 := l3
		onlyL3.Layer = "2-3"
		onlyL3.View = "summary"
		return onlyL3, true
	}
	if !l3OK {
		onlyL2 := l2
		onlyL2.Layer = "2-3"
		onlyL2.View = "summary"
		return onlyL2, true
	}

	merged := topologyData{
		SchemaVersion: topologySchemaVersion,
		Source:        "snmp",
		Layer:         "2-3",
		View:          "summary",
		AgentID:       agentID,
		CollectedAt:   collectedAt,
		Actors:        append([]topologyActor{}, l2.Actors...),
		Links:         append([]topologyLink{}, l2.Links...),
		Stats:         make(map[string]any),
	}
	actorSeen := make(map[string]struct{}, len(merged.Actors))
	for _, actor := range merged.Actors {
		for _, key := range topologyMatchIdentityKeys(actor.Match) {
			actorSeen[key] = struct{}{}
		}
	}
	for _, actor := range l3.Actors {
		keys := topologyMatchIdentityKeys(actor.Match)
		if len(keys) == 0 {
			continue
		}
		overlap := false
		for _, key := range keys {
			if _, ok := actorSeen[key]; ok {
				overlap = true
				break
			}
		}
		if overlap {
			continue
		}
		for _, key := range keys {
			actorSeen[key] = struct{}{}
		}
		merged.Actors = append(merged.Actors, actor)
	}
	merged.Links = append(merged.Links, l3.Links...)
	for key, value := range l2.Stats {
		merged.Stats["l2_"+key] = value
	}
	for key, value := range l3.Stats {
		merged.Stats["l3_"+key] = value
	}
	merged.Stats["actors_total"] = len(merged.Actors)
	merged.Stats["links_total"] = len(merged.Links)

	sort.Slice(merged.Actors, func(i, j int) bool {
		return canonicalMatchKey(merged.Actors[i].Match) < canonicalMatchKey(merged.Actors[j].Match)
	})
	sort.Slice(merged.Links, func(i, j int) bool {
		return topologyLinkSortKey(merged.Links[i]) < topologyLinkSortKey(merged.Links[j])
	})
	return merged, true
}
