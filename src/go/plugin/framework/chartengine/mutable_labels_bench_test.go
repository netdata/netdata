// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

var benchmarkMutableLabelPlanSink Plan

func BenchmarkBuildPlanMutableLabels(b *testing.B) {
	for _, chartCount := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("unchanged_chart_scaling/charts_%d/labels_4", chartCount), func(b *testing.B) {
			reader := newBenchmarkMutableLabelReader(b, chartCount, 1, 4, nil)
			benchmarkMutableLabelPlanning(b, chartCount, 1, 4, 0, reader)
		})

		b.Run(fmt.Sprintf("one_changed_unrelated_series_scaling/charts_%d/labels_4", chartCount), func(b *testing.B) {
			base := newBenchmarkMutableLabelReader(b, chartCount, 1, 4, nil)
			changed := newBenchmarkMutableLabelReader(b, chartCount, 1, 4, func(chart int) bool { return chart == 0 })
			benchmarkMutableLabelPlanning(b, chartCount, 1, 4, 1, base, changed)
		})
	}

	for _, seriesCount := range []int{100, 1000, 10000} {
		chartCount := seriesCount / 2
		b.Run(fmt.Sprintf("unchanged_later_membership_shape/series_%d/charts_%d/dims_2/labels_4", seriesCount, chartCount), func(b *testing.B) {
			reader := newBenchmarkMutableLabelReader(b, chartCount, 2, 4, nil)
			benchmarkMutableLabelPlanning(b, chartCount, 2, 4, 0, reader)
		})

		b.Run(fmt.Sprintf("later_membership_changed_unrelated_series_scaling/series_%d/charts_%d/dims_2/labels_4", seriesCount, chartCount), func(b *testing.B) {
			base := newBenchmarkMutableLabelReaderWithSeriesMembershipChange(b, chartCount, 2, 4, func(int, int) bool { return false })
			changed := newBenchmarkMutableLabelReaderWithSeriesMembershipChange(b, chartCount, 2, 4, func(chart, dimension int) bool {
				return chart == 0 && dimension == 1
			})
			benchmarkMutableLabelPlanning(b, chartCount, 2, 4, 0, base, changed)
		})
	}

	for _, labelCount := range []int{0, 4, 16, 64} {
		b.Run(fmt.Sprintf("unchanged_label_scaling/charts_100/dims_10/labels_%d", labelCount), func(b *testing.B) {
			reader := newBenchmarkMutableLabelReader(b, 100, 10, labelCount, nil)
			benchmarkMutableLabelPlanning(b, 100, 10, labelCount, 0, reader)
		})
	}

	for _, shape := range []struct {
		charts int
		dims   int
	}{
		{charts: 1000, dims: 1},
		{charts: 100, dims: 10},
		{charts: 1, dims: 1000},
	} {
		b.Run(fmt.Sprintf("unchanged_shape/charts_%d/dims_%d/labels_4", shape.charts, shape.dims), func(b *testing.B) {
			reader := newBenchmarkMutableLabelReader(b, shape.charts, shape.dims, 4, nil)
			benchmarkMutableLabelPlanning(b, shape.charts, shape.dims, 4, 0, reader)
		})
	}

	for _, labelCount := range []int{4, 16, 64} {
		b.Run(fmt.Sprintf("one_changed/charts_1000/dims_10/labels_%d", labelCount), func(b *testing.B) {
			base := newBenchmarkMutableLabelReader(b, 1000, 10, labelCount, nil)
			changed := newBenchmarkMutableLabelReader(b, 1000, 10, labelCount, func(chart int) bool { return chart == 0 })
			benchmarkMutableLabelPlanning(b, 1000, 10, labelCount, 1, base, changed)
		})

		b.Run(fmt.Sprintf("all_changed/charts_1000/dims_10/labels_%d", labelCount), func(b *testing.B) {
			base := newBenchmarkMutableLabelReader(b, 1000, 10, labelCount, nil)
			changed := newBenchmarkMutableLabelReader(b, 1000, 10, labelCount, func(int) bool { return true })
			benchmarkMutableLabelPlanning(b, 1000, 10, labelCount, 1000, base, changed)
		})

		b.Run(fmt.Sprintf("membership_changed_effective_labels_unchanged/charts_1000/dims_10/labels_%d", labelCount), func(b *testing.B) {
			base := newBenchmarkMutableLabelReaderWithMembershipChange(b, 1000, 10, labelCount, func(int) bool { return false })
			changed := newBenchmarkMutableLabelReaderWithMembershipChange(b, 1000, 10, labelCount, func(chart int) bool { return chart == 0 })
			benchmarkMutableLabelPlanning(b, 1000, 10, labelCount, 0, base, changed)
		})
	}
}

