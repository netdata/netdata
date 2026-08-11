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

	"github.com/netdata/netdata/go/plugins/internal/promprofile/testutil"
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
		"success if endpoint exposes only a summary without quantiles": {
			wantFail: false,
			prepare: func() (collr *Collector, cleanup func()) {
				srv := httptest.NewServer(http.HandlerFunc(
					func(w http.ResponseWriter, _ *http.Request) {
						_, _ = w.Write([]byte(`
# TYPE app_payload_bytes summary
app_payload_bytes_sum 12.5
app_payload_bytes_count 4
`))
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
		"summary without quantiles flattens to sum and count": {
			prepare: New,
			input: `
# TYPE test_payload_bytes summary
test_payload_bytes_sum{handler="/v1/items"} 12.5
test_payload_bytes_count{handler="/v1/items"} 4
`,
			want: func(t *testing.T, fr metrix.Reader) {
				labels := metrix.Labels{"handler": "/v1/items"}
				assert.InDelta(t, 12.5, value(t, fr, "test_payload_bytes_sum", labels), 1e-9)
				assert.InDelta(t, 4, value(t, fr, "test_payload_bytes_count", labels), 1e-9)
				_, ok := fr.Value("test_payload_bytes", labels)
				assert.False(t, ok, "a quantile-free summary must not fabricate a base quantile series")
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
		"gauge _info family is skipped and counter is kept": {
			prepare: New,
			input: `
# TYPE test_metric gauge
test_metric{label1="value1"} 11
# TYPE test_metric_info gauge
test_metric_info{version="1.2.3"} 1
# TYPE test_pg_info counter
test_pg_info 7
`,
			want: func(t *testing.T, fr metrix.Reader) {
				assert.InDelta(t, 11, value(t, fr, "test_metric", metrix.Labels{"label1": "value1"}), 1e-9)
				_, ok := fr.Value("test_metric_info", metrix.Labels{"version": "1.2.3"})
				assert.False(t, ok, "a gauge _info family must be skipped")
				assert.InDelta(t, 7, value(t, fr, "test_pg_info", nil), 1e-9)
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
		"prometheus.haproxy.frontend_status":                       {"UP"},
		"prometheus.haproxy.frontend_current_sessions":             {"value"},
		"prometheus.haproxy.frontend_sessions_total":               {"value"},
		"prometheus.haproxy.frontend_traffic":                      {"in", "out"},
		"prometheus.haproxy.frontend_http_requests_total":          {"value"},
		"prometheus.haproxy.frontend_http_responses_total":         {"2xx", "5xx"},
		"prometheus.haproxy.backend_current_sessions":              {"value"},
		"prometheus.haproxy.backend_sessions_total":                {"value"},
		"prometheus.haproxy.backend_current_queue":                 {"value"},
		"prometheus.haproxy.backend_traffic":                       {"in", "out"},
		"prometheus.haproxy.backend_response_time_average_seconds": {"value"},
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
// (every source-derived default family; optional extra-counters are outside this profile)
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
		"prometheus.haproxy.process_connections_total":     {"value"},
		"prometheus.haproxy.frontend_status":               {"UP", "DOWN"},
		"prometheus.haproxy.frontend_http_responses_total": {"1xx", "2xx", "3xx", "4xx", "5xx", "other"},
		"prometheus.haproxy.listener_current_sessions":     {"value"},
		"prometheus.haproxy.backend_status":                {"UP", "DOWN"},
		"prometheus.haproxy.server_current_sessions":       {"value"},
		"prometheus.haproxy.resolver_sent":                 {"value"},
		"prometheus.haproxy.resolver_valid":                {"value"},
		"prometheus.haproxy.sticktable_used":               {"value"},
		"prometheus.haproxy.sticktable_size":               {"value"},
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

func TestCollector_VLLMRayTransportProfile(t *testing.T) {
	const input = `
# TYPE ray_vllm_num_requests_running gauge
ray_vllm_num_requests_running{Component="core_worker",ReplicaId="replica-a",SessionName="session-a",Version="2.48.0",WorkerId="worker-a",engine="0",model_name="example-model"} 1
`
	testCollectorStockProfileModes(
		t,
		"vllm",
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
		map[string][]string{"prometheus.litellm.gateway.client_traffic.in_flight_requests": {"requests"}},
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
		map[string][]string{"prometheus.ceph.health.cluster_status.state": {"value"}},
		map[string][]string{"prometheus.ceph_health_status": {"ceph_health_status"}},
	)
}

func configureProfileJobFromMetadata(
	t *testing.T,
	collr *Collector,
	integrationID, profileName, jobName string,
	supportingProfileNames ...string,
) []string {
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
	require.Empty(t, config.Application, "stock metadata must derive app identity from the application profile")
	require.Equal(t, ProfilesConfig{Mode: profilesModeAuto}, config.Profiles,
		"stock metadata must exercise automatic profile selection")

	config.URL = collr.URL
	collr.Config = *config
	return append([]string{profileName}, supportingProfileNames...)
}

func requireSelectedProfiles(t *testing.T, collr *Collector, expected ...string) {
	t.Helper()
	require.NotNil(t, collr.runtime)
	actual := make([]string, 0, len(collr.runtime.profiles))
	for _, profile := range collr.runtime.profiles {
		actual = append(actual, profile.Name)
	}
	require.ElementsMatch(t, expected, actual)
}

// TestCollector_VLLMProfileAllMetrics proves the stock profile against a
// sanitized structural union, including optional and mutually exclusive
// source-defined surfaces and each distinct entity identity.
func TestCollector_VLLMProfileAllMetrics(t *testing.T) {
	fixture := promtestutil.Require(t, "prometheus/profiles/vllm/fixtures/vllm_all_metrics.prom")
	input, err := os.ReadFile(fixture)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(input) }))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	expectedProfiles := configureProfileJobFromMetadata(t, collr, "collector-go.d.plugin-prometheus-vllm", "vllm", "vllm",
		"fastapi", "process_runtime", "python_gc")
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	requireSelectedProfiles(t, collr, expectedProfiles...)

	cc := cycle(t, collr.MetricStore())
	cc.BeginCycle()
	require.NoError(t, collr.Collect(context.Background()))
	require.NoError(t, cc.CommitCycleSuccess())

	var sawCanonicalOffload bool
	collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()).ForEachSeries(
		func(name string, _ metrix.LabelView, _ metrix.SampleValue) {
			assert.Falsef(t, strings.HasSuffix(name, "_created"), "profile relabeling retained generated timestamp %q", name)
			assert.NotEqual(t, "process_start_time_seconds", name)
			assert.NotEqual(t, "vllm:kv_offload_total_bytes_total", name)
			assert.NotEqual(t, "vllm:kv_offload_total_time_total", name)
			assert.Falsef(t, strings.HasPrefix(name, "vllm:kv_offload_size"),
				"profile relabeling retained deprecated offload family %q", name)
			if name == "vllm:kv_offload_store_bytes_total" {
				sawCanonicalOffload = true
			}
		})
	assert.True(t, sawCanonicalOffload, "profile relabeling must retain canonical CPU-offload counters")

	eng, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, eng.LoadYAML([]byte(collr.ChartTemplateYAML()), 1))

	attempt, err := eng.PreparePlan(collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	defer attempt.Abort()
	plan := attempt.Plan()
	require.NoError(t, attempt.Commit())
	runtimeReader := eng.RuntimeStore().Read(metrix.ReadRaw())
	name := "netdata.go.plugin.framework.chartengine.series_autogen_matched_total"
	value, ok := runtimeReader.Value(name, nil)
	require.Truef(t, ok, "chartengine runtime metric %q is missing", name)
	assert.Zero(t, value, "source-complete stock fixture must not create fallback charts")

	seenChartIDs := make(map[string]string)
	var pythonGCGenerations []string
	identityCounts := map[string]int{
		"prometheus.vllm.request_lifecycle.outcomes":              0,
		"prometheus.vllm.kv_offloading.transfer_operations":       0,
		"prometheus.vllm.mooncake_connector.volume.keys":          0,
		"prometheus.vllm.diffusion_decoding.denoising_steps":      0,
		"prometheus.vllm.websocket_service.connection_lifecycle":  0,
		"prometheus.vllm.fastapi.http_endpoints.request_outcomes": 0,
		"prometheus.vllm.tool_parsing.invocations":                0,
		"prometheus.vllm.process_runtime.process_cpu":             0,
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
			assert.NotEmpty(t, create.Labels["finished_reason"])
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.kv_offloading.transfer_operations":
			assert.Equal(t, "example-model", create.Labels["model_name"])
			assert.Equal(t, "0", create.Labels["engine"])
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.mooncake_connector.volume.keys":
			assert.Equal(t, "example-model", create.Labels["model_name"])
			assert.Equal(t, "0", create.Labels["engine"])
			assert.NotEmpty(t, create.Labels["operation"])
			assert.NotEmpty(t, create.Labels["status"])
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.diffusion_decoding.denoising_steps":
			assert.Equal(t, "example-model", create.Labels["model_name"])
			assert.Equal(t, "0", create.Labels["engine"])
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.websocket_service.connection_lifecycle":
			assert.NotContains(t, create.Labels, "model_name")
			assert.NotContains(t, create.Labels, "engine")
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.fastapi.http_endpoints.request_outcomes":
			assert.NotEmpty(t, create.Labels["handler"])
			assert.NotEmpty(t, create.Labels["method"])
			assert.NotContains(t, create.Labels, "model_name")
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.tool_parsing.invocations":
			assert.Equal(t, "example-model", create.Labels["model_name"])
			assert.NotEmpty(t, create.Labels["request_type"])
			assert.NotEmpty(t, create.Labels["mode"])
			assert.NotEmpty(t, create.Labels["outcome"])
			assert.NotContains(t, create.Labels, "engine")
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.process_runtime.process_cpu":
			assert.NotContains(t, create.Labels, "model_name")
			assert.NotContains(t, create.Labels, "engine")
			identityCounts[create.Meta.Context]++
		case "prometheus.vllm.process_runtime.python_gc.collections":
			generation := create.Labels["generation"]
			assert.NotEmpty(t, generation)
			pythonGCGenerations = append(pythonGCGenerations, generation)
			assert.NotContains(t, create.Labels, "model_name")
			assert.NotContains(t, create.Labels, "engine")
			identityCounts[create.Meta.Context]++
		}
	}
	assert.NotEmpty(t, seenChartIDs)
	assert.Equal(t, 5, identityCounts["prometheus.vllm.request_lifecycle.outcomes"])
	assert.Equal(t, 1, identityCounts["prometheus.vllm.kv_offloading.transfer_operations"])
	assert.Equal(t, 3, identityCounts["prometheus.vllm.mooncake_connector.volume.keys"])
	assert.Equal(t, 1, identityCounts["prometheus.vllm.diffusion_decoding.denoising_steps"])
	assert.Equal(t, 1, identityCounts["prometheus.vllm.websocket_service.connection_lifecycle"])
	assert.Equal(t, 2, identityCounts["prometheus.vllm.fastapi.http_endpoints.request_outcomes"])
	assert.Equal(t, 12, identityCounts["prometheus.vllm.tool_parsing.invocations"])
	assert.Equal(t, 1, identityCounts["prometheus.vllm.process_runtime.process_cpu"])
	assert.Equal(t, 3, identityCounts["prometheus.vllm.process_runtime.python_gc.collections"])
	assert.ElementsMatch(t, []string{"0", "1", "2"}, pythonGCGenerations)

	curated := map[string][]string{
		"prometheus.vllm.request_lifecycle.outcomes":                  {"requests"},
		"prometheus.vllm.request_lifecycle.corrupted_requests":        {"corrupted"},
		"prometheus.vllm.scheduler.request_state":                     {"running", "waiting_capacity", "waiting_deferred"},
		"prometheus.vllm.prefill.prompt_tokens_by_source":             {"local_compute", "local_cache_hit", "external_kv_transfer"},
		"prometheus.vllm.decode.inter_token_intervals":                {"intervals"},
		"prometheus.vllm.engine_execution.estimated_memory_bandwidth": {"read", "write"},
		"prometheus.vllm.kv_cache.local_prefix":                       {"queries", "hits"},
		"prometheus.vllm.kv_cache_residency.events":                   {"evictions", "reuse_gaps"},
		"prometheus.vllm.kv_offloading.cpu_cache_usage":               {"total", "writes", "reads"},
		"prometheus.vllm.nixl_connector.failures":                     {"transfers", "notifications"},
		"prometheus.vllm.hf3fs_connector.failures":                    {"saves", "loads"},
		"prometheus.vllm.mooncake_connector.volume.keys":              {"keys"},
		"prometheus.vllm.speculative_decoding.accepted_by_position":   {"0", "1"},
		"prometheus.vllm.diffusion_decoding.committed_tokens":         {"committed"},
		"prometheus.vllm.websocket_service.connection_lifecycle":      {"opened", "closed"},
		"prometheus.vllm.fastapi.http_endpoints.request_outcomes":     {"2xx"},
		"prometheus.vllm.fastapi.http_service.request_measurements":   {"requests"},
		"prometheus.vllm.tool_parsing.invocations":                    {"invocations"},
		"prometheus.vllm.process_runtime.process_cpu":                 {"used"},
		"prometheus.vllm.process_runtime.python_gc.collections":       {"collections"},
	}
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{RequiredContexts: curated})
}

func testCollectorStockProfileAllMetrics(
	t *testing.T,
	fixture string,
	configure func(*Collector) []string,
	contextPrefix string,
	expectedUnmatched float64,
	requiredContexts map[string][]string,
	inspectStore func(metrix.Reader),
	inspectPlan func(chartengine.Plan),
) {
	t.Helper()

	input, err := os.ReadFile(promtestutil.Require(t, fixture))
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(input) }))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	collr.Profiles = ProfilesConfig{Mode: "auto"}
	var expectedProfiles []string
	if configure != nil {
		expectedProfiles = configure(collr)
	}
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	if expectedProfiles != nil {
		requireSelectedProfiles(t, collr, expectedProfiles...)
	}

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
	name := "netdata.go.plugin.framework.chartengine.series_autogen_matched_total"
	value, ok := runtimeReader.Value(name, nil)
	require.Truef(t, ok, "chartengine runtime metric %q is missing", name)
	assert.Zero(t, value, "source-complete stock fixture must not create fallback charts")
	name = "netdata.go.plugin.framework.chartengine.series_unmatched_total"
	value, ok = runtimeReader.Value(name, nil)
	require.Truef(t, ok, "chartengine runtime metric %q is missing", name)
	assert.Equal(t, expectedUnmatched, value,
		"raw unmatched series must agree with the profile's semantically adjudicated expectation")

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

// TestCollector_VLLMRayTransportAllMetrics proves the unified profile against
// the Ray transport's source-derived structural union. Profile relabeling drops
// compatibility aliases, normalizes the remaining names, and removes
// pre-canonical KV-offload duplicates.
func TestCollector_VLLMRayTransportAllMetrics(t *testing.T) {
	testCollectorStockProfileAllMetrics(
		t,
		"prometheus/profiles/vllm/fixtures/vllm_ray_all_metrics.prom",
		func(collr *Collector) []string {
			return configureProfileJobFromMetadata(t, collr, "collector-go.d.plugin-prometheus-vllm", "vllm", "vllm-ray")
		},
		"prometheus.vllm.",
		22,
		map[string][]string{
			"prometheus.vllm.scheduler.request_state":                   {"running", "waiting_capacity", "waiting_deferred"},
			"prometheus.vllm.request_lifecycle.outcomes":                {"requests"},
			"prometheus.vllm.kv_offloading.transfer_bytes":              {"loaded", "stored"},
			"prometheus.vllm.kv_offloading.admission_outcomes":          {"allocation_failures", "stores_skipped"},
			"prometheus.vllm.speculative_decoding.token_outcomes":       {"proposed", "accepted"},
			"prometheus.vllm.speculative_decoding.accepted_by_position": {"0", "1"},
			"prometheus.vllm.diffusion_decoding.committed_tokens":       {"committed"},
			"prometheus.vllm.nixl_connector.failures":                   {"transfers", "notifications"},
			"prometheus.vllm.hf3fs_connector.failures":                  {"saves", "loads"},
			"prometheus.vllm.mooncake_connector.volume.keys":            {"keys"},
		},
		func(reader metrix.Reader) {
			var sawCanonicalCounter bool
			reader.ForEachSeries(func(name string, _ metrix.LabelView, _ metrix.SampleValue) {
				assert.NotEqual(t, "vllm:request_success", name,
					"Ray compatibility gauge survived profile relabeling")
				assert.Falsef(t, strings.HasPrefix(name, "vllm:kv_offload_size_"),
					"deprecated Ray KV-offload histogram component %q survived profile relabeling", name)
				if name == "vllm:request_success_total" {
					sawCanonicalCounter = true
				}
			})
			assert.True(t, sawCanonicalCounter, "Ray profile relabeling must retain canonical _total counters")
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
				require.Len(t, values, 2)
				for _, name := range []string{"loaded", "stored"} {
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
		func(collr *Collector) []string {
			return configureProfileJobFromMetadata(t, collr, "collector-go.d.plugin-prometheus-litellm", "litellm", "litellm",
				"process_runtime", "python_gc")
		},
		"prometheus.litellm.",
		24,
		map[string][]string{
			"prometheus.litellm.gateway.client_traffic.in_flight_requests":      {"requests"},
			"prometheus.litellm.gateway.client_traffic.request_outcomes":        {"requests"},
			"prometheus.litellm.gateway.client_traffic.failure_causes":          {"failures"},
			"prometheus.litellm.gateway.client_traffic.rate_limit_attribution":  {"failures"},
			"prometheus.litellm.gateway.request_latency.measurements":           {"requests"},
			"prometheus.litellm.routing.deployment_requests.workload":           {"requests"},
			"prometheus.litellm.routing.deployment_requests.outcomes":           {"successful", "failed"},
			"prometheus.litellm.routing.deployment_requests.failure_causes":     {"failures"},
			"prometheus.litellm.routing.deployment_health.state":                {"state"},
			"prometheus.litellm.routing.deployment_capacity.remaining_requests": {"remaining"},
			"prometheus.litellm.routing.fallbacks.outcomes":                     {"successful", "failed"},
			"prometheus.litellm.provider_api.latency_measurements":              {"requests"},
			"prometheus.litellm.usage.deployment.request_token_throughput":      {"input", "output"},
			"prometheus.litellm.usage.api_key.spend":                            {"spend"},
			"prometheus.litellm.caching.outcomes":                               {"hits", "misses"},
			"prometheus.litellm.governance.api_keys.remaining_budget":           {"remaining"},
			"prometheus.litellm.guardrails.invocations":                         {"invocations"},
			"prometheus.litellm.mcp.tool_calls":                                 {"calls"},
			"prometheus.litellm.managed.batches_created":                        {"created"},
			"prometheus.litellm.inventory.callback_logging_failures":            {"failures"},
			"prometheus.litellm.internal_services.request_outcomes":             {"successful", "failed"},
			"prometheus.litellm.internal_services.failure_causes":               {"auth", "redis"},
			"prometheus.litellm.process_runtime.process_cpu":                    {"used"},
			"prometheus.litellm.process_runtime.python_gc.collections":          {"collections"},
		},
		nil,
		func(plan chartengine.Plan) {
			creates := make(map[string][]chartengine.CreateChartAction)
			dimensions := make(map[string]map[string]struct{})
			for _, action := range plan.Actions {
				switch action := action.(type) {
				case chartengine.CreateChartAction:
					creates[action.Meta.Context] = append(creates[action.Meta.Context], action)
				case chartengine.CreateDimensionAction:
					if dimensions[action.ChartMeta.Context] == nil {
						dimensions[action.ChartMeta.Context] = make(map[string]struct{})
					}
					dimensions[action.ChartMeta.Context][action.Name] = struct{}{}
				}
			}

			requireIdentity := func(context string, labels ...string) {
				t.Helper()
				require.NotEmpty(t, creates[context], "context %q", context)
				for _, chart := range creates[context] {
					for _, label := range labels {
						assert.NotEmptyf(t, chart.Labels[label], "context %q chart %q identity %q", context, chart.ChartID, label)
					}
				}
			}

			const (
				requestOutcomes = "prometheus.litellm.gateway.client_traffic.request_outcomes"
				failureCauses   = "prometheus.litellm.gateway.client_traffic.failure_causes"
				rateLimits      = "prometheus.litellm.gateway.client_traffic.rate_limit_attribution"
				requestLatency  = "prometheus.litellm.gateway.request_latency.measurements"
				fallbacks       = "prometheus.litellm.routing.fallbacks.outcomes"
				guardrails      = "prometheus.litellm.guardrails.invocations"
				serviceOutcomes = "prometheus.litellm.internal_services.request_outcomes"
				serviceFailures = "prometheus.litellm.internal_services.failure_causes"
				pythonGC        = "prometheus.litellm.process_runtime.python_gc.collections"
			)

			requireIdentity(requestOutcomes, "route", "status_class")
			requireIdentity(failureCauses, "status_class", "exception_class")
			requireIdentity(rateLimits, "rate_limit_category", "rate_limit_type")
			requireIdentity(requestLatency, "model_id", "service_tier")
			requireIdentity(fallbacks, "requested_model", "fallback_model")
			requireIdentity(guardrails, "guardrail_name", "hook_type", "status")
			requireIdentity(serviceOutcomes, "service")
			requireIdentity(serviceFailures, "function_name", "error_class")
			requireIdentity(pythonGC, "generation")

			valuesFor := func(context, label string) []string {
				t.Helper()
				seen := make(map[string]struct{})
				for _, chart := range creates[context] {
					if value := chart.Labels[label]; value != "" {
						seen[value] = struct{}{}
					}
				}
				values := make([]string, 0, len(seen))
				for value := range seen {
					values = append(values, value)
				}
				return values
			}
			assert.ElementsMatch(t, []string{"2xx", "3xx", "4xx", "5xx", "unclassified"},
				valuesFor(requestOutcomes, "status_class"))
			assert.ElementsMatch(t, []string{"priority", "None"}, valuesFor(requestLatency, "service_tier"))
			assert.ElementsMatch(t, []string{"success", "intervened", "error"}, valuesFor(guardrails, "status"))
			assert.ElementsMatch(t, []string{"0", "1", "2"}, valuesFor(pythonGC, "generation"))

			for _, context := range []string{requestOutcomes, failureCauses, rateLimits} {
				for _, chart := range creates[context] {
					for _, label := range []string{"status_code", "exception_status", "client_ip", "user_email", "pid"} {
						assert.NotContainsf(t, chart.Labels, label, "context %q chart %q leaked source label %q",
							context, chart.ChartID, label)
					}
				}
			}
			for _, chart := range creates[fallbacks] {
				assert.NotContains(t, chart.Labels, "model_id")
			}

			assert.Contains(t, dimensions[serviceOutcomes], "successful")
			assert.Contains(t, dimensions[serviceOutcomes], "failed")
			assert.Contains(t, dimensions[serviceFailures], "auth")
			assert.Contains(t, dimensions[serviceFailures], "redis")

			for context := range creates {
				assert.NotContains(t, context, "managed_file_size")
				assert.NotContains(t, context, "jobs_polled")
				assert.NotContains(t, context, "queue_size")
				assert.NotContains(t, context, "pod_lock")
			}
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
