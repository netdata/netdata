// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

func TestInspectPlanUsesEmissionNormalization(t *testing.T) {
	metaA := chartengine.ChartMeta{Context: "service.'requests\n"}
	metaB := chartengine.ChartMeta{Context: "service.requests "}
	plan := Plan{Actions: []EngineAction{
		CreateChartAction{ChartTemplateID: "a", ChartID: "chart'\n", Meta: metaA},
		CreateDimensionAction{ChartID: "chart'\n", ChartMeta: metaA, Name: "value'\n"},
		CreateChartAction{ChartTemplateID: "b", ChartID: "chart", Meta: metaB},
		CreateDimensionAction{ChartID: "chart", ChartMeta: metaB, Name: "value"},
	}}

	inspection, err := InspectPlan(plan, EmitEnv{TypeID: "job.full_name", UpdateEvery: 1})
	require.NoError(t, err)
	require.Len(t, inspection.Charts, 2)
	require.Len(t, inspection.Dimensions, 2)

	assert.Equal(t, "job.full_name", inspection.Charts[0].WireTypeID)
	assert.Equal(t, "job.full_name", inspection.Charts[1].WireTypeID)
	assert.Equal(t, "chart", inspection.Charts[0].WireChartID)
	assert.Equal(t, "chart", inspection.Charts[1].WireChartID)
	assert.Equal(t, "service.requests ", inspection.Charts[0].WireContext)
	assert.Equal(t, "service.requests ", inspection.Charts[1].WireContext)
	assert.Equal(t, "value", inspection.Dimensions[0].WireName)
	assert.Equal(t, "value", inspection.Dimensions[1].WireName)
	assert.NotEqual(t, inspection.Charts[0].SourceChartID, inspection.Charts[1].SourceChartID)
}

func TestInspectPlanClassifiesTypeIDBudgetFailure(t *testing.T) {
	plan := Plan{Actions: []EngineAction{
		CreateChartAction{ChartID: "chart", Meta: chartengine.ChartMeta{}},
	}}
	_, err := InspectPlan(plan, EmitEnv{TypeID: strings.Repeat("x", maxTypeIDLen)})
	require.ErrorIs(t, err, ErrTypeIDBudgetExceeded)
}

func TestInspectPlanVisitsEveryDefinitionPhase(t *testing.T) {
	meta := chartengine.ChartMeta{Context: "service.requests"}
	plan := Plan{Actions: []EngineAction{
		CreateChartAction{ChartID: "created", Meta: meta},
		CreateDimensionAction{ChartID: "created", ChartMeta: meta, Name: "created_dim"},
		UpdateChartLabelsAction{ChartID: "labels", Meta: meta},
		RemoveDimensionAction{ChartID: "removed_dim_chart", ChartMeta: meta, Name: "removed_dim"},
		RemoveChartAction{ChartID: "removed_chart", Meta: meta},
	}}

	inspection, err := InspectPlan(plan, EmitEnv{TypeID: "job.full_name", UpdateEvery: 1})
	require.NoError(t, err)
	assert.Equal(t, []ChartInspection{
		{SourceChartID: "created", SourceContext: meta.Context, WireTypeID: "job.full_name", WireChartID: "created", WireContext: meta.Context},
		{SourceChartID: "labels", SourceContext: meta.Context, WireTypeID: "job.full_name", WireChartID: "labels", WireContext: meta.Context},
		{SourceChartID: "removed_dim_chart", SourceContext: meta.Context, WireTypeID: "job.full_name", WireChartID: "removed_dim_chart", WireContext: meta.Context},
		{SourceChartID: "removed_chart", SourceContext: meta.Context, WireTypeID: "job.full_name", WireChartID: "removed_chart", WireContext: meta.Context, Obsolete: true},
	}, inspection.Charts)
	assert.Equal(t, []DimensionInspection{
		{SourceChartID: "created", SourceName: "created_dim", WireTypeID: "job.full_name", WireChartID: "created", WireName: "created_dim"},
		{SourceChartID: "removed_dim_chart", SourceName: "removed_dim", WireTypeID: "job.full_name", WireChartID: "removed_dim_chart", WireName: "removed_dim", Obsolete: true},
	}, inspection.Dimensions)
}
