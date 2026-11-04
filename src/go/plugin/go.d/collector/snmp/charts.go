// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"maps"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

const (
	prioProfileChart = collectorapi.Priority + iota
	prioPingRtt
	prioPingStdDev
)

const (
	prioInternalStatsTimings = prioLicenseState + 1 + iota
	prioInternalStatsSnmpOps
	prioInternalStatsMetrics
	prioInternalStatsTableCache
	prioInternalStatsErrors
)

var (
	pingCharts = collectorapi.Charts{
		pingRTTChart.Copy(),
		pingRTTStdDevChart.Copy(),
	}
	pingRTTChart = collectorapi.Chart{
		ID:       "ping_rtt",
		Title:    "Ping round-trip time",
		Units:    "milliseconds",
		Fam:      "Ping/RTT",
		Ctx:      "snmp.device_ping_rtt",
		Priority: prioPingRtt,
		Type:     collectorapi.Area,
		Dims: collectorapi.Dims{
			{ID: "ping_rtt_min", Name: "min", Div: 1e3},
			{ID: "ping_rtt_max", Name: "max", Div: 1e3},
			{ID: "ping_rtt_avg", Name: "avg", Div: 1e3},
		},
	}
	pingRTTStdDevChart = collectorapi.Chart{
		ID:       "ping_rtt_stddev",
		Title:    "Ping round-trip time standard deviation",
		Units:    "milliseconds",
		Fam:      "Ping/RTT",
		Ctx:      "snmp.device_ping_rtt_stddev",
		Priority: prioPingStdDev,
		Dims: collectorapi.Dims{
			{ID: "ping_rtt_stddev", Name: "stddev", Div: 1e3},
		},
	}
)

func (c *Collector) addPingCharts() {
	charts := pingCharts.Copy()

	labels := c.chartBaseLabels()

	for _, chart := range *charts {
		for k, v := range labels {
			chart.Labels = append(chart.Labels, collectorapi.Label{
				Key:   k,
				Value: v,
			})
		}
	}

	for _, chart := range *charts {
		c.installChart(chart)
	}
}

