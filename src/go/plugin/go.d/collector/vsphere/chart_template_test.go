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
