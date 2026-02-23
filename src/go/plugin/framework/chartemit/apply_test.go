// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"bytes"
	"strings"
	"testing"

	chartengine2 "github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

func TestApplyPlanEmitsNetdataWire(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine2.ChartMeta{
		Title:     "NIC traffic",
		Family:    "Net",
		Context:   "nic_traffic",
		Units:     "bytes/s",
		Algorithm: chartengine2.AlgorithmIncremental,
		Type:      chartengine2.ChartTypeLine,
		Priority:  1,
	}

	plan := Plan{
		Actions: []EngineAction{
			chartengine2.CreateChartAction{
				ChartTemplateID: "g0c0",
				ChartID:         "win_nic_traffic_eth0",
				Meta:            meta,
			},
			chartengine2.CreateDimensionAction{
				ChartID:    "win_nic_traffic_eth0",
				ChartMeta:  meta,
				Name:       "received",
				Hidden:     false,
				Algorithm:  chartengine2.AlgorithmIncremental,
				Multiplier: 1,
				Divisor:    1,
			},
			chartengine2.UpdateChartAction{
				ChartID: "win_nic_traffic_eth0",
				Values: []chartengine2.UpdateDimensionValue{
					{
						Name:    "received",
						IsFloat: true,
						Float64: 123.5,
					},
				},
			},
			chartengine2.RemoveDimensionAction{
				ChartID:    "win_nic_traffic_eth0",
				ChartMeta:  meta,
				Name:       "received",
				Hidden:     false,
				Algorithm:  chartengine2.AlgorithmIncremental,
				Multiplier: 1,
				Divisor:    1,
			},
			chartengine2.RemoveChartAction{
				ChartID: "win_nic_traffic_eth0",
				Meta:    meta,
			},
		},
	}

	err := ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 5,
		Plugin:      "go.d.plugin",
		Module:      "windows",
		JobName:     "job01",
		JobLabels: map[string]string{
			"instance": "localhost",
		},
		MSSinceLast: 100,
	})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "CHART 'collector.job.win_nic_traffic_eth0'")
	assert.Contains(t, out, "CLABEL 'instance' 'localhost' '2'")
	assert.Contains(t, out, "CLABEL '_collect_job' 'job01' '1'")
	assert.Contains(t, out, "CLABEL_COMMIT")
	assert.Contains(t, out, "DIMENSION 'received' 'received' 'incremental' '1' '1' ''")
	assert.Contains(t, out, "BEGIN 'collector.job.win_nic_traffic_eth0' 100")
	assert.Contains(t, out, "SET 'received' = 123")
	assert.Contains(t, out, "DIMENSION 'received' 'received' 'incremental' '1' '1' 'obsolete'")
	assert.Contains(t, out, "obsolete")

	createPos := strings.Index(out, "CHART 'collector.job.win_nic_traffic_eth0'")
	beginPos := strings.Index(out, "BEGIN 'collector.job.win_nic_traffic_eth0' 100")
	require.NotEqual(t, -1, createPos)
	require.NotEqual(t, -1, beginPos)
	assert.Less(t, createPos, beginPos)
}

func TestApplyPlanAutogenChartCreateUpdateRemove(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine2.ChartMeta{
		Title:     `Metric "svc.errors_total"`,
		Family:    "svc_errors",
		Context:   "svc.errors_total",
		Units:     "events/s",
		Algorithm: chartengine2.AlgorithmIncremental,
		Type:      chartengine2.ChartTypeLine,
	}

	plan := Plan{
		Actions: []EngineAction{
			chartengine2.CreateChartAction{
				ChartTemplateID: "__autogen__:svc.errors_total-method=GET",
				ChartID:         "svc.errors_total-method=GET",
				Meta:            meta,
				Labels: map[string]string{
					"method": "GET",
				},
			},
			chartengine2.CreateDimensionAction{
				ChartID:    "svc.errors_total-method=GET",
				ChartMeta:  meta,
				Name:       "svc.errors_total",
				Algorithm:  chartengine2.AlgorithmIncremental,
				Multiplier: 1,
				Divisor:    1,
			},
			chartengine2.UpdateChartAction{
				ChartID: "svc.errors_total-method=GET",
				Values: []chartengine2.UpdateDimensionValue{
					{
						Name:    "svc.errors_total",
						IsFloat: true,
						Float64: 10,
					},
				},
			},
			chartengine2.RemoveChartAction{
				ChartID: "svc.errors_total-method=GET",
				Meta:    meta,
			},
		},
	}

	err := ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "prometheus",
		JobName:     "job01",
	})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "CHART 'collector.job.svc.errors_total-method=GET'")
	assert.Contains(t, out, "CLABEL 'method' 'GET' '1'")
	assert.Contains(t, out, "DIMENSION 'svc.errors_total' 'svc.errors_total' 'incremental' '1' '1' ''")
	assert.Contains(t, out, "BEGIN 'collector.job.svc.errors_total-method=GET'")
	assert.Contains(t, out, "SET 'svc.errors_total' = 10")
	assert.Contains(t, out, "obsolete")
}

func TestApplyPlanDimensionOnlyCreateEmitsLabelsAndCommit(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine2.ChartMeta{
		Title:     "Dimension-only chart",
		Family:    "Runtime",
		Context:   "runtime.dimension_only",
		Units:     "1",
		Algorithm: chartengine2.AlgorithmAbsolute,
		Type:      chartengine2.ChartTypeLine,
	}

	plan := Plan{
		Actions: []EngineAction{
			chartengine2.CreateDimensionAction{
				ChartID:    "dimension_only_chart",
				ChartMeta:  meta,
				Name:       "value",
				Algorithm:  chartengine2.AlgorithmAbsolute,
				Multiplier: 1,
				Divisor:    1,
			},
		},
	}

	err := ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "runtime",
		JobName:     "job01",
		JobLabels: map[string]string{
			"instance": "localhost",
		},
	})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "CHART 'collector.job.dimension_only_chart'")
	assert.Contains(t, out, "CLABEL 'instance' 'localhost' '2'")
	assert.Contains(t, out, "CLABEL '_collect_job' 'job01' '1'")
	assert.Contains(t, out, "CLABEL_COMMIT")
	assert.Contains(t, out, "DIMENSION 'value' 'value' 'absolute' '1' '1' ''")
}

