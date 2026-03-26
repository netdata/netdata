// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
)

func TestAssembler_MetricFamilies(t *testing.T) {
	tests := map[string]struct {
		apply  func(a *Assembler) error
		verify func(t *testing.T, mfs MetricFamilies)
	}{
		"assembles mixed families": {
			apply: func(a *Assembler) error {
				a.beginCycle()
				a.applyHelp("test_gauge", "Test Gauge")
				a.applyHelp("test_summary", "Test Summary")
				a.applyHelp("test_histogram", "Test Histogram")

				for _, sample := range []promscrapemodel.Sample{
					{
						Name:       "test_gauge",
						Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
						Value:      11,
						Kind:       promscrapemodel.SampleKindScalar,
						FamilyType: model.MetricTypeGauge,
					},
					{
						Name: "test_summary",
						Labels: labels.Labels{
							{Name: "label1", Value: "value1"},
							{Name: "quantile", Value: "0.5"},
						},
						Value:      1,
						Kind:       promscrapemodel.SampleKindSummaryQuantile,
						FamilyType: model.MetricTypeSummary,
					},
					{
						Name:       "test_summary_sum",
						Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
						Value:      2,
						Kind:       promscrapemodel.SampleKindSummarySum,
						FamilyType: model.MetricTypeSummary,
					},
					{
						Name:       "test_summary_count",
						Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
						Value:      3,
						Kind:       promscrapemodel.SampleKindSummaryCount,
						FamilyType: model.MetricTypeSummary,
					},
					{
						Name: "test_histogram_bucket",
						Labels: labels.Labels{
							{Name: "label1", Value: "value1"},
							{Name: "le", Value: "0.5"},
						},
						Value:      4,
						Kind:       promscrapemodel.SampleKindHistogramBucket,
						FamilyType: model.MetricTypeHistogram,
					},
					{
						Name:       "test_histogram_sum",
						Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
						Value:      5,
						Kind:       promscrapemodel.SampleKindHistogramSum,
						FamilyType: model.MetricTypeHistogram,
					},
					{
						Name:       "test_histogram_count",
						Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
						Value:      6,
						Kind:       promscrapemodel.SampleKindHistogramCount,
						FamilyType: model.MetricTypeHistogram,
					},
				} {
					if err := a.ApplySample(sample); err != nil {
						return err
					}
				}

				return nil
			},
			verify: func(t *testing.T, mfs MetricFamilies) {
				require.Len(t, mfs, 3)

				gauge := mfs.GetGauge("test_gauge")
				require.NotNil(t, gauge)
				assert.Equal(t, "Test Gauge", gauge.Help())
				require.Len(t, gauge.Metrics(), 1)
				assert.Equal(t, 11.0, gauge.Metrics()[0].Gauge().Value())

				summary := mfs.GetSummary("test_summary")
				require.NotNil(t, summary)
				assert.Equal(t, "Test Summary", summary.Help())
				require.Len(t, summary.Metrics(), 1)
				assert.Equal(t, 2.0, summary.Metrics()[0].Summary().Sum())
				assert.Equal(t, 3.0, summary.Metrics()[0].Summary().Count())
				assert.Equal(t, []Quantile{{quantile: 0.5, value: 1}}, summary.Metrics()[0].Summary().Quantiles())

				histogram := mfs.GetHistogram("test_histogram")
				require.NotNil(t, histogram)
				assert.Equal(t, "Test Histogram", histogram.Help())
				require.Len(t, histogram.Metrics(), 1)
				assert.Equal(t, 5.0, histogram.Metrics()[0].Histogram().Sum())
				assert.Equal(t, 6.0, histogram.Metrics()[0].Histogram().Count())
				assert.Equal(t, []Bucket{{upperBound: 0.5, cumulativeCount: 4}}, histogram.Metrics()[0].Histogram().Buckets())
			},
		},
		"sealed readout is idempotent and next sample starts new cycle": {
			apply: func(a *Assembler) error {
				a.beginCycle()
				if err := a.ApplySample(promscrapemodel.Sample{
					Name:       "first_metric",
					Labels:     labels.Labels{{Name: "label1", Value: "value1"}},
					Value:      1,
					Kind:       promscrapemodel.SampleKindScalar,
					FamilyType: model.MetricTypeGauge,
				}); err != nil {
					return err
				}

				first := a.MetricFamilies()
				require.Len(t, first, 1)
				assert.NotNil(t, first.GetGauge("first_metric"))

				secondRead := a.MetricFamilies()
				assert.Equal(t, first, secondRead)

				return a.ApplySample(promscrapemodel.Sample{
					Name:       "second_metric",
					Labels:     labels.Labels{{Name: "label1", Value: "value2"}},
					Value:      2,
					Kind:       promscrapemodel.SampleKindScalar,
					FamilyType: model.MetricTypeGauge,
				})
			},
			verify: func(t *testing.T, mfs MetricFamilies) {
				require.Len(t, mfs, 1)
				assert.Nil(t, mfs.Get("first_metric"))
				second := mfs.GetGauge("second_metric")
				require.NotNil(t, second)
				require.Len(t, second.Metrics(), 1)
				assert.Equal(t, 2.0, second.Metrics()[0].Gauge().Value())
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			a := NewAssembler()
			require.NoError(t, test.apply(a))
			test.verify(t, a.MetricFamilies())
		})
	}
}
