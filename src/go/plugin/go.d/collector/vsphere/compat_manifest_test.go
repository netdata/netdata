// SPDX-License-Identifier: GPL-3.0-or-later
package vsphere

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestCollector_V2CompatibilitySurface(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}

	require.NotEmpty(t, collectScalarSeriesForTest(t, collr))

	plan := buildV2PlanForTest(t, collr)
	createdCharts, createdDims := v2CreatedChartsAndDims(plan)
	require.NotEmpty(t, createdCharts)

	for chartID, chart := range createdCharts {
		require.NotEmpty(t, chart.Labels["id"], "chart %s must have the V2 instance id label", chartID)
		require.NotEmpty(t, createdDims[chartID], "chart %s must have dimensions", chartID)
	}
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
}

func buildV2PlanForTest(t *testing.T, collr *Collector) chartengine.Plan {
	t.Helper()

	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(collr.ChartTemplateYAML()), 1))

	reader := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	attempt, err := engine.PreparePlan(reader)
	require.NoError(t, err)
	defer attempt.Abort()

	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())
	return plan
}

func v2CreatedChartsAndDims(plan chartengine.Plan) (map[string]chartengine.CreateChartAction, map[string]map[string]chartengine.CreateDimensionAction) {
	charts := make(map[string]chartengine.CreateChartAction)
	dims := make(map[string]map[string]chartengine.CreateDimensionAction)
	for _, action := range plan.Actions {
		switch v := action.(type) {
		case chartengine.CreateChartAction:
			charts[v.ChartID] = v
		case chartengine.CreateDimensionAction:
			if _, ok := dims[v.ChartID]; !ok {
				dims[v.ChartID] = make(map[string]chartengine.CreateDimensionAction)
			}
			dims[v.ChartID][v.Name] = v
		}
	}
	return charts, dims
}
