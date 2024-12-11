// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioClusterStatus = module.Priority + iota
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

var clusterCharts = module.Charts{
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

var osdChartsTmpl = module.Charts{
	osdStatusChartTmpl.Copy(),
	osdSpaceUsageChartTmpl.Copy(),
	osdIOChartTmpl.Copy(),
	osdIOPSChartTmpl.Copy(),
	osdLatencyChartTmpl.Copy(),
}

var poolChartsTmpl = module.Charts{
	poolSpaceUtilizationChartTmpl.Copy(),
	poolSpaceUsageChartTmpl.Copy(),
	poolObjectsCountChartTmpl.Copy(),
	poolIOChartTmpl.Copy(),
	poolIOPSChartTmpl.Copy(),
}

var (
	clusterStatusChart = module.Chart{
		ID:       "cluster_status",
		Title:    "Ceph Cluster Status",
		Fam:      "status",
		Units:    "status",
		Ctx:      "ceph.cluster_status",
		Type:     module.Line,
		Priority: prioClusterStatus,
		Dims: module.Dims{
			{ID: "health_ok", Name: "ok"},
			{ID: "health_err", Name: "err"},
			{ID: "health_warn", Name: "warn"},
		},
	}
	clusterHostsCountChart = module.Chart{
		ID:       "cluster_hosts_count",
		Title:    "Ceph Cluster Hosts",
		Fam:      "status",
		Units:    "hosts",
		Ctx:      "ceph.cluster_hosts_count",
		Type:     module.Line,
		Priority: prioClusterHostsCount,
		Dims: module.Dims{
			{ID: "hosts_num", Name: "hosts"},
		},
	}
	clusterMonitorsCountChart = module.Chart{
		ID:       "cluster_monitors_count",
		Title:    "Ceph Cluster Monitors",
		Fam:      "status",
		Units:    "monitors",
		Ctx:      "ceph.cluster_monitors_count",
		Type:     module.Line,
		Priority: prioClusterMonitorsCount,
		Dims: module.Dims{
			{ID: "monitors_num", Name: "monitors"},
		},
	}
	clusterOsdsCountChart = module.Chart{
		ID:       "cluster_osds_count",
		Title:    "Ceph Cluster OSDs",
		Fam:      "status",
		Units:    "osds",
		Ctx:      "ceph.cluster_osds_count",
		Type:     module.Line,
		Priority: prioClusterOSDsCount,
		Dims: module.Dims{
			{ID: "osds_num", Name: "osds"},
		},
	}
	clusterOsdsByStatusCountChart = module.Chart{
		ID:       "cluster_osds_by_status_count",
		Title:    "Ceph Cluster OSDs by Status",
		Fam:      "status",
		Units:    "osds",
		Ctx:      "ceph.cluster_osds_by_status_count",
		Type:     module.Line,
		Priority: prioClusterOSDsByStatusCount,
		Dims: module.Dims{
			{ID: "osds_up_num", Name: "up"},
			{ID: "osds_down_num", Name: "down"},
			{ID: "osds_in_num", Name: "in"},
			{ID: "osds_out_num", Name: "out"},
		},
	}
	clusterManagersCountChart = module.Chart{
		ID:       "cluster_managers_count",
		Title:    "Ceph Cluster Managers",
		Fam:      "status",
		Units:    "managers",
		Ctx:      "ceph.cluster_managers_count",
		Type:     module.Line,
		Priority: prioClusterManagersCount,
		Dims: module.Dims{
			{ID: "mgr_active_num", Name: "active"},
			{ID: "mgr_standby_num", Name: "standby"},
		},
	}
	clusterObjectGatewaysCountChart = module.Chart{
		ID:       "cluster_object_gateways_count",
		Title:    "Ceph Cluster Object Gateways (RGW)",
		Fam:      "status",
		Units:    "gateways",
		Ctx:      "ceph.cluster_object_gateways_count",
		Type:     module.Line,
		Priority: prioClusterObjectGatewaysCount,
		Dims: module.Dims{
			{ID: "rgw_num", Name: "object"},
		},
	}
	clusterIScsiGatewaysCountChart = module.Chart{
		ID:       "cluster_iscsi_gateways_count",
		Title:    "Ceph Cluster iSCSI Gateways",
		Fam:      "status",
		Units:    "gateways",
		Ctx:      "ceph.cluster_iscsi_gateways_count",
		Type:     module.Line,
		Priority: prioClusterIScsiGatewaysCount,
		Dims: module.Dims{
			{ID: "iscsi_daemons_num", Name: "iscsi"},
		},
	}
	clusterIScsiGatewaysByStatusCountChart = module.Chart{
		ID:       "cluster_iscsi_gateways_by_status_count",
		Title:    "Ceph Cluster iSCSI Gateways by Status",
		Fam:      "status",
		Units:    "gateways",
		Ctx:      "ceph.cluster_iscsi_gateways_by_status_count",
		Type:     module.Line,
		Priority: prioClusterIScsiGatewaysByStatusCount,
		Dims: module.Dims{
			{ID: "iscsi_daemons_up_num", Name: "up"},
			{ID: "iscsi_daemons_down_num", Name: "down"},
		},
	}
)

var (
	clusterPhysCapacityUtilizationChart = module.Chart{
		ID:       "cluster_physical_capacity_utilization",
		Title:    "Ceph Cluster Physical Capacity Utilization",
		Fam:      "capacity",
		Units:    "percent",
		Ctx:      "ceph.cluster_physical_capacity_utilization",
		Type:     module.Area,
		Priority: prioClusterPhysCapacityUtilization,
		Dims: module.Dims{
			{ID: "raw_capacity_utilization", Name: "utilization", Div: precision},
		},
	}
	clusterPhysCapacityUsageChart = module.Chart{
		ID:       "cluster_physical_capacity_usage",
		Title:    "Ceph Cluster Physical Capacity Usage",
		Fam:      "capacity",
		Units:    "bytes",
		Ctx:      "ceph.cluster_physical_capacity_usage",
		Type:     module.Stacked,
		Priority: prioClusterPhysCapacityUsage,
		Dims: module.Dims{
			{ID: "raw_capacity_avail_bytes", Name: "avail"},
			{ID: "raw_capacity_used_bytes", Name: "used"},
		},
	}
	clusterObjectsCountChart = module.Chart{
		ID:       "cluster_objects_count",
		Title:    "Ceph Cluster Objects",
		Fam:      "capacity",
		Units:    "objects",
		Ctx:      "ceph.cluster_objects_count",
		Type:     module.Line,
		Priority: prioClusterObjectsCount,
		Dims: module.Dims{
			{ID: "objects_num", Name: "objects"},
		},
	}
	clusterObjectsByStatusPercentChart = module.Chart{
		ID:       "cluster_objects_by_status",
		Title:    "Ceph Cluster Objects by Status",
		Fam:      "capacity",
		Units:    "percent",
		Ctx:      "ceph.cluster_objects_by_status_distribution",
		Type:     module.Stacked,
		Priority: prioClusterObjectsByStatusPercent,
		Dims: module.Dims{
			{ID: "objects_healthy_num", Name: "healthy", Algo: module.PercentOfAbsolute},
			{ID: "objects_misplaced_num", Name: "misplaced", Algo: module.PercentOfAbsolute},
			{ID: "objects_degraded_num", Name: "degraded", Algo: module.PercentOfAbsolute},
			{ID: "objects_unfound_num", Name: "unfound", Algo: module.PercentOfAbsolute},
		},
	}
	clusterPoolsCountChart = module.Chart{
		ID:       "cluster_pools_count",
		Title:    "Ceph Cluster Pools",
		Fam:      "capacity",
		Units:    "pools",
		Ctx:      "ceph.cluster_pools_count",
		Type:     module.Line,
		Priority: prioClusterPoolsCount,
		Dims: module.Dims{
			{ID: "pools_num", Name: "pools"},
		},
	}
	clusterPGsCountChart = module.Chart{
		ID:       "cluster_pgs_count",
		Title:    "Ceph Cluster Placement Groups",
		Fam:      "capacity",
		Units:    "pgs",
		Ctx:      "ceph.cluster_pgs_count",
		Type:     module.Line,
		Priority: prioClusterPGsCount,
		Dims: module.Dims{
			{ID: "pgs_num", Name: "pgs"},
		},
	}
	clusterPGsByStatusCountChart = module.Chart{
		ID:       "cluster_pgs_by_status_count",
		Title:    "Ceph Cluster Placement Groups by Status",
		Fam:      "capacity",
		Units:    "pgs",
		Ctx:      "ceph.cluster_pgs_by_status_count",
		Type:     module.Stacked,
		Priority: prioClusterPGsByStatusCount,
		Dims: module.Dims{
			{ID: "pg_status_category_clean", Name: "clean"},
			{ID: "pg_status_category_working", Name: "working"},
			{ID: "pg_status_category_warning", Name: "warning"},
			{ID: "pg_status_category_unknown", Name: "unknown"},
		},
	}
	clusterPgsPerOsdCountChart = module.Chart{
		ID:       "cluster_pgs_per_osd_count",
		Title:    "Ceph Cluster Placement Groups per OSD",
		Fam:      "capacity",
		Units:    "pgs",
		Ctx:      "ceph.cluster_pgs_per_osd_count",
		Type:     module.Line,
		Priority: prioClusterPGsPerOsdCount,
		Dims: module.Dims{
			{ID: "pgs_per_osd", Name: "per_osd"},
		},
	}
)

var (
	clusterClientIOChart = module.Chart{
		ID:       "cluster_client_io",
		Title:    "Ceph Cluster Client IO",
		Fam:      "performance",
		Units:    "bytes/s",
		Ctx:      "ceph.cluster_client_io",
		Type:     module.Area,
		Priority: prioClusterClientIO,
		Dims: module.Dims{
			{ID: "client_perf_read_bytes_sec", Name: "read"},
			{ID: "client_perf_write_bytes_sec", Name: "written", Mul: -1},
		},
	}
	clusterClientIOPSChart = module.Chart{
		ID:       "cluster_client_iops",
		Title:    "Ceph Cluster Client IOPS",
		Fam:      "performance",
		Units:    "ops/s",
		Ctx:      "ceph.cluster_client_iops",
		Type:     module.Line,
		Priority: prioClusterClientIOPS,
		Dims: module.Dims{
			{ID: "client_perf_read_op_per_sec", Name: "read"},
			{ID: "client_perf_write_op_per_sec", Name: "write", Mul: -1},
		},
	}
	clusterRecoveryThroughputChart = module.Chart{
		ID:       "cluster_recovery_throughput",
		Title:    "Ceph Cluster Recovery Throughput",
		Fam:      "performance",
		Units:    "bytes/s",
		Ctx:      "ceph.cluster_recovery_throughput",
		Type:     module.Line,
		Priority: prioClusterClientRecoveryThroughput,
		Dims: module.Dims{
			{ID: "client_perf_recovering_bytes_per_sec", Name: "recovery"},
		},
	}
	clusterScrubStatusChart = module.Chart{
		ID:       "cluster_scrub_status",
		Title:    "Ceph Cluster Scrubbing Status",
		Fam:      "performance",
		Units:    "status",
		Ctx:      "ceph.cluster_scrub_status",
		Type:     module.Line,
		Priority: prioClusterScrubStatus,
		Dims: module.Dims{
			{ID: "scrub_status_disabled", Name: "disabled"},
			{ID: "scrub_status_active", Name: "active"},
			{ID: "scrub_status_inactive", Name: "inactive"},
		},
	}
)

var (
	osdStatusChartTmpl = module.Chart{
		ID:       "osd_%s_status",
		Title:    "Ceph OSD Status",
		Fam:      "osd",
		Units:    "status",
		Ctx:      "ceph.osd_status",
		Type:     module.Line,
		Priority: prioOsdStatus,
		Dims: module.Dims{
			{ID: "osd_%s_status_up", Name: "up"},
			{ID: "osd_%s_status_down", Name: "down"},
			{ID: "osd_%s_status_in", Name: "in"},
			{ID: "osd_%s_status_out", Name: "out"},
		},
	}
	osdSpaceUsageChartTmpl = module.Chart{
		ID:       "osd_%s_space_usage",
		Title:    "Ceph OSD Space Usage",
		Fam:      "osd",
		Units:    "bytes",
		Ctx:      "ceph.osd_space_usage",
		Type:     module.Stacked,
		Priority: prioOsdSpaceUsage,
		Dims: module.Dims{
			{ID: "osd_%s_space_avail_bytes", Name: "avail"},
			{ID: "osd_%s_space_used_bytes", Name: "used"},
		},
	}
	osdIOChartTmpl = module.Chart{
		ID:       "osd_%s_io",
		Title:    "Ceph OSD IO",
		Fam:      "osd",
		Units:    "bytes/s",
		Ctx:      "ceph.osd_io",
		Type:     module.Area,
		Priority: prioOsdIO,
		Dims: module.Dims{
			{ID: "osd_%s_read_bytes", Name: "read", Algo: module.Incremental},
			{ID: "osd_%s_written_bytes", Name: "written", Algo: module.Incremental, Mul: -1},
		},
	}
	osdIOPSChartTmpl = module.Chart{
		ID:       "osd_%s_iops",
		Title:    "Ceph OSD IOPS",
		Fam:      "osd",
		Units:    "ops/s",
		Ctx:      "ceph.osd_iops",
		Type:     module.Line,
		Priority: prioOsdIOPS,
		Dims: module.Dims{
			{ID: "osd_%s_read_ops", Name: "read", Algo: module.Incremental},
			{ID: "osd_%s_write_ops", Name: "write", Algo: module.Incremental},
		},
	}
	osdLatencyChartTmpl = module.Chart{
		ID:       "osd_%s_latency",
		Title:    "Ceph OSD Latency",
		Fam:      "osd",
		Units:    "milliseconds",
		Ctx:      "ceph.osd_latency",
		Type:     module.Line,
		Priority: prioOsdLatency,
		Dims: module.Dims{
			{ID: "osd_%s_commit_latency_ms", Name: "commit"},
			{ID: "osd_%s_apply_latency_ms", Name: "apply"},
		},
	}
)

var (
	poolSpaceUtilizationChartTmpl = module.Chart{
		ID:       "pool_%s_space_utilization",
		Title:    "Ceph Pool Space Utilization",
		Fam:      "pool",
		Units:    "percent",
		Ctx:      "ceph.pool_space_utilization",
		Type:     module.Area,
		Priority: prioPoolSpaceUtilization,
		Dims: module.Dims{
			{ID: "pool_%s_space_utilization", Name: "utilization", Div: precision},
		},
	}
	poolSpaceUsageChartTmpl = module.Chart{
		ID:       "pool_%s_space_usage",
		Title:    "Ceph Pool Space Usage",
		Fam:      "pool",
		Units:    "bytes",
		Ctx:      "ceph.pool_space_usage",
		Type:     module.Stacked,
		Priority: prioPoolSpaceUsage,
		Dims: module.Dims{
			{ID: "pool_%s_space_avail_bytes", Name: "avail"},
			{ID: "pool_%s_space_used_bytes", Name: "used"},
		},
	}
	poolObjectsCountChartTmpl = module.Chart{
		ID:       "pool_%s_objects_count",
		Title:    "Ceph Pool Objects",
		Fam:      "pool",
		Units:    "objects",
		Ctx:      "ceph.pool_objects_count",
		Type:     module.Line,
		Priority: prioPoolObjectsCount,
		Dims: module.Dims{
			{ID: "pool_%s_objects", Name: "objects"},
		},
	}
	poolIOChartTmpl = module.Chart{
		ID:       "pool_%s_io",
		Title:    "Ceph Pool IO",
		Fam:      "pool",
		Units:    "bytes/s",
		Ctx:      "ceph.pool_io",
		Type:     module.Area,
		Priority: prioPoolIO,
		Dims: module.Dims{
			{ID: "pool_%s_read_bytes", Name: "read", Algo: module.Incremental},
			{ID: "pool_%s_written_bytes", Name: "written", Algo: module.Incremental, Mul: -1},
		},
	}
	poolIOPSChartTmpl = module.Chart{
		ID:       "pool_%s_iops",
		Title:    "Ceph Pool IOPS",
		Fam:      "pool",
		Units:    "ops/s",
		Ctx:      "ceph.pool_iops",
		Type:     module.Line,
		Priority: prioPoolIOPS,
		Dims: module.Dims{
			{ID: "pool_%s_read_ops", Name: "read", Algo: module.Incremental},
			{ID: "pool_%s_write_ops", Name: "write", Algo: module.Incremental, Mul: -1},
		},
	}
)

func (c *Collector) addClusterCharts() {
	charts := clusterCharts.Copy()

	for _, chart := range *charts {
		chart.Labels = []module.Label{
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
		chart.Labels = []module.Label{
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
		chart.Labels = []module.Label{
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
