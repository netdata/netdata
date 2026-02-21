// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/chartengine/internal/program"
)

func TestEnforceChartInstanceCapsSoftWhenAllExistingAreActive(t *testing.T) {
	const currentSuccessSeq = 42
	const templateID = "g0.c0"

	lifecycle := program.LifecyclePolicy{
		MaxInstances: 1,
	}
	state := newMaterializedState()
	state.charts["win_nic_traffic_eth0"] = &materializedChartState{
		templateID:         templateID,
		lifecycle:          lifecycle,
		lastSeenSuccessSeq: currentSuccessSeq,
		dimensions:         make(map[string]*materializedDimensionState),
	}
	state.charts["win_nic_traffic_eth1"] = &materializedChartState{
		templateID:         templateID,
		lifecycle:          lifecycle,
		lastSeenSuccessSeq: currentSuccessSeq,
		dimensions:         make(map[string]*materializedDimensionState),
	}

	chartsByID := map[string]*chartState{
		"win_nic_traffic_eth0": {
			templateID: templateID,
			lifecycle:  lifecycle,
			entries:    make(map[string]*dimBuildEntry),
		},
		"win_nic_traffic_eth1": {
			templateID: templateID,
			lifecycle:  lifecycle,
			entries:    make(map[string]*dimBuildEntry),
		},
	}

	removeCharts := enforceChartInstanceCaps(currentSuccessSeq, chartsByID, &state)
	assert.Empty(t, removeCharts)
	assert.Len(t, chartsByID, 2)
	assert.Len(t, state.charts, 2)
}
