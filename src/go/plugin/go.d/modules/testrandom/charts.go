// SPDX-License-Identifier: GPL-3.0-or-later

package testrandom

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var chartTemplate = module.Chart{
	ID:    "random_%d",
	Title: "A Random Number",
	Units: "random",
	Fam:   "random",
	Ctx:   "testrandom.random",
}

var hiddenChartTemplate = module.Chart{
	ID:    "hidden_random_%d",
	Title: "A Random Number",
	Units: "random",
	Fam:   "random",
	Ctx:   "testrandom.random",
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
			Key:   fmt.Sprintf("random_name_%d", i),
			Value: fmt.Sprintf("random_value_%d_%d", num, i),
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
			Key:   fmt.Sprintf("random_name_%d", i),
			Value: fmt.Sprintf("random_value_%d_%d", num, i),
		})
	}
	return chart
}
