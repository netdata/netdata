// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dmcache

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioDeviceCacheSpaceUsage = collectorapi.Priority + iota
	prioDeviceMetaSpaceUsage
	prioDeviceReadEfficiency
	prioDeviceWriteEfficiency
	prioDeviceActivity
	prioDeviceDirty
)

var deviceChartsTmpl = collectorapi.Charts{
	chartDeviceCacheSpaceUsageTmpl.Copy(),
	chartDeviceMetadataSpaceUsageTmpl.Copy(),

	chartDeviceReadEfficiencyTmpl.Copy(),
	chartDeviceWriteEfficiencyTmpl.Copy(),

	chartDeviceActivityTmpl.Copy(),

	chartDeviceDirtySizeTmpl.Copy(),
}

var (
	chartDeviceCacheSpaceUsageTmpl = collectorapi.Chart{
		ID:       "dmcache_device_%s_cache_space_usage",
		Title:    "DMCache space usage",
		Units:    "bytes",
		Fam:      "space usage",
		Ctx:      "dmcache.device_cache_space_usage",
		Type:     collectorapi.Stacked,
		Priority: prioDeviceCacheSpaceUsage,
		Dims: collectorapi.Dims{
			{ID: "dmcache_device_%s_cache_free_bytes", Name: "free"},
			{ID: "dmcache_device_%s_cache_used_bytes", Name: "used"},
		},
	}
	chartDeviceMetadataSpaceUsageTmpl = collectorapi.Chart{
		ID:       "dmcache_device_%s_metadata_space_usage",
		Title:    "DMCache metadata space usage",
		Units:    "bytes",
		Fam:      "space usage",
		Ctx:      "dmcache.device_metadata_space_usage",
		Type:     collectorapi.Stacked,
		Priority: prioDeviceMetaSpaceUsage,
		Dims: collectorapi.Dims{
			{ID: "dmcache_device_%s_metadata_free_bytes", Name: "free"},
			{ID: "dmcache_device_%s_metadata_used_bytes", Name: "used"},
		},
	}
)

var (
	chartDeviceReadEfficiencyTmpl = collectorapi.Chart{
		ID:       "dmcache_device_%s_read_efficiency",
		Title:    "DMCache read efficiency",
		Units:    "requests/s",
		Fam:      "efficiency",
		Ctx:      "dmcache.device_cache_read_efficiency",
		Type:     collectorapi.Stacked,
		Priority: prioDeviceReadEfficiency,
		Dims: collectorapi.Dims{
			{ID: "dmcache_device_%s_read_hits", Name: "hits", Algo: collectorapi.Incremental},
			{ID: "dmcache_device_%s_read_misses", Name: "misses", Algo: collectorapi.Incremental},
		},
	}
	chartDeviceWriteEfficiencyTmpl = collectorapi.Chart{
		ID:       "dmcache_device_%s_write_efficiency",
		Title:    "DMCache write efficiency",
		Units:    "requests/s",
		Fam:      "efficiency",
		Ctx:      "dmcache.device_cache_write_efficiency",
		Type:     collectorapi.Stacked,
		Priority: prioDeviceWriteEfficiency,
		Dims: collectorapi.Dims{
			{ID: "dmcache_device_%s_write_hits", Name: "hits", Algo: collectorapi.Incremental},
			{ID: "dmcache_device_%s_write_misses", Name: "misses", Algo: collectorapi.Incremental},
		},
	}
)

var chartDeviceActivityTmpl = collectorapi.Chart{
	ID:       "dmcache_device_%s_activity",
	Title:    "DMCache activity",
	Units:    "bytes/s",
	Fam:      "activity",
	Ctx:      "dmcache.device_cache_activity",
	Type:     collectorapi.Area,
	Priority: prioDeviceActivity,
	Dims: collectorapi.Dims{
		{ID: "dmcache_device_%s_promotions_bytes", Name: "promotions", Algo: collectorapi.Incremental},
		{ID: "dmcache_device_%s_demotions_bytes", Name: "demotions", Mul: -1, Algo: collectorapi.Incremental},
	},
}

var chartDeviceDirtySizeTmpl = collectorapi.Chart{
	ID:       "dmcache_device_%s_dirty_size",
	Title:    "DMCache dirty data size",
	Units:    "bytes",
	Fam:      "dirty size",
	Ctx:      "dmcache.device_cache_dirty_size",
	Type:     collectorapi.Area,
	Priority: prioDeviceDirty,
	Dims: collectorapi.Dims{
		{ID: "dmcache_device_%s_dirty_bytes", Name: "dirty"},
	},
}

func (c *Collector) addDeviceCharts(device string) {
	charts := deviceChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, cleanDeviceName(device))
		chart.Labels = []collectorapi.Label{
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
