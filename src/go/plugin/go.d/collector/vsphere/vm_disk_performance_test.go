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

func TestCollector_VMDiskPerformanceDefaultOff(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	vm := firstSortedVM(t, collr)
	collr.scraper = mockVMDiskPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		vmID:        vm.ID,
		series:      testVMDiskPerformanceSeries("scsi0:0"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	require.Zero(t, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), vmDiskDeviceIOReadMetric))
}

func TestCollector_VMDiskPerformanceOptInEmitsCharts(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.CollectVMDiskPerformance = true

	require.NoError(t, collr.Init(context.Background()))
	vm := firstSortedVM(t, collr)
	collr.scraper = mockVMDiskPerformanceScraper{
		mockScraper: mockScraper{collr.scraper},
		vmID:        vm.ID,
		series:      testVMDiskPerformanceSeries("scsi0:0"),
	}

	require.NotEmpty(t, collectMapForTest(t, collr))

	labels := vmDiskPerformanceLabelsMap(collr, vm, "scsi0:0")
	reader := collr.MetricStore().Read(metrix.ReadRaw())
	requireMetricValue(t, reader, vmDiskDeviceIOReadMetric, labels, 11)
	requireMetricValue(t, reader, vmDiskDeviceIOWriteMetric, labels, 12)
	requireMetricValue(t, reader, vmDiskDeviceIOPSReadMetric, labels, 21)
	requireMetricValue(t, reader, vmDiskDeviceIOPSWriteMetric, labels, 22)
	requireMetricValue(t, reader, vmDiskDeviceLatencyReadMetric, labels, 31)
	requireMetricValue(t, reader, vmDiskDeviceLatencyWriteMetric, labels, 32)
	requireMetricValue(t, reader, vmDiskDeviceOIOReadMetric, labels, 41)
	requireMetricValue(t, reader, vmDiskDeviceOIOWriteMetric, labels, 42)

	createdCharts, createdDims := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))
	chartID := findChartIDByLabelsAndContext(t, createdCharts, vmDiskDeviceIOContext, map[string]string{
		"id":                   vm.ID,
		vmDiskInstanceLabel:    "scsi0:0",
		vmDiskDisplayNameLabel: "scsi0:0",
	})
	require.Contains(t, createdDims[chartID], vmDiskDeviceIOReadDim)
	require.Contains(t, createdDims[chartID], vmDiskDeviceIOWriteDim)
}

func TestCollector_VMDiskPerformanceSelectorAndCap(t *testing.T) {
	tests := instanceSelectorCapCases("disk", "scsi0:1", "NoSuchDisk")

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _, teardown := prepareVSphereSim(t)
			defer teardown()
			collr.CollectVMDiskPerformance = true
			collr.VMDisksInclude = tc.include
			collr.MaxVMDisks = tc.max

			require.NoError(t, collr.Init(context.Background()))
			vm := firstSortedVM(t, collr)
			collr.scraper = mockVMDiskPerformanceScraper{
				mockScraper: mockScraper{collr.scraper},
				vmID:        vm.ID,
				series: append(
					testVMDiskPerformanceSeries("scsi0:0"),
					testVMDiskPerformanceSeries("scsi0:1")...,
				),
			}

			require.NotEmpty(t, collectMapForTest(t, collr))

			require.Equal(t, tc.want, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), vmDiskDeviceIOReadMetric))
		})
	}
}

type mockVMDiskPerformanceScraper struct {
	mockScraper
	vmID   string
	series []performance.MetricSeries
}

func (s mockVMDiskPerformanceScraper) ScrapeVMs(vms rs.VMs) []performance.EntityMetric {
	vm := vms.Get(s.vmID)
	if vm == nil {
		return nil
	}
	return []performance.EntityMetric{{
		Entity: vm.Ref,
		Value:  append([]performance.MetricSeries(nil), s.series...),
	}}
}

func testVMDiskPerformanceSeries(instance string) []performance.MetricSeries {
	return []performance.MetricSeries{
		{Name: "virtualDisk.read.average", Instance: instance, Value: []int64{11}},
		{Name: "virtualDisk.write.average", Instance: instance, Value: []int64{12}},
		{Name: "virtualDisk.numberReadAveraged.average", Instance: instance, Value: []int64{21}},
		{Name: "virtualDisk.numberWriteAveraged.average", Instance: instance, Value: []int64{22}},
		{Name: "virtualDisk.totalReadLatency.average", Instance: instance, Value: []int64{31}},
		{Name: "virtualDisk.totalWriteLatency.average", Instance: instance, Value: []int64{32}},
		{Name: "virtualDisk.readOIO.latest", Instance: instance, Value: []int64{41}},
		{Name: "virtualDisk.writeOIO.latest", Instance: instance, Value: []int64{42}},
	}
}

func firstSortedVM(t *testing.T, collr *Collector) *rs.VM {
	t.Helper()

	vms := sortedVMs(collr.resources.VMs)
	require.NotEmpty(t, vms)
	return vms[0]
}

func vmDiskPerformanceLabelsMap(collr *Collector, vm *rs.VM, instance string) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range collr.vmDiskPerformanceLabels(vm, instance) {
		labels[label.Key] = label.Value
	}
	return labels
}
