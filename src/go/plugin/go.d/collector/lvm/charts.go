// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || netbsd

package lvm

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioLVDataPercent = 2920 + iota
	prioLVMetadataPercent
)

var lvThinPoolChartsTmpl = collectorapi.Charts{
	lvDataSpaceUtilizationChartTmpl.Copy(),
	lvMetadataSpaceUtilizationChartTmpl.Copy(),
}

var (
	lvDataSpaceUtilizationChartTmpl = collectorapi.Chart{
		ID:       "lv_%s_vg_%s_lv_data_space_utilization",
		Title:    "Logical volume space allocated for data",
		Units:    "percentage",
		Fam:      "lv space usage",
		Ctx:      "lvm.lv_data_space_utilization",
		Type:     collectorapi.Area,
		Priority: prioLVDataPercent,
		Dims: collectorapi.Dims{
			{ID: "lv_%s_vg_%s_data_percent", Name: "utilization", Div: 100},
		},
	}
	lvMetadataSpaceUtilizationChartTmpl = collectorapi.Chart{
		ID:       "lv_%s_vg_%s_lv_metadata_space_utilization",
		Title:    "Logical volume space allocated for metadata",
		Units:    "percentage",
		Fam:      "lv space usage",
		Ctx:      "lvm.lv_metadata_space_utilization",
		Type:     collectorapi.Area,
		Priority: prioLVMetadataPercent,
		Dims: collectorapi.Dims{
			{ID: "lv_%s_vg_%s_metadata_percent", Name: "utilization", Div: 100},
		},
	}
)

func (c *Collector) addLVMThinPoolCharts(lvName, vgName string) {
	charts := lvThinPoolChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, lvName, vgName)
		chart.Labels = []collectorapi.Label{
			{Key: "lv_name", Value: lvName},
			{Key: "vg_name", Value: vgName},
			{Key: "volume_type", Value: "thin_pool"},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, lvName, vgName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}
