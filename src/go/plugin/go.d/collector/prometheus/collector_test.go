// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
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
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
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

func TestCollector_Collect(t *testing.T) {
	type testCaseStep struct {
		desc          string
		input         string
		wantCollected map[string]int64
		wantCharts    int
	}
	tests := map[string]struct {
		prepare func() *Collector
		steps   []testCaseStep
	}{
		"Gauge": {
			prepare: New,
			steps: []testCaseStep{
				{
					desc: "Two first seen series, no meta series ignored",
					input: `
# HELP test_gauge_metric_1 Test Gauge Metric 1
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 11
test_gauge_metric_1{label1="value2"} 12
test_gauge_no_meta_metric_1{label1="value1"} 11
test_gauge_no_meta_metric_1{label1="value2"} 12
`,
					wantCollected: map[string]int64{
						"test_gauge_metric_1-label1=value1": 11000,
						"test_gauge_metric_1-label1=value2": 12000,
					},
					wantCharts: 2,
				},
				{
					desc: "One series removed",
					input: `
# HELP test_gauge_metric_1 Test Gauge Metric 1
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 11
`,
					wantCollected: map[string]int64{
						"test_gauge_metric_1-label1=value1": 11000,
					},
					wantCharts: 1,
				},
				{
					desc: "One series (re)added",
					input: `
# HELP test_gauge_metric_1 Test Gauge Metric 1
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 11
test_gauge_metric_1{label1="value2"} 12
`,
					wantCollected: map[string]int64{
						"test_gauge_metric_1-label1=value1": 11000,
						"test_gauge_metric_1-label1=value2": 12000,
					},
					wantCharts: 2,
				},
			},
		},
		"Counter": {
			prepare: New,
			steps: []testCaseStep{
				{
					desc: "Four first seen series, no meta series collected",
					input: `
# HELP test_counter_metric_1_total Test Counter Metric 1
# TYPE test_counter_metric_1_total counter
test_counter_metric_1_total{label1="value1"} 11
test_counter_metric_1_total{label1="value2"} 12
test_counter_no_meta_metric_1_total{label1="value1"} 11
test_counter_no_meta_metric_1_total{label1="value2"} 12
`,
					wantCollected: map[string]int64{
						"test_counter_metric_1_total-label1=value1":         11000,
						"test_counter_metric_1_total-label1=value2":         12000,
						"test_counter_no_meta_metric_1_total-label1=value1": 11000,
						"test_counter_no_meta_metric_1_total-label1=value2": 12000,
					},
					wantCharts: 4,
				},
				{
					desc: "Two series removed",
					input: `
# HELP test_counter_metric_1_total Test Counter Metric 1
# TYPE test_counter_metric_1_total counter
test_counter_metric_1_total{label1="value1"} 11
test_counter_no_meta_metric_1_total{label1="value1"} 11
`,
					wantCollected: map[string]int64{
						"test_counter_metric_1_total-label1=value1":         11000,
						"test_counter_no_meta_metric_1_total-label1=value1": 11000,
					},
					wantCharts: 2,
				},
				{
					desc: "Two series (re)added",
					input: `
# HELP test_counter_metric_1_total Test Counter Metric 1
# TYPE test_counter_metric_1_total counter
test_counter_metric_1_total{label1="value1"} 11
test_counter_metric_1_total{label1="value2"} 12
test_counter_no_meta_metric_1_total{label1="value1"} 11
test_counter_no_meta_metric_1_total{label1="value2"} 12
`,
					wantCollected: map[string]int64{
						"test_counter_metric_1_total-label1=value1":         11000,
						"test_counter_metric_1_total-label1=value2":         12000,
						"test_counter_no_meta_metric_1_total-label1=value1": 11000,
						"test_counter_no_meta_metric_1_total-label1=value2": 12000,
					},
					wantCharts: 4,
				},
			},
		},
		"Summary": {
			prepare: New,
			steps: []testCaseStep{
				{
					desc: "Two first seen series, no meta series collected",
					input: `
# HELP test_summary_1_duration_microseconds Test Summary Metric 1
# TYPE test_summary_1_duration_microseconds summary
test_summary_1_duration_microseconds{label1="value1",quantile="0.5"} 4931.921
test_summary_1_duration_microseconds{label1="value1",quantile="0.9"} 4932.921
test_summary_1_duration_microseconds{label1="value1",quantile="0.99"} 4933.921
test_summary_1_duration_microseconds_sum{label1="value1"} 283201.29
test_summary_1_duration_microseconds_count{label1="value1"} 31
test_summary_no_meta_1_duration_microseconds{label1="value1",quantile="0.5"} 4931.921
test_summary_no_meta_1_duration_microseconds{label1="value1",quantile="0.9"} 4932.921
test_summary_no_meta_1_duration_microseconds{label1="value1",quantile="0.99"} 4933.921
test_summary_no_meta_1_duration_microseconds_sum{label1="value1"} 283201.29
test_summary_no_meta_1_duration_microseconds_count{label1="value1"} 31
`,
					wantCollected: map[string]int64{
						"test_summary_1_duration_microseconds-label1=value1_count":                 31,
						"test_summary_1_duration_microseconds-label1=value1_quantile=0.5":          4931921000,
						"test_summary_1_duration_microseconds-label1=value1_quantile=0.9":          4932921000,
						"test_summary_1_duration_microseconds-label1=value1_quantile=0.99":         4933921000,
						"test_summary_1_duration_microseconds-label1=value1_sum":                   283201290,
						"test_summary_no_meta_1_duration_microseconds-label1=value1_count":         31,
						"test_summary_no_meta_1_duration_microseconds-label1=value1_quantile=0.5":  4931921000,
						"test_summary_no_meta_1_duration_microseconds-label1=value1_quantile=0.9":  4932921000,
						"test_summary_no_meta_1_duration_microseconds-label1=value1_quantile=0.99": 4933921000,
						"test_summary_no_meta_1_duration_microseconds-label1=value1_sum":           283201290,
					},
					wantCharts: 6,
				},
				{
					desc: "One series removed",
					input: `
# HELP test_summary_1_duration_microseconds Test Summary Metric 1
# TYPE test_summary_1_duration_microseconds summary
test_summary_1_duration_microseconds{label1="value1",quantile="0.5"} 4931.921
test_summary_1_duration_microseconds{label1="value1",quantile="0.9"} 4932.921
test_summary_1_duration_microseconds{label1="value1",quantile="0.99"} 4933.921
test_summary_1_duration_microseconds_sum{label1="value1"} 283201.29
test_summary_1_duration_microseconds_count{label1="value1"} 31
`,
					wantCollected: map[string]int64{
						"test_summary_1_duration_microseconds-label1=value1_count":         31,
						"test_summary_1_duration_microseconds-label1=value1_quantile=0.5":  4931921000,
						"test_summary_1_duration_microseconds-label1=value1_quantile=0.9":  4932921000,
						"test_summary_1_duration_microseconds-label1=value1_quantile=0.99": 4933921000,
						"test_summary_1_duration_microseconds-label1=value1_sum":           283201290,
					},
					wantCharts: 3,
				},
				{
					desc: "One series (re)added",
					input: `
# HELP test_summary_1_duration_microseconds Test Summary Metric 1
# TYPE test_summary_1_duration_microseconds summary
test_summary_1_duration_microseconds{label1="value1",quantile="0.5"} 4931.921
test_summary_1_duration_microseconds{label1="value1",quantile="0.9"} 4932.921
test_summary_1_duration_microseconds{label1="value1",quantile="0.99"} 4933.921
test_summary_1_duration_microseconds_sum{label1="value1"} 283201.29
test_summary_1_duration_microseconds_count{label1="value1"} 31
test_summary_no_meta_1_duration_microseconds{label1="value1",quantile="0.5"} 4931.921
test_summary_no_meta_1_duration_microseconds{label1="value1",quantile="0.9"} 4932.921
test_summary_no_meta_1_duration_microseconds{label1="value1",quantile="0.99"} 4933.921
test_summary_no_meta_1_duration_microseconds_sum{label1="value1"} 283201.29
test_summary_no_meta_1_duration_microseconds_count{label1="value1"} 31
`,
					wantCollected: map[string]int64{
						"test_summary_1_duration_microseconds-label1=value1_count":                 31,
						"test_summary_1_duration_microseconds-label1=value1_quantile=0.5":          4931921000,
						"test_summary_1_duration_microseconds-label1=value1_quantile=0.9":          4932921000,
						"test_summary_1_duration_microseconds-label1=value1_quantile=0.99":         4933921000,
						"test_summary_1_duration_microseconds-label1=value1_sum":                   283201290,
						"test_summary_no_meta_1_duration_microseconds-label1=value1_count":         31,
						"test_summary_no_meta_1_duration_microseconds-label1=value1_quantile=0.5":  4931921000,
						"test_summary_no_meta_1_duration_microseconds-label1=value1_quantile=0.9":  4932921000,
						"test_summary_no_meta_1_duration_microseconds-label1=value1_quantile=0.99": 4933921000,
						"test_summary_no_meta_1_duration_microseconds-label1=value1_sum":           283201290,
					},
					wantCharts: 6,
				},
			},
		},
		"Summary with NaN": {
			prepare: New,
			steps: []testCaseStep{
				{
					desc: "Two first seen series, no meta series collected",
					input: `
# HELP test_summary_1_duration_microseconds Test Summary Metric 1
# TYPE test_summary_1_duration_microseconds summary
test_summary_1_duration_microseconds{label1="value1",quantile="0.5"} NaN
test_summary_1_duration_microseconds{label1="value1",quantile="0.9"} NaN
test_summary_1_duration_microseconds{label1="value1",quantile="0.99"} NaN
test_summary_1_duration_microseconds_sum{label1="value1"} 283201.29
test_summary_1_duration_microseconds_count{label1="value1"} 31
test_summary_no_meta_1_duration_microseconds{label1="value1",quantile="0.5"} NaN
test_summary_no_meta_1_duration_microseconds{label1="value1",quantile="0.9"} NaN
test_summary_no_meta_1_duration_microseconds{label1="value1",quantile="0.99"} NaN
test_summary_no_meta_1_duration_microseconds_sum{label1="value1"} 283201.29
test_summary_no_meta_1_duration_microseconds_count{label1="value1"} 31
`,
					wantCollected: map[string]int64{
						"test_summary_1_duration_microseconds-label1=value1_count":         31,
						"test_summary_1_duration_microseconds-label1=value1_sum":           283201290,
						"test_summary_no_meta_1_duration_microseconds-label1=value1_count": 31,
						"test_summary_no_meta_1_duration_microseconds-label1=value1_sum":   283201290,
					},
					wantCharts: 6,
				},
			},
		},
		"Histogram": {
			prepare: New,
			steps: []testCaseStep{
				{
					desc: "Two first seen series, no meta series collected",
					input: `
# HELP test_histogram_1_duration_seconds Test Histogram Metric 1
# TYPE test_histogram_1_duration_seconds histogram
test_histogram_1_duration_seconds_bucket{label1="value1",le="0.1"} 4
test_histogram_1_duration_seconds_bucket{label1="value1",le="0.5"} 5
test_histogram_1_duration_seconds_bucket{label1="value1",le="+Inf"} 6
test_histogram_1_duration_seconds_sum{label1="value1"} 0.00147889
test_histogram_1_duration_seconds_count{label1="value1"} 6
test_histogram_no_meta_1_duration_seconds_bucket{label1="value1",le="0.1"} 4
test_histogram_no_meta_1_duration_seconds_bucket{label1="value1",le="0.5"} 5
test_histogram_no_meta_1_duration_seconds_bucket{label1="value1",le="+Inf"} 6
test_histogram_no_meta_1_duration_seconds_sum{label1="value1"} 0.00147889
test_histogram_no_meta_1_duration_seconds_count{label1="value1"} 6
`,
					wantCollected: map[string]int64{
						"test_histogram_1_duration_seconds-label1=value1_bucket=+Inf":         6,
						"test_histogram_1_duration_seconds-label1=value1_bucket=0.1":          4,
						"test_histogram_1_duration_seconds-label1=value1_bucket=0.5":          5,
						"test_histogram_1_duration_seconds-label1=value1_count":               6,
						"test_histogram_1_duration_seconds-label1=value1_sum":                 1,
						"test_histogram_no_meta_1_duration_seconds-label1=value1_bucket=+Inf": 6,
						"test_histogram_no_meta_1_duration_seconds-label1=value1_bucket=0.1":  4,
						"test_histogram_no_meta_1_duration_seconds-label1=value1_bucket=0.5":  5,
						"test_histogram_no_meta_1_duration_seconds-label1=value1_count":       6,
						"test_histogram_no_meta_1_duration_seconds-label1=value1_sum":         1,
					},
					wantCharts: 6,
				},
				{
					desc: "One series removed",
					input: `
# HELP test_histogram_1_duration_seconds Test Histogram Metric 1
# TYPE test_histogram_1_duration_seconds histogram
test_histogram_1_duration_seconds_bucket{label1="value1",le="0.1"} 4
test_histogram_1_duration_seconds_bucket{label1="value1",le="0.5"} 5
test_histogram_1_duration_seconds_bucket{label1="value1",le="+Inf"} 6
`,
					wantCollected: map[string]int64{
						"test_histogram_1_duration_seconds-label1=value1_bucket=+Inf": 6,
						"test_histogram_1_duration_seconds-label1=value1_bucket=0.1":  4,
						"test_histogram_1_duration_seconds-label1=value1_bucket=0.5":  5,
						"test_histogram_1_duration_seconds-label1=value1_count":       0,
						"test_histogram_1_duration_seconds-label1=value1_sum":         0,
					},
					wantCharts: 3,
				},
				{
					desc: "One series (re)added",
					input: `
# HELP test_histogram_1_duration_seconds Test Histogram Metric 1
# TYPE test_histogram_1_duration_seconds histogram
test_histogram_1_duration_seconds_bucket{label1="value1",le="0.1"} 4
test_histogram_1_duration_seconds_bucket{label1="value1",le="0.5"} 5
test_histogram_1_duration_seconds_bucket{label1="value1",le="+Inf"} 6
test_histogram_1_duration_seconds_sum{label1="value1"} 0.00147889
test_histogram_1_duration_seconds_count{label1="value1"} 6
test_histogram_no_meta_1_duration_seconds_bucket{label1="value1",le="0.1"} 4
test_histogram_no_meta_1_duration_seconds_bucket{label1="value1",le="0.5"} 5
test_histogram_no_meta_1_duration_seconds_bucket{label1="value1",le="+Inf"} 6
test_histogram_no_meta_1_duration_seconds_sum{label1="value1"} 0.00147889
test_histogram_no_meta_1_duration_seconds_count{label1="value1"} 6
`,
					wantCollected: map[string]int64{
						"test_histogram_1_duration_seconds-label1=value1_bucket=+Inf":         6,
						"test_histogram_1_duration_seconds-label1=value1_bucket=0.1":          4,
						"test_histogram_1_duration_seconds-label1=value1_bucket=0.5":          5,
						"test_histogram_1_duration_seconds-label1=value1_count":               6,
						"test_histogram_1_duration_seconds-label1=value1_sum":                 1,
						"test_histogram_no_meta_1_duration_seconds-label1=value1_bucket=+Inf": 6,
						"test_histogram_no_meta_1_duration_seconds-label1=value1_bucket=0.1":  4,
						"test_histogram_no_meta_1_duration_seconds-label1=value1_bucket=0.5":  5,
						"test_histogram_no_meta_1_duration_seconds-label1=value1_count":       6,
						"test_histogram_no_meta_1_duration_seconds-label1=value1_sum":         1,
					},
					wantCharts: 6,
				},
			},
		},
		"match Untyped as Gauge": {
			prepare: func() *Collector {
				collr := New()
				collr.FallbackType.Gauge = []string{"test_gauge_no_meta*"}
				return collr
			},
			steps: []testCaseStep{
				{
					desc: "Two first seen series, meta series processed as Gauge",
					input: `
# HELP test_gauge_metric_1 Test Untyped Metric 1
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 11
test_gauge_metric_1{label1="value2"} 12
test_gauge_no_meta_metric_1{label1="value1"} 11
test_gauge_no_meta_metric_1{label1="value2"} 12
`,
					wantCollected: map[string]int64{
						"test_gauge_metric_1-label1=value1":         11000,
						"test_gauge_metric_1-label1=value2":         12000,
						"test_gauge_no_meta_metric_1-label1=value1": 11000,
						"test_gauge_no_meta_metric_1-label1=value2": 12000,
					},
					wantCharts: 4,
				},
			},
		},
		"match Untyped as Counter": {
			prepare: func() *Collector {
				collr := New()
				collr.FallbackType.Counter = []string{"test_gauge_no_meta*"}
				return collr
			},
			steps: []testCaseStep{
				{
					desc: "Two first seen series, meta series processed as Counter",
					input: `
# HELP test_gauge_metric_1 Test Untyped Metric 1
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 11
test_gauge_metric_1{label1="value2"} 12
test_gauge_no_meta_metric_1{label1="value1"} 11
test_gauge_no_meta_metric_1{label1="value2"} 12
`,
					wantCollected: map[string]int64{
						"test_gauge_metric_1-label1=value1":         11000,
						"test_gauge_metric_1-label1=value2":         12000,
						"test_gauge_no_meta_metric_1-label1=value1": 11000,
						"test_gauge_no_meta_metric_1-label1=value2": 12000,
					},
					wantCharts: 4,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare()

			var metrics []byte
			srv := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write(metrics)
				}))
			defer srv.Close()

			collr.URL = srv.URL
			require.NoError(t, collr.Init(context.Background()))

			for num, step := range test.steps {
				t.Run(fmt.Sprintf("step num %d ('%s')", num+1, step.desc), func(t *testing.T) {

					metrics = []byte(step.input)

					var mx map[string]int64

					for i := 0; i < maxNotSeenTimes+1; i++ {
						mx = collr.Collect(context.Background())
					}

					assert.Equal(t, step.wantCollected, mx)
					removeObsoleteCharts(collr.Charts())
					assert.Len(t, *collr.Charts(), step.wantCharts)
				})
			}
		})
	}
}

func removeObsoleteCharts(charts *module.Charts) {
	var i int
	for _, chart := range *charts {
		if !chart.Obsolete {
			(*charts)[i] = chart
			i++
		}
	}
	*charts = (*charts)[:i]
}
