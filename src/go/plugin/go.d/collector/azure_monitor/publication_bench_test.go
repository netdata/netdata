// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

// BenchmarkPublication measures framework publication for discovered PostgreSQL
// flexible servers; Collect (mocked Azure clients) is untimed and each cycle
// advances the clock by one minute so every metric is due.
func BenchmarkPublication(b *testing.B) {
	const resources = 50
	now := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	rg := &mockResourceGraph{}
	mx := &mockMetricsClient{}
	for i := range resources {
		name := fmt.Sprintf("pg-%d", i)
		rg.resources = append(rg.resources, map[string]any{
			"id":            "/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/" + name,
			"name":          name,
			"type":          "Microsoft.DBforPostgreSQL/flexibleServers",
			"resourceGroup": "rg-a",
			"location":      "eastus",
		})
	}
	respond := func(ts time.Time) {
		values := make([]azmetrics.MetricData, 0, resources)
		for i := range resources {
			id := strings.ToLower(
				fmt.Sprintf(
					"/subscriptions/sub-1/resourceGroups/rg-a/providers/Microsoft.DBforPostgreSQL/flexibleServers/pg-%d",
					i,
				),
			)
			values = append(values, azmetrics.MetricData{
				ResourceID: new(id),
				Values: []azmetrics.Metric{
					metricWithAvg("cpu_percent", ts, float64(i)),
					metricWithAvg("storage_percent", ts, float64(i)/2),
				},
			})
		}
		mx.queryResponse = azmetrics.QueryResourcesResponse{
			MetricResults: azmetrics.MetricResults{
				Values: values,
			},
		}
	}
	collr := newTestCollectorWithMocks(rg, mx)
	collr.Config = testConfig()
	collr.now = func() time.Time { return now }
	require.NoError(b, collr.Init(context.Background()))

	collecttest.BenchmarkPublication(b, collr, func(ctx context.Context) error {
		now = now.Add(time.Minute)
		respond(now)
		return collr.Collect(ctx)
	})
}
