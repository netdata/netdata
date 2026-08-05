// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInspectWriterEligibilityUsesInitializedWriterPolicy(t *testing.T) {
	collector := New()
	collector.URL = "file:///unused"
	require.NoError(t, collector.Init(context.Background()))
	defer collector.Cleanup(context.Background())

	families := scrape(t, `
# TYPE app_value gauge
app_value 1
# TYPE app_build_info gauge
app_build_info{version="test"} 1
# TYPE app_empty summary
app_empty_sum 0
app_empty_count 0
`)

	got, err := collector.InspectWriterEligibility(families)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, WriterEligibility{Family: "app_build_info", TotalSeries: 1, RejectionReason: PipelineReasonInfoFamily}, got[0])
	assert.Equal(t, WriterEligibility{Family: "app_empty", TotalSeries: 1, RejectionReason: PipelineReasonInvalidFamilySchema}, got[1])
	assert.Equal(t, WriterEligibility{Family: "app_value", TotalSeries: 1, WritableSeries: 1}, got[2])
}

func TestInspectWriterEligibilityClassifiesAllInvalidValues(t *testing.T) {
	collector := New()
	collector.URL = "file:///unused"
	require.NoError(t, collector.Init(context.Background()))
	defer collector.Cleanup(context.Background())

	families := scrape(t, `
# TYPE app_gauge gauge
app_gauge NaN
# TYPE app_histogram histogram
app_histogram_bucket{le="1"} 1
app_histogram_bucket{le="+Inf"} 1
app_histogram_sum +Inf
app_histogram_count 1
# TYPE app_summary summary
app_summary{quantile="0.5"} NaN
app_summary_sum 0
app_summary_count 0
`)

	got, err := collector.InspectWriterEligibility(families)
	require.NoError(t, err)
	require.Len(t, got, 3)
	for _, item := range got {
		assert.Equal(t, PipelineReasonInvalidSeriesValue, item.RejectionReason, item.Family)
		assert.Zero(t, item.WritableSeries, item.Family)
	}
}
