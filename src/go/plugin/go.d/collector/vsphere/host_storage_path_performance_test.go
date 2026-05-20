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

func TestCollector_HostStoragePathPerformanceDefaultOff(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	host := firstSortedHost(t, collr)
	collr.scraper = mockHostStoragePathPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		hostID:      host.ID,
		series:      testHostStoragePathPerformanceSeries("vmhba0:C0:T0:L0"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	require.Zero(t, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), hostStoragePathIOReadMetric))
	require.Zero(t, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), hostStoragePathMaxLatencyMetric))
}

func TestCollector_HostStoragePathPerformanceOptInEmitsCharts(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.CollectHostStoragePathPerformance = true

	require.NoError(t, collr.Init(context.Background()))
	host := firstSortedHost(t, collr)
	collr.scraper = mockHostStoragePathPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		hostID:      host.ID,
		series:      testHostStoragePathPerformanceSeries("vmhba0:C0:T0:L0"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	labels := hostStoragePathPerformanceLabelsMap(collr, host, "vmhba0:C0:T0:L0")
	reader := collr.MetricStore().Read(metrix.ReadRaw())
	requireMetricValue(t, reader, hostStoragePathIOReadMetric, labels, 11)
	requireMetricValue(t, reader, hostStoragePathIOWriteMetric, labels, 12)
	requireMetricValue(t, reader, hostStoragePathCommandsIssuedMetric, labels, 21)
	requireMetricValue(t, reader, hostStoragePathCommandsReadMetric, labels, 22)
	requireMetricValue(t, reader, hostStoragePathCommandsWriteMetric, labels, 23)
	requireMetricValue(t, reader, hostStoragePathLatencyReadMetric, labels, 31)
	requireMetricValue(t, reader, hostStoragePathLatencyWriteMetric, labels, 32)
	requireMetricValue(t, reader, hostStoragePathCommandEventsAbortedMetric, labels, 41)
	requireMetricValue(t, reader, hostStoragePathCommandEventsBusResetsMetric, labels, 42)
	requireMetricValue(t, reader, hostStoragePathThroughputMetric, labels, 51)
	requireMetricValue(t, reader, hostStoragePathThroughputContentionMetric, labels, 52)

	aggregateLabels := hostStoragePathAggregatePerformanceLabelsMap(collr, host)
	requireMetricValue(t, reader, hostStoragePathMaxLatencyMetric, aggregateLabels, 61)

	createdCharts, createdDims := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))
	chartID := findChartIDByLabelsAndContext(t, createdCharts, hostStoragePathIOContext, map[string]string{
		"id":                         host.ID,
		hostStoragePathInstanceLabel: "vmhba0:C0:T0:L0",
		hostStoragePathLabel:         "vmhba0:C0:T0:L0",
	})
	require.Contains(t, createdDims[chartID], hostStoragePathIOReadDim)
	require.Contains(t, createdDims[chartID], hostStoragePathIOWriteDim)

	aggregateChartID := findChartIDByLabelsAndContext(t, createdCharts, hostStoragePathMaxLatencyContext, map[string]string{
		"id": host.ID,
	})
	require.Contains(t, createdDims[aggregateChartID], hostStoragePathMaxLatencyDim)
}

func TestCollector_HostStoragePathPerformanceSelectorAndCap(t *testing.T) {
	tests := prefixedSelectorCapCases("path", "path", "vmhba1:C0:T0:L0", "NoSuchPath")

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _, teardown := prepareVSphereSim(t)
			defer teardown()
			collr.CollectHostStoragePathPerformance = true
			collr.HostStoragePathsInclude = tc.include
			collr.MaxHostStoragePaths = tc.max

			require.NoError(t, collr.Init(context.Background()))
			host := firstSortedHost(t, collr)
			collr.scraper = mockHostStoragePathPerformanceScraper{
				mockScraper: mockScraper{collr.scraper},
				hostID:      host.ID,
				series: append(
					testHostStoragePathPerformanceSeries("vmhba0:C0:T0:L0"),
					testHostStoragePathPerformanceSeries("vmhba1:C0:T0:L0")...,
				),
			}

			require.NotEmpty(t, collectMapForTest(t, collr))

			require.Equal(t, tc.want, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), hostStoragePathIOReadMetric))
		})
	}
}

func TestCollector_Init_ReturnsFalseIfInvalidHostStoragePathConfig(t *testing.T) {
	collr := New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "pass"
	collr.CollectHostStoragePathPerformance = true
	collr.MaxHostStoragePaths = -1

	require.ErrorContains(t, collr.Init(context.Background()), "max_host_storage_paths must be greater than zero")

	collr.MaxHostStoragePaths = 1
	collr.HostStoragePathsInclude = []string{"["}
	require.ErrorContains(t, collr.Init(context.Background()), "host_storage_path_include has invalid pattern")
}

type mockHostStoragePathPerformanceScraper struct {
	mockScraper
	hostID string
	series []performance.MetricSeries
}

func (s mockHostStoragePathPerformanceScraper) ScrapeHosts(hosts rs.Hosts) []performance.EntityMetric {
	host := hosts.Get(s.hostID)
	if host == nil {
		return nil
	}
	return []performance.EntityMetric{{
		Entity: host.Ref,
		Value:  append([]performance.MetricSeries(nil), s.series...),
	}}
}

func testHostStoragePathPerformanceSeries(instance string) []performance.MetricSeries {
	return []performance.MetricSeries{
		{Name: "storagePath.read.average", Instance: instance, Value: []int64{11}},
		{Name: "storagePath.write.average", Instance: instance, Value: []int64{12}},
		{Name: "storagePath.commandsAveraged.average", Instance: instance, Value: []int64{21}},
		{Name: "storagePath.numberReadAveraged.average", Instance: instance, Value: []int64{22}},
		{Name: "storagePath.numberWriteAveraged.average", Instance: instance, Value: []int64{23}},
		{Name: "storagePath.totalReadLatency.average", Instance: instance, Value: []int64{31}},
		{Name: "storagePath.totalWriteLatency.average", Instance: instance, Value: []int64{32}},
		{Name: "storagePath.commandsAborted.summation", Instance: instance, Value: []int64{41}},
		{Name: "storagePath.busResets.summation", Instance: instance, Value: []int64{42}},
		{Name: "storagePath.throughput.usage.average", Instance: instance, Value: []int64{51}},
		{Name: "storagePath.throughput.cont.average", Instance: instance, Value: []int64{52}},
		{Name: "storagePath.maxTotalLatency.latest", Value: []int64{61}},
	}
}

func hostStoragePathPerformanceLabelsMap(collr *Collector, host *rs.Host, instance string) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range collr.hostStoragePathPerformanceLabels(host, instance) {
		labels[label.Key] = label.Value
	}
	return labels
}

func hostStoragePathAggregatePerformanceLabelsMap(collr *Collector, host *rs.Host) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range collr.hostStoragePathAggregatePerformanceLabels(host) {
		labels[label.Key] = label.Value
	}
	return labels
}
