// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"strings"

	"github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioDefault   = collectorapi.Priority
	prioGORuntime = prioDefault + 10
)

func (c *Collector) addGaugeChart(id, name, help string, labels labels.Labels) {
	units := getChartUnits(name)

	cType := collectorapi.Line
	if strings.HasSuffix(units, "bytes") {
		cType = collectorapi.Area
	}

	title, ctx, units, cType := c.chartMeta(name, help, units, cType)

	chart := &collectorapi.Chart{
		ID:       id,
		Title:    title,
		Units:    units,
		Fam:      getChartFamily(name),
		Ctx:      ctx,
		Type:     cType,
		Priority: getChartPriority(name),
		Dims: collectorapi.Dims{
			{ID: id, Name: name, Div: precision},
		},
	}

	for _, lbl := range labels {
		chart.Labels = append(chart.Labels,
			collectorapi.Label{
				Key:   c.labelName(lbl.Name),
				Value: apostropheReplacer.Replace(lbl.Value),
			},
		)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
		return
	}

	c.cache.addChart(id, chart)
}

func (c *Collector) addGaugeChartWithDim(id, name, help string, labels labels.Labels, dimID, dimName string) {
	units := getChartUnits(name)

	cType := collectorapi.Line
	if strings.HasSuffix(units, "bytes") {
		cType = collectorapi.Area
	}

	title, ctx, units, cType := c.chartMeta(name, help, units, cType)

	chart := &collectorapi.Chart{
		ID:       id,
		Title:    title,
		Units:    units,
		Fam:      getChartFamily(name),
		Ctx:      ctx,
		Type:     cType,
		Priority: getChartPriority(name),
		Dims: collectorapi.Dims{
			{ID: dimID, Name: dimName, Div: precision},
		},
	}

	for _, lbl := range labels {
		chart.Labels = append(chart.Labels, collectorapi.Label{
			Key:   c.labelName(lbl.Name),
			Value: apostropheReplacer.Replace(lbl.Value),
		})
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
		return
	}

	c.cache.addChart(id, chart)
}

func (c *Collector) addCounterChart(id, name, help string, labels labels.Labels) {
	units := getChartUnits(name)

	switch units {
	case "seconds", "time":
	default:
		units += "/s"
	}

	cType := collectorapi.Line
	if strings.HasSuffix(units, "bytes/s") {
		cType = collectorapi.Area
	}

	title, ctx, units, cType := c.chartMeta(name, help, units, cType)

	chart := &collectorapi.Chart{
		ID:       id,
		Title:    title,
		Units:    units,
		Fam:      getChartFamily(name),
		Ctx:      ctx,
		Type:     cType,
		Priority: getChartPriority(name),
		Dims: collectorapi.Dims{
			{ID: id, Name: name, Algo: collectorapi.Incremental, Div: precision},
		},
	}
	for _, lbl := range labels {
		chart.Labels = append(chart.Labels,
			collectorapi.Label{
				Key:   c.labelName(lbl.Name),
				Value: apostropheReplacer.Replace(lbl.Value),
			},
		)
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
		return
	}

	c.cache.addChart(id, chart)
}

func (c *Collector) addCounterChartWithDim(id, name, help string, labels labels.Labels, dimID, dimName string) {
	units := getChartUnits(name)

	switch units {
	case "seconds", "time":
	default:
		units += "/s"
	}

	cType := collectorapi.Line
	if strings.HasSuffix(units, "bytes/s") {
		cType = collectorapi.Area
	}

	title, ctx, units, cType := c.chartMeta(name, help, units, cType)

	chart := &collectorapi.Chart{
		ID:       id,
		Title:    title,
		Units:    units,
		Fam:      getChartFamily(name),
		Ctx:      ctx,
		Type:     cType,
		Priority: getChartPriority(name),
		Dims: collectorapi.Dims{
			{ID: dimID, Name: dimName, Algo: collectorapi.Incremental, Div: precision},
		},
	}

	for _, lbl := range labels {
		chart.Labels = append(chart.Labels, collectorapi.Label{
			Key:   c.labelName(lbl.Name),
			Value: apostropheReplacer.Replace(lbl.Value),
		})
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warning(err)
		return
	}

	c.cache.addChart(id, chart)
}

