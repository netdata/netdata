// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
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
	assert.Equal(t, WriterEligibility{Family: "app_build_info"}, got[0])
	assert.Equal(t, WriterEligibility{Family: "app_empty", WritableSeries: 1}, got[1])
	assert.Equal(t, WriterEligibility{Family: "app_value", WritableSeries: 1}, got[2])
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
		assert.Zero(t, item.WritableSeries, item.Family)
	}
}

func TestInspectWriterEligibilityHonorsExistingFamilyContract(t *testing.T) {
	tests := map[string]struct {
		initial string
		inspect string
	}{
		"family type drift": {
			initial: "# TYPE app_value gauge\napp_value 1\n",
			inspect: "# TYPE app_value counter\napp_value 2\n",
		},
		"distribution schema drift": {
			initial: "# TYPE app_value histogram\napp_value_bucket{le=\"1\"} 1\napp_value_bucket{le=\"+Inf\"} 1\napp_value_sum 1\napp_value_count 1\n",
			inspect: "# TYPE app_value histogram\napp_value_bucket{le=\"2\"} 1\napp_value_bucket{le=\"+Inf\"} 1\napp_value_sum 1\napp_value_count 1\n",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			dumpPath := filepath.Join(t.TempDir(), "metrics.txt")
			require.NoError(t, os.WriteFile(dumpPath, []byte(tc.initial), 0o600))

			collector := New()
			collector.URL = "file://" + dumpPath
			require.NoError(t, collector.Init(context.Background()))
			defer collector.Cleanup(context.Background())
			require.NoError(t, collector.Check(context.Background()))

			managed, ok := metrix.AsCycleManagedStore(collector.MetricStore())
			require.True(t, ok)
			managed.CycleController().BeginCycle()
			require.NoError(t, collector.Collect(context.Background()))
			require.NoError(t, managed.CycleController().CommitCycleSuccess())

			got, err := collector.InspectWriterEligibility(scrape(t, tc.inspect))
			require.NoError(t, err)
			require.Equal(t, []WriterEligibility{{Family: "app_value"}}, got)
		})
	}
}
