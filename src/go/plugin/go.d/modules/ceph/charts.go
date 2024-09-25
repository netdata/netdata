// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioGeneralUsage = module.Priority + iota
	prioGeneralObjects
	prioGeneralBytes
	prioGeneralOperations
	prioGeneralLatency
	prioPoolUsage
	prioPoolObjects
	prioReadBytes
	prioWriteBytes
	prioReadOperations
	prioWriteOperations
	prioOsdUsage
	prioOsdSize
	prioApplyLatency
	prioCommitLatency
)

var generalCharts = module.Charts{
	generalUsageChart.Copy(),
	generalObjectsChart.Copy(),
	generalBytesChart.Copy(),
	generalOperationsChart.Copy(),
	generalLatencyChart.Copy(),
}

var poolChartsTmpl = module.Charts{
	poolUsageChartTmpl.Copy(),
	poolObjectsChartTmpl.Copy(),
	poolReadBytesChartTmpl.Copy(),
	poolWriteBytesChartTmpl.Copy(),
	poolReadOperationsChartTmpl.Copy(),
	poolWriteOperationChartTmpl.Copy(),
}

var osdChartsTmpl = module.Charts{
	osdUsageChartTmpl.Copy(),
	osdSizeChartTmpl.Copy(),
	osdApplyLatencyChartTmpl.Copy(),
	osdCommitLatencyChartTmpl.Copy(),
}

// General Charts
var (
	generalUsageChart = module.Chart{
		ID:       "general_usage",
		Title:    "Ceph General Space",
		Fam:      "general",
		Units:    "KiB",
		Ctx:      "ceph.general_usage",
		Type:     module.Stacked,
		Priority: prioGeneralUsage,
		Dims: module.Dims{
			{ID: "total_avail_bytes", Name: "avail"},
			{ID: "total_used_bytes", Name: "used"},
		},
	}

	generalObjectsChart = module.Chart{
		ID:       "general_objects",
		Title:    "Ceph General Objects",
		Fam:      "general",
		Units:    "objects",
		Ctx:      "ceph.general_objects",
		Type:     module.Area,
		Priority: prioGeneralObjects,
		Dims: module.Dims{
			{ID: "general_objects", Name: "cluster"},
		},
	}

	generalBytesChart = module.Chart{
		ID:       "general_bytes",
		Title:    "Ceph General Read/Write Data/s",
		Fam:      "general",
		Units:    "KiB/s",
		Ctx:      "ceph.general_bytes",
		Type:     module.Area,
		Priority: prioGeneralBytes,
		Dims: module.Dims{
			{ID: "general_read_bytes", Name: "read", Div: 1024},
			{ID: "general_write_bytes", Name: "write", Mul: -1, Div: 1024},
		},
	}

	generalOperationsChart = module.Chart{
		ID:       "general_operations",
		Title:    "Ceph General Read/Write Operations/s",
		Fam:      "general",
		Units:    "operations",
		Ctx:      "ceph.general_operations",
		Type:     module.Area,
		Priority: prioGeneralOperations,
		Dims: module.Dims{
			{ID: "general_read_operations", Name: "read"},
			{ID: "general_write_operations", Name: "write", Mul: -1},
		},
	}

	generalLatencyChart = module.Chart{
		ID:       "general_latency",
		Title:    "Ceph General Apply/Commit latency",
		Fam:      "general",
		Units:    "milliseconds",
		Ctx:      "ceph.general_latency",
		Type:     module.Area,
		Priority: prioGeneralLatency,
		Dims: module.Dims{
			{ID: "general_apply_latency", Name: "apply"},
			{ID: "general_commit_latency", Name: "commit"},
		},
	}
)

