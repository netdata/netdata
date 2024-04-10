// SPDX-License-Identifier: GPL-3.0-or-later

package zfspool

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

const (
	prioZpoolSpaceUtilization = 2820 + iota
	prioZpoolSpaceUsage
	prioZpoolFragmentation
	prioZpoolHealthState
)

var zpoolChartsTmpl = module.Charts{
	zpoolSpaceUtilizationChartTmpl.Copy(),
	zpoolSpaceUsageChartTmpl.Copy(),

	zpoolFragmentationChartTmpl.Copy(),

	zpoolHealthStateChartTmpl.Copy(),
}

var (
	zpoolSpaceUtilizationChartTmpl = module.Chart{
		ID:       "zfspool_%s_space_utilization",
		Title:    "Zpool space utilization",
		Units:    "percentage",
		Fam:      "space usage",
		Ctx:      "zfspool.pool_space_utilization",
		Type:     module.Area,
		Priority: prioZpoolSpaceUtilization,
		Dims: module.Dims{
			{ID: "zpool_%s_cap", Name: "utilization"},
		},
	}
	zpoolSpaceUsageChartTmpl = module.Chart{
		ID:       "zfspool_%s_space_usage",
		Title:    "Zpool space usage",
		Units:    "bytes",
		Fam:      "space usage",
		Ctx:      "zfspool.pool_space_usage",
		Type:     module.Stacked,
		Priority: prioZpoolSpaceUsage,
		Dims: module.Dims{
			{ID: "zpool_%s_free", Name: "free"},
			{ID: "zpool_%s_alloc", Name: "used"},
		},
	}

	zpoolFragmentationChartTmpl = module.Chart{
		ID:       "zfspool_%s_fragmentation",
		Title:    "Zpool fragmentation",
		Units:    "percentage",
		Fam:      "fragmentation",
		Ctx:      "zfspool.pool_fragmentation",
		Type:     module.Line,
		Priority: prioZpoolFragmentation,
		Dims: module.Dims{
			{ID: "zpool_%s_frag", Name: "fragmentation"},
		},
	}

	zpoolHealthStateChartTmpl = module.Chart{
		ID:       "zfspool_%s_health_state",
		Title:    "Zpool health state",
		Units:    "state",
		Fam:      "health",
		Ctx:      "zfspool.pool_health_state",
		Type:     module.Line,
		Priority: prioZpoolHealthState,
		Dims: module.Dims{
			{ID: "zpool_%s_health_state_online", Name: "online"},
			{ID: "zpool_%s_health_state_degraded", Name: "degraded"},
			{ID: "zpool_%s_health_state_faulted", Name: "faulted"},
			{ID: "zpool_%s_health_state_offline", Name: "offline"},
			{ID: "zpool_%s_health_state_unavail", Name: "unavail"},
			{ID: "zpool_%s_health_state_removed", Name: "removed"},
			{ID: "zpool_%s_health_state_suspended", Name: "suspended"},
		},
	}
)

func (z *ZFSPool) addZpoolCharts(name string) {
	charts := zpoolChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		chart.Labels = []module.Label{
			{Key: "pool", Value: name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := z.Charts().Add(*charts...); err != nil {
		z.Warning(err)
	}
}

func (z *ZFSPool) removeZpoolCharts(name string) {
	px := fmt.Sprintf("zpool_%s_", name)

	for _, chart := range *z.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
