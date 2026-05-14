// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

const chartTemplatePath = "charts.yaml"

func TestCollector_ChartTemplateYAML(t *testing.T) {
	want := buildV2ChartTemplateYAML(t)

	if os.Getenv("UPDATE_VSPHERE_CHARTS") == "1" {
		require.NoError(t, os.WriteFile(chartTemplatePath, want, 0644))
		return
	}

	assert.Equal(t, string(want), chartTemplateYAML)
	collecttest.AssertChartTemplateSchema(t, chartTemplateYAML)
	spec, err := charttpl.DecodeYAML([]byte(chartTemplateYAML))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func buildV2ChartTemplateYAML(t *testing.T) []byte {
	t.Helper()

	spec := buildV2ChartTemplateSpec()
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	require.NoError(t, enc.Encode(spec))
	require.NoError(t, enc.Close())
	return buf.Bytes()
}

func buildV2ChartTemplateSpec() charttpl.Spec {
	return charttpl.Spec{
		Version:          charttpl.VersionV1,
		ContextNamespace: "vsphere",
		Groups:           buildV2ChartGroups(),
	}
}

func buildV2ChartGroups() []charttpl.Group {
	var groups []charttpl.Group
	for _, set := range legacyChartTemplateSets() {
		byFamily := make(map[string]int)
		for _, chart := range set.charts {
			idx, ok := byFamily[chart.Fam]
			if !ok {
				groups = append(groups, charttpl.Group{
					Family: chart.Fam,
					ChartDefaults: &charttpl.ChartDefaults{
						Instances: &charttpl.Instances{ByLabels: []string{"id"}},
					},
				})
				idx = len(groups) - 1
				byFamily[chart.Fam] = idx
			}
			appendChartToGroup(&groups[idx], chart)
		}
	}
	groups = append(groups, datastoreClusterChartGroups()...)
	groups = append(groups, vmDiskCapacityChartGroup())
	groups = append(groups, vmDiskPerformanceChartGroups()...)
	groups = append(groups, vmNICPerformanceChartGroups()...)
	groups = append(groups, hostNICPerformanceChartGroups()...)
	groups = append(groups, hostDiskPerformanceChartGroups()...)
	groups = append(groups, hostStorageAdapterPerformanceChartGroups()...)
	groups = append(groups, hostStoragePathPerformanceChartGroups()...)
	groups = append(groups, hostCPUInstancePerformanceChartGroups())
	groups = append(groups, powerMetricsChartGroups()...)
	groups = append(groups, vsanChartGroups()...)
	return groups
}

func datastoreClusterChartGroups() []charttpl.Group {
	return []charttpl.Group{
		{
			Family: "datastore clusters space",
			Metrics: []string{
				datastoreClusterSpaceUsageCapacityMetric,
				datastoreClusterSpaceUsageFreeMetric,
				datastoreClusterSpaceUsageUsedMetric,
				datastoreClusterSpaceUtilizationUsedMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id"}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(datastoreClusterSpaceUtilizationContext),
					Title:     "Datastore Cluster space utilization",
					Context:   v2ChartTemplateID(datastoreClusterSpaceUtilizationContext),
					Units:     "percentage",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioInventoryObjects + 1,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{
							Selector: datastoreClusterSpaceUtilizationUsedMetric,
							Name:     "used",
							Options:  &charttpl.DimensionOptions{Divisor: 100},
						},
					},
				},
				{
					ID:        v2ChartTemplateID(datastoreClusterSpaceUsageContext),
					Title:     "Datastore Cluster space usage",
					Context:   v2ChartTemplateID(datastoreClusterSpaceUsageContext),
					Units:     "bytes",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioInventoryObjects + 2,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: datastoreClusterSpaceUsageCapacityMetric, Name: "capacity"},
						{Selector: datastoreClusterSpaceUsageFreeMetric, Name: "free"},
						{Selector: datastoreClusterSpaceUsageUsedMetric, Name: "used"},
					},
				},
			},
		},
		{
			Family: "datastore clusters status",
			Metrics: []string{
				datastoreClusterStorageDRSEnabledMetric,
				datastoreClusterStorageDRSDisabledMetric,
				datastoreClusterOverallStatusGreenMetric,
				datastoreClusterOverallStatusRedMetric,
				datastoreClusterOverallStatusYellowMetric,
				datastoreClusterOverallStatusGrayMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id"}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(datastoreClusterStorageDRSContext),
					Title:     "Datastore Cluster Storage DRS status",
					Context:   v2ChartTemplateID(datastoreClusterStorageDRSContext),
					Units:     "status",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioInventoryObjects + 3,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: datastoreClusterStorageDRSEnabledMetric, Name: "enabled"},
						{Selector: datastoreClusterStorageDRSDisabledMetric, Name: "disabled"},
					},
				},
				{
					ID:        v2ChartTemplateID(datastoreClusterOverallStatusContext),
					Title:     "Datastore Cluster overall status",
					Context:   v2ChartTemplateID(datastoreClusterOverallStatusContext),
					Units:     "status",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioInventoryObjects + 4,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: datastoreClusterOverallStatusGreenMetric, Name: "green"},
						{Selector: datastoreClusterOverallStatusRedMetric, Name: "red"},
						{Selector: datastoreClusterOverallStatusYellowMetric, Name: "yellow"},
						{Selector: datastoreClusterOverallStatusGrayMetric, Name: "gray"},
					},
				},
			},
		},
	}
}

