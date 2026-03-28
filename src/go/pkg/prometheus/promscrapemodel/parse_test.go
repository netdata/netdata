// SPDX-License-Identifier: GPL-3.0-or-later

package promscrapemodel

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
)

func TestParser_ParseStreamSamples(t *testing.T) {
	tests := map[string]struct {
		selector string
		input    []byte
		want     Samples
	}{
		"mixed kinds": {
			input: []byte(`
# TYPE test_gauge gauge
test_gauge{label1="value1"} 11
# TYPE test_counter_total counter
test_counter_total{label1="value1"} 22
# TYPE test_summary summary
test_summary{label1="value1",quantile="0.5"} 1
test_summary_sum{label1="value1"} 2
test_summary_count{label1="value1"} 3
# TYPE test_histogram histogram
test_histogram_bucket{label1="value1",le="0.5"} 4
test_histogram_sum{label1="value1"} 5
test_histogram_count{label1="value1"} 6
`),
			want: Samples{
				{
					Name:       "test_gauge",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      11,
					Kind:       SampleKindScalar,
					FamilyType: model.MetricTypeGauge,
				},
				{
					Name:       "test_counter_total",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      22,
					Kind:       SampleKindScalar,
					FamilyType: model.MetricTypeCounter,
				},
				{
					Name: "test_summary",
					Labels: labels.Labels{
						{Name: "label1", Value: "value1"},
						{Name: "quantile", Value: "0.5"},
					},
					Value:      1,
					Kind:       SampleKindSummaryQuantile,
					FamilyType: model.MetricTypeSummary,
				},
				{
					Name:       "test_summary_sum",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      2,
					Kind:       SampleKindSummarySum,
					FamilyType: model.MetricTypeSummary,
				},
				{
					Name:       "test_summary_count",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      3,
					Kind:       SampleKindSummaryCount,
					FamilyType: model.MetricTypeSummary,
				},
				{
					Name: "test_histogram_bucket",
					Labels: labels.Labels{
						{Name: "label1", Value: "value1"},
						{Name: "le", Value: "0.5"},
					},
					Value:      4,
					Kind:       SampleKindHistogramBucket,
					FamilyType: model.MetricTypeHistogram,
				},
				{
					Name:       "test_histogram_sum",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      5,
					Kind:       SampleKindHistogramSum,
					FamilyType: model.MetricTypeHistogram,
				},
				{
					Name:       "test_histogram_count",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      6,
					Kind:       SampleKindHistogramCount,
					FamilyType: model.MetricTypeHistogram,
				},
			},
		},
		"reclassifies no meta sum and count": {
			input: []byte(`
test_summary_count{label1="value1"} 3
test_summary_sum{label1="value1"} 2
test_summary{label1="value1",quantile="0.5"} 1
test_histogram_count{label1="value1"} 6
test_histogram_sum{label1="value1"} 5
test_histogram_bucket{label1="value1",le="0.5"} 4
`),
			want: Samples{
				{
					Name:       "test_summary_count",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      3,
					Kind:       SampleKindSummaryCount,
					FamilyType: model.MetricTypeSummary,
				},
				{
					Name:       "test_summary_sum",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      2,
					Kind:       SampleKindSummarySum,
					FamilyType: model.MetricTypeSummary,
				},
				{
					Name: "test_summary",
					Labels: labels.Labels{
						{Name: "label1", Value: "value1"},
						{Name: "quantile", Value: "0.5"},
					},
					Value:      1,
					Kind:       SampleKindSummaryQuantile,
					FamilyType: model.MetricTypeSummary,
				},
				{
					Name:       "test_histogram_count",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      6,
					Kind:       SampleKindHistogramCount,
					FamilyType: model.MetricTypeHistogram,
				},
				{
					Name:       "test_histogram_sum",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      5,
					Kind:       SampleKindHistogramSum,
					FamilyType: model.MetricTypeHistogram,
				},
				{
					Name: "test_histogram_bucket",
					Labels: labels.Labels{
						{Name: "label1", Value: "value1"},
						{Name: "le", Value: "0.5"},
					},
					Value:      4,
					Kind:       SampleKindHistogramBucket,
					FamilyType: model.MetricTypeHistogram,
				},
			},
		},
		"keeps explicit scalar sum and count types": {
			input: []byte(`
# TYPE handler_latency_test_sum counter
handler_latency_test_sum{label1="value1"} 2
# TYPE handler_latency_test_count counter
handler_latency_test_count{label1="value1"} 3
`),
			want: Samples{
				{
					Name:       "handler_latency_test_sum",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      2,
					Kind:       SampleKindScalar,
					FamilyType: model.MetricTypeCounter,
				},
				{
					Name:       "handler_latency_test_count",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      3,
					Kind:       SampleKindScalar,
					FamilyType: model.MetricTypeCounter,
				},
			},
		},
		"selector filters raw labels": {
			selector: `test_metric{label1="value2"}`,
			input: []byte(`
test_metric{label1="value1"} 1
test_metric{label1="value2"} 2
test_other{label1="value2"} 3
`),
			want: Samples{
				{
					Name:       "test_metric",
					Labels:     labels.Labels{{Name: "label1", Value: "value2"}},
					Value:      2,
					Kind:       SampleKindScalar,
					FamilyType: model.MetricTypeUnknown,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var p Parser
			if test.selector != "" {
				sr, err := selector.Parse(test.selector)
				require.NoError(t, err)
				p = NewParser(sr)
			}

			var samples Samples
			err := p.ParseStream(test.input, func(sample Sample) error {
				samples.Add(sample)
				return nil
			})
			require.NoError(t, err)
			assert.Equal(t, test.want, samples)
		})
	}
}

func TestParser_ParseStream(t *testing.T) {
	tests := map[string]struct {
		input    []byte
		wantHelp map[string]string
		want     Samples
	}{
		"emits help and final classified samples": {
			input: []byte(`
# HELP test_summary Test Summary
test_summary_count{label1="value1"} 3
test_summary_sum{label1="value1"} 2
test_summary{label1="value1",quantile="0.5"} 1
test_histogram_count{label1="value1"} 6
test_histogram_sum{label1="value1"} 5
test_histogram_bucket{label1="value1",le="0.5"} 4
`),
			wantHelp: map[string]string{
				"test_summary": "Test Summary",
			},
			want: Samples{
				{
					Name:       "test_summary_count",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      3,
					Kind:       SampleKindSummaryCount,
					FamilyType: model.MetricTypeSummary,
				},
				{
					Name:       "test_summary_sum",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      2,
					Kind:       SampleKindSummarySum,
					FamilyType: model.MetricTypeSummary,
				},
				{
					Name: "test_summary",
					Labels: labels.Labels{
						{Name: "label1", Value: "value1"},
						{Name: "quantile", Value: "0.5"},
					},
					Value:      1,
					Kind:       SampleKindSummaryQuantile,
					FamilyType: model.MetricTypeSummary,
				},
				{
					Name:       "test_histogram_count",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      6,
					Kind:       SampleKindHistogramCount,
					FamilyType: model.MetricTypeHistogram,
				},
				{
					Name:       "test_histogram_sum",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      5,
					Kind:       SampleKindHistogramSum,
					FamilyType: model.MetricTypeHistogram,
				},
				{
					Name: "test_histogram_bucket",
					Labels: labels.Labels{
						{Name: "label1", Value: "value1"},
						{Name: "le", Value: "0.5"},
					},
					Value:      4,
					Kind:       SampleKindHistogramBucket,
					FamilyType: model.MetricTypeHistogram,
				},
			},
		},
		"unresolved sum and count remain unknown scalar at eof": {
			input: []byte(`
test_unknown_count{label1="value1"} 1
test_unknown_sum{label1="value1"} 2
`),
			wantHelp: map[string]string{},
			want: Samples{
				{
					Name:       "test_unknown_count",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      1,
					Kind:       SampleKindScalar,
					FamilyType: model.MetricTypeUnknown,
				},
				{
					Name:       "test_unknown_sum",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      2,
					Kind:       SampleKindScalar,
					FamilyType: model.MetricTypeUnknown,
				},
			},
		},
		"keeps explicit scalar sum and count types": {
			input: []byte(`
# TYPE handler_latency_test_sum counter
handler_latency_test_sum{label1="value1"} 2
# TYPE handler_latency_test_count counter
handler_latency_test_count{label1="value1"} 3
`),
			wantHelp: map[string]string{},
			want: Samples{
				{
					Name:       "handler_latency_test_sum",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      2,
					Kind:       SampleKindScalar,
					FamilyType: model.MetricTypeCounter,
				},
				{
					Name:       "handler_latency_test_count",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      3,
					Kind:       SampleKindScalar,
					FamilyType: model.MetricTypeCounter,
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var (
				p    Parser
				help = make(map[string]string)
				got  Samples
			)

			err := p.ParseStreamWithMeta(
				test.input,
				func(name, helpText string) { help[name] = helpText },
				func(sample Sample) error {
					got.Add(sample)
					return nil
				},
			)
			require.NoError(t, err)
			assert.Equal(t, test.wantHelp, help)
			assert.Equal(t, test.want, got)
		})
	}
}
