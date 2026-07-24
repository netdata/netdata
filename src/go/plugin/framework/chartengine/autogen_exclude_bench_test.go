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
			for _, scenario := range []string{
				"exact_early",
				"exact_late",
				"prefix_early",
				"prefix_late",
				"general_star_early",
				"general_star_late",
				"general_star_miss",
				"exact_miss",
				"wildcard",
			} {
				b.Run(fmt.Sprintf("patterns_%d/length_%d/%s", count, length, scenario), func(b *testing.B) {
					patterns, source := benchmarkExcludeMatchCase(count, length, scenario)
					compiled, err := matcher.CompilePositivePatternList(patterns)
					if err != nil {
						b.Fatalf("compile patterns: %v", err)
					}

					b.ReportAllocs()
					b.ResetTimer()
					b.ReportMetric(float64(len(compiled.Patterns())), "patterns/op")
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
				b.ResetTimer()
				b.ReportMetric(float64(count), "patterns/op")
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

func BenchmarkEngineLoadYAMLAutogenExclude(b *testing.B) {
	for _, length := range []int{16, 128} {
		for _, count := range []int{1, 32, 256} {
			b.Run(fmt.Sprintf("patterns_%d/length_%d", count, length), func(b *testing.B) {
				patterns, _ := benchmarkExcludeMatchCase(count, length, "exact_miss")
				template := benchmarkAutogenExcludeTemplate(patterns)
				engine, err := New(WithRuntimeStore(nil))
				if err != nil {
					b.Fatalf("new engine: %v", err)
				}

				b.ReportAllocs()
				b.ResetTimer()
				b.ReportMetric(float64(count), "patterns/op")
				for i := range b.N {
					if err := engine.LoadYAML(template, uint64(i+1)); err != nil {
						b.Fatalf("load template: %v", err)
					}
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

func BenchmarkBuildPlanAutogenExcludeEnvelope(b *testing.B) {
	const (
		patternCount  = 256
		patternLength = 128
	)
	for _, shape := range []string{"one_family_many_labels", "distinct_families"} {
		for _, seriesCount := range []int{100, 1000, 10000} {
			for _, scenario := range []string{"late_exact", "general_star_hit", "general_star_miss", "wildcard"} {
				for _, cacheState := range []string{"cold", "warm"} {
					name := fmt.Sprintf(
						"%s/series_%d/%s/%s",
						shape,
						seriesCount,
						scenario,
						cacheState,
					)
					b.Run(name, func(b *testing.B) {
						reader, target := benchmarkAutogenExcludeReader(b, shape, seriesCount, patternLength)
						patterns := benchmarkPlanExcludePatterns(
							scenario,
							patternCount,
							patternLength,
							target,
						)
						engine := benchmarkAutogenExcludeEngine(b, patterns)
						if cacheState == "warm" {
							if _, err := buildPlan(engine, reader); err != nil {
								b.Fatalf("warm plan: %v", err)
							}
						}

						b.ReportAllocs()
						b.ResetTimer()
						b.ReportMetric(patternCount, "input-patterns/op")
						b.ReportMetric(float64(seriesCount), "series/pass")
						for i := range b.N {
							if cacheState == "cold" {
								b.StopTimer()
								if err := engine.LoadYAML(
									[]byte(benchAutogenTemplateYAML),
									uint64(i+2),
								); err != nil {
									b.Fatalf("reload template: %v", err)
								}
								b.StartTimer()
							}
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
	}
}

func BenchmarkBuildPlanAutogenExcludeStructuredKinds(b *testing.B) {
	for _, kind := range []string{"histogram", "summary", "stateset", "measureset"} {
		for _, scenario := range []string{"hit", "miss"} {
			b.Run(kind+"/"+scenario, func(b *testing.B) {
				reader, familyName := benchmarkStructuredAutogenReader(b, kind)
				pattern := familyName
				if scenario == "miss" {
					pattern = "other_*"
				}
				engine := benchmarkAutogenExcludeEngine(b, []string{pattern})
				if _, err := buildPlan(engine, reader); err != nil {
					b.Fatalf("warm plan: %v", err)
				}

				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
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

func BenchmarkBuildPlanAutogenExcludeForcedReplay(b *testing.B) {
	for _, length := range []int{16, 128} {
		target := fixedBenchmarkToken("z_excluded", 0, length)
		for _, count := range []int{1, 32, 256} {
			for _, scenario := range []string{"late_exact", "general_star_miss"} {
				b.Run(fmt.Sprintf("patterns_%d/length_%d/%s", count, length, scenario), func(b *testing.B) {
					patterns := benchmarkPlanExcludePatterns(scenario, count, length, target)
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
					b.ResetTimer()
					b.ReportMetric(float64(count), "patterns/op")
					b.ReportMetric(130, "series/pass")
					b.ReportMetric(2, "passes/op")
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

func benchmarkAutogenExcludeEngine(b *testing.B, patterns []string) *Engine {
	b.Helper()
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
	if err := engine.LoadYAML([]byte(benchAutogenTemplateYAML), 1); err != nil {
		b.Fatalf("load template: %v", err)
	}
	return engine
}

func benchmarkAutogenExcludeReader(
	b *testing.B,
	shape string,
	seriesCount int,
	nameLength int,
) (metrix.Reader, string) {
	b.Helper()

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		b.Fatal("collector store is not cycle-managed")
	}
	cc := managed.CycleController()
	meter := store.Write().SnapshotMeter("")
	target := fixedBenchmarkToken("z_metric", 0, nameLength)

	cc.BeginCycle()
	switch shape {
	case "one_family_many_labels":
		gauge := meter.Gauge(target)
		for i := range seriesCount {
			gauge.Observe(metrix.SampleValue(i), meter.LabelSet(
				metrix.Label{Key: "series", Value: strconv.Itoa(i)},
			))
		}
	default:
		for i := range seriesCount {
			name := fixedBenchmarkToken("z_metric", i, nameLength)
			meter.Gauge(name).Observe(metrix.SampleValue(i))
		}
	}
	if err := cc.CommitCycleSuccess(); err != nil {
		b.Fatalf("commit benchmark cycle: %v", err)
	}
	return store.Read(metrix.ReadRaw()), target
}

func benchmarkPlanExcludePatterns(scenario string, count, length int, target string) []string {
	switch scenario {
	case "late_exact":
		return lateHitBenchmarkPatterns(count, length, target)
	case "general_star_hit":
		return lateHitBenchmarkPatterns(count, length, internalStarPattern(target, true))
	case "wildcard":
		return lateHitBenchmarkPatterns(count, length, "*")
	default:
		patterns, _ := benchmarkExcludeMatchCase(count, length, "general_star_miss")
		return patterns
	}
}

func benchmarkStructuredAutogenReader(b *testing.B, kind string) (metrix.Reader, string) {
	b.Helper()

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		b.Fatal("collector store is not cycle-managed")
	}
	cc := managed.CycleController()
	meter := store.Write().SnapshotMeter("svc")
	familyName := "svc." + kind

	cc.BeginCycle()
	switch kind {
	case "histogram":
		histogram := meter.Histogram("histogram", metrix.WithHistogramBounds(0.5, 1))
		histogram.ObservePoint(metrix.HistogramPoint{
			Count: 3,
			Sum:   1.7,
			Buckets: []metrix.BucketPoint{
				{UpperBound: 0.5, CumulativeCount: 1},
				{UpperBound: 1, CumulativeCount: 3},
			},
		})
	case "summary":
		summary := meter.Summary("summary", metrix.WithSummaryQuantiles(0.5, 0.9))
		summary.ObservePoint(metrix.SummaryPoint{
			Count: 4,
			Sum:   2.4,
			Quantiles: []metrix.QuantilePoint{
				{Quantile: 0.5, Value: 0.4},
				{Quantile: 0.9, Value: 0.9},
			},
		})
	case "stateset":
		stateSet := meter.StateSet(
			"stateset",
			metrix.WithStateSetStates("ready", "stopped"),
			metrix.WithStateSetMode(metrix.ModeEnum),
		)
		stateSet.Enable("ready")
	default:
		measureSet := meter.MeasureSetGauge(
			"measureset",
			metrix.WithMeasureSetFields(
				metrix.MeasureFieldSpec{Name: "used"},
				metrix.MeasureFieldSpec{Name: "free"},
			),
		)
		measureSet.ObservePoint(metrix.MeasureSetPoint{Values: []metrix.SampleValue{7, 3}})
	}
	if err := cc.CommitCycleSuccess(); err != nil {
		b.Fatalf("commit benchmark cycle: %v", err)
	}
	return store.Read(metrix.ReadRaw(), metrix.ReadFlatten()), familyName
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
	case "exact_late":
		source := fixedBenchmarkToken("z_target", 0, length)
		return lateHitBenchmarkPatterns(count, length, source), source
	case "prefix_early":
		source := fixedBenchmarkToken("a_target", 0, length)
		patterns := []string{source[:len(source)-1] + "*"}
		for i := 1; i < count; i++ {
			patterns = append(patterns, fixedBenchmarkToken("z_miss", i, length))
		}
		return patterns, source
	case "prefix_late":
		source := fixedBenchmarkToken("z_target", 0, length)
		patterns := lateHitBenchmarkPatterns(count, length, source[:len(source)-1]+"*")
		return patterns, source
	case "general_star_early":
		source := fixedBenchmarkToken("a_target", 0, length)
		patterns := []string{internalStarPattern(source, true)}
		for i := 1; i < count; i++ {
			patterns = append(patterns, fixedBenchmarkToken("z_miss", i, length))
		}
		return patterns, source
	case "general_star_late":
		source := fixedBenchmarkToken("z_target", 0, length)
		patterns := make([]string, 0, count)
		for i := 0; i < count-1; i++ {
			patterns = append(patterns, fixedBenchmarkToken("a_miss", i, length))
		}
		patterns = append(patterns, internalStarPattern(source, true))
		return patterns, source
	case "general_star_miss":
		source := fixedBenchmarkToken("z_target", 0, length)
		patterns := make([]string, 0, count)
		for i := range count {
			token := fixedBenchmarkToken("a_miss", i, length)
			patterns = append(patterns, internalStarPattern(token, false))
		}
		return patterns, source
	case "wildcard":
		source := fixedBenchmarkToken("z_target", 0, length)
		patterns := lateHitBenchmarkPatterns(count, length, "*")
		return patterns, source
	default: // exact_miss
		source := fixedBenchmarkToken("z_target", 0, length)
		patterns := make([]string, 0, count)
		for i := range count {
			patterns = append(patterns, fixedBenchmarkToken("a_miss", i, length))
		}
		return patterns, source
	}
}

func internalStarPattern(token string, matches bool) string {
	middle := len(token) / 2
	suffix := token[middle+1:]
	if !matches {
		suffix = "never_" + suffix
	}
	return token[:middle] + "*" + suffix
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

func benchmarkAutogenExcludeTemplate(patterns []string) []byte {
	var out strings.Builder
	out.WriteString(`
version: v1
engine:
  autogen:
    enabled: true
    exclude:
`)
	for _, pattern := range patterns {
		fmt.Fprintf(&out, "      - %q\n", pattern)
	}
	out.WriteString(`groups:
  - family: Bench
    metrics: [authored_metric]
    charts:
      - title: Authored
        context: authored
        units: units
        dimensions:
          - selector: authored_metric
            name: authored
`)
	return []byte(out.String())
}
