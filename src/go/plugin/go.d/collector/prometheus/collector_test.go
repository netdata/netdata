// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
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
		"valid relabeling block": {
			wantFail: false,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Relabeling: []RelabelBlock{{
					Match:                "app_*",
					MetricRelabelConfigs: []relabel.Config{{SourceLabels: []string{"__name__"}, Regex: relabel.MustNewRegexp("x"), Action: relabel.Drop}},
				}},
			},
		},
		"invalid relabeling match pattern": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Relabeling: []RelabelBlock{{
					Match:                "[a-",
					MetricRelabelConfigs: []relabel.Config{{SourceLabels: []string{"__name__"}, Regex: relabel.MustNewRegexp("x"), Action: relabel.Drop}},
				}},
			},
		},
		"invalid relabeling rule": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Relabeling: []RelabelBlock{{Match: "*", MetricRelabelConfigs: []relabel.Config{{Action: "bogus"}}}},
			},
		},
		"relabeling block with no match": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Relabeling: []RelabelBlock{{MetricRelabelConfigs: []relabel.Config{{SourceLabels: []string{"__name__"}, Regex: relabel.MustNewRegexp("x"), Action: relabel.Drop}}}},
			},
		},
		"relabeling block with no rules": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Relabeling: []RelabelBlock{{Match: "app_*"}},
			},
		},
		"profiles mode none": {
			wantFail: false,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Profiles:   ProfilesConfig{Mode: "none"},
			},
		},
		"profiles mode exact with entries": {
			wantFail: false,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Profiles:   ProfilesConfig{Mode: "exact", ModeExact: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "haproxy"}}}},
			},
		},
		"profiles mode exact without entries": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Profiles:   ProfilesConfig{Mode: "exact"},
			},
		},
		"profiles mode combined with entries": {
			wantFail: false,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Profiles:   ProfilesConfig{Mode: "combined", ModeCombined: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "haproxy"}}}},
			},
		},
		"profiles mode combined without entries": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Profiles:   ProfilesConfig{Mode: "combined"},
			},
		},
		"profiles unknown mode": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Profiles:   ProfilesConfig{Mode: "bogus"},
			},
		},
		"profiles duplicate entries": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Profiles:   ProfilesConfig{Mode: "exact", ModeExact: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "haproxy"}, {Name: "haproxy"}}}},
			},
		},
		"profiles invalid entry name": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Profiles:   ProfilesConfig{Mode: "exact", ModeExact: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "HAProxy"}}}},
			},
		},
		"profiles entry empty name": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Profiles:   ProfilesConfig{Mode: "exact", ModeExact: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "  "}}}},
			},
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
		"relabel applies before assembly (drop + rename via __name__)": {
			prepare: func() *Collector {
				c := New()
				c.Relabeling = []RelabelBlock{{Match: "*", MetricRelabelConfigs: []relabel.Config{
					{
						SourceLabels: []string{"__name__"},
						Regex:        relabel.MustNewRegexp("test_drop_me"),
						Action:       relabel.Drop,
					},
					{
						SourceLabels: []string{"__name__"},
						Regex:        relabel.MustNewRegexp("test_(.+)"),
						TargetLabel:  "__name__",
						Replacement:  "renamed_${1}",
						Action:       relabel.Replace,
					},
				}}}
				return c
			},
			input: `
# TYPE test_keep gauge
test_keep{label1="value1"} 11
# TYPE test_drop_me gauge
test_drop_me{label1="value1"} 22
`,
			want: func(t *testing.T, fr metrix.Reader) {
				assert.InDelta(t, 11, value(t, fr, "renamed_keep", metrix.Labels{"label1": "value1"}), 1e-9)
				_, ok := fr.Value("test_keep", metrix.Labels{"label1": "value1"})
				assert.False(t, ok, "the renamed metric must not appear under its original name")
				_, ok = fr.Value("test_drop_me", metrix.Labels{"label1": "value1"})
				assert.False(t, ok, "the drop rule must drop test_drop_me before assembly")
				_, ok = fr.Value("renamed_drop_me", metrix.Labels{"label1": "value1"})
				assert.False(t, ok, "a dropped sample must not be renamed or assembled")
			},
		},
		"relabel rewrites a regular label (copy via Replace)": {
			prepare: func() *Collector {
				c := New()
				c.Relabeling = []RelabelBlock{{Match: "*", MetricRelabelConfigs: []relabel.Config{
					{
						SourceLabels: []string{"method"},
						Regex:        relabel.MustNewRegexp("(.+)"),
						TargetLabel:  "verb",
						Replacement:  "${1}",
						Action:       relabel.Replace,
					},
				}}}
				return c
			},
			input: `
# TYPE test_requests_total counter
test_requests_total{method="get"} 5
`,
			want: func(t *testing.T, fr metrix.Reader) {
				// Replace copies method -> verb; the series carries both labels.
				assert.InDelta(t, 5, value(t, fr, "test_requests_total", metrix.Labels{"method": "get", "verb": "get"}), 1e-9)
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
// autogen template built at Check from the configured app) plus the collected store materialize
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
			require.NoError(t, collr.Check(context.Background()))

			cc := cycle(t, collr.MetricStore())
			cc.BeginCycle()
			require.NoError(t, collr.Collect(context.Background()))
			require.NoError(t, cc.CommitCycleSuccess())

			collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{RequiredContexts: tc.want})
		})
	}
}

