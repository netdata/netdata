// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The oracle is the plugin protocol contract shared by the native Windows
// plugin (nic_speed_max) and V1 chart Vars: a chart variable is defined as nan
// after the chart's dimensions, set inside BEGIN..END when a value exists, and
// left untouched (the Agent keeps its last value) when it does not.
const chartVariablesTemplate = `
version: v1
engine:
  autogen:
    enabled: true
groups:
  - family: net
    context_namespace: net
    metrics: [net_received_bytes, net_speed_bits]
    charts:
      - context: net
        title: Bandwidth
        units: kilobits/s
        aggregation: max
        instances:
          by_labels: [device]
        dimensions:
          - selector: net_received_bytes
            name: received
        variables:
          - name: nic_speed_max
            selector: net_speed_bits
            options:
              divisor: 1000
          - name: nic_speed_bytes
            selector: net_speed_bits
            options:
              multiplier: 1
              divisor: 8
`

type variableObservation struct {
	metric string
	labels []metrix.Label
	value  float64
}

func deviceObs(metric, device string, value float64, extra ...metrix.Label) variableObservation {
	return variableObservation{
		metric: metric,
		labels: append([]metrix.Label{{Key: "device", Value: device}}, extra...),
		value:  value,
	}
}

func TestApplyPlanChartVariables(t *testing.T) {
	tests := map[string]struct {
		cycles [][]variableObservation
		want   [][]string
	}{
		"creation presets nan after dimensions, then the scaled value is set inside the update": {
			cycles: [][]variableObservation{{
				deviceObs("net_received_bytes", "eth0", 100),
				deviceObs("net_speed_bits", "eth0", 1e9),
			}},
			want: [][]string{{
				"CHART collector.job.net_net_eth0",
				"DIMENSION received",
				"VARIABLE CHART 'nic_speed_max' = nan",
				"VARIABLE CHART 'nic_speed_bytes' = nan",
				"BEGIN collector.job.net_net_eth0",
				"SET 'received' = 100",
				"VARIABLE CHART 'nic_speed_max' = 1000000",
				"VARIABLE CHART 'nic_speed_bytes' = 125000000",
				"END",
			}},
		},
		"a cycle without the variable series sends no variable": {
			cycles: [][]variableObservation{
				{deviceObs("net_received_bytes", "eth0", 100), deviceObs("net_speed_bits", "eth0", 1e9)},
				{deviceObs("net_received_bytes", "eth0", 200)},
			},
			want: [][]string{
				nil, // checked by the previous case
				{
					"BEGIN collector.job.net_net_eth0",
					"SET 'received' = 200",
					"END",
				},
			},
		},
		"each chart instance gets the value its identity labels select": {
			cycles: [][]variableObservation{{
				deviceObs("net_received_bytes", "eth0", 100),
				deviceObs("net_received_bytes", "eth1", 300),
				deviceObs("net_speed_bits", "eth1", 1e8),
				deviceObs("net_speed_bits", "eth0", 1e9),
			}},
			want: [][]string{{
				"CHART collector.job.net_net_eth0",
				"DIMENSION received",
				"VARIABLE CHART 'nic_speed_max' = nan",
				"VARIABLE CHART 'nic_speed_bytes' = nan",
				"CHART collector.job.net_net_eth1",
				"DIMENSION received",
				"VARIABLE CHART 'nic_speed_max' = nan",
				"VARIABLE CHART 'nic_speed_bytes' = nan",
				"BEGIN collector.job.net_net_eth0",
				"SET 'received' = 100",
				"VARIABLE CHART 'nic_speed_max' = 1000000",
				"VARIABLE CHART 'nic_speed_bytes' = 125000000",
				"END",
				"BEGIN collector.job.net_net_eth1",
				"SET 'received' = 300",
				"VARIABLE CHART 'nic_speed_max' = 100000",
				"VARIABLE CHART 'nic_speed_bytes' = 12500000",
				"END",
			}},
		},
		"several series of one instance reduce with the chart aggregation": {
			cycles: [][]variableObservation{{
				deviceObs("net_received_bytes", "eth0", 100),
				deviceObs("net_speed_bits", "eth0", 1e8, metrix.Label{
					Key:   "port",
					Value: "a",
				}),
				deviceObs("net_speed_bits", "eth0", 1e9, metrix.Label{
					Key:   "port",
					Value: "b",
				}),
			}},
			want: [][]string{{
				"CHART collector.job.net_net_eth0",
				"DIMENSION received",
				"VARIABLE CHART 'nic_speed_max' = nan",
				"VARIABLE CHART 'nic_speed_bytes' = nan",
				"BEGIN collector.job.net_net_eth0",
				"SET 'received' = 100",
				"VARIABLE CHART 'nic_speed_max' = 1000000",
				"VARIABLE CHART 'nic_speed_bytes' = 125000000",
				"END",
			}},
		},
		"a variable never creates a chart and its series never falls back to autogen": {
			cycles: [][]variableObservation{{
				deviceObs("net_speed_bits", "eth0", 1e9),
			}},
			want: [][]string{nil},
		},
		"a variable series without the identity labels binds to no chart and falls back like a dimension series": {
			cycles: [][]variableObservation{{
				deviceObs("net_received_bytes", "eth0", 100),
				{metric: "net_speed_bits", value: 1e9},
			}},
			want: [][]string{{
				"CHART collector.job.net_net_eth0",
				"DIMENSION received",
				"VARIABLE CHART 'nic_speed_max' = nan",
				"VARIABLE CHART 'nic_speed_bytes' = nan",
				"CHART collector.job.net_speed_bits",
				"DIMENSION net_speed_bits",
				"BEGIN collector.job.net_net_eth0",
				"SET 'received' = 100",
				"END",
				"BEGIN collector.job.net_speed_bits",
				"SET 'net_speed_bits' = 1000000000",
				"END",
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Len(t, tc.want, len(tc.cycles))
			f := newRevivalFixture(t, chartVariablesTemplate, 0)
			for i, observations := range tc.cycles {
				wire := f.publish(t, f.collect(t, func() {
					for _, obs := range observations {
						meter := f.store.Write().SnapshotMeter("")
						if len(obs.labels) > 0 {
							meter = meter.WithLabels(obs.labels...)
						}
						meter.Gauge(obs.metric).Observe(obs.value)
					}
				}))
				if tc.want[i] == nil && i < len(tc.cycles)-1 {
					continue
				}
				assert.Equal(t, tc.want[i], chartVariableWire(wire), "cycle %d", i)
			}
		})
	}
}

// chartVariableWire keeps the commands that show variable placement: chart and
// dimension definitions by ID, and update blocks with their SET and VARIABLE lines.
func chartVariableWire(wire string) []string {
	var out []string
	for _, line := range strings.Split(wire, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "CHART", "DIMENSION", "BEGIN":
			out = append(out, fields[0]+" "+strings.Trim(fields[1], "'"))
		case "SET", "VARIABLE":
			out = append(out, line)
		case "END":
			out = append(out, "END")
		}
	}
	return out
}

