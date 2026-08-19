// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

// BenchmarkRelabelExecutor isolates the collector-side cost relabeling adds on top
// of a no-rules scrape. Both sub-benchmarks operate on the same pre-parsed sample
// batch, so HTTP and parsing are excluded and the delta is purely the executor:
//
//   - assemble_only: prompkg.Assemble — what the no-rules fast path does after parse.
//   - relabel_assemble_validate: Pipeline.Apply + Assemble + typed-family validation.
//
// The relabel block adds a static label to every series, so every typed family is
// touched and the full validation pass runs (a near worst case). Parsing's own
// streaming-vs-fused cost is measured separately in pkg/prometheus/parse_bench_test.go.
//
// Command:
//
//	go test ./plugin/go.d/collector/prometheus -run '^$' -bench RelabelExecutor -benchmem
//
// Measured on a developer laptop (Apple M4 Pro), 165 series (150 counters + 15
// histograms), not CI — compare relative deltas, not absolutes (captured 2026-08-05):
//
//	assemble_only                13511 ns/op    38272 B/op    414 allocs/op
//	relabel_assemble_validate   121398 ns/op   119950 B/op   1577 allocs/op
//
// The executor adds ~105us per scrape in this near worst case (a rule rewriting every
// series, so every typed family is validated). It is paid only on jobs that configure
// relabeling, once per scrape interval; the no-rules fast path keeps the assemble-only
// profile. The bulk is the per-sample relabel.Apply (builder reset + label rebuild),
// not validation.
func BenchmarkRelabelExecutor(b *testing.B) {
	batch := scrapeSamples(b, benchExposition())

	b.Run("assemble_only", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := prompkg.Assemble(batch); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("relabel_assemble_validate", func(b *testing.B) {
		pipeline, err := relabel.NewPipeline([]relabel.Block{{
			Match: "*",
			MetricRelabelConfigs: []relabel.Config{{
				SourceLabels: []string{"__name__"},
				Regex:        relabel.MustNewRegexp("(.+)"),
				TargetLabel:  "env",
				Replacement:  "prod",
				Action:       relabel.Replace,
			}},
		}})
		if err != nil {
			b.Fatal(err)
		}
		c := &Collector{jobRelabel: pipeline}

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, _, err := c.relabelAndAssemble(batch, c.jobRelabel, jobRelabelStage, false); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkRelabelScrapeModes measures the complete steady-state fetch and parse
// path for every job/profile relabel and profile fallback combination. Unlike
// BenchmarkRelabelExecutor, this includes the direct parser versus buffered-sample
// parser difference. The fixture adds one untyped scalar to benchExposition so the
// fallback cases exercise classification as well as the bound-sample dispatch.
// A three-run 2026-08-06 developer-laptop sample (Apple M4 Pro), not a CI gate:
//
//	mode                              ns/op         B/op       allocs/op
//	no_rules                         93.4-95.7 us   46.9 KB     586
//	job_rules                       210.6-242.7 us  217.6-217.9 KB  1706
//	fallback_rules                  141.8-148.1 us  144.0-144.3 KB  1212
//	profile_rules                   243.8-245.1 us  217.9-218.2 KB  1707-1708
//	job_and_fallback_rules          258.5-263.4 us  260.7-261.2 KB  2139-2140
//	job_and_profile_rules           337.3-340.0 us  347.1-347.3 KB  2664
//	fallback_and_profile_rules      234.7-237.3 us  216.4-216.7 KB  1706
//	job_fallback_and_profile_rules  335.1-337.0 us  346.3-346.5 KB  2663
//
// An earlier three-run comparison against the pre-feature master snapshot used
// benchExposition without the extra untyped scalar and found no material
// existing-path regression. Those historical numbers are not directly comparable
// to the 166-series table above:
//
//	mode        pre-feature master                              current branch
//	no_rules    93.2-94.3 us, 46.8-46.9 KB, 584 allocs/op       93.5-95.9 us, 46.8-47.0 KB, 584 allocs/op
//	job_rules   208-239 us, 217.3-217.6 KB, 1696-1697 allocs    212-243 us, 217.4-217.9 KB, 1696-1697 allocs
//
// Profile-policy modes had no pre-feature equivalent; their absolute opt-in costs
// are reported above.
func BenchmarkRelabelScrapeModes(b *testing.B) {
	exposition := benchExposition() + "app_untyped_value 1\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(exposition))
	}))
	b.Cleanup(srv.Close)

	tests := []struct {
		name          string
		jobRules      bool
		fallbackRules bool
		profileRules  bool
	}{
		{name: "no_rules"},
		{name: "job_rules", jobRules: true},
		{name: "fallback_rules", fallbackRules: true},
		{name: "profile_rules", profileRules: true},
		{name: "job_and_fallback_rules", jobRules: true, fallbackRules: true},
		{name: "job_and_profile_rules", jobRules: true, profileRules: true},
		{name: "fallback_and_profile_rules", fallbackRules: true, profileRules: true},
		{name: "job_fallback_and_profile_rules", jobRules: true, fallbackRules: true, profileRules: true},
	}

	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			collr := New()
			collr.URL = srv.URL
			collr.Profiles = ProfilesConfig{Mode: profilesModeNone}
			if tc.jobRules {
				collr.Relabeling = benchmarkRelabelBlocks("job_stage")
			}
			if err := collr.Init(context.Background()); err != nil {
				b.Fatal(err)
			}

			runtime := &promRuntime{}
			if tc.fallbackRules {
				runtime.fallbacks = []profileFallback{{
					root:    matcher.Must(matcher.NewSimplePatternsMatcher("app_*")),
					gauge:   matcher.Must(matcher.NewGlobMatcher("app_untyped_value")),
					counter: matcher.FALSE(),
				}}
			}
			if tc.profileRules {
				runtime.normalizers = []profileNormalizer{benchmarkProfileNormalizer(b, "*", "*", "profile_stage")}
			}

			b.ReportAllocs()
			b.SetBytes(int64(len(exposition)))
			b.ResetTimer()
			for range b.N {
				var err error
				if !runtime.hasProfileSamplePolicy() {
					_, err = collr.scrape(context.Background(), false)
				} else {
					_, err = collr.scrapeProfilePipeline(context.Background(), runtime, false)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkProfileRelabelDispatch isolates first-applicable family dispatch.
// The sixteen-profile cases guard against accidental profile chaining,
// repeated matcher work for labeled series, or an unbounded dispatch cache.
// The 2026-08-05 baseline measured 6,462-6,517 ns/op for one profile,
// 7,582-7,813 ns/op for sixteen disjoint roots, and 23,870-24,008 ns/op for
// sixteen overlapping roots with disjoint blocks; all used 16,216 B/op and 93
// allocations. Before physical-name caching, first-applicable dispatch measured
// 2,455-2,560 ns/op, 4,807-4,865 ns/op, and 7,066-7,145 ns/op respectively;
// the sixteen-profile all-miss case measured 24,166-24,685 ns/op. The bounded
// one-entry cache measured 2,497-2,564 ns/op, 4,074-4,212 ns/op,
// 5,459-5,684 ns/op, and 6,604-6,658 ns/op respectively. The first three use
// 1,640 B/op and 7 allocations; all-miss uses 48 B/op and 1 allocation.
func BenchmarkProfileRelabelDispatch(b *testing.B) {
	batch := scrapeSamples(b, benchExposition())

	b.Run("one_profile", func(b *testing.B) {
		normalizers := []profileNormalizer{benchmarkProfileNormalizer(b, "*", "*", "profile_stage")}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			selectProfileNormalizers(batch, normalizers)
		}
	})

	b.Run("sixteen_disjoint_profiles", func(b *testing.B) {
		normalizers := benchmarkDisjointNormalizers(b, false)

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			selectProfileNormalizers(batch, normalizers)
		}
	})

	b.Run("sixteen_overlapping_roots_disjoint_blocks", func(b *testing.B) {
		normalizers := benchmarkDisjointNormalizers(b, true)

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			selectProfileNormalizers(batch, normalizers)
		}
	})

	b.Run("sixteen_overlapping_roots_nonmatching_blocks", func(b *testing.B) {
		normalizers := benchmarkNonmatchingNormalizers(b, 16)

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			selectProfileNormalizers(batch, normalizers)
		}
	})
}

