// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

var rpkiCacheStateChartTmpl = Chart{
	ID:       "rpki_%s_state",
	Title:    "BGP RPKI cache state",
	Units:    "state",
	Fam:      "%s",
	Ctx:      "bgp.rpki_cache_state",
	Type:     collectorapi.Stacked,
	Priority: prioRPKICacheState,
	Dims: Dims{
		{ID: "rpki_%s_up", Name: "up"},
		{ID: "rpki_%s_down", Name: "down"},
	},
}

var rpkiCacheUptimeChartTmpl = Chart{
	ID:       "rpki_%s_uptime",
	Title:    "BGP RPKI cache uptime",
	Units:    "seconds",
	Fam:      "%s",
	Ctx:      "bgp.rpki_cache_uptime",
	Priority: prioRPKICacheUptime,
	Dims: Dims{
		{ID: "rpki_%s_uptime_seconds", Name: "uptime"},
	},
}

var rpkiCacheRecordsChartTmpl = Chart{
	ID:       "rpki_%s_records",
	Title:    "BGP RPKI cache records",
	Units:    "records",
	Fam:      "%s",
	Ctx:      "bgp.rpki_cache_records",
	Type:     collectorapi.Stacked,
	Priority: prioRPKICacheRecords,
	Dims: Dims{
		{ID: "rpki_%s_record_ipv4", Name: "ipv4"},
		{ID: "rpki_%s_record_ipv6", Name: "ipv6"},
	},
}

var rpkiCachePrefixesChartTmpl = Chart{
	ID:       "rpki_%s_prefixes",
	Title:    "BGP RPKI cache prefixes",
	Units:    "prefixes",
	Fam:      "%s",
	Ctx:      "bgp.rpki_cache_prefixes",
	Type:     collectorapi.Stacked,
	Priority: prioRPKICachePrefixes,
	Dims: Dims{
		{ID: "rpki_%s_prefix_ipv4", Name: "ipv4"},
		{ID: "rpki_%s_prefix_ipv6", Name: "ipv6"},
	},
}

func (c *Collector) addRPKICacheCharts(cache rpkiCacheStats) {
	if c.Charts().Has(rpkiCacheStateChartID(cache.ID)) {
		c.addOptionalRPKICacheCharts(cache)
		return
	}

	charts := Charts{
		rpkiCacheStateChartTmpl.Copy(),
	}
	if cache.HasUptime {
		charts = append(charts, rpkiCacheUptimeChartTmpl.Copy())
	}
	fam := rpkiCacheDisplay(cache)
	labels := rpkiCacheLabels(cache)

	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cache.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, fam)
		chart.Labels = labels
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cache.ID)
		}
	}

	if err := c.Charts().Add(charts...); err != nil {
		c.Warning(err)
	}

	c.addOptionalRPKICacheCharts(cache)
}

func (c *Collector) addOptionalRPKICacheCharts(cache rpkiCacheStats) {
	charts := make(Charts, 0, 3)
	if cache.HasUptime && !c.Charts().Has(rpkiCacheUptimeChartID(cache.ID)) {
		charts = append(charts, rpkiCacheUptimeChartTmpl.Copy())
	}
	if cache.HasRecords && !c.Charts().Has(rpkiCacheRecordsChartID(cache.ID)) {
		charts = append(charts, rpkiCacheRecordsChartTmpl.Copy())
	}
	if cache.HasPrefixes && !c.Charts().Has(rpkiCachePrefixesChartID(cache.ID)) {
		charts = append(charts, rpkiCachePrefixesChartTmpl.Copy())
	}
	if len(charts) == 0 {
		return
	}

	fam := rpkiCacheDisplay(cache)
	labels := rpkiCacheLabels(cache)
	for _, chart := range charts {
		chart.ID = fmt.Sprintf(chart.ID, cache.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, fam)
		chart.Labels = labels
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, cache.ID)
		}
	}

	if err := c.Charts().Add(charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeRPKICacheCharts(id string) {
	c.removeCharts("rpki_" + id)
}

func rpkiCacheStateChartID(cacheID string) string {
	return fmt.Sprintf(rpkiCacheStateChartTmpl.ID, cacheID)
}

func rpkiCacheUptimeChartID(cacheID string) string {
	return fmt.Sprintf(rpkiCacheUptimeChartTmpl.ID, cacheID)
}

func rpkiCacheRecordsChartID(cacheID string) string {
	return fmt.Sprintf(rpkiCacheRecordsChartTmpl.ID, cacheID)
}

func rpkiCachePrefixesChartID(cacheID string) string {
	return fmt.Sprintf(rpkiCachePrefixesChartTmpl.ID, cacheID)
}

func rpkiCacheLabels(cache rpkiCacheStats) []collectorapi.Label {
	labels := []collectorapi.Label{
		{Key: "backend", Value: cache.Backend},
		{Key: "cache", Value: cache.Name},
	}
	if cache.Desc != "" {
		labels = append(labels, collectorapi.Label{Key: "cache_desc", Value: cache.Desc})
	}
	if cache.StateText != "" {
		labels = append(labels, collectorapi.Label{Key: "state_text", Value: cache.StateText})
	}
	return labels
}
