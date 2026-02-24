// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package zfspool

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioZpoolHealthState = 2820 + iota
	prioVdevHealthState

	prioZpoolSpaceUtilization
	prioZpoolSpaceUsage

	prioZpoolFragmentation
)

var zpoolChartsTmpl = collectorapi.Charts{
	zpoolHealthStateChartTmpl.Copy(),

	zpoolSpaceUtilizationChartTmpl.Copy(),
	zpoolSpaceUsageChartTmpl.Copy(),

	zpoolFragmentationChartTmpl.Copy(),
}

var (
	zpoolHealthStateChartTmpl = collectorapi.Chart{
		ID:       "zfspool_%s_health_state",
		Title:    "Zpool health state",
		Units:    "state",
		Fam:      "health",
		Ctx:      "zfspool.pool_health_state",
		Type:     collectorapi.Line,
		Priority: prioZpoolHealthState,
		Dims: collectorapi.Dims{
			{ID: "zpool_%s_health_state_online", Name: "online"},
			{ID: "zpool_%s_health_state_degraded", Name: "degraded"},
			{ID: "zpool_%s_health_state_faulted", Name: "faulted"},
			{ID: "zpool_%s_health_state_offline", Name: "offline"},
			{ID: "zpool_%s_health_state_unavail", Name: "unavail"},
			{ID: "zpool_%s_health_state_removed", Name: "removed"},
			{ID: "zpool_%s_health_state_suspended", Name: "suspended"},
		},
	}

	zpoolSpaceUtilizationChartTmpl = collectorapi.Chart{
		ID:       "zfspool_%s_space_utilization",
		Title:    "Zpool space utilization",
		Units:    "percentage",
		Fam:      "space usage",
		Ctx:      "zfspool.pool_space_utilization",
		Type:     collectorapi.Area,
		Priority: prioZpoolSpaceUtilization,
		Dims: collectorapi.Dims{
			{ID: "zpool_%s_cap", Name: "utilization"},
		},
	}
	zpoolSpaceUsageChartTmpl = collectorapi.Chart{
		ID:       "zfspool_%s_space_usage",
		Title:    "Zpool space usage",
		Units:    "bytes",
		Fam:      "space usage",
		Ctx:      "zfspool.pool_space_usage",
		Type:     collectorapi.Stacked,
		Priority: prioZpoolSpaceUsage,
		Dims: collectorapi.Dims{
			{ID: "zpool_%s_free", Name: "free"},
			{ID: "zpool_%s_alloc", Name: "used"},
		},
	}

	zpoolFragmentationChartTmpl = collectorapi.Chart{
		ID:       "zfspool_%s_fragmentation",
		Title:    "Zpool fragmentation",
		Units:    "percentage",
		Fam:      "fragmentation",
		Ctx:      "zfspool.pool_fragmentation",
		Type:     collectorapi.Line,
		Priority: prioZpoolFragmentation,
		Dims: collectorapi.Dims{
			{ID: "zpool_%s_frag", Name: "fragmentation"},
		},
	}
)

var vdevChartsTmpl = collectorapi.Charts{
	vdevHealthStateChartTmpl.Copy(),
}

var (
	vdevHealthStateChartTmpl = collectorapi.Chart{
		ID:       "vdev_%s_health_state",
		Title:    "Zpool Vdev health state",
		Units:    "state",
		Fam:      "health",
		Ctx:      "zfspool.vdev_health_state",
		Type:     collectorapi.Line,
		Priority: prioVdevHealthState,
		Dims: collectorapi.Dims{
			{ID: "vdev_%s_health_state_online", Name: "online"},
			{ID: "vdev_%s_health_state_degraded", Name: "degraded"},
			{ID: "vdev_%s_health_state_faulted", Name: "faulted"},
			{ID: "vdev_%s_health_state_offline", Name: "offline"},
			{ID: "vdev_%s_health_state_unavail", Name: "unavail"},
			{ID: "vdev_%s_health_state_removed", Name: "removed"},
			{ID: "vdev_%s_health_state_suspended", Name: "suspended"},
		},
	}
)

func (c *Collector) addZpoolCharts(name string) {
	charts := zpoolChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		chart.Labels = []collectorapi.Label{
			{Key: "pool", Value: name},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeZpoolCharts(name string) {
	px := fmt.Sprintf("zfspool_%s_", name)
	c.removeCharts(px)
}

func (c *Collector) addVdevCharts(pool, vdev string) {
	charts := vdevChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanVdev(vdev))
		chart.Labels = []collectorapi.Label{
			{Key: "pool", Value: pool},
			{Key: "vdev", Value: vdev},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, vdev)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeVdevCharts(vdev string) {
	px := fmt.Sprintf("vdev_%s_", cleanVdev(vdev))
	c.removeCharts(px)
}

func (c *Collector) removeCharts(px string) {
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanVdev(vdev string) string {
	r := strings.NewReplacer(".", "_")
	return r.Replace(vdev)
}
