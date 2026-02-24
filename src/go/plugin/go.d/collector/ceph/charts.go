// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioClusterStatus = collectorapi.Priority + iota
	prioClusterHostsCount
	prioClusterMonitorsCount
	prioClusterOSDsCount
	prioClusterOSDsByStatusCount
	prioClusterManagersCount
	prioClusterObjectGatewaysCount
	prioClusterIScsiGatewaysCount
	prioClusterIScsiGatewaysByStatusCount

	prioClusterPhysCapacityUtilization
	prioClusterPhysCapacityUsage
	prioClusterObjectsCount
	prioClusterObjectsByStatusPercent
	prioClusterPoolsCount
	prioClusterPGsCount
	prioClusterPGsByStatusCount
	prioClusterPGsPerOsdCount

	prioClusterClientIO
	prioClusterClientIOPS
	prioClusterClientRecoveryThroughput
	prioClusterScrubStatus

	prioOsdStatus
	prioOsdSpaceUsage
	prioOsdIO
	prioOsdIOPS
	prioOsdLatency

	prioPoolSpaceUtilization
	prioPoolSpaceUsage
	prioPoolObjectsCount
	prioPoolIO
	prioPoolIOPS
)

var clusterCharts = collectorapi.Charts{
	clusterStatusChart.Copy(),
	clusterHostsCountChart.Copy(),
	clusterMonitorsCountChart.Copy(),
	clusterOsdsCountChart.Copy(),
	clusterOsdsByStatusCountChart.Copy(),
	clusterManagersCountChart.Copy(),
	clusterObjectGatewaysCountChart.Copy(),
	clusterIScsiGatewaysCountChart.Copy(),
	clusterIScsiGatewaysByStatusCountChart.Copy(),

	clusterPhysCapacityUtilizationChart.Copy(),
	clusterPhysCapacityUsageChart.Copy(),
	clusterObjectsCountChart.Copy(),
	clusterObjectsByStatusPercentChart.Copy(),
	clusterPoolsCountChart.Copy(),
	clusterPGsCountChart.Copy(),
	clusterPGsByStatusCountChart.Copy(),
	clusterPgsPerOsdCountChart.Copy(),

	clusterClientIOChart.Copy(),
	clusterClientIOPSChart.Copy(),
	clusterRecoveryThroughputChart.Copy(),
	clusterScrubStatusChart.Copy(),
}

var osdChartsTmpl = collectorapi.Charts{
	osdStatusChartTmpl.Copy(),
	osdSpaceUsageChartTmpl.Copy(),
	osdIOChartTmpl.Copy(),
	osdIOPSChartTmpl.Copy(),
	osdLatencyChartTmpl.Copy(),
}

var poolChartsTmpl = collectorapi.Charts{
	poolSpaceUtilizationChartTmpl.Copy(),
	poolSpaceUsageChartTmpl.Copy(),
	poolObjectsCountChartTmpl.Copy(),
	poolIOChartTmpl.Copy(),
	poolIOPSChartTmpl.Copy(),
}

