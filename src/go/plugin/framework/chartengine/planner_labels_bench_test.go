// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

var benchmarkChartLabelAccumulatorSink *chartLabelAccumulator

func BenchmarkChartLabelAccumulatorObserveEmpty(b *testing.B) {
	tests := map[string]struct {
		chart            program.Chart
		populated        map[string]string
		wantSelected     int
		wantInstanceKeys int
	}{
		"automatic_1": {
			chart: program.Chart{
				Labels: program.LabelPolicy{Mode: program.PromotionModeAutoIntersection},
			},
			populated:    benchmarkLabelValues(1),
			wantSelected: 1,
		},
		"automatic_64": {
			chart: program.Chart{
				Labels: program.LabelPolicy{Mode: program.PromotionModeAutoIntersection},
			},
			populated:    benchmarkLabelValues(64),
			wantSelected: 64,
		},
		"explicit_64": {
			chart: program.Chart{
				Labels: program.LabelPolicy{
					Mode:        program.PromotionModeExplicitIntersection,
					PromoteKeys: benchmarkPromotedLabelKeys(64),
				},
			},
			populated:    benchmarkLabelValues(64),
			wantSelected: 64,
		},
		"wildcard_identity": {
			chart: program.Chart{
				Identity: program.ChartIdentity{
					InstanceByLabels: []program.InstanceLabelSelector{{IncludeAll: true}},
				},
				Labels: program.LabelPolicy{Mode: program.PromotionModeAutoIntersection},
			},
			populated:        benchmarkEmptyLabelValues(64),
			wantInstanceKeys: 64,
		},
	}
	empty := sortedLabelView(nil)

	for name, tc := range tests {
		b.Run(name, func(b *testing.B) {
			policy := compileChartLabelPolicy(tc.chart)
			populated := sortedLabelView(tc.populated)
			probe := newChartLabelAccumulator(policy)
			if err := probe.observe(populated, ""); err != nil {
				b.Fatal(err)
			}
			if len(probe.selected) != tc.wantSelected {
				b.Fatalf("prepopulate selected: got %d entries, want %d", len(probe.selected), tc.wantSelected)
			}
			if len(probe.instanceKeys) != tc.wantInstanceKeys {
				b.Fatalf("prepopulate instance keys: got %d entries, want %d", len(probe.instanceKeys), tc.wantInstanceKeys)
			}

			acc := newChartLabelAccumulator(policy)
			b.ReportAllocs()
			for b.Loop() {
				acc.reset()
				if err := acc.observe(populated, ""); err != nil {
					b.Fatal(err)
				}
				if err := acc.observe(empty, ""); err != nil {
					b.Fatal(err)
				}
			}
			benchmarkChartLabelAccumulatorSink = acc
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

func benchmarkEmptyLabelValues(count int) map[string]string {
	labels := make(map[string]string, count)
	for i := range count {
		labels[fmt.Sprintf("label_%02d", i)] = ""
	}
	return labels
}
