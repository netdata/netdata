// SPDX-License-Identifier: GPL-3.0-or-later

package scrape

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/vim25/types"
	vsantypes "github.com/vmware/govmomi/vsan/types"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func TestParseVSANEntityMetrics(t *testing.T) {
	tests := map[string]struct {
		raw   []vsantypes.VsanPerfEntityMetricCSV
		specs map[string]vsanMetricSpec
		want  map[string]VSANEntityMetrics
	}{
		"cluster latest values and rates": {
			raw: []vsantypes.VsanPerfEntityMetricCSV{{
				EntityRefId: "cluster-domclient:cluster-uuid",
				Value: []vsantypes.VsanPerfMetricSeriesCSV{
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "iopsRead"},
						Values:   "10,20",
					},
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "throughputRead", MetricsCollectInterval: 20},
						Values:   "100,200",
					},
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "congestion"},
						Values:   "300",
					},
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "ignored"},
						Values:   "999",
					},
				},
			}},
			want: map[string]VSANEntityMetrics{
				"cluster-uuid": {
					"read_operations": 20,
					"read_throughput": 10,
					"congestions":     1,
				},
			},
		},
		"host label set follows vSAN API labels": {
			specs: vsanHostMetricSpecs,
			raw: []vsantypes.VsanPerfEntityMetricCSV{{
				EntityRefId: "host-domclient:host-uuid",
				Value: []vsantypes.VsanPerfMetricSeriesCSV{
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "throughputRead", MetricsCollectInterval: 300},
						Values:   "600",
					},
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "latencyAvgRead"},
						Values:   "700",
					},
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "clientCacheHitRate"},
						Values:   "85",
					},
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "congestion", MetricsCollectInterval: 300},
						Values:   "900",
					},
				},
			}},
			want: map[string]VSANEntityMetrics{
				"host-uuid": {
					"read_throughput": 2,
					"read_latency":    700,
					"cache_hit_rate":  85,
					"congestions":     3,
				},
			},
		},
		"vm label set uses latencyRead latencyWrite": {
			specs: vsanVMMetricSpecs,
			raw: []vsantypes.VsanPerfEntityMetricCSV{{
				EntityRefId: "virtual-machine:vm-uuid",
				Value: []vsantypes.VsanPerfMetricSeriesCSV{
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "throughputWrite", MetricsCollectInterval: 300},
						Values:   "1200",
					},
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "latencyRead"},
						Values:   "11",
					},
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "latencyWrite"},
						Values:   "12",
					},
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "latencyAvgRead"},
						Values:   "13",
					},
				},
			}},
			want: map[string]VSANEntityMetrics{
				"vm-uuid": {
					"write_throughput": 4,
					"read_latency":     11,
					"write_latency":    12,
				},
			},
		},
		"sample info aligns series to the latest sample bucket": {
			raw: []vsantypes.VsanPerfEntityMetricCSV{{
				EntityRefId: "cluster-domclient:cluster-uuid",
				SampleInfo:  "2026-05-09 10:00:00,2026-05-09 10:05:00",
				Value: []vsantypes.VsanPerfMetricSeriesCSV{
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "iopsRead"},
						Values:   "20,",
					},
					{
						MetricId: vsantypes.VsanPerfMetricId{Label: "throughputRead", MetricsCollectInterval: 20},
						Values:   "100,200",
					},
				},
			}},
			want: map[string]VSANEntityMetrics{
				"cluster-uuid": {
					"read_throughput": 10,
				},
			},
		},
		"empty value skipped": {
			raw: []vsantypes.VsanPerfEntityMetricCSV{{
				EntityRefId: "host-domclient:host-uuid",
				Value: []vsantypes.VsanPerfMetricSeriesCSV{{
					MetricId: vsantypes.VsanPerfMetricId{Label: "iopsRead"},
					Values:   "",
				}},
			}},
			want: map[string]VSANEntityMetrics{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			specs := tc.specs
			if specs == nil {
				specs = vsanClusterMetricSpecs
			}
			got, err := parseVSANEntityMetrics(tc.raw, specs)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseVSANEntityMetricsSkipsEntityWithoutUUID(t *testing.T) {
	got, err := parseVSANEntityMetrics([]vsantypes.VsanPerfEntityMetricCSV{
		{
			EntityRefId: "cluster-domclient",
			Value: []vsantypes.VsanPerfMetricSeriesCSV{{
				MetricId: vsantypes.VsanPerfMetricId{Label: "iopsRead"},
				Values:   "1",
			}},
		},
		{
			EntityRefId: "cluster-domclient:cluster-uuid",
			Value: []vsantypes.VsanPerfMetricSeriesCSV{{
				MetricId: vsantypes.VsanPerfMetricId{Label: "iopsRead"},
				Values:   "2",
			}},
		},
	}, vsanClusterMetricSpecs)

	require.NoError(t, err)
	require.Equal(t, map[string]VSANEntityMetrics{
		"cluster-uuid": {"read_operations": 2},
	}, got)
}

func TestVSANQueryIDsUseConcreteDiscoveredEntities(t *testing.T) {
	cluster := &rs.Cluster{
		ID:       "domain-c1",
		VSANUUID: "cluster-uuid",
		Ref:      types.ManagedObjectReference{Type: "ClusterComputeResource", Value: "domain-c1"},
	}
	hosts := rs.Hosts{
		"host-1": {ID: "host-1", VSANNodeUUID: "node-2", Hier: rs.HostHierarchy{Cluster: rs.HierarchyValue{ID: "domain-c1"}}},
		"host-2": {ID: "host-2", VSANNodeUUID: "node-1", Hier: rs.HostHierarchy{Cluster: rs.HierarchyValue{ID: "domain-c1"}}},
		"host-3": {ID: "host-3", VSANNodeUUID: "node-3", Hier: rs.HostHierarchy{Cluster: rs.HierarchyValue{ID: "domain-c2"}}},
	}
	vms := rs.VMs{
		"vm-1": {ID: "vm-1", InstanceUUID: "vm-2", Hier: rs.VMHierarchy{Cluster: rs.HierarchyValue{ID: "domain-c1"}}},
		"vm-2": {ID: "vm-2", InstanceUUID: "vm-1", Hier: rs.VMHierarchy{Cluster: rs.HierarchyValue{ID: "domain-c1"}}},
		"vm-3": {ID: "vm-3", InstanceUUID: "vm-3", Hier: rs.VMHierarchy{Cluster: rs.HierarchyValue{ID: "domain-c2"}}},
	}

	require.Equal(t, []string{"cluster-domclient:cluster-uuid"}, vsanClusterQueryIDs(cluster))
	require.Equal(t, []string{"host-domclient:node-1", "host-domclient:node-2"}, vsanHostQueryIDs(cluster, hosts))
	require.Equal(t, []string{"virtual-machine:vm-1", "virtual-machine:vm-2"}, vsanVMQueryIDs(cluster, vms))
}