func vmDiskCapacityChartGroup() charttpl.Group {
	return charttpl.Group{
		Family:  "vms disk devices",
		Metrics: []string{vmDiskCapacityMetric},
		ChartDefaults: &charttpl.ChartDefaults{
			Instances: &charttpl.Instances{ByLabels: []string{"id", vmDiskKeyLabel}},
		},
		Charts: []charttpl.Chart{
			{
				ID:        v2ChartTemplateID(vmDiskCapacityContext),
				Title:     "Virtual Machine virtual disk capacity",
				Context:   v2ChartTemplateID(vmDiskCapacityContext),
				Units:     "bytes",
				Algorithm: collectorapi.Absolute.String(),
				Type:      collectorapi.Line.String(),
				Priority:  prioVmDiskMaxLatency + 1,
				Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
				Dimensions: []charttpl.Dimension{
					{
						Selector: vmDiskCapacityMetric,
						Name:     vmDiskCapacityDim,
					},
				},
			},
		},
	}
}

func vmDiskPerformanceChartGroups() []charttpl.Group {
	return []charttpl.Group{
		{
			Family: "vms disk devices",
			Metrics: []string{
				vmDiskDeviceIOReadMetric,
				vmDiskDeviceIOWriteMetric,
				vmDiskDeviceIOPSReadMetric,
				vmDiskDeviceIOPSWriteMetric,
				vmDiskDeviceLatencyReadMetric,
				vmDiskDeviceLatencyWriteMetric,
				vmDiskDeviceOIOReadMetric,
				vmDiskDeviceOIOWriteMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id", vmDiskInstanceLabel}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(vmDiskDeviceIOContext),
					Title:     "Virtual Machine virtual disk I/O",
					Context:   v2ChartTemplateID(vmDiskDeviceIOContext),
					Units:     "KiB/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Area.String(),
					Priority:  prioVmDiskMaxLatency + 2,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vmDiskDeviceIOReadMetric, Name: vmDiskDeviceIOReadDim},
						{Selector: vmDiskDeviceIOWriteMetric, Name: vmDiskDeviceIOWriteDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(vmDiskDeviceIOPSContext),
					Title:     "Virtual Machine virtual disk IOPS",
					Context:   v2ChartTemplateID(vmDiskDeviceIOPSContext),
					Units:     "operations/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioVmDiskMaxLatency + 3,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vmDiskDeviceIOPSReadMetric, Name: vmDiskDeviceIOPSReadDim},
						{Selector: vmDiskDeviceIOPSWriteMetric, Name: vmDiskDeviceIOPSWriteDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(vmDiskDeviceLatencyContext),
					Title:     "Virtual Machine virtual disk latency",
					Context:   v2ChartTemplateID(vmDiskDeviceLatencyContext),
					Units:     "milliseconds",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioVmDiskMaxLatency + 4,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vmDiskDeviceLatencyReadMetric, Name: vmDiskDeviceLatencyReadDim},
						{Selector: vmDiskDeviceLatencyWriteMetric, Name: vmDiskDeviceLatencyWriteDim},
					},
				},
				{
					ID:        v2ChartTemplateID(vmDiskDeviceOutstandingIOContext),
					Title:     "Virtual Machine virtual disk outstanding I/O",
					Context:   v2ChartTemplateID(vmDiskDeviceOutstandingIOContext),
					Units:     "operations",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioVmDiskMaxLatency + 5,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vmDiskDeviceOIOReadMetric, Name: vmDiskDeviceOIOReadDim},
						{Selector: vmDiskDeviceOIOWriteMetric, Name: vmDiskDeviceOIOWriteDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
			},
		},
	}
}

func vmNICPerformanceChartGroups() []charttpl.Group {
	return []charttpl.Group{
		{
			Family: "vms network interfaces",
			Metrics: []string{
				vmNetInterfaceTrafficRxMetric,
				vmNetInterfaceTrafficTxMetric,
				vmNetInterfacePacketsRxMetric,
				vmNetInterfacePacketsTxMetric,
				vmNetInterfaceDropsRxMetric,
				vmNetInterfaceDropsTxMetric,
				vmNetInterfaceBroadcastPacketsRxMetric,
				vmNetInterfaceBroadcastPacketsTxMetric,
				vmNetInterfaceMulticastPacketsRxMetric,
				vmNetInterfaceMulticastPacketsTxMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id", vmNICInstanceLabel}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(vmNetInterfaceTrafficContext),
					Title:     "Virtual Machine network interface traffic",
					Context:   v2ChartTemplateID(vmNetInterfaceTrafficContext),
					Units:     "KiB/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Area.String(),
					Priority:  prioVmNetworkTraffic + 1,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vmNetInterfaceTrafficRxMetric, Name: vmNetInterfaceTrafficRxDim},
						{Selector: vmNetInterfaceTrafficTxMetric, Name: vmNetInterfaceTrafficTxDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(vmNetInterfacePacketsContext),
					Title:     "Virtual Machine network interface packets",
					Context:   v2ChartTemplateID(vmNetInterfacePacketsContext),
					Units:     "packets",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioVmNetworkPackets + 1,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vmNetInterfacePacketsRxMetric, Name: vmNetInterfacePacketsRxDim},
						{Selector: vmNetInterfacePacketsTxMetric, Name: vmNetInterfacePacketsTxDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(vmNetInterfaceDropsContext),
					Title:     "Virtual Machine network interface packet drops",
					Context:   v2ChartTemplateID(vmNetInterfaceDropsContext),
					Units:     "drops",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioVmNetworkDrops + 1,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vmNetInterfaceDropsRxMetric, Name: vmNetInterfaceDropsRxDim},
						{Selector: vmNetInterfaceDropsTxMetric, Name: vmNetInterfaceDropsTxDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(vmNetInterfaceBroadcastPacketsContext),
					Title:     "Virtual Machine network interface broadcast packets",
					Context:   v2ChartTemplateID(vmNetInterfaceBroadcastPacketsContext),
					Units:     "packets",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioVmNetworkDrops + 2,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vmNetInterfaceBroadcastPacketsRxMetric, Name: vmNetInterfaceBroadcastPacketsRxDim},
						{Selector: vmNetInterfaceBroadcastPacketsTxMetric, Name: vmNetInterfaceBroadcastPacketsTxDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(vmNetInterfaceMulticastPacketsContext),
					Title:     "Virtual Machine network interface multicast packets",
					Context:   v2ChartTemplateID(vmNetInterfaceMulticastPacketsContext),
					Units:     "packets",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioVmNetworkDrops + 3,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vmNetInterfaceMulticastPacketsRxMetric, Name: vmNetInterfaceMulticastPacketsRxDim},
						{Selector: vmNetInterfaceMulticastPacketsTxMetric, Name: vmNetInterfaceMulticastPacketsTxDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
			},
		},
	}
}

