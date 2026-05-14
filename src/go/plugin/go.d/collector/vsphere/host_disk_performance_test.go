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

func TestCollector_HostDiskPerformanceDefaultOff(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	host := firstSortedHost(t, collr)
	collr.scraper = mockHostDiskPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		hostID:      host.ID,
		series:      testHostDiskPerformanceSeries("naa.123"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	require.Zero(t, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), hostDiskDeviceIOReadMetric))
}

func TestCollector_HostDiskPerformanceOptInEmitsCharts(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.CollectHostDiskPerformance = true

	require.NoError(t, collr.Init(context.Background()))
	host := firstSortedHost(t, collr)
	collr.scraper = mockHostDiskPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		hostID:      host.ID,
		series: append(
			[]performance.MetricSeries{{Name: "disk.read.average", Value: []int64{99}}},
			testHostDiskPerformanceSeries("naa.123")...,
		),
	}

	mx := collectMapForTest(t, collr)
	require.Equal(t, int64(99), mx[host.ID+"_disk.read.average"])

	labels := hostDiskPerformanceLabelsMap(collr, host, "naa.123")
	reader := collr.MetricStore().Read(metrix.ReadRaw())
	requireMetricValue(t, reader, hostDiskDeviceIOReadMetric, labels, 11)
	requireMetricValue(t, reader, hostDiskDeviceIOWriteMetric, labels, 12)
	requireMetricValue(t, reader, hostDiskDeviceIOPSReadMetric, labels, 21)
	requireMetricValue(t, reader, hostDiskDeviceIOPSWriteMetric, labels, 22)
	requireMetricValue(t, reader, hostDiskDeviceRequestsReadMetric, labels, 31)
	requireMetricValue(t, reader, hostDiskDeviceRequestsWriteMetric, labels, 32)
	requireMetricValue(t, reader, hostDiskDeviceLatencyTotalMetric, labels, 41)
	requireMetricValue(t, reader, hostDiskDeviceLatencyReadMetric, labels, 42)
	requireMetricValue(t, reader, hostDiskDeviceLatencyWriteMetric, labels, 43)
	requireMetricValue(t, reader, hostDiskDeviceLatencyDeviceMetric, labels, 51)
	requireMetricValue(t, reader, hostDiskDeviceLatencyKernelMetric, labels, 52)
	requireMetricValue(t, reader, hostDiskDeviceLatencyQueueMetric, labels, 53)
	requireMetricValue(t, reader, hostDiskDeviceReadLatencyDeviceMetric, labels, 61)
	requireMetricValue(t, reader, hostDiskDeviceReadLatencyKernelMetric, labels, 62)
	requireMetricValue(t, reader, hostDiskDeviceReadLatencyQueueMetric, labels, 63)
	requireMetricValue(t, reader, hostDiskDeviceWriteLatencyDeviceMetric, labels, 71)
	requireMetricValue(t, reader, hostDiskDeviceWriteLatencyKernelMetric, labels, 72)
	requireMetricValue(t, reader, hostDiskDeviceWriteLatencyQueueMetric, labels, 73)
	requireMetricValue(t, reader, hostDiskDeviceCommandsMetric, labels, 81)
	requireMetricValue(t, reader, hostDiskDeviceCommandEventsIssuedMetric, labels, 91)
	requireMetricValue(t, reader, hostDiskDeviceCommandEventsAbortedMetric, labels, 92)
	requireMetricValue(t, reader, hostDiskDeviceCommandEventsBusResetsMetric, labels, 93)
	requireMetricValue(t, reader, hostDiskDeviceQueueDepthMetric, labels, 101)
	requireMetricValue(t, reader, hostDiskDeviceSCSIReservationConflictsMetric, labels, 111)
	requireMetricValue(t, reader, hostDiskDeviceSCSIReservationConflictsPctMetric, labels, 112)

	createdCharts, createdDims := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))
	chartID := findChartIDByLabelsAndContext(t, createdCharts, hostDiskDeviceIOContext, map[string]string{
		"id":                  host.ID,
		hostDiskInstanceLabel: "naa.123",
		hostDiskLabel:         "naa.123",
	})
	require.Contains(t, createdDims[chartID], hostDiskDeviceIOReadDim)
	require.Contains(t, createdDims[chartID], hostDiskDeviceIOWriteDim)
}

func TestCollector_HostDiskPerformanceSelectorAndCap(t *testing.T) {
	tests := instanceSelectorCapCases("disk", "naa.124", "NoSuchDisk")

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _, teardown := prepareVSphereSim(t)
			defer teardown()
			collr.CollectHostDiskPerformance = true
			collr.HostDisksInclude = tc.include
			collr.MaxHostDisks = tc.max

			require.NoError(t, collr.Init(context.Background()))
			host := firstSortedHost(t, collr)
			collr.scraper = mockHostDiskPerformanceScraper{
				mockScraper: mockScraper{collr.scraper},
				hostID:      host.ID,
				series: append(
					testHostDiskPerformanceSeries("naa.123"),
					testHostDiskPerformanceSeries("naa.124")...,
				),
			}

			require.NotEmpty(t, collectMapForTest(t, collr))

			require.Equal(t, tc.want, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), hostDiskDeviceIOReadMetric))
		})
	}
}

