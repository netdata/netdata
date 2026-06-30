package prometheus

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
)

var (
	dataMultilineHelp, _ = os.ReadFile("testdata/multiline-help.txt")

	dataGaugeMeta, _       = os.ReadFile("testdata/gauge-meta.txt")
	dataGaugeNoMeta, _     = os.ReadFile("testdata/gauge-no-meta.txt")
	dataCounterMeta, _     = os.ReadFile("testdata/counter-meta.txt")
	dataCounterNoMeta, _   = os.ReadFile("testdata/counter-no-meta.txt")
	dataSummaryMeta, _     = os.ReadFile("testdata/summary-meta.txt")
	dataSummaryNoMeta, _   = os.ReadFile("testdata/summary-no-meta.txt")
	dataHistogramMeta, _   = os.ReadFile("testdata/histogram-meta.txt")
	dataHistogramNoMeta, _ = os.ReadFile("testdata/histogram-no-meta.txt")
	dataAllTypes           = joinData(
		dataGaugeMeta, dataGaugeNoMeta, dataCounterMeta, dataCounterNoMeta,
		dataSummaryMeta, dataSummaryNoMeta, dataHistogramMeta, dataHistogramNoMeta,
	)
)

func Test_testParseDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataMultilineHelp":   dataMultilineHelp,
		"dataGaugeMeta":       dataGaugeMeta,
		"dataGaugeNoMeta":     dataGaugeNoMeta,
		"dataCounterMeta":     dataCounterMeta,
		"dataCounterNoMeta":   dataCounterNoMeta,
		"dataSummaryMeta":     dataSummaryMeta,
		"dataSummaryNoMeta":   dataSummaryNoMeta,
		"dataHistogramMeta":   dataHistogramMeta,
		"dataHistogramNoMeta": dataHistogramNoMeta,
		"dataAllTypes":        dataAllTypes,
	} {
		require.NotNilf(t, data, name)
	}
}

