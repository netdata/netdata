// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
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
				Relabeling: []relabel.Block{{
					Match:                "app_*",
					MetricRelabelConfigs: []relabel.Config{{SourceLabels: []string{"__name__"}, Regex: relabel.MustNewRegexp("x"), Action: relabel.Drop}},
				}},
			},
		},
		"invalid relabeling match pattern": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Relabeling: []relabel.Block{{
					Match:                "[a-",
					MetricRelabelConfigs: []relabel.Config{{SourceLabels: []string{"__name__"}, Regex: relabel.MustNewRegexp("x"), Action: relabel.Drop}},
				}},
			},
		},
		"invalid relabeling rule": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Relabeling: []relabel.Block{{Match: "*", MetricRelabelConfigs: []relabel.Config{{Action: "bogus"}}}},
			},
		},
		"relabeling block with no match": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Relabeling: []relabel.Block{{MetricRelabelConfigs: []relabel.Config{{SourceLabels: []string{"__name__"}, Regex: relabel.MustNewRegexp("x"), Action: relabel.Drop}}}},
			},
		},
		"relabeling block with no rules": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				Relabeling: []relabel.Block{{Match: "app_*"}},
			},
		},
		"fallback type blank pattern": {
			wantFail: true,
			config: Config{
				HTTPConfig:   web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				FallbackType: promprofiles.FallbackType{Gauge: []string{"   "}},
			},
		},
		"fallback type padded pattern": {
			wantFail: true,
			config: Config{
				HTTPConfig:   web.HTTPConfig{RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:9090/metric"}},
				FallbackType: promprofiles.FallbackType{Counter: []string{" app_requests "}},
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

// TestCollector_Collect drives the real V2 collector (Init → Check, then a framework-style
// store cycle around Collect) and asserts the metrics it wrote into the metrix store, by metric
// name + flattened labels. Per-type correctness is exercised exhaustively in writer_test.go;
// this checks the collector's end-to-end wiring (client/selector/fallback built in Init →
// scrape → writer → store) plus the config-driven behaviors.
func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare    func() *Collector
		checkInput string
		input      string
		want       func(t *testing.T, fr metrix.Reader)
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
				assert.InDelta(t, 2, value(t, fr, "test_dur_bucket", metrix.Labels{"le": "+Inf"}), 1e-9)
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
			checkInput: `
# TYPE test_gauge_metric gauge
test_gauge_metric{label1="value1"} 11
`,
			want: func(t *testing.T, fr metrix.Reader) {
				_, ok := fr.Value("test_gauge_metric", metrix.Labels{"label1": "value1"})
				assert.False(t, ok, "a family over the per-metric series limit must be skipped entirely")
			},
		},
		"relabel applies before assembly (drop + rename via __name__)": {
			prepare: func() *Collector {
				c := New()
				c.Relabeling = []relabel.Block{{Match: "*", MetricRelabelConfigs: []relabel.Config{
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
				c.Relabeling = []relabel.Block{{Match: "*", MetricRelabelConfigs: []relabel.Config{
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
			currentInput := tc.checkInput
			if currentInput == "" {
				currentInput = tc.input
			}
			srv := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(currentInput)) }))
			defer srv.Close()

			collr := tc.prepare()
			collr.URL = srv.URL
			require.NoError(t, collr.Init(context.Background()))
			require.NoError(t, collr.Check(context.Background()))
			currentInput = tc.input

			// Drive Collect exactly as the framework does after successful Check.
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

	// curatedUnder reprefixes the curated contexts with an explicit <app> segment: it models
	// a job whose app is set (by config, or automatically by service discovery in k8s) to
	// something other than the profile's own app:. The resolved app becomes the <app> segment
	// and the profile's context_namespace (haproxy) is KEPT as the exporter-type sub-segment
	// (prometheus.<app>.haproxy.<context>) — the dedup's keep branch.
	curatedUnder := func(app string) map[string][]string {
		out := make(map[string][]string, len(curated))
		for ctx, dims := range curated {
			out[strings.Replace(ctx, "prometheus.", "prometheus."+app+".", 1)] = dims
		}
		return out
	}

	tests := map[string]struct {
		configApp string
		profiles  ProfilesConfig
		want      map[string][]string
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
		"a configured app keeps the profile namespace as a sub-segment": {
			configApp: "myproxy",
			profiles:  ProfilesConfig{Mode: "auto"},
			want:      curatedUnder("myproxy"),
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
			collr.Application = tc.configApp
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

	// Every materialized context — curated and autogen-fallback alike — must carry the
	// resolved app segment from the profile's app: (prometheus.haproxy.<leaf>). The trailing
	// dot is load-bearing: if app: were ignored, autogen would emit prometheus.haproxy_<metric>
	// (the metric name, no app segment), which this prefix check rejects.
	for chartID, ctx := range seenChartIDs {
		assert.Truef(t, strings.HasPrefix(ctx, "prometheus.haproxy."),
			"context %q (chart %q) is missing the prometheus.haproxy. app segment", ctx, chartID)
	}

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

func testCollectorStockProfileModes(
	t *testing.T,
	profileName string,
	input string,
	curated map[string][]string,
	autogen map[string][]string,
) {
	t.Helper()

	tests := []struct {
		name     string
		profiles ProfilesConfig
		want     map[string][]string
	}{
		{
			name:     "auto mode selects " + profileName,
			profiles: ProfilesConfig{Mode: "auto"},
			want:     curated,
		},
		{
			name:     "exact mode selects " + profileName,
			profiles: ProfilesConfig{Mode: "exact", ModeExact: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: profileName}}}},
			want:     curated,
		},
		{
			name:     "combined mode selects " + profileName,
			profiles: ProfilesConfig{Mode: "combined", ModeCombined: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: profileName}}}},
			want:     curated,
		},
		{
			name:     "none mode falls back to autogen",
			profiles: ProfilesConfig{Mode: "none"},
			want:     autogen,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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

func TestCollector_VLLMProfile(t *testing.T) {
	const input = `
# TYPE vllm:num_requests_running gauge
vllm:num_requests_running{engine="0",model_name="example-model"} 1
`
	testCollectorStockProfileModes(
		t,
		"vllm",
		input,
		map[string][]string{"prometheus.vllm.scheduler.request_state": {"running"}},
		map[string][]string{"prometheus.vllm:num_requests_running": {"vllm:num_requests_running"}},
	)
}

func TestCollector_LiteLLMProfile(t *testing.T) {
	const input = `
# TYPE litellm_in_flight_requests gauge
litellm_in_flight_requests 1
`
	testCollectorStockProfileModes(
		t,
		"litellm",
		input,
		map[string][]string{"prometheus.litellm.gateway.in_flight_requests": {"requests"}},
		map[string][]string{"prometheus.litellm_in_flight_requests": {"litellm_in_flight_requests"}},
	)
}

func TestCollector_CephProfile(t *testing.T) {
	const input = `
# TYPE ceph_health_status gauge
ceph_health_status 0
`
	testCollectorStockProfileModes(
		t,
		"ceph",
		input,
		map[string][]string{"prometheus.ceph.cluster_mgr.health.cluster_status.state": {"value"}},
		map[string][]string{"prometheus.ceph_health_status": {"ceph_health_status"}},
	)
}

// TestCollector_VLLMProfileAllMetrics proves the stock profile against a
// sanitized structural union, including optional and mutually exclusive
// source-defined surfaces and each distinct entity identity.
func TestCollector_VLLMProfileAllMetrics(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("testdata", "vllm_all_metrics.prom"))
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(input) }))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	collr.Selector = selector.Expr{Deny: []string{
		"*_created",
		"process_start_time_seconds",
		"vllm:kv_offload_total_bytes_total",
		"vllm:kv_offload_total_time_total",
		"vllm:kv_offload_size*",
	}}
	collr.Profiles = ProfilesConfig{Mode: "auto"}
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	cc := cycle(t, collr.MetricStore())
	cc.BeginCycle()
	require.NoError(t, collr.Collect(context.Background()))
	require.NoError(t, cc.CommitCycleSuccess())

	eng, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, eng.LoadYAML([]byte(collr.ChartTemplateYAML()), 1))

	attempt, err := eng.PreparePlan(collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	defer attempt.Abort()
	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())

	seenChartIDs := make(map[string]string)
	identityCounts := map[string]int{
		"prometheus.vllm.request_lifecycle.outcomes":             0,
		"prometheus.vllm.kv_offloading.transfer_operations":      0,
		"prometheus.vllm.mooncake_connector.volume.keys":         0,
		"prometheus.vllm.diffusion_decoding.denoising_steps":     0,
		"prometheus.vllm.websocket_service.connection_lifecycle": 0,
		"prometheus.vllm.http_endpoints.request_outcomes":        0,
		"prometheus.vllm.tool_parsing.invocations":               0,
		"prometheus.vllm.runtime.process_cpu":                    0,
	}
	for _, action := range plan.Actions {
		create, ok := action.(chartengine.CreateChartAction)
		if !ok {
			continue
		}
		if prev, dup := seenChartIDs[create.ChartID]; dup {
			t.Fatalf("chart-ID collision %q: contexts %q and %q", create.ChartID, prev, create.Meta.Context)
		}
		seenChartIDs[create.ChartID] = create.Meta.Context
		assert.Truef(t, strings.HasPrefix(create.Meta.Context, "prometheus.vllm."),
			"context %q (chart %q) is missing the prometheus.vllm. app segment", create.Meta.Context, create.ChartID)

		switch create.Meta.Context {
		case "prometheus.vllm.request_lifecycle.outcomes":
			assert.Equal(t, "example-model", create.Labels["model_name"])
			assert.Equal(t, "0", create.Labels["engine"])
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.kv_offloading.transfer_operations":
			assert.Equal(t, "example-model", create.Labels["model_name"])
			assert.Equal(t, "0", create.Labels["engine"])
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.mooncake_connector.volume.keys":
			assert.Equal(t, "example-model", create.Labels["model_name"])
			assert.Equal(t, "0", create.Labels["engine"])
			assert.NotEmpty(t, create.Labels["operation"])
			assert.NotContains(t, create.Labels, "status")
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.diffusion_decoding.denoising_steps":
			assert.Equal(t, "example-model", create.Labels["model_name"])
			assert.Equal(t, "0", create.Labels["engine"])
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.websocket_service.connection_lifecycle":
			assert.NotContains(t, create.Labels, "model_name")
			assert.NotContains(t, create.Labels, "engine")
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.http_endpoints.request_outcomes":
			assert.NotEmpty(t, create.Labels["handler"])
			assert.NotEmpty(t, create.Labels["method"])
			assert.NotContains(t, create.Labels, "model_name")
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.tool_parsing.invocations":
			assert.Equal(t, "example-model", create.Labels["model_name"])
			assert.NotEmpty(t, create.Labels["request_type"])
			assert.NotEmpty(t, create.Labels["mode"])
			assert.NotContains(t, create.Labels, "engine")
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.runtime.process_cpu":
			assert.NotContains(t, create.Labels, "model_name")
			assert.NotContains(t, create.Labels, "engine")
			identityCounts[create.Meta.Context]++
		}
	}
	assert.NotEmpty(t, seenChartIDs)
	assert.Equal(t, 1, identityCounts["prometheus.vllm.request_lifecycle.outcomes"])
	assert.Equal(t, 1, identityCounts["prometheus.vllm.kv_offloading.transfer_operations"])
	assert.Equal(t, 2, identityCounts["prometheus.vllm.mooncake_connector.volume.keys"])
	assert.Equal(t, 1, identityCounts["prometheus.vllm.diffusion_decoding.denoising_steps"])
	assert.Equal(t, 1, identityCounts["prometheus.vllm.websocket_service.connection_lifecycle"])
	assert.Equal(t, 2, identityCounts["prometheus.vllm.http_endpoints.request_outcomes"])
	assert.Equal(t, 6, identityCounts["prometheus.vllm.tool_parsing.invocations"])
	assert.Equal(t, 1, identityCounts["prometheus.vllm.runtime.process_cpu"])

	curated := map[string][]string{
		"prometheus.vllm.request_lifecycle.outcomes":                  {"stop", "length", "abort", "error", "repetition"},
		"prometheus.vllm.request_lifecycle.corrupted_requests":        {"corrupted"},
		"prometheus.vllm.scheduler.waiting_by_reason":                 {"capacity", "deferred"},
		"prometheus.vllm.prefill.prompt_tokens_by_source":             {"local_compute", "local_cache_hit", "external_kv_transfer"},
		"prometheus.vllm.decode.inter_token_intervals":                {"intervals"},
		"prometheus.vllm.engine_execution.estimated_memory_bandwidth": {"read", "write"},
		"prometheus.vllm.kv_cache.local_prefix":                       {"queries", "hits"},
		"prometheus.vllm.kv_cache_residency.measurements":             {"lifetimes", "idle_periods", "reuse_gaps"},
		"prometheus.vllm.kv_offloading.cpu_cache_usage":               {"total", "writes", "reads"},
		"prometheus.vllm.nixl_connector.failures":                     {"transfers", "notifications"},
		"prometheus.vllm.hf3fs_connector.failures":                    {"saves", "loads"},
		"prometheus.vllm.mooncake_connector.volume.keys":              {"ok", "error"},
		"prometheus.vllm.speculative_decoding.accepted_by_position":   {"0", "1"},
		"prometheus.vllm.diffusion_decoding.committed_tokens":         {"committed"},
		"prometheus.vllm.websocket_service.connection_lifecycle":      {"opened", "closed"},
		"prometheus.vllm.http_endpoints.request_outcomes":             {"2xx"},
		"prometheus.vllm.http_service.request_measurements":           {"requests"},
		"prometheus.vllm.tool_parsing.invocations":                    {"tool_call", "no_tool_call"},
		"prometheus.vllm.runtime.process_cpu":                         {"used"},
		"prometheus.vllm.runtime.python_gc.collections":               {"0", "1", "2"},
	}
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{RequiredContexts: curated})
}

