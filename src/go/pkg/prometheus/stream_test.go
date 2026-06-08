// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"math"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromTextParser_parseToStream(t *testing.T) {
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

					err := p.parseToStream(test.input,
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

// Deferred _sum/_count (emitted before the family type is known) fold into the
// typed family once it is resolved — criterion #5 at the assembled level.
func TestPromTextParser_parseToMetricFamilies_deferredClassification(t *testing.T) {
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
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var p promTextParser

			for i := range 10 {
				t.Run(fmt.Sprintf("parse num %d", i+1), func(t *testing.T) {
					mfs, err := p.parseToMetricFamilies(test.input)
					require.NoError(t, err)
					assert.Equal(t, test.want, mfs)
				})
			}
		})
	}
}

// The stream supports Prometheus-style relabeling on __name__/le/quantile.
// ownLabels=true isolates each sample's labels, so a transform can rename via
// Name and mutate Labels in place without affecting later samples.
func TestPromTextParser_parseToStream_relabelStyle(t *testing.T) {
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
	err := p.parseToStream(data, nil, func(s Sample) error {
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

func TestPromTextParser_parseToStream_nilCallbackIsNoop(t *testing.T) {
	var p promTextParser
	require.NoError(t, p.parseToStream([]byte("metric 1\n"), nil, nil))
}

// Byte-identical series ordering: textparse sorts labels, so __name__ is NOT
// always first — a label like "UUID" (0x55) sorts before "__name__" (0x5f).
// ScrapeSeries must preserve that raw sorted order (same as the legacy parser),
// not force __name__ first, which would also break the labels.Labels sorted
// invariant.
func TestPromTextParser_parseToSeries_labelOrderMatchesTextparse(t *testing.T) {
	var p promTextParser
	series, err := p.parseToSeries([]byte("m{UUID=\"x\",gpu=\"0\"} 5\n"))
	require.NoError(t, err)
	require.Len(t, series, 1)
	assert.Equal(t, `{UUID="x", __name__="m", gpu="0"}`, series[0].Labels.String())
}

// The deferred _sum/_count buffer (criterion #5) can emit a _sum after a later,
// unrelated sample: here a_sum is buffered (type unknown), b is emitted, then
// a_sum resolves on "# TYPE a summary". Documents the ScrapeStream order caveat.
func TestPromTextParser_parseToStream_deferralReordersAcrossMetrics(t *testing.T) {
	data := []byte("a_sum 1\nb 2\n# TYPE a summary\na{quantile=\"0.5\"} 3\n")

	var p promTextParser
	var names []string
	err := p.parseToStream(data, nil, func(s Sample) error {
		names = append(names, s.Name)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"b", "a_sum", "a"}, names)
}

// A _bucket-named series is a histogram bucket only if it carries an "le" label.
// Without le it is malformed and is kept as a plain metric (value preserved) — not
// folded into the histogram family. (Legacy folded it and dropped the value;
// valid buckets always have le, so real histograms are unaffected.)
func TestPromTextParser_parseToMetricFamilies_bucketRequiresLe(t *testing.T) {
	tests := map[string]struct {
		input    []byte
		wantFam  string
		wantType model.MetricType
	}{
		"valid _bucket (has le) folds into the histogram family": {
			input:    []byte("# TYPE h histogram\nh_bucket{le=\"1\",label=\"x\"} 1\n"),
			wantFam:  "h",
			wantType: model.MetricTypeHistogram,
		},
		"_bucket without le is a plain metric, not a histogram bucket": {
			input:    []byte("# TYPE h histogram\nh_bucket{label=\"x\"} 1\n"),
			wantFam:  "h_bucket",
			wantType: model.MetricTypeUnknown,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var p promTextParser
			mfs, err := p.parseToMetricFamilies(tc.input)
			require.NoError(t, err)

			mf := mfs.Get(tc.wantFam)
			require.NotNilf(t, mf, "expected family %q", tc.wantFam)
			assert.Equal(t, tc.wantType, mf.Type())
			assert.Len(t, mf.Metrics(), 1)
		})
	}
}