func BenchmarkBuildPlanMutableLabelsCreate(b *testing.B) {
	for _, chartCount := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("charts_%d/labels_4", chartCount), func(b *testing.B) {
			reader := newBenchmarkMutableLabelReader(b, chartCount, 1, 4, nil)
			template := benchmarkMutableLabelTemplate(4)
			engine, err := New(WithRuntimeStore(nil))
			if err != nil {
				b.Fatalf("new engine: %v", err)
			}
			if err := engine.LoadYAML(template, 1); err != nil {
				b.Fatalf("load template: %v", err)
			}
			if _, err := buildPlan(engine, reader); err != nil {
				b.Fatalf("warm routes: %v", err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			b.ReportMetric(float64(chartCount), "charts/op")
			b.ReportMetric(float64(chartCount), "dims/op")
			b.ReportMetric(float64(chartCount*4), "promoted_label_entries/op")
			b.ReportMetric(float64(chartCount), "series/op")
			for i := range b.N {
				b.StopTimer()
				engine.ResetMaterialized()
				reader.seq = uint64(i) + 2
				b.StartTimer()

				plan, err := buildPlan(engine, reader)
				if err != nil {
					b.Fatalf("build creation plan: %v", err)
				}
				benchmarkMutableLabelPlanSink = plan
			}
			b.StopTimer()
			benchmarkAssertActionMix(b, benchmarkMutableLabelPlanSink, chartCount, chartCount, 0, chartCount)
		})
	}
}

func benchmarkMutableLabelPlanning(b *testing.B, chartCount, dimsPerChart, labelCount, expectedLabelUpdates int, readers ...*benchmarkSequenceReader) {
	b.Helper()
	if len(readers) == 0 {
		b.Fatal("at least one reader is required")
	}

	engine, err := New(WithRuntimeStore(nil))
	if err != nil {
		b.Fatalf("new engine: %v", err)
	}
	if err := engine.LoadYAML(benchmarkMutableLabelTemplate(labelCount), 1); err != nil {
		b.Fatalf("load template: %v", err)
	}
	initial, err := buildPlan(engine, readers[0])
	if err != nil {
		b.Fatalf("materialize charts: %v", err)
	}
	benchmarkAssertActionMix(b, initial, chartCount, chartCount*dimsPerChart, 0, chartCount)
	for _, reader := range readers {
		reader.raw.ForEachSeriesIdentityRaw(func(metrix.SeriesIdentity, metrix.SeriesMeta, string, []metrix.Label, metrix.SampleValue) {})
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.ReportMetric(float64(chartCount), "charts/op")
	b.ReportMetric(float64(chartCount*dimsPerChart), "dims/op")
	b.ReportMetric(float64(chartCount*dimsPerChart*labelCount), "promoted_label_entries/op")
	b.ReportMetric(float64(chartCount*dimsPerChart), "series/op")
	for i := range b.N {
		reader := readers[0]
		if len(readers) > 1 && i%2 == 0 {
			reader = readers[1+(i/2)%(len(readers)-1)]
		}
		reader.seq = uint64(i) + 2
		plan, err := buildPlan(engine, reader)
		if err != nil {
			b.Fatalf("build steady-state plan: %v", err)
		}
		benchmarkMutableLabelPlanSink = plan
	}
	b.StopTimer()
	benchmarkAssertActionMix(b, benchmarkMutableLabelPlanSink, 0, 0, expectedLabelUpdates, chartCount)
}

type benchmarkSequenceReader struct {
	metrix.Reader
	raw metrix.SeriesIdentityRawIterator
	seq uint64
}

func (r *benchmarkSequenceReader) CollectMeta() metrix.CollectMeta {
	meta := r.Reader.CollectMeta()
	meta.LastAttemptSeq = r.seq
	meta.LastAttemptStatus = metrix.CollectStatusSuccess
	meta.LastSuccessSeq = r.seq
	return meta
}

func (r *benchmarkSequenceReader) ForEachSeriesIdentity(fn func(metrix.SeriesIdentity, metrix.SeriesMeta, string, metrix.LabelView, metrix.SampleValue)) {
	r.Reader.ForEachSeriesIdentity(func(identity metrix.SeriesIdentity, meta metrix.SeriesMeta, name string, labels metrix.LabelView, value metrix.SampleValue) {
		meta.LastSeenSuccessSeq = r.seq
		fn(identity, meta, name, labels, value)
	})
}

func (r *benchmarkSequenceReader) ForEachSeriesIdentityRaw(fn func(metrix.SeriesIdentity, metrix.SeriesMeta, string, []metrix.Label, metrix.SampleValue)) {
	r.raw.ForEachSeriesIdentityRaw(func(identity metrix.SeriesIdentity, meta metrix.SeriesMeta, name string, labels []metrix.Label, value metrix.SampleValue) {
		meta.LastSeenSuccessSeq = r.seq
		fn(identity, meta, name, labels, value)
	})
}

func (r *benchmarkSequenceReader) FlattenedRead() bool {
	aware, ok := r.Reader.(interface{ FlattenedRead() bool })
	return ok && aware.FlattenedRead()
}

func newBenchmarkMutableLabelReader(b *testing.B, chartCount, dimsPerChart, labelCount int, changed func(int) bool) *benchmarkSequenceReader {
	var promotedChanged func(int, int) bool
	if changed != nil {
		promotedChanged = func(chart, _ int) bool { return changed(chart) }
	}
	return newBenchmarkMutableLabelReaderWithMutations(b, chartCount, dimsPerChart, labelCount, promotedChanged, nil)
}

func newBenchmarkMutableLabelReaderWithMembershipChange(
	b *testing.B,
	chartCount,
	dimsPerChart,
	labelCount int,
	changed func(int) bool,
) *benchmarkSequenceReader {
	var membershipChanged func(int, int) bool
	if changed != nil {
		membershipChanged = func(chart, _ int) bool { return changed(chart) }
	}
	return newBenchmarkMutableLabelReaderWithMutations(b, chartCount, dimsPerChart, labelCount, nil, membershipChanged)
}

func newBenchmarkMutableLabelReaderWithSeriesMembershipChange(
	b *testing.B,
	chartCount,
	dimsPerChart,
	labelCount int,
	changed func(chart, dimension int) bool,
) *benchmarkSequenceReader {
	return newBenchmarkMutableLabelReaderWithMutations(b, chartCount, dimsPerChart, labelCount, nil, changed)
}

func newBenchmarkMutableLabelReaderWithMutations(
	b *testing.B,
	chartCount,
	dimsPerChart,
	labelCount int,
	promotedChanged func(int, int) bool,
	membershipChanged func(int, int) bool,
) *benchmarkSequenceReader {
	b.Helper()
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		b.Fatal("collector store is not cycle-managed")
	}
	cc := managed.CycleController()
	meter := store.Write().SnapshotMeter("")
	gauge := meter.Gauge("bench_metric")

	cc.BeginCycle()
	for chart := range chartCount {
		for dimension := range dimsPerChart {
			labels := make([]metrix.Label, 0, labelCount+2)
			labels = append(labels,
				metrix.Label{Key: "chart_id", Value: strconv.Itoa(chart)},
				metrix.Label{Key: "dim_id", Value: strconv.Itoa(dimension)},
			)
			if membershipChanged != nil {
				value := "base"
				if membershipChanged(chart, dimension) {
					value = "next"
				}
				labels = append(labels, metrix.Label{Key: "membership_marker", Value: value})
			}
			for label := range labelCount {
				value := "base_" + strconv.Itoa(label)
				if promotedChanged != nil && promotedChanged(chart, dimension) {
					value = "next_" + strconv.Itoa(label)
				}
				labels = append(labels, metrix.Label{Key: fmt.Sprintf("label_%02d", label), Value: value})
			}
			gauge.Observe(metrix.SampleValue(chart*dimsPerChart+dimension), meter.LabelSet(labels...))
		}
	}
	if err := cc.CommitCycleSuccess(); err != nil {
		b.Fatalf("commit benchmark cycle: %v", err)
	}
	reader := store.Read(metrix.ReadRaw(), metrix.ReadFlatten())
	raw, ok := reader.(metrix.SeriesIdentityRawIterator)
	if !ok {
		b.Fatal("benchmark reader does not support raw identity iteration")
	}
	return &benchmarkSequenceReader{Reader: reader, raw: raw, seq: 1}
}

