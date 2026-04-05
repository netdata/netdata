// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"bytes"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
)

func TestApplyPlanEmitsNetdataWire(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine.ChartMeta{
		Title:     "NIC traffic",
		Family:    "Net",
		Context:   "nic_traffic",
		Units:     "bytes/s",
		Algorithm: chartengine.AlgorithmIncremental,
		Type:      chartengine.ChartTypeLine,
		Priority:  1,
	}

	plan := Plan{
		Actions: []EngineAction{
			chartengine.CreateChartAction{
				ChartTemplateID: "g0c0",
				ChartID:         "win_nic_traffic_eth0",
				Meta:            meta,
			},
			chartengine.CreateDimensionAction{
				ChartID:    "win_nic_traffic_eth0",
				ChartMeta:  meta,
				Name:       "received",
				Hidden:     false,
				Float:      true,
				Algorithm:  chartengine.AlgorithmIncremental,
				Multiplier: 1,
				Divisor:    1,
			},
			chartengine.UpdateChartAction{
				ChartID: "win_nic_traffic_eth0",
				Values: []chartengine.UpdateDimensionValue{
					{
						Name:    "received",
						IsFloat: true,
						Float64: 123.5,
					},
				},
			},
			chartengine.RemoveDimensionAction{
				ChartID:    "win_nic_traffic_eth0",
				ChartMeta:  meta,
				Name:       "received",
				Hidden:     false,
				Float:      true,
				Algorithm:  chartengine.AlgorithmIncremental,
				Multiplier: 1,
				Divisor:    1,
			},
			chartengine.RemoveChartAction{
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
	assert.Equal(t, `HOST ''

CHART 'collector.job.win_nic_traffic_eth0' '' 'NIC traffic' 'bytes/s' 'Net' 'nic_traffic' 'line' '1' '5' '' 'go.d.plugin' 'windows'
CLABEL 'instance' 'localhost' '2'
CLABEL '_collect_job' 'job01' '1'
CLABEL_COMMIT
DIMENSION 'received' 'received' 'incremental' '1' '1' 'type=float'
BEGIN 'collector.job.win_nic_traffic_eth0' 100
SET 'received' = 123.5
END

CHART 'collector.job.win_nic_traffic_eth0' '' 'NIC traffic' 'bytes/s' 'Net' 'nic_traffic' 'line' '1' '5' '' 'go.d.plugin' 'windows'
DIMENSION 'received' 'received' 'incremental' '1' '1' 'obsolete type=float'
CHART 'collector.job.win_nic_traffic_eth0' '' 'NIC traffic' 'bytes/s' 'Net' 'nic_traffic' 'line' '1' '5' 'obsolete' 'go.d.plugin' 'windows'
`, out)
}

func TestApplyPlanAutogenChartCreateUpdateRemove(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine.ChartMeta{
		Title:     `Metric "svc.errors_total"`,
		Family:    "svc_errors",
		Context:   "svc.errors_total",
		Units:     "events/s",
		Algorithm: chartengine.AlgorithmIncremental,
		Type:      chartengine.ChartTypeLine,
	}

	plan := Plan{
		Actions: []EngineAction{
			chartengine.CreateChartAction{
				ChartTemplateID: "__autogen__:svc.errors_total-method=GET",
				ChartID:         "svc.errors_total-method=GET",
				Meta:            meta,
				Labels: map[string]string{
					"method": "GET",
				},
			},
			chartengine.CreateDimensionAction{
				ChartID:    "svc.errors_total-method=GET",
				ChartMeta:  meta,
				Name:       "svc.errors_total",
				Algorithm:  chartengine.AlgorithmIncremental,
				Multiplier: 1,
				Divisor:    1,
			},
			chartengine.UpdateChartAction{
				ChartID: "svc.errors_total-method=GET",
				Values: []chartengine.UpdateDimensionValue{
					{
						Name:    "svc.errors_total",
						IsFloat: true,
						Float64: 10,
					},
				},
			},
			chartengine.RemoveChartAction{
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
	assert.Equal(t, `HOST ''

CHART 'collector.job.svc.errors_total-method=GET' '' 'Metric "svc.errors_total"' 'events/s' 'svc_errors' 'svc.errors_total' 'line' '0' '1' '' 'go.d.plugin' 'prometheus'
CLABEL 'method' 'GET' '1'
CLABEL '_collect_job' 'job01' '1'
CLABEL_COMMIT
DIMENSION 'svc.errors_total' 'svc.errors_total' 'incremental' '1' '1' ''
BEGIN 'collector.job.svc.errors_total-method=GET'
SET 'svc.errors_total' = 10
END

CHART 'collector.job.svc.errors_total-method=GET' '' 'Metric "svc.errors_total"' 'events/s' 'svc_errors' 'svc.errors_total' 'line' '0' '1' 'obsolete' 'go.d.plugin' 'prometheus'
`, out)
}

func TestApplyPlanUsesIntegerSETForNonFloatUpdates(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine.ChartMeta{
		Title:     "Runtime jobs",
		Family:    "Runtime",
		Context:   "runtime.jobs",
		Units:     "jobs",
		Algorithm: chartengine.AlgorithmAbsolute,
		Type:      chartengine.ChartTypeLine,
	}

	plan := Plan{
		Actions: []EngineAction{
			chartengine.CreateChartAction{
				ChartTemplateID: "g0c0",
				ChartID:         "runtime_jobs",
				Meta:            meta,
			},
			chartengine.CreateDimensionAction{
				ChartID:    "runtime_jobs",
				ChartMeta:  meta,
				Name:       "total",
				Algorithm:  chartengine.AlgorithmAbsolute,
				Multiplier: 1,
				Divisor:    1,
			},
			chartengine.UpdateChartAction{
				ChartID: "runtime_jobs",
				Values: []chartengine.UpdateDimensionValue{
					{
						Name:    "total",
						IsFloat: false,
						Int64:   7,
						Float64: 7.9,
					},
				},
			},
		},
	}

	err := ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "runtime",
		JobName:     "job01",
		MSSinceLast: 1,
	})
	require.NoError(t, err)

	out := buf.String()
	assert.Equal(t, `HOST ''

CHART 'collector.job.runtime_jobs' '' 'Runtime jobs' 'jobs' 'Runtime' 'runtime.jobs' 'line' '0' '1' '' 'go.d.plugin' 'runtime'
CLABEL '_collect_job' 'job01' '1'
CLABEL_COMMIT
DIMENSION 'total' 'total' 'absolute' '1' '1' ''
BEGIN 'collector.job.runtime_jobs' 1
SET 'total' = 7
END

`, out)
	assert.NotContains(t, out, "SET 'total' = 7.9")
}

func TestApplyPlanDimensionOnlyCreateEmitsLabelsAndCommit(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine.ChartMeta{
		Title:     "Dimension-only chart",
		Family:    "Runtime",
		Context:   "runtime.dimension_only",
		Units:     "1",
		Algorithm: chartengine.AlgorithmAbsolute,
		Type:      chartengine.ChartTypeLine,
	}

	plan := Plan{
		Actions: []EngineAction{
			chartengine.CreateDimensionAction{
				ChartID:    "dimension_only_chart",
				ChartMeta:  meta,
				Name:       "value",
				Algorithm:  chartengine.AlgorithmAbsolute,
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
	assert.Equal(t, `HOST ''

CHART 'collector.job.dimension_only_chart' '' 'Dimension-only chart' '1' 'Runtime' 'runtime.dimension_only' 'line' '0' '1' '' 'go.d.plugin' 'runtime'
CLABEL 'instance' 'localhost' '2'
CLABEL '_collect_job' 'job01' '1'
CLABEL_COMMIT
DIMENSION 'value' 'value' 'absolute' '1' '1' ''
`, out)
}

func TestApplyPlanSanitizesWireValues(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine.ChartMeta{
		Title:     "Title'\n",
		Family:    "Family'\n",
		Context:   "Context'\n",
		Units:     "units'\n",
		Algorithm: chartengine.AlgorithmAbsolute,
		Type:      chartengine.ChartTypeLine,
	}

	plan := Plan{
		Actions: []EngineAction{
			chartengine.CreateChartAction{
				ChartTemplateID: "g0.c0",
				ChartID:         "chart'id\n",
				Meta:            meta,
				Labels: map[string]string{
					"la'bel\n": "va'lue\n",
				},
			},
			chartengine.CreateDimensionAction{
				ChartID:    "chart'id\n",
				ChartMeta:  meta,
				Name:       "dim'name\n",
				Algorithm:  chartengine.AlgorithmAbsolute,
				Multiplier: 1,
				Divisor:    1,
			},
			chartengine.UpdateChartAction{
				ChartID: "chart'id\n",
				Values: []chartengine.UpdateDimensionValue{
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
	assert.Equal(t, `HOST ''

CHART 'collector.job.chartid' '' 'Title ' 'units ' 'Family ' 'Context ' 'line' '0' '1' '' 'go.dplugin ' 'module '
CLABEL 'instance' 'localhost ' '2'
CLABEL 'label' 'value ' '1'
CLABEL '_collect_job' 'job01 ' '1'
CLABEL_COMMIT
DIMENSION 'dimname' 'dimname' 'absolute' '1' '1' ''
BEGIN 'collector.job.chartid' 1
SET 'dimname' = 5
END

`, out)
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

func TestApplyPlanDefaultGlobalHostSelection(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine.ChartMeta{
		Title:   "Requests",
		Family:  "Service",
		Context: "requests",
		Units:   "req/s",
		Type:    chartengine.ChartTypeLine,
	}
	plan := Plan{
		Actions: []EngineAction{
			chartengine.CreateChartAction{
				ChartID: "requests",
				Meta:    meta,
			},
		},
	}

	require.NoError(t, ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "httpcheck",
		JobName:     "job01",
	}))

	out := buf.String()
	assert.Equal(t, `HOST ''

CHART 'collector.job.requests' '' 'Requests' 'req/s' 'Service' 'requests' 'line' '0' '1' '' 'go.d.plugin' 'httpcheck'
CLABEL '_collect_job' 'job01' '1'
CLABEL_COMMIT
`, out)
}

func TestApplyPlanVnodeHostSelectionAndDefine(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine.ChartMeta{
		Title:   "Workers Busy",
		Family:  "Workers",
		Context: "workers_busy",
		Units:   "workers",
		Type:    chartengine.ChartTypeLine,
	}
	info, err := PrepareHostInfo(netdataapi.HostInfo{
		GUID:     "node-guid",
		Hostname: "node-host",
		Labels: map[string]string{
			"region": "eu'\n",
		},
	})
	require.NoError(t, err)

	plan := Plan{
		Actions: []EngineAction{
			chartengine.CreateChartAction{
				ChartID: "workers_busy",
				Meta:    meta,
			},
		},
	}

	require.NoError(t, ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "apache",
		JobName:     "job01",
		HostScope: &HostScope{
			GUID:   "node-guid",
			Define: &info,
		},
	}))

	out := buf.String()
	assert.Equal(t, `HOST_DEFINE 'node-guid' 'node-host'
HOST_LABEL '_hostname' 'node-host'
HOST_LABEL 'region' 'eu '
HOST_DEFINE_END

HOST 'node-guid'

CHART 'collector.job.workers_busy' '' 'Workers Busy' 'workers' 'Workers' 'workers_busy' 'line' '0' '1' '' 'go.d.plugin' 'apache'
CLABEL '_collect_job' 'job01' '1'
CLABEL_COMMIT
`, out)
}

func TestApplyPlanSkipsHostSelectionForEmptyPlans(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	require.NoError(t, ApplyPlan(api, Plan{}, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "runtime",
		JobName:     "job01",
	}))
	assert.Equal(t, "", buf.String())
}

func TestApplyPlanRejectsMismatchedHostDefine(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine.ChartMeta{
		Title:   "Requests",
		Family:  "Service",
		Context: "requests",
		Units:   "req/s",
		Type:    chartengine.ChartTypeLine,
	}
	plan := Plan{
		Actions: []EngineAction{
			chartengine.CreateChartAction{
				ChartID: "requests",
				Meta:    meta,
			},
		},
	}

	err := ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "httpcheck",
		JobName:     "job01",
		HostScope: &HostScope{
			GUID: "guid-a",
			Define: &netdataapi.HostInfo{
				GUID:     "guid-b",
				Hostname: "node-host",
			},
		},
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "does not match")
	assert.Equal(t, "", buf.String())
}

func TestApplyPlanRemoveOnlyBatchStillSelectsHost(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := chartengine.ChartMeta{
		Title:   "Requests",
		Family:  "Service",
		Context: "requests",
		Units:   "req/s",
		Type:    chartengine.ChartTypeLine,
	}
	plan := Plan{
		Actions: []EngineAction{
			chartengine.RemoveChartAction{
				ChartID: "requests",
				Meta:    meta,
			},
		},
	}

	require.NoError(t, ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "httpcheck",
		JobName:     "job01",
	}))

	out := buf.String()
	assert.Equal(t, `HOST ''

CHART 'collector.job.requests' '' 'Requests' 'req/s' 'Service' 'requests' 'line' '0' '1' 'obsolete' 'go.d.plugin' 'httpcheck'
`, out)
}

func TestNormalizeActionsOrderingDeterminism(t *testing.T) {
	meta := chartengine.ChartMeta{
		Title:     "Requests",
		Family:    "Service",
		Context:   "requests",
		Units:     "requests/s",
		Algorithm: chartengine.AlgorithmIncremental,
		Type:      chartengine.ChartTypeLine,
	}

	tests := map[string]struct {
		actions []EngineAction
	}{
		"preserves update/remove order and groups create actions by chart id": {
			actions: []EngineAction{
				chartengine.UpdateChartAction{ChartID: "chart_b"},
				chartengine.UpdateChartAction{ChartID: "chart_a"},
				chartengine.RemoveDimensionAction{ChartID: "chart_b", Name: "dim_b", ChartMeta: meta},
				chartengine.RemoveDimensionAction{ChartID: "chart_a", Name: "dim_a", ChartMeta: meta},
				chartengine.RemoveChartAction{ChartID: "chart_b", Meta: meta},
				chartengine.RemoveChartAction{ChartID: "chart_a", Meta: meta},
				chartengine.CreateDimensionAction{ChartID: "chart_b", Name: "dim_b", ChartMeta: meta},
				chartengine.CreateChartAction{ChartID: "chart_b", Meta: meta},
				chartengine.CreateDimensionAction{ChartID: "chart_a", Name: "dim_a", ChartMeta: meta},
				chartengine.CreateChartAction{ChartID: "chart_a", Meta: meta},
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