var (
	profileStatsChartsTmpl = collectorapi.Charts{
		profileStatsTimingsChartTmpl.Copy(),
		profileStatsSnmpChartTmpl.Copy(),
		profileStatsMetricsChartTmpl.Copy(),
		profileStatsTableCacheChartTmpl.Copy(),
		profileStatsErrorsChartTmpl.Copy(),
	}

	profileStatsTimingsChartTmpl = collectorapi.Chart{
		ID:       "snmp_device_prof_%s_stats_timings",
		Title:    "SNMP profile collection timings",
		Units:    "milliseconds",
		Fam:      "Internal/Stats",
		Ctx:      "snmp.device_prof_stats_timings",
		Priority: prioInternalStatsTimings,
		Dims: collectorapi.Dims{
			{ID: "snmp_device_prof_%s_stats_timings_scalar", Name: "scalar"},
			{ID: "snmp_device_prof_%s_stats_timings_preparation", Name: "preparation"},
			{ID: "snmp_device_prof_%s_stats_timings_table", Name: "table"},
			{ID: "snmp_device_prof_%s_stats_timings_licensing", Name: "licensing"},
			{ID: "snmp_device_prof_%s_stats_timings_bgp", Name: "bgp"},
			{ID: "snmp_device_prof_%s_stats_timings_virtual", Name: "virtual"},
		},
	}

	profileStatsSnmpChartTmpl = collectorapi.Chart{
		ID:       "snmp_device_prof_%s_stats_snmp",
		Title:    "SNMP profile operations",
		Units:    "operations",
		Fam:      "Internal/Stats",
		Ctx:      "snmp.device_prof_stats_snmp",
		Priority: prioInternalStatsSnmpOps,
		Dims: collectorapi.Dims{
			{ID: "snmp_device_prof_%s_stats_snmp_get_requests", Name: "get_requests"},
			{ID: "snmp_device_prof_%s_stats_snmp_get_oids", Name: "get_oids"},
			{ID: "snmp_device_prof_%s_stats_snmp_walk_requests", Name: "walk_requests"},
			{ID: "snmp_device_prof_%s_stats_snmp_walk_pdus", Name: "walk_pdus"},
			{ID: "snmp_device_prof_%s_stats_snmp_tables_walked", Name: "tables_walked"},
			{ID: "snmp_device_prof_%s_stats_snmp_tables_cached", Name: "tables_cached"},
		},
	}

	profileStatsMetricsChartTmpl = collectorapi.Chart{
		ID:       "snmp_device_prof_%s_stats_metrics",
		Title:    "SNMP profile metric counts",
		Units:    "metrics",
		Fam:      "Internal/Stats",
		Ctx:      "snmp.device_prof_stats_metrics",
		Priority: prioInternalStatsMetrics,
		Dims: collectorapi.Dims{
			{ID: "snmp_device_prof_%s_stats_metrics_scalar", Name: "scalar"},
			{ID: "snmp_device_prof_%s_stats_metrics_table", Name: "table"},
			{ID: "snmp_device_prof_%s_stats_metrics_virtual", Name: "virtual"},
			{ID: "snmp_device_prof_%s_stats_metrics_licensing", Name: "licensing"},
			{ID: "snmp_device_prof_%s_stats_metrics_bgp", Name: "bgp"},
			{ID: "snmp_device_prof_%s_stats_metrics_tables", Name: "tables"},
			{ID: "snmp_device_prof_%s_stats_metrics_rows", Name: "rows"},
		},
	}

	profileStatsTableCacheChartTmpl = collectorapi.Chart{
		ID:       "snmp_device_prof_%s_stats_table_cache",
		Title:    "SNMP profile table cache",
		Units:    "tables",
		Fam:      "Internal/Stats",
		Ctx:      "snmp.device_prof_stats_table_cache",
		Priority: prioInternalStatsTableCache,
		Dims: collectorapi.Dims{
			{ID: "snmp_device_prof_%s_stats_table_cache_hits", Name: "hits"},
			{ID: "snmp_device_prof_%s_stats_table_cache_misses", Name: "misses"},
		},
	}

	profileStatsErrorsChartTmpl = collectorapi.Chart{
		ID:       "snmp_device_prof_%s_stats_errors",
		Title:    "SNMP profile errors",
		Units:    "errors",
		Fam:      "Internal/Stats",
		Ctx:      "snmp.device_prof_stats_errors",
		Priority: prioInternalStatsErrors,
		Dims: collectorapi.Dims{
			{ID: "snmp_device_prof_%s_stats_errors_snmp", Name: "snmp"},
			{ID: "snmp_device_prof_%s_stats_errors_processing_scalar", Name: "processing_scalar"},
			{ID: "snmp_device_prof_%s_stats_errors_processing_preparation", Name: "processing_preparation"},
			{ID: "snmp_device_prof_%s_stats_errors_processing_table", Name: "processing_table"},
			{ID: "snmp_device_prof_%s_stats_errors_processing_licensing", Name: "processing_licensing"},
			{ID: "snmp_device_prof_%s_stats_errors_processing_bgp", Name: "processing_bgp"},
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
			chart.Labels = append(chart.Labels, collectorapi.Label{
				Key:   k,
				Value: v,
			})
		}
	}

	for _, chart := range *charts {
		c.installChart(chart)
	}
}

func (c *Collector) addProfileScalarMetricChart(m ddsnmp.Metric) {
	c.addProfileMetricChart(m, m.Name, chartIDFromName(m.Name), collectorapi.ChartType(m.ChartType))
}

func (c *Collector) addProfileTableMetricChart(m ddsnmp.Metric) {
	key := tableMetricKey(m)
	c.addProfileMetricChart(m, key, chartIDFromKey(key), "")
}