func TestApplyPlanSanitizesWireValues(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine2.ChartMeta{
		Title:     "Title'\n",
		Family:    "Family'\n",
		Context:   "Context'\n",
		Units:     "units'\n",
		Algorithm: chartengine2.AlgorithmAbsolute,
		Type:      chartengine2.ChartTypeLine,
	}

	plan := Plan{
		Actions: []EngineAction{
			chartengine2.CreateChartAction{
				ChartTemplateID: "g0.c0",
				ChartID:         "chart'id\n",
				Meta:            meta,
				Labels: map[string]string{
					"la'bel\n": "va'lue\n",
				},
			},
			chartengine2.CreateDimensionAction{
				ChartID:    "chart'id\n",
				ChartMeta:  meta,
				Name:       "dim'name\n",
				Algorithm:  chartengine2.AlgorithmAbsolute,
				Multiplier: 1,
				Divisor:    1,
			},
			chartengine2.UpdateChartAction{
				ChartID: "chart'id\n",
				Values: []chartengine2.UpdateDimensionValue{
					{
						Name:    "dim'name\n",
						IsFloat: true,
						Float64: 5,
					},
				},
			},
		},
	}

	err := ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.'job\n",
		UpdateEvery: 1,
		Plugin:      "go.d'plugin\n",
		Module:      "mod'ule\n",
		JobName:     "job'01\n",
		JobLabels: map[string]string{
			"inst'ance\n": "local'host\n",
		},
		MSSinceLast: 1,
	})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "CHART 'collector.job.chartid'")
	assert.Contains(t, out, "DIMENSION 'dimname' 'dimname' 'absolute' '1' '1' ''")
	assert.Contains(t, out, "BEGIN 'collector.job.chartid' 1")
	assert.Contains(t, out, "SET 'dimname' = 5")
	assert.Contains(t, out, "CLABEL 'instance' 'localhost ' '2'")
	assert.Contains(t, out, "CLABEL 'label' 'value ' '1'")
	assert.Contains(t, out, "CLABEL '_collect_job' 'job01 ' '1'")
}

func TestApplyPlanRejectsEmptyTypeID(t *testing.T) {
	tests := map[string]struct {
		typeID  string
		wantErr string
	}{
		"rejects empty type id": {
			typeID:  "",
			wantErr: "type_id is required",
		},
		"rejects whitespace type id": {
			typeID:  "   \t",
			wantErr: "type_id is required",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			api := netdataapi.New(&buf)

			err := ApplyPlan(api, Plan{}, EmitEnv{
				TypeID: tc.typeID,
			})
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestNormalizeActionsOrderingDeterminism(t *testing.T) {
	meta := chartengine2.ChartMeta{
		Title:     "Requests",
		Family:    "Service",
		Context:   "requests",
		Units:     "requests/s",
		Algorithm: chartengine2.AlgorithmIncremental,
		Type:      chartengine2.ChartTypeLine,
	}

	tests := map[string]struct {
		actions []EngineAction
	}{
		"preserves update/remove order and groups create actions by chart id": {
			actions: []EngineAction{
				chartengine2.UpdateChartAction{ChartID: "chart_b"},
				chartengine2.UpdateChartAction{ChartID: "chart_a"},
				chartengine2.RemoveDimensionAction{ChartID: "chart_b", Name: "dim_b", ChartMeta: meta},
				chartengine2.RemoveDimensionAction{ChartID: "chart_a", Name: "dim_a", ChartMeta: meta},
				chartengine2.RemoveChartAction{ChartID: "chart_b", Meta: meta},
				chartengine2.RemoveChartAction{ChartID: "chart_a", Meta: meta},
				chartengine2.CreateDimensionAction{ChartID: "chart_b", Name: "dim_b", ChartMeta: meta},
				chartengine2.CreateChartAction{ChartID: "chart_b", Meta: meta},
				chartengine2.CreateDimensionAction{ChartID: "chart_a", Name: "dim_a", ChartMeta: meta},
				chartengine2.CreateChartAction{ChartID: "chart_a", Meta: meta},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			normalized := normalizeActions(tc.actions)

			require.Len(t, normalized.updateCharts, 2)
			assert.Equal(t, "chart_b", normalized.updateCharts[0].ChartID)
			assert.Equal(t, "chart_a", normalized.updateCharts[1].ChartID)

			require.Len(t, normalized.removeDimensions, 2)
			assert.Equal(t, "chart_b", normalized.removeDimensions[0].ChartID)
			assert.Equal(t, "chart_a", normalized.removeDimensions[1].ChartID)

			require.Len(t, normalized.removeCharts, 2)
			assert.Equal(t, "chart_b", normalized.removeCharts[0].ChartID)
			assert.Equal(t, "chart_a", normalized.removeCharts[1].ChartID)

			require.Len(t, normalized.createCharts, 2)
			assert.Contains(t, normalized.createCharts, "chart_a")
			assert.Contains(t, normalized.createCharts, "chart_b")
			require.Len(t, normalized.createDimsByID["chart_a"], 1)
			require.Len(t, normalized.createDimsByID["chart_b"], 1)
		})
	}
}
