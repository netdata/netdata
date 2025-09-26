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

// deprecated custom oid charts

func newUserInputCharts(configs []ChartConfig) (*module.Charts, error) {
	charts := &module.Charts{}
	for _, cfg := range configs {
		if len(cfg.IndexRange) == 2 {
			cs, err := newUserInputChartsFromIndexRange(cfg)
			if err != nil {
				return nil, err
			}
			if err := charts.Add(*cs...); err != nil {
				return nil, err
			}
		} else {
			chart, err := newUserInputChart(cfg)
			if err != nil {
				return nil, err
			}
			if err = charts.Add(chart); err != nil {
				return nil, err
			}
		}
	}
	return charts, nil
}

func newUserInputChartsFromIndexRange(cfg ChartConfig) (*module.Charts, error) {
	var addPrio int
	charts := &module.Charts{}
	for i := cfg.IndexRange[0]; i <= cfg.IndexRange[1]; i++ {
		chart, err := newUserInputChartWithOIDIndex(i, cfg)
		if err != nil {
			return nil, err
		}
		chart.Priority += addPrio
		addPrio += 1
		if err = charts.Add(chart); err != nil {
			return nil, err
		}
	}
	return charts, nil
}

func newUserInputChartWithOIDIndex(oidIndex int, cfg ChartConfig) (*module.Chart, error) {
	chart, err := newUserInputChart(cfg)
	if err != nil {
		return nil, err
	}

	chart.ID = fmt.Sprintf("%s_%d", chart.ID, oidIndex)
	chart.Title = fmt.Sprintf("%s %d", chart.Title, oidIndex)
	for _, dim := range chart.Dims {
		dim.ID = fmt.Sprintf("%s.%d", dim.ID, oidIndex)
	}

	return chart, nil
}

func newUserInputChart(cfg ChartConfig) (*module.Chart, error) {
	chart := &module.Chart{
		ID:       cfg.ID,
		Title:    cfg.Title,
		Units:    cfg.Units,
		Fam:      cfg.Family,
		Ctx:      fmt.Sprintf("snmp.%s", cfg.ID),
		Type:     module.ChartType(cfg.Type),
		Priority: cfg.Priority,
	}

	if chart.Title == "" {
		chart.Title = "Untitled chart"
	}
	if chart.Units == "" {
		chart.Units = "num"
	}
	if chart.Priority < module.Priority {
		chart.Priority += module.Priority
	}

	seen := make(map[string]struct{})
	var a string
	for _, cfg := range cfg.Dimensions {
		if cfg.Algorithm != "" {
			seen[cfg.Algorithm] = struct{}{}
			a = cfg.Algorithm
		}
		dim := &module.Dim{
			ID:   strings.TrimPrefix(cfg.OID, "."),
			Name: cfg.Name,
			Algo: module.DimAlgo(cfg.Algorithm),
			Mul:  cfg.Multiplier,
			Div:  cfg.Divisor,
		}
		if err := chart.AddDim(dim); err != nil {
			return nil, err
		}
	}
	if len(seen) == 1 && a != "" && len(chart.Dims) > 1 {
		for _, d := range chart.Dims {
			if d.Algo == "" {
				d.Algo = module.DimAlgo(a)
			}
		}
	}

	return chart, nil
}
