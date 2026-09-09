// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"bytes"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyPlanUpdatesChartLabelsWithoutDimensions(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)
	meta := chartengine.ChartMeta{
		Title:   "Service",
		Family:  "Service",
		Context: "service.value",
		Units:   "value",
		Type:    chartengine.ChartTypeLine,
	}
	plan := Plan{Actions: []EngineAction{
		chartengine.UpdateChartLabelsAction{
			ChartID: "service_node-1",
			Meta:    meta,
			Labels:  map[string]string{"owner": "owner-b"},
		},
		chartengine.UpdateChartAction{
			ChartID: "service_node-1",
			Values:  []chartengine.UpdateDimensionValue{{Name: "value", Int64: 1}},
		},
	}}

	err := ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "service",
		JobName:     "job01",
		JobLabels:   map[string]string{"configured": "label"},
	})
	require.NoError(t, err)

	assert.Equal(t, "HOST ''\n\n"+
		"CHART 'collector.job.service_node-1' '' 'Service' 'value' 'Service' 'service.value' 'line' '0' '1' '' 'go.d.plugin' 'service'\n"+
		"CLABEL 'configured' 'label' '2'\n"+
		"CLABEL 'owner' 'owner-b' '1'\n"+
		"CLABEL '_collect_job' 'job01' '1'\n"+
		"CLABEL_COMMIT\n"+
		"BEGIN 'collector.job.service_node-1'\n"+
		"SET 'value' = 1\n"+
		"END\n\n", buf.String())
	assert.NotContains(t, buf.String(), "DIMENSION")
}

func TestApplyPlanLabelReplacementKeepsConfiguredAndReservedLabels(t *testing.T) {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)
	plan := Plan{Actions: []EngineAction{
		chartengine.UpdateChartLabelsAction{
			ChartID: "service_node-1",
			Meta: chartengine.ChartMeta{
				Title:   "Service",
				Family:  "Service",
				Context: "service.value",
				Units:   "value",
				Type:    chartengine.ChartTypeLine,
			},
			Labels: nil,
		},
	}}

	err := ApplyPlan(api, plan, EmitEnv{
		TypeID:      "collector.job",
		UpdateEvery: 1,
		Plugin:      "go.d.plugin",
		Module:      "service",
		JobName:     "job01",
		JobLabels:   map[string]string{"configured": "label"},
	})
	require.NoError(t, err)

	assert.Equal(t, "HOST ''\n\n"+
		"CHART 'collector.job.service_node-1' '' 'Service' 'value' 'Service' 'service.value' 'line' '0' '1' '' 'go.d.plugin' 'service'\n"+
		"CLABEL 'configured' 'label' '2'\n"+
		"CLABEL '_collect_job' 'job01' '1'\n"+
		"CLABEL_COMMIT\n", buf.String())
	assert.NotContains(t, buf.String(), "DIMENSION")
	assert.NotContains(t, buf.String(), "BEGIN")
}
