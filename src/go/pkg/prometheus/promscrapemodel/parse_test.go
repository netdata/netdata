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

func TestParser_Parse(t *testing.T) {
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

			samples, err := p.Parse(test.input)
			require.NoError(t, err)
			assert.Equal(t, test.want, samples)
		})
	}
}
