// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
	promlabels "github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promselector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
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
				Selector:   promselector.Expr{Allow: []string{`name{label=#"value"}`}},
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
	tests := map[string]struct {
		configure func(*Collector)
		input     string
		want      map[string]metrix.SampleValue
	}{
		"gauge and typed counter": {
			input: `
# HELP test_gauge_metric_1 Test Gauge Metric 1
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 11
test_gauge_metric_1{label1="value2"} 12
# HELP test_counter_metric_1_total Test Counter Metric 1
# TYPE test_counter_metric_1_total counter
test_counter_metric_1_total{label1="value1"} 21
test_counter_metric_1_total{label1="value2"} 22
test_untyped_metric{label1="value1"} 33
`,
			want: map[string]metrix.SampleValue{
				`test_counter_metric_1_total{label1="value1"}`: 21,
				`test_counter_metric_1_total{label1="value2"}`: 22,
				`test_gauge_metric_1{label1="value1"}`:         11,
				`test_gauge_metric_1{label1="value2"}`:         12,
			},
		},
		"fallback typed untyped as gauge": {
			configure: func(c *Collector) {
				c.FallbackType.Gauge = []string{"test_untyped*"}
			},
			input: `
test_untyped_metric{label1="value1"} 11.5
test_untyped_metric{label1="value2"} 12.5
`,
			want: map[string]metrix.SampleValue{
				`test_untyped_metric{label1="value1"}`: 11.5,
				`test_untyped_metric{label1="value2"}`: 12.5,
			},
		},
		"selector keeps only matching metric": {
			configure: func(c *Collector) {
				c.Selector = promselector.Expr{Allow: []string{`allowed_metric{job="demo"}`}}
			},
			input: `
# TYPE allowed_metric gauge
allowed_metric{job="demo"} 7
# TYPE blocked_metric gauge
blocked_metric{job="demo"} 9
`,
			want: map[string]metrix.SampleValue{
				`allowed_metric{job="demo"}`: 7,
			},
		},
		"mixed label family drops only offending series": {
			input: `
# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 10
http_requests_total{method="POST"} 5
`,
			want: map[string]metrix.SampleValue{
				`http_requests_total{code="200",method="GET"}`: 10,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			if test.configure != nil {
				test.configure(collr)
			}

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(test.input))
			}))
			defer srv.Close()

			collr.URL = srv.URL
			require.NoError(t, collr.Init(context.Background()))

			got, err := collecttest.CollectScalarSeries(collr, metrix.ReadRaw())
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestCollector_CollectSummaryAndHistogram(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# TYPE test_summary_duration_seconds summary
test_summary_duration_seconds{label1="value1",quantile="0.5"} 4.5
test_summary_duration_seconds{label1="value1",quantile="0.9"} 9.5
test_summary_duration_seconds_sum{label1="value1"} 31.25
test_summary_duration_seconds_count{label1="value1"} 7
# TYPE test_histogram_duration_seconds histogram
test_histogram_duration_seconds_bucket{label1="value1",le="0.1"} 1
test_histogram_duration_seconds_bucket{label1="value1",le="0.5"} 3
test_histogram_duration_seconds_bucket{label1="value1",le="+Inf"} 5
test_histogram_duration_seconds_sum{label1="value1"} 1.25
test_histogram_duration_seconds_count{label1="value1"} 5
# TYPE test_inf_only_histogram_duration_seconds histogram
test_inf_only_histogram_duration_seconds_bucket{label1="value1",le="+Inf"} 2
test_inf_only_histogram_duration_seconds_sum{label1="value1"} 0.25
test_inf_only_histogram_duration_seconds_count{label1="value1"} 2
`))
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	reader := collectRawReader(t, collr)

	summary, ok := reader.Summary("test_summary_duration_seconds", metrix.Labels{"label1": "value1"})
	require.True(t, ok)
	assert.Equal(t, metrix.SampleValue(7), summary.Count)
	assert.Equal(t, metrix.SampleValue(31.25), summary.Sum)
	require.Len(t, summary.Quantiles, 2)
	assert.Equal(t, 0.5, summary.Quantiles[0].Quantile)
	assert.Equal(t, metrix.SampleValue(4.5), summary.Quantiles[0].Value)
	assert.Equal(t, 0.9, summary.Quantiles[1].Quantile)
	assert.Equal(t, metrix.SampleValue(9.5), summary.Quantiles[1].Value)

	histogram, ok := reader.Histogram("test_histogram_duration_seconds", metrix.Labels{"label1": "value1"})
	require.True(t, ok)
	assert.Equal(t, metrix.SampleValue(5), histogram.Count)
	assert.Equal(t, metrix.SampleValue(1.25), histogram.Sum)
	require.Len(t, histogram.Buckets, 2)
	assert.Equal(t, 0.1, histogram.Buckets[0].UpperBound)
	assert.Equal(t, metrix.SampleValue(1), histogram.Buckets[0].CumulativeCount)
	assert.Equal(t, 0.5, histogram.Buckets[1].UpperBound)
	assert.Equal(t, metrix.SampleValue(3), histogram.Buckets[1].CumulativeCount)

	infOnly, ok := reader.Histogram("test_inf_only_histogram_duration_seconds", metrix.Labels{"label1": "value1"})
	require.True(t, ok)
	assert.Equal(t, metrix.SampleValue(2), infOnly.Count)
	assert.Equal(t, metrix.SampleValue(0.25), infOnly.Sum)
	assert.Empty(t, infOnly.Buckets)
}

func TestCollector_ChartTemplateYAML(t *testing.T) {
	collr := New()
	templateYAML := collr.ChartTemplateYAML()
	collecttest.AssertChartTemplateSchema(t, templateYAML)
	assert.Contains(t, templateYAML, "autogen")
	assert.Contains(t, templateYAML, "family: prometheus")
}

func TestCollector_CollectAutogenPlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`
# HELP test_gauge_metric_1 Requests in flight
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 11.5
`))
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	collectRawReader(t, collr)

	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(collr.ChartTemplateYAML()), 1))

	attempt, err := engine.PreparePlan(collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	defer attempt.Abort()

	plan := attempt.Plan()
	require.NotEmpty(t, plan.Actions)

	var foundCreateChart bool
	for _, action := range plan.Actions {
		create, ok := action.(chartengine.CreateChartAction)
		if !ok {
			continue
		}
		if strings.Contains(create.ChartID, "test_gauge_metric_1") {
			foundCreateChart = true
			assert.Equal(t, "Requests in flight", create.Meta.Title)
			break
		}
	}
	assert.True(t, foundCreateChart)
}

func TestCollector_ScrapeMetricFamiliesNoStaleAfterEmptyScrape(t *testing.T) {
	var call atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch call.Add(1) {
		case 1:
			_, _ = w.Write([]byte(`
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 11
`))
		default:
			_, _ = w.Write([]byte(""))
		}
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	mfs, err := collr.scrapeMetricFamilies()
	require.NoError(t, err)
	assert.Equal(t, map[string]metrix.SampleValue{
		`test_gauge_metric_1{label1="value1"}`: 11,
	}, scalarMetricFamilies(mfs))

	mfs, err = collr.scrapeMetricFamilies()
	require.NoError(t, err)
	assert.Empty(t, scalarMetricFamilies(mfs))
}

func TestCollector_FailedScrapeDoesNotContaminateLaterCollect(t *testing.T) {
	var call atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch call.Add(1) {
		case 1:
			_, _ = w.Write([]byte(`
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 11
`))
		case 2:
			_, _ = w.Write([]byte(`
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 12
hello and goodbye
`))
		default:
			_, _ = w.Write([]byte(`
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 13
`))
		}
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	mfs, err := collr.scrapeMetricFamilies()
	require.NoError(t, err)
	assert.Equal(t, map[string]metrix.SampleValue{
		`test_gauge_metric_1{label1="value1"}`: 11,
	}, scalarMetricFamilies(mfs))

	_, err = collr.scrapeMetricFamilies()
	require.Error(t, err)

	mfs, err = collr.scrapeMetricFamilies()
	require.NoError(t, err)
	assert.Equal(t, map[string]metrix.SampleValue{
		`test_gauge_metric_1{label1="value1"}`: 13,
	}, scalarMetricFamilies(mfs))
}

func TestCollector_MixedLabelFamilyKeepsMatchingSeriesAfterHandleExists(t *testing.T) {
	var call atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch call.Add(1) {
		case 1:
			_, _ = w.Write([]byte(`
# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 10
`))
		default:
			_, _ = w.Write([]byte(`
# TYPE http_requests_total counter
http_requests_total{method="POST"} 5
http_requests_total{method="GET",code="200"} 11
`))
		}
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	got, err := collecttest.CollectScalarSeries(collr, metrix.ReadRaw())
	require.NoError(t, err)
	assert.Equal(t, map[string]metrix.SampleValue{
		`http_requests_total{code="200",method="GET"}`: 10,
	}, got)

	got, err = collecttest.CollectScalarSeries(collr, metrix.ReadRaw())
	require.NoError(t, err)
	assert.Equal(t, map[string]metrix.SampleValue{
		`http_requests_total{code="200",method="GET"}`: 11,
	}, got)
}

func TestCollector_CheckThenScrapeMetricFamiliesDoesNotLeakState(t *testing.T) {
	var call atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch call.Add(1) {
		case 1:
			_, _ = w.Write([]byte(`
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 11
`))
		default:
			_, _ = w.Write([]byte(""))
		}
	}))
	defer srv.Close()

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	require.NoError(t, collr.Check(context.Background()))

	mfs, err := collr.scrapeMetricFamilies()
	require.NoError(t, err)
	assert.Empty(t, scalarMetricFamilies(mfs))
}

func collectRawReader(t *testing.T, collr *Collector) metrix.Reader {
	t.Helper()

	managed, ok := metrix.AsCycleManagedStore(collr.MetricStore())
	require.True(t, ok)

	cc := managed.CycleController()
	committed := false
	cc.BeginCycle()
	defer func() {
		if !committed {
			cc.AbortCycle()
		}
	}()

	require.NoError(t, collr.Collect(context.Background()))
	cc.CommitCycleSuccess()
	committed = true

	return collr.MetricStore().Read(metrix.ReadRaw())
}

func scalarMetricFamilies(mfs prompkg.MetricFamilies) map[string]metrix.SampleValue {
	out := make(map[string]metrix.SampleValue)
	for _, mf := range mfs {
		for _, metric := range mf.Metrics() {
			key := scalarMetricFamilyKey(mf.Name(), metric.Labels())
			switch {
			case metric.Gauge() != nil:
				out[key] = metrix.SampleValue(metric.Gauge().Value())
			case metric.Counter() != nil:
				out[key] = metrix.SampleValue(metric.Counter().Value())
			case metric.Untyped() != nil:
				out[key] = metrix.SampleValue(metric.Untyped().Value())
			}
		}
	}
	return out
}

func scalarMetricFamilyKey(name string, labels promlabels.Labels) string {
	if len(labels) == 0 {
		return name
	}

	items := make([]promlabels.Label, len(labels))
	copy(items, labels)
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})

	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('{')
	for i, label := range items {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(label.Name)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(label.Value))
	}
	b.WriteByte('}')
	return b.String()
}