// addSummaryCharts creates the three summary sub-charts (quantile, _sum, _count).
// Pass withDims=false when a dimension_rule is active so ensureChartDim populates
// dims instead, avoiding phantom structural dims that would become immediately stale.
func (c *Collector) addSummaryCharts(id, name, help string, labels labels.Labels, quantiles []prometheus.Quantile, withDims bool) {
	units := getChartUnits(name)

	switch units {
	case "seconds", "time":
	default:
		units += "/s"
	}

	title, ctx, units, cType := c.chartMeta(name, help, units, collectorapi.Line)

	var sumDims, countDims collectorapi.Dims
	if withDims {
		sumDims = collectorapi.Dims{{ID: id + "_sum", Name: name + "_sum", Algo: collectorapi.Incremental, Div: precision}}
		countDims = collectorapi.Dims{{ID: id + "_count", Name: name + "_count", Algo: collectorapi.Incremental}}
	}

	charts := collectorapi.Charts{
		{
			ID:       id,
			Title:    title,
			Units:    units,
			Fam:      getChartFamily(name),
			Ctx:      ctx,
			Type:     cType,
			Priority: getChartPriority(name),
			Dims: func() (dims collectorapi.Dims) {
				if !withDims {
					return nil
				}
				for _, v := range quantiles {
					s := formatFloat(v.Quantile())
					dims = append(dims, &collectorapi.Dim{
						ID:   fmt.Sprintf("%s_quantile=%s", id, s),
						Name: fmt.Sprintf("quantile_%s", s),
						Div:  precision * precision,
					})
				}
				return dims
			}(),
		},
		{
			ID:       id + "_sum",
			Title:    title,
			Units:    units,
			Fam:      getChartFamily(name),
			Ctx:      ctx + "_sum",
			Type:     cType,
			Priority: getChartPriority(name),
			Dims:     sumDims,
		},
		{
			ID:       id + "_count",
			Title:    title,
			Units:    "events/s",
			Fam:      getChartFamily(name),
			Ctx:      ctx + "_count",
			Type:     cType,
			Priority: getChartPriority(name),
			Dims:     countDims,
		},
	}

	for _, chart := range charts {
		for _, lbl := range labels {
			chart.Labels = append(chart.Labels, collectorapi.Label{
				Key:   c.labelName(lbl.Name),
				Value: apostropheReplacer.Replace(lbl.Value),
			})
		}
		if err := c.Charts().Add(chart); err != nil {
			c.Warning(err)
			continue
		}
		c.cache.addChart(id, chart)
	}
}

// addHistogramCharts creates the three histogram sub-charts (bucket, _sum, _count).
// Pass withDims=false when a dimension_rule is active so ensureChartDim populates
// dims instead, avoiding phantom structural dims that would become immediately stale.
func (c *Collector) addHistogramCharts(id, name, help string, labels labels.Labels, buckets []prometheus.Bucket, withDims bool) {
	units := getChartUnits(name)

	switch units {
	case "seconds", "time":
	default:
		units += "/s"
	}

	title, ctx, units, cType := c.chartMeta(name, help, units, collectorapi.Line)

	var sumDims, countDims collectorapi.Dims
	if withDims {
		sumDims = collectorapi.Dims{{ID: id + "_sum", Name: name + "_sum", Algo: collectorapi.Incremental, Div: precision}}
		countDims = collectorapi.Dims{{ID: id + "_count", Name: name + "_count", Algo: collectorapi.Incremental}}
	}

	charts := collectorapi.Charts{
		{
			ID:       id,
			Title:    title,
			Units:    "observations/s",
			Fam:      getChartFamily(name),
			Ctx:      ctx,
			Type:     cType,
			Priority: getChartPriority(name),
			Dims: func() (dims collectorapi.Dims) {
				if !withDims {
					return nil
				}
				for _, v := range buckets {
					s := formatFloat(v.UpperBound())
					dims = append(dims, &collectorapi.Dim{
						ID:   fmt.Sprintf("%s_bucket=%s", id, s),
						Name: fmt.Sprintf("bucket_%s", s),
						Algo: collectorapi.Incremental,
					})
				}
				return dims
			}(),
		},
		{
			ID:       id + "_sum",
			Title:    title,
			Units:    units,
			Fam:      getChartFamily(name),
			Ctx:      ctx + "_sum",
			Type:     cType,
			Priority: getChartPriority(name),
			Dims:     sumDims,
		},
		{
			ID:       id + "_count",
			Title:    title,
			Units:    "events/s",
			Fam:      getChartFamily(name),
			Ctx:      ctx + "_count",
			Type:     cType,
			Priority: getChartPriority(name),
			Dims:     countDims,
		},
	}

	for _, chart := range charts {
		for _, lbl := range labels {
			chart.Labels = append(chart.Labels, collectorapi.Label{
				Key:   c.labelName(lbl.Name),
				Value: apostropheReplacer.Replace(lbl.Value),
			})
		}
		if err := c.Charts().Add(chart); err != nil {
			c.Warning(err)
			continue
		}
		c.cache.addChart(id, chart)
	}
}

