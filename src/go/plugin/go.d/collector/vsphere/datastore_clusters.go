// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"
)

const (
	datastoreClusterSpaceUsageCapacityMetric   = "datastore_cluster_space_usage_capacity"
	datastoreClusterSpaceUsageFreeMetric       = "datastore_cluster_space_usage_free"
	datastoreClusterSpaceUsageUsedMetric       = "datastore_cluster_space_usage_used"
	datastoreClusterSpaceUtilizationUsedMetric = "datastore_cluster_space_utilization_used"
	datastoreClusterStorageDRSEnabledMetric    = "datastore_cluster_storage_drs_status_enabled"
	datastoreClusterStorageDRSDisabledMetric   = "datastore_cluster_storage_drs_status_disabled"
	datastoreClusterOverallStatusGreenMetric   = "datastore_cluster_overall_status_green"
	datastoreClusterOverallStatusRedMetric     = "datastore_cluster_overall_status_red"
	datastoreClusterOverallStatusYellowMetric  = "datastore_cluster_overall_status_yellow"
	datastoreClusterOverallStatusGrayMetric    = "datastore_cluster_overall_status_gray"
	datastoreClusterNameLabel                  = "datastore_cluster"
)

func (c *Collector) writeDatastoreClusterMetrics() {
	if !c.CollectDatastoreClusters || c.resources == nil {
		return
	}

	for _, pod := range sortedStoragePods(c.resources.StoragePods) {
		labels := c.labelSet(c.datastoreClusterLabels(pod))
		c.observeGauge(datastoreClusterSpaceUsageCapacityMetric, pod.Capacity, labels)
		c.observeGauge(datastoreClusterSpaceUsageFreeMetric, pod.FreeSpace, labels)
		used := max(pod.Capacity-pod.FreeSpace, 0)
		c.observeGauge(datastoreClusterSpaceUsageUsedMetric, used, labels)
		if pod.Capacity > 0 {
			c.observeGauge(datastoreClusterSpaceUtilizationUsedMetric, int64(float64(used)/float64(pod.Capacity)*scaledPercent), labels)
		} else {
			c.observeGauge(datastoreClusterSpaceUtilizationUsedMetric, 0, labels)
		}
		c.observeGauge(datastoreClusterStorageDRSEnabledMetric, oldmetrix.Bool(pod.StorageDRSEnabled != nil && *pod.StorageDRSEnabled), labels)
		c.observeGauge(datastoreClusterStorageDRSDisabledMetric, oldmetrix.Bool(pod.StorageDRSEnabled != nil && !*pod.StorageDRSEnabled), labels)
		status := pod.OverallStatus
		if status == "" {
			status = "gray"
		}
		c.observeGauge(datastoreClusterOverallStatusGreenMetric, oldmetrix.Bool(status == "green"), labels)
		c.observeGauge(datastoreClusterOverallStatusRedMetric, oldmetrix.Bool(status == "red"), labels)
		c.observeGauge(datastoreClusterOverallStatusYellowMetric, oldmetrix.Bool(status == "yellow"), labels)
		c.observeGauge(datastoreClusterOverallStatusGrayMetric, oldmetrix.Bool(status == "gray"), labels)
	}
}

func (c *Collector) datastoreClusterLabels(pod *rs.StoragePod) []metrix.Label {
	return c.v2MetricLabels(pod.ID, datastoreClusterLabels(pod), pod.Labels)
}

func datastoreClusterLabels(pod *rs.StoragePod) []metrix.Label {
	return []metrix.Label{
		{Key: "datacenter", Value: pod.Hier.DC.Name},
		{Key: datastoreClusterNameLabel, Value: pod.Name},
	}
}
