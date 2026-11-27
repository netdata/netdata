// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"bytes"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmittedChartsRecord(t *testing.T) {
	recordedMeta := chartengine.ChartMeta{
		Title:   "Load (recorded)",
		Context: "app.load",
		Units:   "load",
	}
	createMeta := chartengine.ChartMeta{
		Title:   "Load (created)",
		Context: "app.load",
		Units:   "load",
	}
	firstDimMeta := chartengine.ChartMeta{
		Title:   "Load (first dimension)",
		Context: "app.load",
		Units:   "load",
	}
	laterDimMeta := chartengine.ChartMeta{
		Title:   "Load (later dimension)",
		Context: "app.load",
		Units:   "load",
	}

	tests := map[string]struct {
		charts  EmittedCharts
		actions []EngineAction
		want    EmittedCharts
	}{
		"empty plan leaves a nil set nil": {
			charts:  nil,
			actions: nil,
			want:    nil,
		},
		"created chart is recorded in a nil set": {
			charts: nil,
			actions: []EngineAction{
				CreateChartAction{
					ChartID: "load",
					Meta:    createMeta,
				},
			},
			want: EmittedCharts{
				"load": createMeta,
			},
		},
		"created chart replaces recorded metadata": {
			charts: EmittedCharts{
				"load": recordedMeta,
			},
			actions: []EngineAction{
				CreateChartAction{
					ChartID: "load",
					Meta:    createMeta,
				},
			},
			want: EmittedCharts{
				"load": createMeta,
			},
		},
		"dimension-only chart takes its first dimension's metadata": {
			charts: nil,
			actions: []EngineAction{
				CreateDimensionAction{
					ChartID:   "load",
					ChartMeta: firstDimMeta,
					Name:      "a",
				},
				CreateDimensionAction{
					ChartID:   "load",
					ChartMeta: laterDimMeta,
					Name:      "b",
				},
			},
			want: EmittedCharts{
				"load": firstDimMeta,
			},
		},
		"dimension of a recorded chart keeps its metadata": {
			charts: EmittedCharts{
				"load": recordedMeta,
			},
			actions: []EngineAction{
				CreateDimensionAction{
					ChartID:   "load",
					ChartMeta: firstDimMeta,
					Name:      "a",
				},
			},
			want: EmittedCharts{
				"load": recordedMeta,
			},
		},
		"dimension of a created chart takes the create metadata in any action order": {
			charts: nil,
			actions: []EngineAction{
				CreateDimensionAction{
					ChartID:   "load",
					ChartMeta: firstDimMeta,
					Name:      "a",
				},
				CreateChartAction{
					ChartID: "load",
					Meta:    createMeta,
				},
			},
			want: EmittedCharts{
				"load": createMeta,
			},
		},
		"removed chart leaves the set": {
			charts: EmittedCharts{
				"load":  recordedMeta,
				"other": recordedMeta,
			},
			actions: []EngineAction{
				RemoveChartAction{
					ChartID: "load",
					Meta:    recordedMeta,
				},
			},
			want: EmittedCharts{
				"other": recordedMeta,
			},
		},
		"removal applies after a create of the same chart": {
			charts: nil,
			actions: []EngineAction{
				RemoveChartAction{
					ChartID: "load",
					Meta:    recordedMeta,
				},
				CreateChartAction{
					ChartID: "load",
					Meta:    createMeta,
				},
			},
			want: EmittedCharts{},
		},
		"removal applies after a dimension-only definition of the same chart": {
			charts: EmittedCharts{
				"load": recordedMeta,
			},
			actions: []EngineAction{
				RemoveChartAction{
					ChartID: "load",
					Meta:    recordedMeta,
				},
				CreateDimensionAction{
					ChartID:   "load",
					ChartMeta: firstDimMeta,
					Name:      "a",
				},
			},
			want: EmittedCharts{},
		},
		"value, label and dimension removal actions change nothing": {
			charts: EmittedCharts{
				"load": recordedMeta,
			},
			actions: []EngineAction{
				UpdateChartLabelsAction{
					ChartID: "other",
					Meta:    createMeta,
				},
				UpdateChartAction{
					ChartID: "other",
				},
				RemoveDimensionAction{
					ChartID:   "load",
					ChartMeta: createMeta,
					Name:      "a",
				},
			},
			want: EmittedCharts{
				"load": recordedMeta,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			charts := tc.charts
			charts.Record(Plan{
				Actions: tc.actions,
			})
			assert.Equal(t, tc.want, charts)
		})
	}
}

func TestEmittedChartsRecordMatchesApplyPlanRemovalOrder(t *testing.T) {
	meta := chartengine.ChartMeta{
		Title:   "Load",
		Context: "app.load",
		Units:   "load",
	}
	plan := Plan{
		Actions: []EngineAction{
			RemoveChartAction{
				ChartID: "load",
				Meta:    meta,
			},
			CreateChartAction{
				ChartID: "load",
				Meta:    meta,
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, ApplyPlan(netdataapi.New(&buf), plan, EmitEnv{
		TypeID: "app",
	}))
	var chartLines []string
	for line := range strings.Lines(buf.String()) {
		if strings.HasPrefix(line, "CHART 'app.load'") {
			chartLines = append(chartLines, line)
		}
	}
	require.Len(t, chartLines, 2)
	assert.NotContains(t, chartLines[0], "'obsolete'")
	assert.Contains(t, chartLines[1], "'obsolete'", "ApplyPlan must emit the removal after the definition")

	var charts EmittedCharts
	charts.Record(plan)
	assert.NotContains(t, charts, "load")
}

func TestEmittedChartsObsoletePlan(t *testing.T) {
	first := chartengine.ChartMeta{
		Title:   "First",
		Context: "app.first",
	}
	second := chartengine.ChartMeta{
		Title:   "Second",
		Context: "app.second",
	}

	tests := map[string]struct {
		charts EmittedCharts
		want   Plan
	}{
		"nil set": {
			charts: nil,
			want:   Plan{},
		},
		"charts are obsoleted in chart ID order": {
			charts: EmittedCharts{
				"b": second,
				"a": first,
			},
			want: Plan{
				Actions: []EngineAction{
					RemoveChartAction{
						ChartID: "a",
						Meta:    first,
					},
					RemoveChartAction{
						ChartID: "b",
						Meta:    second,
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.charts.ObsoletePlan())
		})
	}
}
