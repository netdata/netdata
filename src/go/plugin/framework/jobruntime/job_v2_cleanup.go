// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"maps"
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/hostoutput"
)

type jobV2CleanupSnapshot struct {
	scopeKey   string
	charts     map[string]chartengine.ChartMeta
	host       jobV2HostRef
	owner      *hostoutput.Owner
	definition *hostoutput.Definition
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
		snapshot := jobV2CleanupSnapshot{
			host:   state.host.cleanupOwner,
			charts: maps.Clone(state.host.cleanupCharts),
		}
		snapshot.scopeKey = key
		snapshot.owner = state.host.owner
		snapshot.definition = state.host.cleanupDefinition
		if state.scopeKey == defaultHostScopeKey && j.module != nil && j.module.VirtualNode() != nil &&
			j.vnodeName == "" {
			vnode := j.currentVnode()
			info := netdataapi.HostInfo{
				GUID:     vnode.GUID,
				Hostname: vnode.Hostname,
				Labels:   vnode.HostLabels(),
			}
			if info.GUID == snapshot.host.guid {
				if definition, err := hostoutput.NewDefinition(info); err == nil {
					snapshot.definition = definition
				}
			}
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func (j *JobV2) releaseAllScopeOwners() {
	if j == nil {
		return
	}
	for _, state := range j.scopeStates {
		if state != nil {
			state.host.owner.Release()
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
	s.owner = nil
	s.ownerGUID = ""
	s.cleanupDefinition = nil
	s.engineHost = jobV2HostRef{}
	s.cleanupOwner = jobV2HostRef{}
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
	return chartengine.Plan{
		Actions: actions,
	}
}
