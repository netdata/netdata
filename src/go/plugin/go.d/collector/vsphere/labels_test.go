// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func TestCollector_OptInInventoryPathAndVMGuestLabels(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.InventoryPathLabel = true
	collr.VMGuestLabels = []string{vmGuestLabelHostName, vmGuestLabelIP, vmGuestLabelOS}

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}
	for _, vm := range collr.resources.VMs {
		vm.GuestHostName = "guest-" + vm.ID
		vm.GuestIPAddress = "192.0.2.10"
		vm.GuestFullName = "Test Guest OS"
		vm.Labels = map[string]string{
			"vsphere_custom_attribute_owner": "platform",
			"vsphere_tag_service":            "payments",
		}
	}
	for _, host := range collr.resources.Hosts {
		host.Labels = map[string]string{"vsphere_tag_env": "prod"}
	}

	mx := collectMapForTest(t, collr)
	require.NotEmpty(t, mx)

	vm := firstVMWithMetric(t, collr.resources.VMs, mx, "cpu.usage.average")
	host := firstHostWithMetric(t, collr.resources.Hosts, mx, "cpu.usage.average")
	createdCharts, _ := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))

	vmChartID := v2ChartTemplateID("vsphere.vm_cpu_utilization") + "_" + vm.ID
	require.Equal(t, collr.inventoryPath(vm.ID), createdCharts[vmChartID].Labels["inventory_path"])
	require.Equal(t, "guest-"+vm.ID, createdCharts[vmChartID].Labels[vmGuestLabelHostName])
	require.Equal(t, "192.0.2.10", createdCharts[vmChartID].Labels[vmGuestLabelIP])
	require.Equal(t, "Test Guest OS", createdCharts[vmChartID].Labels[vmGuestLabelOS])
	require.Equal(t, "platform", createdCharts[vmChartID].Labels["vsphere_custom_attribute_owner"])
	require.Equal(t, "payments", createdCharts[vmChartID].Labels["vsphere_tag_service"])

	hostChartID := v2ChartTemplateID("vsphere.host_cpu_utilization") + "_" + host.ID
	require.Equal(t, collr.inventoryPath(host.ID), createdCharts[hostChartID].Labels["inventory_path"])
	require.Equal(t, "prod", createdCharts[hostChartID].Labels["vsphere_tag_env"])
	require.NotContains(t, createdCharts[hostChartID].Labels, vmGuestLabelHostName)
	require.NotContains(t, createdCharts[hostChartID].Labels, vmGuestLabelIP)
	require.NotContains(t, createdCharts[hostChartID].Labels, vmGuestLabelOS)
}

func TestCollector_Init_ReturnsFalseIfInvalidVMGuestLabel(t *testing.T) {
	collr := New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "pass"
	collr.VMGuestLabels = []string{vmGuestLabelHostName, "invalid"}

	require.ErrorContains(t, collr.Init(context.Background()), `vm_guest_labels has invalid value "invalid"`)
}

func TestCollector_Init_ReturnsFalseIfInvalidUserMetadataLabelConfig(t *testing.T) {
	collr := New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "pass"
	collr.VSphereTagCategories = []string{"["}
	require.ErrorContains(t, collr.Init(context.Background()), "vsphere_tag_categories has invalid pattern")

	collr = New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "pass"
	collr.VSphereTagCategories = []string{"!"}
	require.ErrorContains(t, collr.Init(context.Background()), "vsphere_tag_categories has invalid empty negative pattern")

	collr = New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "pass"
	collr.CustomAttributes = []string{"!Secret"}
	require.ErrorContains(t, collr.Init(context.Background()), "custom_attributes must include at least one positive pattern")
}

func firstHostWithMetric(t *testing.T, hosts rs.Hosts, mx map[string]int64, metric string) *rs.Host {
	t.Helper()
	for _, host := range hosts {
		if _, ok := mx[host.ID+"_"+metric]; ok {
			return host
		}
	}
	t.Fatalf("expected at least one host with metric %q", metric)
	return nil
}

func firstVMWithMetric(t *testing.T, vms rs.VMs, mx map[string]int64, metric string) *rs.VM {
	t.Helper()
	for _, vm := range vms {
		if _, ok := mx[vm.ID+"_"+metric]; ok {
			return vm
		}
	}
	t.Fatalf("expected at least one VM with metric %q", metric)
	return nil
}

func TestUserMetadataPatternMatcherPreservesListItems(t *testing.T) {
	m, err := newUserMetadataPatternMatcher("custom_attributes", []string{"!Business Secret", "Cost Center", "Business*"})
	require.NoError(t, err)

	require.True(t, m.MatchString("Cost Center"))
	require.True(t, m.MatchString("Business Unit"))
	require.False(t, m.MatchString("Cost"))
	require.False(t, m.MatchString("Business Secret"))
}

func TestInventoryPathFromParts(t *testing.T) {
	require.Equal(t, "/DC/Cluster/Host/VM", inventoryPathFromParts("DC", "Cluster", "Host", "VM"))
	require.Equal(t, "/DC/VM", inventoryPathFromParts("/DC/", "", " VM "))
	require.Empty(t, inventoryPathFromParts("", " "))
}