func TestCollector_HostDiskPerformanceUsesESXIVnodeScope(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.CollectHostDiskPerformance = true
	collr.ESXIVnodes = true

	require.NoError(t, collr.Init(context.Background()))
	host := firstSortedHost(t, collr)
	collr.scraper = mockHostDiskPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		hostID:      host.ID,
		series:      testHostDiskPerformanceSeries("naa.123"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	labels := hostDiskPerformanceLabelsMap(collr, host, "naa.123")
	_, ok := collr.MetricStore().Read(metrix.ReadRaw()).Value(hostDiskDeviceIOReadMetric, labels)
	require.False(t, ok, "host disk performance metric should move out of the default scope when esxi_vnodes is enabled")

	scope := collr.esxiHostScope(host)
	_, ok = collr.MetricStore().Read(metrix.ReadRaw(), metrix.ReadHostScope(scope.ScopeKey)).Value(hostDiskDeviceIOReadMetric, labels)
	require.True(t, ok, "host disk performance metric should be present in the ESXi host scope")
}

func TestCollector_Init_ReturnsFalseIfInvalidHostDiskConfig(t *testing.T) {
	collr := New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "pass"
	collr.CollectHostDiskPerformance = true
	collr.MaxHostDisks = -1

	require.ErrorContains(t, collr.Init(context.Background()), "max_host_disks must be greater than zero")

	collr.MaxHostDisks = 1
	collr.HostDisksInclude = []string{"["}
	require.ErrorContains(t, collr.Init(context.Background()), "host_disk_include has invalid pattern")
}

type mockHostDiskPerformanceScraper struct {
	mockScraper
	hostID string
	series []performance.MetricSeries
}

func (s mockHostDiskPerformanceScraper) ScrapeHosts(hosts rs.Hosts) []performance.EntityMetric {
	host := hosts.Get(s.hostID)
	if host == nil {
		return nil
	}
	return []performance.EntityMetric{{
		Entity: host.Ref,
		Value:  append([]performance.MetricSeries(nil), s.series...),
	}}
}

func testHostDiskPerformanceSeries(instance string) []performance.MetricSeries {
	return []performance.MetricSeries{
		{Name: "disk.read.average", Instance: instance, Value: []int64{11}},
		{Name: "disk.write.average", Instance: instance, Value: []int64{12}},
		{Name: "disk.numberReadAveraged.average", Instance: instance, Value: []int64{21}},
		{Name: "disk.numberWriteAveraged.average", Instance: instance, Value: []int64{22}},
		{Name: "disk.numberRead.summation", Instance: instance, Value: []int64{31}},
		{Name: "disk.numberWrite.summation", Instance: instance, Value: []int64{32}},
		{Name: "disk.totalLatency.average", Instance: instance, Value: []int64{41}},
		{Name: "disk.totalReadLatency.average", Instance: instance, Value: []int64{42}},
		{Name: "disk.totalWriteLatency.average", Instance: instance, Value: []int64{43}},
		{Name: "disk.deviceLatency.average", Instance: instance, Value: []int64{51}},
		{Name: "disk.kernelLatency.average", Instance: instance, Value: []int64{52}},
		{Name: "disk.queueLatency.average", Instance: instance, Value: []int64{53}},
		{Name: "disk.deviceReadLatency.average", Instance: instance, Value: []int64{61}},
		{Name: "disk.kernelReadLatency.average", Instance: instance, Value: []int64{62}},
		{Name: "disk.queueReadLatency.average", Instance: instance, Value: []int64{63}},
		{Name: "disk.deviceWriteLatency.average", Instance: instance, Value: []int64{71}},
		{Name: "disk.kernelWriteLatency.average", Instance: instance, Value: []int64{72}},
		{Name: "disk.queueWriteLatency.average", Instance: instance, Value: []int64{73}},
		{Name: "disk.commandsAveraged.average", Instance: instance, Value: []int64{81}},
		{Name: "disk.commands.summation", Instance: instance, Value: []int64{91}},
		{Name: "disk.commandsAborted.summation", Instance: instance, Value: []int64{92}},
		{Name: "disk.busResets.summation", Instance: instance, Value: []int64{93}},
		{Name: "disk.maxQueueDepth.average", Instance: instance, Value: []int64{101}},
		{Name: "disk.scsiReservationConflicts.summation", Instance: instance, Value: []int64{111}},
		{Name: "disk.scsiReservationCnflctsPct.average", Instance: instance, Value: []int64{112}},
	}
}

func hostDiskPerformanceLabelsMap(collr *Collector, host *rs.Host, instance string) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range collr.hostDiskPerformanceLabels(host, instance) {
		labels[label.Key] = label.Value
	}
	return labels
}
