// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dmcache

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioDeviceCacheSpaceUsage = module.Priority + iota
	prioDeviceMetaSpaceUsage
	prioDeviceReadEfficiency
	prioDeviceWriteEfficiency
	prioDeviceActivity
	prioDeviceDirty
)

var deviceChartsTmpl = module.Charts{
	chartDeviceCacheSpaceUsageTmpl.Copy(),
	chartDeviceMetadataSpaceUsageTmpl.Copy(),

	chartDeviceReadEfficiencyTmpl.Copy(),
	chartDeviceWriteEfficiencyTmpl.Copy(),

	chartDeviceActivityTmpl.Copy(),

	chartDeviceDirtySizeTmpl.Copy(),
}

var (
	chartDeviceCacheSpaceUsageTmpl = module.Chart{
		ID:       "dmcache_device_%s_cache_space_usage",
		Title:    "DMCache space usage",
		Units:    "bytes",
		Fam:      "space usage",
		Ctx:      "dmcache.device_cache_space_usage",
		Type:     module.Stacked,
		Priority: prioDeviceCacheSpaceUsage,
		Dims: module.Dims{
			{ID: "dmcache_device_%s_cache_free_bytes", Name: "free"},
			{ID: "dmcache_device_%s_cache_used_bytes", Name: "used"},
		},
	}
	chartDeviceMetadataSpaceUsageTmpl = module.Chart{
		ID:       "dmcache_device_%s_metadata_space_usage",
		Title:    "DMCache metadata space usage",
		Units:    "bytes",
		Fam:      "space usage",
		Ctx:      "dmcache.device_metadata_space_usage",
		Type:     module.Stacked,
		Priority: prioDeviceMetaSpaceUsage,
		Dims: module.Dims{
			{ID: "dmcache_device_%s_metadata_free_bytes", Name: "free"},
			{ID: "dmcache_device_%s_metadata_used_bytes", Name: "used"},
		},
	}
)

var (
	chartDeviceReadEfficiencyTmpl = module.Chart{
		ID:       "dmcache_device_%s_read_efficiency",
		Title:    "DMCache read efficiency",
		Units:    "requests/s",
		Fam:      "efficiency",
		Ctx:      "dmcache.device_cache_read_efficiency",
		Type:     module.Stacked,
		Priority: prioDeviceReadEfficiency,
		Dims: module.Dims{
			{ID: "dmcache_device_%s_read_hits", Name: "hits", Algo: module.Incremental},
			{ID: "dmcache_device_%s_read_misses", Name: "misses", Algo: module.Incremental},
		},
	}
	chartDeviceWriteEfficiencyTmpl = module.Chart{
		ID:       "dmcache_device_%s_write_efficiency",
		Title:    "DMCache write efficiency",
		Units:    "requests/s",
		Fam:      "efficiency",
		Ctx:      "dmcache.device_cache_write_efficiency",
		Type:     module.Stacked,
		Priority: prioDeviceWriteEfficiency,
		Dims: module.Dims{
			{ID: "dmcache_device_%s_write_hits", Name: "hits", Algo: module.Incremental},
			{ID: "dmcache_device_%s_write_misses", Name: "misses", Algo: module.Incremental},
		},
	}
)

var chartDeviceActivityTmpl = module.Chart{
	ID:       "dmcache_device_%s_activity",
	Title:    "DMCache activity",
	Units:    "bytes/s",
	Fam:      "activity",
	Ctx:      "dmcache.device_cache_activity",
	Type:     module.Area,
	Priority: prioDeviceActivity,
	Dims: module.Dims{
		{ID: "dmcache_device_%s_promotions_bytes", Name: "promotions", Algo: module.Incremental},
		{ID: "dmcache_device_%s_demotions_bytes", Name: "demotions", Mul: -1, Algo: module.Incremental},
	},
}

var chartDeviceDirtySizeTmpl = module.Chart{
	ID:       "dmcache_device_%s_dirty_size",
	Title:    "DMCache dirty data size",
	Units:    "bytes",
	Fam:      "dirty size",
	Ctx:      "dmcache.device_cache_dirty_size",
	Type:     module.Area,
	Priority: prioDeviceDirty,
	Dims: module.Dims{
		{ID: "dmcache_device_%s_dirty_bytes", Name: "dirty"},
	},
}

func (c *Collector) addDeviceCharts(device string) {
	charts := deviceChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanDeviceName(device))
		chart.Labels = []module.Label{
			{Key: "device", Value: device},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, device)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeDeviceCharts(device string) {
	px := fmt.Sprintf("dmcache_device_%s_", cleanDeviceName(device))

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanDeviceName(device string) string {
	return strings.ReplaceAll(device, ".", "_")
}
