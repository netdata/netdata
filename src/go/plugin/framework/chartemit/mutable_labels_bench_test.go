// SPDX-License-Identifier: GPL-3.0-or-later

package chartemit

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

var benchmarkChartLabelWireBytes int64

type benchmarkByteCounter struct {
	bytes int64
}

func (w *benchmarkByteCounter) Write(p []byte) (int, error) {
	w.bytes += int64(len(p))
	return len(p), nil
}

func BenchmarkApplyPlanChartCreateLabels(b *testing.B) {
	for _, chartCount := range []int{1, 100, 1000} {
		for _, labelCount := range []int{0, 4, 16, 64} {
			b.Run(fmt.Sprintf("charts_%d/labels_%d", chartCount, labelCount), func(b *testing.B) {
				plan := benchmarkChartLabelPlan(chartCount, labelCount)
				writer := &benchmarkByteCounter{}
				api := netdataapi.New(writer)
				env := EmitEnv{
					TypeID:      "benchmark.job",
					UpdateEvery: 1,
					Plugin:      "go.d.plugin",
					Module:      "benchmark",
					JobName:     "benchmark",
					JobLabels:   map[string]string{"configured": "label"},
				}
				if err := ApplyPlan(api, plan, env); err != nil {
					b.Fatalf("warm label plan: %v", err)
				}
				warmBytes := writer.bytes
				if warmBytes == 0 {
					b.Fatal("warm label plan emitted no bytes")
				}
				b.SetBytes(warmBytes)

				b.ReportAllocs()
				b.ResetTimer()
				b.ReportMetric(float64(chartCount), "charts/op")
				b.ReportMetric(float64(chartCount*labelCount), "promoted_labels/op")
				for range b.N {
					writer.bytes = 0
					if err := ApplyPlan(api, plan, env); err != nil {
						b.Fatalf("apply label plan: %v", err)
					}
					benchmarkChartLabelWireBytes = writer.bytes
				}
				b.StopTimer()
				if writer.bytes != warmBytes {
					b.Fatalf("wire bytes = %d, want %d", writer.bytes, warmBytes)
				}
			})
		}
	}
}

func BenchmarkApplyPlanChartValueUpdates(b *testing.B) {
	for _, chartCount := range []int{1, 100, 1000} {
		b.Run(fmt.Sprintf("charts_%d", chartCount), func(b *testing.B) {
			plan := benchmarkChartValuePlan(chartCount)
			writer := &benchmarkByteCounter{}
			api := netdataapi.New(writer)
			env := EmitEnv{TypeID: "benchmark.job", UpdateEvery: 1}
			if err := ApplyPlan(api, plan, env); err != nil {
				b.Fatalf("warm value plan: %v", err)
			}
			warmBytes := writer.bytes
			if warmBytes == 0 {
				b.Fatal("warm value plan emitted no bytes")
			}
			b.SetBytes(warmBytes)

			b.ReportAllocs()
			b.ResetTimer()
			b.ReportMetric(float64(chartCount), "charts/op")
			for range b.N {
				writer.bytes = 0
				if err := ApplyPlan(api, plan, env); err != nil {
					b.Fatalf("apply value plan: %v", err)
				}
				benchmarkChartLabelWireBytes = writer.bytes
			}
			b.StopTimer()
			if writer.bytes != warmBytes {
				b.Fatalf("wire bytes = %d, want %d", writer.bytes, warmBytes)
			}
		})
	}
}

func benchmarkChartLabelPlan(chartCount, labelCount int) Plan {
	actions := make([]EngineAction, 0, chartCount)
	for chart := range chartCount {
		labels := make(map[string]string, labelCount)
		for label := range labelCount {
			labels[fmt.Sprintf("label_%02d", label)] = "value_" + strconv.Itoa(label)
		}
		actions = append(actions, chartengine.CreateChartAction{
			ChartTemplateID: "benchmark",
			ChartID:         "chart_" + strconv.Itoa(chart),
			Meta: chartengine.ChartMeta{
				Title:   "Benchmark",
				Family:  "Benchmark",
				Context: "benchmark.chart",
				Units:   "units",
				Type:    chartengine.ChartTypeLine,
			},
			Labels: labels,
		})
	}
	return Plan{Actions: actions}
}

func benchmarkChartValuePlan(chartCount int) Plan {
	actions := make([]EngineAction, 0, chartCount)
	for chart := range chartCount {
		actions = append(actions, chartengine.UpdateChartAction{
			ChartID: "chart_" + strconv.Itoa(chart),
			Values: []chartengine.UpdateDimensionValue{{
				Name:  "value",
				Int64: int64(chart),
			}},
		})
	}
	return Plan{Actions: actions}
}
