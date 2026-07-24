// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

var (
	benchmarkExcludeMatchSink bool
	benchmarkExcludeRouteSink []routeBinding
)

func BenchmarkAutogenExcludeMatch(b *testing.B) {
	for _, length := range []int{16, 128} {
		for _, count := range []int{1, 32, 256} {
			for _, scenario := range []string{"exact_early", "prefix_early", "general_late", "miss"} {
				b.Run(fmt.Sprintf("patterns_%d/length_%d/%s", count, length, scenario), func(b *testing.B) {
					patterns, source := benchmarkExcludeMatchCase(count, length, scenario)
					compiled, err := matcher.CompilePositivePatternList(patterns)
					if err != nil {
						b.Fatalf("compile patterns: %v", err)
					}

					b.ReportAllocs()
					b.ReportMetric(float64(len(compiled.Patterns())), "patterns/op")
					b.ResetTimer()
					for range b.N {
						benchmarkExcludeMatchSink = compiled.MatchString(source)
					}
				})
			}
		}
	}
}

func BenchmarkCompileAutogenExclude(b *testing.B) {
	for _, length := range []int{16, 128} {
		for _, count := range []int{1, 32, 256} {
			b.Run(fmt.Sprintf("patterns_%d/length_%d", count, length), func(b *testing.B) {
				patterns, _ := benchmarkExcludeMatchCase(count, length, "miss")
				b.ReportAllocs()
				b.ReportMetric(float64(count), "patterns/op")
				b.ResetTimer()
				for range b.N {
					compiled, err := matcher.CompilePositivePatternList(patterns)
					if err != nil {
						b.Fatalf("compile patterns: %v", err)
					}
					benchmarkExcludeMatchSink = compiled.MatchString("never_matches")
				}
			})
		}
	}
}

func BenchmarkResolveAutogenRouteExcludeStructuredKinds(b *testing.B) {
	cases := map[string]struct {
		metricName string
		labels     map[string]string
		meta       metrix.SeriesMeta
		familyName string
	}{
		"histogram_bucket": {
			metricName: "svc.latency_seconds_bucket",
			labels:     map[string]string{metrix.HistogramBucketLabel: "1"},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindHistogram,
				FlattenRole: metrix.FlattenRoleHistogramBucket,
			},
			familyName: "svc.latency_seconds",
		},
		"histogram_count": {
			metricName: "svc.latency_seconds_count",
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindHistogram,
				FlattenRole: metrix.FlattenRoleHistogramCount,
			},
			familyName: "svc.latency_seconds",
		},
		"histogram_sum": {
			metricName: "svc.latency_seconds_sum",
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindHistogram,
				FlattenRole: metrix.FlattenRoleHistogramSum,
			},
			familyName: "svc.latency_seconds",
		},
		"summary_quantile": {
			metricName: "svc.latency_seconds",
			labels:     map[string]string{metrix.SummaryQuantileLabel: "0.9"},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindGauge,
				SourceKind:  metrix.MetricKindSummary,
				FlattenRole: metrix.FlattenRoleSummaryQuantile,
			},
			familyName: "svc.latency_seconds",
		},
		"summary_count": {
			metricName: "svc.latency_seconds_count",
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindSummary,
				FlattenRole: metrix.FlattenRoleSummaryCount,
			},
			familyName: "svc.latency_seconds",
		},
		"summary_sum": {
			metricName: "svc.latency_seconds_sum",
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindCounter,
				SourceKind:  metrix.MetricKindSummary,
				FlattenRole: metrix.FlattenRoleSummarySum,
			},
			familyName: "svc.latency_seconds",
		},
		"stateset": {
			metricName: "svc.status",
			labels:     map[string]string{"svc.status": "ok"},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindGauge,
				SourceKind:  metrix.MetricKindStateSet,
				FlattenRole: metrix.FlattenRoleStateSetState,
			},
			familyName: "svc.status",
		},
		"measureset": {
			metricName: "svc.usage_used",
			labels:     map[string]string{metrix.MeasureSetFieldLabel: "used"},
			meta: metrix.SeriesMeta{
				Kind:        metrix.MetricKindGauge,
				SourceKind:  metrix.MetricKindMeasureSet,
				FlattenRole: metrix.FlattenRoleMeasureSetField,
			},
			familyName: "svc.usage",
		},
	}

	for name, test := range cases {
		for _, scenario := range []string{"hit", "miss"} {
			b.Run(name+"/"+scenario, func(b *testing.B) {
				pattern := test.familyName
				if scenario == "miss" {
					pattern = "other_*"
				}
				exclude, err := matcher.CompilePositivePatternList([]string{pattern})
				if err != nil {
					b.Fatalf("compile patterns: %v", err)
				}
				engine := &Engine{state: engineState{cfg: engineConfig{
					autogen:        AutogenPolicy{Enabled: true, MaxTypeIDLen: defaultMaxTypeIDLen},
					autogenExclude: exclude,
				}}}
				labels := sortedLabelView(test.labels)

				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					routes, _, err := engine.resolveAutogenRoute(nil, test.metricName, labels, test.meta)
					if err != nil {
						b.Fatalf("resolve route: %v", err)
					}
					benchmarkExcludeRouteSink = routes
				}
			})
		}
	}
}