// TestCollector_HAProxyProfile exercises the full profile path against the stock
// haproxy profile: selection (auto/exact/combined select it; none falls back to
// autogen) and the curated charts rendering under the per-app namespace, including
// the label-split charts (status by `state`, http responses by `code`).
func TestCollector_HAProxyProfile(t *testing.T) {
	const input = `
# TYPE haproxy_frontend_status gauge
haproxy_frontend_status{proxy="http",state="UP"} 1
# TYPE haproxy_frontend_current_sessions gauge
haproxy_frontend_current_sessions{proxy="http"} 5
haproxy_frontend_current_sessions{proxy="https"} 12
# TYPE haproxy_frontend_sessions_total counter
haproxy_frontend_sessions_total{proxy="http"} 100
# TYPE haproxy_frontend_bytes_in_total counter
haproxy_frontend_bytes_in_total{proxy="http"} 1000
# TYPE haproxy_frontend_bytes_out_total counter
haproxy_frontend_bytes_out_total{proxy="http"} 2000
# TYPE haproxy_frontend_http_requests_total counter
haproxy_frontend_http_requests_total{proxy="http"} 50
# TYPE haproxy_frontend_http_responses_total counter
haproxy_frontend_http_responses_total{proxy="http",code="2xx"} 40
haproxy_frontend_http_responses_total{proxy="http",code="5xx"} 2
# TYPE haproxy_backend_current_sessions gauge
haproxy_backend_current_sessions{proxy="app"} 3
# TYPE haproxy_backend_sessions_total counter
haproxy_backend_sessions_total{proxy="app"} 70
# TYPE haproxy_backend_current_queue gauge
haproxy_backend_current_queue{proxy="app"} 1
# TYPE haproxy_backend_bytes_in_total counter
haproxy_backend_bytes_in_total{proxy="app"} 500
# TYPE haproxy_backend_bytes_out_total counter
haproxy_backend_bytes_out_total{proxy="app"} 800
# TYPE haproxy_backend_response_time_average_seconds gauge
haproxy_backend_response_time_average_seconds{proxy="app"} 0.05
`
	curated := map[string][]string{
		"prometheus.haproxy.frontend_status":           {"UP"},
		"prometheus.haproxy.frontend_current_sessions": {"current"},
		"prometheus.haproxy.frontend_sessions":         {"sessions"},
		"prometheus.haproxy.frontend_traffic":          {"received", "sent"},
		"prometheus.haproxy.frontend_http_requests":    {"requests"},
		"prometheus.haproxy.frontend_http_responses":   {"2xx", "5xx"},
		"prometheus.haproxy.backend_current_sessions":  {"current"},
		"prometheus.haproxy.backend_sessions":          {"sessions"},
		"prometheus.haproxy.backend_queue":             {"queued"},
		"prometheus.haproxy.backend_traffic":           {"received", "sent"},
		"prometheus.haproxy.backend_response_time":     {"response"},
	}

	tests := map[string]struct {
		profiles ProfilesConfig
		want     map[string][]string
	}{
		"auto mode selects haproxy": {
			profiles: ProfilesConfig{Mode: "auto"},
			want:     curated,
		},
		"exact mode selects haproxy": {
			profiles: ProfilesConfig{Mode: "exact", ModeExact: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "haproxy"}}}},
			want:     curated,
		},
		"combined mode selects haproxy": {
			profiles: ProfilesConfig{Mode: "combined", ModeCombined: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "haproxy"}}}},
			want:     curated,
		},
		"none mode falls back to autogen": {
			profiles: ProfilesConfig{Mode: "none"},
			want: map[string][]string{
				"prometheus.haproxy_frontend_current_sessions": {"haproxy_frontend_current_sessions"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(input)) }))
			defer srv.Close()

			collr := New()
			collr.URL = srv.URL
			collr.Profiles = tc.profiles
			require.NoError(t, collr.Init(context.Background()))
			require.NoError(t, collr.Check(context.Background()))

			cc := cycle(t, collr.MetricStore())
			cc.BeginCycle()
			require.NoError(t, collr.Collect(context.Background()))
			require.NoError(t, cc.CommitCycleSuccess())

			collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{RequiredContexts: tc.want})
		})
	}
}