var (
	clusterStatusChart = collectorapi.Chart{
		ID:       "cluster_status",
		Title:    "Ceph Cluster Status",
		Fam:      "status",
		Units:    "status",
		Ctx:      "ceph.cluster_status",
		Type:     collectorapi.Line,
		Priority: prioClusterStatus,
		Dims: collectorapi.Dims{
			{ID: "health_ok", Name: "ok"},
			{ID: "health_err", Name: "err"},
			{ID: "health_warn", Name: "warn"},
		},
	}
	clusterHostsCountChart = collectorapi.Chart{
		ID:       "cluster_hosts_count",
		Title:    "Ceph Cluster Hosts",
		Fam:      "status",
		Units:    "hosts",
		Ctx:      "ceph.cluster_hosts_count",
		Type:     collectorapi.Line,
		Priority: prioClusterHostsCount,
		Dims: collectorapi.Dims{
			{ID: "hosts_num", Name: "hosts"},
		},
	}
	clusterMonitorsCountChart = collectorapi.Chart{
		ID:       "cluster_monitors_count",
		Title:    "Ceph Cluster Monitors",
		Fam:      "status",
		Units:    "monitors",
		Ctx:      "ceph.cluster_monitors_count",
		Type:     collectorapi.Line,
		Priority: prioClusterMonitorsCount,
		Dims: collectorapi.Dims{
			{ID: "monitors_num", Name: "monitors"},
		},
	}
	clusterOsdsCountChart = collectorapi.Chart{
		ID:       "cluster_osds_count",
		Title:    "Ceph Cluster OSDs",
		Fam:      "status",
		Units:    "osds",
		Ctx:      "ceph.cluster_osds_count",
		Type:     collectorapi.Line,
		Priority: prioClusterOSDsCount,
		Dims: collectorapi.Dims{
			{ID: "osds_num", Name: "osds"},
		},
	}
	clusterOsdsByStatusCountChart = collectorapi.Chart{
		ID:       "cluster_osds_by_status_count",
		Title:    "Ceph Cluster OSDs by Status",
		Fam:      "status",
		Units:    "osds",
		Ctx:      "ceph.cluster_osds_by_status_count",
		Type:     collectorapi.Line,
		Priority: prioClusterOSDsByStatusCount,
		Dims: collectorapi.Dims{
			{ID: "osds_up_num", Name: "up"},
			{ID: "osds_down_num", Name: "down"},
			{ID: "osds_in_num", Name: "in"},
			{ID: "osds_out_num", Name: "out"},
		},
	}
	clusterManagersCountChart = collectorapi.Chart{
		ID:       "cluster_managers_count",
		Title:    "Ceph Cluster Managers",
		Fam:      "status",
		Units:    "managers",
		Ctx:      "ceph.cluster_managers_count",
		Type:     collectorapi.Line,
		Priority: prioClusterManagersCount,
		Dims: collectorapi.Dims{
			{ID: "mgr_active_num", Name: "active"},
			{ID: "mgr_standby_num", Name: "standby"},
		},
	}
	clusterObjectGatewaysCountChart = collectorapi.Chart{
		ID:       "cluster_object_gateways_count",
		Title:    "Ceph Cluster Object Gateways (RGW)",
		Fam:      "status",
		Units:    "gateways",
		Ctx:      "ceph.cluster_object_gateways_count",
		Type:     collectorapi.Line,
		Priority: prioClusterObjectGatewaysCount,
		Dims: collectorapi.Dims{
			{ID: "rgw_num", Name: "object"},
		},
	}
	clusterIScsiGatewaysCountChart = collectorapi.Chart{
		ID:       "cluster_iscsi_gateways_count",
		Title:    "Ceph Cluster iSCSI Gateways",
		Fam:      "status",
		Units:    "gateways",
		Ctx:      "ceph.cluster_iscsi_gateways_count",
		Type:     collectorapi.Line,
		Priority: prioClusterIScsiGatewaysCount,
		Dims: collectorapi.Dims{
			{ID: "iscsi_daemons_num", Name: "iscsi"},
		},
	}
	clusterIScsiGatewaysByStatusCountChart = collectorapi.Chart{
		ID:       "cluster_iscsi_gateways_by_status_count",
		Title:    "Ceph Cluster iSCSI Gateways by Status",
		Fam:      "status",
		Units:    "gateways",
		Ctx:      "ceph.cluster_iscsi_gateways_by_status_count",
		Type:     collectorapi.Line,
		Priority: prioClusterIScsiGatewaysByStatusCount,
		Dims: collectorapi.Dims{
			{ID: "iscsi_daemons_up_num", Name: "up"},
			{ID: "iscsi_daemons_down_num", Name: "down"},
		},
	}
)

