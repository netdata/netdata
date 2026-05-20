// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/performance"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func TestCollector_HostNICPerformanceDefaultOff(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	host := firstSortedHost(t, collr)
	collr.scraper = mockHostNICPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		hostID:      host.ID,
		series:      testHostNICPerformanceSeries("vmnic0"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	require.Zero(t, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), hostNetInterfaceTrafficRxMetric))
}

func TestCollector_HostNICPerformanceOptInEmitsCharts(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.CollectHostNICPerformance = true

	require.NoError(t, collr.Init(context.Background()))
	host := firstSortedHost(t, collr)
	collr.scraper = mockHostNICPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		hostID:      host.ID,
		series: append(
			[]performance.MetricSeries{{Name: "net.bytesRx.average", Value: []int64{99}}},
			testHostNICPerformanceSeries("vmnic0")...,
		),
	}

	mx := collectMapForTest(t, collr)
	require.Equal(t, int64(99), mx[host.ID+"_net.bytesRx.average"])

	labels := hostNICPerformanceLabelsMap(collr, host, "vmnic0")
	reader := collr.MetricStore().Read(metrix.ReadRaw())
	requireMetricValue(t, reader, hostNetInterfaceTrafficRxMetric, labels, 11)
	requireMetricValue(t, reader, hostNetInterfaceTrafficTxMetric, labels, 12)
	requireMetricValue(t, reader, hostNetInterfacePacketsRxMetric, labels, 21)
	requireMetricValue(t, reader, hostNetInterfacePacketsTxMetric, labels, 22)
	requireMetricValue(t, reader, hostNetInterfaceDropsRxMetric, labels, 31)
	requireMetricValue(t, reader, hostNetInterfaceDropsTxMetric, labels, 32)
	requireMetricValue(t, reader, hostNetInterfaceErrorsRxMetric, labels, 41)
	requireMetricValue(t, reader, hostNetInterfaceErrorsTxMetric, labels, 42)
	requireMetricValue(t, reader, hostNetInterfaceBroadcastPacketsRxMetric, labels, 51)
	requireMetricValue(t, reader, hostNetInterfaceBroadcastPacketsTxMetric, labels, 52)
	requireMetricValue(t, reader, hostNetInterfaceMulticastPacketsRxMetric, labels, 61)
	requireMetricValue(t, reader, hostNetInterfaceMulticastPacketsTxMetric, labels, 62)
	requireMetricValue(t, reader, hostNetInterfaceUnknownProtocolFramesMetric, labels, 71)
	requireMetricValue(t, reader, hostNetInterfaceUsageMetric, labels, 81)

	createdCharts, createdDims := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))
	chartID := findChartIDByLabelsAndContext(t, createdCharts, hostNetInterfaceTrafficContext, map[string]string{
		"id":                 host.ID,
		hostNICInstanceLabel: "vmnic0",
		hostNICLabel:         "vmnic0",
	})
	require.Contains(t, createdDims[chartID], hostNetInterfaceTrafficRxDim)
	require.Contains(t, createdDims[chartID], hostNetInterfaceTrafficTxDim)
}

func TestCollector_HostNICPerformanceSelector(t *testing.T) {
	tests := prefixedSelectorCases("interface", "interface", "vmnic1", "NoSuchInterface")

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _, teardown := prepareVSphereSim(t)
			defer teardown()
			collr.CollectHostNICPerformance = true
			collr.HostNICsInclude = tc.include

			require.NoError(t, collr.Init(context.Background()))
			host := firstSortedHost(t, collr)
			collr.scraper = mockHostNICPerformanceScraper{
				mockScraper: mockScraper{collr.scraper},
				hostID:      host.ID,
				series: append(
					testHostNICPerformanceSeries("vmnic0"),
					testHostNICPerformanceSeries("vmnic1")...,
				),
			}

			require.NotEmpty(t, collectMapForTest(t, collr))

			require.Equal(t, tc.want, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), hostNetInterfaceTrafficRxMetric))
		})
	}
}

func TestCollector_Init_ReturnsFalseIfInvalidHostNICConfig(t *testing.T) {
	collr := New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "pass"
	collr.CollectHostNICPerformance = true

	collr.HostNICsInclude = []string{"["}
	require.ErrorContains(t, collr.Init(context.Background()), "host_nic_include has invalid pattern")
}

type mockHostNICPerformanceScraper struct {
	mockScraper
	hostID string
	series []performance.MetricSeries
}

func (s mockHostNICPerformanceScraper) ScrapeHosts(hosts rs.Hosts) []performance.EntityMetric {
	host := hosts.Get(s.hostID)
	if host == nil {
		return nil
	}
	return []performance.EntityMetric{{
		Entity: host.Ref,
		Value:  append([]performance.MetricSeries(nil), s.series...),
	}}
}

func testHostNICPerformanceSeries(instance string) []performance.MetricSeries {
	return []performance.MetricSeries{
		{Name: "net.bytesRx.average", Instance: instance, Value: []int64{11}},
		{Name: "net.bytesTx.average", Instance: instance, Value: []int64{12}},
		{Name: "net.packetsRx.summation", Instance: instance, Value: []int64{21}},
		{Name: "net.packetsTx.summation", Instance: instance, Value: []int64{22}},
		{Name: "net.droppedRx.summation", Instance: instance, Value: []int64{31}},
		{Name: "net.droppedTx.summation", Instance: instance, Value: []int64{32}},
		{Name: "net.errorsRx.summation", Instance: instance, Value: []int64{41}},
		{Name: "net.errorsTx.summation", Instance: instance, Value: []int64{42}},
		{Name: "net.broadcastRx.summation", Instance: instance, Value: []int64{51}},
		{Name: "net.broadcastTx.summation", Instance: instance, Value: []int64{52}},
		{Name: "net.multicastRx.summation", Instance: instance, Value: []int64{61}},
		{Name: "net.multicastTx.summation", Instance: instance, Value: []int64{62}},
		{Name: "net.unknownProtos.summation", Instance: instance, Value: []int64{71}},
		{Name: "net.usage.average", Instance: instance, Value: []int64{81}},
	}
}

func firstSortedHost(t *testing.T, collr *Collector) *rs.Host {
	t.Helper()

	hosts := make([]*rs.Host, 0, len(collr.resources.Hosts))
	for _, host := range collr.resources.Hosts {
		hosts = append(hosts, host)
	}
	sort.Slice(hosts, func(i, j int) bool {
		return hosts[i].ID < hosts[j].ID
	})
	require.NotEmpty(t, hosts)
	return hosts[0]
}

func hostNICPerformanceLabelsMap(collr *Collector, host *rs.Host, instance string) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range collr.hostNICPerformanceLabels(host, instance) {
		labels[label.Key] = label.Value
	}
	return labels
}
