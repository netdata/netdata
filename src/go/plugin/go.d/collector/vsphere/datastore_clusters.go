// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"sort"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

const (
	defaultMaxDatastoreClusters = 256

	datastoreClusterSpaceUsageContext       = "vsphere.datastore_cluster_space_usage"
	datastoreClusterSpaceUtilizationContext = "vsphere.datastore_cluster_space_utilization"
	datastoreClusterStorageDRSContext       = "vsphere.datastore_cluster_storage_drs_status"
	datastoreClusterOverallStatusContext    = "vsphere.datastore_cluster_overall_status"

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

func datastoreClusterOptionalMetricNames() []string {
	return []string{
		datastoreClusterSpaceUsageCapacityMetric,
		datastoreClusterSpaceUsageFreeMetric,
		datastoreClusterSpaceUsageUsedMetric,
		datastoreClusterSpaceUtilizationUsedMetric,
		datastoreClusterStorageDRSEnabledMetric,
		datastoreClusterStorageDRSDisabledMetric,
		datastoreClusterOverallStatusGreenMetric,
		datastoreClusterOverallStatusRedMetric,
		datastoreClusterOverallStatusYellowMetric,
		datastoreClusterOverallStatusGrayMetric,
	}
}

func (c *Collector) writeDatastoreClusterMetrics(meter metrix.SnapshotMeter) {
	if !c.CollectDatastoreClusters || c.resources == nil || c.MaxDatastoreClusters < 1 {
		return
	}

	m := c.datastoreClusterMatcher
	if m == nil {
		m = matcher.TRUE()
	}

	count := 0
	for _, pod := range sortedStoragePods(c.resources.StoragePods) {
		if !datastoreClusterMatches(m, pod) {
			continue
		}
		if count >= c.MaxDatastoreClusters {
			return
		}
		count++

		labels := meter.LabelSet(c.datastoreClusterLabels(pod)...)
		c.observeGauge(datastoreClusterSpaceUsageCapacityMetric, pod.Capacity, labels)
		c.observeGauge(datastoreClusterSpaceUsageFreeMetric, pod.FreeSpace, labels)
		used := max(pod.Capacity-pod.FreeSpace, 0)
		c.observeGauge(datastoreClusterSpaceUsageUsedMetric, used, labels)
		if pod.Capacity > 0 {
			c.observeGauge(datastoreClusterSpaceUtilizationUsedMetric, int64(float64(used)/float64(pod.Capacity)*10000), labels)
		} else {
			c.observeGauge(datastoreClusterSpaceUtilizationUsedMetric, 0, labels)
		}
		c.observeGauge(datastoreClusterStorageDRSEnabledMetric, boolInt(pod.StorageDRSEnabled), labels)
		c.observeGauge(datastoreClusterStorageDRSDisabledMetric, boolInt(!pod.StorageDRSEnabled), labels)
		status := pod.OverallStatus
		if status == "" {
			status = "gray"
		}
		c.observeGauge(datastoreClusterOverallStatusGreenMetric, boolInt(status == "green"), labels)
		c.observeGauge(datastoreClusterOverallStatusRedMetric, boolInt(status == "red"), labels)
		c.observeGauge(datastoreClusterOverallStatusYellowMetric, boolInt(status == "yellow"), labels)
		c.observeGauge(datastoreClusterOverallStatusGrayMetric, boolInt(status == "gray"), labels)
	}
}

func (c *Collector) observeGauge(name string, value int64, labels metrix.LabelSet) {
	if gauge := c.mx.gauge(name); gauge != nil {
		gauge.Observe(metrix.SampleValue(value), labels)
	}
}

func (c *Collector) datastoreClusterLabels(pod *rs.StoragePod) []metrix.Label {
	labels := datastoreClusterLabels(pod)
	labels = append(labels, c.resourceEnrichmentLabels(pod.ID)...)
	return labels
}

func datastoreClusterLabels(pod *rs.StoragePod) []metrix.Label {
	return []metrix.Label{
		{Key: "id", Value: pod.ID},
		{Key: "datacenter", Value: pod.Hier.DC.Name},
		{Key: datastoreClusterNameLabel, Value: pod.Name},
	}
}

func datastoreClusterMatches(m matcher.Matcher, pod *rs.StoragePod) bool {
	return m.MatchString(datastoreClusterPath(pod)) || m.MatchString(pod.Name) || m.MatchString(pod.ID)
}

func datastoreClusterPath(pod *rs.StoragePod) string {
	if pod.Hier.DC.Name == "" {
		return "/" + pod.Name
	}
	return "/" + pod.Hier.DC.Name + "/" + pod.Name
}

func sortedStoragePods(pods rs.StoragePods) []*rs.StoragePod {
	out := make([]*rs.StoragePod, 0, len(pods))
	for _, pod := range pods {
		out = append(out, pod)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func boolInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
