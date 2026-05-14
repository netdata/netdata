// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/performance"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

func TestVSphereMethods(t *testing.T) {
	methods := vsphereMethods()

	require.Len(t, methods, 2)
	byID := make(map[string]funcapi.MethodConfig, len(methods))
	for _, method := range methods {
		byID[method.ID] = method
	}

	readiness := byID["readiness"]
	require.Equal(t, "vSphere Readiness", readiness.Name)
	require.Equal(t, 30, readiness.UpdateEvery)
	require.True(t, readiness.RequireCloud)
	require.False(t, readiness.AgentWide)

	topology := byID["topology:vsphere"]
	require.Equal(t, "vSphere Topology", topology.Name)
	require.Equal(t, 30, topology.UpdateEvery)
	require.True(t, topology.RequireCloud)
	require.False(t, topology.AgentWide)
	require.Contains(t, topology.Aliases, "topology:vsphere")
	require.Equal(t, "topology", topology.ResponseType)
	require.NotNil(t, topology.Presentation())
}

func TestFuncReadiness_HandleWithoutDiscovery(t *testing.T) {
	collr := New()
	handler := &funcReadiness{collector: collr}

	resp := handler.Handle(context.Background(), "readiness", nil)

	require.Equal(t, 200, resp.Status)
	require.NotEmpty(t, resp.Columns)
	rows := readinessRowsFromResponse(t, resp)
	require.Equal(t, "not_ready", rows["target_url"][2])
	require.Equal(t, "not_ready", rows["inventory_cache"][2])
	require.Equal(t, "disabled", rows["vsan"][2])
}

func TestFuncReadiness_HandlePartialInitState(t *testing.T) {
	collr := New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "[REDACTED_SECRET]"
	collr.vcenterInstanceUUID = "vcenter-instance-uuid"

	handler := &funcReadiness{collector: collr}
	resp := handler.Handle(context.Background(), "readiness", nil)

	require.Equal(t, 200, resp.Status)
	rows := readinessRowsFromResponse(t, resp)
	require.Equal(t, "ok", rows["target_url"][2])
	require.Equal(t, "ok", rows["credentials"][2])
	require.Equal(t, "not_ready", rows["client"][2])
	require.Equal(t, "not_ready", rows["inventory_cache"][2])
}

func TestFuncReadiness_HandleWithVSANCachedData(t *testing.T) {
	collr := newVSANTestCollector(true)
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "[REDACTED_SECRET]"
	for _, cluster := range collr.resources.Clusters {
		cluster.MetricList = performance.MetricList{{CounterId: 1}}
	}
	for _, host := range collr.resources.Hosts {
		host.MetricList = performance.MetricList{{CounterId: 1}}
	}
	for _, vm := range collr.resources.VMs {
		vm.MetricList = performance.MetricList{{CounterId: 1}}
	}

	handler := &funcReadiness{collector: collr}
	resp := handler.Handle(context.Background(), "readiness", nil)

	require.Equal(t, 200, resp.Status)
	rows := readinessRowsFromResponse(t, resp)
	require.Equal(t, "ok", rows["target_url"][2])
	require.Equal(t, "ok", rows["credentials"][2])
	require.Equal(t, "ok", rows["inventory_cache"][2])
	require.Equal(t, "ok", rows["vsan"][2])
	require.Contains(t, rows["vsan"][3], "vSAN data cached")
}

func TestFuncReadiness_UnknownMethod(t *testing.T) {
	handler := &funcReadiness{collector: New()}

	resp := handler.Handle(context.Background(), "unknown", nil)

	require.Equal(t, 404, resp.Status)
}

func readinessRowsFromResponse(t *testing.T, resp *funcapi.FunctionResponse) map[string][]any {
	t.Helper()

	rows, ok := resp.Data.([][]any)
	require.True(t, ok)
	byCheck := make(map[string][]any, len(rows))
	for _, row := range rows {
		require.Len(t, row, len(readinessColumns))
		check, ok := row[0].(string)
		require.True(t, ok)
		byCheck[check] = row
	}
	return byCheck
}
