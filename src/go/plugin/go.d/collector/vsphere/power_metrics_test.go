// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/performance"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestCollector_PowerMetricsEmitCharts(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	host := firstSortedHost(t, collr)
	vm := firstSortedVM(t, collr)
	collr.scraper = mockPowerMetricsScraper{
		mockScraper: mockScraper{collr.scraper},
		hostID:      host.ID,
		vmID:        vm.ID,
		hostSeries:  testHostPowerSeries(),
		vmSeries:    testVMPowerSeries(),
	}

	require.NotEmpty(t, collectScalarSeriesForTest(t, collr))

	reader := collr.MetricStore().Read(metrix.ReadRaw())
	hostLabels := hostPowerLabelsMap(collr, host)
	requireMetricValue(t, reader, hostPowerUsagePowerMetric, hostLabels, 101)
	requireMetricValue(t, reader, hostPowerUsageCapMetric, hostLabels, 102)
	requireMetricValue(t, reader, hostPowerCapacityUsageUsedMetric, hostLabels, 103)
	requireMetricValue(t, reader, hostPowerCapacityUsageUsableMetric, hostLabels, 104)
	requireMetricValue(t, reader, hostPowerCapacityUsageIdleMetric, hostLabels, 105)
	requireMetricValue(t, reader, hostPowerCapacityUsageSystemMetric, hostLabels, 106)
	requireMetricValue(t, reader, hostPowerCapacityUsageVMMetric, hostLabels, 107)
	requireMetricValue(t, reader, hostPowerCapacityUtilizationMetric, hostLabels, 108)
	requireMetricValue(t, reader, hostEnergyUsageMetric, hostLabels, 109)

	vmLabels := vmPowerLabelsMap(collr, vm)
	requireMetricValue(t, reader, vmPowerUsagePowerMetric, vmLabels, 201)
	requireMetricValue(t, reader, vmEnergyUsageMetric, vmLabels, 202)

	createdCharts, createdDims := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))
	hostPowerChartID := findChartIDByLabelsAndContext(t, createdCharts, "vsphere.host_power_usage", map[string]string{
		"id": host.ID,
	})
	require.Contains(t, createdDims[hostPowerChartID], "power")
	require.Contains(t, createdDims[hostPowerChartID], "cap")

	vmPowerChartID := findChartIDByLabelsAndContext(t, createdCharts, "vsphere.vm_power_usage", map[string]string{
		"id": vm.ID,
	})
	require.Contains(t, createdDims[vmPowerChartID], "power")
	collecttest.AssertChartCoverage(t, collr, collecttest.ChartCoverageExpectation{})
	requireChartSelectorsMatchSeries(t, collr,
		"vsphere.host_power_",
		"vsphere.host_energy_usage",
		"vsphere.vm_power_usage",
		"vsphere.vm_energy_usage",
	)
}

type mockPowerMetricsScraper struct {
	mockScraper
	hostID     string
	vmID       string
	hostSeries []performance.MetricSeries
	vmSeries   []performance.MetricSeries
}

func (s mockPowerMetricsScraper) ScrapeHosts(hosts rs.Hosts) []performance.EntityMetric {
	host := hosts.Get(s.hostID)
	if host == nil {
		return nil
	}
	return []performance.EntityMetric{{
		Entity: host.Ref,
		Value:  append([]performance.MetricSeries(nil), s.hostSeries...),
	}}
}

func (s mockPowerMetricsScraper) ScrapeVMs(vms rs.VMs) []performance.EntityMetric {
	vm := vms.Get(s.vmID)
	if vm == nil {
		return nil
	}
	return []performance.EntityMetric{{
		Entity: vm.Ref,
		Value:  append([]performance.MetricSeries(nil), s.vmSeries...),
	}}
}

func testHostPowerSeries() []performance.MetricSeries {
	return []performance.MetricSeries{
		{Name: "power.power.average", Value: []int64{101}},
		{Name: "power.powerCap.average", Value: []int64{102}},
		{Name: "power.capacity.usage.average", Value: []int64{103}},
		{Name: "power.capacity.usable.average", Value: []int64{104}},
		{Name: "power.capacity.usageIdle.average", Value: []int64{105}},
		{Name: "power.capacity.usageSystem.average", Value: []int64{106}},
		{Name: "power.capacity.usageVm.average", Value: []int64{107}},
		{Name: "power.capacity.usagePct.average", Value: []int64{108}},
		{Name: "power.energy.summation", Value: []int64{109}},
	}
}

func testVMPowerSeries() []performance.MetricSeries {
	return []performance.MetricSeries{
		{Name: "power.power.average", Value: []int64{201}},
		{Name: "power.energy.summation", Value: []int64{202}},
	}
}

func hostPowerLabelsMap(collr *Collector, host *rs.Host) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range collr.hostPowerLabels(host) {
		labels[label.Key] = label.Value
	}
	return labels
}

func vmPowerLabelsMap(collr *Collector, vm *rs.VM) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range collr.vmPowerLabels(vm) {
		labels[label.Key] = label.Value
	}
	return labels
}
