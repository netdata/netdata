// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine/internal/program"
)

func BenchmarkChartLabelAccumulatorObserveEmpty(b *testing.B) {
	tests := map[string]program.LabelPolicy{
		"automatic": {
			Mode: program.PromotionModeAutoIntersection,
		},
		"explicit_64": {
			Mode:        program.PromotionModeExplicitIntersection,
			PromoteKeys: benchmarkPromotedLabelKeys(64),
		},
	}
	empty := sortedLabelView(nil)

	for name, labels := range tests {
		b.Run(name, func(b *testing.B) {
			policy := compileChartLabelPolicy(program.Chart{Labels: labels})

			b.Run("steady", func(b *testing.B) {
				acc := newChartLabelAccumulator(policy)
				if err := acc.observe(empty, ""); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				for b.Loop() {
					if err := acc.observe(empty, ""); err != nil {
						b.Fatal(err)
					}
				}
			})

			b.Run("after_reset", func(b *testing.B) {
				acc := newChartLabelAccumulator(policy)
				b.ReportAllocs()
				for b.Loop() {
					acc.reset()
					if err := acc.observe(empty, ""); err != nil {
						b.Fatal(err)
					}
				}
			})
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