func testCollectorStockProfileAllMetrics(
	t *testing.T,
	fixture string,
	configure func(*Collector),
	contextPrefix string,
	requiredContexts map[string][]string,
) {
	t.Helper()

	input, err := os.ReadFile(filepath.Join("testdata", fixture))
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(input) }))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	collr.Profiles = ProfilesConfig{Mode: "auto"}
	if configure != nil {
		configure(collr)
	}
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	cc := cycle(t, collr.MetricStore())
	cc.BeginCycle()
	require.NoError(t, collr.Collect(context.Background()))
	require.NoError(t, cc.CommitCycleSuccess())

	eng, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, eng.LoadYAML([]byte(collr.ChartTemplateYAML()), 1))

	attempt, err := eng.PreparePlan(collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	defer attempt.Abort()
	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())

	seenChartIDs := make(map[string]string)
	for _, action := range plan.Actions {
		create, ok := action.(chartengine.CreateChartAction)
		if !ok {
			continue
		}
		if prev, dup := seenChartIDs[create.ChartID]; dup {
			t.Fatalf("chart-ID collision %q: contexts %q and %q", create.ChartID, prev, create.Meta.Context)
		}
		seenChartIDs[create.ChartID] = create.Meta.Context
		assert.Truef(t, strings.HasPrefix(create.Meta.Context, contextPrefix),
			"context %q (chart %q) is missing prefix %q", create.Meta.Context, create.ChartID, contextPrefix)
	}
	assert.NotEmpty(t, seenChartIDs)

	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{RequiredContexts: requiredContexts})
}

