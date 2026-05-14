// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func TestCollector_Init_ReturnsFalseIfInvalidDatastoreClusterConfig(t *testing.T) {
	collr := New()
	collr.URL = "https://vcenter.local"
	collr.Username = "user"
	collr.Password = "pass"
	collr.CollectDatastoreClusters = true
	collr.MaxDatastoreClusters = -1

	require.ErrorContains(t, collr.Init(context.Background()), "max_datastore_clusters must be greater than zero")

	collr.MaxDatastoreClusters = 1
	collr.DatastoreClustersInclude = []string{"["}
	require.ErrorContains(t, collr.Init(context.Background()), "datastore_cluster_include has invalid pattern")
}

func TestCollector_DatastoreClustersDefaultOff(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}
	setOnlyTestStoragePods(collr, []*rs.StoragePod{testStoragePod("group-p1", "DC0_POD0", 1000, 400, true)})

	require.NotEmpty(t, collectMapForTest(t, collr))

	require.Zero(t, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), datastoreClusterSpaceUsageCapacityMetric))
}

func TestCollector_DatastoreClustersOptInEmitsCharts(t *testing.T) {
	collr, _, teardown := prepareVSphereSim(t)
	defer teardown()
	collr.CollectDatastoreClusters = true

	require.NoError(t, collr.Init(context.Background()))
	collr.scraper = mockScraper{collr.scraper}
	pod := testStoragePod("group-p1", "DC0_POD0", 1000, 400, true)
	pod.Labels = map[string]string{"vsphere_tag_environment": "prod"}
	setOnlyTestStoragePods(collr, []*rs.StoragePod{pod})

	require.NotEmpty(t, collectMapForTest(t, collr))

	labels := datastoreClusterLabelsMap(pod)
	labels["vsphere_tag_environment"] = "prod"
	reader := collr.MetricStore().Read(metrix.ReadRaw())
	requireMetricValue(t, reader, datastoreClusterSpaceUsageCapacityMetric, labels, 1000)
	requireMetricValue(t, reader, datastoreClusterSpaceUsageFreeMetric, labels, 400)
	requireMetricValue(t, reader, datastoreClusterSpaceUsageUsedMetric, labels, 600)
	requireMetricValue(t, reader, datastoreClusterSpaceUtilizationUsedMetric, labels, 6000)
	requireMetricValue(t, reader, datastoreClusterStorageDRSEnabledMetric, labels, 1)
	requireMetricValue(t, reader, datastoreClusterStorageDRSDisabledMetric, labels, 0)
	requireMetricValue(t, reader, datastoreClusterOverallStatusGreenMetric, labels, 1)
	requireMetricValue(t, reader, datastoreClusterOverallStatusRedMetric, labels, 0)
	requireMetricValue(t, reader, datastoreClusterOverallStatusYellowMetric, labels, 0)
	requireMetricValue(t, reader, datastoreClusterOverallStatusGrayMetric, labels, 0)

	createdCharts, createdDims := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))
	chartID := findChartIDByLabelsAndContext(t, createdCharts, datastoreClusterSpaceUsageContext, map[string]string{"id": pod.ID})
	require.Equal(t, pod.Name, createdCharts[chartID].Labels[datastoreClusterNameLabel])
	require.Contains(t, createdDims[chartID], "capacity")
	statusChartID := findChartIDByLabelsAndContext(t, createdCharts, datastoreClusterOverallStatusContext, map[string]string{"id": pod.ID})
	require.Contains(t, createdDims[statusChartID], "green")
}

func TestCollector_DatastoreClustersSelectorAndCap(t *testing.T) {
	tests := map[string]struct {
		include []string
		max     int
		want    int
	}{
		"selector keeps matching path": {
			include: []string{"/DC0/DC0_POD1"},
			max:     10,
			want:    1,
		},
		"selector keeps matching name": {
			include: []string{"DC0_POD1"},
			max:     10,
			want:    1,
		},
		"cap limits emitted datastore clusters": {
			include: []string{"/*"},
			max:     1,
			want:    1,
		},
		"selector can exclude all datastore clusters": {
			include: []string{"NoSuchPod"},
			max:     10,
			want:    0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr, _, teardown := prepareVSphereSim(t)
			defer teardown()
			collr.CollectDatastoreClusters = true
			collr.DatastoreClustersInclude = tc.include
			collr.MaxDatastoreClusters = tc.max

			require.NoError(t, collr.Init(context.Background()))
			collr.scraper = mockScraper{collr.scraper}
			setOnlyTestStoragePods(collr, []*rs.StoragePod{
				testStoragePod("group-p1", "DC0_POD0", 1000, 400, true),
				testStoragePod("group-p2", "DC0_POD1", 2000, 500, false),
			})

			require.NotEmpty(t, collectMapForTest(t, collr))

			require.Equal(t, tc.want, countMetricSeries(collr.MetricStore().Read(metrix.ReadRaw()), datastoreClusterSpaceUsageCapacityMetric))
		})
	}
}

func setOnlyTestStoragePods(collr *Collector, pods []*rs.StoragePod) {
	collr.resources.StoragePods = make(rs.StoragePods, len(pods))
	for _, pod := range pods {
		collr.resources.StoragePods.Put(pod)
	}
}

func testStoragePod(id, name string, capacity, freeSpace int64, storageDRSEnabled bool) *rs.StoragePod {
	return &rs.StoragePod{
		ID:                id,
		Name:              name,
		Hier:              rs.StoragePodHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC0"}},
		Capacity:          capacity,
		FreeSpace:         freeSpace,
		StorageDRSEnabled: storageDRSEnabled,
		OverallStatus:     "green",
	}
}

func datastoreClusterLabelsMap(pod *rs.StoragePod) metrix.Labels {
	labels := make(metrix.Labels)
	for _, label := range datastoreClusterLabels(pod) {
		labels[label.Key] = label.Value
	}
	return labels
}

func requireMetricValue(t *testing.T, reader metrix.Reader, name string, labels metrix.Labels, want int64) {
	t.Helper()

	got, ok := reader.Value(name, labels)
	require.True(t, ok, name)
	require.EqualValues(t, want, got)
}

func findChartIDByLabelsAndContext(t *testing.T, charts map[string]chartengine.CreateChartAction, context string, labels map[string]string) string {
	t.Helper()

	for chartID, chart := range charts {
		if chart.Meta.Context != context {
			continue
		}
		matches := true
		for key, value := range labels {
			if chart.Labels[key] != value {
				matches = false
				break
			}
		}
		if matches {
			return chartID
		}
	}
	t.Fatalf("expected %s chart with labels %#v", context, labels)
	return ""
}
