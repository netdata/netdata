// SPDX-License-Identifier: GPL-3.0-or-later

package jobruntime

import (
	"maps"
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type jobV2CleanupSnapshot struct {
	charts               map[string]chartengine.ChartMeta
	host                 jobV2HostRef
	staleVnodeSuppressed bool
}

func (s *jobV2HostState) captureCleanupSnapshot(vnode vnodes.VirtualNode) jobV2CleanupSnapshot {
	if s == nil {
		return jobV2CleanupSnapshot{}
	}
	host := s.cleanupOwner
	return jobV2CleanupSnapshot{
		charts:               maps.Clone(s.cleanupCharts),
		host:                 host,
		staleVnodeSuppressed: shouldSuppressCleanupForStaleVnode(host, vnode),
	}
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

func shouldSuppressCleanupForStaleVnode(cleanupHost jobV2HostRef, vnode vnodes.VirtualNode) bool {
	return cleanupHost.isVnode() &&
		vnode.GUID == cleanupHost.guid &&
		vnode.Labels["_node_stale_after_seconds"] != ""
}
