// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioMetricChart = module.Priority + iota
	prioQueryTimingChart
)

func (c *Collector) createMetricBlockChart(chartID string, m ConfigMetricBlock, ch ConfigChartConfig, row map[string]string) {
	if c.seenCharts[chartID] {
		return
	}
	c.seenCharts[chartID] = true

	chart := &module.Chart{
		ID:       chartID,
		Title:    ch.Title,
		Units:    ch.Units,
		Type:     module.ChartType(ch.Type),
		Ctx:      fmt.Sprintf("sql.%s_%s", c.Driver, ch.Context),
		Fam:      ch.Family,
		Priority: prioMetricChart,
		Labels: []module.Label{
			{Key: "driver", Value: c.Driver},
		},
	}

	for k, v := range c.StaticLabels {
		chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
	}

	for _, lf := range m.LabelsFromRow {
		if v, ok := row[lf.Source]; ok {
			chart.Labels = append(chart.Labels, module.Label{Key: lf.Name, Value: v})
		}
	}

	for _, d := range ch.Dims {
		chart.Dims = append(chart.Dims, &module.Dim{
			ID:   buildDimID(chartID, d.Name),
			Name: d.Name,
			Algo: module.DimAlgo(ch.Algorithm),
		})
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warningf("failed to add chart %q: %v", chartID, err)
	}
}

func (c *Collector) createQueryTimingChart(chartID, label string) {
	if c.seenCharts[chartID] {
		return
	}
	c.seenCharts[chartID] = true

	chart := &module.Chart{
		ID:       chartID,
		Title:    "SQL query execution time",
		Units:    "ms",
		Ctx:      fmt.Sprintf("sql.%s_query_time", c.Driver),
		Fam:      "Query/Timings",
		Priority: prioQueryTimingChart,
		Labels: []module.Label{
			{Key: "driver", Value: c.Driver},
			{Key: "query_id", Value: label},
		},
		Dims: []*module.Dim{
			{
				ID:   buildDimID(chartID, "duration"),
				Name: "duration",
			},
		},
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warningf("failed to add query timing chart %q: %v", chartID, err)
		return
	}
}