func (c *Collector) chartMeta(name, help, units string, cType collectorapi.ChartType) (title, ctx, outUnits string, outType collectorapi.ChartType) {
	ov := c.chartOverrides(name)
	title = getChartTitle(name, help)
	if ov.title != "" {
		title = ov.title
	}
	outUnits = units
	if ov.units != "" {
		outUnits = ov.units
	}
	outType = cType
	if ov.hasType {
		outType = ov.cType
	}
	contextName := name
	if ov.context != "" {
		contextName = ov.context
	}
	ctx = getChartContext(c.application(), contextName)
	return title, ctx, outUnits, outType
}

func (c *Collector) application() string {
	if c.Application != "" {
		return c.Application
	}
	return c.Name
}

func (c *Collector) labelName(lblName string) string {
	if c.LabelPrefix == "" {
		return lblName
	}
	return c.LabelPrefix + "_" + lblName
}

func getChartTitle(name, help string) string {
	if help == "" {
		return fmt.Sprintf("Metric \"%s\"", name)
	}

	help = strings.ReplaceAll(help, "'", "")
	help = strings.TrimSuffix(help, ".")

	return help
}

func getChartContext(app, name string) string {
	if app == "" {
		return fmt.Sprintf("prometheus.%s", name)
	}
	return fmt.Sprintf("prometheus.%s.%s", app, name)
}

func getChartFamily(metric string) (fam string) {
	if strings.HasPrefix(metric, "go_") {
		return "go"
	}
	if strings.HasPrefix(metric, "process_") {
		return "process"
	}
	if parts := strings.SplitN(metric, "_", 3); len(parts) < 3 {
		fam = metric
	} else {
		fam = parts[0] + "_" + parts[1]
	}

	// remove number suffix if any
	// load1, load5, load15 => load
	i := len(fam) - 1
	for i >= 0 && fam[i] >= '0' && fam[i] <= '9' {
		i--
	}
	if i > 0 {
		return fam[:i+1]
	}
	return fam
}

func getChartUnits(metric string) string {
	// https://prometheus.io/docs/practices/naming/#metric-names
	// ...must have a single unit (i.e. do not mix seconds with milliseconds, or seconds with bytes).
	// ...should have a suffix describing the unit, in plural form.
	// Note that an accumulating count has total as a suffix, in addition to the unit if applicable

	idx := strings.LastIndexByte(metric, '_')
	if idx == -1 {
		// snmp_exporter: e.g. ifOutUcastPkts, ifOutOctets.
		if idx = strings.LastIndexFunc(metric, func(r rune) bool { return r >= 'A' && r <= 'Z' }); idx != -1 {
			v := strings.ToLower(metric[idx:])
			switch v {
			case "pkts":
				return "packets"
			case "octets":
				return "bytes"
			case "mtu":
				return "octets"
			case "speed":
				return "bits"
			}
			return v
		}
		return "events"
	}
	switch suffix := metric[idx:]; suffix {
	case "_total", "_sum", "_count", "_ratio":
		return getChartUnits(metric[:idx])
	}
	switch units := metric[idx+1:]; units {
	case "hertz":
		return "Hz"
	default:
		return units
	}
}

func getChartPriority(name string) int {
	if strings.HasPrefix(name, "go_") || strings.HasPrefix(name, "process_") {
		return prioGORuntime
	}
	return prioDefault
}