func BenchmarkBuildPlanAutogenExcludeForcedReplay(b *testing.B) {
	for _, length := range []int{16, 128} {
		target := fixedBenchmarkToken("z_excluded", 0, length)
		for _, count := range []int{1, 32, 256} {
			b.Run(fmt.Sprintf("patterns_%d/length_%d", count, length), func(b *testing.B) {
				patterns := lateHitBenchmarkPatterns(count, length, target)
				engine, err := New(
					WithRuntimeStore(nil),
					WithEnginePolicy(EnginePolicy{Autogen: &AutogenPolicy{
						Enabled: true,
						Exclude: patterns,
					}}),
				)
				if err != nil {
					b.Fatalf("new engine: %v", err)
				}
				if err := engine.LoadYAML([]byte(mutableLabelsTestTemplate), 1); err != nil {
					b.Fatalf("load template: %v", err)
				}

				base := newAutogenExcludeReplayReader(b, target, "shard-b")
				changed := newAutogenExcludeReplayReader(b, target, "shard-z")
				if _, err := buildPlan(engine, base); err != nil {
					b.Fatalf("materialize initial plan: %v", err)
				}

				changed.seq = 2
				probe := &benchmarkCountingReplayReader{benchmarkSequenceReader: changed}
				if _, err := buildPlan(engine, probe); err != nil {
					b.Fatalf("probe forced replay: %v", err)
				}
				if probe.iterations != 2 {
					b.Fatalf("forced replay fixture iterated %d times, want 2", probe.iterations)
				}
				base.seq = 3
				if _, err := buildPlan(engine, base); err != nil {
					b.Fatalf("reset replay fixture: %v", err)
				}

				b.ReportAllocs()
				b.ReportMetric(float64(count), "patterns/op")
				b.ReportMetric(130, "series/pass")
				b.ReportMetric(2, "passes/op")
				b.ResetTimer()
				for i := range b.N {
					reader := changed
					if i%2 == 1 {
						reader = base
					}
					reader.seq = uint64(i) + 4
					plan, err := buildPlan(engine, reader)
					if err != nil {
						b.Fatalf("build plan: %v", err)
					}
					benchmarkMutableLabelPlanSink = plan
				}
			})
		}
	}
}

type benchmarkCountingReplayReader struct {
	*benchmarkSequenceReader
	iterations int
}

func (r *benchmarkCountingReplayReader) ForEachSeriesIdentityRaw(
	fn func(metrix.SeriesIdentity, metrix.SeriesMeta, string, []metrix.Label, metrix.SampleValue),
) {
	r.iterations++
	r.raw.ForEachSeriesIdentityRaw(func(
		identity metrix.SeriesIdentity,
		meta metrix.SeriesMeta,
		name string,
		labels []metrix.Label,
		value metrix.SampleValue,
	) {
		meta.LastSeenSuccessSeq = r.seq
		fn(identity, meta, name, labels, value)
	})
}

func newAutogenExcludeReplayReader(b *testing.B, excludedMetric, secondShard string) *benchmarkSequenceReader {
	b.Helper()

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		b.Fatal("collector store is not cycle-managed")
	}
	cc := managed.CycleController()
	meter := store.Write().SnapshotMeter("")
	authored := meter.Gauge("service_value")
	excluded := meter.Gauge(excludedMetric)

	cc.BeginCycle()
	for _, shard := range []string{"shard-a", secondShard} {
		authored.Observe(1, meter.LabelSet(
			metrix.Label{Key: "instance", Value: "node-1"},
			metrix.Label{Key: "owner", Value: "owner-a"},
			metrix.Label{Key: "zone", Value: "zone-a"},
			metrix.Label{Key: "shard", Value: shard},
		))
	}
	for i := range 128 {
		excluded.Observe(metrix.SampleValue(i), meter.LabelSet(
			metrix.Label{Key: "series", Value: strconv.Itoa(i)},
		))
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

func benchmarkExcludeMatchCase(count, length int, scenario string) ([]string, string) {
	switch scenario {
	case "exact_early":
		source := fixedBenchmarkToken("a_target", 0, length)
		patterns := []string{source}
		for i := 1; i < count; i++ {
			patterns = append(patterns, fixedBenchmarkToken("z_miss", i, length))
		}
		return patterns, source
	case "prefix_early":
		source := fixedBenchmarkToken("a_target", 0, length)
		patterns := []string{source[:len(source)-1] + "*"}
		for i := 1; i < count; i++ {
			patterns = append(patterns, fixedBenchmarkToken("z_miss", i, length))
		}
		return patterns, source
	case "general_late":
		source := fixedBenchmarkToken("z_target", 0, length)
		patterns := make([]string, 0, count)
		for i := 0; i < count-1; i++ {
			patterns = append(patterns, fixedBenchmarkToken("a_miss", i, length))
		}
		general := []byte(source)
		general[len(general)/2] = '?'
		patterns = append(patterns, string(general))
		return patterns, source
	default:
		source := fixedBenchmarkToken("z_target", 0, length)
		patterns := make([]string, 0, count)
		for i := range count {
			patterns = append(patterns, fixedBenchmarkToken("a_miss", i, length))
		}
		return patterns, source
	}
}

func lateHitBenchmarkPatterns(count, length int, target string) []string {
	patterns := make([]string, 0, count)
	for i := 0; i < count-1; i++ {
		patterns = append(patterns, fixedBenchmarkToken("a_miss", i, length))
	}
	return append(patterns, target)
}

func fixedBenchmarkToken(prefix string, index, length int) string {
	value := fmt.Sprintf("%s_%03d", prefix, index)
	if len(value) >= length {
		return value[:length]
	}
	return value + strings.Repeat("x", length-len(value))
}
