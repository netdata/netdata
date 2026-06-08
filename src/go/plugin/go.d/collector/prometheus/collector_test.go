// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"non empty URL": {
			wantFail: false,
			config:   Config{HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}}},
		},
		"invalid selector syntax": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Selector:   selector.Expr{Allow: []string{`name{label=#"value"}`}},
			},
		},
		"default": {
			wantFail: true,
			config:   New().Config,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })

	collr := New()
	collr.URL = "http://127.0.0.1"
	require.NoError(t, collr.Init(context.Background()))
	assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (collr *Collector, cleanup func())
		wantFail bool
	}{
		"success if endpoint returns valid metrics in prometheus format": {
			wantFail: false,
			prepare: func() (collr *Collector, cleanup func()) {
				srv := httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write([]byte(`test_counter_no_meta_metric_1_total{label1="value1"} 11`))
					}))
				collr = New()
				collr.URL = srv.URL

				return collr, srv.Close
			},
		},
		"fail if the total num of metrics exceeds the limit": {
			wantFail: true,
			prepare: func() (collr *Collector, cleanup func()) {
				srv := httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write([]byte(`
test_counter_no_meta_metric_1_total{label1="value1"} 11
test_counter_no_meta_metric_1_total{label1="value2"} 11
`))
					}))
				collr = New()
				collr.URL = srv.URL
				collr.MaxTS = 1

				return collr, srv.Close
			},
		},
		"fail if the num time series in the metric exceeds the limit": {
			wantFail: true,
			prepare: func() (collr *Collector, cleanup func()) {
				srv := httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write([]byte(`
test_counter_no_meta_metric_1_total{label1="value1"} 11
test_counter_no_meta_metric_1_total{label1="value2"} 11
`))
					}))
				collr = New()
				collr.URL = srv.URL
				collr.MaxTSPerMetric = 1

				return collr, srv.Close
			},
		},
		"fail if metrics have no expected prefix": {
			wantFail: true,
			prepare: func() (collr *Collector, cleanup func()) {
				srv := httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write([]byte(`test_counter_no_meta_metric_1_total{label1="value1"} 11`))
					}))
				collr = New()
				collr.URL = srv.URL
				collr.ExpectedPrefix = "prefix_"

				return collr, srv.Close
			},
		},
		"fail if endpoint returns data not in prometheus format": {
			wantFail: true,
			prepare: func() (collr *Collector, cleanup func()) {
				srv := httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write([]byte("hello and\n goodbye"))
					}))
				collr = New()
				collr.URL = srv.URL

				return collr, srv.Close
			},
		},
		"fail if endpoint exposes only non-writable metrics": {
			wantFail: true,
			prepare: func() (collr *Collector, cleanup func()) {
				srv := httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write([]byte(`app_x_info{version="1.0"} 1`))
					}))
				collr = New()
				collr.URL = srv.URL

				return collr, srv.Close
			},
		},
		"fail if endpoint returns an empty body (no metric families)": {
			wantFail: true,
			prepare: func() (collr *Collector, cleanup func()) {
				srv := httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						_, _ = w.Write([]byte(""))
					}))
				collr = New()
				collr.URL = srv.URL

				return collr, srv.Close
			},
		},
		"fail if connection refused": {
			wantFail: true,
			prepare: func() (collr *Collector, cleanup func()) {
				collr = New()
				collr.URL = "http://127.0.0.1:38001/metrics"

				return collr, func() {}
			},
		},
		"fail if endpoint returns 404": {
			wantFail: true,
			prepare: func() (collr *Collector, cleanup func()) {
				srv := httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(http.StatusNotFound)
					}))
				collr = New()
				collr.URL = srv.URL

				return collr, srv.Close
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare()
			defer cleanup()

			require.NoError(t, collr.Init(context.Background()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

// TestCollector_Collect drives the real V2 collector (Init, then a framework-style store
// cycle around Collect) and asserts the metrics it wrote into the metrix store, by metric
// name + flattened labels. Per-type correctness is exercised exhaustively in writer_test.go;
// this checks the collector's end-to-end wiring (client/selector/fallback built in Init →
// scrape → writer → store) plus the config-driven behaviors.
func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Collector
		input   string
		want    func(t *testing.T, fr metrix.Reader)
	}{
		"gauge and counter values": {
			prepare: New,
			input: `
# TYPE test_gauge_metric gauge
test_gauge_metric{label1="value1"} 11
test_gauge_metric{label1="value2"} 12.5
# TYPE test_counter_metric_total counter
test_counter_metric_total{label1="value1"} 11
`,
			want: func(t *testing.T, fr metrix.Reader) {
				assert.InDelta(t, 11, value(t, fr, "test_gauge_metric", metrix.Labels{"label1": "value1"}), 1e-9)
				assert.InDelta(t, 12.5, value(t, fr, "test_gauge_metric", metrix.Labels{"label1": "value2"}), 1e-9)
				assert.InDelta(t, 11, value(t, fr, "test_counter_metric_total", metrix.Labels{"label1": "value1"}), 1e-9)
			},
		},
		"summary flattens to quantiles, sum and count": {
			prepare: New,
			input: `
# TYPE test_latency summary
test_latency{quantile="0.5"} 0.25
test_latency{quantile="0.99"} 0.5
test_latency_sum 12.5
test_latency_count 42
`,
			want: func(t *testing.T, fr metrix.Reader) {
				assert.InDelta(t, 0.25, value(t, fr, "test_latency", metrix.Labels{"quantile": "0.5"}), 1e-9)
				assert.InDelta(t, 0.5, value(t, fr, "test_latency", metrix.Labels{"quantile": "0.99"}), 1e-9)
				assert.InDelta(t, 12.5, value(t, fr, "test_latency_sum", nil), 1e-9)
				assert.InDelta(t, 42, value(t, fr, "test_latency_count", nil), 1e-9)
			},
		},
		"histogram flattens to buckets, sum and count": {
			prepare: New,
			input: `
# TYPE test_dur histogram
test_dur_bucket{le="0.1"} 4
test_dur_bucket{le="+Inf"} 6
test_dur_sum 2.5
test_dur_count 6
`,
			want: func(t *testing.T, fr metrix.Reader) {
				assert.InDelta(t, 4, value(t, fr, "test_dur_bucket", metrix.Labels{"le": "0.1"}), 1e-9)
				assert.InDelta(t, 6, value(t, fr, "test_dur_bucket", metrix.Labels{"le": "+Inf"}), 1e-9)
				assert.InDelta(t, 2.5, value(t, fr, "test_dur_sum", nil), 1e-9)
				assert.InDelta(t, 6, value(t, fr, "test_dur_count", nil), 1e-9)
			},
		},
		"untyped falls back to gauge and counter": {
			prepare: func() *Collector {
				c := New()
				c.FallbackType.Gauge = []string{"test_fallback_gauge"}
				return c
			},
			input: `
test_fallback_gauge{label1="value1"} 7
test_things_total{label1="value1"} 5
test_untyped_dropped{label1="value1"} 9
`,
			want: func(t *testing.T, fr metrix.Reader) {
				assert.InDelta(t, 7, value(t, fr, "test_fallback_gauge", metrix.Labels{"label1": "value1"}), 1e-9)
				assert.InDelta(t, 5, value(t, fr, "test_things_total", metrix.Labels{"label1": "value1"}), 1e-9)
				_, ok := fr.Value("test_untyped_dropped", metrix.Labels{"label1": "value1"})
				assert.False(t, ok, "an untyped metric with no fallback and no _total suffix must be dropped")
			},
		},
		"selector drops non-matching metrics": {
			prepare: func() *Collector {
				c := New()
				c.Selector = selector.Expr{Allow: []string{"test_keep"}}
				return c
			},
			input: `
# TYPE test_keep gauge
test_keep{label1="value1"} 11
# TYPE test_drop gauge
test_drop{label1="value1"} 22
`,
			want: func(t *testing.T, fr metrix.Reader) {
				assert.InDelta(t, 11, value(t, fr, "test_keep", metrix.Labels{"label1": "value1"}), 1e-9)
				_, ok := fr.Value("test_drop", metrix.Labels{"label1": "value1"})
				assert.False(t, ok, "a metric not matched by the selector must be dropped")
			},
		},
		"_info family is skipped": {
			prepare: New,
			input: `
# TYPE test_metric gauge
test_metric{label1="value1"} 11
# TYPE test_metric_info gauge
test_metric_info{version="1.2.3"} 1
`,
			want: func(t *testing.T, fr metrix.Reader) {
				assert.InDelta(t, 11, value(t, fr, "test_metric", metrix.Labels{"label1": "value1"}), 1e-9)
				_, ok := fr.Value("test_metric_info", metrix.Labels{"version": "1.2.3"})
				assert.False(t, ok, "an _info family must be skipped")
			},
		},
		"per-metric series limit skips the family": {
			prepare: func() *Collector {
				c := New()
				c.MaxTSPerMetric = 1
				return c
			},
			input: `
# TYPE test_gauge_metric gauge
test_gauge_metric{label1="value1"} 11
test_gauge_metric{label1="value2"} 12
`,
			want: func(t *testing.T, fr metrix.Reader) {
				_, ok := fr.Value("test_gauge_metric", metrix.Labels{"label1": "value1"})
				assert.False(t, ok, "a family over the per-metric series limit must be skipped entirely")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tc.input)) }))
			defer srv.Close()

			collr := tc.prepare()
			collr.URL = srv.URL
			require.NoError(t, collr.Init(context.Background()))

			// Drive Collect exactly as the framework does: one store cycle around it.
			cc := cycle(t, collr.MetricStore())
			cc.BeginCycle()
			require.NoError(t, collr.Collect(context.Background()))
			require.NoError(t, cc.CommitCycleSuccess())

			tc.want(t, collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
		})
	}
}

