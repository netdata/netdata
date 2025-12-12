// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"maps"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

const (
	prioProfileChart = module.Priority + iota
	prioPingRtt
	prioPingStdDev

	prioInternalStatsTimings
	prioInternalStatsSnmpOps
	prioInternalStatsMetrics
	prioInternalStatsTableCache
	prioInternalStatsErrors
)

var (
	pingCharts = module.Charts{
		pingRTTChart.Copy(),
		pingRTTStdDevChart.Copy(),
	}
	pingRTTChart = module.Chart{
		ID:       "ping_rtt",
		Title:    "Ping round-trip time",
		Units:    "milliseconds",
		Fam:      "Ping/RTT",
		Ctx:      "snmp.device_ping_rtt",
		Priority: prioPingRtt,
		Type:     module.Area,
		Dims: module.Dims{
			{ID: "ping_rtt_min", Name: "min", Div: 1e3},
			{ID: "ping_rtt_max", Name: "max", Div: 1e3},
			{ID: "ping_rtt_avg", Name: "avg", Div: 1e3},
		},
	}
	pingRTTStdDevChart = module.Chart{
		ID:       "ping_rtt_stddev",
		Title:    "Ping round-trip time standard deviation",
		Units:    "milliseconds",
		Fam:      "Ping/RTT",
		Ctx:      "snmp.device_ping_rtt_stddev",
		Priority: prioPingStdDev,
		Dims: module.Dims{
			{ID: "ping_rtt_stddev", Name: "stddev", Div: 1e3},
		},
	}
)

func (c *Collector) addPingCharts() {
	charts := pingCharts.Copy()

	labels := c.chartBaseLabels()

	for _, chart := range *charts {
		for k, v := range labels {
			chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warningf("failed to add ping charts: %v", err)
	}
}

var (
	profileStatsChartsTmpl = module.Charts{
		profileStatsTimingsChartTmpl.Copy(),
		profileStatsSnmpChartTmpl.Copy(),
		profileStatsMetricsChartTmpl.Copy(),
		profileStatsTableCacheChartTmpl.Copy(),
		profileStatsErrorsChartTmpl.Copy(),
	}

	profileStatsTimingsChartTmpl = module.Chart{
		ID:       "snmp_device_prof_%s_stats_timings",
		Title:    "SNMP profile collection timings",
		Units:    "milliseconds",
		Fam:      "Internal/Stats",
		Ctx:      "snmp.device_prof_stats_timings",
		Priority: prioInternalStatsTimings,
		Dims: module.Dims{
			{ID: "snmp_device_prof_%s_stats_timings_scalar", Name: "scalar"},
			{ID: "snmp_device_prof_%s_stats_timings_table", Name: "table"},
			{ID: "snmp_device_prof_%s_stats_timings_virtual", Name: "virtual"},
		},
	}

	profileStatsSnmpChartTmpl = module.Chart{
		ID:       "snmp_device_prof_%s_stats_snmp",
		Title:    "SNMP profile operations",
		Units:    "operations",
		Fam:      "Internal/Stats",
		Ctx:      "snmp.device_prof_stats_snmp",
		Priority: prioInternalStatsSnmpOps,
		Dims: module.Dims{
			{ID: "snmp_device_prof_%s_stats_snmp_get_requests", Name: "get_requests"},
			{ID: "snmp_device_prof_%s_stats_snmp_get_oids", Name: "get_oids"},
			{ID: "snmp_device_prof_%s_stats_snmp_walk_requests", Name: "walk_requests"},
			{ID: "snmp_device_prof_%s_stats_snmp_walk_pdus", Name: "walk_pdus"},
			{ID: "snmp_device_prof_%s_stats_snmp_tables_walked", Name: "tables_walked"},
			{ID: "snmp_device_prof_%s_stats_snmp_tables_cached", Name: "tables_cached"},
		},
	}

	profileStatsMetricsChartTmpl = module.Chart{
		ID:       "snmp_device_prof_%s_stats_metrics",
		Title:    "SNMP profile metric counts",
		Units:    "metrics",
		Fam:      "Internal/Stats",
		Ctx:      "snmp.device_prof_stats_metrics",
		Priority: prioInternalStatsMetrics,
		Dims: module.Dims{
			{ID: "snmp_device_prof_%s_stats_metrics_scalar", Name: "scalar"},
			{ID: "snmp_device_prof_%s_stats_metrics_table", Name: "table"},
			{ID: "snmp_device_prof_%s_stats_metrics_virtual", Name: "virtual"},
			{ID: "snmp_device_prof_%s_stats_metrics_tables", Name: "tables"},
			{ID: "snmp_device_prof_%s_stats_metrics_rows", Name: "rows"},
		},
	}

	profileStatsTableCacheChartTmpl = module.Chart{
		ID:       "snmp_device_prof_%s_stats_table_cache",
		Title:    "SNMP profile table cache",
		Units:    "tables",
		Fam:      "Internal/Stats",
		Ctx:      "snmp.device_prof_stats_table_cache",
		Priority: prioInternalStatsTableCache,
		Dims: module.Dims{
			{ID: "snmp_device_prof_%s_stats_table_cache_hits", Name: "hits"},
			{ID: "snmp_device_prof_%s_stats_table_cache_misses", Name: "misses"},
		},
	}

	profileStatsErrorsChartTmpl = module.Chart{
		ID:       "snmp_device_prof_%s_stats_errors",
		Title:    "SNMP profile errors",
		Units:    "errors",
		Fam:      "Internal/Stats",
		Ctx:      "snmp.device_prof_stats_errors",
		Priority: prioInternalStatsErrors,
		Dims: module.Dims{
			{ID: "snmp_device_prof_%s_stats_errors_snmp", Name: "snmp"},
			{ID: "snmp_device_prof_%s_stats_errors_processing_scalar", Name: "processing_scalar"},
			{ID: "snmp_device_prof_%s_stats_errors_processing_table", Name: "processing_table"},
		},
	}
)

func (c *Collector) addProfileStatsCharts(name string) {
	if name == "" {
		return
	}

	charts := profileStatsChartsTmpl.Copy()

	labels := c.chartBaseLabels()
	labels["profile"] = name

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, name)
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, name)
		}
		for k, v := range labels {
			chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warningf("failed to add profile stats charts for %s: %v", name, err)
	}
}

