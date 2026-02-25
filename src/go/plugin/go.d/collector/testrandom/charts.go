// SPDX-License-Identifier: GPL-3.0-or-later

package testrandom

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

var chartTemplate = collectorapi.Chart{
	ID:    "random_%d",
	Title: "A Random Number",
	Units: "random",
	Fam:   "random",
	Ctx:   "testrandom.random",
}

var hiddenChartTemplate = collectorapi.Chart{
	ID:    "hidden_random_%d",
	Title: "A Random Number",
	Units: "random",
	Fam:   "random",
	Ctx:   "testrandom.random",
	Opts: collectorapi.Opts{
		Hidden: true,
	},
}

func newChart(num, ctx, labels int, typ collectorapi.ChartType) *collectorapi.Chart {
	chart := chartTemplate.Copy()
	chart.ID = fmt.Sprintf(chart.ID, num)
	chart.Type = typ
	if ctx > 0 {
		chart.Ctx += fmt.Sprintf("_%d", ctx)
	}
	for i := 0; i < labels; i++ {
		chart.Labels = append(chart.Labels, collectorapi.Label{
			Key:   fmt.Sprintf("random_name_%d", i),
			Value: fmt.Sprintf("random_value_%d_%d", num, i),
		})
	}
	return chart
}

func newHiddenChart(num, ctx, labels int, typ collectorapi.ChartType) *collectorapi.Chart {
	chart := hiddenChartTemplate.Copy()
	chart.ID = fmt.Sprintf(chart.ID, num)
	chart.Type = typ
	if ctx > 0 {
		chart.Ctx += fmt.Sprintf("_%d", ctx)
	}
	for i := 0; i < labels; i++ {
		chart.Labels = append(chart.Labels, collectorapi.Label{
			Key:   fmt.Sprintf("random_name_%d", i),
			Value: fmt.Sprintf("random_value_%d_%d", num, i),
		})
	}
	return chart
}
