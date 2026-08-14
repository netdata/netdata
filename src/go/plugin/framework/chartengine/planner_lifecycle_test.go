// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlannerLifecycleScenarios(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"enforce lifecycle caps dimension cap evicts lru": {
			run: runTestEnforceLifecycleCapsDimensionCapEvictsLRU,
		},
		"enforce lifecycle caps mixed policies": {
			run: runTestEnforceLifecycleCapsMixedPolicies,
		},
		"dimension cap removal retries after abort": {
			run: runTestDimensionCapRemovalRetriesAfterAbort,
		},
		"collect expiry removals dimension and chart expiry": {
			run: runTestCollectExpiryRemovalsDimensionAndChartExpiry,
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func runTestEnforceLifecycleCapsDimensionCapEvictsLRU(t *testing.T) {
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
				algorithm:  dimensionAlgorithmAbsolute,
				multiplier: 1,
				divisor:    1,
			})
			require.True(t, created)
			oldA.lastSeenSuccessSeq = 1

			oldB, created := matChart.ensureDimension("old_b", dimensionState{
				algorithm:  dimensionAlgorithmAbsolute,
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
								algorithm:  dimensionAlgorithmAbsolute,
								multiplier: 1,
								divisor:    1,
							},
						},
					},
				},
			}

			removeDims, removeCharts := enforceLifecycleCapsWithObserver(tc.currentSeq, chartsByID, &state, nil)

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

func runTestEnforceLifecycleCapsMixedPolicies(t *testing.T) {
	const currentSeq = uint64(10)

	meta := program.ChartMeta{Title: "Requests", Context: "requests", Units: "requests/s"}
	state := newMaterializedState()

	disabledLifecycle := program.LifecyclePolicy{}
	disabledActive, created := state.ensureChart(
		"disabled_active",
		"tpl.disabled",
		meta,
		disabledLifecycle,
	)
	require.True(t, created)
	disabledActive.lastSeenSuccessSeq = 1
	for _, name := range []string{"old_a", "old_b"} {
		dim, created := disabledActive.ensureDimension(name, dimensionState{
			algorithm:  dimensionAlgorithmAbsolute,
			multiplier: 1,
			divisor:    1,
		})
		require.True(t, created)
		dim.lastSeenSuccessSeq = 1
	}
	disabledStale, created := state.ensureChart(
		"disabled_stale",
		"tpl.disabled",
		meta,
		disabledLifecycle,
	)
	require.True(t, created)
	disabledStale.lastSeenSuccessSeq = 1

	enabledOld, created := state.ensureChart(
		"enabled_old",
		"tpl.enabled",
		meta,
		program.LifecyclePolicy{MaxInstances: 1},
	)
	require.True(t, created)
	enabledOld.lastSeenSuccessSeq = 1

	dimsLifecycle := program.LifecyclePolicy{
		Dimensions: program.DimensionLifecyclePolicy{MaxDims: 1},
	}
	dimsActive, created := state.ensureChart("dims_active", "tpl.dims", meta, dimsLifecycle)
	require.True(t, created)
	dimsActive.lastSeenSuccessSeq = currentSeq
	oldDim, created := dimsActive.ensureDimension("old", dimensionState{
		algorithm:  dimensionAlgorithmAbsolute,
		multiplier: 1,
		divisor:    1,
	})
	require.True(t, created)
	oldDim.lastSeenSuccessSeq = 1

	chartsByID := map[string]*chartState{
		"disabled_active": {
			templateID:      "tpl.disabled",
			chartID:         "disabled_active",
			meta:            meta,
			lifecycle:       disabledLifecycle,
			currentBuildSeq: currentSeq,
			observedCount:   1,
			entries: map[string]*dimBuildEntry{
				"new": observedDimensionEntry(currentSeq),
			},
		},
		"enabled_new": {
			templateID:      "tpl.enabled",
			chartID:         "enabled_new",
			meta:            meta,
			lifecycle:       program.LifecyclePolicy{MaxInstances: 1},
			currentBuildSeq: currentSeq,
			observedCount:   1,
			entries: map[string]*dimBuildEntry{
				"value": observedDimensionEntry(currentSeq),
			},
		},
		"dims_active": {
			templateID:      "tpl.dims",
			chartID:         "dims_active",
			meta:            meta,
			lifecycle:       dimsLifecycle,
			currentBuildSeq: currentSeq,
			observedCount:   1,
			entries: map[string]*dimBuildEntry{
				"new": observedDimensionEntry(currentSeq),
			},
		},
	}

	removeDims, removeCharts := enforceLifecycleCapsWithObserver(currentSeq, chartsByID, &state, nil)

	require.Len(t, removeCharts, 1)
	assert.Equal(t, "enabled_old", removeCharts[0].ChartID)
	require.Len(t, removeDims, 1)
	assert.Equal(t, "dims_active", removeDims[0].ChartID)
	assert.Equal(t, "old", removeDims[0].Name)

	assert.Contains(t, state.charts, "disabled_active")
	assert.Contains(t, state.charts, "disabled_stale")
	assert.Len(t, disabledActive.dimensions, 2)
	assert.Contains(t, chartsByID["disabled_active"].entries, "new")
}

