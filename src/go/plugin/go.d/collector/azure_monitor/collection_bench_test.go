// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/monitor/query/azmetrics"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/require"
)

// BenchmarkResourceCollection covers collection and store publication for 50 resources,
// with stable metadata or a metadata change on every refresh. Azure clients use synthetic responses.
func BenchmarkResourceCollection(b *testing.B) {
	for _, profile := range []string{"app_service", "virtual_machines"} {
		for _, workload := range []string{"steady", "metadata_churn"} {
			b.Run(profile+"/"+workload, func(b *testing.B) {
				benchmarkResourceCollection(b, profile, workload == "metadata_churn")
			})
		}
	}
}

func benchmarkResourceCollection(b *testing.B, profile string, churn bool) {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	rg, mx := &mockResourceGraph{}, &mockMetricsClient{}
	for i := range 50 {
		name := fmt.Sprintf("resource-%d", i)
		typ, metricName := "Microsoft.Web/sites", "Requests"
		point := azmetrics.MetricValue{TimeStamp: &now, Total: new(10.0)}
		row := testResource(typ, name, map[string]any{"nd_properties_serverFarmId": "/plans/plan-a"})
		if profile == "virtual_machines" {
			typ, metricName = "Microsoft.Compute/virtualMachines", "Percentage CPU"
			point = azmetrics.MetricValue{TimeStamp: &now, Average: new(25.0)}
			row = testVirtualMachine(name, "Standard_B2s", "Linux", "PowerState/running")
		}
		rg.resources = append(rg.resources, row)
		mx.queryResponse.Values = append(mx.queryResponse.Values, azmetrics.MetricData{
			ResourceID: new(fmt.Sprintf("/subscriptions/sub-1/resourceGroups/rg-a/providers/%s/%s", typ, name)),
			Values:     []azmetrics.Metric{{Name: &azmetrics.LocalizableString{Value: &metricName}, TimeSeries: []azmetrics.TimeSeriesElement{{Data: []azmetrics.MetricValue{point}}}}},
		})
	}
	c := newTestCollectorWithMocks(rg, mx)
	c.Config = testConfig()
	if churn {
		c.Config.Discovery.RefreshEvery = 60
	}
	c.Config.Profiles.ModeExact = testProfilesModeEntries(profile)
	c.now = func() time.Time { return now }
	require.NoError(b, c.Init(context.Background()))
	require.NoError(b, c.Check(context.Background()))
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		now = now.Add(time.Minute)
		if churn {
			value := fmt.Sprintf("metadata-%d", i)
			for _, row := range rg.resources {
				if profile == "app_service" {
					row["nd_properties_serverFarmId"] = value
				} else {
					row["nd_properties_hardwareProfile_vmSize"] = value
				}
			}
		}
		_, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
		if err != nil {
			b.Fatal(err)
		}
	}
}
