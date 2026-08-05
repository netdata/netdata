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
