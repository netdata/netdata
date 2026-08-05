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
// histograms), not CI — compare relative deltas, not absolutes (captured 2026-06-10):
//
//	assemble_only                13545 ns/op    38272 B/op    414 allocs/op
//	relabel_assemble_validate   118776 ns/op   119949 B/op   1577 allocs/op
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
// path for each supported stage combination. Unlike BenchmarkRelabelExecutor,
// this includes the direct parser versus buffered-sample parser difference.
func BenchmarkRelabelScrapeModes(b *testing.B) {
	exposition := benchExposition()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(exposition))
	}))
	b.Cleanup(srv.Close)

	tests := []struct {
		name         string
		jobRules     bool
		profileRules bool
	}{
		{name: "no_rules"},
		{name: "job_rules", jobRules: true},
		{name: "profile_rules", profileRules: true},
		{name: "job_and_profile_rules", jobRules: true, profileRules: true},
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

			var normalizers []profileNormalizer
			if tc.profileRules {
				normalizers = []profileNormalizer{benchmarkProfileNormalizer(b, "all", "*", "*", "profile_stage")}
			}

			b.ReportAllocs()
			b.SetBytes(int64(len(exposition)))
			b.ResetTimer()
			for range b.N {
				var err error
				if len(normalizers) == 0 {
					_, err = collr.scrape(context.Background(), false)
				} else {
					_, err = collr.scrapeProfileNormalized(context.Background(), normalizers, false)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkProfileRelabelDispatch isolates ownership resolution. The sixteen-
// profile case gives every source family one disjoint owner and guards against
// accidental pairwise profile composition or an unbounded dispatch cache.
func BenchmarkProfileRelabelDispatch(b *testing.B) {
	batch := scrapeSamples(b, benchExposition())

	b.Run("one_profile", func(b *testing.B) {
		normalizers := []profileNormalizer{benchmarkProfileNormalizer(b, "all", "*", "*", "profile_stage")}
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			resolveProfileOwners(batch, normalizers)
		}
	})

	b.Run("sixteen_disjoint_profiles", func(b *testing.B) {
		normalizers := make([]profileNormalizer, 0, 16)
		normalizers = append(normalizers,
			benchmarkProfileNormalizer(b, "http", "http_requests_total", "http_requests_total", "profile_stage"))
		for i := range 15 {
			base := fmt.Sprintf("lat_%d_seconds", i)
			normalizers = append(normalizers,
				benchmarkProfileNormalizer(b, fmt.Sprintf("lat_%d", i), base, base+"*", "profile_stage"))
		}

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			resolveProfileOwners(batch, normalizers)
		}
	})
}

func benchmarkProfileNormalizer(tb testing.TB, name, rootPattern, blockPattern, label string) profileNormalizer {
	tb.Helper()
	root, err := matcher.NewSimplePatternsMatcher(rootPattern)
	if err != nil {
		tb.Fatal(err)
	}
	pipeline, err := relabel.NewPipeline(benchmarkRelabelBlocksForMatch(blockPattern, label))
	if err != nil {
		tb.Fatal(err)
	}
	return profileNormalizer{name: name, root: root, pipeline: pipeline}
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
