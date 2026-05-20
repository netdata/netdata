// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func TestCollector_Init_ReturnsFalseIfInvalidVMDiskConfig(t *testing.T) {
	collr := New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "pass"
	collr.CollectVMDisks = true

	collr.VMDisksInclude = []string{"["}
	require.ErrorContains(t, collr.Init(context.Background()), "vm_disk_include has invalid pattern")

	collr.VMDisksInclude = []string{"!"}
	require.ErrorContains(t, collr.Init(context.Background()), "vm_disk_include has invalid empty negative pattern")

	collr.VMDisksInclude = []string{"!Hard disk 1"}
	require.ErrorContains(t, collr.Init(context.Background()), "vm_disk_include must include at least one positive pattern")
}

func TestCollector_VMDisksDefaultOff(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}
	setOnlyTestDisks(t, collr, []rs.VMDisk{{Key: 2000, Label: "Hard disk 1", CapacityBytes: 42}})

	require.NotEmpty(t, collectMapForTest(t, collr))

	require.Zero(t, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), vmDiskCapacityMetric))
}

func TestCollector_VMDisksOptInEmitsCapacityChart(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.CollectVMDisks = true

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}
	vm := setOnlyTestDisks(t, collr, []rs.VMDisk{{Key: 2000, Label: "Hard disk 1", CapacityBytes: 1024}})

	require.NotEmpty(t, collectMapForTest(t, collr))

	labels := vmDiskLabelsMap(collr, vm, vm.Disks[0])
	got, ok := collr.MetricStore().Read(metrix.ReadRaw()).Value(vmDiskCapacityMetric, labels)
	require.True(t, ok)
	require.EqualValues(t, 1024, got)

	createdCharts, createdDims := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))
	chartID := findChartIDByLabelsAndContext(t, createdCharts, vmDiskCapacityContext, map[string]string{
		"id":           vm.ID,
		vmDiskKeyLabel: "2000",
	})
	require.Equal(t, "Hard disk 1", createdCharts[chartID].Labels[vmDiskDisplayNameLabel])
	require.Contains(t, createdDims[chartID], vmDiskCapacityDim)
}

func TestCollector_VMDisksSelector(t *testing.T) {
	tests := map[string]struct {
		include []string
		want    int
	}{
		"selector keeps matching disk labels": {
			include: []string{"Hard*2"},
			want:    1,
		},
		"selector keeps exact disk labels with spaces": {
			include: []string{"Hard disk 1"},
			want:    1,
		},
		"selector keeps matching disk keys": {
			include: []string{"key:2001"},
			want:    1,
		},
		"selector keeps all disk series": {
			include: []string{"*"},
			want:    2,
		},
		"selector can exclude all disks": {
			include: []string{"NoSuchDisk"},
			want:    0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _, teardown := prepareVSphereSim(t)
			defer teardown()
			collr.CollectVMDisks = true
			collr.VMDisksInclude = tc.include

			require.NoError(t, collr.Init(context.Background()))
			collr.scraper = mockScraper{collr.scraper}
			setOnlyTestDisks(t, collr, []rs.VMDisk{
				{Key: 2000, Label: "Hard disk 1", CapacityBytes: 1024},
				{Key: 2001, Label: "Hard disk 2", CapacityBytes: 2048},
			})

			require.NotEmpty(t, collectMapForTest(t, collr))

			require.Equal(t, tc.want, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), vmDiskCapacityMetric))
		})
	}
}

func setOnlyTestDisks(t *testing.T, collr *Collector, disks []rs.VMDisk) *rs.VM {
	t.Helper()

	vms := sortedVMs(collr.resources.VMs)
	require.NotEmpty(t, vms)
	for _, vm := range vms {
		vm.Disks = nil
	}
	vms[0].Disks = disks
	return vms[0]
}

func vmDiskLabelsMap(collr *Collector, vm *rs.VM, disk rs.VMDisk) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range collr.vmDiskLabels(vm, disk) {
		labels[label.Key] = label.Value
	}
	return labels
}

func countMetricSeries(reader metrix.Reader, name string) (count int) {
	reader.ForEachByName(name, func(metrix.LabelView, metrix.SampleValue) {
		count++
	})
	return count
}
