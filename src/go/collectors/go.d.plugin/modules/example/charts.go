// SPDX-License-Identifier: GPL-3.0-or-later

package example

import (
	"fmt"
	"github.com/netdata/go.d.plugin/agent/module"
)

var chartTemplate = module.Chart{
	ID:    "random_%d",
	Title: "A Random Number",
	Units: "random",
	Fam:   "random",
	Ctx:   "example.random",
}

var hiddenChartTemplate = module.Chart{
	ID:    "hidden_random_%d",
	Title: "A Random Number",
	Units: "random",
	Fam:   "random",
	Ctx:   "example.random",
	Opts: module.Opts{
		Hidden: true,
	},
}

func newChart(num, ctx, labels int, typ module.ChartType) *module.Chart {
	chart := chartTemplate.Copy()
	chart.ID = fmt.Sprintf(chart.ID, num)
	chart.Type = typ
	if ctx > 0 {
		chart.Ctx += fmt.Sprintf("_%d", ctx)
	}
	for i := 0; i < labels; i++ {
		chart.Labels = append(chart.Labels, module.Label{
			Key:   fmt.Sprintf("example_name_%d", i),
			Value: fmt.Sprintf("example_value_%d_%d", num, i),
		})
	}
	return chart
}

func newHiddenChart(num, ctx, labels int, typ module.ChartType) *module.Chart {
	chart := hiddenChartTemplate.Copy()
	chart.ID = fmt.Sprintf(chart.ID, num)
	chart.Type = typ
	if ctx > 0 {
		chart.Ctx += fmt.Sprintf("_%d", ctx)
	}
	for i := 0; i < labels; i++ {
		chart.Labels = append(chart.Labels, module.Label{
			Key:   fmt.Sprintf("example_name_%d", i),
			Value: fmt.Sprintf("example_value_%d_%d", num, i),
		})
	}
	return chart
}
