// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaterializeChartsReportsEffectiveDimensionAlgorithms(t *testing.T) {
	plan := chartengine.Plan{Actions: []chartengine.EngineAction{
		chartengine.CreateChartAction{
			ChartTemplateID: "mixed",
			ChartID:         "mixed",
			Meta: chartengine.ChartMeta{
				Title:   "Mixed",
				Context: "mixed",
				Units:   "items",
			},
		},
		chartengine.CreateDimensionAction{
			ChartID:   "mixed",
			Name:      "gauge",
			Algorithm: chartengine.AlgorithmAbsolute,
		},
		chartengine.CreateDimensionAction{
			ChartID:   "mixed",
			Name:      "counter_a",
			Algorithm: chartengine.AlgorithmIncremental,
		},
		chartengine.CreateDimensionAction{
			ChartID:   "mixed",
			Name:      "counter_b",
			Algorithm: chartengine.AlgorithmIncremental,
		},
	}}

	charts := materializeCharts(plan, nil)
	require.Len(t, charts, 1)
	assert.Equal(t, []string{"absolute", "incremental"}, charts[0].Algorithms)
	assert.Len(t, charts[0].DimensionFingerprints, 3)
}