func hostNICPerformanceChartGroups() []charttpl.Group {
	return []charttpl.Group{
		{
			Family: "hosts network interfaces",
			Metrics: []string{
				hostNetInterfaceTrafficRxMetric,
				hostNetInterfaceTrafficTxMetric,
				hostNetInterfacePacketsRxMetric,
				hostNetInterfacePacketsTxMetric,
				hostNetInterfaceDropsRxMetric,
				hostNetInterfaceDropsTxMetric,
				hostNetInterfaceErrorsRxMetric,
				hostNetInterfaceErrorsTxMetric,
				hostNetInterfaceBroadcastPacketsRxMetric,
				hostNetInterfaceBroadcastPacketsTxMetric,
				hostNetInterfaceMulticastPacketsRxMetric,
				hostNetInterfaceMulticastPacketsTxMetric,
				hostNetInterfaceUnknownProtocolFramesMetric,
				hostNetInterfaceUsageMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id", hostNICInstanceLabel}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(hostNetInterfaceTrafficContext),
					Title:     "ESXi Host network interface traffic",
					Context:   v2ChartTemplateID(hostNetInterfaceTrafficContext),
					Units:     "KiB/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Area.String(),
					Priority:  prioHostNetworkTraffic + 1,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostNetInterfaceTrafficRxMetric, Name: hostNetInterfaceTrafficRxDim},
						{Selector: hostNetInterfaceTrafficTxMetric, Name: hostNetInterfaceTrafficTxDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostNetInterfacePacketsContext),
					Title:     "ESXi Host network interface packets",
					Context:   v2ChartTemplateID(hostNetInterfacePacketsContext),
					Units:     "packets",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostNetworkPackets + 1,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostNetInterfacePacketsRxMetric, Name: hostNetInterfacePacketsRxDim},
						{Selector: hostNetInterfacePacketsTxMetric, Name: hostNetInterfacePacketsTxDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostNetInterfaceDropsContext),
					Title:     "ESXi Host network interface packet drops",
					Context:   v2ChartTemplateID(hostNetInterfaceDropsContext),
					Units:     "drops",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostNetworkDrops + 1,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostNetInterfaceDropsRxMetric, Name: hostNetInterfaceDropsRxDim},
						{Selector: hostNetInterfaceDropsTxMetric, Name: hostNetInterfaceDropsTxDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostNetInterfaceErrorsContext),
					Title:     "ESXi Host network interface packet errors",
					Context:   v2ChartTemplateID(hostNetInterfaceErrorsContext),
					Units:     "errors",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostNetworkErrors + 1,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostNetInterfaceErrorsRxMetric, Name: hostNetInterfaceErrorsRxDim},
						{Selector: hostNetInterfaceErrorsTxMetric, Name: hostNetInterfaceErrorsTxDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostNetInterfaceBroadcastPacketsContext),
					Title:     "ESXi Host network interface broadcast packets",
					Context:   v2ChartTemplateID(hostNetInterfaceBroadcastPacketsContext),
					Units:     "packets",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostNetworkErrors + 2,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostNetInterfaceBroadcastPacketsRxMetric, Name: hostNetInterfaceBroadcastPacketsRxDim},
						{Selector: hostNetInterfaceBroadcastPacketsTxMetric, Name: hostNetInterfaceBroadcastPacketsTxDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostNetInterfaceMulticastPacketsContext),
					Title:     "ESXi Host network interface multicast packets",
					Context:   v2ChartTemplateID(hostNetInterfaceMulticastPacketsContext),
					Units:     "packets",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostNetworkErrors + 3,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostNetInterfaceMulticastPacketsRxMetric, Name: hostNetInterfaceMulticastPacketsRxDim},
						{Selector: hostNetInterfaceMulticastPacketsTxMetric, Name: hostNetInterfaceMulticastPacketsTxDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostNetInterfaceUnknownProtocolFramesContext),
					Title:     "ESXi Host network interface unknown protocol frames",
					Context:   v2ChartTemplateID(hostNetInterfaceUnknownProtocolFramesContext),
					Units:     "frames",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostNetworkErrors + 4,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostNetInterfaceUnknownProtocolFramesMetric, Name: hostNetInterfaceUnknownProtocolFramesDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostNetInterfaceUsageContext),
					Title:     "ESXi Host network interface usage",
					Context:   v2ChartTemplateID(hostNetInterfaceUsageContext),
					Units:     "KiB/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostNetworkErrors + 5,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostNetInterfaceUsageMetric, Name: hostNetInterfaceUsageDim},
					},
				},
			},
		},
	}
}

