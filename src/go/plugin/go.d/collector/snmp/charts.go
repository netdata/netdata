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
	for _, chart := range *pingCharts.Copy() {
		chart.Labels = toChartLabels(c.chartBaseLabels())
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
		chart.Labels = toChartLabels(labels)
	}

	for _, chart := range *charts {
		c.installChart(chart)
	}
}

func (c *Collector) addProfileScalarMetricChart(m ddsnmp.Metric) *collectorapi.Chart {
	labels := make(map[string]string)
	c.profileMetricLabels(labels, m)
	return c.addProfileMetricChart(m, m.Name, chartIDFromName(m.Name), collectorapi.ChartType(m.ChartType), labels)
}

func (c *Collector) addProfileTableMetricChart(m ddsnmp.Metric, key string, labels map[string]string) *collectorapi.Chart {
	return c.addProfileMetricChart(m, key, key, "", labels)
}

func (c *Collector) addProfileMetricChart(
	m ddsnmp.Metric,
	key, id string,
	chartType collectorapi.ChartType,
	labels map[string]string,
) *collectorapi.Chart {
	if m.Name == "" {
		return nil
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

	updateProfileChartLabels(chart, labels)
	if len(m.MultiValue) > 0 {
		for name := range m.MultiValue {
			var id string
			if m.IsTable {
				id = metricIDFromKey(key, name)
			} else {
				id = metricIDFromName(m.Name, name)
			}
			chart.Dims = append(chart.Dims, &collectorapi.Dim{
				ID:   id,
				Name: name,
				Algo: dimAlgoFromDdSnmpType(m),
			})
		}
	} else {
		id := metricIDFromName(m.Name)
		if m.IsTable {
			id = metricIDFromKey(key)
		}
		chart.Dims = collectorapi.Dims{{ID: id, Name: m.Name, Algo: dimAlgoFromDdSnmpType(m)}}
	}
	return c.installChart(chart)
}

// Reuse a chart object for an updated definition, including a replacement that
// supersedes an uncommitted removal. The runtime retains the Charts collection.
func (c *Collector) installChart(chart *collectorapi.Chart) *collectorapi.Chart {
	if err := collectorapi.CheckCharts(chart); err != nil {
		c.Warning(err)
		return nil
	}
	delete(c.staleCharts, chart.ID)
	if existing := c.charts.Get(chart.ID); existing != nil {
		for _, dim := range existing.Dims {
			if chart.HasDim(dim.ID) {
				continue
			}
			// Retain obsolete dimensions until the framework publishes their removal.
			_ = existing.MarkDimRemove(dim.ID, false)
			chart.Dims = append(chart.Dims, dim)
		}
		*existing = *chart
		return existing
	}
	if err := c.charts.Add(chart); err != nil {
		c.Warning(err)
		return nil
	}
	return chart
}

// chartInstalled reports whether the current device state already owns the chart.
func (c *Collector) chartInstalled(id string) bool {
	chart := c.charts.Get(id)
	_, stale := c.staleCharts[id]
	return chart != nil && !chart.IsRemoved() && !stale
}

// retireStaleCharts removes the charts a rebuild left behind that the new state
// did not report in its first successful poll.
func (c *Collector) retireStaleCharts() {
	for id := range c.staleCharts {
		if chart := c.charts.Get(id); chart != nil {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
	c.staleCharts = nil
}

// Composite inputs may be partial, including the first replacement sample.
// Keep an index of live declared dimensions so steady collection stays O(values).
func (s *metricChartState) observeDimension(id, name string, algo collectorapi.DimAlgo) {
	if s == nil || s.chart == nil {
		return
	}
	if s.dimensions == nil {
		s.dimensions = make(map[string]bool, len(s.chart.Dims))
		for _, dim := range s.chart.Dims {
			if !dim.IsRemoved() {
				s.dimensions[dim.ID] = true
			}
		}
	}
	if s.dimensions[id] {
		return
	}
	dim := s.chart.GetDim(id)
	if dim == nil {
		dim = &collectorapi.Dim{}
		s.chart.Dims = append(s.chart.Dims, dim)
	}
	*dim = collectorapi.Dim{
		ID:   id,
		Name: name,
		Algo: algo,
	}
	s.dimensions[id] = true
	s.chart.MarkNotCreated()
}

func toChartLabels(labels map[string]string) []collectorapi.Label {
	out := make([]collectorapi.Label, 0, len(labels))
	for key, value := range labels {
		out = append(out, collectorapi.Label{
			Key:   key,
			Value: value,
		})
	}
	return out
}

func updateProfileChartLabels(chart *collectorapi.Chart, labels map[string]string) {
	if len(chart.Labels) == len(labels) {
		equal := true
		for _, label := range chart.Labels {
			if value, ok := labels[label.Key]; !ok || value != label.Value {
				equal = false
				break
			}
		}
		if equal {
			return
		}
	}
	clear(chart.Labels)
	chart.Labels = chart.Labels[:0]
	for k, v := range labels {
		chart.Labels = append(chart.Labels, collectorapi.Label{Key: k, Value: v})
	}
	chart.MarkNotCreated()
}

func (c *Collector) profileMetricLabels(labels map[string]string, m ddsnmp.Metric) {
	clear(labels)
	c.addChartBaseLabels(labels)
	maps.Copy(labels, m.Profile.Tags)
	addMetricTagLabels(labels, m.Tags)
}

func (c *Collector) chartBaseLabels() map[string]string {
	labels := make(map[string]string)
	c.addChartBaseLabels(labels)
	return labels
}

func (c *Collector) addChartBaseLabels(labels map[string]string) {
	si := c.sysInfo
	labels["sysName"] = si.Name
	labels["address"] = c.Hostname

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
