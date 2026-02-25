// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnforceLifecycleCaps_DimensionCapEvictsLRU(t *testing.T) {
	tests := map[string]struct {
		currentSeq uint64
	}{
		"dimension cap evicts least-recently-seen inactive dimension first": {
			currentSeq: 10,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			meta := program.ChartMeta{Title: "Requests", Context: "requests", Units: "requests/s"}
			lifecycle := program.LifecyclePolicy{
				Dimensions: program.DimensionLifecyclePolicy{MaxDims: 2},
			}

			state := newMaterializedState()
			matChart, created := state.ensureChart("chart_a", "tpl.requests", meta, lifecycle)
			require.True(t, created)

			oldA, created := matChart.ensureDimension("old_a", dimensionState{
				algorithm:  program.AlgorithmAbsolute,
				multiplier: 1,
				divisor:    1,
			})
			require.True(t, created)
			oldA.lastSeenSuccessSeq = 1

			oldB, created := matChart.ensureDimension("old_b", dimensionState{
				algorithm:  program.AlgorithmAbsolute,
				multiplier: 1,
				divisor:    1,
			})
			require.True(t, created)
			oldB.lastSeenSuccessSeq = 2

			chartsByID := map[string]*chartState{
				"chart_a": {
					templateID:      "tpl.requests",
					chartID:         "chart_a",
					meta:            meta,
					lifecycle:       lifecycle,
					currentBuildSeq: tc.currentSeq,
					observedCount:   1,
					entries: map[string]*dimBuildEntry{
						"new_c": {
							seenSeq: tc.currentSeq,
							dimensionState: dimensionState{
								static:     false,
								order:      0,
								algorithm:  program.AlgorithmAbsolute,
								multiplier: 1,
								divisor:    1,
							},
						},
					},
				},
			}

			removeDims, removeCharts := enforceLifecycleCaps(tc.currentSeq, chartsByID, &state)

			assert.Empty(t, removeCharts)
			require.Len(t, removeDims, 1)
			assert.Equal(t, "chart_a", removeDims[0].ChartID)
			assert.Equal(t, "old_a", removeDims[0].Name)

			assert.NotContains(t, matChart.dimensions, "old_a")
			assert.Contains(t, matChart.dimensions, "old_b")
			assert.Contains(t, chartsByID["chart_a"].entries, "new_c")
		})
	}
}

func TestCollectExpiryRemovals_DimensionAndChartExpiry(t *testing.T) {
	tests := map[string]struct {
		currentSeq uint64
	}{
		"stale chart and stale dimension are removed in one pass": {
			currentSeq: 5,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			state := newMaterializedState()

			expiredMeta := program.ChartMeta{Title: "Expired", Context: "expired"}
			expiredChart, created := state.ensureChart("chart_expired", "tpl.expired", expiredMeta, program.LifecyclePolicy{
				ExpireAfterCycles: 2,
			})
			require.True(t, created)
			expiredChart.lastSeenSuccessSeq = 3

			liveMeta := program.ChartMeta{Title: "Live", Context: "live"}
			liveChart, created := state.ensureChart("chart_live", "tpl.live", liveMeta, program.LifecyclePolicy{
				Dimensions: program.DimensionLifecyclePolicy{ExpireAfterCycles: 2},
			})
			require.True(t, created)
			liveChart.lastSeenSuccessSeq = tc.currentSeq

			staleDim, created := liveChart.ensureDimension("stale_dim", dimensionState{
				algorithm:  program.AlgorithmAbsolute,
				multiplier: 1,
				divisor:    1,
			})
			require.True(t, created)
			staleDim.lastSeenSuccessSeq = 2

			freshDim, created := liveChart.ensureDimension("fresh_dim", dimensionState{
				algorithm:  program.AlgorithmAbsolute,
				multiplier: 1,
				divisor:    1,
			})
			require.True(t, created)
			freshDim.lastSeenSuccessSeq = 4

			removeDims, removeCharts := collectExpiryRemovals(tc.currentSeq, &state)

			require.Len(t, removeDims, 1)
			assert.Equal(t, "chart_live", removeDims[0].ChartID)
			assert.Equal(t, "stale_dim", removeDims[0].Name)

			require.Len(t, removeCharts, 1)
			assert.Equal(t, "chart_expired", removeCharts[0].ChartID)

			assert.NotContains(t, state.charts, "chart_expired")
			assert.NotContains(t, liveChart.dimensions, "stale_dim")
			assert.Contains(t, liveChart.dimensions, "fresh_dim")
		})
	}
}
