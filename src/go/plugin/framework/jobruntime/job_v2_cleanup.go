// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"maps"
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

type jobV2CleanupSnapshot struct {
	scopeKey             string
	charts               map[string]chartengine.ChartMeta
	host                 jobV2HostRef
	staleVnodeSuppressed bool
}

func (s *jobV2HostState) captureCleanupSnapshot(info netdataapi.HostInfo, allowStaleVnodeSuppression bool) jobV2CleanupSnapshot {
	if s == nil {
		return jobV2CleanupSnapshot{}
	}
	host := s.cleanupOwner
	return jobV2CleanupSnapshot{
		charts:               maps.Clone(s.cleanupCharts),
		host:                 host,
		staleVnodeSuppressed: allowStaleVnodeSuppression && shouldSuppressCleanupForStaleVnode(host, info),
	}
}

func (j *JobV2) captureScopeCleanupSnapshots() []jobV2CleanupSnapshot {
	if j == nil || len(j.scopeStates) == 0 {
		return nil
	}
	keys := sortedScopeStateKeys(j.scopeStates)

	snapshots := make([]jobV2CleanupSnapshot, 0, len(keys))
	for _, key := range keys {
		state := j.scopeStates[key]
		if state == nil {
			continue
		}
		snapshot := state.host.captureCleanupSnapshot(j.cleanupHostInfo(state), key == defaultHostScopeKey)
		snapshot.scopeKey = key
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (j *JobV2) cleanupHostInfo(state *jobV2ScopeState) netdataapi.HostInfo {
	if state == nil {
		return netdataapi.HostInfo{}
	}
	// The returned HostInfo is consumed immediately for read-only
	// stale-suppression checks; callers must not mutate Labels.
	if state.scopeKey == defaultHostScopeKey && j.module != nil && j.module.VirtualNode() != nil {
		vnode := j.currentVnode()
		return netdataapi.HostInfo{
			GUID:     vnode.GUID,
			Hostname: vnode.Hostname,
			Labels:   vnode.Labels,
		}
	}
	return netdataapi.HostInfo{
		GUID:     state.host.cleanupInfo.GUID,
		Hostname: state.host.cleanupInfo.Hostname,
		Labels:   state.host.cleanupInfo.Labels,
	}
}

func (j *JobV2) releaseAllScopeRegistryOwners() {
	if j == nil {
		return
	}
	for _, state := range j.scopeStates {
		if state != nil {
			state.host.releaseRegistryOwners(j.vnodeRegistry)
		}
	}
}

func (j *JobV2) clearAllScopeStateAfterCleanup() {
	if j == nil {
		return
	}
	for _, state := range j.scopeStates {
		if state != nil {
			state.host.clearAfterCleanup()
		}
	}
	clear(j.scopeStates)
}

func (s *jobV2HostState) clearAfterCleanup() {
	if s == nil {
		return
	}
	clear(s.cleanupCharts)
	s.definedHost = jobV2HostRef{}
	s.definedInfo = netdataapi.HostInfo{}
	s.engineHost = jobV2HostRef{}
	s.cleanupOwner = jobV2HostRef{}
	s.cleanupInfo = netdataapi.HostInfo{}
}

func buildJobV2CleanupPlan(charts map[string]chartengine.ChartMeta) chartengine.Plan {
	if len(charts) == 0 {
		return chartengine.Plan{}
	}

	chartIDs := make([]string, 0, len(charts))
	for chartID := range charts {
		chartIDs = append(chartIDs, chartID)
	}
	sort.Strings(chartIDs)

	actions := make([]chartengine.EngineAction, 0, len(chartIDs))
	for _, chartID := range chartIDs {
		actions = append(actions, chartengine.RemoveChartAction{
			ChartID: chartID,
			Meta:    charts[chartID],
		})
	}
	return chartengine.Plan{Actions: actions}
}

func shouldSuppressCleanupForStaleVnode(cleanupHost jobV2HostRef, info netdataapi.HostInfo) bool {
	return cleanupHost.isVnode() &&
		info.GUID == cleanupHost.guid &&
		info.Labels["_node_stale_after_seconds"] != ""
}
