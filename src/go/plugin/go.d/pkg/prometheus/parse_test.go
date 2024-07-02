package prometheus

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus/selector"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

			for i := 0; i < 10; i++ {
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

			for i := 0; i < 10; i++ {
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

func joinData(data ...[]byte) []byte {
	var buf bytes.Buffer
	for _, v := range data {
		_, _ = buf.Write(v)
		_ = buf.WriteByte('\n')
	}
	return buf.Bytes()
}