func hostDiskPerformanceChartGroups() []charttpl.Group {
	return []charttpl.Group{
		{
			Family:  "hosts disk devices",
			Metrics: hostDiskPerformanceOptionalMetricNames(),
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id", hostDiskInstanceLabel}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(hostDiskDeviceIOContext),
					Title:     "ESXi Host disk device I/O",
					Context:   v2ChartTemplateID(hostDiskDeviceIOContext),
					Units:     "KiB/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Area.String(),
					Priority:  prioHostDiskMaxLatency + 1,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostDiskDeviceIOReadMetric, Name: hostDiskDeviceIOReadDim},
						{Selector: hostDiskDeviceIOWriteMetric, Name: hostDiskDeviceIOWriteDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostDiskDeviceIOPSContext),
					Title:     "ESXi Host disk device IOPS",
					Context:   v2ChartTemplateID(hostDiskDeviceIOPSContext),
					Units:     "operations/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 2,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostDiskDeviceIOPSReadMetric, Name: hostDiskDeviceIOPSReadDim},
						{Selector: hostDiskDeviceIOPSWriteMetric, Name: hostDiskDeviceIOPSWriteDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostDiskDeviceRequestsContext),
					Title:     "ESXi Host disk device requests",
					Context:   v2ChartTemplateID(hostDiskDeviceRequestsContext),
					Units:     "requests",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 3,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostDiskDeviceRequestsReadMetric, Name: hostDiskDeviceRequestsReadDim},
						{Selector: hostDiskDeviceRequestsWriteMetric, Name: hostDiskDeviceRequestsWriteDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostDiskDeviceLatencyContext),
					Title:     "ESXi Host disk device latency",
					Context:   v2ChartTemplateID(hostDiskDeviceLatencyContext),
					Units:     "milliseconds",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 4,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostDiskDeviceLatencyTotalMetric, Name: hostDiskDeviceLatencyTotalDim},
						{Selector: hostDiskDeviceLatencyReadMetric, Name: hostDiskDeviceLatencyReadDim},
						{Selector: hostDiskDeviceLatencyWriteMetric, Name: hostDiskDeviceLatencyWriteDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostDiskDeviceLatencyBreakdownContext),
					Title:     "ESXi Host disk device latency breakdown",
					Context:   v2ChartTemplateID(hostDiskDeviceLatencyBreakdownContext),
					Units:     "milliseconds",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 5,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostDiskDeviceLatencyDeviceMetric, Name: hostDiskDeviceLatencyDeviceDim},
						{Selector: hostDiskDeviceLatencyKernelMetric, Name: hostDiskDeviceLatencyKernelDim},
						{Selector: hostDiskDeviceLatencyQueueMetric, Name: hostDiskDeviceLatencyQueueDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostDiskDeviceReadLatencyBreakdownContext),
					Title:     "ESXi Host disk device read latency breakdown",
					Context:   v2ChartTemplateID(hostDiskDeviceReadLatencyBreakdownContext),
					Units:     "milliseconds",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 6,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostDiskDeviceReadLatencyDeviceMetric, Name: hostDiskDeviceLatencyDeviceDim},
						{Selector: hostDiskDeviceReadLatencyKernelMetric, Name: hostDiskDeviceLatencyKernelDim},
						{Selector: hostDiskDeviceReadLatencyQueueMetric, Name: hostDiskDeviceLatencyQueueDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostDiskDeviceWriteLatencyBreakdownContext),
					Title:     "ESXi Host disk device write latency breakdown",
					Context:   v2ChartTemplateID(hostDiskDeviceWriteLatencyBreakdownContext),
					Units:     "milliseconds",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 7,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostDiskDeviceWriteLatencyDeviceMetric, Name: hostDiskDeviceLatencyDeviceDim},
						{Selector: hostDiskDeviceWriteLatencyKernelMetric, Name: hostDiskDeviceLatencyKernelDim},
						{Selector: hostDiskDeviceWriteLatencyQueueMetric, Name: hostDiskDeviceLatencyQueueDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostDiskDeviceCommandsContext),
					Title:     "ESXi Host disk device commands",
					Context:   v2ChartTemplateID(hostDiskDeviceCommandsContext),
					Units:     "commands/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 8,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostDiskDeviceCommandsMetric, Name: hostDiskDeviceCommandsDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostDiskDeviceCommandEventsContext),
					Title:     "ESXi Host disk device command events",
					Context:   v2ChartTemplateID(hostDiskDeviceCommandEventsContext),
					Units:     "events",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 9,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostDiskDeviceCommandEventsIssuedMetric, Name: hostDiskDeviceCommandEventsIssuedDim},
						{Selector: hostDiskDeviceCommandEventsAbortedMetric, Name: hostDiskDeviceCommandEventsAbortedDim},
						{Selector: hostDiskDeviceCommandEventsBusResetsMetric, Name: hostDiskDeviceCommandEventsBusResetsDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostDiskDeviceQueueDepthContext),
					Title:     "ESXi Host disk device maximum queue depth",
					Context:   v2ChartTemplateID(hostDiskDeviceQueueDepthContext),
					Units:     "queue depth",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 10,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostDiskDeviceQueueDepthMetric, Name: hostDiskDeviceQueueDepthDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostDiskDeviceSCSIReservationConflictsContext),
					Title:     "ESXi Host disk device SCSI reservation conflicts",
					Context:   v2ChartTemplateID(hostDiskDeviceSCSIReservationConflictsContext),
					Units:     "conflicts",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 11,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostDiskDeviceSCSIReservationConflictsMetric, Name: hostDiskDeviceSCSIReservationConflictsDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostDiskDeviceSCSIReservationConflictsPctContext),
					Title:     "ESXi Host disk device SCSI reservation conflicts percentage",
					Context:   v2ChartTemplateID(hostDiskDeviceSCSIReservationConflictsPctContext),
					Units:     "percentage",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 12,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostDiskDeviceSCSIReservationConflictsPctMetric, Name: hostDiskDeviceSCSIReservationConflictsPctDim, Options: &charttpl.DimensionOptions{Divisor: 100}},
					},
				},
			},
		},
	}
}

