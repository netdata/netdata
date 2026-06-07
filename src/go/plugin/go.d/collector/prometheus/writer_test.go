// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricFamilyWriter(t *testing.T) {
	tests := map[string]struct {
		exposition string
		policy     metricFamilyWriterPolicy
		assert     func(t *testing.T, fr metrix.Reader, written int)
	}{
		"gauge with label": {
			exposition: `
# TYPE app_temp gauge
app_temp{sensor="cpu"} 12.5
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 1, written)
				assert.InDelta(t, 12.5, value(t, fr, "app_temp", metrix.Labels{"sensor": "cpu"}), 1e-9)
			},
		},
		"counter total": {
			exposition: `
# TYPE app_requests_total counter
app_requests_total{code="200"} 42
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 1, written)
				assert.InDelta(t, 42, value(t, fr, "app_requests_total", metrix.Labels{"code": "200"}), 1e-9)
			},
		},
		"summary with quantiles sum count": {
			exposition: `
# TYPE app_latency summary
app_latency{quantile="0.5"} 0.1
app_latency{quantile="0.9"} 0.4
app_latency_sum 5.0
app_latency_count 10
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 1, written)
				assert.InDelta(t, 0.1, value(t, fr, "app_latency", metrix.Labels{"quantile": "0.5"}), 1e-9)
				assert.InDelta(t, 0.4, value(t, fr, "app_latency", metrix.Labels{"quantile": "0.9"}), 1e-9)
				assert.InDelta(t, 5.0, value(t, fr, "app_latency_sum", nil), 1e-9)
				assert.InDelta(t, 10, value(t, fr, "app_latency_count", nil), 1e-9)
			},
		},
		"summary empty window stores NaN quantiles (renders as a gap downstream)": {
			exposition: `
# TYPE app_latency summary
app_latency{quantile="0.5"} NaN
app_latency{quantile="0.9"} NaN
app_latency_sum 0
app_latency_count 0
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 1, written)
				v, ok := fr.Value("app_latency", metrix.Labels{"quantile": "0.5"})
				require.True(t, ok, "NaN quantile series must be stored")
				assert.True(t, math.IsNaN(float64(v)), "quantile value must be stored as NaN, got %v", v)
			},
		},
		"histogram with buckets": {
			exposition: `
# TYPE app_dur histogram
app_dur_bucket{le="0.1"} 1
app_dur_bucket{le="0.5"} 3
app_dur_bucket{le="+Inf"} 4
app_dur_sum 0.9
app_dur_count 4
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 1, written)
				assert.InDelta(t, 1, value(t, fr, "app_dur_bucket", metrix.Labels{"le": "0.1"}), 1e-9)
				assert.InDelta(t, 3, value(t, fr, "app_dur_bucket", metrix.Labels{"le": "0.5"}), 1e-9)
				assert.InDelta(t, 4, value(t, fr, "app_dur_count", nil), 1e-9)
				assert.InDelta(t, 0.9, value(t, fr, "app_dur_sum", nil), 1e-9)
			},
		},
		"family with heterogeneous label keys writes every series (P0-1)": {
			exposition: `
# TYPE app_state gauge
app_state{az="a"} 1
app_state{region="eu",az="b"} 2
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 2, written, "both series must be written despite differing label keys")
				assert.InDelta(t, 1, value(t, fr, "app_state", metrix.Labels{"az": "a"}), 1e-9)
				assert.InDelta(t, 2, value(t, fr, "app_state", metrix.Labels{"region": "eu", "az": "b"}), 1e-9)
			},
		},
		"skips NaN scalar value": {
			exposition: `
# TYPE app_temp gauge
app_temp{sensor="ok"} 3
app_temp{sensor="bad"} NaN
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 1, written)
				_, ok := fr.Value("app_temp", metrix.Labels{"sensor": "bad"})
				assert.False(t, ok, "NaN scalar series must be skipped, not written")
			},
		},
		"skips summary series with Inf quantile value": {
			exposition: `
# TYPE app_latency summary
app_latency{quantile="0.5"} +Inf
app_latency_sum 1
app_latency_count 1
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 0, written, "summary with an infinite quantile value must be skipped")
			},
		},
		"applies label_prefix to label keys": {
			exposition: `
# TYPE app_temp gauge
app_temp{sensor="cpu"} 7
`,
			policy: metricFamilyWriterPolicy{labelPrefix: "px"},
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 1, written)
				assert.InDelta(t, 7, value(t, fr, "app_temp", metrix.Labels{"px_sensor": "cpu"}), 1e-9)
				_, ok := fr.Value("app_temp", metrix.Labels{"sensor": "cpu"})
				assert.False(t, ok, "unprefixed label key must not exist")
			},
		},
		"skips _info family": {
			exposition: `
# TYPE app_build_info gauge
app_build_info{version="1.2.3"} 1
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 0, written, "_info family must be skipped entirely")
			},
		},
		"skips family exceeding maxTSPerMetric": {
			exposition: `
# TYPE app_temp gauge
app_temp{id="1"} 1
app_temp{id="2"} 2
app_temp{id="3"} 3
`,
			policy: metricFamilyWriterPolicy{maxTSPerMetric: 2},
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 0, written, "family over the per-metric series limit must be skipped")
			},
		},
		"untyped falls back to gauge and counter": {
			exposition: `
app_fallback_gauge 7
app_things_total 5
`,
			policy: metricFamilyWriterPolicy{
				isFallbackTypeGauge: func(name string) bool { return name == "app_fallback_gauge" },
			},
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 2, written)
				assert.InDelta(t, 7, value(t, fr, "app_fallback_gauge", nil), 1e-9)
				assert.InDelta(t, 5, value(t, fr, "app_things_total", nil), 1e-9)
			},
		},
		"float bucket-bound label format (B1 probe)": {
			exposition: `
# TYPE app_size histogram
app_size_bucket{le="0.00001"} 1
app_size_bucket{le="1000000"} 2
app_size_bucket{le="+Inf"} 2
app_size_sum 3
app_size_count 2
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 1, written)
				// metrix formats the flattened bucket "le" label with strconv 'g'; V1 used 'f'.
				// For sci-notation bounds the dim names differ (decision B1). This probe pins the
				// metrix format so we know whether any real golden/target trips it.
				for _, bound := range []float64{0.00001, 1000000} {
					leG := strconv.FormatFloat(bound, 'g', -1, 64)
					_, ok := fr.Value("app_size_bucket", metrix.Labels{"le": leG})
					assert.Truef(t, ok, "bucket le label expected in metrix 'g' format %q (V1 'f' was %q)",
						leG, strconv.FormatFloat(bound, 'f', -1, 64))
				}
			},
		},
		"metadata: summary unit gets /s and title is sanitized": {
			exposition: `
# HELP app_resp_bytes Response 'size' in bytes.
# TYPE app_resp_bytes summary
app_resp_bytes{quantile="0.5"} 100
app_resp_bytes_sum 500
app_resp_bytes_count 5
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, 1, written)
				mm := mustMeta(t, fr, "app_resp_bytes")
				assert.Equal(t, "bytes/s", mm.Unit, "V1 appends /s to summary quantile units")
				assert.Equal(t, "Response size in bytes", mm.Description, "title strips apostrophes and a trailing period")
				assert.True(t, mm.Float)
				assert.Equal(t, getChartFamily("app_resp_bytes"), mm.ChartFamily)
				assert.Equal(t, getChartPriority("app_resp_bytes"), mm.ChartPriority)
			},
		},
		"metadata: gauge unit has no /s": {
			exposition: `
# TYPE app_used_bytes gauge
app_used_bytes 10
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, "bytes", mustMeta(t, fr, "app_used_bytes").Unit)
			},
		},
		"metadata: counter unit is the base (autogen's incremental route adds /s)": {
			exposition: `
# TYPE app_io_bytes_total counter
app_io_bytes_total 5
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, "bytes", mustMeta(t, fr, "app_io_bytes_total").Unit)
			},
		},
		"metadata: empty HELP yields V1 default title": {
			exposition: `
# TYPE app_widgets gauge
app_widgets 3
`,
			assert: func(t *testing.T, fr metrix.Reader, written int) {
				assert.Equal(t, `Metric "app_widgets"`, mustMeta(t, fr, "app_widgets").Description)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			store := metrix.NewCollectorStore()
			w := newMetricFamilyWriter(store, tc.policy, logger.New())

			mfs := scrape(t, tc.exposition)

			cc := cycle(t, store)
			cc.BeginCycle()
			written := w.writeMetricFamilies(mfs)
			require.NoError(t, cc.CommitCycleSuccess())

			tc.assert(t, store.Read(metrix.ReadFlatten()), written)
		})
	}
}

func scrape(t *testing.T, exposition string) prompkg.MetricFamilies {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(exposition))
	}))
	t.Cleanup(srv.Close)

	mfs, err := prompkg.New(srv.Client(), web.RequestConfig{URL: srv.URL}).Scrape()
	require.NoError(t, err)
	return mfs
}

func cycle(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	return managed.CycleController()
}

func value(t *testing.T, fr metrix.Reader, name string, labels metrix.Labels) float64 {
	t.Helper()
	v, ok := fr.Value(name, labels)
	require.Truef(t, ok, "expected flattened series %q labels=%v", name, labels)
	return float64(v)
}

func mustMeta(t *testing.T, fr metrix.Reader, name string) metrix.MetricMeta {
	t.Helper()
	mm, ok := fr.MetricMeta(name)
	require.Truef(t, ok, "expected MetricMeta for %q", name)
	return mm
}

func TestMetricFamilyWriterEdgeCases(t *testing.T) {
	t.Run("countWritable counts writable series and skips _info", func(t *testing.T) {
		store := metrix.NewCollectorStore()
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		mfs := scrape(t, `
# TYPE app_a gauge
app_a{x="1"} 1
app_a{x="2"} 2
# TYPE app_b_info gauge
app_b_info{v="x"} 1
`)
		assert.Equal(t, 2, w.countWritable(mfs))
	})

	t.Run("metric type drift skips the family after the type changes", func(t *testing.T) {
		store := metrix.NewCollectorStore()
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		cc := cycle(t, store)

		cc.BeginCycle()
		require.Equal(t, 1, w.writeMetricFamilies(scrape(t, "# TYPE app_x gauge\napp_x 1\n")))
		require.NoError(t, cc.CommitCycleSuccess())

		cc.BeginCycle()
		assert.Equal(t, 0, w.writeMetricFamilies(scrape(t, "# TYPE app_x counter\napp_x 5\n")),
			"same metric name with a changed type must be skipped")
		require.NoError(t, cc.CommitCycleSuccess())
	})

	t.Run("summary distribution-schema drift skips the off-schema series", func(t *testing.T) {
		store := metrix.NewCollectorStore()
		w := newMetricFamilyWriter(store, metricFamilyWriterPolicy{}, logger.New())
		cc := cycle(t, store)

		cc.BeginCycle()
		written := w.writeMetricFamilies(scrape(t, `
# TYPE app_lat summary
app_lat{id="a",quantile="0.5"} 1
app_lat_sum{id="a"} 1
app_lat_count{id="a"} 1
app_lat{id="b",quantile="0.5"} 2
app_lat{id="b",quantile="0.9"} 3
app_lat_sum{id="b"} 5
app_lat_count{id="b"} 2
`))
		require.NoError(t, cc.CommitCycleSuccess())

		assert.Equal(t, 1, written, "the series whose quantile set differs from the family canonical is skipped")
		fr := store.Read(metrix.ReadFlatten())
		_, ok := fr.Value("app_lat", metrix.Labels{"id": "a", "quantile": "0.5"})
		assert.True(t, ok, "canonical-schema series is written")
		_, ok = fr.Value("app_lat", metrix.Labels{"id": "b", "quantile": "0.5"})
		assert.False(t, ok, "off-schema series is skipped")
	})
}