func TestPromTextParser_parseToMetricFamilies(t *testing.T) {
	tests := map[string]struct {
		input []byte
		want  MetricFamilies
	}{
		"summary _sum/_count before # TYPE fold into one summary": {
			input: []byte(`my_summary_sum{label1="value1"} 10
my_summary_count{label1="value1"} 2
# TYPE my_summary summary
my_summary{label1="value1",quantile="0.5"} 0.5
`),
			want: MetricFamilies{
				"my_summary": {
					name: "my_summary",
					typ:  model.MetricTypeSummary,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							summary: &Summary{
								sum:       10,
								count:     2,
								quantiles: []Quantile{{quantile: 0.5, value: 0.5}},
							},
						},
					},
				},
			},
		},
		"histogram _sum/_count before _bucket (no # TYPE) fold into one histogram": {
			input: []byte(`my_hist_sum{label1="value1"} 5
my_hist_count{label1="value1"} 3
my_hist_bucket{label1="value1",le="0.1"} 1
my_hist_bucket{label1="value1",le="+Inf"} 3
`),
			want: MetricFamilies{
				"my_hist": {
					name: "my_hist",
					typ:  model.MetricTypeHistogram,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							histogram: &Histogram{
								sum:   5,
								count: 3,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 1},
									{upperBound: math.Inf(1), cumulativeCount: 3},
								},
							},
						},
					},
				},
			},
		},
		"metric name found by lookup, not first label": {
			input: []byte(`
# HELP DCGM_FI_DEV_GPU_UTIL GPU utilization
# TYPE DCGM_FI_DEV_GPU_UTIL gauge
DCGM_FI_DEV_GPU_UTIL{UUID="GPU-aaa",gpu="0"} 80
`),
			want: MetricFamilies{
				"DCGM_FI_DEV_GPU_UTIL": {
					name: "DCGM_FI_DEV_GPU_UTIL",
					help: "GPU utilization",
					typ:  model.MetricTypeGauge,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "UUID", Value: "GPU-aaa"}, {Name: "gpu", Value: "0"}},
							gauge:  &Gauge{value: 80},
						},
					},
				},
			},
		},
		"valid _bucket (has le) folds into the histogram family": {
			input: []byte("# TYPE h histogram\nh_bucket{le=\"1\",label=\"x\"} 1\n"),
			want: MetricFamilies{
				"h": {
					name: "h",
					typ:  model.MetricTypeHistogram,
					metrics: []Metric{
						{
							labels:    labels.Labels{{Name: "label", Value: "x"}},
							histogram: &Histogram{buckets: []Bucket{{upperBound: 1, cumulativeCount: 1}}},
						},
					},
				},
			},
		},
		"_bucket without le is a plain metric, not a histogram bucket": {
			input: []byte("# TYPE h histogram\nh_bucket{label=\"x\"} 1\n"),
			want: MetricFamilies{
				"h_bucket": {
					name: "h_bucket",
					typ:  model.MetricTypeUnknown,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label", Value: "x"}},
							untyped: &Untyped{value: 1},
						},
					},
				},
			},
		},
		"Gauge with multiline HELP": {
			input: dataMultilineHelp,
			want: MetricFamilies{
				"test_gauge_metric_1": {
					name: "test_gauge_metric_1",
					help: "First line. Second line.",
					typ:  model.MetricTypeGauge,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							gauge:  &Gauge{value: 11},
						},
					},
				},
			},
		},
		"Gauge with meta parsed as Gauge": {
			input: dataGaugeMeta,
			want: MetricFamilies{
				"test_gauge_metric_1": {
					name: "test_gauge_metric_1",
					help: "Test Gauge Metric 1",
					typ:  model.MetricTypeGauge,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							gauge:  &Gauge{value: 11},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							gauge:  &Gauge{value: 12},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							gauge:  &Gauge{value: 13},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							gauge:  &Gauge{value: 14},
						},
					},
				},
				"test_gauge_metric_2": {
					name: "test_gauge_metric_2",
					typ:  model.MetricTypeGauge,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							gauge:  &Gauge{value: 11},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							gauge:  &Gauge{value: 12},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							gauge:  &Gauge{value: 13},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							gauge:  &Gauge{value: 14},
						},
					},
				},
			},
		},
		"Counter with meta parsed as Counter": {
			input: dataCounterMeta,
			want: MetricFamilies{
				"test_counter_metric_1_total": {
					name: "test_counter_metric_1_total",
					help: "Test Counter Metric 1",
					typ:  model.MetricTypeCounter,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label1", Value: "value1"}},
							counter: &Counter{value: 11},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value2"}},
							counter: &Counter{value: 12},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value3"}},
							counter: &Counter{value: 13},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value4"}},
							counter: &Counter{value: 14},
						},
					},
				},
				"test_counter_metric_2_total": {
					name: "test_counter_metric_2_total",
					typ:  model.MetricTypeCounter,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label1", Value: "value1"}},
							counter: &Counter{value: 11},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value2"}},
							counter: &Counter{value: 12},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value3"}},
							counter: &Counter{value: 13},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value4"}},
							counter: &Counter{value: 14},
						},
					},
				},
			},
		},
		"Summary with meta parsed as Summary": {
			input: dataSummaryMeta,
			want: MetricFamilies{
				"test_summary_1_duration_microseconds": {
					name: "test_summary_1_duration_microseconds",
					help: "Test Summary Metric 1",
					typ:  model.MetricTypeSummary,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
					},
				},
				"test_summary_2_duration_microseconds": {
					name: "test_summary_2_duration_microseconds",
					typ:  model.MetricTypeSummary,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
					},
				},
			},
		},
		"Histogram with meta parsed as Histogram": {
			input: dataHistogramMeta,
			want: MetricFamilies{
				"test_histogram_1_duration_seconds": {
					name: "test_histogram_1_duration_seconds",
					help: "Test Histogram Metric 1",
					typ:  model.MetricTypeHistogram,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
					},
				},
				"test_histogram_2_duration_seconds": {
					name: "test_histogram_2_duration_seconds",
					typ:  model.MetricTypeHistogram,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
					},
				},
			},
		},
		"Gauge no meta parsed as Untyped": {
			input: dataGaugeNoMeta,
			want: MetricFamilies{
				"test_gauge_no_meta_metric_1": {
					name: "test_gauge_no_meta_metric_1",
					typ:  model.MetricTypeUnknown,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label1", Value: "value1"}},
							untyped: &Untyped{value: 11},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value2"}},
							untyped: &Untyped{value: 12},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value3"}},
							untyped: &Untyped{value: 13},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value4"}},
							untyped: &Untyped{value: 14},
						},
					},
				},
				"test_gauge_no_meta_metric_2": {
					name: "test_gauge_no_meta_metric_2",
					typ:  model.MetricTypeUnknown,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label1", Value: "value1"}},
							untyped: &Untyped{value: 11},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value2"}},
							untyped: &Untyped{value: 12},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value3"}},
							untyped: &Untyped{value: 13},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value4"}},
							untyped: &Untyped{value: 14},
						},
					},
				},
			},
		},
		"Counter no meta parsed as Untyped": {
			input: dataCounterNoMeta,
			want: MetricFamilies{
				"test_counter_no_meta_metric_1_total": {
					name: "test_counter_no_meta_metric_1_total",
					typ:  model.MetricTypeUnknown,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label1", Value: "value1"}},
							untyped: &Untyped{value: 11},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value2"}},
							untyped: &Untyped{value: 12},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value3"}},
							untyped: &Untyped{value: 13},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value4"}},
							untyped: &Untyped{value: 14},
						},
					},
				},
				"test_counter_no_meta_metric_2_total": {
					name: "test_counter_no_meta_metric_2_total",
					typ:  model.MetricTypeUnknown,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label1", Value: "value1"}},
							untyped: &Untyped{value: 11},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value2"}},
							untyped: &Untyped{value: 12},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value3"}},
							untyped: &Untyped{value: 13},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value4"}},
							untyped: &Untyped{value: 14},
						},
					},
				},
			},
		},
		"Summary no meta parsed as Summary": {
			input: dataSummaryNoMeta,
			want: MetricFamilies{
				"test_summary_no_meta_1_duration_microseconds": {
					name: "test_summary_no_meta_1_duration_microseconds",
					typ:  model.MetricTypeSummary,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
					},
				},
				"test_summary_no_meta_2_duration_microseconds": {
					name: "test_summary_no_meta_2_duration_microseconds",
					typ:  model.MetricTypeSummary,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
					},
				},
			},
		},
		"Histogram no meta parsed as Histogram": {
			input: dataHistogramNoMeta,
			want: MetricFamilies{
				"test_histogram_no_meta_1_duration_seconds": {
					name: "test_histogram_no_meta_1_duration_seconds",
					typ:  model.MetricTypeHistogram,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
					},
				},
				"test_histogram_no_meta_2_duration_seconds": {
					name: "test_histogram_no_meta_2_duration_seconds",
					typ:  model.MetricTypeHistogram,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
					},
				},
			},
		},
		"All types": {
			input: dataAllTypes,
			want: MetricFamilies{
				"test_gauge_metric_1": {
					name: "test_gauge_metric_1",
					help: "Test Gauge Metric 1",
					typ:  model.MetricTypeGauge,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							gauge:  &Gauge{value: 11},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							gauge:  &Gauge{value: 12},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							gauge:  &Gauge{value: 13},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							gauge:  &Gauge{value: 14},
						},
					},
				},
				"test_gauge_metric_2": {
					name: "test_gauge_metric_2",
					typ:  model.MetricTypeGauge,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							gauge:  &Gauge{value: 11},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							gauge:  &Gauge{value: 12},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							gauge:  &Gauge{value: 13},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							gauge:  &Gauge{value: 14},
						},
					},
				},
				"test_counter_metric_1_total": {
					name: "test_counter_metric_1_total",
					help: "Test Counter Metric 1",
					typ:  model.MetricTypeCounter,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label1", Value: "value1"}},
							counter: &Counter{value: 11},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value2"}},
							counter: &Counter{value: 12},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value3"}},
							counter: &Counter{value: 13},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value4"}},
							counter: &Counter{value: 14},
						},
					},
				},
				"test_counter_metric_2_total": {
					name: "test_counter_metric_2_total",
					typ:  model.MetricTypeCounter,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label1", Value: "value1"}},
							counter: &Counter{value: 11},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value2"}},
							counter: &Counter{value: 12},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value3"}},
							counter: &Counter{value: 13},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value4"}},
							counter: &Counter{value: 14},
						},
					},
				},
				"test_summary_1_duration_microseconds": {
					name: "test_summary_1_duration_microseconds",
					help: "Test Summary Metric 1",
					typ:  model.MetricTypeSummary,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
					},
				},
				"test_summary_2_duration_microseconds": {
					name: "test_summary_2_duration_microseconds",
					typ:  model.MetricTypeSummary,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
					},
				},
				"test_histogram_1_duration_seconds": {
					name: "test_histogram_1_duration_seconds",
					help: "Test Histogram Metric 1",
					typ:  model.MetricTypeHistogram,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
					},
				},
				"test_histogram_2_duration_seconds": {
					name: "test_histogram_2_duration_seconds",
					typ:  model.MetricTypeHistogram,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
					},
				},
				"test_gauge_no_meta_metric_1": {
					name: "test_gauge_no_meta_metric_1",
					typ:  model.MetricTypeUnknown,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label1", Value: "value1"}},
							untyped: &Untyped{value: 11},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value2"}},
							untyped: &Untyped{value: 12},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value3"}},
							untyped: &Untyped{value: 13},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value4"}},
							untyped: &Untyped{value: 14},
						},
					},
				},
				"test_gauge_no_meta_metric_2": {
					name: "test_gauge_no_meta_metric_2",
					typ:  model.MetricTypeUnknown,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label1", Value: "value1"}},
							untyped: &Untyped{value: 11},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value2"}},
							untyped: &Untyped{value: 12},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value3"}},
							untyped: &Untyped{value: 13},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value4"}},
							untyped: &Untyped{value: 14},
						},
					},
				},
				"test_counter_no_meta_metric_1_total": {
					name: "test_counter_no_meta_metric_1_total",
					typ:  model.MetricTypeUnknown,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label1", Value: "value1"}},
							untyped: &Untyped{value: 11},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value2"}},
							untyped: &Untyped{value: 12},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value3"}},
							untyped: &Untyped{value: 13},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value4"}},
							untyped: &Untyped{value: 14},
						},
					},
				},
				"test_counter_no_meta_metric_2_total": {
					name: "test_counter_no_meta_metric_2_total",
					typ:  model.MetricTypeUnknown,
					metrics: []Metric{
						{
							labels:  labels.Labels{{Name: "label1", Value: "value1"}},
							untyped: &Untyped{value: 11},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value2"}},
							untyped: &Untyped{value: 12},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value3"}},
							untyped: &Untyped{value: 13},
						},
						{
							labels:  labels.Labels{{Name: "label1", Value: "value4"}},
							untyped: &Untyped{value: 14},
						},
					},
				},
				"test_summary_no_meta_1_duration_microseconds": {
					name: "test_summary_no_meta_1_duration_microseconds",
					typ:  model.MetricTypeSummary,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							summary: &Summary{
								sum:   283201.29,
								count: 31,
								quantiles: []Quantile{
									{quantile: 0.5, value: 4931.921},
									{quantile: 0.9, value: 4932.921},
									{quantile: 0.99, value: 4933.921},
								},
							},
						},
					},
				},
				"test_summary_no_meta_2_duration_microseconds": {
					name: "test_summary_no_meta_2_duration_microseconds",
					typ:  model.MetricTypeSummary,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							summary: &Summary{
								sum:   383201.29,
								count: 41,
								quantiles: []Quantile{
									{quantile: 0.5, value: 5931.921},
									{quantile: 0.9, value: 5932.921},
									{quantile: 0.99, value: 5933.921},
								},
							},
						},
					},
				},
				"test_histogram_no_meta_1_duration_seconds": {
					name: "test_histogram_no_meta_1_duration_seconds",
					typ:  model.MetricTypeHistogram,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							histogram: &Histogram{
								sum:   0.00147889,
								count: 6,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 4},
									{upperBound: 0.5, cumulativeCount: 5},
									{upperBound: math.Inf(1), cumulativeCount: 6},
								},
							},
						},
					},
				},
				"test_histogram_no_meta_2_duration_seconds": {
					name: "test_histogram_no_meta_2_duration_seconds",
					typ:  model.MetricTypeHistogram,
					metrics: []Metric{
						{
							labels: labels.Labels{{Name: "label1", Value: "value1"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value2"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value3"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
						{
							labels: labels.Labels{{Name: "label1", Value: "value4"}},
							histogram: &Histogram{
								sum:   0.00247889,
								count: 9,
								buckets: []Bucket{
									{upperBound: 0.1, cumulativeCount: 7},
									{upperBound: 0.5, cumulativeCount: 8},
									{upperBound: math.Inf(1), cumulativeCount: 9},
								},
							},
						},
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var p promTextParser

			for i := range 10 {
				t.Run(fmt.Sprintf("parse num %d", i+1), func(t *testing.T) {
					mfs, err := p.parseToMetricFamilies(test.input)
					if len(test.want) > 0 {
						assert.Equal(t, test.want, mfs)
					} else {
						assert.Error(t, err)
					}
				})
			}
		})
	}
}

func TestPromTextParser_parseToMetricFamiliesWithSelector(t *testing.T) {
	sr, err := selector.Parse(`test_gauge_metric_1{label1="value2"}`)
	require.NoError(t, err)

	p := promTextParser{sr: sr}

	txt := []byte(`
test_gauge_metric_1{label1="value1"} 1
test_gauge_metric_1{label1="value2"} 1
test_gauge_metric_2{label1="value1"} 1
test_gauge_metric_2{label1="value2"} 1
`)

	want := MetricFamilies{
		"test_gauge_metric_1": &MetricFamily{
			name: "test_gauge_metric_1",
			typ:  model.MetricTypeUnknown,
			metrics: []Metric{
				{labels: labels.Labels{{Name: "label1", Value: "value2"}}, untyped: &Untyped{value: 1}},
			},
		},
	}

	mfs, err := p.parseToMetricFamilies(txt)

	require.NoError(t, err)
	assert.Equal(t, want, mfs)
}

func TestPromTextParser_parseToSeries(t *testing.T) {
	tests := map[string]struct {
		input []byte
		want  Series
	}{
		"label order matches textparse (__name__ not forced first)": {
			input: []byte("m{UUID=\"x\",gpu=\"0\"} 5\n"),
			want: Series{SeriesSample{
				Labels: labels.Labels{
					{Name: "UUID", Value: "x"},
					{Name: "__name__", Value: "m"},
					{Name: "gpu", Value: "0"},
				},
				Value: 5,
			}},
		},
		"All types": {
			input: []byte(`
# HELP test_gauge_metric_1 Test Gauge Metric 1
# TYPE test_gauge_metric_1 gauge
test_gauge_metric_1{label1="value1"} 11
test_gauge_no_meta_metric_1{label1="value1"} 11
# HELP test_counter_metric_1_total Test Counter Metric 1
# TYPE test_counter_metric_1_total counter
test_counter_metric_1_total{label1="value1"} 11
test_counter_no_meta_metric_1_total{label1="value1"} 11
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
`),
			want: Series{
				// Gauge
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_gauge_metric_1"},
						{Name: "label1", Value: "value1"},
					},
					Value: 11,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_gauge_no_meta_metric_1"},
						{Name: "label1", Value: "value1"},
					},
					Value: 11,
				},
				// Counter
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_counter_metric_1_total"},
						{Name: "label1", Value: "value1"},
					},
					Value: 11,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_counter_no_meta_metric_1_total"},
						{Name: "label1", Value: "value1"},
					},
					Value: 11,
				},
				//// Summary
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_summary_1_duration_microseconds"},
						{Name: "label1", Value: "value1"},
						{Name: "quantile", Value: "0.5"},
					},
					Value: 4931.921,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_summary_1_duration_microseconds"},
						{Name: "label1", Value: "value1"},
						{Name: "quantile", Value: "0.9"},
					},
					Value: 4932.921,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_summary_1_duration_microseconds"},
						{Name: "label1", Value: "value1"},
						{Name: "quantile", Value: "0.99"},
					},
					Value: 4933.921,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_summary_1_duration_microseconds_sum"},
						{Name: "label1", Value: "value1"},
					},
					Value: 283201.29,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_summary_1_duration_microseconds_count"},
						{Name: "label1", Value: "value1"},
					},
					Value: 31,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_summary_no_meta_1_duration_microseconds"},
						{Name: "label1", Value: "value1"},
						{Name: "quantile", Value: "0.5"},
					},
					Value: 4931.921,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_summary_no_meta_1_duration_microseconds"},
						{Name: "label1", Value: "value1"},
						{Name: "quantile", Value: "0.9"},
					},
					Value: 4932.921,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_summary_no_meta_1_duration_microseconds"},
						{Name: "label1", Value: "value1"},
						{Name: "quantile", Value: "0.99"},
					},
					Value: 4933.921,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_summary_no_meta_1_duration_microseconds_sum"},
						{Name: "label1", Value: "value1"},
					},
					Value: 283201.29,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_summary_no_meta_1_duration_microseconds_count"},
						{Name: "label1", Value: "value1"},
					},
					Value: 31,
				},
				// Histogram
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_histogram_1_duration_seconds_bucket"},
						{Name: "label1", Value: "value1"},
						{Name: "le", Value: "0.1"},
					},
					Value: 4,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_histogram_1_duration_seconds_bucket"},
						{Name: "label1", Value: "value1"},
						{Name: "le", Value: "0.5"},
					},
					Value: 5,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_histogram_1_duration_seconds_bucket"},
						{Name: "label1", Value: "value1"},
						{Name: "le", Value: "+Inf"},
					},
					Value: 6,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_histogram_1_duration_seconds_sum"},
						{Name: "label1", Value: "value1"},
					},
					Value: 0.00147889,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_histogram_1_duration_seconds_count"},
						{Name: "label1", Value: "value1"},
					},
					Value: 6,
				},

				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_histogram_no_meta_1_duration_seconds_bucket"},
						{Name: "label1", Value: "value1"},
						{Name: "le", Value: "0.1"},
					},
					Value: 4,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_histogram_no_meta_1_duration_seconds_bucket"},
						{Name: "label1", Value: "value1"},
						{Name: "le", Value: "0.5"},
					},
					Value: 5,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_histogram_no_meta_1_duration_seconds_bucket"},
						{Name: "label1", Value: "value1"},
						{Name: "le", Value: "+Inf"},
					},
					Value: 6,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_histogram_no_meta_1_duration_seconds_sum"},
						{Name: "label1", Value: "value1"},
					},
					Value: 0.00147889,
				},
				{
					Labels: labels.Labels{
						{Name: "__name__", Value: "test_histogram_no_meta_1_duration_seconds_count"},
						{Name: "label1", Value: "value1"},
					},
					Value: 6,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var p promTextParser

			for i := range 10 {
				t.Run(fmt.Sprintf("parse num %d", i+1), func(t *testing.T) {
					series, err := p.parseToSeries(test.input)

					if len(test.want) > 0 {
						test.want.Sort()
						assert.Equal(t, test.want, series)
					} else {
						assert.Error(t, err)
					}
				})
			}
		})
	}
}