// TestCollector_ChartCoverage verifies the collector's own ChartTemplateYAML() (the per-job
// autogen template built in Init from the configured app) plus the collected store materialize
// the expected chart contexts and dimensions. Unlike the manifest parity test (which builds the
// template directly), this exercises the real CollectorV2.ChartTemplateYAML() method and the
// "prometheus" / "prometheus.<app>" context namespace end-to-end via chartengine autogen.
func TestCollector_ChartCoverage(t *testing.T) {
	tests := map[string]struct {
		prepare func() *Collector
		input   string
		want    map[string][]string
	}{
		"default namespace, scalars and a summary split": {
			prepare: New,
			input: `
# TYPE test_gauge_metric gauge
test_gauge_metric{label1="value1"} 11
# TYPE test_counter_metric_total counter
test_counter_metric_total{label1="value1"} 11
# TYPE test_summary_duration_seconds summary
test_summary_duration_seconds{label1="value1",quantile="0.5"} 0.25
test_summary_duration_seconds{label1="value1",quantile="0.99"} 0.5
test_summary_duration_seconds_sum{label1="value1"} 12.5
test_summary_duration_seconds_count{label1="value1"} 42
`,
			want: map[string][]string{
				"prometheus.test_gauge_metric":                   {"test_gauge_metric"},
				"prometheus.test_counter_metric_total":           {"test_counter_metric_total"},
				"prometheus.test_summary_duration_seconds":       {"quantile_0.5", "quantile_0.99"},
				"prometheus.test_summary_duration_seconds_sum":   {"test_summary_duration_seconds_sum"},
				"prometheus.test_summary_duration_seconds_count": {"test_summary_duration_seconds_count"},
			},
		},
		"app namespace prefixes the context": {
			prepare: func() *Collector { c := New(); c.Application = "myapp"; return c },
			input: `
# TYPE test_gauge_metric gauge
test_gauge_metric{label1="value1"} 11
`,
			want: map[string][]string{
				"prometheus.myapp.test_gauge_metric": {"test_gauge_metric"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tc.input)) }))
			defer srv.Close()

			collr := tc.prepare()
			collr.URL = srv.URL
			require.NoError(t, collr.Init(context.Background()))

			cc := cycle(t, collr.MetricStore())
			cc.BeginCycle()
			require.NoError(t, collr.Collect(context.Background()))
			require.NoError(t, cc.CommitCycleSuccess())

			collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{RequiredContexts: tc.want})
		})
	}
}
