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

func TestCollector_HostStorageAdapterPerformanceDefaultOff(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	host := firstSortedHost(t, collr)
	collr.scraper = mockHostStorageAdapterPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		hostID:      host.ID,
		series:      testHostStorageAdapterPerformanceSeries("vmhba0"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	require.Zero(t, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), hostStorageAdapterIOReadMetric))
	require.Zero(t, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), hostStorageAdapterMaxLatencyMetric))
}

func TestCollector_HostStorageAdapterPerformanceOptInEmitsCharts(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.CollectHostStorageAdapterPerformance = true

	require.NoError(t, collr.Init(context.Background()))
	host := firstSortedHost(t, collr)
	collr.scraper = mockHostStorageAdapterPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		hostID:      host.ID,
		series:      testHostStorageAdapterPerformanceSeries("vmhba0"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	labels := hostStorageAdapterPerformanceLabelsMap(collr, host, "vmhba0")
	reader := collr.MetricStore().Read(metrix.ReadRaw())
	requireMetricValue(t, reader, hostStorageAdapterIOReadMetric, labels, 11)
	requireMetricValue(t, reader, hostStorageAdapterIOWriteMetric, labels, 12)
	requireMetricValue(t, reader, hostStorageAdapterCommandsIssuedMetric, labels, 21)
	requireMetricValue(t, reader, hostStorageAdapterCommandsReadMetric, labels, 22)
	requireMetricValue(t, reader, hostStorageAdapterCommandsWriteMetric, labels, 23)
	requireMetricValue(t, reader, hostStorageAdapterLatencyReadMetric, labels, 31)
	requireMetricValue(t, reader, hostStorageAdapterLatencyWriteMetric, labels, 32)
	requireMetricValue(t, reader, hostStorageAdapterLatencyQueueMetric, labels, 33)
	requireMetricValue(t, reader, hostStorageAdapterQueueOutstandingMetric, labels, 41)
	requireMetricValue(t, reader, hostStorageAdapterQueueQueuedMetric, labels, 42)
	requireMetricValue(t, reader, hostStorageAdapterQueueDepthMetric, labels, 43)
	requireMetricValue(t, reader, hostStorageAdapterOutstandingIOPctMetric, labels, 51)
	requireMetricValue(t, reader, hostStorageAdapterThroughputMetric, labels, 61)
	requireMetricValue(t, reader, hostStorageAdapterThroughputContentionMetric, labels, 62)

	aggregateLabels := hostStorageAdapterAggregatePerformanceLabelsMap(collr, host)
	requireMetricValue(t, reader, hostStorageAdapterMaxLatencyMetric, aggregateLabels, 71)

	createdCharts, createdDims := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))
	chartID := findChartIDByLabelsAndContext(t, createdCharts, hostStorageAdapterIOContext, map[string]string{
		"id":                            host.ID,
		hostStorageAdapterInstanceLabel: "vmhba0",
		hostStorageAdapterLabel:         "vmhba0",
	})
	require.Contains(t, createdDims[chartID], hostStorageAdapterIOReadDim)
	require.Contains(t, createdDims[chartID], hostStorageAdapterIOWriteDim)

	aggregateChartID := findChartIDByLabelsAndContext(t, createdCharts, hostStorageAdapterMaxLatencyContext, map[string]string{
		"id": host.ID,
	})
	require.Contains(t, createdDims[aggregateChartID], hostStorageAdapterMaxLatencyDim)
}

func TestCollector_HostStorageAdapterPerformanceSelector(t *testing.T) {
	tests := prefixedSelectorCases("adapter", "adapter", "vmhba1", "NoSuchAdapter")

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _, teardown := prepareVSphereSim(t)
			defer teardown()
			collr.CollectHostStorageAdapterPerformance = true
			collr.HostStorageAdaptersInclude = tc.include

			require.NoError(t, collr.Init(context.Background()))
			host := firstSortedHost(t, collr)
			collr.scraper = mockHostStorageAdapterPerformanceScraper{
				mockScraper: mockScraper{collr.scraper},
				hostID:      host.ID,
				series: append(
					testHostStorageAdapterPerformanceSeries("vmhba0"),
					testHostStorageAdapterPerformanceSeries("vmhba1")...,
				),
			}

			require.NotEmpty(t, collectMapForTest(t, collr))

			require.Equal(t, tc.want, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), hostStorageAdapterIOReadMetric))
		})
	}
}

func TestCollector_Init_ReturnsFalseIfInvalidHostStorageAdapterConfig(t *testing.T) {
	collr := New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "pass"
	collr.CollectHostStorageAdapterPerformance = true

	collr.HostStorageAdaptersInclude = []string{"["}
	require.ErrorContains(t, collr.Init(context.Background()), "host_storage_adapter_include has invalid pattern")
}

type mockHostStorageAdapterPerformanceScraper struct {
	mockScraper
	hostID string
	series []performance.MetricSeries
}

func (s mockHostStorageAdapterPerformanceScraper) ScrapeHosts(hosts rs.Hosts) []performance.EntityMetric {
	host := hosts.Get(s.hostID)
	if host == nil {
		return nil
	}
	return []performance.EntityMetric{{
		Entity: host.Ref,
		Value:  append([]performance.MetricSeries(nil), s.series...),
	}}
}

func testHostStorageAdapterPerformanceSeries(instance string) []performance.MetricSeries {
	return []performance.MetricSeries{
		{Name: "storageAdapter.read.average", Instance: instance, Value: []int64{11}},
		{Name: "storageAdapter.write.average", Instance: instance, Value: []int64{12}},
		{Name: "storageAdapter.commandsAveraged.average", Instance: instance, Value: []int64{21}},
		{Name: "storageAdapter.numberReadAveraged.average", Instance: instance, Value: []int64{22}},
		{Name: "storageAdapter.numberWriteAveraged.average", Instance: instance, Value: []int64{23}},
		{Name: "storageAdapter.totalReadLatency.average", Instance: instance, Value: []int64{31}},
		{Name: "storageAdapter.totalWriteLatency.average", Instance: instance, Value: []int64{32}},
		{Name: "storageAdapter.queueLatency.average", Instance: instance, Value: []int64{33}},
		{Name: "storageAdapter.outstandingIOs.average", Instance: instance, Value: []int64{41}},
		{Name: "storageAdapter.queued.average", Instance: instance, Value: []int64{42}},
		{Name: "storageAdapter.queueDepth.average", Instance: instance, Value: []int64{43}},
		{Name: "storageAdapter.OIOsPct.average", Instance: instance, Value: []int64{51}},
		{Name: "storageAdapter.throughput.usag.average", Instance: instance, Value: []int64{61}},
		{Name: "storageAdapter.throughput.cont.average", Instance: instance, Value: []int64{62}},
		{Name: "storageAdapter.maxTotalLatency.latest", Value: []int64{71}},
	}
}

func hostStorageAdapterPerformanceLabelsMap(collr *Collector, host *rs.Host, instance string) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range collr.hostStorageAdapterPerformanceLabels(host, instance) {
		labels[label.Key] = label.Value
	}
	return labels
}

func hostStorageAdapterAggregatePerformanceLabelsMap(collr *Collector, host *rs.Host) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range collr.hostStorageAdapterAggregatePerformanceLabels(host) {
		labels[label.Key] = label.Value
	}
	return labels
}
