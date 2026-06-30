// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
)

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

func (r *topologyRegistry) observationSnapshots() []topologymodel.ObservationSnapshot {
	caches := r.activeCaches()
	if len(caches) == 0 {
		return nil
	}

	snapshots := make([]topologymodel.ObservationSnapshot, 0, len(caches))
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

func sortTopologyObservationSnapshots(snapshots []topologymodel.ObservationSnapshot) {
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].LocalDeviceID != snapshots[j].LocalDeviceID {
			return snapshots[i].LocalDeviceID < snapshots[j].LocalDeviceID
		}
		leftMgmt, leftHost := topologyObservationSnapshotIdentity(snapshots[i])
		rightMgmt, rightHost := topologyObservationSnapshotIdentity(snapshots[j])
		if leftMgmt != rightMgmt {
			return leftMgmt < rightMgmt
		}
		if leftHost != rightHost {
			return leftHost < rightHost
		}
		return snapshots[i].CollectedAt.Before(snapshots[j].CollectedAt)
	})
}

func topologyObservationSnapshotIdentity(snapshot topologymodel.ObservationSnapshot) (managementIP, hostname string) {
	if len(snapshot.L2Observations) == 0 {
		return "", ""
	}
	return snapshot.L2Observations[0].ManagementIP, snapshot.L2Observations[0].Hostname
}

func aggregateTopologyObservationSnapshots(snapshots []topologymodel.ObservationSnapshot) (topologymodel.ObservationAggregate, bool) {
	if len(snapshots) == 0 {
		return topologymodel.ObservationAggregate{}, false
	}

	totalObservations := 0
	totalL3Interfaces := 0
	totalOSPFNeighbors := 0
	totalBGPPeers := 0
	for _, snapshot := range snapshots {
		totalObservations += len(snapshot.L2Observations)
		totalL3Interfaces += len(snapshot.L3Interfaces)
		totalOSPFNeighbors += len(snapshot.OSPFNeighbors)
		totalBGPPeers += len(snapshot.BGPPeers)
	}

	aggregate := topologymodel.ObservationAggregate{
		Snapshots:      snapshots,
		L2Observations: make([]topologyengine.L2Observation, 0, totalObservations),
		L3Interfaces:   make([]topologymodel.L3Interface, 0, totalL3Interfaces),
		OSPFNeighbors:  make([]topologymodel.OSPFNeighbor, 0, totalOSPFNeighbors),
		BGPPeers:       make([]topologymodel.BGPPeer, 0, totalBGPPeers),
	}
	for _, snapshot := range snapshots {
		aggregate.L2Observations = append(aggregate.L2Observations, snapshot.L2Observations...)
		aggregate.L3Interfaces = append(aggregate.L3Interfaces, snapshot.L3Interfaces...)
		aggregate.OSPFNeighbors = append(aggregate.OSPFNeighbors, snapshot.OSPFNeighbors...)
		aggregate.BGPPeers = append(aggregate.BGPPeers, snapshot.BGPPeers...)
		if aggregate.LocalDeviceID == "" {
			aggregate.LocalDeviceID = snapshot.LocalDeviceID
		}
		if aggregate.AgentID == "" && snapshot.AgentID != "" {
			aggregate.AgentID = snapshot.AgentID
		}
		if snapshot.CollectedAt.After(aggregate.CollectedAt) {
			aggregate.CollectedAt = snapshot.CollectedAt
		}
	}

	return aggregate, len(aggregate.L2Observations) > 0
}
