// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/performance"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func TestCollector_HostCPUInstancePerformanceDefaultOff(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	host := firstSortedHost(t, collr)
	collr.scraper = mockHostCPUInstancePerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		hostID:      host.ID,
		series:      testHostCPUInstancePerformanceSeries("0"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	require.Zero(t, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), hostCPUInstanceUtilizationUsageMetric))
}

func TestCollector_HostCPUInstancePerformanceOptInEmitsCharts(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.CollectHostCPUInstancePerformance = true

	require.NoError(t, collr.Init(context.Background()))
	host := firstSortedHost(t, collr)
	collr.scraper = mockHostCPUInstancePerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		hostID:      host.ID,
		series:      testHostCPUInstancePerformanceSeries("0"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	labels := hostCPUInstancePerformanceLabelsMap(collr, host, "0")
	reader := collr.MetricStore().Read(metrix.ReadRaw())
	requireMetricValue(t, reader, hostCPUInstanceUtilizationUsageMetric, labels, 1100)
	requireMetricValue(t, reader, hostCPUInstanceUtilizationUtilizationMetric, labels, 1200)
	requireMetricValue(t, reader, hostCPUInstanceUtilizationCoreMetric, labels, 1300)
	requireMetricValue(t, reader, hostCPUInstanceTimeUsedMetric, labels, 21)
	requireMetricValue(t, reader, hostCPUInstanceTimeIdleMetric, labels, 22)

	createdCharts, createdDims := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))
	chartID := findChartIDByLabelsAndContext(t, createdCharts, hostCPUInstanceUtilizationContext, map[string]string{
		"id":                         host.ID,
		hostCPUInstanceInstanceLabel: "0",
		hostCPUInstanceLabel:         "0",
	})
	require.Contains(t, createdDims[chartID], hostCPUInstanceUtilizationUsageDim)
	require.Contains(t, createdDims[chartID], hostCPUInstanceUtilizationUtilizationDim)
	require.Contains(t, createdDims[chartID], hostCPUInstanceUtilizationCoreDim)
}

func TestCollector_HostCPUInstancePerformanceSelector(t *testing.T) {
	tests := prefixedSelectorCases("cpu", "cpu", "1", "NoSuchCPU")

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _, teardown := prepareVSphereSim(t)
			defer teardown()
			collr.CollectHostCPUInstancePerformance = true
			collr.HostCPUInstancesInclude = tc.include

			require.NoError(t, collr.Init(context.Background()))
			host := firstSortedHost(t, collr)
			collr.scraper = mockHostCPUInstancePerformanceScraper{
				mockScraper: mockScraper{collr.scraper},
				hostID:      host.ID,
				series: append(
					testHostCPUInstancePerformanceSeries("0"),
					testHostCPUInstancePerformanceSeries("1")...,
				),
			}

			require.NotEmpty(t, collectMapForTest(t, collr))

			require.Equal(t, tc.want, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), hostCPUInstanceUtilizationUsageMetric))
		})
	}
}

func TestCollector_Init_ReturnsFalseIfInvalidHostCPUInstanceConfig(t *testing.T) {
	collr := New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "pass"
	collr.CollectHostCPUInstancePerformance = true

	collr.HostCPUInstancesInclude = []string{"["}
	require.ErrorContains(t, collr.Init(context.Background()), "host_cpu_instance_include has invalid pattern")
}

type mockHostCPUInstancePerformanceScraper struct {
	mockScraper
	hostID string
	series []performance.MetricSeries
}

func (s mockHostCPUInstancePerformanceScraper) ScrapeHosts(hosts rs.Hosts) []performance.EntityMetric {
	host := hosts.Get(s.hostID)
	if host == nil {
		return nil
	}
	return []performance.EntityMetric{{
		Entity: host.Ref,
		Value:  append([]performance.MetricSeries(nil), s.series...),
	}}
}

func testHostCPUInstancePerformanceSeries(instance string) []performance.MetricSeries {
	return []performance.MetricSeries{
		{Name: "cpu.usage.average", Instance: instance, Value: []int64{1100}},
		{Name: "cpu.utilization.average", Instance: instance, Value: []int64{1200}},
		{Name: "cpu.coreUtilization.average", Instance: instance, Value: []int64{1300}},
		{Name: "cpu.used.summation", Instance: instance, Value: []int64{21}},
		{Name: "cpu.idle.summation", Instance: instance, Value: []int64{22}},
	}
}

func hostCPUInstancePerformanceLabelsMap(collr *Collector, host *rs.Host, instance string) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range collr.hostCPUInstancePerformanceLabels(host, instance) {
		labels[label.Key] = label.Value
	}
	return labels
}
