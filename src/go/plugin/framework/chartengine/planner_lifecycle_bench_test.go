// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

var benchmarkLifecycleRemovalCount int

func BenchmarkEnforceLifecycleCaps(b *testing.B) {
	tests := map[string]struct {
		chartCount int
		dimCount   int
		chartCaps  int
		dimCaps    int
	}{
		"disabled/charts_512": {
			chartCount: 512,
			dimCount:   8,
		},
		"disabled/charts_4096": {
			chartCount: 4096,
			dimCount:   8,
		},
		"chart_only/charts_512": {
			chartCount: 512,
			dimCount:   8,
			chartCaps:  512,
		},
		"chart_only/charts_4096": {
			chartCount: 4096,
			dimCount:   8,
			chartCaps:  4096,
		},
		"dimension_only/charts_512": {
			chartCount: 512,
			dimCount:   8,
			dimCaps:    512,
		},
		"dimension_only/charts_4096": {
			chartCount: 4096,
			dimCount:   8,
			dimCaps:    4096,
		},
		"mixed/charts_512_enabled_8": {
			chartCount: 512,
			dimCount:   8,
			chartCaps:  8,
			dimCaps:    8,
		},
		"mixed/charts_4096_enabled_64": {
			chartCount: 4096,
			dimCount:   8,
			chartCaps:  64,
			dimCaps:    64,
		},
		"both_enabled/charts_512": {
			chartCount: 512,
			dimCount:   8,
			chartCaps:  512,
			dimCaps:    512,
		},
		"both_enabled/charts_4096": {
			chartCount: 4096,
			dimCount:   8,
			chartCaps:  4096,
			dimCaps:    4096,
		},
	}

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			const currentSeq = uint64(10)
			chartsByID, state := benchmarkLifecycleCapFixture(
				currentSeq,
				tc.chartCount,
				tc.dimCount,
				tc.chartCaps,
				tc.dimCaps,
			)

			removeDims, removeCharts := enforceLifecycleCaps(currentSeq, chartsByID, &state)
			if len(removeDims) != 0 || len(removeCharts) != 0 {
				b.Fatalf("unexpected lifecycle removals: dimensions=%d charts=%d", len(removeDims), len(removeCharts))
			}

			b.ReportAllocs()
			b.ResetTimer()
			var removalCount int
			for range b.N {
				removeDims, removeCharts := enforceLifecycleCaps(currentSeq, chartsByID, &state)
				removalCount = len(removeDims) + len(removeCharts)
			}
			benchmarkLifecycleRemovalCount = removalCount
		})
	}
}

func benchmarkLifecycleCapFixture(
	currentSeq uint64,
	chartCount int,
	dimCount int,
	chartCaps int,
	dimCaps int,
) (map[string]*chartState, materializedState) {
	chartsByID := make(map[string]*chartState, chartCount)
	state := newMaterializedState()
	meta := program.ChartMeta{Title: "Benchmark", Context: "benchmark", Units: "units"}

	for chartIndex := range chartCount {
		chartID := fmt.Sprintf("chart_%04d", chartIndex)
		templateID := "template_disabled"
		lifecycle := program.LifecyclePolicy{}
		if chartIndex < chartCaps {
			templateID = "template_chart_cap"
			lifecycle.MaxInstances = chartCaps
		}
		if chartIndex < dimCaps {
			lifecycle.Dimensions.MaxDims = dimCount
		}

		entries := make(map[string]*dimBuildEntry, dimCount)
		for dimIndex := range dimCount {
			name := fmt.Sprintf("dimension_%02d", dimIndex)
			entries[name] = observedDimensionEntry(currentSeq)
		}
		chartsByID[chartID] = &chartState{
			templateID:      templateID,
			chartID:         chartID,
			meta:            meta,
			lifecycle:       lifecycle,
			entries:         entries,
			observedCount:   dimCount,
			currentBuildSeq: currentSeq,
		}

		matChart, _ := state.ensureChart(chartID, templateID, meta, lifecycle)
		matChart.lastSeenSuccessSeq = currentSeq
		for dimIndex := range dimCount {
			name := fmt.Sprintf("dimension_%02d", dimIndex)
			dim, _ := matChart.ensureDimension(name, dimensionState{
				algorithm:  program.AlgorithmAbsolute,
				multiplier: 1,
				divisor:    1,
			})
			dim.lastSeenSuccessSeq = currentSeq
		}
	}

	return chartsByID, state
}
