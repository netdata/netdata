// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
)

func TestMetricsHasAllChartsDims(t *testing.T, charts *collectorapi.Charts, mx map[string]int64) {
	TestMetricsHasAllChartsDimsSkip(t, charts, mx, nil)
}

func TestMetricsHasAllChartsDimsSkip(t *testing.T, charts *collectorapi.Charts, mx map[string]int64, skip func(chart *collectorapi.Chart, dim *collectorapi.Dim) bool) {
	for _, chart := range *charts {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			if skip != nil && skip(chart, dim) {
				continue
			}
			_, ok := mx[dim.ID]
			assert.Truef(t, ok, "missing data for dimension '%s' in chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := mx[v.ID]
			assert.Truef(t, ok, "missing data for variable '%s' in chart '%s'", v.ID, chart.ID)
		}
	}
}