func runTestDimensionCapRemovalRetriesAfterAbort(t *testing.T) {
	engine, err := New()
	require.NoError(t, err)

	require.NoError(t, engine.LoadYAML([]byte(maxDimsTemplateYAML(1)), 1))

	store := metrix.NewCollectorStore()
	cc := mustCycleController(t, store)
	sm := store.Write().SnapshotMeter("")
	gauge := sm.Gauge("svc_mode")
	modeA := sm.LabelSet(metrix.Label{Key: "mode", Value: "a"})
	modeB := sm.LabelSet(metrix.Label{Key: "mode", Value: "b"})

	cc.BeginCycle()
	gauge.Observe(1, modeA)
	require.NoError(t, cc.CommitCycleSuccess())
	_, err = buildPlan(engine, store.Read(metrix.ReadFlatten()))
	require.NoError(t, err)

	materialized := engine.state.materialized.charts["service_mode"]
	require.NotNil(t, materialized)
	assert.Contains(t, materialized.dimensions, "a")

	cc.BeginCycle()
	gauge.Observe(1, modeB)
	require.NoError(t, cc.CommitCycleSuccess())
	reader := store.Read(metrix.ReadFlatten())

	attempt, err := engine.PreparePlan(reader)
	require.NoError(t, err)
	require.Equal(t,
		[]ActionKind{ActionRemoveDimension, ActionCreateDimension, ActionUpdateChart},
		actionKinds(attempt.Plan().Actions),
	)
	removeDim := findRemoveDimensionAction(attempt.Plan())
	require.NotNil(t, removeDim)
	assert.Equal(t, "a", removeDim.Name)
	attempt.Abort()

	materialized = engine.state.materialized.charts["service_mode"]
	require.NotNil(t, materialized)
	assert.Contains(t, materialized.dimensions, "a")
	assert.NotContains(t, materialized.dimensions, "b")

	retry, err := engine.PreparePlan(reader)
	require.NoError(t, err)
	require.Equal(t,
		[]ActionKind{ActionRemoveDimension, ActionCreateDimension, ActionUpdateChart},
		actionKinds(retry.Plan().Actions),
	)
	removeDim = findRemoveDimensionAction(retry.Plan())
	require.NotNil(t, removeDim)
	assert.Equal(t, "a", removeDim.Name)
	require.NoError(t, retry.Commit())

	materialized = engine.state.materialized.charts["service_mode"]
	require.NotNil(t, materialized)
	assert.NotContains(t, materialized.dimensions, "a")
	assert.Contains(t, materialized.dimensions, "b")
}

func maxDimsTemplateYAML(maxDims int) string {
	return fmt.Sprintf(`
version: v1
groups:
  - family: Service
    metrics:
      - svc_mode
    charts:
      - id: service_mode
        title: Service mode
        context: service_mode
        units: state
        lifecycle:
          dimensions:
            max_dims: %d
        dimensions:
          - selector: svc_mode
            name_from_label: mode
`, maxDims)
}

func observedDimensionEntry(seenSeq uint64) *dimBuildEntry {
	return &dimBuildEntry{
		seenSeq: seenSeq,
		dimensionState: dimensionState{
			algorithm:  dimensionAlgorithmAbsolute,
			multiplier: 1,
			divisor:    1,
		},
	}
}

func runTestCollectExpiryRemovalsDimensionAndChartExpiry(t *testing.T) {
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
				algorithm:  dimensionAlgorithmAbsolute,
				multiplier: 1,
				divisor:    1,
			})
			require.True(t, created)
			staleDim.lastSeenSuccessSeq = 2

			freshDim, created := liveChart.ensureDimension("fresh_dim", dimensionState{
				algorithm:  dimensionAlgorithmAbsolute,
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
