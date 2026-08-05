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

	"github.com/netdata/netdata/go/plugins/internal/promtestdata"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"gopkg.in/yaml.v3"
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

func TestDefaultConfigMatchesNewAndReturnsIndependentValues(t *testing.T) {
	first := DefaultConfig()
	second := DefaultConfig()
	assert.Equal(t, New().Config, first)

	first.Profiles.Mode = "none"
	assert.Equal(t, "auto", second.Profiles.Mode)
	assert.Equal(t, 2000, second.MaxTS)
	assert.Equal(t, 200, second.MaxTSPerMetric)
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

func requireAutogenOnlyRuntime(t *testing.T, collr *Collector) {
	t.Helper()

	require.NotNil(t, collr.runtime)
	require.Empty(t, collr.runtime.profiles, "profiles.mode none must not retain a selected profile")
	want, err := buildMergedChartTemplate(collr.resolveApp(nil), nil)
	require.NoError(t, err)
	require.Equal(t, want, collr.runtime.chartTemplate, "profiles.mode none must use the pure autogen template")
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
			if tc.profiles.effectiveMode() == profilesModeNone {
				requireAutogenOnlyRuntime(t, collr)
			}

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
			if tc.profiles.effectiveMode() == profilesModeNone {
				requireAutogenOnlyRuntime(t, collr)
			}

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

func TestCollector_VLLMRayProfile(t *testing.T) {
	const input = `
# TYPE ray_vllm_num_requests_running gauge
ray_vllm_num_requests_running{Component="core_worker",ReplicaId="replica-a",SessionName="session-a",Version="2.48.0",WorkerId="worker-a",engine="0",model_name="example-model"} 1
`
	testCollectorStockProfileModes(
		t,
		"vllm_ray",
		input,
		map[string][]string{"prometheus.vllm.scheduler.request_state": {"running"}},
		map[string][]string{"prometheus.ray_vllm_num_requests_running": {"ray_vllm_num_requests_running"}},
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

func configureProfileJobFromMetadata(t *testing.T, collr *Collector, integrationID, profileName, jobName string) {
	t.Helper()

	var metadata struct {
		Modules []struct {
			Meta struct {
				ID string `yaml:"id"`
			} `yaml:"meta"`
			Setup struct {
				Configuration struct {
					Examples struct {
						List []struct {
							Config string `yaml:"config"`
						} `yaml:"list"`
					} `yaml:"examples"`
				} `yaml:"configuration"`
			} `yaml:"setup"`
		} `yaml:"modules"`
	}
	content, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(content, &metadata))

	var config *Config
	for _, module := range metadata.Modules {
		if module.Meta.ID != integrationID {
			continue
		}
		require.NotEmpty(t, module.Setup.Configuration.Examples.List)
		for _, item := range module.Setup.Configuration.Examples.List {
			var example struct {
				Jobs []yaml.Node `yaml:"jobs"`
			}
			require.NoError(t, yaml.Unmarshal([]byte(item.Config), &example))
			for idx := range example.Jobs {
				candidate := New().Config
				require.NoError(t, example.Jobs[idx].Decode(&candidate))
				if candidate.Name == jobName {
					config = &candidate
					break
				}
			}
			if config != nil {
				break
			}
		}
		break
	}
	require.NotNilf(t, config, "metadata integration %q has no job %q", integrationID, jobName)
	require.Equal(t, ProfilesConfig{
		Mode:      "exact",
		ModeExact: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: profileName}}},
	}, config.Profiles)

	config.URL = collr.URL
	collr.Config = *config
}

// TestCollector_VLLMProfileAllMetrics proves the stock profile against a
// sanitized structural union, including optional and mutually exclusive
// source-defined surfaces and each distinct entity identity.
func TestCollector_VLLMProfileAllMetrics(t *testing.T) {
	fixture := promtestdata.Require(t, "prometheus/profiles/vllm/fixtures/vllm_all_metrics.prom")
	input, err := os.ReadFile(fixture)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(input) }))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	configureProfileJobFromMetadata(t, collr, "collector-go.d.plugin-prometheus-vllm", "vllm", "vllm")
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	cc := cycle(t, collr.MetricStore())
	cc.BeginCycle()
	require.NoError(t, collr.Collect(context.Background()))
	require.NoError(t, cc.CommitCycleSuccess())

	var sawCanonicalOffload bool
	collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()).ForEachSeries(
		func(name string, _ metrix.LabelView, _ metrix.SampleValue) {
			assert.Falsef(t, strings.HasSuffix(name, "_created"), "job selector retained generated timestamp %q", name)
			assert.NotEqual(t, "process_start_time_seconds", name)
			assert.NotEqual(t, "vllm:kv_offload_total_bytes_total", name)
			assert.NotEqual(t, "vllm:kv_offload_total_time_total", name)
			assert.Falsef(t, strings.HasPrefix(name, "vllm:kv_offload_size"), "job selector retained deprecated offload family %q", name)
			if name == "vllm:kv_offload_store_bytes_total" {
				sawCanonicalOffload = true
			}
		})
	assert.True(t, sawCanonicalOffload, "job selector must retain canonical CPU-offload counters")

	eng, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, eng.LoadYAML([]byte(collr.ChartTemplateYAML()), 1))

	attempt, err := eng.PreparePlan(collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	defer attempt.Abort()
	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())
	runtimeReader := eng.RuntimeStore().Read(metrix.ReadRaw())
	for _, metric := range []string{"series_autogen_matched_total", "series_unmatched_total"} {
		name := "netdata.go.plugin.framework.chartengine." + metric
		value, ok := runtimeReader.Value(name, nil)
		require.Truef(t, ok, "chartengine runtime metric %q is missing", name)
		assert.Zerof(t, value, "source-complete stock fixture must leave %s at zero", metric)
	}

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
	inspectStore func(metrix.Reader),
	inspectPlan func(chartengine.Plan),
) {
	t.Helper()

	input, err := os.ReadFile(promtestdata.Require(t, fixture))
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
	reader := collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten())
	if inspectStore != nil {
		inspectStore(reader)
	}

	eng, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, eng.LoadYAML([]byte(collr.ChartTemplateYAML()), 1))

	attempt, err := eng.PreparePlan(reader)
	require.NoError(t, err)
	defer attempt.Abort()
	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())
	runtimeReader := eng.RuntimeStore().Read(metrix.ReadRaw())
	for _, metric := range []string{"series_autogen_matched_total", "series_unmatched_total"} {
		name := "netdata.go.plugin.framework.chartengine." + metric
		value, ok := runtimeReader.Value(name, nil)
		require.Truef(t, ok, "chartengine runtime metric %q is missing", name)
		assert.Zerof(t, value, "source-complete stock fixture must leave %s at zero", metric)
	}

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
	if inspectPlan != nil {
		inspectPlan(plan)
	}

	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{RequiredContexts: requiredContexts})
}

