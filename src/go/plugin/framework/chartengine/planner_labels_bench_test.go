// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

var benchmarkPromotedIntersectionEmptySink bool

func BenchmarkChartLabelAccumulatorObserveEmpty(b *testing.B) {
	tests := map[string]struct {
		chart     program.Chart
		populated map[string]string
	}{
		"automatic_1": {
			chart: program.Chart{
				Labels: program.LabelPolicy{Mode: program.PromotionModeAutoIntersection},
			},
			populated: benchmarkLabelValues(1),
		},
		"automatic_64": {
			chart: program.Chart{
				Labels: program.LabelPolicy{Mode: program.PromotionModeAutoIntersection},
			},
			populated: benchmarkLabelValues(64),
		},
		"explicit_64": {
			chart: program.Chart{
				Labels: program.LabelPolicy{
					Mode:        program.PromotionModeExplicitIntersection,
					PromoteKeys: benchmarkPromotedLabelKeys(64),
				},
			},
			populated: benchmarkLabelValues(64),
		},
		"wildcard_identity": {
			chart: program.Chart{
				Identity: program.ChartIdentity{
					InstanceByLabels: []program.InstanceLabelSelector{{IncludeAll: true}},
				},
				Labels: program.LabelPolicy{Mode: program.PromotionModeAutoIntersection},
			},
		},
	}
	empty := sortedLabelView(nil)

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			acc := newChartLabelAccumulator(compileChartLabelPolicy(tc.chart))
			if len(tc.populated) > 0 {
				if err := acc.observe(sortedLabelView(tc.populated), ""); err != nil {
					b.Fatal(err)
				}
				if len(acc.selected) != len(tc.populated) {
					b.Fatalf("prepopulate selected: got %d entries, want %d", len(acc.selected), len(tc.populated))
				}
			}

			b.ReportAllocs()
			for b.Loop() {
				// Re-arm only the semantic witness so the benchmark isolates the
				// populated-to-empty observation without rebuilding its map.
				acc.promotedIntersectionEmpty = false
				if err := acc.observe(empty, ""); err != nil {
					b.Fatal(err)
				}
			}
			benchmarkPromotedIntersectionEmptySink = acc.promotedIntersectionEmpty
		})
	}
}

func benchmarkPromotedLabelKeys(count int) []string {
	keys := make([]string, count)
	for i := range keys {
		keys[i] = fmt.Sprintf("label_%02d", i)
	}
	return keys
}

func benchmarkLabelValues(count int) map[string]string {
	labels := make(map[string]string, count)
	for i := range count {
		labels[fmt.Sprintf("label_%02d", i)] = fmt.Sprintf("value_%02d", i)
	}
	return labels
}