func TestApplyPlanChartVariableUnavailableValue(t *testing.T) {
	// An unavailable MeasureSet field is NaN in the snapshot; the variable is
	// sent as nan (unknown), the only non-finite spelling the Agent parses.
	const template = `
version: v1
groups:
  - family: net
    metrics: [net_link_received, net_link_speed]
    charts:
      - context: link
        title: Link
        units: bits/s
        instances:
          by_labels: [device]
        dimensions:
          - selector: net_link_received
            name: received
        variables:
          - name: link_speed
            selector: net_link_speed
`
	f := newRevivalFixture(t, template, 0)
	wire := f.publish(t, f.collect(t, func() {
		f.store.Write().SnapshotMeter("").WithLabels(metrix.Label{
			Key:   "device",
			Value: "eth0",
		}).MeasureSetGauge(
			"net_link",
			metrix.WithMeasureSetFields(
				metrix.MeasureFieldSpec{
					Name: "received",
				},
				metrix.MeasureFieldSpec{
					Name:  "speed",
					Float: true,
				},
			),
		).ObserveFields(map[string]metrix.SampleValue{
			"received": 10,
			"speed":    math.NaN(),
		})
	}))
	assert.Equal(t, []string{
		"CHART collector.job.link_eth0",
		"DIMENSION received",
		"VARIABLE CHART 'link_speed' = nan",
		"BEGIN collector.job.link_eth0",
		"SET 'received' = 10",
		"VARIABLE CHART 'link_speed' = nan",
		"END",
	}, chartVariableWire(wire))
}

func TestApplyPlanChartVariablesEmitsOnlyFromChartScope(t *testing.T) {
	// The Agent rejects VARIABLE CHART without a scope chart; the emitter may
	// write it only after CHART/DIMENSION of a created chart or inside BEGIN..END.
	var wire bytes.Buffer
	plan := Plan{
		Actions: []EngineAction{
			chartengine.UpdateChartAction{
				ChartID:   "net_net_eth0",
				Values:    []UpdateDimensionValue{{Name: "received", Int64: 1}},
				Variables: []UpdateVariableValue{{Name: "nic_speed_max", Value: 1000}},
			},
		},
	}
	require.NoError(t, ApplyPlan(netdataapi.New(&wire), plan, EmitEnv{
		TypeID:      "collector.job",
		CollectedAt: testSample,
	}))
	lines := strings.Split(strings.TrimSpace(wire.String()), "\n")
	require.GreaterOrEqual(t, len(lines), 4)
	assert.Equal(t, "BEGIN 'collector.job.net_net_eth0'", lines[len(lines)-4])
	assert.Equal(t, "SET 'received' = 1", lines[len(lines)-3])
	assert.Equal(t, "VARIABLE CHART 'nic_speed_max' = 1000", lines[len(lines)-2])
	assert.True(t, strings.HasPrefix(lines[len(lines)-1], "END "))
}
