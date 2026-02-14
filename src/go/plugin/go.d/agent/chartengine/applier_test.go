// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"
)

func TestApplyPlanEmitsNetdataWire(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := program.ChartMeta{
		Title:     "NIC traffic",
		Family:    "Net",
		Context:   "nic_traffic",
		Units:     "bytes/s",
		Algorithm: program.AlgorithmIncremental,
		Type:      program.ChartTypeLine,
		Priority:  1,
	}

	plan := Plan{
		Actions: []EngineAction{
			CreateChartAction{
				ChartTemplateID: "g0c0",
				ChartID:         "win_nic_traffic_eth0",
				Meta:            meta,
			},
			CreateDimensionAction{
				ChartID:    "win_nic_traffic_eth0",
				ChartMeta:  meta,
				Name:       "received",
				Hidden:     false,
				Algorithm:  program.AlgorithmIncremental,
				Multiplier: 1,
				Divisor:    1,
			},
			UpdateChartAction{
				ChartID: "win_nic_traffic_eth0",
				Values: []UpdateDimensionValue{
					{
						Name:    "received",
						IsFloat: true,
						Float64: 123.5,
					},
				},
			},
			RemoveDimensionAction{
				ChartID:    "win_nic_traffic_eth0",
				ChartMeta:  meta,
				Name:       "received",
				Hidden:     false,
				Algorithm:  program.AlgorithmIncremental,
				Multiplier: 1,
				Divisor:    1,
			},
			RemoveChartAction{
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
	assert.Contains(t, out, "SET 'received' = 123.5")
	assert.Contains(t, out, "DIMENSION 'received' 'received' 'incremental' '1' '1' 'obsolete'")
	assert.Contains(t, out, "obsolete")

	createPos := strings.Index(out, "CHART 'collector.job.win_nic_traffic_eth0'")
	beginPos := strings.Index(out, "BEGIN 'collector.job.win_nic_traffic_eth0' 100")
	require.NotEqual(t, -1, createPos)
	require.NotEqual(t, -1, beginPos)
	assert.Less(t, createPos, beginPos)
}

func TestApplyPlanEmitsOneChartPerCreateDimensionBatch(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := program.ChartMeta{
		Title:     "NIC traffic",
		Family:    "Net",
		Context:   "nic_traffic",
		Units:     "bytes/s",
		Algorithm: program.AlgorithmIncremental,
		Type:      program.ChartTypeLine,
		Priority:  1,
	}

	plan := Plan{
		Actions: []EngineAction{
			CreateChartAction{
				ChartTemplateID: "g0c0",
				ChartID:         "win_nic_traffic_eth0",
				Meta:            meta,
			},
			CreateDimensionAction{
				ChartID:    "win_nic_traffic_eth0",
				ChartMeta:  meta,
				Name:       "received",
				Algorithm:  program.AlgorithmIncremental,
				Multiplier: 1,
				Divisor:    1,
			},
			CreateDimensionAction{
				ChartID:    "win_nic_traffic_eth0",
				ChartMeta:  meta,
				Name:       "sent",
				Algorithm:  program.AlgorithmIncremental,
				Multiplier: 1,
				Divisor:    1,
			},
		},
	}

	err := ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 5,
	})
	require.NoError(t, err)

	out := buf.String()
	assert.Equal(t, 1, strings.Count(out, "CHART 'collector.job.win_nic_traffic_eth0'"))
	assert.Equal(t, 2, strings.Count(out, "DIMENSION '"))
}

func TestApplyPlanChartLabelsOverrideJobLabels(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)

	meta := program.ChartMeta{
		Title:     "NIC traffic",
		Family:    "Net",
		Context:   "nic_traffic",
		Units:     "bytes/s",
		Algorithm: program.AlgorithmIncremental,
		Type:      program.ChartTypeLine,
		Priority:  1,
	}
	plan := Plan{
		Actions: []EngineAction{
			CreateChartAction{
				ChartTemplateID: "g0c0",
				ChartID:         "win_nic_traffic_eth0",
				Meta:            meta,
				Labels: map[string]string{
					"instance":     "eth0",
					"interface":    "ethernet",
					"_collect_job": "ignored",
				},
			},
		},
	}

	err := ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 5,
		JobName:     "job01",
		JobLabels: map[string]string{
			"instance": "localhost",
			"owner":    "ops",
		},
	})
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "CLABEL 'owner' 'ops' '2'")
	assert.Contains(t, out, "CLABEL 'instance' 'eth0' '1'")
	assert.Contains(t, out, "CLABEL 'interface' 'ethernet' '1'")
	assert.Contains(t, out, "CLABEL '_collect_job' 'job01' '1'")
	assert.NotContains(t, out, "CLABEL 'instance' 'localhost' '2'")
	assert.NotContains(t, out, "CLABEL '_collect_job' 'ignored' '1'")
}
