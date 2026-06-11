// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
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
//   - relabel_assemble_validate: applyBlocks + Assemble + typed-family validation.
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
		proc, err := relabel.New([]relabel.Config{{
			SourceLabels: []string{"__name__"},
			Regex:        relabel.MustNewRegexp("(.+)"),
			TargetLabel:  "env",
			Replacement:  "prod",
			Action:       relabel.Replace,
		}})
		if err != nil {
			b.Fatal(err)
		}
		c := &Collector{relabelBlocks: []relabelBlock{{match: matcher.TRUE(), proc: proc}}}

		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if _, err := c.relabelAndAssemble(batch, false); err != nil {
				b.Fatal(err)
			}
		}
	})
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
