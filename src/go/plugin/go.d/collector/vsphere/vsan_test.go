// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/performance"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	scrapepkg "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/scrape"
)

func TestCollector_VSANMetricsDefaultOff(t *testing.T) {
	collr := newVSANTestCollector(false)
	cycle := mustCycleController(t, collr.MetricStore())
	cycle.BeginCycle()
	collr.writeMetrics(nil)
	require.NoError(t, cycle.CommitCycleSuccess())

	reader := collr.MetricStore().Read(metrix.ReadRaw())
	require.Zero(t, countMetricSeries(reader, vsanClusterSpaceUsageTotalMetric))
	require.Zero(t, countMetricSeries(reader, vsanHostOperationsReadMetric))
	require.Zero(t, countMetricSeries(reader, vsanVMOperationsReadMetric))
}

func TestCollector_VSANMetricsOptInEmitsCharts(t *testing.T) {
	collr := newVSANTestCollector(true)
	cycle := mustCycleController(t, collr.MetricStore())
	cycle.BeginCycle()
	collr.writeMetrics(nil)
	require.NoError(t, cycle.CommitCycleSuccess())

	cluster := collr.resources.Clusters.Get("domain-c1")
	host := collr.resources.Hosts.Get("host-1")
	vm := collr.resources.VMs.Get("vm-1")
	reader := collr.MetricStore().Read(metrix.ReadRaw())

	clusterLabels := labelsFromMetrix(collr.vsanClusterLabels(cluster))
	requireMetricValue(t, reader, vsanClusterSpaceUsageTotalMetric, clusterLabels, 1000)
	requireMetricValue(t, reader, vsanClusterSpaceUsageFreeMetric, clusterLabels, 400)
	requireMetricValue(t, reader, vsanClusterSpaceUsageUsedMetric, clusterLabels, 600)
	requireMetricValue(t, reader, vsanClusterSpaceUtilizationUsedMetric, clusterLabels, 6000)
	requireMetricValue(t, reader, vsanClusterHealthStatusGreenMetric, clusterLabels, 1)
	requireMetricValue(t, reader, vsanClusterOperationsReadMetric, clusterLabels, 10)
	requireMetricValue(t, reader, vsanClusterThroughputReadMetric, clusterLabels, 11)
	requireMetricValue(t, reader, vsanClusterLatencyReadMetric, clusterLabels, 12)
	requireMetricValue(t, reader, vsanClusterCongestionsMetric, clusterLabels, 13)

	hostLabels := labelsFromMetrix(collr.vsanHostLabels(host))
	requireMetricValue(t, reader, vsanHostOperationsReadMetric, hostLabels, 20)
	requireMetricValue(t, reader, vsanHostThroughputReadMetric, hostLabels, 21)
	requireMetricValue(t, reader, vsanHostLatencyReadMetric, hostLabels, 22)
	requireMetricValue(t, reader, vsanHostCongestionsMetric, hostLabels, 23)
	requireMetricValue(t, reader, vsanHostCacheHitRateMetric, hostLabels, 95)

	vmLabels := labelsFromMetrix(collr.vsanVMLabels(vm))
	requireMetricValue(t, reader, vsanVMOperationsReadMetric, vmLabels, 30)
	requireMetricValue(t, reader, vsanVMThroughputReadMetric, vmLabels, 31)
	requireMetricValue(t, reader, vsanVMLatencyReadMetric, vmLabels, 32)

	createdCharts, createdDims := v2CreatedChartsAndDims(buildV2PlanForTest(t, collr))
	chartID := findChartIDByLabelsAndContext(t, createdCharts, vsanClusterSpaceUsageContext, map[string]string{"id": cluster.ID})
	require.Contains(t, createdDims[chartID], vsanSpaceUsageUsedDim)
	hostChartID := findChartIDByLabelsAndContext(t, createdCharts, vsanHostOperationsContext, map[string]string{"id": host.ID})
	require.Contains(t, createdDims[hostChartID], vsanOperationsReadDim)
	vmChartID := findChartIDByLabelsAndContext(t, createdCharts, vsanVMOperationsContext, map[string]string{"id": vm.ID})
	require.Contains(t, createdDims[vmChartID], vsanOperationsReadDim)
}