func hostStorageAdapterPerformanceChartGroups() []charttpl.Group {
	return []charttpl.Group{
		{
			Family:  "hosts storage adapters",
			Metrics: hostStorageAdapterInstancePerformanceOptionalMetricNames(),
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id", hostStorageAdapterInstanceLabel}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(hostStorageAdapterIOContext),
					Title:     "ESXi Host storage adapter I/O",
					Context:   v2ChartTemplateID(hostStorageAdapterIOContext),
					Units:     "KiB/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Area.String(),
					Priority:  prioHostDiskMaxLatency + 13,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStorageAdapterIOReadMetric, Name: hostStorageAdapterIOReadDim},
						{Selector: hostStorageAdapterIOWriteMetric, Name: hostStorageAdapterIOWriteDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostStorageAdapterCommandsContext),
					Title:     "ESXi Host storage adapter commands",
					Context:   v2ChartTemplateID(hostStorageAdapterCommandsContext),
					Units:     "commands/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 14,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStorageAdapterCommandsIssuedMetric, Name: hostStorageAdapterCommandsIssuedDim},
						{Selector: hostStorageAdapterCommandsReadMetric, Name: hostStorageAdapterCommandsReadDim},
						{Selector: hostStorageAdapterCommandsWriteMetric, Name: hostStorageAdapterCommandsWriteDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostStorageAdapterLatencyContext),
					Title:     "ESXi Host storage adapter latency",
					Context:   v2ChartTemplateID(hostStorageAdapterLatencyContext),
					Units:     "milliseconds",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 15,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStorageAdapterLatencyReadMetric, Name: hostStorageAdapterLatencyReadDim},
						{Selector: hostStorageAdapterLatencyWriteMetric, Name: hostStorageAdapterLatencyWriteDim},
						{Selector: hostStorageAdapterLatencyQueueMetric, Name: hostStorageAdapterLatencyQueueDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostStorageAdapterQueueContext),
					Title:     "ESXi Host storage adapter queue",
					Context:   v2ChartTemplateID(hostStorageAdapterQueueContext),
					Units:     "I/O requests",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 16,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStorageAdapterQueueOutstandingMetric, Name: hostStorageAdapterQueueOutstandingDim},
						{Selector: hostStorageAdapterQueueQueuedMetric, Name: hostStorageAdapterQueueQueuedDim},
						{Selector: hostStorageAdapterQueueDepthMetric, Name: hostStorageAdapterQueueDepthDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostStorageAdapterOutstandingIOPctContext),
					Title:     "ESXi Host storage adapter outstanding I/O percentage",
					Context:   v2ChartTemplateID(hostStorageAdapterOutstandingIOPctContext),
					Units:     "percentage",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 17,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStorageAdapterOutstandingIOPctMetric, Name: hostStorageAdapterOutstandingIOPctDim, Options: &charttpl.DimensionOptions{Divisor: 100}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostStorageAdapterThroughputContext),
					Title:     "ESXi Host storage adapter throughput usage",
					Context:   v2ChartTemplateID(hostStorageAdapterThroughputContext),
					Units:     "KiB/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 18,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStorageAdapterThroughputMetric, Name: hostStorageAdapterThroughputDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostStorageAdapterThroughputContentionContext),
					Title:     "ESXi Host storage adapter throughput contention",
					Context:   v2ChartTemplateID(hostStorageAdapterThroughputContentionContext),
					Units:     "milliseconds",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 19,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStorageAdapterThroughputContentionMetric, Name: hostStorageAdapterThroughputContentionDim},
					},
				},
			},
		},
		{
			Family: "hosts storage adapters",
			Metrics: []string{
				hostStorageAdapterMaxLatencyMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id"}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(hostStorageAdapterMaxLatencyContext),
					Title:     "ESXi Host storage adapter maximum latency",
					Context:   v2ChartTemplateID(hostStorageAdapterMaxLatencyContext),
					Units:     "milliseconds",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 20,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStorageAdapterMaxLatencyMetric, Name: hostStorageAdapterMaxLatencyDim},
					},
				},
			},
		},
	}
}

func hostStoragePathPerformanceChartGroups() []charttpl.Group {
	return []charttpl.Group{
		{
			Family:  "hosts storage paths",
			Metrics: hostStoragePathInstancePerformanceOptionalMetricNames(),
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id", hostStoragePathInstanceLabel}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(hostStoragePathIOContext),
					Title:     "ESXi Host storage path I/O",
					Context:   v2ChartTemplateID(hostStoragePathIOContext),
					Units:     "KiB/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Area.String(),
					Priority:  prioHostDiskMaxLatency + 21,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStoragePathIOReadMetric, Name: hostStoragePathIOReadDim},
						{Selector: hostStoragePathIOWriteMetric, Name: hostStoragePathIOWriteDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostStoragePathCommandsContext),
					Title:     "ESXi Host storage path commands",
					Context:   v2ChartTemplateID(hostStoragePathCommandsContext),
					Units:     "commands/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 22,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStoragePathCommandsIssuedMetric, Name: hostStoragePathCommandsIssuedDim},
						{Selector: hostStoragePathCommandsReadMetric, Name: hostStoragePathCommandsReadDim},
						{Selector: hostStoragePathCommandsWriteMetric, Name: hostStoragePathCommandsWriteDim, Options: &charttpl.DimensionOptions{Multiplier: -1}},
					},
				},
				{
					ID:        v2ChartTemplateID(hostStoragePathLatencyContext),
					Title:     "ESXi Host storage path latency",
					Context:   v2ChartTemplateID(hostStoragePathLatencyContext),
					Units:     "milliseconds",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 23,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStoragePathLatencyReadMetric, Name: hostStoragePathLatencyReadDim},
						{Selector: hostStoragePathLatencyWriteMetric, Name: hostStoragePathLatencyWriteDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostStoragePathCommandEventsContext),
					Title:     "ESXi Host storage path command events",
					Context:   v2ChartTemplateID(hostStoragePathCommandEventsContext),
					Units:     "events",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 24,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStoragePathCommandEventsAbortedMetric, Name: hostStoragePathCommandEventsAbortedDim},
						{Selector: hostStoragePathCommandEventsBusResetsMetric, Name: hostStoragePathCommandEventsBusResetsDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostStoragePathThroughputContext),
					Title:     "ESXi Host storage path throughput usage",
					Context:   v2ChartTemplateID(hostStoragePathThroughputContext),
					Units:     "KiB/s",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 25,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStoragePathThroughputMetric, Name: hostStoragePathThroughputDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostStoragePathThroughputContentionContext),
					Title:     "ESXi Host storage path throughput contention",
					Context:   v2ChartTemplateID(hostStoragePathThroughputContentionContext),
					Units:     "milliseconds",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 26,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStoragePathThroughputContentionMetric, Name: hostStoragePathThroughputContentionDim},
					},
				},
			},
		},
		{
			Family: "hosts storage paths",
			Metrics: []string{
				hostStoragePathMaxLatencyMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id"}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(hostStoragePathMaxLatencyContext),
					Title:     "ESXi Host storage path maximum latency",
					Context:   v2ChartTemplateID(hostStoragePathMaxLatencyContext),
					Units:     "milliseconds",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 27,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostStoragePathMaxLatencyMetric, Name: hostStoragePathMaxLatencyDim},
					},
				},
			},
		},
	}
}