// Pool Charts
var (
	poolUsageChartTmpl = module.Chart{
		ID:       "%s_pool_usage",
		Title:    "Ceph Pools",
		Fam:      "pool",
		Units:    "KiB",
		Ctx:      "ceph.pool_usage",
		Type:     module.Line,
		Priority: prioPoolUsage,
		Dims: module.Dims{
			{ID: "%s_kb_used", Name: "%s"},
		},
	}

	poolObjectsChartTmpl = module.Chart{
		ID:       "%s_pool_objects",
		Title:    "Ceph Pool Objects",
		Fam:      "pool",
		Units:    "objects",
		Ctx:      "ceph.pool_objects",
		Type:     module.Line,
		Priority: prioPoolObjects,
		Dims: module.Dims{
			{ID: "%s_objects", Name: "%s"},
		},
	}

	poolReadBytesChartTmpl = module.Chart{
		ID:       "%s_pool_read_bytes",
		Title:    "Ceph Read Pool Data/s",
		Fam:      "pool",
		Units:    "KiB/s",
		Ctx:      "ceph.pool_read_bytes",
		Type:     module.Area,
		Priority: prioReadBytes,
		Dims: module.Dims{
			{ID: "%s_read_bytes", Name: "%s", Div: 1024},
		},
	}

	poolWriteBytesChartTmpl = module.Chart{
		ID:       "%s_pool_write_bytes",
		Title:    "Ceph Write Pool Data/s",
		Fam:      "pool",
		Units:    "KiB/s",
		Ctx:      "ceph.pool_write_bytes",
		Type:     module.Area,
		Priority: prioWriteBytes,
		Dims: module.Dims{
			{ID: "%s_write_bytes", Name: "%s", Div: 1024},
		},
	}

	poolReadOperationsChartTmpl = module.Chart{
		ID:       "%s_pool_read_operations",
		Title:    "Ceph Read Pool Operations/s",
		Fam:      "pool",
		Units:    "KiB/s",
		Ctx:      "ceph.pool_read_operations",
		Type:     module.Area,
		Priority: prioReadOperations,
		Dims: module.Dims{
			{ID: "%s_read_operations", Name: "%s"},
		},
	}

	poolWriteOperationChartTmpl = module.Chart{
		ID:       "%s_pool_write_operations",
		Title:    "Ceph Write Pool Operations/s",
		Fam:      "pool",
		Units:    "KiB/s",
		Ctx:      "ceph.pool_read_operations",
		Type:     module.Area,
		Priority: prioWriteOperations,
		Dims: module.Dims{
			{ID: "%s_write_operations", Name: "%s"},
		},
	}
)

// Osd Charts
var (
	osdUsageChartTmpl = module.Chart{
		ID:       "%s_osd_usage",
		Title:    "Ceph OSDs",
		Fam:      "osd",
		Units:    "KiB",
		Ctx:      "ceph.osd_usage",
		Type:     module.Line,
		Priority: prioOsdUsage,
		Dims: module.Dims{
			{ID: "%s_usage", Name: "%s"},
		},
	}

	osdSizeChartTmpl = module.Chart{
		ID:       "%s_osd_size",
		Title:    "Ceph OSDs size",
		Fam:      "osd",
		Units:    "KiB",
		Ctx:      "ceph.osd_size",
		Type:     module.Line,
		Priority: prioOsdSize,
		Dims: module.Dims{
			{ID: "%s_size", Name: "%s"},
		},
	}

	osdApplyLatencyChartTmpl = module.Chart{
		ID:       "%s_osd_apply_latency",
		Title:    "Ceph OSDs apply latency",
		Fam:      "osd",
		Units:    "milliseconds",
		Ctx:      "ceph.apply_latency",
		Type:     module.Line,
		Priority: prioApplyLatency,
		Dims: module.Dims{
			{ID: "%s_apply_latency", Name: "%s"},
		},
	}
	osdCommitLatencyChartTmpl = module.Chart{
		ID:       "%s_osd_commit_latency",
		Title:    "Ceph OSDs commit latency",
		Fam:      "osd",
		Units:    "milliseconds",
		Ctx:      "ceph.commit_latency",
		Type:     module.Line,
		Priority: prioCommitLatency,
		Dims: module.Dims{
			{ID: "%s_commit_latency", Name: "%s"},
		},
	}
)

func (c *Ceph) addPoolCharts(poolName string) {
	charts := poolChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, poolName)
		chart.Labels = []module.Label{
			{Key: "pool", Value: poolName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, poolName)
			dim.Name = fmt.Sprintf(dim.Name, poolName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}

}

func (c *Ceph) addOsdCharts(osdUuid, devClass, osdName string) {
	charts := osdChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, osdID)
		chart.Labels = []module.Label{
			{Key: "osd", Value: osdID},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, osdID)
			dim.Name = fmt.Sprintf(dim.Name, osdID)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}

}

func (c *Ceph) removePoolCharts(poolName string) {
	px := fmt.Sprintf("%s_", poolName)
	c.removeCharts(px)
}

func (c *Ceph) removeOsdCharts(name string) {
	px := fmt.Sprintf("%s_", name)
	c.removeCharts(px)
}

func (c *Ceph) removeCharts(prefix string) {
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