func benchmarkMutableLabelTemplate(labelCount int) []byte {
	var sb strings.Builder
	sb.WriteString("version: v1\n" +
		"groups:\n" +
		"  - family: Bench\n" +
		"    metrics: [bench_metric]\n" +
		"    charts:\n" +
		"      - id: bench\n" +
		"        title: Bench metric\n" +
		"        context: bench.metric\n" +
		"        units: units\n" +
		"        instances:\n" +
		"          by_labels: [chart_id]\n")
	if labelCount > 0 {
		sb.WriteString("        label_promotion:\n")
		for label := range labelCount {
			fmt.Fprintf(&sb, "          - label_%02d\n", label)
		}
	}
	sb.WriteString("        dimensions:\n" +
		"          - selector: bench_metric\n" +
		"            name_from_label: dim_id\n")
	return []byte(sb.String())
}

func benchmarkActionKindCount(plan Plan, kind ActionKind) int {
	var count int
	for _, action := range plan.Actions {
		if action.Kind() == kind {
			count++
		}
	}
	return count
}

func benchmarkAssertActionMix(b *testing.B, plan Plan, createCharts, createDimensions, updateChartLabels, updateCharts int) {
	b.Helper()
	wantTotal := createCharts + createDimensions + updateChartLabels + updateCharts
	if len(plan.Actions) != wantTotal {
		b.Fatalf("actions = %d, want %d", len(plan.Actions), wantTotal)
	}
	expected := map[ActionKind]int{
		ActionCreateChart:       createCharts,
		ActionCreateDimension:   createDimensions,
		ActionUpdateChartLabels: updateChartLabels,
		ActionUpdateChart:       updateCharts,
		ActionRemoveDimension:   0,
		ActionRemoveChart:       0,
	}
	for kind, want := range expected {
		if got := benchmarkActionKindCount(plan, kind); got != want {
			b.Fatalf("action kind %d count = %d, want %d", kind, got, want)
		}
	}
}
