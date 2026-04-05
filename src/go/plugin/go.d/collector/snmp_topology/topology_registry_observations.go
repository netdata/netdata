// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"sort"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/topology/engine"
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