// TestCollector_VLLMRayProfileAllMetrics proves the Ray transport profile
// against the source-derived vLLM/Ray structural union. The job selector
// removes Ray's deprecated unsuffixed counter aliases and pre-canonical
// KV-offload duplicates so canonical counters are represented exactly once.
func TestCollector_VLLMRayProfileAllMetrics(t *testing.T) {
	testCollectorStockProfileAllMetrics(
		t,
		"prometheus/profiles/vllm_ray/fixtures/vllm_ray_all_metrics.prom",
		func(collr *Collector) {
			configureProfileJobFromMetadata(t, collr, "collector-go.d.plugin-prometheus-vllm", "vllm_ray", "vllm-ray")
		},
		"prometheus.vllm.",
		map[string][]string{
			"prometheus.vllm.scheduler.request_state":                   {"running", "waiting"},
			"prometheus.vllm.request_lifecycle.outcomes":                {"stop", "length", "abort", "error", "repetition"},
			"prometheus.vllm.kv_offloading.transfer_bytes":              {"loaded", "loaded_observed", "stored", "stored_observed"},
			"prometheus.vllm.kv_offloading.admission_outcomes":          {"allocation_failures", "stores_skipped"},
			"prometheus.vllm.speculative_decoding.token_outcomes":       {"proposed", "accepted"},
			"prometheus.vllm.speculative_decoding.accepted_by_position": {"0", "1"},
			"prometheus.vllm.diffusion_decoding.committed_tokens":       {"committed"},
			"prometheus.vllm.nixl_connector.failures":                   {"transfers", "notifications"},
			"prometheus.vllm.hf3fs_connector.failures":                  {"saves", "loads"},
			"prometheus.vllm.mooncake_connector.volume.failed_keys":     {"ok", "error"},
		},
		func(reader metrix.Reader) {
			var sawCanonicalCounter bool
			reader.ForEachSeries(func(name string, _ metrix.LabelView, _ metrix.SampleValue) {
				assert.NotEqual(t, "ray_vllm_request_success", name, "Ray compatibility gauge survived the job selector")
				assert.Falsef(t, strings.HasPrefix(name, "ray_vllm_kv_offload_size_"),
					"deprecated Ray KV-offload histogram component %q survived the job selector", name)
				if name == "ray_vllm_request_success_total" {
					sawCanonicalCounter = true
				}
			})
			assert.True(t, sawCanonicalCounter, "Ray job selector must retain canonical _total counters")
		},
		func(plan chartengine.Plan) {
			const byteContext = "prometheus.vllm.kv_offloading.transfer_bytes"
			var byteCharts []chartengine.CreateChartAction
			updates := make(map[string]chartengine.UpdateChartAction)
			for _, action := range plan.Actions {
				switch action := action.(type) {
				case chartengine.CreateChartAction:
					if action.Meta.Context == byteContext {
						byteCharts = append(byteCharts, action)
					}
				case chartengine.UpdateChartAction:
					updates[action.ChartID] = action
				}
			}

			require.Len(t, byteCharts, 1)
			for _, chart := range byteCharts {
				assert.Equal(t, "example-model", chart.Labels["model_name"])
				assert.Equal(t, "0", chart.Labels["engine"])
				assert.Equal(t, "replica-a", chart.Labels["ReplicaId"])
				assert.Equal(t, "worker-a", chart.Labels["WorkerId"])
				assert.Equal(t, "2.48.0", chart.Labels["Version"])
				assert.Equal(t, "session-a", chart.Labels["SessionName"])
				assert.Equal(t, "core_worker", chart.Labels["Component"])

				values := make(map[string]float64)
				for _, value := range updates[chart.ChartID].Values {
					if value.IsFloat {
						values[value.Name] = value.Float64
					} else {
						values[value.Name] = float64(value.Int64)
					}
				}
				require.Len(t, values, 4)
				for _, name := range []string{"loaded", "loaded_observed", "stored", "stored_observed"} {
					assert.Positivef(t, values[name], "dimension %q must carry the synthetic fixture value", name)
				}
			}
		},
	)
}