func benchmarkDisjointNormalizers(tb testing.TB, overlappingRoots bool) []profileNormalizer {
	tb.Helper()
	normalizers := make([]profileNormalizer, 0, 16)
	patterns := []string{"http_requests_total"}
	for i := range 15 {
		patterns = append(patterns, fmt.Sprintf("lat_%d_seconds", i))
	}
	for _, base := range patterns {
		root := base
		if overlappingRoots {
			root = "*"
		}
		normalizers = append(normalizers, benchmarkProfileNormalizer(tb, root, base+"*", "profile_stage"))
	}
	return normalizers
}

func benchmarkNonmatchingNormalizers(tb testing.TB, count int) []profileNormalizer {
	tb.Helper()
	normalizers := make([]profileNormalizer, 0, count)
	for i := range count {
		normalizers = append(normalizers,
			benchmarkProfileNormalizer(tb, "*", fmt.Sprintf("missing_%d*", i), "profile_stage"))
	}
	return normalizers
}

func benchmarkProfileNormalizer(tb testing.TB, rootPattern, blockPattern, label string) profileNormalizer {
	tb.Helper()
	root, err := matcher.NewSimplePatternsMatcher(rootPattern)
	if err != nil {
		tb.Fatal(err)
	}
	pipeline, err := relabel.NewPipeline(benchmarkRelabelBlocksForMatch(blockPattern, label))
	if err != nil {
		tb.Fatal(err)
	}
	return profileNormalizer{root: root, pipeline: pipeline}
}

func benchmarkRelabelBlocks(label string) []relabel.Block {
	return benchmarkRelabelBlocksForMatch("*", label)
}

func benchmarkRelabelBlocksForMatch(match, label string) []relabel.Block {
	return []relabel.Block{{
		Match: match,
		MetricRelabelConfigs: []relabel.Config{{
			TargetLabel: label,
			Replacement: "true",
			Action:      relabel.Replace,
		}},
	}}
}

// benchExposition builds a representative scrape: many labeled counters plus a set
// of histograms, so assembly and typed-family validation both do real work.
func benchExposition() string {
	var sb strings.Builder
	sb.WriteString("# TYPE http_requests_total counter\n")
	for i := range 150 {
		fmt.Fprintf(&sb, "http_requests_total{code=\"200\",path=\"/p%d\"} %d\n", i, i)
	}
	for i := range 15 {
		fmt.Fprintf(&sb, "# TYPE lat_%d_seconds histogram\n", i)
		fmt.Fprintf(&sb, "lat_%d_seconds_bucket{le=\"0.1\"} 4\n", i)
		fmt.Fprintf(&sb, "lat_%d_seconds_bucket{le=\"0.5\"} 8\n", i)
		fmt.Fprintf(&sb, "lat_%d_seconds_bucket{le=\"+Inf\"} 10\n", i)
		fmt.Fprintf(&sb, "lat_%d_seconds_sum 2.5\n", i)
		fmt.Fprintf(&sb, "lat_%d_seconds_count 10\n", i)
	}
	return sb.String()
}