func TestCollector_LiteLLMProfileAllMetrics(t *testing.T) {
	testCollectorStockProfileAllMetrics(
		t,
		"litellm_all_metrics.prom",
		func(collr *Collector) {
			collr.Selector = selector.Expr{Deny: []string{
				"*_created",
				"litellm_requests_metric_total",
				"litellm_llm_api_failed_requests_metric_total",
				"litellm_check_batch_cost_last_run_timestamp",
			}}
			collr.MaxTS = 20000
			collr.MaxTSPerMetric = 2000
		},
		"prometheus.litellm.",
		map[string][]string{
			"prometheus.litellm.gateway.in_flight_requests":                      {"requests"},
			"prometheus.litellm.routing_and_deployments.deployment_state":        {"state"},
			"prometheus.litellm.provider_api.accumulated_provider_latency":       {"api_latency"},
			"prometheus.litellm.usage_and_cost.request_token_throughput":         {"total"},
			"prometheus.litellm.caching.cache_outcomes":                          {"hits"},
			"prometheus.litellm.governance.team_budget":                          {"team_remaining"},
			"prometheus.litellm.guardrails.guardrail_invocations_by_status":      {"success", "failure"},
			"prometheus.litellm.mcp_gateway.mcp_tool_calls":                      {"calls"},
			"prometheus.litellm.managed_batch_and_files.managed_batches_created": {"created"},
			"prometheus.litellm.callbacks_and_inventory.user_inventory":          {"total"},
			"prometheus.litellm.internal_services.internal_service_requests":     {"redis"},
		},
	)
}