func hostCPUInstancePerformanceChartGroups() charttpl.Group {
	return charttpl.Group{
		Family:  "hosts cpu instances",
		Metrics: hostCPUInstancePerformanceOptionalMetricNames(),
		ChartDefaults: &charttpl.ChartDefaults{
			Instances: &charttpl.Instances{ByLabels: []string{"id", hostCPUInstanceInstanceLabel}},
		},
		Charts: []charttpl.Chart{
			{
				ID:        v2ChartTemplateID(hostCPUInstanceUtilizationContext),
				Title:     "ESXi Host CPU instance utilization",
				Context:   v2ChartTemplateID(hostCPUInstanceUtilizationContext),
				Units:     "percentage",
				Algorithm: collectorapi.Absolute.String(),
				Type:      collectorapi.Line.String(),
				Priority:  prioHostDiskMaxLatency + 28,
				Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
				Dimensions: []charttpl.Dimension{
					{
						Selector: hostCPUInstanceUtilizationUsageMetric,
						Name:     hostCPUInstanceUtilizationUsageDim,
						Options:  &charttpl.DimensionOptions{Divisor: 100},
					},
					{
						Selector: hostCPUInstanceUtilizationUtilizationMetric,
						Name:     hostCPUInstanceUtilizationUtilizationDim,
						Options:  &charttpl.DimensionOptions{Divisor: 100},
					},
					{
						Selector: hostCPUInstanceUtilizationCoreMetric,
						Name:     hostCPUInstanceUtilizationCoreDim,
						Options:  &charttpl.DimensionOptions{Divisor: 100},
					},
				},
			},
			{
				ID:        v2ChartTemplateID(hostCPUInstanceTimeContext),
				Title:     "ESXi Host CPU instance time",
				Context:   v2ChartTemplateID(hostCPUInstanceTimeContext),
				Units:     "milliseconds",
				Algorithm: collectorapi.Absolute.String(),
				Type:      collectorapi.Line.String(),
				Priority:  prioHostDiskMaxLatency + 29,
				Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
				Dimensions: []charttpl.Dimension{
					{Selector: hostCPUInstanceTimeUsedMetric, Name: hostCPUInstanceTimeUsedDim},
					{Selector: hostCPUInstanceTimeIdleMetric, Name: hostCPUInstanceTimeIdleDim},
				},
			},
		},
	}
}

func powerMetricsChartGroups() []charttpl.Group {
	return []charttpl.Group{
		{
			Family: "hosts power",
			Metrics: []string{
				hostPowerUsagePowerMetric,
				hostPowerUsageCapMetric,
				hostPowerCapacityUsageUsedMetric,
				hostPowerCapacityUsageUsableMetric,
				hostPowerCapacityUsageIdleMetric,
				hostPowerCapacityUsageSystemMetric,
				hostPowerCapacityUsageVMMetric,
				hostPowerCapacityUtilizationMetric,
				hostEnergyUsageMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id"}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(hostPowerUsageContext),
					Title:     "ESXi Host power usage",
					Context:   v2ChartTemplateID(hostPowerUsageContext),
					Units:     "watts",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 30,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostPowerUsagePowerMetric, Name: hostPowerUsagePowerDim},
						{Selector: hostPowerUsageCapMetric, Name: hostPowerUsageCapDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostPowerCapacityUsageContext),
					Title:     "ESXi Host power capacity usage",
					Context:   v2ChartTemplateID(hostPowerCapacityUsageContext),
					Units:     "watts",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 31,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostPowerCapacityUsageUsedMetric, Name: hostPowerCapacityUsageUsedDim},
						{Selector: hostPowerCapacityUsageUsableMetric, Name: hostPowerCapacityUsageUsableDim},
						{Selector: hostPowerCapacityUsageIdleMetric, Name: hostPowerCapacityUsageIdleDim},
						{Selector: hostPowerCapacityUsageSystemMetric, Name: hostPowerCapacityUsageSystemDim},
						{Selector: hostPowerCapacityUsageVMMetric, Name: hostPowerCapacityUsageVMDim},
					},
				},
				{
					ID:        v2ChartTemplateID(hostPowerCapacityUtilizationContext),
					Title:     "ESXi Host power capacity utilization",
					Context:   v2ChartTemplateID(hostPowerCapacityUtilizationContext),
					Units:     "percentage",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 32,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{
							Selector: hostPowerCapacityUtilizationMetric,
							Name:     hostPowerCapacityUtilizationDim,
							Options:  &charttpl.DimensionOptions{Divisor: 100},
						},
					},
				},
				{
					ID:        v2ChartTemplateID(hostEnergyUsageContext),
					Title:     "ESXi Host energy usage",
					Context:   v2ChartTemplateID(hostEnergyUsageContext),
					Units:     "joules",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioHostDiskMaxLatency + 33,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: hostEnergyUsageMetric, Name: hostEnergyUsageDim},
					},
				},
			},
		},
		{
			Family: "vms power",
			Metrics: []string{
				vmPowerUsagePowerMetric,
				vmEnergyUsageMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id"}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(vmPowerUsageContext),
					Title:     "Virtual Machine power usage",
					Context:   v2ChartTemplateID(vmPowerUsageContext),
					Units:     "watts",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioVmNetworkDrops + 4,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vmPowerUsagePowerMetric, Name: vmPowerUsagePowerDim},
					},
				},
				{
					ID:        v2ChartTemplateID(vmEnergyUsageContext),
					Title:     "Virtual Machine energy usage",
					Context:   v2ChartTemplateID(vmEnergyUsageContext),
					Units:     "joules",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioVmNetworkDrops + 5,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vmEnergyUsageMetric, Name: vmEnergyUsageDim},
					},
				},
			},
		},
	}
}

