// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func TestCollector_OptionalVnodesDefaultOff(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}

	require.NotEmpty(t, collectMapForTest(t, collr))

	require.Empty(t, nonDefaultScopes(collr.MetricStore().Read()))
}

func TestCollector_ESXIVnodesScopeOnlyHostMetrics(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.ESXIVnodes = true

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}

	mx := collectMapForTest(t, collr)
	require.NotEmpty(t, mx)

	host := firstHostWithMetric(t, collr.resources.Hosts, mx, "cpu.usage.average")
	scope := collr.esxiHostScope(host)
	require.False(t, scope.IsDefault())

	require.Contains(t, scopesByKey(collr.MetricStore().Read()), scope.ScopeKey)
	require.Equal(t, vsphereESXIVnodeType, scope.Labels[vsphereVnodeTypeLabel])

	name := v2MetricName("vsphere.host_cpu_utilization", "used")
	labels := labelsForChart(t, collr, "vsphere.host_cpu_utilization", host.ID)

	_, ok := collr.MetricStore().Read().Value(name, labels)
	require.False(t, ok, "host metric should move out of the default scope when esxi_vnodes is enabled")

	_, ok = collr.MetricStore().Read(metrix.ReadHostScope(scope.ScopeKey)).Value(name, labels)
	require.True(t, ok, "host metric should be present in the ESXi host scope")
}

func TestCollector_VMVnodesScopeOnlyVMMetrics(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.VMVnodes = true

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}

	mx := collectMapForTest(t, collr)
	require.NotEmpty(t, mx)

	vm := firstVMWithMetric(t, collr.resources.VMs, mx, "cpu.usage.average")
	scope := collr.vmHostScope(vm)
	require.False(t, scope.IsDefault())

	require.Contains(t, scopesByKey(collr.MetricStore().Read()), scope.ScopeKey)
	require.Equal(t, vsphereVMVnodeType, scope.Labels[vsphereVnodeTypeLabel])

	name := v2MetricName("vsphere.vm_cpu_utilization", "used")
	labels := labelsForChart(t, collr, "vsphere.vm_cpu_utilization", vm.ID)

	_, ok := collr.MetricStore().Read().Value(name, labels)
	require.False(t, ok, "VM metric should move out of the default scope when vm_vnodes is enabled")

	_, ok = collr.MetricStore().Read(metrix.ReadHostScope(scope.ScopeKey)).Value(name, labels)
	require.True(t, ok, "VM metric should be present in the VM host scope")
}

func TestCollector_VnodesRequireVCenterInstanceUUID(t *testing.T) {
	collr := New()
	collr.URL = "https://user:pass@vcenter.local"

	scope := collr.esxiHostScope(&rs.Host{ID: "host-1", Name: "esxi-1"})

	require.True(t, scope.IsDefault())
}

func nonDefaultScopes(reader metrix.Reader) []metrix.HostScope {
	var scopes []metrix.HostScope
	for _, scope := range reader.HostScopes() {
		if !scope.IsDefault() {
			scopes = append(scopes, scope)
		}
	}
	return scopes
}

func scopesByKey(reader metrix.Reader) map[string]metrix.HostScope {
	scopes := make(map[string]metrix.HostScope)
	for _, scope := range reader.HostScopes() {
		if scope.IsDefault() {
			continue
		}
		scopes[scope.ScopeKey] = scope
	}
	return scopes
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

func labelsForChart(t *testing.T, collr *Collector, context, resourceID string) metrix.Labels {
	t.Helper()
	for _, chart := range *collr.Charts() {
		id, ok := chartResourceID(chart)
		if !ok || id != resourceID || chart.Ctx != context {
			continue
		}
		labels := make(metrix.Labels)
		for _, label := range collr.v2ChartLabels(chart, resourceID) {
			labels[label.Key] = label.Value
		}
		return labels
	}
	t.Fatalf("expected chart context %q for resource %q", context, resourceID)
	return nil
}