var (
	clusterPhysCapacityUtilizationChart = collectorapi.Chart{
		ID:       "cluster_physical_capacity_utilization",
		Title:    "Ceph Cluster Physical Capacity Utilization",
		Fam:      "capacity",
		Units:    "percent",
		Ctx:      "ceph.cluster_physical_capacity_utilization",
		Type:     collectorapi.Area,
		Priority: prioClusterPhysCapacityUtilization,
		Dims: collectorapi.Dims{
			{ID: "raw_capacity_utilization", Name: "utilization", Div: precision},
		},
	}
	clusterPhysCapacityUsageChart = collectorapi.Chart{
		ID:       "cluster_physical_capacity_usage",
		Title:    "Ceph Cluster Physical Capacity Usage",
		Fam:      "capacity",
		Units:    "bytes",
		Ctx:      "ceph.cluster_physical_capacity_usage",
		Type:     collectorapi.Stacked,
		Priority: prioClusterPhysCapacityUsage,
		Dims: collectorapi.Dims{
			{ID: "raw_capacity_avail_bytes", Name: "avail"},
			{ID: "raw_capacity_used_bytes", Name: "used"},
		},
	}
	clusterObjectsCountChart = collectorapi.Chart{
		ID:       "cluster_objects_count",
		Title:    "Ceph Cluster Objects",
		Fam:      "capacity",
		Units:    "objects",
		Ctx:      "ceph.cluster_objects_count",
		Type:     collectorapi.Line,
		Priority: prioClusterObjectsCount,
		Dims: collectorapi.Dims{
			{ID: "objects_num", Name: "objects"},
		},
	}
	clusterObjectsByStatusPercentChart = collectorapi.Chart{
		ID:       "cluster_objects_by_status",
		Title:    "Ceph Cluster Objects by Status",
		Fam:      "capacity",
		Units:    "percent",
		Ctx:      "ceph.cluster_objects_by_status_distribution",
		Type:     collectorapi.Stacked,
		Priority: prioClusterObjectsByStatusPercent,
		Dims: collectorapi.Dims{
			{ID: "objects_healthy_num", Name: "healthy", Algo: collectorapi.PercentOfAbsolute},
			{ID: "objects_misplaced_num", Name: "misplaced", Algo: collectorapi.PercentOfAbsolute},
			{ID: "objects_degraded_num", Name: "degraded", Algo: collectorapi.PercentOfAbsolute},
			{ID: "objects_unfound_num", Name: "unfound", Algo: collectorapi.PercentOfAbsolute},
		},
	}
	clusterPoolsCountChart = collectorapi.Chart{
		ID:       "cluster_pools_count",
		Title:    "Ceph Cluster Pools",
		Fam:      "capacity",
		Units:    "pools",
		Ctx:      "ceph.cluster_pools_count",
		Type:     collectorapi.Line,
		Priority: prioClusterPoolsCount,
		Dims: collectorapi.Dims{
			{ID: "pools_num", Name: "pools"},
		},
	}
	clusterPGsCountChart = collectorapi.Chart{
		ID:       "cluster_pgs_count",
		Title:    "Ceph Cluster Placement Groups",
		Fam:      "capacity",
		Units:    "pgs",
		Ctx:      "ceph.cluster_pgs_count",
		Type:     collectorapi.Line,
		Priority: prioClusterPGsCount,
		Dims: collectorapi.Dims{
			{ID: "pgs_num", Name: "pgs"},
		},
	}
	clusterPGsByStatusCountChart = collectorapi.Chart{
		ID:       "cluster_pgs_by_status_count",
		Title:    "Ceph Cluster Placement Groups by Status",
		Fam:      "capacity",
		Units:    "pgs",
		Ctx:      "ceph.cluster_pgs_by_status_count",
		Type:     collectorapi.Stacked,
		Priority: prioClusterPGsByStatusCount,
		Dims: collectorapi.Dims{
			{ID: "pg_status_category_clean", Name: "clean"},
			{ID: "pg_status_category_working", Name: "working"},
			{ID: "pg_status_category_warning", Name: "warning"},
			{ID: "pg_status_category_unknown", Name: "unknown"},
		},
	}
	clusterPgsPerOsdCountChart = collectorapi.Chart{
		ID:       "cluster_pgs_per_osd_count",
		Title:    "Ceph Cluster Placement Groups per OSD",
		Fam:      "capacity",
		Units:    "pgs",
		Ctx:      "ceph.cluster_pgs_per_osd_count",
		Type:     collectorapi.Line,
		Priority: prioClusterPGsPerOsdCount,
		Dims: collectorapi.Dims{
			{ID: "pgs_per_osd", Name: "per_osd"},
		},
	}
)

