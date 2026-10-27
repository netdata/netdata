// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// Compare identical accepted workloads with the policy enabled and disabled.
// Full fan-in keeps one output context and pair as source cardinality increases:
// incremental admission allocations must stay O(contexts), not O(contributors).
func BenchmarkContextLimitFanIn(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		for _, limit := range []int{0, 1000} {
			b.Run(fmt.Sprintf("series_%d/limit_%d", n, limit), func(b *testing.B) {
				reader := benchmarkAggregationReader(b, n, 1)
				e, err := New(WithRuntimeStore(nil), WithRuntimePlannerMode(), WithEnginePolicy(EnginePolicy{
					MaxOutputSeriesPerContext: &limit,
				}))
				if err != nil {
					b.Fatal(err)
				}
				template := strings.ReplaceAll(benchAggregationTemplateYAML, "AGGREGATION", "sum")
				if err := e.LoadYAML([]byte(template), 1); err != nil {
					b.Fatal(err)
				}
				if _, err := buildPlan(e, reader); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if _, err := buildPlan(e, reader); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// Each candidate context contains one aggregated output. Counts and reporting
// population may grow with contexts, while remaining independent of source fan-in.
func BenchmarkContextLimitManyContexts(b *testing.B) {
	for _, n := range []int{1, 100} {
		var template strings.Builder
		template.WriteString("version: v1\ngroups:\n  - family: Many\n    metrics: [bench_metric]\n    charts:\n")
		for i := range n {
			fmt.Fprintf(&template, "      - id: c%d\n        title: Work\n        context: c%d\n        units: work\n        dimensions:\n          - selector: bench_metric\n            name: value\n", i, i)
		}
		for _, limit := range []int{0, 1000} {
			b.Run(fmt.Sprintf("contexts_%d/limit_%d", n, limit), func(b *testing.B) {
				reader := benchmarkAggregationReader(b, 100, 1)
				e, err := New(WithRuntimeStore(nil), WithRuntimePlannerMode(), WithEnginePolicy(EnginePolicy{
					MaxOutputSeriesPerContext: &limit,
				}))
				if err != nil {
					b.Fatal(err)
				}
				if err := e.LoadYAML([]byte(template.String()), 1); err != nil {
					b.Fatal(err)
				}
				if _, err := buildPlan(e, reader); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if _, err := buildPlan(e, reader); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// A context may exceed its budget through many one-dimensional chart instances,
// even when no individual chart exceeds the limit.
func BenchmarkContextLimitInstances(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		for _, limit := range []int{0, 1000} {
			b.Run(fmt.Sprintf("instances_%d/limit_%d", n, limit), func(b *testing.B) {
				reader := benchmarkAggregationReader(b, n, n)
				e, err := New(WithRuntimeStore(nil), WithRuntimePlannerMode(), WithEnginePolicy(EnginePolicy{MaxOutputSeriesPerContext: &limit}))
				if err != nil {
					b.Fatal(err)
				}
				template := strings.ReplaceAll(benchAggregationTemplateYAML, "AGGREGATION", "sum")
				if err := e.LoadYAML([]byte(template), 1); err != nil {
					b.Fatal(err)
				}
				if _, err := buildPlan(e, reader); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if _, err := buildPlan(e, reader); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// Detail is omitted at limit=3 while both summaries retain every contributor.
// This measures the cost of the admitted workload, not an old rejected scrape.
func BenchmarkContextLimitRejectedDetail(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("series_%d", n), func(b *testing.B) {
			e := newContextLimitEngine(b, 3, contextLimitYAML)
			store := metrix.NewCollectorStore()
			reader := contextLimitCycle(b, store, n, 0, 1)
			if _, err := buildPlan(e, reader); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := buildPlan(e, reader); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Every operation includes both growth past the output budget and recovery.
// Recovery must reconcile labels again; this includes that cost instead of
// measuring only the cheaper repeated-omission case.
func BenchmarkContextLimitRecoveryCycle(b *testing.B) {
	for _, limit := range []int{0, 1000} {
		b.Run(fmt.Sprintf("limit_%d", limit), func(b *testing.B) {
			small := benchmarkAggregationReader(b, 1000, 1000)
			large := benchmarkAggregationReader(b, 10000, 10000)
			e, err := New(WithRuntimeStore(nil), WithRuntimePlannerMode(), WithEnginePolicy(EnginePolicy{MaxOutputSeriesPerContext: &limit}))
			if err != nil {
				b.Fatal(err)
			}
			if err := e.LoadYAML([]byte(strings.ReplaceAll(benchAggregationTemplateYAML, "AGGREGATION", "sum")), 1); err != nil {
				b.Fatal(err)
			}
			if _, err := buildPlan(e, small); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := buildPlan(e, large); err != nil {
					b.Fatal(err)
				}
				if _, err := buildPlan(e, small); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
