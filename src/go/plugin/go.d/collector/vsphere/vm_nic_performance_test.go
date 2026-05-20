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

func TestCollector_VMNICPerformanceDefaultOff(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	vm := firstSortedVM(t, collr)
	collr.scraper = mockVMNICPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		vmID:        vm.ID,
		series:      testVMNICPerformanceSeries("4000"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	require.Zero(t, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), vmNetInterfaceTrafficRxMetric))
}

func TestCollector_VMNICPerformanceOptInEmitsCharts(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.CollectVMNICPerformance = true

	require.NoError(t, collr.Init(context.Background()))
	vm := firstSortedVM(t, collr)
	collr.scraper = mockVMNICPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		vmID:        vm.ID,
		series:      testVMNICPerformanceSeries("4000"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	labels := vmNICPerformanceLabelsMap(collr, vm, "4000")
	reader := collr.MetricStore().Read(metrix.ReadRaw())
	requireMetricValue(t, reader, vmNetInterfaceTrafficRxMetric, labels, 11)
	requireMetricValue(t, reader, vmNetInterfaceTrafficTxMetric, labels, 12)
	requireMetricValue(t, reader, vmNetInterfacePacketsRxMetric, labels, 21)
	requireMetricValue(t, reader, vmNetInterfacePacketsTxMetric, labels, 22)
	requireMetricValue(t, reader, vmNetInterfaceDropsRxMetric, labels, 31)
	requireMetricValue(t, reader, vmNetInterfaceDropsTxMetric, labels, 32)
	requireMetricValue(t, reader, vmNetInterfaceBroadcastPacketsRxMetric, labels, 41)
	requireMetricValue(t, reader, vmNetInterfaceBroadcastPacketsTxMetric, labels, 42)
	requireMetricValue(t, reader, vmNetInterfaceMulticastPacketsRxMetric, labels, 51)
	requireMetricValue(t, reader, vmNetInterfaceMulticastPacketsTxMetric, labels, 52)

	createdCharts, createdDims := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))
	chartID := findChartIDByLabelsAndContext(t, createdCharts, vmNetInterfaceTrafficContext, map[string]string{
		"id":               vm.ID,
		vmNICInstanceLabel: "4000",
		vmNICLabel:         "4000",
	})
	require.Contains(t, createdDims[chartID], vmNetInterfaceTrafficRxDim)
	require.Contains(t, createdDims[chartID], vmNetInterfaceTrafficTxDim)
}

func TestCollector_VMNICPerformanceSelectorAndCap(t *testing.T) {
	tests := prefixedSelectorCapCases("interface", "interface", "4001", "NoSuchInterface")

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _, teardown := prepareVSphereSim(t)
			defer teardown()
			collr.CollectVMNICPerformance = true
			collr.VMNICsInclude = tc.include
			collr.MaxVMNICs = tc.max

			require.NoError(t, collr.Init(context.Background()))
			vm := firstSortedVM(t, collr)
			collr.scraper = mockVMNICPerformanceScraper{
				mockScraper: mockScraper{collr.scraper},
				vmID:        vm.ID,
				series: append(
					testVMNICPerformanceSeries("4000"),
					testVMNICPerformanceSeries("4001")...,
				),
			}

			require.NotEmpty(t, collectMapForTest(t, collr))

			require.Equal(t, tc.want, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), vmNetInterfaceTrafficRxMetric))
		})
	}
}

func TestCollector_Init_ReturnsFalseIfInvalidVMNICConfig(t *testing.T) {
	collr := New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "pass"
	collr.CollectVMNICPerformance = true
	collr.MaxVMNICs = -1

	require.ErrorContains(t, collr.Init(context.Background()), "max_vm_nics must be greater than zero")

	collr.MaxVMNICs = 1
	collr.VMNICsInclude = []string{"["}
	require.ErrorContains(t, collr.Init(context.Background()), "vm_nic_include has invalid pattern")
}

type mockVMNICPerformanceScraper struct {
	mockScraper
	vmID   string
	series []performance.MetricSeries
}

func (s mockVMNICPerformanceScraper) ScrapeVMs(vms rs.VMs) []performance.EntityMetric {
	vm := vms.Get(s.vmID)
	if vm == nil {
		return nil
	}
	return []performance.EntityMetric{{
		Entity: vm.Ref,
		Value:  append([]performance.MetricSeries(nil), s.series...),
	}}
}

func testVMNICPerformanceSeries(instance string) []performance.MetricSeries {
	return []performance.MetricSeries{
		{Name: "net.bytesRx.average", Instance: instance, Value: []int64{11}},
		{Name: "net.bytesTx.average", Instance: instance, Value: []int64{12}},
		{Name: "net.packetsRx.summation", Instance: instance, Value: []int64{21}},
		{Name: "net.packetsTx.summation", Instance: instance, Value: []int64{22}},
		{Name: "net.droppedRx.summation", Instance: instance, Value: []int64{31}},
		{Name: "net.droppedTx.summation", Instance: instance, Value: []int64{32}},
		{Name: "net.broadcastRx.summation", Instance: instance, Value: []int64{41}},
		{Name: "net.broadcastTx.summation", Instance: instance, Value: []int64{42}},
		{Name: "net.multicastRx.summation", Instance: instance, Value: []int64{51}},
		{Name: "net.multicastTx.summation", Instance: instance, Value: []int64{52}},
	}
}

func vmNICPerformanceLabelsMap(collr *Collector, vm *rs.VM, instance string) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range collr.vmNICPerformanceLabels(vm, instance) {
		labels[label.Key] = label.Value
	}
	return labels
}