var (
	clusterClientIOChart = collectorapi.Chart{
		ID:       "cluster_client_io",
		Title:    "Ceph Cluster Client IO",
		Fam:      "performance",
		Units:    "bytes/s",
		Ctx:      "ceph.cluster_client_io",
		Type:     collectorapi.Area,
		Priority: prioClusterClientIO,
		Dims: collectorapi.Dims{
			{ID: "client_perf_read_bytes_sec", Name: "read"},
			{ID: "client_perf_write_bytes_sec", Name: "written", Mul: -1},
		},
	}
	clusterClientIOPSChart = collectorapi.Chart{
		ID:       "cluster_client_iops",
		Title:    "Ceph Cluster Client IOPS",
		Fam:      "performance",
		Units:    "ops/s",
		Ctx:      "ceph.cluster_client_iops",
		Type:     collectorapi.Line,
		Priority: prioClusterClientIOPS,
		Dims: collectorapi.Dims{
			{ID: "client_perf_read_op_per_sec", Name: "read"},
			{ID: "client_perf_write_op_per_sec", Name: "write", Mul: -1},
		},
	}
	clusterRecoveryThroughputChart = collectorapi.Chart{
		ID:       "cluster_recovery_throughput",
		Title:    "Ceph Cluster Recovery Throughput",
		Fam:      "performance",
		Units:    "bytes/s",
		Ctx:      "ceph.cluster_recovery_throughput",
		Type:     collectorapi.Line,
		Priority: prioClusterClientRecoveryThroughput,
		Dims: collectorapi.Dims{
			{ID: "client_perf_recovering_bytes_per_sec", Name: "recovery"},
		},
	}
	clusterScrubStatusChart = collectorapi.Chart{
		ID:       "cluster_scrub_status",
		Title:    "Ceph Cluster Scrubbing Status",
		Fam:      "performance",
		Units:    "status",
		Ctx:      "ceph.cluster_scrub_status",
		Type:     collectorapi.Line,
		Priority: prioClusterScrubStatus,
		Dims: collectorapi.Dims{
			{ID: "scrub_status_disabled", Name: "disabled"},
			{ID: "scrub_status_active", Name: "active"},
			{ID: "scrub_status_inactive", Name: "inactive"},
		},
	}
)