func (c *Collector) addProfileScalarMetricChart(m ddsnmp.Metric) {
	if m.Name == "" {
		return
	}

	chart := &module.Chart{
		ID:       fmt.Sprintf("snmp_device_prof_%s", cleanMetricName.Replace(m.Name)),
		Title:    m.Description,
		Type:     module.ChartType(m.ChartType),
		Units:    m.Unit,
		Fam:      m.Family,
		Ctx:      fmt.Sprintf("snmp.device_prof_%s", cleanMetricName.Replace(m.Name)),
		Priority: prioProfileChart,
	}
	if chart.Title == "" {
		chart.Title = fmt.Sprintf("SNMP metric %s", m.Name)
	}
	if chart.Units == "" {
		chart.Units = "1"
	}
	if chart.Fam == "" {
		chart.Fam = m.Name
	}
	if chart.Units == "bit/s" {
		chart.Type = module.Area
	}

	tags := c.chartBaseLabels()

	maps.Copy(tags, m.Profile.Tags)
	for k, v := range tags {
		chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
	}

	if len(m.MultiValue) > 0 {
		seen := make(map[string]bool)
		for k := range m.MultiValue {
			if !seen[k] {
				seen[k] = true
				id := fmt.Sprintf("snmp_device_prof_%s_%s", m.Name, k)
				chart.Dims = append(chart.Dims, &module.Dim{ID: id, Name: k, Algo: dimAlgoFromDdSnmpType(m)})
			}
		}
	} else {
		id := fmt.Sprintf("snmp_device_prof_%s", m.Name)
		chart.Dims = module.Dims{
			{ID: id, Name: m.Name, Algo: dimAlgoFromDdSnmpType(m)},
		}
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addProfileTableMetricChart(m ddsnmp.Metric) {
	if m.Name == "" {
		return
	}

	key := tableMetricKey(m)

	chart := &module.Chart{
		ID:       fmt.Sprintf("snmp_device_prof_%s", cleanMetricName.Replace(key)),
		Title:    m.Description,
		Units:    m.Unit,
		Fam:      m.Family,
		Ctx:      fmt.Sprintf("snmp.device_prof_%s", cleanMetricName.Replace(m.Name)),
		Priority: prioProfileChart,
	}
	if chart.Title == "" {
		chart.Title = fmt.Sprintf("SNMP metric %s", m.Name)
	}
	if chart.Units == "" {
		chart.Units = "1"
	}
	if chart.Fam == "" {
		chart.Fam = m.Name
	}
	if chart.Units == "bit/s" {
		chart.Type = module.Area
	}

	tags := c.chartBaseLabels()

	maps.Copy(tags, m.Profile.Tags)
	for k, v := range m.Tags {
		newKey := strings.TrimPrefix(k, "_")
		tags[newKey] = v
	}

	for k, v := range tags {
		chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
	}

	if len(m.MultiValue) > 0 {
		seen := make(map[string]bool)
		for k := range m.MultiValue {
			if !seen[k] {
				seen[k] = true
				id := fmt.Sprintf("snmp_device_prof_%s_%s", key, k)
				chart.Dims = append(chart.Dims, &module.Dim{ID: id, Name: k, Algo: dimAlgoFromDdSnmpType(m)})
			}
		}
	} else {
		id := fmt.Sprintf("snmp_device_prof_%s", key)
		chart.Dims = module.Dims{
			{ID: id, Name: m.Name, Algo: dimAlgoFromDdSnmpType(m)},
		}
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeProfileTableMetricChart(key string) {
	id := fmt.Sprintf("snmp_device_prof_%s", cleanMetricName.Replace(key))
	if chart := c.Charts().Get(id); chart != nil {
		chart.MarkRemove()
		chart.MarkNotCreated()
	}
}

func (c *Collector) chartBaseLabels() map[string]string {
	si := c.sysInfo

	labels := map[string]string{
		"sysName": si.Name,
		"address": c.Hostname,
	}

	if si.Vendor != "" {
		labels["vendor"] = si.Vendor
	} else if si.Organization != "" {
		labels["vendor"] = si.Organization
	}
	if si.Category != "" {
		labels["device_type"] = si.Category
	}
	if si.Model != "" {
		labels["model"] = si.Model
	}

	return labels
}

func dimAlgoFromDdSnmpType(m ddsnmp.Metric) module.DimAlgo {
	switch m.MetricType {
	case ddprofiledefinition.ProfileMetricTypeGauge,
		ddprofiledefinition.ProfileMetricTypeMonotonicCount,
		ddprofiledefinition.ProfileMetricTypeMonotonicCountAndRate:
		return module.Absolute
	default:
		return module.Incremental
	}
}

var cleanMetricName = strings.NewReplacer(".", "_", " ", "_")