// TestCollector_HAProxyProfileAllMetrics feeds the synthetic full HAProxy scrape
// (every source-derived family, incl. ?extra-counters) through the haproxy profile
// in auto mode and proves the merged template (autogen + profile groups) accepts the
// whole set: it compiles and plans with no error and produces no chart-ID collision.
// It also checks that a representative curated context from every scope materializes
// (process/frontend/listener/backend/server/resolver/sticktable + the state/code
// label-split charts). Synthetic placeholder values are not asserted.
func TestCollector_HAProxyProfileAllMetrics(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "haproxy_all_metrics.prom"))
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(input) }))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	collr.Profiles = ProfilesConfig{Mode: "auto"}
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	cc := cycle(t, collr.MetricStore())
	cc.BeginCycle()
	require.NoError(t, collr.Collect(context.Background()))
	require.NoError(t, cc.CommitCycleSuccess())

	// The merged template (autogen base + haproxy profile groups) must compile and
	// plan over the whole scraped set without error, and every materialized chart ID
	// must be unique (curated and autogen contexts share no derived chart ID).
	eng, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, eng.LoadYAML([]byte(collr.ChartTemplateYAML()), 1))

	attempt, err := eng.PreparePlan(collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	defer attempt.Abort()
	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())

	seenChartIDs := make(map[string]string)
	for _, a := range plan.Actions {
		create, ok := a.(chartengine.CreateChartAction)
		if !ok {
			continue
		}
		if prev, dup := seenChartIDs[create.ChartID]; dup {
			t.Fatalf("chart-ID collision %q: contexts %q and %q", create.ChartID, prev, create.Meta.Context)
		}
		seenChartIDs[create.ChartID] = create.Meta.Context
	}
	assert.NotEmpty(t, seenChartIDs, "the merged template must materialize charts")

	// One representative curated context per scope, including the label-split charts.
	curated := map[string][]string{
		"prometheus.haproxy.process_connections":       {"connections"},
		"prometheus.haproxy.frontend_status":           {"UP", "DOWN"},
		"prometheus.haproxy.frontend_http_responses":   {"1xx", "2xx", "3xx", "4xx", "5xx", "other"},
		"prometheus.haproxy.listener_current_sessions": {"current"},
		"prometheus.haproxy.backend_status":            {"UP", "DOWN"},
		"prometheus.haproxy.server_current_sessions":   {"current"},
		"prometheus.haproxy.resolver_events":           {"sent", "valid"},
		"prometheus.haproxy.sticktable_entries":        {"used", "size"},
	}
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{RequiredContexts: curated})
}

// TestCollector_ProfileSelectionErrors covers the exact/combined contract: a named
// profile that does not exist, or that matches no scraped metric, fails Check.
func TestCollector_ProfileSelectionErrors(t *testing.T) {
	// No haproxy_* metrics, so the haproxy profile resolves but matches nothing.
	const nonHAProxyInput = "# TYPE up gauge\nup 1\n"

	tests := map[string]struct {
		input    string
		profiles ProfilesConfig
	}{
		"exact names an unknown profile": {
			input:    nonHAProxyInput,
			profiles: ProfilesConfig{Mode: "exact", ModeExact: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "does_not_exist"}}}},
		},
		"exact profile matches no scraped metric": {
			input:    nonHAProxyInput,
			profiles: ProfilesConfig{Mode: "exact", ModeExact: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "haproxy"}}}},
		},
		"combined names an unknown profile": {
			input:    nonHAProxyInput,
			profiles: ProfilesConfig{Mode: "combined", ModeCombined: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "does_not_exist"}}}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tc.input)) }))
			defer srv.Close()

			collr := New()
			collr.URL = srv.URL
			collr.Profiles = tc.profiles
			require.NoError(t, collr.Init(context.Background()))
			assert.Error(t, collr.Check(context.Background()))
		})
	}
}
