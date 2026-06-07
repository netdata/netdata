// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

// BenchmarkMetricFamilyWriter measures the steady-state cost of writing a high-cardinality scrape
// (seriesPerType series of each of the four metric types) to metrix each cycle. The scrape is parsed
// once; the loop exercises only writeMetricFamilies so the per-series instrument-resolution cost is
// isolated from parsing/HTTP.
//
// Indicative results on a developer laptop (macOS, 14 logical CPUs; `-benchmem -count=3`),
// 2000 series/cycle — relative figures, not an absolute or CI baseline:
//
//	per-series resolve, no cache:    ~1.60 ms/op   3.46 MB/op   38088 allocs/op
//	bounded per-series handle cache: ~0.96 ms/op   1.92 MB/op   18608 allocs/op
//
// The writer caches the per-series instrument handle and evicts it after seriesCacheRetentionCycles
// unobserved cycles. A metrix vec is deliberately NOT used: its vecCache is unbounded for the vec's
// lifetime and is not pruned by store retention (pkg/metrix/vec.go), so it would leak under
// label-value churn; the bounded cache keeps the speedup without that risk.
func BenchmarkMetricFamilyWriter(b *testing.B) {
	const seriesPerType = 500

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(buildBenchExposition(seriesPerType)))
	}))
	defer srv.Close()

	mfs, err := prompkg.New(srv.Client(), web.RequestConfig{URL: srv.URL}).Scrape()
	if err != nil {
		b.Fatal(err)
	}

	store := metrix.NewCollectorStore()
	w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		b.Fatal("store is not cycle-managed")
	}
	cc := managed.CycleController()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cc.BeginCycle()
		w.writeMetricFamilies(mfs)
		if err := cc.CommitCycleSuccess(); err != nil {
			b.Fatal(err)
		}
	}
}

func buildBenchExposition(n int) string {
	var b strings.Builder

	b.WriteString("# TYPE bench_gauge_bytes gauge\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "bench_gauge_bytes{id=\"%d\",az=\"a\"} %d\n", i, i)
	}
	b.WriteString("# TYPE bench_ops_total counter\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "bench_ops_total{id=\"%d\",az=\"a\"} %d\n", i, i)
	}
	b.WriteString("# TYPE bench_latency_seconds summary\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "bench_latency_seconds{id=\"%d\",quantile=\"0.5\"} 0.1\n", i)
		fmt.Fprintf(&b, "bench_latency_seconds{id=\"%d\",quantile=\"0.9\"} 0.2\n", i)
		fmt.Fprintf(&b, "bench_latency_seconds_sum{id=\"%d\"} 1.0\n", i)
		fmt.Fprintf(&b, "bench_latency_seconds_count{id=\"%d\"} 10\n", i)
	}
	b.WriteString("# TYPE bench_dur_seconds histogram\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "bench_dur_seconds_bucket{id=\"%d\",le=\"0.1\"} 1\n", i)
		fmt.Fprintf(&b, "bench_dur_seconds_bucket{id=\"%d\",le=\"0.5\"} 2\n", i)
		fmt.Fprintf(&b, "bench_dur_seconds_bucket{id=\"%d\",le=\"+Inf\"} 3\n", i)
		fmt.Fprintf(&b, "bench_dur_seconds_sum{id=\"%d\"} 0.5\n", i)
		fmt.Fprintf(&b, "bench_dur_seconds_count{id=\"%d\"} 3\n", i)
	}

	return b.String()
}