func vsanChartGroups() []charttpl.Group {
	return []charttpl.Group{
		{
			Family: "vSAN clusters space",
			Metrics: []string{
				vsanClusterSpaceUsageTotalMetric,
				vsanClusterSpaceUsageFreeMetric,
				vsanClusterSpaceUsageUsedMetric,
				vsanClusterSpaceUtilizationUsedMetric,
				vsanClusterHealthStatusGreenMetric,
				vsanClusterHealthStatusYellowMetric,
				vsanClusterHealthStatusRedMetric,
				vsanClusterHealthStatusUnknownMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id"}},
			},
			Charts: []charttpl.Chart{
				{
					ID:        v2ChartTemplateID(vsanClusterSpaceUtilizationContext),
					Title:     "vSAN Cluster space utilization",
					Context:   v2ChartTemplateID(vsanClusterSpaceUtilizationContext),
					Units:     "percentage",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioInventoryObjects + 16,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vsanClusterSpaceUtilizationUsedMetric, Name: "used", Options: &charttpl.DimensionOptions{Divisor: 100}},
					},
				},
				{
					ID:        v2ChartTemplateID(vsanClusterSpaceUsageContext),
					Title:     "vSAN Cluster space usage",
					Context:   v2ChartTemplateID(vsanClusterSpaceUsageContext),
					Units:     "bytes",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Stacked.String(),
					Priority:  prioInventoryObjects + 17,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vsanClusterSpaceUsageUsedMetric, Name: vsanSpaceUsageUsedDim},
						{Selector: vsanClusterSpaceUsageFreeMetric, Name: vsanSpaceUsageFreeDim},
						{Selector: vsanClusterSpaceUsageTotalMetric, Name: vsanSpaceUsageTotalDim, Options: &charttpl.DimensionOptions{Hidden: true}},
					},
				},
				{
					ID:        v2ChartTemplateID(vsanClusterHealthStatusContext),
					Title:     "vSAN Cluster health status",
					Context:   v2ChartTemplateID(vsanClusterHealthStatusContext),
					Units:     "status",
					Algorithm: collectorapi.Absolute.String(),
					Type:      collectorapi.Line.String(),
					Priority:  prioInventoryObjects + 18,
					Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
					Dimensions: []charttpl.Dimension{
						{Selector: vsanClusterHealthStatusGreenMetric, Name: vsanHealthStatusGreenDim},
						{Selector: vsanClusterHealthStatusYellowMetric, Name: vsanHealthStatusYellowDim},
						{Selector: vsanClusterHealthStatusRedMetric, Name: vsanHealthStatusRedDim},
						{Selector: vsanClusterHealthStatusUnknownMetric, Name: vsanHealthStatusUnknownDim},
					},
				},
			},
		},
		{
			Family: "vSAN clusters performance",
			Metrics: []string{
				vsanClusterOperationsReadMetric,
				vsanClusterOperationsWriteMetric,
				vsanClusterThroughputReadMetric,
				vsanClusterThroughputWriteMetric,
				vsanClusterLatencyReadMetric,
				vsanClusterLatencyWriteMetric,
				vsanClusterCongestionsMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id"}},
			},
			Charts: []charttpl.Chart{
				vsanOperationsChart(vsanClusterOperationsContext, "vSAN Cluster operations", prioInventoryObjects+19, vsanClusterOperationsReadMetric, vsanClusterOperationsWriteMetric),
				vsanThroughputChart(vsanClusterThroughputContext, "vSAN Cluster throughput", prioInventoryObjects+20, vsanClusterThroughputReadMetric, vsanClusterThroughputWriteMetric),
				vsanLatencyChart(vsanClusterLatencyContext, "vSAN Cluster latency", prioInventoryObjects+21, vsanClusterLatencyReadMetric, vsanClusterLatencyWriteMetric),
				vsanSingleMetricChart(vsanClusterCongestionsContext, "vSAN Cluster congestions", "congestions/s", prioInventoryObjects+22, vsanClusterCongestionsMetric, vsanCongestionsDim),
			},
		},
		{
			Family: "vSAN hosts performance",
			Metrics: []string{
				vsanHostOperationsReadMetric,
				vsanHostOperationsWriteMetric,
				vsanHostThroughputReadMetric,
				vsanHostThroughputWriteMetric,
				vsanHostLatencyReadMetric,
				vsanHostLatencyWriteMetric,
				vsanHostCongestionsMetric,
				vsanHostCacheHitRateMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id"}},
			},
			Charts: []charttpl.Chart{
				vsanOperationsChart(vsanHostOperationsContext, "vSAN Host operations", prioInventoryObjects+23, vsanHostOperationsReadMetric, vsanHostOperationsWriteMetric),
				vsanThroughputChart(vsanHostThroughputContext, "vSAN Host throughput", prioInventoryObjects+24, vsanHostThroughputReadMetric, vsanHostThroughputWriteMetric),
				vsanLatencyChart(vsanHostLatencyContext, "vSAN Host latency", prioInventoryObjects+25, vsanHostLatencyReadMetric, vsanHostLatencyWriteMetric),
				vsanSingleMetricChart(vsanHostCongestionsContext, "vSAN Host congestions", "congestions/s", prioInventoryObjects+26, vsanHostCongestionsMetric, vsanCongestionsDim),
				vsanSingleMetricChart(vsanHostCacheHitRateContext, "vSAN Host client cache hit rate", "percentage", prioInventoryObjects+27, vsanHostCacheHitRateMetric, vsanCacheHitRateDim),
			},
		},
		{
			Family: "vSAN VMs performance",
			Metrics: []string{
				vsanVMOperationsReadMetric,
				vsanVMOperationsWriteMetric,
				vsanVMThroughputReadMetric,
				vsanVMThroughputWriteMetric,
				vsanVMLatencyReadMetric,
				vsanVMLatencyWriteMetric,
			},
			ChartDefaults: &charttpl.ChartDefaults{
				Instances: &charttpl.Instances{ByLabels: []string{"id"}},
			},
			Charts: []charttpl.Chart{
				vsanOperationsChart(vsanVMOperationsContext, "vSAN Virtual Machine operations", prioInventoryObjects+28, vsanVMOperationsReadMetric, vsanVMOperationsWriteMetric),
				vsanThroughputChart(vsanVMThroughputContext, "vSAN Virtual Machine throughput", prioInventoryObjects+29, vsanVMThroughputReadMetric, vsanVMThroughputWriteMetric),
				vsanLatencyChart(vsanVMLatencyContext, "vSAN Virtual Machine latency", prioInventoryObjects+30, vsanVMLatencyReadMetric, vsanVMLatencyWriteMetric),
			},
		},
	}
}

