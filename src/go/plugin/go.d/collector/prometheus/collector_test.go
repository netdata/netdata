// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/testutil"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
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
# TYPE ceph_health_detail gauge
ceph_health_detail{name="RECENT_CRASH",severity="HEALTH_WARN"} 1
# TYPE ceph_daemon_health_metrics gauge
ceph_daemon_health_metrics{ceph_daemon="osd.0",type="SLOW_OPS"} 1
# TYPE ceph_healthcheck_slow_ops gauge
ceph_healthcheck_slow_ops 1
`
	testCollectorStockProfileModes(
		t,
		"ceph",
		input,
		map[string][]string{
			"prometheus.ceph.health.cluster_status.state":               {"value"},
			"prometheus.ceph.health.health_checks.state":                {"value"},
			"prometheus.ceph.health.daemon_health.item_count":           {"value"},
			"prometheus.ceph.health.slow_operations.current_operations": {"value"},
		},
		map[string][]string{
			"prometheus.ceph_health_status":         {"ceph_health_status"},
			"prometheus.ceph_health_detail":         {"ceph_health_detail"},
			"prometheus.ceph_daemon_health_metrics": {"ceph_daemon_health_metrics"},
			"prometheus.ceph_healthcheck_slow_ops":  {"ceph_healthcheck_slow_ops"},
		},
	)
}

func TestCollector_CephProfileFamilySegments(t *testing.T) {
	input, err := os.ReadFile(promtestutil.Require(t, "prometheus/profiles/ceph/fixtures/ceph_squid_mgr_limit11.prom"))
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(input)
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	spec, _, err := charttpl.DecodeYAMLValidated([]byte(collr.ChartTemplateYAML()))
	require.NoError(t, err)

	var families []string
	var collectFamilies func(charttpl.Group, []string)
	collectFamilies = func(group charttpl.Group, parents []string) {
		parts := parents
		if family := strings.TrimSpace(group.Family); family != "" {
			parts = append(append([]string(nil), parents...), family)
		}
		families = append(families, strings.Join(parts, "/"))
		for _, chart := range group.Charts {
			if family := strings.TrimSpace(chart.Family); family != "" {
				families = append(families, strings.Join(append(append([]string(nil), parts...), family), "/"))
			} else if len(parts) != 0 {
				families = append(families, strings.Join(parts, "/"))
			}
		}
		for _, child := range group.Groups {
			collectFamilies(child, parts)
		}
	}
	for _, group := range spec.Groups {
		collectFamilies(group, nil)
	}
	require.NotEmpty(t, families)

	for _, family := range families {
		for segment := range strings.SplitSeq(family, "/") {
			require.NotContains(t, []string{"I", "O"}, segment,
				"family %q uses the reserved path segment %q", family, segment)
		}
	}
}

func TestCollector_CephMGRReleaseFixturesMaterializeM01AlertIdentities(t *testing.T) {
	const (
		healthChecksContext = "prometheus.ceph.health.health_checks.state"
		daemonHealthContext = "prometheus.ceph.health.daemon_health.item_count"
	)
	wantHealthCheckNames := []string{
		"RECENT_CRASH",
		"RECENT_MGR_MODULE_CRASH",
		"CEPHADM_FAILED_DAEMON",
		"CEPHADM_PAUSED",
		"UPGRADE_EXCEPTION",
	}
	fixtures := []string{
		"prometheus/profiles/ceph/fixtures/ceph_reef_mgr_limit11.prom",
		"prometheus/profiles/ceph/fixtures/ceph_squid_mgr_limit11.prom",
		"prometheus/profiles/ceph/fixtures/ceph_tentacle_mgr_limit11.prom",
	}

	for _, fixture := range fixtures {
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			input, err := os.ReadFile(promtestutil.Require(t, fixture))
			require.NoError(t, err)

			plan := collectCephPlan(t, input).plan
			var healthCheckNames, daemonHealthTypes []string
			for _, item := range plan.Actions {
				create, ok := item.(chartengine.CreateChartAction)
				if !ok {
					continue
				}
				switch create.Meta.Context {
				case healthChecksContext:
					healthCheckNames = append(healthCheckNames, create.Labels["name"])
				case daemonHealthContext:
					daemonHealthTypes = append(daemonHealthTypes, create.Labels["type"])
				}
			}

			assert.ElementsMatch(t, wantHealthCheckNames, healthCheckNames)
			assert.Equal(t, []string{"SLOW_OPS"}, daemonHealthTypes)
		})
	}
}

func TestCollector_CephMGRNamedHealthCheckIdentities(t *testing.T) {
	manifest := loadCephAlertManifest(t)
	var want []string
	for _, mapping := range manifest.Alerts {
		if mapping.Netdata.Disposition == "reuse" || mapping.Netdata.Disposition == "internal-helper" ||
			mapping.Netdata.Context != "prometheus.ceph.health.health_checks.state" {
			continue
		}
		name, ok := strings.CutPrefix(mapping.Netdata.Labels, "name=")
		require.True(t, ok)
		want = append(want, name)
	}

	var input strings.Builder
	input.WriteString("# TYPE ceph_health_detail gauge\n")
	for _, name := range want {
		fmt.Fprintf(&input, "ceph_health_detail{name=\"%s\",severity=\"HEALTH_WARN\"} 1\n", name)
	}
	plan := collectCephPlan(t, []byte(input.String())).plan

	var actual []string
	for _, action := range plan.Actions {
		create, ok := action.(chartengine.CreateChartAction)
		if ok && create.Meta.Context == "prometheus.ceph.health.health_checks.state" {
			actual = append(actual, create.Labels["name"])
		}
	}
	assert.ElementsMatch(t, want, actual)
}

func TestCollector_CephMGRClusterSummaries(t *testing.T) {
	var input strings.Builder
	input.WriteString("# TYPE ceph_mon_metadata gauge\n# TYPE ceph_mon_quorum_status gauge\n")
	for i := range 3 {
		fmt.Fprintf(&input,
			"ceph_mon_metadata{ceph_daemon=\"mon.%c\",ceph_version=\"19.2.5\",hostname=\"mon-host-%d\",public_addr=\"192.0.2.%d:3300\",rank=\"%d\"} 1\n",
			'a'+i, i, i+1, i)
		quorum := 1
		if i == 2 {
			quorum = 0
		}
		fmt.Fprintf(&input, "ceph_mon_quorum_status{ceph_daemon=\"mon.%c\"} %d\n", 'a'+i, quorum)
	}
	input.WriteString("# TYPE ceph_osd_metadata gauge\n# TYPE ceph_osd_up gauge\n")
	for i := range 10 {
		fmt.Fprintf(&input,
			"ceph_osd_metadata{back_iface=\"back%d\",ceph_daemon=\"osd.%d\",ceph_version=\"19.2.5\",cluster_addr=\"192.0.2.%d:6800\",device_class=\"ssd\",front_iface=\"front%d\",hostname=\"osd-host-%d\",objectstore=\"bluestore\",public_addr=\"198.51.100.%d:6800\"} 1\n",
			i, i, i+1, i, i, i+1)
		up := 1
		if i == 9 {
			up = 0
		}
		fmt.Fprintf(&input, "ceph_osd_up{ceph_daemon=\"osd.%d\"} %d\n", i, up)
	}

	plan := collectCephPlan(t, []byte(input.String())).plan
	const (
		monSummary  = "prometheus.ceph.control_plane.monitor.cluster_summary"
		monMetadata = "prometheus.ceph.control_plane.monitor.state_metadata"
		monQuorum   = "prometheus.ceph.control_plane.monitor.state_quorum_status"
		osdSummary  = "prometheus.ceph.osd_capacity_and_state.osd_state.cluster_summary"
		osdMetadata = "prometheus.ceph.osd_capacity_and_state.metadata.state"
		osdState    = "prometheus.ceph.osd_capacity_and_state.osd_state.state"
	)
	creates := make(map[string][]chartengine.CreateChartAction)
	updates := make(map[string]chartengine.UpdateChartAction)
	for _, action := range plan.Actions {
		switch action := action.(type) {
		case chartengine.CreateChartAction:
			creates[action.Meta.Context] = append(creates[action.Meta.Context], action)
		case chartengine.UpdateChartAction:
			updates[action.ChartID] = action
		}
	}

	require.Len(t, creates[monSummary], 1)
	require.Len(t, creates[osdSummary], 1)
	assert.Equal(t, "monitors", creates[monSummary][0].Meta.Units)
	assert.Equal(t, "OSDs", creates[osdSummary][0].Meta.Units)
	assert.Len(t, creates[monMetadata], 3, "cluster summary must not replace monitor member charts")
	assert.Len(t, creates[monQuorum], 3, "cluster summary must not replace monitor quorum charts")
	assert.Len(t, creates[osdMetadata], 10, "cluster summary must not replace OSD metadata charts")
	assert.Len(t, creates[osdState], 10, "cluster summary must not replace OSD state charts")

	requireChartValues := func(chart chartengine.CreateChartAction, expected map[string]float64) {
		t.Helper()
		update, ok := updates[chart.ChartID]
		require.Truef(t, ok, "chart %q has no update", chart.ChartID)
		assert.Equal(t, expected, chartUpdateValues(t, update))
	}
	requireChartValues(creates[monSummary][0], map[string]float64{"total": 3, "in_quorum": 2})
	requireChartValues(creates[osdSummary][0], map[string]float64{"total": 10, "up": 9})
}

func TestCollector_CephM05RBDMirrorTimestampBoundaries(t *testing.T) {
	drift := func(local, remote float64) float64 {
		if math.IsNaN(local) || math.IsInf(local, 0) || math.IsNaN(remote) || math.IsInf(remote, 0) ||
			local < 0 || remote < 0 {
			return math.NaN()
		}
		return local - remote
	}
	for _, tc := range []struct {
		name      string
		local     float64
		remote    float64
		want      float64
		unsynced  bool
		undefined bool
	}{
		{name: "invalid NaN local", local: math.NaN(), remote: 1, undefined: true},
		{name: "invalid infinite remote", local: 1, remote: math.Inf(1), undefined: true},
		{name: "negative local timestamp", local: -1, remote: 1, undefined: true},
		{name: "synchronized", local: 100, remote: 100},
		{name: "local behind", local: 99, remote: 100, want: -1, unsynced: true},
		{name: "local ahead", local: 101, remote: 100, want: 1, unsynced: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := drift(tc.local, tc.remote)
			if tc.undefined {
				assert.True(t, math.IsNaN(got))
				return
			}
			assert.InDelta(t, tc.want, got, 1e-12)
			assert.Equal(t, tc.unsynced, got != 0)
		})
	}
}

func TestCollector_CephM05RBDMirrorTimestampMaterialization(t *testing.T) {
	var input strings.Builder
	input.WriteString("# TYPE ceph_rbd_mirror_snapshot_image_local_timestamp gauge\n")
	input.WriteString("# TYPE ceph_rbd_mirror_snapshot_image_remote_timestamp gauge\n")
	input.WriteString("ceph_rbd_mirror_snapshot_image_local_timestamp{ceph_daemon=\"rbd-mirror.a\",image=\"synced\",namespace=\"ns\",pool=\"pool\"} 100\n")
	input.WriteString("ceph_rbd_mirror_snapshot_image_remote_timestamp{ceph_daemon=\"rbd-mirror.a\",image=\"synced\",namespace=\"ns\",pool=\"pool\"} 100\n")
	input.WriteString("ceph_rbd_mirror_snapshot_image_local_timestamp{ceph_daemon=\"rbd-mirror.a\",image=\"ahead\",namespace=\"ns\",pool=\"pool\"} 101\n")
	input.WriteString("ceph_rbd_mirror_snapshot_image_remote_timestamp{ceph_daemon=\"rbd-mirror.a\",image=\"ahead\",namespace=\"ns\",pool=\"pool\"} 100\n")
	input.WriteString("ceph_rbd_mirror_snapshot_image_local_timestamp{ceph_daemon=\"rbd-mirror.a\",image=\"behind\",namespace=\"other\",pool=\"other\"} 99\n")
	input.WriteString("ceph_rbd_mirror_snapshot_image_remote_timestamp{ceph_daemon=\"rbd-mirror.a\",image=\"behind\",namespace=\"other\",pool=\"other\"} 100\n")

	plan := collectCephPlan(t, []byte(input.String())).plan
	const context = "prometheus.ceph.rbd_mirror.snapshot_replication.timestamp"
	chartContexts := make(map[string]string)
	updates := make(map[string]chartengine.UpdateChartAction)
	for _, action := range plan.Actions {
		create, ok := action.(chartengine.CreateChartAction)
		if ok {
			chartContexts[create.ChartID] = create.Meta.Context
		}
		update, ok := action.(chartengine.UpdateChartAction)
		if ok && chartContexts[update.ChartID] == context {
			updates[update.ChartID] = update
		}
	}
	require.Len(t, updates, 3, "each mirrored image retains its own chart identity")

	for chartID, update := range updates {
		actual := chartUpdateValues(t, update)
		require.Len(t, actual, 2, "chart %q must expose both timestamps", chartID)
		switch actual["local_timestamp"] {
		case 100:
			assert.EqualValues(t, 100, actual["remote_timestamp"])
		case 101:
			assert.EqualValues(t, 100, actual["remote_timestamp"])
		case 99:
			assert.EqualValues(t, 100, actual["remote_timestamp"])
		default:
			t.Fatalf("unexpected timestamp chart values %q: %v", chartID, actual)
		}
	}
}

func TestCollector_CephM04DerivedBoundaries(t *testing.T) {
	difference := func(total, partial float64) float64 {
		if math.IsNaN(total) || math.IsInf(total, 0) || math.IsNaN(partial) || math.IsInf(partial, 0) ||
			total < 0 || partial < 0 || partial > total {
			return math.NaN()
		}
		return total - partial
	}
	for _, tc := range []struct {
		name           string
		total, partial float64
		want           float64
		active         bool
		undefined      bool
	}{
		{name: "invalid NaN total", total: math.NaN(), partial: 1, undefined: true},
		{name: "invalid infinite partial", total: 10, partial: math.Inf(1), undefined: true},
		{name: "negative total", total: -1, partial: 0, undefined: true},
		{name: "partial exceeds total", total: 1, partial: 2, undefined: true},
		{name: "zero pool", total: 0, partial: 0},
		{name: "healthy pool", total: 100, partial: 100},
		{name: "one inactive or unclean PG", total: 100, partial: 99, want: 1, active: true},
		{name: "all inactive or unclean PGs", total: 100, partial: 0, want: 100, active: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := difference(tc.total, tc.partial)
			if tc.undefined {
				assert.True(t, math.IsNaN(got))
				return
			}
			assert.InDelta(t, tc.want, got, 1e-12)
			assert.Equal(t, tc.active, got > 0)
		})
	}

	allUp := func(total, up float64) float64 {
		if math.IsNaN(total) || math.IsInf(total, 0) || math.IsNaN(up) || math.IsInf(up, 0) ||
			total <= 0 || up < 0 || up > total {
			return math.NaN()
		}
		if up == total {
			return 1
		}
		return 0
	}
	for _, tc := range []struct {
		name      string
		total, up float64
		want      float64
		undefined bool
	}{
		{name: "invalid NaN up", total: 10, up: math.NaN(), undefined: true},
		{name: "zero population", total: 0, up: 0, undefined: true},
		{name: "all OSDs up", total: 10, up: 10, want: 1},
		{name: "one OSD down", total: 10, up: 9},
	} {
		t.Run("all up "+tc.name, func(t *testing.T) {
			got := allUp(tc.total, tc.up)
			if tc.undefined {
				assert.True(t, math.IsNaN(got))
				return
			}
			assert.InDelta(t, tc.want, got, 1e-12)
		})
	}

	blockingIO := func(pgAvailability, osdDown float64) float64 {
		if math.IsNaN(pgAvailability) || math.IsInf(pgAvailability, 0) {
			return math.NaN()
		}
		if pgAvailability != 1 {
			return 0
		}
		if osdDown == 1 {
			return 0
		}
		if math.IsNaN(osdDown) || math.IsInf(osdDown, 0) {
			return math.NaN()
		}
		return 1
	}
	for _, tc := range []struct {
		name              string
		availability, osd float64
		want              float64
		undefined         bool
	}{
		{name: "availability with OSD down", availability: 1, osd: 1},
		{name: "availability without OSD down", availability: 1, osd: 0, want: 1},
		{name: "no availability", availability: 0, osd: 1},
		{name: "inactive condition clears without evaluating OSD state", availability: 0, osd: math.NaN()},
		{name: "active condition with unresolved OSD state", availability: 1, osd: math.NaN(), undefined: true},
		{name: "active condition with infinite OSD state", availability: 1, osd: math.Inf(1), undefined: true},
	} {
		t.Run("blocking I/O "+tc.name, func(t *testing.T) {
			got := blockingIO(tc.availability, tc.osd)
			if tc.undefined {
				assert.True(t, math.IsNaN(got))
				return
			}
			assert.InDelta(t, tc.want, got, 1e-12)
		})
	}
}

func TestCollector_CephM04PoolDifferenceMaterialization(t *testing.T) {
	var input strings.Builder
	input.WriteString("# TYPE ceph_pg_active gauge\n# TYPE ceph_pg_clean gauge\n# TYPE ceph_pg_total gauge\n")
	for pool, values := range map[string][3]float64{
		"healthy": {100, 100, 100},
		"partial": {100, 99, 100},
		"zero":    {0, 0, 0},
	} {
		fmt.Fprintf(&input, "ceph_pg_active{pool_id=%q} %v\n", pool, values[0])
		fmt.Fprintf(&input, "ceph_pg_clean{pool_id=%q} %v\n", pool, values[1])
		fmt.Fprintf(&input, "ceph_pg_total{pool_id=%q} %v\n", pool, values[2])
	}

	plan := collectCephPlan(t, []byte(input.String())).plan
	const context = "prometheus.ceph.placement_groups_and_recovery.healthy_state.pg_state"
	chartContexts := make(map[string]string)
	updates := make(map[string]chartengine.UpdateChartAction)
	for _, action := range plan.Actions {
		if create, ok := action.(chartengine.CreateChartAction); ok {
			chartContexts[create.ChartID] = create.Meta.Context
		}
		update, ok := action.(chartengine.UpdateChartAction)
		if ok && chartContexts[update.ChartID] == context {
			updates[update.ChartID] = update
		}
	}
	require.Len(t, updates, 3, "one PG chart instance must remain per pool")
	for chartID, update := range updates {
		actual := chartUpdateValues(t, update)
		switch actual["pg_total"] {
		case 100:
			switch actual["clean"] {
			case 100:
				assert.Equal(t, map[string]float64{"active": 100, "clean": 100, "pg_total": 100}, actual)
			case 99:
				assert.Equal(t, map[string]float64{"active": 100, "clean": 99, "pg_total": 100}, actual)
			default:
				t.Fatalf("unexpected healthy-total PG chart values %q: %v", chartID, actual)
			}
		case 0:
			assert.Equal(t, map[string]float64{"active": 0, "clean": 0, "pg_total": 0}, actual)
		default:
			t.Fatalf("unexpected PG chart values %q: %v", chartID, actual)
		}
	}
}

func TestCollector_CephHardwareProfileReleaseGating(t *testing.T) {
	fixtures := []string{
		"prometheus/profiles/ceph/fixtures/ceph_reef_mgr_limit11.prom",
		"prometheus/profiles/ceph/fixtures/ceph_squid_mgr_limit11.prom",
		"prometheus/profiles/ceph/fixtures/ceph_tentacle_mgr_limit11.prom",
	}

	for _, fixturePath := range fixtures {
		t.Run(filepath.Base(fixturePath), func(t *testing.T) {
			input, err := os.ReadFile(promtestutil.Require(t, fixturePath))
			require.NoError(t, err)

			plan := collectCephPlan(t, input).plan
			healthIDs := make(map[string]chartengine.CreateChartAction)
			temperatureIDs := make(map[string]chartengine.CreateChartAction)
			for _, action := range plan.Actions {
				create, ok := action.(chartengine.CreateChartAction)
				if !ok {
					continue
				}
				switch create.Meta.Context {
				case "prometheus.ceph.node_hardware.health.state":
					healthIDs[create.ChartID] = create
				case "prometheus.ceph.node_hardware.temperature.temperature":
					temperatureIDs[create.ChartID] = create
				}
			}

			isTentacle := strings.Contains(fixturePath, "_tentacle_")
			if !isTentacle {
				assert.Empty(t, healthIDs, "Reef/Squid must not materialize Tentacle-only hardware charts")
				assert.Empty(t, temperatureIDs, "Reef/Squid must not materialize Tentacle-only temperature charts")
				return
			}

			assert.Len(t, healthIDs, 9, "each collision-bearing health component remains a separate instance")
			assert.Len(t, temperatureIDs, 8, "each collision-bearing temperature sensor remains a separate instance")

			wantHealth := []string{
				"prometheus_ceph_node_hardware_health_state_node-a_storage_drive-0",
				"prometheus_ceph_node_hardware_health_state_node-a_processors_cpu-0",
				"prometheus_ceph_node_hardware_health_state_node-a_memory_dimm-0",
				"prometheus_ceph_node_hardware_health_state_node-a_power_psu-0",
				"prometheus_ceph_node_hardware_health_state_node-a_network_nic-0",
				"prometheus_ceph_node_hardware_health_state_node-a_fans_fan-0",
				"prometheus_ceph_node_hardware_health_state_node-a_temperatures_temp-0",
				"prometheus_ceph_node_hardware_health_state_node-b_storage_drive-0",
				"prometheus_ceph_node_hardware_health_state_node-b_storage_drive-1",
			}
			assert.ElementsMatch(t, wantHealth, slices.Collect(maps.Keys(healthIDs)))

			wantTemperature := []string{
				"prometheus_ceph_node_hardware_temperature_temperature_node-a_MB_TEMP_0",
				"prometheus_ceph_node_hardware_temperature_temperature_node-a_DIMM_0_TEMP",
				"prometheus_ceph_node_hardware_temperature_temperature_node-a_CPU_TEMP_0",
				"prometheus_ceph_node_hardware_temperature_temperature_node-a_NVME_0_TEMP",
				"prometheus_ceph_node_hardware_temperature_temperature_node-b_MB_TEMP_0",
				"prometheus_ceph_node_hardware_temperature_temperature_node-b_DIMM_0_TEMP",
				"prometheus_ceph_node_hardware_temperature_temperature_node-b_CPU_TEMP_0",
				"prometheus_ceph_node_hardware_temperature_temperature_node-b_NVME_0_TEMP",
			}
			assert.ElementsMatch(t, wantTemperature, slices.Collect(maps.Keys(temperatureIDs)))
		})
	}
}

func TestCollector_CephMGRClusterAlertBoundaries(t *testing.T) {
	monState := func(total, inQuorum float64) (down, atRisk float64) {
		if math.IsNaN(total) || math.IsInf(total, 0) || math.IsNaN(inQuorum) || math.IsInf(inQuorum, 0) ||
			total <= 0 || inQuorum < 0 || inQuorum > total {
			return math.NaN(), math.NaN()
		}
		if inQuorum < total && inQuorum*2 > total {
			down = 1
			if (inQuorum-1)*2 <= total {
				atRisk = 1
			}
		}
		return down, atRisk
	}
	for _, tc := range []struct {
		name               string
		total, inQuorum    float64
		wantDown, wantRisk float64
		undefined          bool
	}{
		{name: "invalid NaN total", total: math.NaN(), inQuorum: 2, undefined: true},
		{name: "invalid infinite quorum", total: 3, inQuorum: math.Inf(1), undefined: true},
		{name: "zero population", total: 0, inQuorum: 0, undefined: true},
		{name: "negative quorum", total: 3, inQuorum: -1, undefined: true},
		{name: "quorum exceeds population", total: 3, inQuorum: 4, undefined: true},
		{name: "healthy odd population", total: 3, inQuorum: 3},
		{name: "minimum odd majority", total: 3, inQuorum: 2, wantDown: 1, wantRisk: 1},
		{name: "odd majority above minimum", total: 5, inQuorum: 4, wantDown: 1},
		{name: "minimum five member majority", total: 5, inQuorum: 3, wantDown: 1, wantRisk: 1},
		{name: "minimum even majority", total: 4, inQuorum: 3, wantDown: 1, wantRisk: 1},
		{name: "quorum already lost", total: 4, inQuorum: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			down, risk := monState(tc.total, tc.inQuorum)
			if tc.undefined {
				assert.True(t, math.IsNaN(down))
				assert.True(t, math.IsNaN(risk))
				return
			}
			assert.Equal(t, tc.wantDown, down)
			assert.Equal(t, tc.wantRisk, risk)
		})
	}

	osdDownPercent := func(total, up float64) float64 {
		if math.IsNaN(total) || math.IsInf(total, 0) || math.IsNaN(up) || math.IsInf(up, 0) ||
			total <= 0 || up < 0 || up > total {
			return math.NaN()
		}
		return (total - up) * 100 / total
	}
	for _, tc := range []struct {
		name      string
		total, up float64
		want      float64
		critical  bool
		undefined bool
	}{
		{name: "invalid NaN up", total: 10, up: math.NaN(), undefined: true},
		{name: "zero population", total: 0, up: 0, undefined: true},
		{name: "negative up", total: 10, up: -1, undefined: true},
		{name: "up exceeds population", total: 10, up: 11, undefined: true},
		{name: "all OSDs up", total: 10, up: 10},
		{name: "below threshold", total: 10000, up: 9001, want: 9.99},
		{name: "at threshold", total: 10, up: 9, want: 10, critical: true},
		{name: "above threshold", total: 10, up: 8, want: 20, critical: true},
	} {
		t.Run("osd "+tc.name, func(t *testing.T) {
			value := osdDownPercent(tc.total, tc.up)
			if tc.undefined {
				assert.True(t, math.IsNaN(value))
				return
			}
			assert.InDelta(t, tc.want, value, 1e-12)
			assert.Equal(t, tc.critical, value >= 10)
		})
	}
}

func TestCollector_CephNVMeoFMaterializesLocalAlertIdentities(t *testing.T) {
	input, err := os.ReadFile(promtestutil.Require(t, "prometheus/profiles/ceph/fixtures/ceph_nvmeof.prom"))
	require.NoError(t, err)

	plan := collectCephPlan(t, input).plan
	byContext := make(map[string][]chartengine.CreateChartAction)
	updates := make(map[string]chartengine.UpdateChartAction)
	for _, action := range plan.Actions {
		switch action := action.(type) {
		case chartengine.CreateChartAction:
			byContext[action.Meta.Context] = append(byContext[action.Meta.Context], action)
		case chartengine.UpdateChartAction:
			updates[action.ChartID] = action
		}
	}

	require.Len(t, byContext["prometheus.ceph.nvme_of.subsystems.state_metadata"], 3)
	require.Len(t, byContext["prometheus.ceph.nvme_of.gateways.time_accumulation"], 2)
	require.Len(t, byContext["prometheus.ceph.nvme_of.block_devices.operations"], 2)
	require.Len(t, byContext["prometheus.ceph.nvme_of.block_devices.time_accumulation"], 2)
	require.Len(t, byContext["prometheus.ceph.nvme_of.hosts.keepalive_timeout_state"], 2)
	require.Len(t, byContext["prometheus.ceph.nvme_of.subsystems.state_namespace_metadata"], 4)
	require.Len(t, byContext["prometheus.ceph.nvme_of.subsystems.namespace_capacity"], 2)
	require.Len(t, byContext["prometheus.ceph.nvme_of.subsystems.namespace_count"], 2)
	require.Len(t, byContext["prometheus.ceph.nvme_of.gateways.namespace_count"], 1)

	open := byContext["prometheus.ceph.nvme_of.subsystems.state_metadata"][0]
	for _, create := range byContext["prometheus.ceph.nvme_of.subsystems.state_metadata"] {
		if create.Labels["allow_any_host"] == "yes" {
			open = create
		}
	}
	require.Equal(t, "yes", open.Labels["allow_any_host"])

	chartValues := func(chart chartengine.CreateChartAction) map[string]float64 {
		t.Helper()
		update, ok := updates[chart.ChartID]
		require.Truef(t, ok, "chart %q has no update", chart.ChartID)
		return chartUpdateValues(t, update)
	}
	for _, reactor := range byContext["prometheus.ceph.nvme_of.gateways.time_accumulation"] {
		require.Contains(t, chartValues(reactor), "busy")
		require.Contains(t, chartValues(reactor), "idle")
	}

	for _, bdev := range byContext["prometheus.ceph.nvme_of.block_devices.operations"] {
		values := chartValues(bdev)
		require.Contains(t, values, "reads_completed")
		require.Contains(t, values, "writes_completed")
		require.Greater(t, values["reads_completed"], float64(0))
	}

	gatewayNamespaces := make(map[string]float64)
	for _, gateway := range byContext["prometheus.ceph.nvme_of.gateways.namespace_count"] {
		gatewayNamespaces[gateway.ChartID] = chartValues(gateway)["namespace_count"]
	}
	assert.InDeltaMapValues(t, map[string]float64{"prometheus_ceph_nvme_of_gateways_namespace_count": 3}, gatewayNamespaces, 1e-12)
}

func TestCollector_CephNVMeoFAlertBoundaries(t *testing.T) {
	cpuPercent := func(minimumBusyRate float64) float64 {
		if math.IsNaN(minimumBusyRate) || math.IsInf(minimumBusyRate, 0) || minimumBusyRate < 0 {
			return math.NaN()
		}
		return minimumBusyRate * 100
	}
	for _, tc := range []struct {
		name  string
		busy  float64
		want  float64
		actor bool
		undef bool
	}{
		{name: "invalid NaN busy", busy: math.NaN(), undef: true},
		{name: "negative busy", busy: -0.01, undef: true},
		{name: "exactly eighty does not act", busy: .8, want: 80},
		{name: "above eighty acts", busy: .81, want: 81, actor: true},
	} {
		t.Run("cpu "+tc.name, func(t *testing.T) {
			value := cpuPercent(tc.busy)
			if tc.undef {
				assert.True(t, math.IsNaN(value))
				return
			}
			assert.InDelta(t, tc.want, value, 1e-12)
			assert.Equal(t, tc.actor, value > 80)
		})
	}

	latency := func(secondsPerSecond, operationsPerSecond float64) float64 {
		if math.IsNaN(secondsPerSecond) || math.IsInf(secondsPerSecond, 0) ||
			math.IsNaN(operationsPerSecond) || math.IsInf(operationsPerSecond, 0) || operationsPerSecond <= 0 {
			return math.NaN()
		}
		return secondsPerSecond / operationsPerSecond
	}
	for _, tc := range []struct {
		name                           string
		secondsPerSecond, opsPerSecond float64
		want                           float64
		read, write                    bool
		undef                          bool
	}{
		{name: "invalid NaN time", secondsPerSecond: math.NaN(), opsPerSecond: 1, undef: true},
		{name: "zero denominator", secondsPerSecond: 1, opsPerSecond: 0, undef: true},
		{name: "normal read", secondsPerSecond: .005, opsPerSecond: 1, want: .005},
		{name: "read boundary", secondsPerSecond: .01, opsPerSecond: 1, want: .01},
		{name: "normal write", secondsPerSecond: .015, opsPerSecond: 1, want: .015, read: true},
		{name: "write threshold", secondsPerSecond: .021, opsPerSecond: 1, want: .021, read: true, write: true},
	} {
		t.Run("latency "+tc.name, func(t *testing.T) {
			value := latency(tc.secondsPerSecond, tc.opsPerSecond)
			if tc.undef {
				assert.True(t, math.IsNaN(value))
				return
			}
			assert.InDelta(t, tc.want, value, 1e-12)
			assert.Equal(t, tc.read, value > .01)
			assert.Equal(t, tc.write, value > .02)
		})
	}

	namespaceLimit := func(count, limit float64) bool {
		if math.IsNaN(count) || math.IsInf(count, 0) || math.IsNaN(limit) || math.IsInf(limit, 0) ||
			count < 0 || limit < 0 {
			return false
		}
		return count >= limit
	}
	for _, tc := range []struct {
		name    string
		count   float64
		limit   float64
		reached bool
		undef   bool
	}{
		{name: "invalid negative count", count: -1, limit: 1, undef: true},
		{name: "invalid negative limit", count: 1, limit: -1, undef: true},
		{name: "below limit", count: 1, limit: 2},
		{name: "at limit", count: 2, limit: 2, reached: true},
		{name: "above limit", count: 3, limit: 2, reached: true},
	} {
		t.Run("namespace "+tc.name, func(t *testing.T) {
			got := namespaceLimit(tc.count, tc.limit)
			if tc.undef {
				assert.False(t, got)
				return
			}
			assert.Equal(t, tc.reached, got)
		})
	}
}

func TestCollector_CephRGWAlertChartsMaterialize(t *testing.T) {
	input, err := os.ReadFile(promtestutil.Require(t,
		"prometheus/profiles/ceph/fixtures/ceph_reef_ceph_exporter_limit11.prom"))
	require.NoError(t, err)

	plan := collectCephPlan(t, input).plan
	created := make(map[string]int)
	for _, action := range plan.Actions {
		if create, ok := action.(chartengine.CreateChartAction); ok {
			created[create.Meta.Context]++
		}
	}
	require.Equal(t, 1, created["prometheus.ceph.object_gateway.notifications.events"])
	require.Equal(t, 1, created["prometheus.ceph.object_gateway.notifications.missing_configurations"])
	require.Equal(t, 1, created["prometheus.ceph.object_gateway.lua.script_executions"])
	require.Equal(t, 1, created["prometheus.ceph.object_gateway.requests_and_queue.failed_requests"])
	require.Equal(t, 1, created["prometheus.ceph.object_gateway.notifications.failure_outcomes"])
	require.Equal(t, 1, created["prometheus.ceph.object_gateway.notifications.pending_events"])
	require.Equal(t, 1, created["prometheus.ceph.object_gateway.notifications.event_state"])
	require.Equal(t, 1, created["prometheus.ceph.object_gateway.requests_and_queue.current_requests"])
	require.Equal(t, 1, created["prometheus.ceph.object_gateway_multisite.object_work"])
	require.Equal(t, 1, created["prometheus.ceph.object_gateway_multisite.replication_log_request_errors"])
}

func TestCollector_CephRGWAlertBoundaries(t *testing.T) {
	templates := cephHealthAlertTemplates(t)
	occurrence := map[string]struct {
		lookup    string
		condition string
		recipient string
	}{
		"rgw_notification_event_lost":            {"max -1m unaligned of event_lost", "crit: ($this == nan or $this == inf) ? (nan) : ($this > 0)", "sysadmin"},
		"rgw_notification_missing_configuration": {"max -1m unaligned of value", "warn: ($this == nan or $this == inf) ? (nan) : ($this > 0)", "sysadmin"},
		"rgw_lua_script_failed":                  {"max -1m unaligned of failure", "warn: ($this == nan or $this == inf) ? (nan) : ($this > 0)", "sysadmin"},
	}
	for name, expected := range occurrence {
		t.Run(name, func(t *testing.T) {
			block, ok := templates[name]
			require.Truef(t, ok, "template %s is absent", name)
			assert.Equal(t, expected.lookup, block["lookup"])
			assert.Equal(t, expected.condition, block["condition"])
			assert.Equal(t, expected.recipient, block["to"])
		})
	}

	sustained := map[string]struct {
		lookup    string
		condition string
	}{
		"rgw_failed_request_rate_fallback":        {"min -5m unaligned of value", "warn: ($this == nan or $this == inf) ? (nan) : ($this > 0)"},
		"rgw_notification_push_or_store_failures": {"min -5m unaligned of push_failed,store_fail", "warn: ($this == nan or $this == inf) ? (nan) : ($this > 0)"},
		"rgw_notification_inflight_pressure":      {"min -5m unaligned of value", "warn: ($this == nan or $this == inf) ? (nan) : ($this >= 1000)"},
		"rgw_notification_store_backlog":          {"min -5m unaligned of value", "warn: ($this == nan or $this == inf) ? (nan) : ($this >= 1000)"},
		"rgw_request_queue_pressure":              {"min -5m unaligned of qlen", "warn: ($this == nan or $this == inf) ? (nan) : ($this >= 1000)"},
		"rgw_multisite_fetch_errors":              {"min -5m unaligned of errors", "warn: ($this == nan or $this == inf) ? (nan) : ($this > 0)"},
		"rgw_multisite_poll_errors":               {"min -5m unaligned of value", "warn: ($this == nan or $this == inf) ? (nan) : ($this > 0)"},
	}
	for name, expected := range sustained {
		t.Run(name, func(t *testing.T) {
			block, ok := templates[name]
			require.Truef(t, ok, "template %s is absent", name)
			assert.Equal(t, expected.lookup, block["lookup"])
			assert.Equal(t, expected.condition, block["condition"])
			assert.Equal(t, "silent", block["to"])
		})
	}

	// max preserves any failure occurrence in the window, independent of event volume.
	anyOccurrence := func(samples []float64) float64 {
		if len(samples) == 0 {
			return math.NaN()
		}
		maximum := samples[0]
		for _, sample := range samples[1:] {
			if sample > maximum {
				maximum = sample
			}
		}
		return maximum
	}
	assert.InDelta(t, .1, anyOccurrence([]float64{0, .1, 0}), 1e-12)
	assert.InDelta(t, float64(0), anyOccurrence([]float64{0, 0, 0}), 1e-12)
	assert.True(t, math.IsNaN(anyOccurrence(nil)))
}
func TestCollector_CephMGRAlertContract(t *testing.T) {
	manifest := loadCephAlertManifest(t)
	templates := cephHealthAlertTemplates(t)
	s3checkTemplates := healthAlertTemplatesFromFile(t, filepath.Join("..", "..", "..", "..", "..",
		"health", "health.d", "s3check.conf"))
	templateFor := func(name string) (map[string]string, bool) {
		if block, ok := s3checkTemplates[name]; ok {
			return block, true
		}
		block, ok := templates[name]
		return block, ok
	}

	require.Equal(t, "v1", manifest.Version)
	require.Equal(t, "source-alert-map", manifest.Kind)
	require.Equal(t, "ceph", manifest.Profile)
	require.Equal(t, "10s", manifest.NetdataAlertCadence)
	require.Equal(t, "SHA-256 of the UTF-8 canonical JSON for the complete parsed source alert rule, with object keys sorted "+
		"lexicographically, no insignificant whitespace, and non-ASCII characters preserved", manifest.DefinitionSHA256Contract)
	require.Equal(t, map[string]cephAlertSource{
		"reef": {
			Tag: "v18.2.8", Commit: "efac5a54607c13fa50d4822e50242b86e6e446df",
			Path:   "monitoring/ceph-mixin/prometheus_alerts.yml",
			SHA256: "0325e5c481d00c674f7faf759e00f6c5c22028dbcc8bb95491404d600c6f3efd",
		},
		"squid": {
			Tag: "v19.2.5", Commit: "abc7aa7f2701e5d46878fd5e6bb7e2955f1a395a",
			Path:   "monitoring/ceph-mixin/prometheus_alerts.yml",
			SHA256: "259ee363694d174f46427a443ba0a8952b28df0c17af2bb65a2378511bc321ba",
		},
		"tentacle": {
			Tag: "v20.2.3", Commit: "06c2f9c35b67055a8a6fb99d1be236b3c4832ace",
			Path:   "monitoring/ceph-mixin/prometheus_alerts.yml",
			SHA256: "09308346d3d143ff128813f1142f1499142799174da4e0505ddad7144b8d8716",
		},
	}, manifest.Sources)
	require.ElementsMatch(t, []string{
		"CEPH-001", "CEPH-002", "CEPH-003", "CEPH-004", "CEPH-005", "CEPH-013", "CEPH-014", "CEPH-015",
		"CEPH-016", "CEPH-017", "CEPH-018", "CEPH-019", "CEPH-020", "CEPH-021", "CEPH-025", "CEPH-027",
		"CEPH-028", "CEPH-029", "CEPH-030", "CEPH-032", "CEPH-033", "CEPH-034", "CEPH-035", "CEPH-036",
		"CEPH-037", "CEPH-038", "CEPH-039", "CEPH-063", "CEPH-064", "CEPH-065", "CEPH-066", "CEPH-067",
		"CEPH-068", "CEPH-069", "CEPH-070", "CEPH-071", "CEPH-072", "CEPH-073", "CEPH-074", "CEPH-075",
		"CEPH-076", "CEPH-077", "CEPH-098", "CEPH-099", "CEPH-100", "CEPH-101", "CEPH-102", "CEPH-103",
		"CEPH-006", "CEPH-007", "CEPH-008", "CEPH-009", "CEPH-010", "CEPH-011", "CEPH-012", "CEPH-055",
		"CEPH-056", "CEPH-057", "CEPH-058", "CEPH-104", "CEPH-040", "CEPH-040-HELPER", "CEPH-041", "CEPH-043", "CEPH-044", "CEPH-045",
		"CEPH-046", "CEPH-046-HELPER", "CEPH-047", "CEPH-047-HELPER", "CEPH-048", "CEPH-049", "CEPH-050",
		"CEPH-051", "CEPH-052", "CEPH-054", "CEPH-059", "CEPH-060", "CEPH-061", "CEPH-062",
		"CEPH-078", "CEPH-080", "CEPH-080-HELPER-PERSIST", "CEPH-082", "CEPH-082-HELPER-READ-TIME",
		"CEPH-082-HELPER-READ-OPS", "CEPH-083", "CEPH-083-HELPER-WRITE-TIME", "CEPH-083-HELPER-WRITE-OPS",
		"CEPH-084", "CEPH-092", "CEPH-094", "CEPH-094-HELPER-GATEWAY",
	}, cephManifestSOWIDs(manifest.Alerts))
	require.ElementsMatch(t, []string{
		"CEPH-ND-001", "CEPH-ND-002", "RGW-M-01", "RGW-M-02", "RGW-M-08", "RGW-M-09", "RGW-M-10",
		"RGW-S-04", "RGW-S-12", "RGW-S-16", "RGW-S-17", "RGW-S-18", "RGW-S-19", "RGW-S-20", "RGW-S-20",
		"RGW-M-03", "RGW-M-04", "RGW-S-01", "RGW-S-02", "RGW-S-03", "RGW-S-09", "RGW-S-13", "RGW-S-14",
		"RGW-S-15", "RGW-M-05", "RGW-M-05", "RGW-S-11", "RGW-M-11", "RGW-S-24", "RGW-S-25",
	}, cephManifestExtensionSOWIDs(manifest.NetdataExtensions))
	for name, extension := range manifest.NetdataExtensions {
		require.NotEmptyf(t, extension.Reason, "%s has no extension rationale", name)
		if extension.Fidelity == "UNSUPPORTED" {
			require.Empty(t, extension.Context)
			require.NotContains(t, templates, name)
			continue
		}
		block, hasTemplate := templateFor(name)
		if hasTemplate && extension.Condition != "" {
			require.NotEmpty(t, extension.Context)
			assert.Equal(t, extension.Context, block["on"])
			assert.Equal(t, extension.Lookup, block["lookup"])
			assert.Equal(t, extension.Calc, block["calc"])
			assert.Equal(t, extension.Units, block["units"])
			assert.Equal(t, extension.Condition, block["condition"])
			assert.Equal(t, extension.Recipient, block["to"])
		} else {
			require.Empty(t, extension.Lookup)
			require.Empty(t, extension.Calc)
			require.Empty(t, extension.Condition)
			require.Empty(t, extension.Recipient)
		}
		require.NotEmptyf(t, extension.Fidelity, "%s has no extension fidelity", name)
		require.NotEmptyf(t, extension.Owner, "%s has no extension owner", name)
		switch extension.SOWID {
		case "RGW-M-08", "RGW-M-09", "RGW-M-10":
			require.Equal(t, "rgw_categorical_counter_occurrence", extension.Adaptation)
		case "RGW-S-04", "RGW-S-16":
			require.Equal(t, "rgw_counter_rate_window", extension.Adaptation)
		}
		if extension.Adaptation != "" {
			_, ok := manifest.AdaptationContracts[extension.Adaptation]
			require.Truef(t, ok, "%s references unknown adaptation %q", name, extension.Adaptation)
		}
	}

	inventoryAdaptation := manifest.AdaptationContracts["local_nvmeof_inventory_limit"]
	require.NotContains(t, strings.Join(inventoryAdaptation.Preserved, "\n"), "one-minute observation",
		"current-only namespace-capacity rule must not claim a persisted observation window")
	require.NotContains(t, strings.Join(inventoryAdaptation.Differences, "\n"), "persistence uses every available sample",
		"current-only namespace-capacity rule must disclose that source persistence is not represented")

	for name, mapping := range manifest.Alerts {
		require.Truef(t, mapping.SourceAlert != "" || len(mapping.SourceAlerts) > 0,
			"%s has no canonical or release source alert", name)
		hasExpression := mapping.Source.Expression != ""
		hasExpressions := len(mapping.Source.Expressions) > 0
		require.NotEqualf(t, hasExpression, hasExpressions,
			"%s must define exactly one of source expression or release expressions", name)
		isHelper := mapping.Netdata.Disposition == "internal-helper" || mapping.Netdata.Disposition == "subsumed-helper"
		if !isHelper {
			require.NotEmptyf(t, mapping.Source.Releases, "%s has no supported-release set", name)
		}
		for _, release := range []string{"reef", "squid", "tentacle"} {
			if isHelper {
				continue
			}
			supported := slices.Contains(mapping.Source.Releases, release)
			require.Equalf(t, supported, mapping.Source.Lines[release] > 0,
				"%s release availability and line pin disagree for %s", name, release)
			require.Equalf(t, supported, mapping.Source.DefinitionSHA256[release] != "",
				"%s release availability and definition pin disagree for %s", name, release)
			if hasExpressions {
				require.Containsf(t, mapping.Source.Expressions, release,
					"%s has no source expression for %s", name, release)
			}
		}
		require.NotEmpty(t, mapping.Source.Severity)
		adaptation, ok := manifest.AdaptationContracts[mapping.Netdata.Adaptation]
		require.Truef(t, ok, "%s references unknown adaptation %q", name, mapping.Netdata.Adaptation)
		require.NotEmpty(t, adaptation.Preserved)
		require.NotEmpty(t, adaptation.Differences)

		if mapping.Netdata.Disposition == "reject" {
			require.Equal(t, "UNSUPPORTED", mapping.Netdata.Fidelity)
			require.Empty(t, mapping.Netdata.Context)
			require.NotEmptyf(t, mapping.Netdata.Reason, "%s has no rejection rationale", name)
			require.NotContains(t, templates, name, "%s rejected rule must not ship an alert template")
			continue
		}
		if mapping.Netdata.Disposition == "internal-helper" {
			require.Equalf(t, "silent", mapping.Netdata.Recipient, "%s internal helper must be silent", name)
			require.Containsf(t, templates, name, "internal helper template is absent")
			assert.Equalf(t, mapping.Netdata.Context, templates[name]["on"], "%s helper context mismatch", name)
			assert.Equalf(t, mapping.Netdata.Calc, templates[name]["calc"], "%s helper calc mismatch", name)
			assert.Equalf(t, manifest.NetdataAlertCadence, templates[name]["every"], "%s helper cadence mismatch", name)
			assert.Equalf(t, mapping.Netdata.Condition, templates[name]["condition"], "%s helper condition mismatch", name)
			assert.Equalf(t, mapping.Netdata.Recipient, templates[name]["to"], "%s helper routing mismatch", name)
			continue
		}
		if mapping.Netdata.Disposition == "subsumed-helper" {
			require.Equalf(t, "silent", mapping.Netdata.Recipient, "%s subsumed helper must be silent", name)
			require.NotEmptyf(t, mapping.Netdata.Owner, "%s has no owning alert", name)
			_, ok := templates[mapping.Netdata.Owner]
			require.Truef(t, ok, "%s owner %q is absent", name, mapping.Netdata.Owner)
			continue
		}
		if mapping.Netdata.Disposition == "reuse" {
			require.NotEmptyf(t, mapping.Netdata.OwnerFile, "%s has no generic owner file", name)
			require.NotEmptyf(t, mapping.Netdata.OwnerAlerts, "%s has no generic owner templates", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			block, ok := templates[name]
			require.True(t, ok, "missing alert template")
			assert.Equal(t, mapping.Netdata.Context, block["on"])
			assert.Equal(t, mapping.Netdata.Labels, block["chart labels"])
			assert.Equal(t, mapping.Netdata.Lookup, block["lookup"])
			assert.Equal(t, mapping.Netdata.Calc, block["calc"])
			if mapping.Netdata.Calc != "" && mapping.Netdata.Disposition == "correct" {
				assert.Empty(t, mapping.Netdata.Lookup, "current compound alert must not mix historical and current dimensions")
				assert.NotContains(t, mapping.Netdata.Calc, "$last_collected_t",
					"data-state alert must not duplicate generic collection-failure ownership")
			}
			assert.Equal(t, mapping.Netdata.Units, block["units"])
			assert.Equal(t, manifest.NetdataAlertCadence, block["every"])
			assert.Empty(t, block["delay"], "source persistence adaptation must not use notification delay")
			assert.Equal(t, mapping.Netdata.Condition, block["condition"])
			assert.Equal(t, mapping.Netdata.Recipient, block["to"])
			assert.Contains(t, []string{"NETDATA-ADAPTED", "CORRECTED-INTENT"}, mapping.Netdata.Fidelity)
			assert.True(t, strings.HasPrefix(block["on"], "prometheus.ceph."))
			if mapping.Netdata.Calc == "" {
				assert.Contains(t, block["info"], "available")
			} else {
				assert.Contains(t, block["info"], "current")
			}
			assert.NotContains(t, block["info"], "continuously")
		})
	}

	for name, mapping := range manifest.Alerts {
		if mapping.Netdata.Disposition != "reuse" {
			continue
		}
		generic := healthAlertTemplatesFromFile(t, filepath.Join("../../../../../..", mapping.Netdata.OwnerFile))
		for _, owner := range mapping.Netdata.OwnerAlerts {
			block, ok := generic[owner]
			require.Truef(t, ok, "%s generic owner %q is absent from %s", name, owner, mapping.Netdata.OwnerFile)
			assert.Equal(t, mapping.Netdata.Context, block["on"])
			assert.NotContains(t, templates, owner, "Ceph alert pack duplicates a generic owner")
		}
		assert.Equal(t, "CORRECTED-INTENT", mapping.Netdata.Fidelity)
	}

	reuse := manifest.Alerts["plugin_data_collection_status"].Netdata
	generic := healthAlertTemplatesFromFile(t, filepath.Join("../../../../../..", reuse.OwnerFile))["plugin_data_collection_status"]
	assert.Equal(t, reuse.Lookup, generic["lookup"])
	assert.Equal(t, reuse.Units, generic["units"])
	assert.Equal(t, reuse.Condition, generic["condition"])
	assert.Equal(t, reuse.Recipient, generic["to"])

	// The Dashboard API collector owns this separate native collection-integrity alert.
	assert.Equal(t, manifest.NetdataExtensions["ceph_component_collection_failed"].Context,
		templates["ceph_component_collection_failed"]["on"])
	assert.NotContains(t, templates, "ceph_mgr_prometheus_module_inactive")
}

func TestCollector_CephRGWGenericOwnerExamples(t *testing.T) {
	for _, tc := range []struct {
		module  string
		matches func(*testing.T, yaml.Node) bool
	}{
		{
			module: "httpcheck",
			matches: func(t *testing.T, node yaml.Node) bool {
				var job struct {
					URL              string `yaml:"url"`
					AcceptedStatuses []int  `yaml:"status_accepted"`
				}
				require.NoError(t, node.Decode(&job))
				slices.Sort(job.AcceptedStatuses)
				return job.URL != "" && slices.Equal(job.AcceptedStatuses, []int{200, 204, 403, 405})
			},
		},
		{
			module: "x509check",
			matches: func(t *testing.T, node yaml.Node) bool {
				var job struct {
					Source          string `yaml:"source"`
					CheckRevocation bool   `yaml:"check_revocation_status"`
				}
				require.NoError(t, node.Decode(&job))
				return job.Source != "" && job.CheckRevocation
			},
		},
		{
			module: "weblog",
			matches: func(t *testing.T, node yaml.Node) bool {
				var job struct {
					JSONConfig struct {
						Mapping map[string]string `yaml:"mapping"`
					} `yaml:"json_config"`
					CustomNumericFields []struct {
						Name  string `yaml:"name"`
						Units string `yaml:"units"`
					} `yaml:"custom_numeric_fields"`
				}
				require.NoError(t, node.Decode(&job))
				return job.JSONConfig.Mapping["total_time"] == "total_time" &&
					len(job.CustomNumericFields) == 1 &&
					job.CustomNumericFields[0].Name == "total_time" &&
					job.CustomNumericFields[0].Units == "milliseconds"
			},
		},
	} {
		t.Run(tc.module, func(t *testing.T) {
			var metadata struct {
				Modules []struct {
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
			content, err := os.ReadFile(filepath.Join("..", tc.module, "metadata.yaml"))
			require.NoError(t, err)
			require.NoError(t, yaml.Unmarshal(content, &metadata))
			require.NotEmpty(t, metadata.Modules)

			for _, module := range metadata.Modules {
				for _, example := range module.Setup.Configuration.Examples.List {
					var config struct {
						Jobs []yaml.Node `yaml:"jobs"`
					}
					require.NoError(t, yaml.Unmarshal([]byte(example.Config), &config))
					for _, node := range config.Jobs {
						if tc.matches(t, node) {
							return
						}
					}
				}
			}
			t.Fatalf("%s metadata has no Ceph RGW configuration example", tc.module)
		})
	}

	manifest := loadCephAlertManifest(t)
	require.Equal(t, "x509check.revocation_status", manifest.NetdataExtensions["rgw_tls_certificate_revoked"].Context)
	require.Equal(t, "httpcheck.status", manifest.NetdataExtensions["rgw_basic_endpoint_unavailable"].Context)
	require.Equal(t,
		"web_log.custom_numeric_field_total_time_summary",
		manifest.NetdataExtensions["rgw_overall_request_latency"].Context)

	x509Templates := healthAlertTemplatesFromFile(t, filepath.Join("..", "..", "..", "..", "..", "health", "health.d", "x509check.conf"))
	require.Equal(t, "x509check.revocation_status", x509Templates["x509check_revocation_status"]["on"])
	httpcheckTemplates := healthAlertTemplatesFromFile(t, filepath.Join("..", "..", "..", "..", "..", "health", "health.d", "httpcheck.conf"))
	require.Equal(t, "httpcheck.status", httpcheckTemplates["httpcheck_web_service_up"]["on"])
	weblogTemplates := healthAlertTemplatesFromFile(t, filepath.Join("..", "..", "..", "..", "..", "health", "health.d", "web_log.conf"))
	require.Equal(t, "web_log.request_processing_time", weblogTemplates["web_log_web_slow"]["on"])
}

func TestCollector_CephS3CheckManifestMatchesCollectorArtifacts(t *testing.T) {
	manifest := loadCephAlertManifest(t)
	expected := map[string]cephNetdataExtension{
		"s3check_stage_failed": {
			SOWID: "RGW-M-05", Owner: "s3check", Context: "s3check.stage_status",
			Calc: "$failed", Units: "status", Recipient: "sysadmin",
		},
		"s3check_stage_latency": {
			SOWID: "RGW-S-11", Owner: "s3check", Context: "s3check.stage_latency_status",
			Calc: "$exceeded", Units: "status", Recipient: "silent",
		},
		"s3check_multisite_phase_failed": {
			SOWID: "RGW-M-05", Owner: "s3check", Context: "s3check.multisite_phase_failure",
			Calc: "$failed", Units: "status", Recipient: "sysadmin",
		},
		"s3check_multisite_payload_mismatch": {
			SOWID: "RGW-M-11", Owner: "s3check", Context: "s3check.multisite_payload_mismatch",
			Calc: "$mismatch", Units: "status", Recipient: "sysadmin",
		},
		"s3check_multisite_replication_rpo_breach": {
			SOWID: "RGW-S-24", Owner: "s3check", Context: "s3check.multisite_rpo_status",
			Calc: "$breached", Units: "status", Recipient: "silent",
		},
		"s3check_multisite_delete_propagation_breach": {
			SOWID: "RGW-S-25", Owner: "s3check", Context: "s3check.multisite_delete_status",
			Calc: "$breached", Units: "status", Recipient: "silent",
		},
	}
	for name, want := range expected {
		got := manifest.NetdataExtensions[name]
		require.Equalf(t, want.SOWID, got.SOWID, "%s SOW ID", name)
		require.Equalf(t, want.Owner, got.Owner, "%s owner", name)
		require.Equalf(t, want.Context, got.Context, "%s context", name)
		require.Equalf(t, want.Calc, got.Calc, "%s calculation", name)
		require.Equalf(t, want.Units, got.Units, "%s units", name)
		require.Equalf(t, want.Recipient, got.Recipient, "%s recipient", name)
		require.NotEmptyf(t, got.Adaptation, "%s adaptation", name)
	}

	var metadata struct {
		Modules []struct {
			Alerts []struct {
				Name   string `yaml:"name"`
				Metric string `yaml:"metric"`
				Info   string `yaml:"info"`
			} `yaml:"alerts"`
		} `yaml:"modules"`
	}
	content, err := os.ReadFile(filepath.Join("..", "s3check", "metadata.yaml"))
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(content, &metadata))
	require.Len(t, metadata.Modules, 1)

	actual := make(map[string]string)
	info := make(map[string]string)
	for _, alert := range metadata.Modules[0].Alerts {
		actual[alert.Name] = alert.Metric
		info[alert.Name] = alert.Info
	}
	require.Equal(t, map[string]string{
		"s3check_stage_failed":                        "s3check.stage_status",
		"s3check_stage_latency":                       "s3check.stage_latency_status",
		"s3check_multisite_phase_failed":              "s3check.multisite_phase_failure",
		"s3check_multisite_payload_mismatch":          "s3check.multisite_payload_mismatch",
		"s3check_multisite_replication_rpo_breach":    "s3check.multisite_rpo_status",
		"s3check_multisite_delete_propagation_breach": "s3check.multisite_delete_status",
	}, actual)

	templates := healthAlertTemplatesFromFile(t, filepath.Join("..", "..", "..", "..", "..",
		"health", "health.d", "s3check.conf"))
	for name := range expected {
		require.Equalf(t, expected[name].Context, templates[name]["on"], "%s context", name)
		require.Equalf(t, expected[name].Calc, templates[name]["calc"], "%s calculation", name)
		require.Equalf(t, info[name], templates[name]["info"], "%s metadata info", name)
	}
}

func TestCollector_CephDeliveredAlertMatrixComplete(t *testing.T) {
	manifest := loadCephAlertManifest(t)
	got := append(cephManifestSOWIDs(manifest.Alerts), cephManifestExtensionSOWIDs(manifest.NetdataExtensions)...)

	// The native physical-capacity policy is owned directly by the native collector health
	// configuration and metadata; it is not part of the Prometheus source-alert manifest.
	got = append(got, "CEPH-ND-003")

	want := []string{
		// M01
		"CEPH-001", "CEPH-002", "CEPH-013", "CEPH-014", "CEPH-015", "CEPH-016", "CEPH-059",
		"CEPH-060", "CEPH-061", "CEPH-062", "CEPH-ND-001", "CEPH-ND-002", "RGW-M-01", "RGW-M-02",
		// M02
		"CEPH-003", "CEPH-004", "CEPH-005", "CEPH-017", "CEPH-018", "CEPH-019", "CEPH-020",
		"CEPH-021", "CEPH-025", "CEPH-027", "CEPH-028", "CEPH-029", "CEPH-030", "CEPH-032",
		"CEPH-033", "CEPH-034", "CEPH-035", "CEPH-036", "CEPH-037", "CEPH-038", "CEPH-039",
		// M03
		"CEPH-063", "CEPH-064", "CEPH-065", "CEPH-066", "CEPH-067", "CEPH-068", "CEPH-069",
		"CEPH-070", "CEPH-071", "CEPH-072", "CEPH-073", "CEPH-074", "CEPH-075", "CEPH-076",
		"CEPH-077", "CEPH-098", "CEPH-099", "CEPH-100", "CEPH-101", "CEPH-102", "CEPH-103", "CEPH-104",
		// M04
		"CEPH-040", "CEPH-040-HELPER", "CEPH-041", "CEPH-043", "CEPH-044", "CEPH-045", "CEPH-046",
		"CEPH-046-HELPER", "CEPH-047", "CEPH-047-HELPER", "CEPH-048", "CEPH-049", "CEPH-050",
		"CEPH-051", "CEPH-052", "CEPH-054", "CEPH-ND-003",
		// M05
		"CEPH-006", "CEPH-007", "CEPH-008", "CEPH-009", "CEPH-010", "CEPH-011", "CEPH-012",
		"CEPH-055", "CEPH-056", "CEPH-057", "CEPH-058",
		// M06
		"CEPH-078", "CEPH-080", "CEPH-080-HELPER-PERSIST", "CEPH-082", "CEPH-082-HELPER-READ-TIME",
		"CEPH-082-HELPER-READ-OPS", "CEPH-083", "CEPH-083-HELPER-WRITE-TIME", "CEPH-083-HELPER-WRITE-OPS",
		"CEPH-084", "CEPH-092", "CEPH-094", "CEPH-094-HELPER-GATEWAY",
		// M07
		"RGW-M-03", "RGW-M-04", "RGW-M-08", "RGW-M-09", "RGW-M-10", "RGW-S-01", "RGW-S-02",
		"RGW-S-03", "RGW-S-04", "RGW-S-09", "RGW-S-12", "RGW-S-13", "RGW-S-14", "RGW-S-15",
		"RGW-S-16", "RGW-S-17", "RGW-S-18", "RGW-S-19", "RGW-S-20",
		// P2-M01/P2-M02
		"RGW-M-05", "RGW-S-11", "RGW-M-11", "RGW-S-24", "RGW-S-25",
	}

	require.ElementsMatch(t, want, slices.Compact(slices.Sorted(slices.Values(got))))
}

func TestCollector_CephMGRAlertSourcePins(t *testing.T) {
	manifest := loadCephAlertManifest(t)

	for release, source := range manifest.Sources {
		t.Run(release, func(t *testing.T) {
			path := filepath.Join("testdata", "ceph_source_alerts", release+".yaml")
			content, err := os.ReadFile(path)
			require.NoError(t, err)

			fileDigest := sha256.Sum256(content)
			require.Equal(t, source.SHA256, hex.EncodeToString(fileDigest[:]), "pinned source file digest")

			rules := parseCephSourceAlertRules(t, content)
			for name, mapping := range manifest.Alerts {
				if slices.Contains([]string{"internal-helper", "subsumed-helper"}, mapping.Netdata.Disposition) {
					continue
				}
				if !slices.Contains(mapping.Source.Releases, release) {
					require.NotContainsf(t, rules, mapping.SourceAlert,
						"%s is incorrectly present in %s", mapping.SourceAlert, release)
					continue
				}
				t.Run(name, func(t *testing.T) {
					sourceAlert := mapping.SourceAlert
					if len(mapping.SourceAlerts) > 0 {
						sourceAlert = mapping.SourceAlerts[release]
					}
					rule, ok := rules[sourceAlert]
					require.Truef(t, ok, "source alert %q is absent", sourceAlert)

					expression := mapping.Source.Expression
					if len(mapping.Source.Expressions) > 0 {
						expression = mapping.Source.Expressions[release]
					}
					assert.Equal(t, expression, rule.Expression)
					assert.Equal(t, mapping.Source.Persistence, rule.Persistence)
					assert.Equal(t, mapping.Source.Severity, rule.Severity)
					assert.Equal(t, mapping.Source.Lines[release], rule.Line)
					assert.Equal(t, mapping.Source.DefinitionSHA256[release], rule.DefinitionSHA256)
				})
			}
		})
	}
}

type cephSourceAlertRule struct {
	Expression       string
	Persistence      string
	Severity         string
	Line             int
	DefinitionSHA256 string
}

func parseCephSourceAlertRules(t *testing.T, content []byte) map[string]cephSourceAlertRule {
	t.Helper()

	var source struct {
		Groups []struct {
			Rules []yaml.Node `yaml:"rules"`
		} `yaml:"groups"`
	}
	require.NoError(t, yaml.Unmarshal(content, &source))

	rules := make(map[string]cephSourceAlertRule)
	for _, group := range source.Groups {
		for _, node := range group.Rules {
			var definition map[string]any
			require.NoError(t, node.Decode(&definition))

			name, ok := definition["alert"].(string)
			if !ok {
				continue
			}
			_, exists := rules[name]
			require.Falsef(t, exists, "duplicate source alert %q", name)

			labels, ok := definition["labels"].(map[string]any)
			require.Truef(t, ok, "source alert %q has no labels mapping", name)
			severity, ok := labels["severity"].(string)
			require.Truef(t, ok, "source alert %q has no severity label", name)

			rules[name] = cephSourceAlertRule{
				Expression:       cephSourceString(t, name, definition, "expr"),
				Persistence:      cephSourceOptionalString(t, name, definition, "for"),
				Severity:         severity,
				Line:             node.Line,
				DefinitionSHA256: cephCanonicalDefinitionSHA256(t, name, definition),
			}
		}
	}
	return rules
}

func cephSourceString(t *testing.T, alertName string, definition map[string]any, field string) string {
	t.Helper()

	value, ok := definition[field].(string)
	require.Truef(t, ok, "source alert %q has no string %q field", alertName, field)
	return value
}

func cephSourceOptionalString(t *testing.T, alertName string, definition map[string]any, field string) string {
	t.Helper()

	value, exists := definition[field]
	if !exists {
		return ""
	}
	text, ok := value.(string)
	require.Truef(t, ok, "source alert %q has non-string %q field", alertName, field)
	return text
}

func cephCanonicalDefinitionSHA256(t *testing.T, alertName string, definition map[string]any) string {
	t.Helper()

	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	require.NoErrorf(t, encoder.Encode(definition), "canonicalize source alert %q", alertName)

	digest := sha256.Sum256(bytes.TrimSuffix(canonical.Bytes(), []byte{'\n'}))
	return hex.EncodeToString(digest[:])
}

func TestCollector_CephMGRObservationWindowAdaptation(t *testing.T) {
	active, healthy, warning, critical := 1, 0, 1, 2
	const window, updateEvery = 30, 15

	tests := []struct {
		name             string
		now, first, last int
		obsolete         bool
		points           []cephObservedPoint
		matches          func(int) bool
		want             string
	}{
		{
			name: "new chart can raise with one update interval less history",
			now:  15, first: 0, last: 15,
			points:  []cephObservedPoint{{0, &active}, {15, &active}},
			matches: func(v int) bool { return v > 0 }, want: "active",
		},
		{
			name: "prior healthy value remains in the observation window",
			now:  30, first: 0, last: 30,
			points:  []cephObservedPoint{{0, &healthy}, {15, &active}, {30, &active}},
			matches: func(v int) bool { return v > 0 }, want: "clear",
		},
		{
			name: "active observed values fill the window after the healthy value expires",
			now:  45, first: 0, last: 45,
			points:  []cephObservedPoint{{0, &healthy}, {15, &active}, {30, &active}, {45, &active}},
			matches: func(v int) bool { return v > 0 }, want: "active",
		},
		{
			name: "partial gap does not reset observed active values",
			now:  45, first: 0, last: 45,
			points:  []cephObservedPoint{{0, &healthy}, {15, &active}, {45, &active}},
			matches: func(v int) bool { return v > 0 }, want: "active",
		},
		{
			name: "relative window remains anchored to evaluation time during a gap",
			now:  45, first: 0, last: 30,
			points:  []cephObservedPoint{{0, &healthy}, {15, &active}, {30, &active}},
			matches: func(v int) bool { return v > 0 }, want: "active",
		},
		{
			name: "runnable all-null lookup is undefined",
			now:  15, first: 0, last: 15,
			points:  []cephObservedPoint{{0, nil}, {15, nil}},
			matches: func(v int) bool { return v > 0 }, want: "undefined",
		},
		{
			name: "gap becomes undefined when stored values leave a still-runnable window",
			now:  60, first: 0, last: 15,
			points:  []cephObservedPoint{{0, &active}, {15, &active}},
			matches: func(v int) bool { return v > 0 }, want: "undefined",
		},
		{
			name: "stale lookup stops evaluating",
			now:  61, first: 0, last: 15,
			points:  []cephObservedPoint{{0, &active}, {15, &active}},
			matches: func(v int) bool { return v > 0 }, want: "not-run",
		},
		{
			name: "collection resumption evaluates the new stored window",
			now:  75, first: 0, last: 75,
			points:  []cephObservedPoint{{0, &active}, {15, &active}, {75, &active}},
			matches: func(v int) bool { return v > 0 }, want: "active",
		},
		{
			name: "ordinary zero recovery clears",
			now:  45, first: 0, last: 45,
			points:  []cephObservedPoint{{15, &active}, {30, &active}, {45, &healthy}},
			matches: func(v int) bool { return v > 0 }, want: "clear",
		},
		{
			name: "other enumerated state clears warning predicate",
			now:  45, first: 0, last: 45,
			points:  []cephObservedPoint{{15, &warning}, {30, &warning}, {45, &critical}},
			matches: func(v int) bool { return v == 1 }, want: "clear",
		},
		{
			name: "obsolete chart removes instance alert",
			now:  45, first: 0, last: 45, obsolete: true,
			points:  []cephObservedPoint{{15, &active}, {30, &active}, {45, &active}},
			matches: func(v int) bool { return v > 0 }, want: "removed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, cephObservationWindowState(
				tc.now, tc.first, tc.last, window, updateEvery, tc.obsolete, tc.points, tc.matches))
		})
	}
}

type cephObservedPoint struct {
	at    int
	value *int
}

// cephObservationWindowState mirrors the relevant gates in rrdcalc_isrunnable()
// and the numeric-point selection of a relative unaligned health lookup. The
// query window remains relative to the evaluation time, not the latest stored
// sample. Tests pass timestamps; there is deliberately no synthetic "window
// complete" or Prometheus-style pending state.
func cephObservationWindowState(
	now, first, last, window, updateEvery int,
	obsolete bool,
	points []cephObservedPoint,
	matches func(int) bool,
) string {
	if obsolete {
		return "removed"
	}
	if now-window+updateEvery < first || now-window-updateEvery > last {
		return "not-run"
	}

	start := now - window
	hasValue := false
	for _, point := range points {
		if point.at < start || point.at > now || point.value == nil {
			continue
		}
		hasValue = true
		if !matches(*point.value) {
			return "clear"
		}
	}
	if !hasValue {
		return "undefined"
	}
	return "active"
}

func TestCollector_CephUnknownHealthCheckFallbackContract(t *testing.T) {
	manifest := loadCephAlertManifest(t)
	extension := manifest.NetdataExtensions["ceph_unknown_health_check"]
	block := cephHealthAlertTemplates(t)["ceph_unknown_health_check"]
	require.Equal(t, "NETDATA-ADAPTED", extension.Fidelity)
	require.NotEmpty(t, extension.LabelsPolicy)
	assert.Equal(t, extension.Context, block["on"])
	assert.Equal(t, extension.Lookup, block["lookup"])
	assert.Equal(t, extension.Units, block["units"])
	assert.Equal(t, manifest.NetdataAlertCadence, block["every"])
	assert.Empty(t, block["delay"])
	assert.Equal(t, extension.Condition, block["condition"])
	assert.Equal(t, extension.Recipient, block["to"])
	labels, ok := strings.CutPrefix(block["chart labels"], "name=")
	require.True(t, ok, "fallback must filter the health-check name label")
	fields := strings.Fields(labels)
	require.NotEmpty(t, fields)
	require.Equal(t, "*", fields[len(fields)-1], "negative label matches require a final positive match")

	got := make([]string, 0, len(fields)-1)
	for _, field := range fields[:len(fields)-1] {
		require.Truef(t, strings.HasPrefix(field, "!"), "fallback matcher %q is not a name exclusion", field)
		got = append(got, strings.TrimPrefix(field, "!"))
	}
	assert.ElementsMatch(t, []string{
		"BLUESTORE_DISK_SIZE_MISMATCH", "BLUESTORE_SPURIOUS_READ_ERRORS", "CEPHADM_CERT_ERROR",
		"CEPHADM_CERT_WARNING", "CEPHADM_FAILED_DAEMON", "CEPHADM_PAUSED", "DEVICE_HEALTH",
		"DEVICE_HEALTH_IN_USE", "DEVICE_HEALTH_TOOMANY", "FS_DEGRADED", "FS_WITH_FAILED_MDS",
		"HARDWARE_FANS", "HARDWARE_MEMORY", "HARDWARE_NETWORK", "HARDWARE_POWER", "HARDWARE_PROCESSOR",
		"HARDWARE_STORAGE", "MDS_ALL_DOWN", "MDS_DAMAGE", "MDS_HEALTH_READ_ONLY", "MDS_INSUFFICIENT_STANDBY",
		"MDS_UP_LESS_THAN_MAX", "MON_CLOCK_SKEW", "MON_DISK_CRIT", "MON_DISK_LOW", "MON_DOWN",
		"OBJECT_UNFOUND", "OSD_BACKFILLFULL", "OSD_DOWN", "OSD_FULL", "OSD_HOST_DOWN", "OSD_NEARFULL",
		"OSD_SCRUB_ERRORS", "OSD_SLOW_PING_TIME_BACK", "OSD_SLOW_PING_TIME_FRONT", "OSD_TOO_MANY_REPAIRS",
		"PG_AVAILABILITY", "PG_BACKFILL_FULL", "PG_DAMAGED", "PG_NOT_DEEP_SCRUBBED", "PG_NOT_SCRUBBED",
		"PG_RECOVERY_FULL", "POOL_BACKFILLFULL", "POOL_FULL", "POOL_NEAR_FULL", "RECENT_CRASH",
		"RECENT_MGR_MODULE_CRASH", "SLOW_OPS", "TOO_MANY_PGS", "UPGRADE_EXCEPTION",
	}, got)
}

func TestCollector_CephNVMeoFAlertsAreOwnedByLocalExporter(t *testing.T) {
	manifest := loadCephAlertManifest(t)
	for _, mapping := range manifest.Alerts {
		if !strings.HasPrefix(mapping.Netdata.Context, "prometheus.ceph.nvme_of.") {
			continue
		}
		assert.Equalf(t, "nvmeof_exporter", mapping.Netdata.Owner,
			"%s must name its local gateway-exporter owner", mapping.SOWID)
	}
}

func TestCollector_CephMetadataAlertsMatchMGRAlertTemplates(t *testing.T) {
	manifest := loadCephAlertManifest(t)
	var metadata struct {
		Modules []struct {
			Meta struct {
				ID string `yaml:"id"`
			} `yaml:"meta"`
			Alerts []struct {
				Name   string `yaml:"name"`
				Metric string `yaml:"metric"`
				Info   string `yaml:"info"`
			} `yaml:"alerts"`
		} `yaml:"modules"`
	}
	content, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(content, &metadata))

	actual := make(map[string]string)
	info := make(map[string]string)
	for _, module := range metadata.Modules {
		if module.Meta.ID != "collector-go.d.plugin-prometheus-ceph" {
			continue
		}
		for _, alert := range module.Alerts {
			actual[alert.Name] = alert.Metric
			info[alert.Name] = alert.Info
		}
		break
	}
	expected := make(map[string]string)
	publicMapping := func(disposition string) bool {
		return disposition != "reuse" && disposition != "reject" &&
			disposition != "internal-helper" && disposition != "subsumed-helper"
	}
	for name, mapping := range manifest.Alerts {
		if publicMapping(mapping.Netdata.Disposition) {
			expected[name] = mapping.Netdata.Context
		}
	}
	for name, extension := range manifest.NetdataExtensions {
		if extension.Owner == "s3check" {
			continue
		}
		if extension.Context != "" && extension.Condition != "" {
			expected[name] = extension.Context
		}
	}
	assert.Equal(t, expected, actual)
	templates := cephHealthAlertTemplates(t)
	for name, metadataInfo := range info {
		assert.Equalf(t, templates[name]["info"], metadataInfo,
			"metadata alert %q does not exactly match its shipped health-template info", name)
	}
}

type cephAlertManifest struct {
	Version                  string                            `yaml:"version"`
	Kind                     string                            `yaml:"kind"`
	Profile                  string                            `yaml:"profile"`
	NetdataAlertCadence      string                            `yaml:"netdata_alert_cadence"`
	DefinitionSHA256Contract string                            `yaml:"definition_sha256_contract"`
	Sources                  map[string]cephAlertSource        `yaml:"sources"`
	AdaptationContracts      map[string]cephAdaptationContract `yaml:"adaptation_contracts"`
	Alerts                   map[string]cephAlertMapping       `yaml:"alerts"`
	NetdataExtensions        map[string]cephNetdataExtension   `yaml:"netdata_extensions"`
}

type cephAlertSource struct {
	Tag    string `yaml:"tag"`
	Commit string `yaml:"commit"`
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
}

type cephAdaptationContract struct {
	Preserved   []string `yaml:"preserved"`
	Differences []string `yaml:"differences"`
}

type cephNetdataExtension struct {
	SOWID        string `yaml:"sow_id"`
	Reason       string `yaml:"reason"`
	Fidelity     string `yaml:"fidelity"`
	Owner        string `yaml:"owner"`
	Context      string `yaml:"context"`
	LabelsPolicy string `yaml:"labels_policy"`
	Lookup       string `yaml:"lookup"`
	Calc         string `yaml:"calc"`
	Units        string `yaml:"units"`
	Condition    string `yaml:"condition"`
	Recipient    string `yaml:"recipient"`
	Adaptation   string `yaml:"adaptation"`
}

type cephAlertMapping struct {
	SOWID        string            `yaml:"sow_id"`
	SourceAlert  string            `yaml:"source_alert"`
	SourceAlerts map[string]string `yaml:"source_alerts"`
	Source       struct {
		Expression       string            `yaml:"expression"`
		Expressions      map[string]string `yaml:"expressions"`
		Releases         []string          `yaml:"releases"`
		Persistence      string            `yaml:"persistence"`
		Severity         string            `yaml:"severity"`
		Lines            map[string]int    `yaml:"lines"`
		DefinitionSHA256 map[string]string `yaml:"definition_sha256"`
	} `yaml:"source"`
	Netdata struct {
		Disposition string   `yaml:"disposition"`
		Fidelity    string   `yaml:"fidelity"`
		Owner       string   `yaml:"owner"`
		Context     string   `yaml:"context"`
		Labels      string   `yaml:"labels"`
		Lookup      string   `yaml:"lookup"`
		Calc        string   `yaml:"calc"`
		Units       string   `yaml:"units"`
		Condition   string   `yaml:"condition"`
		Recipient   string   `yaml:"recipient"`
		Reason      string   `yaml:"reason"`
		Adaptation  string   `yaml:"adaptation"`
		OwnerFile   string   `yaml:"owner_file"`
		OwnerAlerts []string `yaml:"owner_alerts"`
	} `yaml:"netdata"`
}

func loadCephAlertManifest(t *testing.T) cephAlertManifest {
	t.Helper()

	content, err := os.ReadFile("testdata/ceph_alerts.yaml")
	require.NoError(t, err)
	var manifest cephAlertManifest
	require.NoError(t, yaml.Unmarshal(content, &manifest))
	return manifest
}

func cephManifestSOWIDs(alerts map[string]cephAlertMapping) []string {
	ids := make([]string, 0, len(alerts))
	for _, alert := range alerts {
		ids = append(ids, alert.SOWID)
	}
	return ids
}

func cephManifestExtensionSOWIDs(extensions map[string]cephNetdataExtension) []string {
	ids := make([]string, 0, len(extensions))
	for _, extension := range extensions {
		ids = append(ids, extension.SOWID)
	}
	return ids
}

func cephHealthAlertTemplates(t *testing.T) map[string]map[string]string {
	t.Helper()
	return healthAlertTemplatesFromFile(t, "../../../../../health/health.d/ceph.conf")
}

type cephCollectorPlan struct {
	collector *Collector
	plan      chartengine.Plan
}

func collectCephPlan(t *testing.T, body []byte) cephCollectorPlan {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	collr.Profiles = ProfilesConfig{Mode: "auto"}
	collr.MaxTS = 20_000
	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

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
	return cephCollectorPlan{collector: collr, plan: plan}
}

func chartUpdateValues(t *testing.T, update chartengine.UpdateChartAction) map[string]float64 {
	t.Helper()
	values := make(map[string]float64)
	for _, value := range update.Values {
		if value.IsFloat {
			values[value.Name] = value.Float64
		} else {
			values[value.Name] = float64(value.Int64)
		}
	}
	return values
}

func healthAlertTemplatesFromFile(t *testing.T, path string) map[string]map[string]string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	templates := make(map[string]map[string]string)
	var current map[string]string
	for source := range strings.SplitSeq(string(content), "\n") {
		line := strings.TrimSpace(source)
		if name, ok := strings.CutPrefix(line, "template:"); ok {
			current = make(map[string]string)
			templates[strings.TrimSpace(name)] = current
			continue
		}
		if current == nil {
			continue
		}
		for _, field := range []string{"on", "chart labels", "lookup", "calc", "units", "every", "delay", "warn", "crit", "info", "to"} {
			if value, ok := strings.CutPrefix(line, field+":"); ok {
				key := field
				if field == "warn" || field == "crit" {
					key = "condition"
					value = field + ":" + value
				}
				current[key] = strings.TrimSpace(value)
			}
		}
	}
	return templates
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
	collr.ExpectedPrefix = "vllm:"
	expectedProfiles := []string{"vllm", "fastapi", "process_runtime", "python_gc"}
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
	collr.Profiles = ProfilesConfig{Mode: profilesModeAuto}
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
			collr.ExpectedPrefix = "ray_vllm_"
			return []string{"vllm"}
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
			collr.ExpectedPrefix = "litellm_"
			return []string{"litellm", "process_runtime", "python_gc"}
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