func TestCollector_VSANSpaceUsageEdgeCases(t *testing.T) {
	tests := map[string]struct {
		space scrapepkg.VSANSpaceUsage
		want  map[string]int64
	}{
		"zero total": {
			space: scrapepkg.VSANSpaceUsage{Total: 0, Free: 0},
			want: map[string]int64{
				vsanClusterSpaceUsageUsedMetric:       0,
				vsanClusterSpaceUtilizationUsedMetric: 0,
			},
		},
		"free greater than total": {
			space: scrapepkg.VSANSpaceUsage{Total: 100, Free: 200},
			want: map[string]int64{
				vsanClusterSpaceUsageUsedMetric:       0,
				vsanClusterSpaceUtilizationUsedMetric: 0,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := newVSANTestCollector(true)
			collr.vsanMetrics.Space["domain-c1"] = tc.space
			cycle := mustCycleController(t, collr.MetricStore())
			cycle.BeginCycle()
			collr.writeMetrics(nil)
			require.NoError(t, cycle.CommitCycleSuccess())

			labels := labelsFromMetrix(collr.vsanClusterLabels(collr.resources.Clusters.Get("domain-c1")))
			reader := collr.MetricStore().Read(metrix.ReadRaw())
			for metric, want := range tc.want {
				requireMetricValue(t, reader, metric, labels, want)
			}
		})
	}
}

func TestCollector_CollectVSANUsesSelectors(t *testing.T) {
	collr := New()
	collr.URL = "https://127.0.0.1"
	collr.Username = "user"
	collr.Password = "pass"
	collr.CollectVSAN = true
	collr.VSANClustersInclude = []string{"vsan_uuid:cluster-uuid-2"}
	collr.VSANHostsInclude = []string{"vsan_node_uuid:host-uuid-2"}
	collr.VSANVMsInclude = []string{"instance_uuid:vm-uuid-2"}
	collr.resources = newVSANFilterTestResources()
	scraper := &capturingVSANScraper{}
	collr.scraper = scraper

	require.NoError(t, collr.validateConfig())
	collr.collectVSAN()

	require.NotNil(t, collr.vsanMetrics)
	require.Contains(t, scraper.clusters, "domain-c2")
	require.NotContains(t, scraper.clusters, "domain-c1")
	require.Contains(t, scraper.hosts, "host-2")
	require.NotContains(t, scraper.hosts, "host-1")
	require.Contains(t, scraper.vms, "vm-2")
	require.NotContains(t, scraper.vms, "vm-1")
	require.Len(t, scraper.clusters, 1)
	require.Len(t, scraper.hosts, 1)
	require.Len(t, scraper.vms, 1)
}

func newVSANTestCollector(enabled bool) *Collector {
	collr := New()
	collr.CollectVSAN = enabled
	collr.resources = &rs.Resources{
		Clusters: rs.Clusters{
			"domain-c1": &rs.Cluster{
				ID:          "domain-c1",
				Name:        "Cluster1",
				Hier:        rs.ClusterHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}},
				VSANEnabled: true,
				VSANUUID:    "cluster-uuid",
			},
		},
		Hosts: rs.Hosts{
			"host-1": &rs.Host{
				ID:           "host-1",
				Name:         "Host1",
				Hier:         rs.HostHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c1", Name: "Cluster1"}},
				VSANNodeUUID: "host-uuid",
			},
		},
		VMs: rs.VMs{
			"vm-1": &rs.VM{
				ID:           "vm-1",
				Name:         "VM1",
				Hier:         rs.VMHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c1", Name: "Cluster1"}, Host: rs.HierarchyValue{ID: "host-1", Name: "Host1"}},
				InstanceUUID: "vm-uuid",
			},
		},
	}
	collr.vsanMetrics = &scrapepkg.VSANMetrics{
		Clusters: map[string]scrapepkg.VSANEntityMetrics{
			"domain-c1": {"read_operations": 10, "read_throughput": 11, "read_latency": 12, "congestions": 13},
		},
		Hosts: map[string]scrapepkg.VSANEntityMetrics{
			"host-1": {"read_operations": 20, "read_throughput": 21, "read_latency": 22, "congestions": 23, "cache_hit_rate": 95},
		},
		VMs: map[string]scrapepkg.VSANEntityMetrics{
			"vm-1": {"read_operations": 30, "read_throughput": 31, "read_latency": 32},
		},
		Space: map[string]scrapepkg.VSANSpaceUsage{
			"domain-c1": {Total: 1000, Free: 400},
		},
		Health: map[string]string{
			"domain-c1": "green",
		},
	}
	return collr
}