func TestCollector_CephProfileAllMetrics(t *testing.T) {
	testCollectorStockProfileAllMetrics(
		t,
		"ceph_all_metrics.prom",
		func(collr *Collector) {
			collr.Selector = selector.Expr{Deny: []string{"ceph_objecter_0x*"}}
			collr.FallbackType.Gauge = []string{"ceph_*"}
			collr.MaxTS = 10000
			collr.MaxTSPerMetric = 2000
		},
		"prometheus.ceph.",
		map[string][]string{
			"prometheus.ceph.cluster_mgr.health.cluster_status.state":                           {"value"},
			"prometheus.ceph.cluster_mgr.pools.capacity_and_quotas.space":                       {"bytes_used"},
			"prometheus.ceph.daemon_exporters.exporter_and_process_runtime.state":               {"value"},
			"prometheus.ceph.daemon_exporters.object_gateway.operations.operations":             {"get_obj_ops"},
			"prometheus.ceph.daemon_exporters.osd_scrubbing_ceph_19.outcomes.scrub_work":        {"dp_repl_failed_scrubs"},
			"prometheus.ceph.daemon_exporters.osd_scrubbing_ceph_20.outcomes.scrub_work":        {"failed_scrubs_replicated"},
			"prometheus.ceph.daemon_exporters.cephfs_mds.request_handling_and_state.operations": {"request"},
			"prometheus.ceph.daemon_exporters.rbd_mirror.journal.byte_throughput":               {"value"},
			"prometheus.ceph.nvme_of.block_devices.byte_throughput":                             {"read_bytes"},
		},
	)
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