func vsanOperationsChart(ctx, title string, priority int, readMetric, writeMetric string) charttpl.Chart {
	return charttpl.Chart{
		ID:        v2ChartTemplateID(ctx),
		Title:     title,
		Context:   v2ChartTemplateID(ctx),
		Units:     "operations/s",
		Algorithm: collectorapi.Absolute.String(),
		Type:      collectorapi.Line.String(),
		Priority:  priority,
		Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
		Dimensions: []charttpl.Dimension{
			{Selector: readMetric, Name: vsanOperationsReadDim},
			{Selector: writeMetric, Name: vsanOperationsWriteDim},
		},
	}
}

func vsanThroughputChart(ctx, title string, priority int, readMetric, writeMetric string) charttpl.Chart {
	return charttpl.Chart{
		ID:        v2ChartTemplateID(ctx),
		Title:     title,
		Context:   v2ChartTemplateID(ctx),
		Units:     "bytes/s",
		Algorithm: collectorapi.Absolute.String(),
		Type:      collectorapi.Area.String(),
		Priority:  priority,
		Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
		Dimensions: []charttpl.Dimension{
			{Selector: readMetric, Name: vsanThroughputReadDim},
			{Selector: writeMetric, Name: vsanThroughputWriteDim},
		},
	}
}

func vsanLatencyChart(ctx, title string, priority int, readMetric, writeMetric string) charttpl.Chart {
	return charttpl.Chart{
		ID:        v2ChartTemplateID(ctx),
		Title:     title,
		Context:   v2ChartTemplateID(ctx),
		Units:     "microseconds",
		Algorithm: collectorapi.Absolute.String(),
		Type:      collectorapi.Line.String(),
		Priority:  priority,
		Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
		Dimensions: []charttpl.Dimension{
			{Selector: readMetric, Name: vsanLatencyReadDim},
			{Selector: writeMetric, Name: vsanLatencyWriteDim},
		},
	}
}

func vsanSingleMetricChart(ctx, title, units string, priority int, metric, dim string) charttpl.Chart {
	return charttpl.Chart{
		ID:        v2ChartTemplateID(ctx),
		Title:     title,
		Context:   v2ChartTemplateID(ctx),
		Units:     units,
		Algorithm: collectorapi.Absolute.String(),
		Type:      collectorapi.Line.String(),
		Priority:  priority,
		Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
		Dimensions: []charttpl.Dimension{
			{Selector: metric, Name: dim},
		},
	}
}

func appendChartToGroup(group *charttpl.Group, chart *collectorapi.Chart) {
	out := charttpl.Chart{
		ID:        v2ChartTemplateID(chart.Ctx),
		Title:     chart.Title,
		Context:   v2ChartTemplateID(chart.Ctx),
		Units:     chart.Units,
		Algorithm: chartAlgorithm(chart),
		Type:      chart.Type.String(),
		Priority:  chart.Priority,
		Lifecycle: &charttpl.Lifecycle{ExpireAfterCycles: failedUpdatesLimit},
	}

	for _, dim := range chart.Dims {
		name := dim.Name
		metricName := v2MetricName(chart.Ctx, name)
		if !hasMetric(group.Metrics, metricName) {
			group.Metrics = append(group.Metrics, metricName)
		}
		out.Dimensions = append(out.Dimensions, charttpl.Dimension{
			Selector: metricName,
			Name:     name,
			Options:  dimensionOptions(dim),
		})
	}
	group.Charts = append(group.Charts, out)
}

func hasMetric(metrics []string, name string) bool {
	for _, metric := range metrics {
		if metric == name {
			return true
		}
	}
	return false
}

func chartAlgorithm(chart *collectorapi.Chart) string {
	if len(chart.Dims) == 0 {
		return collectorapi.Absolute.String()
	}
	return chart.Dims[0].Algo.String()
}

func dimensionOptions(dim *collectorapi.Dim) *charttpl.DimensionOptions {
	if dim.Mul == 0 && dim.Div == 0 && !dim.Hidden && !dim.Float {
		return nil
	}
	return &charttpl.DimensionOptions{
		Multiplier: dim.Mul,
		Divisor:    dim.Div,
		Hidden:     dim.Hidden,
		Float:      dim.Float,
	}
}