func TestCollector_LiteLLMProfileAllMetrics(t *testing.T) {
	testCollectorStockProfileAllMetrics(
		t,
		"prometheus/profiles/litellm/fixtures/litellm_all_metrics.prom",
		func(collr *Collector) {
			configureProfileJobFromMetadata(t, collr, "collector-go.d.plugin-prometheus-litellm", "litellm", "litellm")
		},
		"prometheus.litellm.",
		map[string][]string{
			"prometheus.litellm.gateway.in_flight_requests":                                        {"requests"},
			"prometheus.litellm.gateway.client_responses_by_status":                                {"200", "unclassified"},
			"prometheus.litellm.gateway.client_responses_by_stream_mode":                           {"True", "False", "unclassified"},
			"prometheus.litellm.gateway.client_failures_by_status":                                 {"429", "unclassified"},
			"prometheus.litellm.gateway.client_failures_by_rate_limit_category":                    {"vendor_rate_limit", "litellm_rate_limit", "unclassified"},
			"prometheus.litellm.gateway.client_failures_by_rate_limit_type":                        {"requests", "tokens", "unclassified"},
			"prometheus.litellm.gateway.request_total_latency_measurements_by_service_tier":        {"priority", "unclassified"},
			"prometheus.litellm.gateway.accumulated_request_total_latency_by_service_tier":         {"priority", "unclassified"},
			"prometheus.litellm.gateway.client_responses_by_status_by_provider_deployment":         {"200", "unclassified"},
			"prometheus.litellm.gateway.client_failures_by_status_by_provider_deployment":          {"429", "unclassified"},
			"prometheus.litellm.gateway.client_responses_by_status_by_route":                       {"200", "unclassified"},
			"prometheus.litellm.gateway.client_failures_by_status_by_route":                        {"429", "unclassified"},
			"prometheus.litellm.routing_and_deployments.deployment_state":                          {"state"},
			"prometheus.litellm.routing_and_deployments.deployment_failures_by_status":             {"429", "unclassified"},
			"prometheus.litellm.routing_and_deployments.deployment_cooldowns_by_status":            {"429", "unclassified"},
			"prometheus.litellm.routing_and_deployments.deployment_request_workload_by_deployment": {"requests"},
			"prometheus.litellm.routing_and_deployments.deployment_failures_by_status_by_deployment": {
				"429", "unclassified",
			},
			"prometheus.litellm.routing_and_deployments.deployment_cooldowns_by_status_by_deployment": {
				"429", "unclassified",
			},
			"prometheus.litellm.provider_api.accumulated_provider_latency":                      {"api_latency"},
			"prometheus.litellm.provider_api.provider_api_latency_measurements_by_service_tier": {"priority", "unclassified"},
			"prometheus.litellm.provider_api.provider_time_to_first_token_measurements_by_service_tier": {
				"priority", "unclassified",
			},
			"prometheus.litellm.provider_api.accumulated_provider_api_latency_by_service_tier": {
				"priority", "unclassified",
			},
			"prometheus.litellm.provider_api.accumulated_provider_time_to_first_token_by_service_tier": {
				"priority", "unclassified",
			},
			"prometheus.litellm.provider_api.accumulated_provider_latency_by_provider_model":       {"api_latency"},
			"prometheus.litellm.usage_and_cost.request_token_throughput":                           {"total"},
			"prometheus.litellm.usage_and_cost.llm_spend_by_service_tier":                          {"priority", "unclassified"},
			"prometheus.litellm.usage_and_cost.request_token_throughput_by_api_key":                {"total"},
			"prometheus.litellm.usage_and_cost.llm_spend_by_team":                                  {"spend"},
			"prometheus.litellm.caching.cache_outcomes":                                            {"hits"},
			"prometheus.litellm.caching.cache_outcomes_by_provider_model":                          {"hits"},
			"prometheus.litellm.governance.team_budget":                                            {"team_remaining"},
			"prometheus.litellm.guardrails.guardrail_invocations_by_status":                        {"success", "failure"},
			"prometheus.litellm.guardrails.guardrail_invocations_by_status_by_guardrail_hook":      {"success", "failure"},
			"prometheus.litellm.mcp_gateway.mcp_tool_calls":                                        {"calls"},
			"prometheus.litellm.mcp_gateway.mcp_tool_calls_by_server_tool":                         {"calls"},
			"prometheus.litellm.managed_batch_and_files.managed_batches_created":                   {"created"},
			"prometheus.litellm.managed_batch_and_files.managed_batches_created_by_provider_model": {"created"},
			"prometheus.litellm.callbacks_and_inventory.user_inventory":                            {"total"},
			"prometheus.litellm.callbacks_and_inventory.callback_logging_failures_by_callback":     {"failures"},
			"prometheus.litellm.internal_services.internal_service_requests":                       {"redis"},
			"prometheus.litellm.runtime.process_cpu":                                               {"used"},
			"prometheus.litellm.runtime.python_gc.collections":                                     {"0", "1", "2"},
		},
		nil,
		func(plan chartengine.Plan) {
			const (
				totalContext                          = "prometheus.litellm.gateway.client_responses_by_status"
				streamContext                         = "prometheus.litellm.gateway.client_responses_by_stream_mode"
				rateLimitCategoryContext              = "prometheus.litellm.gateway.client_failures_by_rate_limit_category"
				rateLimitTypeContext                  = "prometheus.litellm.gateway.client_failures_by_rate_limit_type"
				requestTierContext                    = "prometheus.litellm.gateway.request_total_latency_measurements_by_service_tier"
				providerTierContext                   = "prometheus.litellm.provider_api.provider_api_latency_measurements_by_service_tier"
				spendTierContext                      = "prometheus.litellm.usage_and_cost.llm_spend_by_service_tier"
				deploymentContext                     = "prometheus.litellm.gateway.client_responses_by_status_by_provider_deployment"
				routeContext                          = "prometheus.litellm.gateway.client_responses_by_status_by_route"
				providerModelContext                  = "prometheus.litellm.usage_and_cost.request_token_throughput_by_provider_model"
				clientFailureContext                  = "prometheus.litellm.gateway.client_failures_by_status"
				providerDeploymentFailureContext      = "prometheus.litellm.gateway.client_failures_by_status_by_provider_deployment"
				routeFailureContext                   = "prometheus.litellm.gateway.client_failures_by_status_by_route"
				deploymentFailureContext              = "prometheus.litellm.routing_and_deployments.deployment_failures_by_status"
				deploymentCooldownContext             = "prometheus.litellm.routing_and_deployments.deployment_cooldowns_by_status"
				deploymentFailureByDeploymentContext  = "prometheus.litellm.routing_and_deployments.deployment_failures_by_status_by_deployment"
				deploymentCooldownByDeploymentContext = "prometheus.litellm.routing_and_deployments." +
					"deployment_cooldowns_by_status_by_deployment"
				requestTierSumContext      = "prometheus.litellm.gateway.accumulated_request_total_latency_by_service_tier"
				providerTTFTTierContext    = "prometheus.litellm.provider_api.provider_time_to_first_token_measurements_by_service_tier"
				providerTierSumContext     = "prometheus.litellm.provider_api.accumulated_provider_api_latency_by_service_tier"
				providerTTFTTierSumContext = "prometheus.litellm.provider_api.accumulated_provider_time_to_first_token_by_service_tier"
			)

			contexts := make(map[string]string)
			creates := make(map[string][]chartengine.CreateChartAction)
			updates := make(map[string]chartengine.UpdateChartAction)
			for _, action := range plan.Actions {
				switch action := action.(type) {
				case chartengine.CreateChartAction:
					contexts[action.ChartID] = action.Meta.Context
					creates[action.Meta.Context] = append(creates[action.Meta.Context], action)
				case chartengine.UpdateChartAction:
					updates[action.ChartID] = action
				}
			}

			value := func(chartID, dimension string) float64 {
				t.Helper()
				for _, v := range updates[chartID].Values {
					if v.Name == dimension {
						if v.IsFloat {
							return v.Float64
						}
						return float64(v.Int64)
					}
				}
				t.Fatalf("dimension %q not found in chart %q (%s)", dimension, chartID, contexts[chartID])
				return 0
			}
			contextValue := func(context, dimension string) float64 {
				t.Helper()
				var total float64
				for _, chart := range creates[context] {
					for _, v := range updates[chart.ChartID].Values {
						if v.Name != dimension {
							continue
						}
						if v.IsFloat {
							total += v.Float64
						} else {
							total += float64(v.Int64)
						}
					}
				}
				return total
			}

			require.Len(t, creates[totalContext], 1)
			assert.Equal(t, float64(15), value(creates[totalContext][0].ChartID, "200"))
			assert.Equal(t, float64(11), value(creates[totalContext][0].ChartID, "unclassified"))

			require.Len(t, creates[streamContext], 1)
			assert.Equal(t, float64(3), value(creates[streamContext][0].ChartID, "True"))
			assert.Equal(t, float64(4), value(creates[streamContext][0].ChartID, "False"))
			assert.Equal(t, float64(23), value(creates[streamContext][0].ChartID, "unclassified"))

			require.Len(t, creates[rateLimitCategoryContext], 1)
			assert.Equal(t, float64(3), value(creates[rateLimitCategoryContext][0].ChartID, "vendor_rate_limit"))
			assert.Equal(t, float64(2), value(creates[rateLimitCategoryContext][0].ChartID, "litellm_rate_limit"))
			assert.Equal(t, float64(15), value(creates[rateLimitCategoryContext][0].ChartID, "unclassified"))

			require.Len(t, creates[rateLimitTypeContext], 1)
			assert.Equal(t, float64(3), value(creates[rateLimitTypeContext][0].ChartID, "requests"))
			assert.Equal(t, float64(2), value(creates[rateLimitTypeContext][0].ChartID, "tokens"))
			assert.Equal(t, float64(15), value(creates[rateLimitTypeContext][0].ChartID, "unclassified"))

			for context, want := range map[string]map[string]float64{
				clientFailureContext:                  {"429": 5, "500": 4, "unclassified": 11},
				providerDeploymentFailureContext:      {"429": 3, "500": 4, "unclassified": 11},
				routeFailureContext:                   {"429": 3, "500": 4, "unclassified": 11},
				deploymentFailureContext:              {"429": 3, "500": 4, "unclassified": 11},
				deploymentCooldownContext:             {"429": 3, "500": 4, "unclassified": 11},
				deploymentFailureByDeploymentContext:  {"429": 3, "500": 4, "unclassified": 11},
				deploymentCooldownByDeploymentContext: {"429": 3, "500": 4, "unclassified": 11},
				requestTierContext:                    {"priority": 2, "unclassified": 4},
				requestTierSumContext:                 {"priority": 1.3, "unclassified": 6.1},
				providerTierContext:                   {"priority": 2, "unclassified": 4},
				providerTTFTTierContext:               {"priority": 2, "unclassified": 4},
				providerTierSumContext:                {"priority": 1.3, "unclassified": 6.1},
				providerTTFTTierSumContext:            {"priority": 1.3, "unclassified": 6.1},
				spendTierContext:                      {"priority": 3, "unclassified": 9},
			} {
				require.NotEmpty(t, creates[context])
				for dimension, expected := range want {
					assert.InDelta(t, expected, contextValue(context, dimension), 1e-9,
						"context %q dimension %q", context, dimension)
				}
			}

			require.Len(t, creates[deploymentContext], 2)
			foundDeployment := false
			for _, create := range creates[deploymentContext] {
				if create.Labels["api_provider"] == "provider-a" && create.Labels["model_id"] == "model-id-a" {
					foundDeployment = true
					assert.Equal(t, float64(8), value(create.ChartID, "200"))
					assert.Equal(t, float64(11), value(create.ChartID, "unclassified"))
				}
			}
			assert.True(t, foundDeployment)

			require.Len(t, creates[routeContext], 2)
			foundRoute := false
			for _, create := range creates[routeContext] {
				if create.Labels["route"] == "/v1/chat/completions" {
					foundRoute = true
					assert.Equal(t, float64(8), value(create.ChartID, "200"))
					assert.Equal(t, float64(11), value(create.ChartID, "unclassified"))
				}
			}
			assert.True(t, foundRoute)

			require.Len(t, creates[providerModelContext], 1)
			assert.Equal(t, "provider-model-a", creates[providerModelContext][0].Labels["model"])
			assert.Equal(t, float64(8), value(creates[providerModelContext][0].ChartID, "total"))

			for context, want := range map[string]int{
				"prometheus.litellm.routing_and_deployments.deployment_state":            2,
				"prometheus.litellm.routing_and_deployments.provider_remaining_requests": 3,
				"prometheus.litellm.governance.api_key_budget":                           2,
				"prometheus.litellm.governance.user_budget":                              3,
				"prometheus.litellm.managed_batch_and_files.managed_file_size":           2,
			} {
				require.Len(t, creates[context], want)
				for _, create := range creates[context] {
					assert.NotEmptyf(t, create.Labels["pid"], "context %q must retain multiprocess identity", context)
				}
			}

			providerHeadroom := creates["prometheus.litellm.routing_and_deployments.provider_remaining_requests"]
			seenCustomGaugeIdentity := map[string]bool{}
			for _, create := range providerHeadroom {
				if create.Labels["pid"] == "1001" {
					seenCustomGaugeIdentity[create.Labels["metadata_project"]+"/"+create.Labels["tag_prod"]] = true
				}
			}
			assert.Equal(t, map[string]bool{"project-a/true": true, "project-b/false": true}, seenCustomGaugeIdentity)

			seenUserBudgetIdentity := map[string]bool{}
			for _, create := range creates["prometheus.litellm.governance.user_budget"] {
				seenUserBudgetIdentity[create.Labels["user_email"]+"/"+create.Labels["user_alias"]] = true
			}
			assert.True(t, seenUserBudgetIdentity["user-a@example.com/user-alias-a"])
			assert.True(t, seenUserBudgetIdentity["user-b@example.com/user-alias-b"])

			for chartID, context := range contexts {
				assert.Falsef(t, strings.HasPrefix(context, "prometheus.litellm.litellm_"),
					"chart %q unexpectedly used generic fallback context %q", chartID, context)
			}
		},
	)
}

