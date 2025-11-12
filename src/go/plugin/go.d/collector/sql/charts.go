// SPDX-License-Identifier: GPL-3.0-or-later

package sql

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioChart = module.Priority + iota
)

func (c *Collector) ensureChart(chartID string, m ConfigMetricBlock, ch ConfigChartConfig, row map[string]string) {
	if c.seenCharts[chartID] {
		return
	}
	c.seenCharts[chartID] = true

	chart := &module.Chart{
		ID:       chartID,
		Title:    ch.Title,
		Units:    ch.Units,
		Type:     module.ChartType(ch.Type),
		Ctx:      ch.Context,
		Fam:      ch.Family,
		Priority: prioChart, // use your existing default
	}

	// --- add static labels ---
	for k, v := range c.StaticLabels {
		chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
	}
	chart.Labels = append(chart.Labels, module.Label{Key: "driver", Value: c.Driver})

	// --- add labels_from_row (if any) ---
	for _, lf := range m.LabelsFromRow {
		v, ok := row[lf.Source]
		if !ok {
			continue
		}
		chart.Labels = append(chart.Labels, module.Label{Key: lf.Name, Value: v})
	}

	// --- add chart dimensions ---
	for _, d := range ch.Dims {
		dimID := strings.ToLower(chartID + "." + d.Name)
		chart.Dims = append(chart.Dims, &module.Dim{
			ID:   dimID,
			Name: d.Name,
			Algo: module.DimAlgo(ch.Algorithm),
		})
	}

	if err := c.Charts().Add(chart); err != nil {
		c.Warningf("failed to add chart %q: %v", chartID, err)
	}
}