func TestPromTextParser_parseToSeriesWithSelector(t *testing.T) {
	sr, err := selector.Parse(`test_gauge_metric_1{label1="value2"}`)
	require.NoError(t, err)

	p := promTextParser{sr: sr}

	txt := []byte(`
test_gauge_metric_1{label1="value1"} 1
test_gauge_metric_1{label1="value2"} 1
test_gauge_metric_2{label1="value1"} 1
test_gauge_metric_2{label1="value2"} 1
`)

	want := Series{SeriesSample{
		Labels: labels.Labels{
			{Name: "__name__", Value: "test_gauge_metric_1"},
			{Name: "label1", Value: "value2"},
		},
		Value: 1,
	}}

	series, err := p.parseToSeries(txt)

	require.NoError(t, err)
	assert.Equal(t, want, series)
}

func TestPromTextParser_parseToMetricFamilies_failsOnInvalidSeriesValue(t *testing.T) {
	var p promTextParser

	txt := []byte(`
# HELP DCGM_FI_DEV_GPU_UTIL GPU utilization
# TYPE DCGM_FI_DEV_GPU_UTIL gauge
DCGM_FI_DEV_GPU_UTIL{UUID="GPU-aaa",gpu="0"} 80
# HELP DCGM_FI_DEV_REQUESTED_POWER_PROFILE_MASK Requested power profile mask
# TYPE DCGM_FI_DEV_REQUESTED_POWER_PROFILE_MASK gauge
DCGM_FI_DEV_REQUESTED_POWER_PROFILE_MASK{UUID="GPU-aaa",gpu="0"} ERROR - FAILED TO CONVERT TO STRING
`)

	_, err := p.parseToMetricFamilies(txt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse prometheus metrics")
}

func TestPromTextParser_parseToSeries_failsOnInvalidSeriesValue(t *testing.T) {
	var p promTextParser

	txt := []byte(`
DCGM_FI_DEV_GPU_UTIL{UUID="GPU-aaa",gpu="0"} 80
DCGM_FI_DEV_REQUESTED_POWER_PROFILE_MASK{UUID="GPU-aaa",gpu="0"} ERROR - FAILED TO CONVERT TO STRING
`)

	_, err := p.parseToSeries(txt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse prometheus metrics")
}

func joinData(data ...[]byte) []byte {
	var buf bytes.Buffer
	for _, v := range data {
		_, _ = buf.Write(v)
		_ = buf.WriteByte('\n')
	}
	return buf.Bytes()
}

func TestPromTextParser_parseSamples(t *testing.T) {
	type wantSample struct {
		name       string
		labels     string
		value      float64
		kind       SampleKind
		familyType model.MetricType
	}

	tests := map[string]struct {
		input    []byte
		wantHelp []string
		want     []wantSample
	}{
		"deferred _sum emits after a later unrelated metric (cross-metric reorder)": {
			input: []byte("a_sum 1\nb 2\n# TYPE a summary\na{quantile=\"0.5\"} 3\n"),
			want: []wantSample{
				{"b", `{}`, 2, SampleKindScalar, model.MetricTypeUnknown},
				{"a_sum", `{}`, 1, SampleKindSummarySum, model.MetricTypeSummary},
				{"a", `{quantile="0.5"}`, 3, SampleKindSummaryQuantile, model.MetricTypeSummary},
			},
		},
		"all metric types are classified in exposition order": {
			input: []byte(`# HELP test_gauge A gauge metric.
# TYPE test_gauge gauge
test_gauge{label="a"} 1
# TYPE test_counter_total counter
test_counter_total 5
# TYPE test_hist histogram
test_hist_bucket{le="0.1"} 1
test_hist_bucket{le="+Inf"} 2
test_hist_sum 3
test_hist_count 2
# TYPE test_summary summary
test_summary{quantile="0.5"} 0.2
test_summary_sum 1
test_summary_count 10
`),
			wantHelp: []string{"test_gauge=A gauge metric."},
			want: []wantSample{
				{"test_gauge", `{label="a"}`, 1, SampleKindScalar, model.MetricTypeGauge},
				{"test_counter_total", `{}`, 5, SampleKindScalar, model.MetricTypeCounter},
				{"test_hist_bucket", `{le="0.1"}`, 1, SampleKindHistogramBucket, model.MetricTypeHistogram},
				{"test_hist_bucket", `{le="+Inf"}`, 2, SampleKindHistogramBucket, model.MetricTypeHistogram},
				{"test_hist_sum", `{}`, 3, SampleKindHistogramSum, model.MetricTypeHistogram},
				{"test_hist_count", `{}`, 2, SampleKindHistogramCount, model.MetricTypeHistogram},
				{"test_summary", `{quantile="0.5"}`, 0.2, SampleKindSummaryQuantile, model.MetricTypeSummary},
				{"test_summary_sum", `{}`, 1, SampleKindSummarySum, model.MetricTypeSummary},
				{"test_summary_count", `{}`, 10, SampleKindSummaryCount, model.MetricTypeSummary},
			},
		},
		"__name__ is delivered via Name and excluded from Labels": {
			input: []byte("# TYPE m gauge\nm{a=\"1\",b=\"2\"} 7\n"),
			want: []wantSample{
				{"m", `{a="1", b="2"}`, 7, SampleKindScalar, model.MetricTypeGauge},
			},
		},
		"deferred sum/count before # TYPE are buffered and back-resolved": {
			input: []byte(`my_summary_sum 10
my_summary_count 2
# TYPE my_summary summary
my_summary{quantile="0.5"} 0.5
`),
			want: []wantSample{
				{"my_summary_sum", `{}`, 10, SampleKindSummarySum, model.MetricTypeSummary},
				{"my_summary_count", `{}`, 2, SampleKindSummaryCount, model.MetricTypeSummary},
				{"my_summary", `{quantile="0.5"}`, 0.5, SampleKindSummaryQuantile, model.MetricTypeSummary},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var p promTextParser

			for i := range 10 {
				t.Run(fmt.Sprintf("parse num %d", i+1), func(t *testing.T) {
					var got []wantSample
					var help []string

					err := p.driver.parseSamples(test.input, true,
						func(name, h string) { help = append(help, name+"="+h) },
						func(s Sample) error {
							assert.Falsef(t, s.Labels.Has(labels.MetricName),
								"sample %q must not carry __name__ in Labels", s.Name)
							got = append(got, wantSample{s.Name, s.Labels.String(), s.Value, s.Kind, s.FamilyType})
							return nil
						},
					)
					require.NoError(t, err)
					assert.Equal(t, test.want, got)
					for _, h := range test.wantHelp {
						assert.Contains(t, help, h)
					}
				})
			}
		})
	}
}

// The stream supports Prometheus-style relabeling on __name__/le/quantile.
// ownLabels=true isolates each sample's labels, so a transform can rename via
// Name and mutate Labels in place without affecting later samples.
func TestPromTextParser_parseSamples_relabelStyle(t *testing.T) {
	data := []byte(`# TYPE req_seconds histogram
req_seconds_bucket{le="0.1",path="/a"} 1
req_seconds_bucket{le="+Inf",path="/a"} 3
# TYPE rpc summary
rpc{quantile="0.99",path="/a"} 0.5
`)

	type out struct {
		name     string
		le       string
		quantile string
		labels   string
	}

	var p promTextParser
	var got []out
	err := p.driver.parseSamples(data, true, nil, func(s Sample) error {
		o := out{
			name:     s.Name + ":relabeled", // __name__ is mutable via Name
			le:       s.Labels.Get(bucketLabel),
			quantile: s.Labels.Get(quantileLabel),
		}
		// Drop the "path" target label in place (this sample owns its labels).
		kept := s.Labels[:0]
		for _, l := range s.Labels {
			if l.Name == "path" {
				continue
			}
			kept = append(kept, l)
		}
		o.labels = labels.Labels(kept).String()
		got = append(got, o)
		return nil
	})
	require.NoError(t, err)

	assert.Equal(t, []out{
		{name: "req_seconds_bucket:relabeled", le: "0.1", quantile: "", labels: `{le="0.1"}`},
		{name: "req_seconds_bucket:relabeled", le: "+Inf", quantile: "", labels: `{le="+Inf"}`},
		{name: "rpc:relabeled", le: "", quantile: "0.99", labels: `{quantile="0.99"}`},
	}, got)
}

// A relabel step's mutations — a renamed Name and changed Labels — are what Assemble
// folds: it must assemble the MUTATED sample, not the original. Mutating Labels in
// place also exercises the ownLabels=true sample stream from parseToSamples.
func TestAssemble_reflectsMutatedSamples(t *testing.T) {
	input := []byte("# TYPE old_name gauge\nold_name{keep=\"yes\",drop=\"me\"} 42\n")

	var p promTextParser
	batch, err := p.parseToSamples(input)
	require.NoError(t, err)

	// Rename the metric and drop the "drop" label in place (each sample owns its labels).
	for i := range batch.Samples {
		batch.Samples[i].Name = "new_name"
		kept := batch.Samples[i].Labels[:0]
		for _, l := range batch.Samples[i].Labels {
			if l.Name == "drop" {
				continue
			}
			kept = append(kept, l)
		}
		batch.Samples[i].Labels = kept
	}

	want := MetricFamilies{
		"new_name": {
			name: "new_name",
			typ:  model.MetricTypeGauge,
			metrics: []Metric{
				{labels: labels.Labels{{Name: "keep", Value: "yes"}}, gauge: &Gauge{value: 42}},
			},
		},
	}

	mfs, err := Assemble(batch)
	require.NoError(t, err)
	assert.Equal(t, want, mfs)
}