func (c *Collector) addProfileMetricChart(m ddsnmp.Metric, key, id string, chartType collectorapi.ChartType) {
	if m.Name == "" {
		return
	}
	chart := &collectorapi.Chart{
		ID:       id,
		Title:    m.Description,
		Type:     chartType,
		Units:    m.Unit,
		Fam:      m.Family,
		Ctx:      chartContextID(m.Name),
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
		chart.Type = collectorapi.Area
	}

	tags := c.chartBaseLabels()
	maps.Copy(tags, m.Profile.Tags)
	addMetricTagLabels(tags, m.Tags)
	for key, value := range tags {
		chart.Labels = append(chart.Labels, collectorapi.Label{
			Key:   key,
			Value: value,
		})
	}
	if len(m.MultiValue) > 0 {
		for name := range m.MultiValue {
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID:   metricIDFromKey(key, name),
				Name: name,
				Algo: dimAlgoFromDdSnmpType(m),
			})
		}
	} else {
		chart.Dims = collectorapi.Dims{{ID: metricIDFromKey(key), Name: m.Name, Algo: dimAlgoFromDdSnmpType(m)}}
	}
	c.installChart(chart)
}

// Reuse a chart object for an updated definition, including a replacement that
// supersedes an uncommitted removal. The runtime retains the Charts collection.
func (c *Collector) installChart(chart *collectorapi.Chart) {
	if err := collectorapi.CheckCharts(chart); err != nil {
		c.Warning(err)
		return
	}
	if existing := c.charts.Get(chart.ID); existing != nil {
		for _, dim := range existing.Dims {
			if !chart.HasDim(dim.ID) && (existing.IsRemoved() || dim.IsRemoved()) {
				_ = existing.MarkDimRemove(dim.ID, false)
				chart.Dims = append(chart.Dims, dim)
			}
		}
		*existing = *chart
		return
	}
	if err := c.charts.Add(chart); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) refreshAuxiliaryChartLabels() {
	for _, chart := range *c.charts {
		if chart.ID == pingRTTChart.ID || chart.ID == pingRTTStdDevChart.ID {
			chart.Labels = nil
			for key, value := range c.chartBaseLabels() {
				chart.Labels = append(chart.Labels, collectorapi.Label{
					Key:   key,
					Value: value,
				})
			}
			chart.MarkNotCreated()
		} else if strings.HasPrefix(chart.Ctx, "snmp.license.") {
			chart.Labels = []collectorapi.Label{{Key: "component", Value: "licensing"}}
			for key, value := range c.chartBaseLabels() {
				chart.Labels = append(chart.Labels, collectorapi.Label{
					Key:   key,
					Value: value,
				})
			}
			chart.MarkNotCreated()
		}
	}
}

func (c *Collector) removeProfileTableMetricChart(key string) {
	id := chartIDFromKey(key)
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

func addMetricTagLabels(labels, tags map[string]string) {
	for k, v := range tags {
		if strings.HasPrefix(k, "_") {
			continue
		}
		addMetricTagLabel(labels, k, v)
	}
	for k, v := range tags {
		if !strings.HasPrefix(k, "_") {
			continue
		}
		key := strings.TrimPrefix(k, "_")
		if key == "" {
			continue
		}
		addMetricTagLabel(labels, key, v)
	}
}

func addMetricTagLabel(labels map[string]string, key, value string) {
	if existing, ok := labels[key]; !ok || existing == "" {
		labels[key] = value
	}
}

func dimAlgoFromDdSnmpType(m ddsnmp.Metric) collectorapi.DimAlgo {
	switch m.MetricType {
	case ddprofiledefinition.ProfileMetricTypeGauge,
		ddprofiledefinition.ProfileMetricTypeMonotonicCount,
		ddprofiledefinition.ProfileMetricTypeMonotonicCountAndRate:
		return collectorapi.Absolute
	default:
		return collectorapi.Incremental
	}
}

var cleanMetricName = strings.NewReplacer(".", "_", " ", "_")