func newVSANFilterTestResources() *rs.Resources {
	return &rs.Resources{
		Clusters: rs.Clusters{
			"domain-c1": &rs.Cluster{
				ID:          "domain-c1",
				Name:        "Cluster1",
				Hier:        rs.ClusterHierarchy{DC: rs.HierarchyValue{Name: "DC1"}},
				VSANEnabled: true,
				VSANUUID:    "cluster-uuid-1",
			},
			"domain-c2": &rs.Cluster{
				ID:          "domain-c2",
				Name:        "Cluster2",
				Hier:        rs.ClusterHierarchy{DC: rs.HierarchyValue{Name: "DC1"}},
				VSANEnabled: true,
				VSANUUID:    "cluster-uuid-2",
			},
		},
		Hosts: rs.Hosts{
			"host-1": &rs.Host{
				ID:           "host-1",
				Name:         "Host1",
				Hier:         rs.HostHierarchy{DC: rs.HierarchyValue{Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c1", Name: "Cluster1"}},
				VSANNodeUUID: "host-uuid-1",
			},
			"host-2": &rs.Host{
				ID:           "host-2",
				Name:         "Host2",
				Hier:         rs.HostHierarchy{DC: rs.HierarchyValue{Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c2", Name: "Cluster2"}},
				VSANNodeUUID: "host-uuid-2",
			},
			"host-3": &rs.Host{
				ID:           "host-3",
				Name:         "Host3",
				Hier:         rs.HostHierarchy{DC: rs.HierarchyValue{Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c2", Name: "Cluster2"}},
				VSANNodeUUID: "host-uuid-3",
			},
		},
		VMs: rs.VMs{
			"vm-1": &rs.VM{
				ID:           "vm-1",
				Name:         "VM1",
				Hier:         rs.VMHierarchy{DC: rs.HierarchyValue{Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c1", Name: "Cluster1"}, Host: rs.HierarchyValue{ID: "host-1", Name: "Host1"}},
				InstanceUUID: "vm-uuid-1",
			},
			"vm-2": &rs.VM{
				ID:           "vm-2",
				Name:         "VM2",
				Hier:         rs.VMHierarchy{DC: rs.HierarchyValue{Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c2", Name: "Cluster2"}, Host: rs.HierarchyValue{ID: "host-2", Name: "Host2"}},
				InstanceUUID: "vm-uuid-2",
			},
			"vm-3": &rs.VM{
				ID:           "vm-3",
				Name:         "VM3",
				Hier:         rs.VMHierarchy{DC: rs.HierarchyValue{Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c2", Name: "Cluster2"}, Host: rs.HierarchyValue{ID: "host-3", Name: "Host3"}},
				InstanceUUID: "vm-uuid-3",
			},
		},
	}
}

type capturingVSANScraper struct {
	clusters rs.Clusters
	hosts    rs.Hosts
	vms      rs.VMs
}

func (s *capturingVSANScraper) ScrapeHosts(rs.Hosts) []performance.EntityMetric {
	return nil
}

func (s *capturingVSANScraper) ScrapeVMs(rs.VMs) []performance.EntityMetric {
	return nil
}

func (s *capturingVSANScraper) ScrapeDatastores(rs.Datastores) []performance.EntityMetric {
	return nil
}

func (s *capturingVSANScraper) ScrapeClusters(rs.Clusters) []performance.EntityMetric {
	return nil
}

func (s *capturingVSANScraper) ScrapeVSAN(clusters rs.Clusters, hosts rs.Hosts, vms rs.VMs) *scrapepkg.VSANMetrics {
	s.clusters = clusters
	s.hosts = hosts
	s.vms = vms
	return &scrapepkg.VSANMetrics{}
}

func labelsFromMetrix(labels []metrix.Label) metrix.Labels {
	out := make(metrix.Labels, len(labels))
	for _, label := range labels {
		out[label.Key] = label.Value
	}
	return out
}
