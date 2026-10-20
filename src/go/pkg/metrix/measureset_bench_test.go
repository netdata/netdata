// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"math"
	"testing"
)

// Writes validate and copy O(declared fields), independent of retained series.
// Named writes also construct one ordered vector; positional writes need no mapping.
func BenchmarkMeasureSetSnapshotGaugeWrite(b *testing.B) {
	for _, named := range []bool{false, true} {
		name := "positional"
		if named {
			name = "named"
		}
		b.Run(name, func(b *testing.B) {
			for _, unavailable := range []bool{false, true} {
				name := "finite"
				values := []SampleValue{-1, 9.5, 2.5, 2, 8}
				if unavailable {
					name = "unavailable"
					values[4] = math.NaN()
				}
				b.Run(name, func(b *testing.B) {
					s := NewCollectorStore()
					cc := benchmarkCycleController(b, s)
					g := s.Write().SnapshotMeter("svc").MeasureSetGauge("values", WithMeasureSetFields(
						MeasureFieldSpec{
							Name: "min",
						}, MeasureFieldSpec{
							Name: "max",
						}, MeasureFieldSpec{
							Name: "mean",
						},
						MeasureFieldSpec{
							Name: "p50",
						}, MeasureFieldSpec{
							Name: "p95",
						},
					))
					fields := map[string]SampleValue{
						"min":  values[0],
						"max":  values[1],
						"mean": values[2],
						"p50":  values[3],
						"p95":  values[4],
					}
					cc.BeginCycle()
					defer cc.AbortCycle()
					b.ReportAllocs()
					for b.Loop() {
						if named {
							g.ObserveFields(fields)
						} else {
							g.ObservePoint(MeasureSetPoint{
								Values: values,
							})
						}
					}
				})
			}
		})
	}
}