var (
	osdStatusChartTmpl = collectorapi.Chart{
		ID:       "osd_%s_status",
		Title:    "Ceph OSD Status",
		Fam:      "osd",
		Units:    "status",
		Ctx:      "ceph.osd_status",
		Type:     collectorapi.Line,
		Priority: prioOsdStatus,
		Dims: collectorapi.Dims{
			{ID: "osd_%s_status_up", Name: "up"},
			{ID: "osd_%s_status_down", Name: "down"},
			{ID: "osd_%s_status_in", Name: "in"},
			{ID: "osd_%s_status_out", Name: "out"},
		},
	}
	osdSpaceUsageChartTmpl = collectorapi.Chart{
		ID:       "osd_%s_space_usage",
		Title:    "Ceph OSD Space Usage",
		Fam:      "osd",
		Units:    "bytes",
		Ctx:      "ceph.osd_space_usage",
		Type:     collectorapi.Stacked,
		Priority: prioOsdSpaceUsage,
		Dims: collectorapi.Dims{
			{ID: "osd_%s_space_avail_bytes", Name: "avail"},
			{ID: "osd_%s_space_used_bytes", Name: "used"},
		},
	}
	osdIOChartTmpl = collectorapi.Chart{
		ID:       "osd_%s_io",
		Title:    "Ceph OSD IO",
		Fam:      "osd",
		Units:    "bytes/s",
		Ctx:      "ceph.osd_io",
		Type:     collectorapi.Area,
		Priority: prioOsdIO,
		Dims: collectorapi.Dims{
			{ID: "osd_%s_read_bytes", Name: "read", Algo: collectorapi.Incremental},
			{ID: "osd_%s_written_bytes", Name: "written", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	osdIOPSChartTmpl = collectorapi.Chart{
		ID:       "osd_%s_iops",
		Title:    "Ceph OSD IOPS",
		Fam:      "osd",
		Units:    "ops/s",
		Ctx:      "ceph.osd_iops",
		Type:     collectorapi.Line,
		Priority: prioOsdIOPS,
		Dims: collectorapi.Dims{
			{ID: "osd_%s_read_ops", Name: "read", Algo: collectorapi.Incremental},
			{ID: "osd_%s_write_ops", Name: "write", Algo: collectorapi.Incremental},
		},
	}
	osdLatencyChartTmpl = collectorapi.Chart{
		ID:       "osd_%s_latency",
		Title:    "Ceph OSD Latency",
		Fam:      "osd",
		Units:    "milliseconds",
		Ctx:      "ceph.osd_latency",
		Type:     collectorapi.Line,
		Priority: prioOsdLatency,
		Dims: collectorapi.Dims{
			{ID: "osd_%s_commit_latency_ms", Name: "commit"},
			{ID: "osd_%s_apply_latency_ms", Name: "apply"},
		},
	}
)

var (
	poolSpaceUtilizationChartTmpl = collectorapi.Chart{
		ID:       "pool_%s_space_utilization",
		Title:    "Ceph Pool Space Utilization",
		Fam:      "pool",
		Units:    "percent",
		Ctx:      "ceph.pool_space_utilization",
		Type:     collectorapi.Area,
		Priority: prioPoolSpaceUtilization,
		Dims: collectorapi.Dims{
			{ID: "pool_%s_space_utilization", Name: "utilization", Div: precision},
		},
	}
	poolSpaceUsageChartTmpl = collectorapi.Chart{
		ID:       "pool_%s_space_usage",
		Title:    "Ceph Pool Space Usage",
		Fam:      "pool",
		Units:    "bytes",
		Ctx:      "ceph.pool_space_usage",
		Type:     collectorapi.Stacked,
		Priority: prioPoolSpaceUsage,
		Dims: collectorapi.Dims{
			{ID: "pool_%s_space_avail_bytes", Name: "avail"},
			{ID: "pool_%s_space_used_bytes", Name: "used"},
		},
	}
	poolObjectsCountChartTmpl = collectorapi.Chart{
		ID:       "pool_%s_objects_count",
		Title:    "Ceph Pool Objects",
		Fam:      "pool",
		Units:    "objects",
		Ctx:      "ceph.pool_objects_count",
		Type:     collectorapi.Line,
		Priority: prioPoolObjectsCount,
		Dims: collectorapi.Dims{
			{ID: "pool_%s_objects", Name: "objects"},
		},
	}
	poolIOChartTmpl = collectorapi.Chart{
		ID:       "pool_%s_io",
		Title:    "Ceph Pool IO",
		Fam:      "pool",
		Units:    "bytes/s",
		Ctx:      "ceph.pool_io",
		Type:     collectorapi.Area,
		Priority: prioPoolIO,
		Dims: collectorapi.Dims{
			{ID: "pool_%s_read_bytes", Name: "read", Algo: collectorapi.Incremental},
			{ID: "pool_%s_written_bytes", Name: "written", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
	poolIOPSChartTmpl = collectorapi.Chart{
		ID:       "pool_%s_iops",
		Title:    "Ceph Pool IOPS",
		Fam:      "pool",
		Units:    "ops/s",
		Ctx:      "ceph.pool_iops",
		Type:     collectorapi.Line,
		Priority: prioPoolIOPS,
		Dims: collectorapi.Dims{
			{ID: "pool_%s_read_ops", Name: "read", Algo: collectorapi.Incremental},
			{ID: "pool_%s_write_ops", Name: "write", Algo: collectorapi.Incremental, Mul: -1},
		},
	}
)

func (c *Collector) addClusterCharts() {
	charts := clusterCharts.Copy()

	for _, chart := range *charts {
		chart.Labels = []collectorapi.Label{
			{Key: "fsid", Value: c.fsid},
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addOsdCharts(osdUuid, devClass, osdName string) {
	charts := osdChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, osdUuid)
		chart.ID = cleanChartID(chart.ID)
		chart.Labels = []collectorapi.Label{
			{Key: "fsid", Value: c.fsid},
			{Key: "osd_uuid", Value: osdUuid},
			{Key: "osd_name", Value: osdName},
			{Key: "device_class", Value: devClass},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, osdUuid)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) addPoolCharts(poolName string) {
	charts := poolChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, poolName)
		chart.ID = cleanChartID(chart.ID)
		chart.Labels = []collectorapi.Label{
			{Key: "fsid", Value: c.fsid},
			{Key: "pool_name", Value: poolName},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, poolName)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeCharts(prefix string) {
	prefix = cleanChartID(prefix)
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func cleanChartID(id string) string {
	r := strings.NewReplacer(".", "_", " ", "_")
	return strings.ToLower(r.Replace(id))
}