func TestCollector_CephProfileAllMetrics(t *testing.T) {
	testCollectorStockProfileAllMetrics(
		t,
		"prometheus/profiles/ceph/fixtures/ceph_all_metrics.prom",
		func(collr *Collector) {
			configureProfileJobFromMetadata(t, collr, "collector-go.d.plugin-prometheus-ceph", "ceph", "ceph-mgr")
		},
		"prometheus.ceph.",
		map[string][]string{
			"prometheus.ceph.cluster_mgr.health.cluster_status.state":                                       {"value"},
			"prometheus.ceph.cluster_mgr.pools.capacity_and_quotas.raw_space":                               {"bytes_used"},
			"prometheus.ceph.daemon_exporters.exporter_and_process_runtime.daemon_availability.state":       {"reachable"},
			"prometheus.ceph.daemon_exporters.exporter_and_process_runtime.daemon_processes.cpu_time":       {"kernel", "user", "idle"},
			"prometheus.ceph.daemon_exporters.exporter_and_process_runtime.daemon_processes.memory":         {"virtual", "resident"},
			"prometheus.ceph.daemon_exporters.manager_daemons.lookup_outcomes":                              {"hit", "miss"},
			"prometheus.ceph.daemon_exporters.bluefs.space_and_files.file_work":                             {"sst", "wal"},
			"prometheus.ceph.daemon_exporters.object_gateway.operations.requests":                           {"get_obj_ops"},
			"prometheus.ceph.daemon_exporters.osd_scrubbing_ceph_19.outcomes.scrub_work":                    {"dp_repl_failed_scrubs"},
			"prometheus.ceph.daemon_exporters.osd_scrubbing_ceph_20.outcomes.scrub_work":                    {"failed_scrubs_replicated"},
			"prometheus.ceph.daemon_exporters.cephfs_mds.request_handling_and_state.operations":             {"request"},
			"prometheus.ceph.daemon_exporters.cephfs_mds_sessions.session_state":                            {"session", "sessions_open", "sessions_stale"},
			"prometheus.ceph.daemon_exporters.cephfs_mds_clients.capability_outcomes":                       {"hits", "misses"},
			"prometheus.ceph.daemon_exporters.rbd_client_images.io_operations":                              {"read", "write"},
			"prometheus.ceph.daemon_exporters.rbd_mirror.journal.byte_throughput":                           {"value"},
			"prometheus.ceph.daemon_exporters.runtime_and_client_components.rbd_persistent_write_log.reads": {"rd", "hit_rd", "part_hit_rd"},
			"prometheus.ceph.daemon_exporters.runtime_and_client_components.rbd_persistent_write_log.writes": {
				"wr", "wr_def", "wr_def_lanes", "wr_def_log", "wr_def_buf", "wr_overlap", "wr_q_barrier",
			},
			"prometheus.ceph.daemon_exporters.runtime_and_client_components.object_caches.cache_outcomes": {
				"hit", "miss",
			},
			"prometheus.ceph.daemon_exporters.runtime_and_client_components.finishers.queue_depth":     {"queued"},
			"prometheus.ceph.daemon_exporters.runtime_and_client_components.mclock_shards.queue_depth": {"immediate", "client", "recovery", "best_effort", "all_type"},
			"prometheus.ceph.daemon_exporters.runtime_and_client_components.memory_pools.memory_use":   {"osd", "ec_extent_cache"},
			"prometheus.ceph.cephfs_mirror.peers.snapshot_outcomes":                                    {"synced", "failed"},
			"prometheus.ceph.object_gateway_scheduling.queue_depth":                                    {"admin", "auth", "data", "metadata"},
			"prometheus.ceph.storage_engine_extensions.rocksdb_binned_cache.lookup_outcomes":           {"hits", "misses"},
			"prometheus.ceph.storage_engine_extensions.external_block_devices.partition_space":         {"physical_size", "logical_size", "physical_available", "logical_available"},
			"prometheus.ceph.ceph_clients.io_operations":                                               {"metadata", "read", "write"},
			"prometheus.ceph.nvme_of.block_devices.byte_throughput":                                    {"read_bytes"},
			"prometheus.ceph.nvme_of.gateway_runtime.process_cpu":                                      {"used"},
		},
		func(reader metrix.Reader) {
			names := make(map[string]int)
			reader.ForEachSeries(func(name string, _ metrix.LabelView, _ metrix.SampleValue) {
				names[name]++
			})
			var mds, image, pwl, rawPWL, rawDynamic, objecter int
			for name, count := range names {
				switch {
				case strings.HasPrefix(name, "ceph_mds_per_client_"):
					mds += count
				case strings.HasPrefix(name, "ceph_rbd_librbd_image_"):
					image += count
				case strings.HasPrefix(name, "ceph_rbd_librbd_pwl_"):
					pwl += count
				case strings.HasPrefix(name, "ceph_librbd_pwl_"):
					rawPWL += count
				case strings.HasPrefix(name, "ceph_mds_client_metrics_filesystem_key_") ||
					strings.HasPrefix(name, "ceph_librbd_image_key_"):
					rawDynamic += count
				case strings.HasPrefix(name, "ceph_objecter_0x"):
					objecter += count
				}
			}
			assert.Equal(t, 15, mds)
			assert.Equal(t, 34, image)
			assert.Equal(t, 81, pwl)
			assert.Zero(t, rawPWL)
			assert.Zero(t, rawDynamic)
			assert.Zero(t, objecter, "dynamic objecter families must be normalized before profile routing")
		},
		func(plan chartengine.Plan) {
			contexts := make(map[string][]chartengine.CreateChartAction)
			dimensions := make(map[string]map[string]chartengine.CreateDimensionAction)
			reservedSourceLabelPreserved := map[string]bool{
				"cluster_mgr": false,
				"nvme_of":     false,
			}
			var pwlFallback, objecterCharts, rawRGWAliasCharts int
			for _, action := range plan.Actions {
				switch action := action.(type) {
				case chartengine.CreateChartAction:
					contexts[action.Meta.Context] = append(contexts[action.Meta.Context], action)
					assert.NotContains(t, action.Labels, "instance",
						"source instance must be renamed before Netdata adds its re-export instance label")
					if action.Labels["ceph_instance"] != "" {
						switch {
						case strings.HasPrefix(action.Meta.Context, "prometheus.ceph.cluster_mgr."):
							reservedSourceLabelPreserved["cluster_mgr"] = true
						case strings.HasPrefix(action.Meta.Context, "prometheus.ceph.nvme_of."):
							reservedSourceLabelPreserved["nvme_of"] = true
						}
					}
					if strings.Contains(action.Meta.Context, ".ceph_librbd_pwl_") {
						pwlFallback++
					}
					if strings.Contains(action.Meta.Context, ".ceph_objecter_0x") {
						objecterCharts++
					}
					if strings.Contains(action.Meta.Context, ".ceph_data_sync_from_zone_a_") {
						rawRGWAliasCharts++
					}
				case chartengine.CreateDimensionAction:
					if dimensions[action.ChartMeta.Context] == nil {
						dimensions[action.ChartMeta.Context] = make(map[string]chartengine.CreateDimensionAction)
					}
					dimensions[action.ChartMeta.Context][action.Name] = action
				}
			}
			assert.Zero(t, pwlFallback, "normalized PWL families must use curated charts")
			assert.Zero(t, objecterCharts, "dynamic objecter families must use the curated canonical context")
			assert.Zero(t, rawRGWAliasCharts, "MGR raw RGW source-zone aliases must not duplicate normalized families")
			assert.Equal(t, map[string]bool{"cluster_mgr": true, "nvme_of": true}, reservedSourceLabelPreserved,
				"official source instance labels must remain available under ceph_instance")

			type semanticExpectation struct {
				units     string
				algorithm chartengine.Algorithm
				dimension string
				divisor   int
				float     bool
			}
			wantSemantics := map[string]semanticExpectation{
				"prometheus.ceph.cluster_mgr.osd_capacity_and_state.osd_state.weight": {"weight", chartengine.AlgorithmAbsolute, "weight", 0, false},
				"prometheus.ceph.cluster_mgr.pools.capacity_and_quotas.logical_space": {"bytes", chartengine.AlgorithmAbsolute, "stored", 0, false},
				"prometheus.ceph.cluster_mgr.pools.capacity_and_quotas.raw_space":     {"bytes", chartengine.AlgorithmAbsolute, "stored_raw", 0, false},
				"prometheus.ceph.cluster_mgr.pools.capacity_and_quotas.compression_space": {
					"bytes", chartengine.AlgorithmAbsolute, "compress_under_bytes", 0, false,
				},
				"prometheus.ceph.daemon_exporters.bluefs.i_o_and_files.zero_read_outcomes": {"reads/s", chartengine.AlgorithmIncremental, "errors", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.allocation.allocation_unit":    {"bytes", chartengine.AlgorithmAbsolute, "alloc_unit", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.allocation.allocated_space":    {"bytes", chartengine.AlgorithmAbsolute, "allocated", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.compression_and_checksums.compressed_extents": {
					"extents/s", chartengine.AlgorithmIncremental, "extent_compress", 0, false,
				},
				"prometheus.ceph.daemon_exporters.bluestore.i_o.skipped_blobs":    {"blobs/s", chartengine.AlgorithmIncremental, "write_big_skipped_blobs", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.i_o.slow_aio_waits":   {"waits/s", chartengine.AlgorithmIncremental, "slow_aio_wait", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.i_o.write_operations": {"writes/s", chartengine.AlgorithmIncremental, "write_big", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.i_o.written_blobs":    {"blobs/s", chartengine.AlgorithmIncremental, "write_big_blobs", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.i_o.penalty_reads":    {"reads/s", chartengine.AlgorithmIncremental, "write_penalty_read_ops", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.omap.open_iterators":  {"iterators", chartengine.AlgorithmAbsolute, "iterator", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.omap.mutation_calls":  {"calls/s", chartengine.AlgorithmIncremental, "setheader", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.omap.key_mutations":   {"keys/s", chartengine.AlgorithmIncremental, "setkeys_records", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.onode_cache.blob_state": {
					"blobs", chartengine.AlgorithmAbsolute, "onode_spanning_blobs", 0, false,
				},
				"prometheus.ceph.daemon_exporters.bluestore.onode_cache.extent_state":          {"extents", chartengine.AlgorithmAbsolute, "onode_extents", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.onode_cache.shard_lookup_outcomes": {"lookups/s", chartengine.AlgorithmIncremental, "shard_misses", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.space_and_core_state.blob_splits":  {"blobs/s", chartengine.AlgorithmIncremental, "blob_split", 0, false},
				"prometheus.ceph.daemon_exporters.bluestore.space_and_core_state.gc_merge_throughput": {
					"bytes/s", chartengine.AlgorithmIncremental, "gc_merged", 0, false,
				},
				"prometheus.ceph.daemon_exporters.bluestore.space_and_core_state.buffer_cache_residency":  {"bytes", chartengine.AlgorithmAbsolute, "buffer_bytes", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.capabilities.capability_messages":            {"messages/s", chartengine.AlgorithmIncremental, "handle_client_caps", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.capabilities.throttled_requests":             {"requests/s", chartengine.AlgorithmIncremental, "server_cap_acquisition_throttle", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.capabilities.evicted_clients":                {"clients/s", chartengine.AlgorithmIncremental, "server_cap_revoke_eviction", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.journal.large_events":                        {"events/s", chartengine.AlgorithmIncremental, "evlrg", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.metadata_cache_directory.discovery_messages": {"messages/s", chartengine.AlgorithmIncremental, "send_discover", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.metadata_cache_directory.discovery_attempts": {"attempts/s", chartengine.AlgorithmIncremental, "try_discover", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.metadata_cache_directory.replication_directives": {
					"directives/s", chartengine.AlgorithmIncremental, "update_receipt", 0, false,
				},
				"prometheus.ceph.daemon_exporters.cephfs_mds.namespace_traversal.dirfrag_lifecycle":                            {"dirfrags/s", chartengine.AlgorithmIncremental, "dir_split", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.namespace_traversal.traversals":                                   {"traversals/s", chartengine.AlgorithmIncremental, "traverse", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.request_handling_and_state.connected_clients":                     {"clients", chartengine.AlgorithmAbsolute, "client_metrics_num_clients", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.request_handling_and_state.queued_requests":                       {"requests", chartengine.AlgorithmAbsolute, "q", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.request_handling_and_state.expired_inodes":                        {"inodes/s", chartengine.AlgorithmIncremental, "expired", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.scrubbing.scrubbed_inodes":                                        {"inodes/s", chartengine.AlgorithmIncremental, "file_inodes", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.scrubbing.backtrace_work":                                         {"backtraces/s", chartengine.AlgorithmIncremental, "backtrace_repaired", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.scrubbing.repaired_inotables":                                     {"inotables/s", chartengine.AlgorithmIncremental, "inotable_repaired", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds.scrubbing.dirfrag_rstat_updates":                                  {"dirfrags/s", chartengine.AlgorithmIncremental, "value", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds_sessions.average_load":                                            {"load", chartengine.AlgorithmAbsolute, "average_load", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds_sessions.metadata_threshold_evictions":                            {"sessions/s", chartengine.AlgorithmIncremental, "value", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds_clients.capability_outcomes":                                      {"accesses/s", chartengine.AlgorithmIncremental, "hits", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds_clients.dentry_lease_outcomes":                                    {"lookups/s", chartengine.AlgorithmIncremental, "misses", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds_clients.reported_io_operations":                                   {"operations/s", chartengine.AlgorithmIncremental, "read", 0, false},
				"prometheus.ceph.daemon_exporters.cephfs_mds_clients.reported_io_volume":                                       {"bytes/s", chartengine.AlgorithmIncremental, "write", 0, false},
				"prometheus.ceph.daemon_exporters.control_plane.paxos.collected_uncommitted_values":                            {"values/s", chartengine.AlgorithmIncremental, "collect_uncommitted", 0, false},
				"prometheus.ceph.daemon_exporters.messenger.rdma_errors":                                                       {"errors/s", chartengine.AlgorithmIncremental, "rdmadispatcher_handshake_errors", 0, false},
				"prometheus.ceph.daemon_exporters.messenger.connection_timeouts":                                               {"connections/s", chartengine.AlgorithmIncremental, "worker_msgr_connection_ready_timeouts", 0, false},
				"prometheus.ceph.daemon_exporters.messenger.active_queue_pairs":                                                {"queue pairs", chartengine.AlgorithmAbsolute, "active_queue_pair", 0, false},
				"prometheus.ceph.daemon_exporters.messenger.receive_buffers":                                                   {"buffers", chartengine.AlgorithmAbsolute, "rx_bufs_in_use", 0, false},
				"prometheus.ceph.daemon_exporters.messenger.work_completions":                                                  {"completions/s", chartengine.AlgorithmIncremental, "tx_total_wc", 0, false},
				"prometheus.ceph.daemon_exporters.osd_runtime.crc_lookup_outcomes":                                             {"lookups/s", chartengine.AlgorithmIncremental, "missed_crc", 0, false},
				"prometheus.ceph.daemon_exporters.osd_runtime.tier_agent_actions":                                              {"actions/s", chartengine.AlgorithmIncremental, "tier_promote", 0, false},
				"prometheus.ceph.daemon_exporters.osd_runtime.map_messages":                                                    {"messages/s", chartengine.AlgorithmIncremental, "messages_delayed_for_map", 0, false},
				"prometheus.ceph.daemon_exporters.osd_runtime.map_epochs":                                                      {"epochs/s", chartengine.AlgorithmIncremental, "map_message_epoch_dups", 0, false},
				"prometheus.ceph.daemon_exporters.osd_runtime.object_context_cache_lookup_outcomes":                            {"lookups/s", chartengine.AlgorithmIncremental, "object_ctx_cache", 0, false},
				"prometheus.ceph.daemon_exporters.osd_runtime.heartbeat_peers":                                                 {"peers", chartengine.AlgorithmAbsolute, "heartbeat_to_peers", 0, false},
				"prometheus.ceph.daemon_exporters.osd_runtime.load_average":                                                    {"load", chartengine.AlgorithmAbsolute, "loadavg", 0, false},
				"prometheus.ceph.daemon_exporters.osd_scrubbing_ceph_19.elapsed_time.scrub_latency_measurements":               {"scrubs/s", chartengine.AlgorithmIncremental, "dp_ec_failed_scrubs_elapsed", 0, false},
				"prometheus.ceph.daemon_exporters.osd_scrubbing_ceph_19.elapsed_time.reservation_latency_measurements":         {"reservations/s", chartengine.AlgorithmIncremental, "dp_ec_failed_reservations_elapsed", 0, false},
				"prometheus.ceph.daemon_exporters.osd_scrubbing_ceph_19.reservations.replicas_in_reservation":                  {"replicas", chartengine.AlgorithmAbsolute, "dp_ec_replicas_in_reservation", 0, false},
				"prometheus.ceph.daemon_exporters.osd_scrubbing_ceph_19.reservations.reservation_process_outcomes":             {"reservations/s", chartengine.AlgorithmIncremental, "dp_ec_reservation_process_failure", 0, false},
				"prometheus.ceph.daemon_exporters.osd_scrubbing_ceph_20.interference_and_scheduling.client_write_interference": {"writes/s", chartengine.AlgorithmIncremental, "ec_io_blocked", 0, false},
				"prometheus.ceph.daemon_exporters.osd_scrubbing_ceph_20.scrub_work.scrub_calls":                                {"calls/s", chartengine.AlgorithmIncremental, "ec_getattr_cnt", 0, false},
				"prometheus.ceph.daemon_exporters.osd_scrubbing_common_work.interference_and_scheduling.object_lock_waits":     {"waits/s", chartengine.AlgorithmIncremental, "dp_ec_locked_object", 0, false},
				"prometheus.ceph.daemon_exporters.osd_scrubbing_common_work.interference_and_scheduling.chunk_selection_outcomes": {
					"chunks/s", chartengine.AlgorithmIncremental, "dp_ec_chunk_selected", 0, false,
				},
				"prometheus.ceph.daemon_exporters.osd_scrubbing_common_work.interference_and_scheduling.preemptions":           {"preemptions/s", chartengine.AlgorithmIncremental, "dp_ec_preemptions", 0, false},
				"prometheus.ceph.daemon_exporters.osd_scrubbing_common_work.interference_and_scheduling.blocked_client_writes": {"writes/s", chartengine.AlgorithmIncremental, "dp_ec_write_blocked_by_scrub", 0, false},
				"prometheus.ceph.daemon_exporters.osd_scrubbing_common_work.reservations.completed_reservations":               {"reservations/s", chartengine.AlgorithmIncremental, "dp_ec_scrub_reservations_completed", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway.lifecycle.lifecycle_actions":                                  {"actions/s", chartengine.AlgorithmIncremental, "expire_current", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway.lifecycle.aborted_multipart_uploads":                          {"uploads/s", chartengine.AlgorithmIncremental, "value", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway.lua.current_vms":                                              {"VMs", chartengine.AlgorithmAbsolute, "value", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway.lua.script_executions":                                        {"executions/s", chartengine.AlgorithmIncremental, "failure", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway.notifications.pending_events":                                 {"events", chartengine.AlgorithmAbsolute, "value", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway.notifications.missing_configurations":                         {"events/s", chartengine.AlgorithmIncremental, "value", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway.operations.requests":                                          {"requests/s", chartengine.AlgorithmIncremental, "copy_obj_ops", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway.requests_and_queue.current_requests":                          {"requests", chartengine.AlgorithmAbsolute, "qactive", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway.requests_and_queue.failed_requests":                           {"requests/s", chartengine.AlgorithmIncremental, "value", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway_cache.d4n_cache_evictions":                                    {"entries/s", chartengine.AlgorithmIncremental, "value", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway_cache.keystone_lookup_outcomes":                               {"lookups/s", chartengine.AlgorithmIncremental, "miss", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway_multisite.replicated_objects":                                 {"objects/s", chartengine.AlgorithmIncremental, "value", 0, false},
				"prometheus.ceph.daemon_exporters.object_gateway_multisite.replication_log_request_errors":                     {"requests/s", chartengine.AlgorithmIncremental, "value", 0, false},
				"prometheus.ceph.daemon_exporters.objecter.active_commands":                                                    {"commands", chartengine.AlgorithmAbsolute, "command_active", 0, false},
				"prometheus.ceph.daemon_exporters.objecter.map_epoch":                                                          {"epoch", chartengine.AlgorithmAbsolute, "map_epoch", 0, false},
				"prometheus.ceph.daemon_exporters.objecter.operation_vector_entries":                                           {"entries/s", chartengine.AlgorithmIncremental, "value", 0, false},
				"prometheus.ceph.daemon_exporters.objecter.commands":                                                           {"commands/s", chartengine.AlgorithmIncremental, "command_send", 0, false},
				"prometheus.ceph.daemon_exporters.objecter.replica_reads":                                                      {"reads/s", chartengine.AlgorithmIncremental, "replica_read_completed", 0, false},
				"prometheus.ceph.daemon_exporters.objecter.osd_sessions":                                                       {"sessions/s", chartengine.AlgorithmIncremental, "osd_session_open", 0, false},
				"prometheus.ceph.daemon_exporters.objecter.stat_requests":                                                      {"requests/s", chartengine.AlgorithmIncremental, "statfs_send", 0, false},
				"prometheus.ceph.daemon_exporters.rbd_mirror.journal.latency_measurements":                                     {"entries/s", chartengine.AlgorithmIncremental, "value", 0, false},
				"prometheus.ceph.daemon_exporters.rbd_mirror.journal.journal_entries":                                          {"entries/s", chartengine.AlgorithmIncremental, "value", 0, false},
				"prometheus.ceph.daemon_exporters.rbd_mirror.snapshot_replication.latency_measurements":                        {"snapshots/s", chartengine.AlgorithmIncremental, "sync_time", 0, false},
				"prometheus.ceph.daemon_exporters.runtime.shared_daemon.kstore_latency_measurements":                           {"transactions/s", chartengine.AlgorithmIncremental, "kstore_state_done_lat", 0, false},
				"prometheus.ceph.daemon_exporters.runtime.shared_daemon.client_latency_measurements":                           {"requests/s", chartengine.AlgorithmIncremental, "client_lat", 0, false},
				"prometheus.ceph.daemon_exporters.runtime.shared_daemon.recovery_state_latency_measurements":                   {"state transitions/s", chartengine.AlgorithmIncremental, "recoverystate_perf_active_latency", 0, false},
				"prometheus.ceph.daemon_exporters.runtime.shared_daemon.sqlite_vfs_latency_measurements":                       {"operations/s", chartengine.AlgorithmIncremental, "libcephsqlite_vfs_op_open", 0, false},
				"prometheus.ceph.daemon_exporters.runtime.shared_daemon.monitor_state":                                         {"monitors", chartengine.AlgorithmAbsolute, "cluster_num_mon_quorum", 0, false},
				"prometheus.ceph.daemon_exporters.runtime.shared_daemon.pg_state":                                              {"PGs", chartengine.AlgorithmAbsolute, "cluster_num_pg_active_clean", 0, false},
				"prometheus.ceph.daemon_exporters.runtime.shared_daemon.worker_state":                                          {"workers", chartengine.AlgorithmAbsolute, "cct_unhealthy_workers", 0, false},
				"prometheus.ceph.daemon_exporters.runtime.shared_daemon.oft_omap_mutations":                                    {"operations/s", chartengine.AlgorithmIncremental, "oft_omap_total_removes", 0, false},
				"prometheus.ceph.daemon_exporters.runtime.shared_daemon.client_latency_sqsum": {
					"microseconds²/s", chartengine.AlgorithmIncremental, "client_mdsqsum", 1000000, true,
				},
				"prometheus.ceph.daemon_exporters.runtime.slow_operations.slow_operations": {"operations/s", chartengine.AlgorithmIncremental, "value", 0, false},
				"prometheus.ceph.daemon_exporters.storage_engine.rocksdb_compaction.compactions": {
					"compactions/s", chartengine.AlgorithmIncremental, "completed", 0, false,
				},
				"prometheus.ceph.daemon_exporters.storage_engine.rocksdb_compaction.queue_merges":                                             {"merges/s", chartengine.AlgorithmIncremental, "queue_merge", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.rbd_persistent_write_log.reads":                               {"reads/s", chartengine.AlgorithmIncremental, "rd", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.rbd_persistent_write_log.writes":                              {"writes/s", chartengine.AlgorithmIncremental, "wr_def_lanes", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.rbd_persistent_write_log.log_appends":                         {"appends/s", chartengine.AlgorithmIncremental, "log_ops", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.rbd_persistent_write_log.log_append_size_measurements":        {"appends/s", chartengine.AlgorithmIncremental, "log_op_bytes_count", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.rbd_persistent_write_log.write_latency_measurements":          {"requests/s", chartengine.AlgorithmIncremental, "wr_latency_count", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.rbd_persistent_write_log.log_operation_latency_measurements":  {"operations/s", chartengine.AlgorithmIncremental, "op_alloc_t_count", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.rbd_persistent_write_log.data_operation_latency_measurements": {"operations/s", chartengine.AlgorithmIncremental, "discard_lat_count", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.rbd_persistent_write_log.transaction_latency_measurements":    {"transactions/s", chartengine.AlgorithmIncremental, "append_tx_lat_count", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.finishers.queue_depth":                                        {"callbacks", chartengine.AlgorithmAbsolute, "queued", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.finishers.completions":                                        {"batches/s", chartengine.AlgorithmIncremental, "completed", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.throttles.slot_throughput":                                    {"slots/s", chartengine.AlgorithmIncremental, "get_sum", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.throttles.wait_measurements":                                  {"waits/s", chartengine.AlgorithmIncremental, "waits", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.messenger_workers.send_queue_latency_measurements":            {"messages/s", chartengine.AlgorithmIncremental, "send_messages_queue", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.messenger_workers.ack_latency_measurements":                   {"acknowledgements/s", chartengine.AlgorithmIncremental, "handle_ack", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.rdma_workers.transfer_errors":                                 {"errors/s", chartengine.AlgorithmIncremental, "tx_failed_post", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.dpdk_queues.packet_fragments":                                 {"fragments/s", chartengine.AlgorithmIncremental, "receive_fragments", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.dpdk_queues.copy_linearize_operations":                        {"operations/s", chartengine.AlgorithmIncremental, "receive_copy_ops", 0, false},
				"prometheus.ceph.daemon_exporters.runtime_and_client_components.service_identity.identity_metadata":                           {"identifier", chartengine.AlgorithmAbsolute, "present", 0, false},
				"prometheus.ceph.object_gateway_scheduling.throttled_requests":                                                                {"requests/s", chartengine.AlgorithmIncremental, "throttled", 0, false},
				"prometheus.ceph.object_gateway_scheduling.outstanding_requests":                                                              {"requests", chartengine.AlgorithmAbsolute, "outstanding", 0, false},
				"prometheus.ceph.storage_engine_extensions.rocksdb_binned_cache.entries":                                                      {"entries", chartengine.AlgorithmAbsolute, "items", 0, false},
			}
			for context, want := range wantSemantics {
				require.NotEmptyf(t, contexts[context], "semantic regression context %q did not materialize", context)
				for _, create := range contexts[context] {
					assert.Equalf(t, want.units, create.Meta.Units, "context %q units", context)
				}
				dim, ok := dimensions[context][want.dimension]
				require.Truef(t, ok, "semantic regression dimension %q/%q did not materialize", context, want.dimension)
				assert.Equalf(t, want.algorithm, dim.Algorithm, "context %q dimension %q algorithm", context, want.dimension)
				if want.divisor != 0 || want.float {
					assert.Equalf(t, want.divisor, dim.Divisor, "context %q dimension %q divisor", context, want.dimension)
					assert.Equalf(t, want.float, dim.Float, "context %q dimension %q float mode", context, want.dimension)
				}
			}

			require.Len(t, contexts["prometheus.ceph.daemon_exporters.rbd_mirror.journal.byte_throughput"], 2)
			require.Len(t, contexts["prometheus.ceph.daemon_exporters.cephfs_mds_clients.capability_outcomes"], 1)
			mdsChart := contexts["prometheus.ceph.daemon_exporters.cephfs_mds_clients.capability_outcomes"][0]
			assert.Equal(t, "filesystem_key_a", mdsChart.Labels["mds_filesystem_key"])
			assert.NotContains(t, mdsChart.Labels, "fs_name")

			require.Len(t, contexts["prometheus.ceph.daemon_exporters.rbd_client_images.io_operations"], 1)
			imageChart := contexts["prometheus.ceph.daemon_exporters.rbd_client_images.io_operations"][0]
			assert.Equal(t, "image_key_a_pool_a_image_a", imageChart.Labels["librbd_image_key"])

			require.Len(t, contexts["prometheus.ceph.daemon_exporters.runtime_and_client_components.rbd_persistent_write_log.reads"], 1)
			pwlChart := contexts["prometheus.ceph.daemon_exporters.runtime_and_client_components.rbd_persistent_write_log.reads"][0]
			assert.Equal(t, "image_key_a_pool_a_image_a", pwlChart.Labels["librbd_pwl_key"])

			require.Len(t, contexts["prometheus.ceph.daemon_exporters.runtime_and_client_components.finishers.queue_depth"], 2)
			finisherKeys := make(map[string]bool)
			for _, chart := range contexts["prometheus.ceph.daemon_exporters.runtime_and_client_components.finishers.queue_depth"] {
				finisherKeys[chart.Labels["finisher_key"]] = true
			}
			assert.Equal(t, map[string]bool{"fixture_a": true, "fixture_a_0x1000": true}, finisherKeys)
		},
	)
}

// TestCollector_CephProfileProducerVariants keeps the distinct official wire
// contracts executable. MGR emits long-running-average sums as counters, while
// ceph-exporter emits those same sums as gauges from the daemon schema.
func TestCollector_CephProfileProducerVariants(t *testing.T) {
	tests := []struct {
		fixture string
		jobName string
		context string
		dims    []string
	}{
		{"prometheus/profiles/ceph/fixtures/ceph_reef_mgr_perf_all_metrics.prom", "ceph-mgr", "prometheus.ceph.cluster_mgr.health.cluster_status.state", []string{"value"}},
		{"prometheus/profiles/ceph/fixtures/ceph_squid_mgr_perf_all_metrics.prom", "ceph-mgr", "prometheus.ceph.cluster_mgr.health.cluster_status.state", []string{"value"}},
		{"prometheus/profiles/ceph/fixtures/ceph_tentacle_mgr_perf_all_metrics.prom", "ceph-mgr", "prometheus.ceph.cluster_mgr.health.cluster_status.state", []string{"value"}},
		{
			"prometheus/profiles/ceph/fixtures/ceph_reef_exporter_prio0_all_metrics.prom",
			"ceph-exporter",
			"prometheus.ceph.daemon_exporters.exporter_and_process_runtime.daemon_processes.cpu_time",
			[]string{"kernel", "user", "idle"},
		},
		{
			"prometheus/profiles/ceph/fixtures/ceph_squid_exporter_prio0_all_metrics.prom",
			"ceph-exporter",
			"prometheus.ceph.daemon_exporters.exporter_and_process_runtime.daemon_processes.cpu_time",
			[]string{"kernel", "user", "idle"},
		},
		{
			"prometheus/profiles/ceph/fixtures/ceph_tentacle_exporter_prio0_all_metrics.prom",
			"ceph-exporter",
			"prometheus.ceph.daemon_exporters.exporter_and_process_runtime.daemon_processes.cpu_time",
			[]string{"kernel", "user", "idle"},
		},
		{"prometheus/profiles/ceph/fixtures/ceph_nvmeof_all_metrics.prom", "ceph-exporter", "prometheus.ceph.nvme_of.block_devices.byte_throughput", []string{"read_bytes", "written_bytes"}},
	}

	for _, test := range tests {
		t.Run(filepath.Base(test.fixture), func(t *testing.T) {
			testCollectorStockProfileAllMetrics(
				t,
				test.fixture,
				func(collr *Collector) {
					configureProfileJobFromMetadata(t, collr, "collector-go.d.plugin-prometheus-ceph", "ceph", test.jobName)
				},
				"prometheus.ceph.",
				map[string][]string{test.context: test.dims},
				nil,
				nil,
			)
		})
	}
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
