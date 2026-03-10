// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/powerstore/client"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

type (
	Charts = collectorapi.Charts
	Dims   = collectorapi.Dims
)

var (
	prioAppliance  = collectorapi.Priority + len(clusterCharts) + 10
	prioVolume     = prioAppliance + len(applianceChartsTmpl)*2 + 10
	prioNode       = prioVolume + len(volumeChartsTmpl)*2 + 10
	prioFcPort     = prioNode + len(nodeChartsTmpl)*2 + 10
	prioEthPort    = prioFcPort + len(fcPortChartsTmpl)*2 + 10
	prioFileSystem = prioEthPort + len(ethPortChartsTmpl)*2 + 10
	prioDrive      = prioFileSystem + len(fileSystemChartsTmpl)*2 + 10
)

// --- Cluster-level (static) charts ---

var clusterCharts = Charts{
	{
		ID:    "cluster_space_usage",
		Title: "Cluster Space Usage",
		Units: "bytes",
		Fam:   "cluster capacity",
		Ctx:   "powerstore.cluster_space_usage",
		Dims: Dims{
			{ID: "cluster_space_physical_used", Name: "used"},
			{ID: "cluster_space_physical_total", Name: "total"},
		},
	},
	{
		ID:    "cluster_space_logical",
		Title: "Cluster Logical Space",
		Units: "bytes",
		Fam:   "cluster capacity",
		Ctx:   "powerstore.cluster_space_logical",
		Dims: Dims{
			{ID: "cluster_space_logical_provisioned", Name: "provisioned"},
			{ID: "cluster_space_logical_used", Name: "used"},
			{ID: "cluster_space_data_physical_used", Name: "data_physical"},
			{ID: "cluster_space_shared_logical_used", Name: "shared"},
		},
	},
	{
		ID:    "cluster_space_efficiency",
		Title: "Cluster Space Efficiency Ratios",
		Units: "ratio",
		Fam:   "cluster capacity",
		Ctx:   "powerstore.cluster_space_efficiency",
		Dims: Dims{
			{ID: "cluster_space_efficiency_ratio", Name: "efficiency", Div: 1000},
			{ID: "cluster_space_data_reduction", Name: "data_reduction", Div: 1000},
			{ID: "cluster_space_snapshot_savings", Name: "snapshot_savings", Div: 1000},
			{ID: "cluster_space_thin_savings", Name: "thin_savings", Div: 1000},
		},
	},
	// Hardware health
	{
		ID:    "hardware_health_fan",
		Title: "Fan Health Status",
		Units: "fans",
		Fam:   "hardware health",
		Type:  collectorapi.Stacked,
		Ctx:   "powerstore.hardware_health_fan",
		Dims: Dims{
			{ID: "hardware_fan_ok", Name: "ok"},
			{ID: "hardware_fan_degraded", Name: "degraded"},
			{ID: "hardware_fan_failed", Name: "failed"},
			{ID: "hardware_fan_unknown", Name: "unknown"},
		},
	},
	{
		ID:    "hardware_health_psu",
		Title: "Power Supply Health Status",
		Units: "PSUs",
		Fam:   "hardware health",
		Type:  collectorapi.Stacked,
		Ctx:   "powerstore.hardware_health_psu",
		Dims: Dims{
			{ID: "hardware_psu_ok", Name: "ok"},
			{ID: "hardware_psu_degraded", Name: "degraded"},
			{ID: "hardware_psu_failed", Name: "failed"},
			{ID: "hardware_psu_unknown", Name: "unknown"},
		},
	},
	{
		ID:    "hardware_health_drive",
		Title: "Drive Health Status",
		Units: "drives",
		Fam:   "hardware health",
		Type:  collectorapi.Stacked,
		Ctx:   "powerstore.hardware_health_drive",
		Dims: Dims{
			{ID: "hardware_drive_ok", Name: "ok"},
			{ID: "hardware_drive_degraded", Name: "degraded"},
			{ID: "hardware_drive_failed", Name: "failed"},
			{ID: "hardware_drive_unknown", Name: "unknown"},
		},
	},
	{
		ID:    "hardware_health_battery",
		Title: "Battery Health Status",
		Units: "batteries",
		Fam:   "hardware health",
		Type:  collectorapi.Stacked,
		Ctx:   "powerstore.hardware_health_battery",
		Dims: Dims{
			{ID: "hardware_battery_ok", Name: "ok"},
			{ID: "hardware_battery_degraded", Name: "degraded"},
			{ID: "hardware_battery_failed", Name: "failed"},
			{ID: "hardware_battery_unknown", Name: "unknown"},
		},
	},
	{
		ID:    "hardware_health_node",
		Title: "Node Health Status",
		Units: "nodes",
		Fam:   "hardware health",
		Type:  collectorapi.Stacked,
		Ctx:   "powerstore.hardware_health_node",
		Dims: Dims{
			{ID: "hardware_node_ok", Name: "ok"},
			{ID: "hardware_node_degraded", Name: "degraded"},
			{ID: "hardware_node_failed", Name: "failed"},
			{ID: "hardware_node_unknown", Name: "unknown"},
		},
	},
	// Alerts
	{
		ID:    "alerts_active",
		Title: "Active Alerts by Severity",
		Units: "alerts",
		Fam:   "alerts",
		Type:  collectorapi.Stacked,
		Ctx:   "powerstore.alerts_active",
		Dims: Dims{
			{ID: "alerts_critical", Name: "critical"},
			{ID: "alerts_major", Name: "major"},
			{ID: "alerts_minor", Name: "minor"},
			{ID: "alerts_info", Name: "info"},
		},
	},
	// NAS server status
	{
		ID:    "nas_server_status",
		Title: "NAS Server Status",
		Units: "servers",
		Fam:   "nas",
		Type:  collectorapi.Stacked,
		Ctx:   "powerstore.nas_server_status",
		Dims: Dims{
			{ID: "nas_started", Name: "started"},
			{ID: "nas_stopped", Name: "stopped"},
			{ID: "nas_degraded", Name: "degraded"},
			{ID: "nas_unknown", Name: "unknown"},
		},
	},
	// Replication
	{
		ID:    "copy_data",
		Title: "Replication Data",
		Units: "bytes",
		Fam:   "replication",
		Ctx:   "powerstore.copy_data",
		Dims: Dims{
			{ID: "copy_data_remaining", Name: "remaining"},
			{ID: "copy_data_transferred", Name: "transferred"},
		},
	},
	{
		ID:    "copy_transfer_rate",
		Title: "Replication Transfer Rate",
		Units: "bytes/s",
		Fam:   "replication",
		Ctx:   "powerstore.copy_transfer_rate",
		Dims: Dims{
			{ID: "copy_transfer_rate", Name: "rate", Div: 1000},
		},
	},
}

// --- Appliance charts (per-appliance, dynamic) ---

var applianceChartsTmpl = Charts{
	{
		ID:    "appliance_%s_iops",
		Title: "Appliance IOPS",
		Units: "ops/s",
		Fam:   "appliance %s",
		Type:  collectorapi.Area,
		Ctx:   "powerstore.appliance_iops",
		Dims: Dims{
			{ID: "appliance_%s_perf_read_iops", Name: "read", Div: 1000},
			{ID: "appliance_%s_perf_write_iops", Name: "write", Mul: -1, Div: 1000},
		},
	},
	{
		ID:    "appliance_%s_bandwidth",
		Title: "Appliance Bandwidth",
		Units: "bytes/s",
		Fam:   "appliance %s",
		Type:  collectorapi.Area,
		Ctx:   "powerstore.appliance_bandwidth",
		Dims: Dims{
			{ID: "appliance_%s_perf_read_bandwidth", Name: "read", Div: 1000},
			{ID: "appliance_%s_perf_write_bandwidth", Name: "write", Mul: -1, Div: 1000},
		},
	},
	{
		ID:    "appliance_%s_latency",
		Title: "Appliance Latency",
		Units: "microseconds",
		Fam:   "appliance %s",
		Ctx:   "powerstore.appliance_latency",
		Dims: Dims{
			{ID: "appliance_%s_perf_avg_read_latency", Name: "read", Div: 1000},
			{ID: "appliance_%s_perf_avg_write_latency", Name: "write", Div: 1000},
			{ID: "appliance_%s_perf_avg_latency", Name: "avg", Div: 1000},
		},
	},
	{
		ID:    "appliance_%s_cpu",
		Title: "Appliance IO Workload CPU Utilization",
		Units: "percentage",
		Fam:   "appliance %s",
		Ctx:   "powerstore.appliance_cpu",
		Dims: Dims{
			{ID: "appliance_%s_cpu_utilization", Name: "utilization", Div: 1000},
		},
	},
	{
		ID:    "appliance_%s_space",
		Title: "Appliance Space Usage",
		Units: "bytes",
		Fam:   "appliance %s",
		Ctx:   "powerstore.appliance_space",
		Dims: Dims{
			{ID: "appliance_%s_space_physical_used", Name: "used"},
			{ID: "appliance_%s_space_physical_total", Name: "total"},
		},
	},
}

// --- Volume charts (per-volume, dynamic) ---

var volumeChartsTmpl = Charts{
	{
		ID:    "volume_%s_iops",
		Title: "Volume IOPS",
		Units: "ops/s",
		Fam:   "volume %s",
		Type:  collectorapi.Area,
		Ctx:   "powerstore.volume_iops",
		Dims: Dims{
			{ID: "volume_%s_perf_read_iops", Name: "read", Div: 1000},
			{ID: "volume_%s_perf_write_iops", Name: "write", Mul: -1, Div: 1000},
		},
	},
	{
		ID:    "volume_%s_bandwidth",
		Title: "Volume Bandwidth",
		Units: "bytes/s",
		Fam:   "volume %s",
		Type:  collectorapi.Area,
		Ctx:   "powerstore.volume_bandwidth",
		Dims: Dims{
			{ID: "volume_%s_perf_read_bandwidth", Name: "read", Div: 1000},
			{ID: "volume_%s_perf_write_bandwidth", Name: "write", Mul: -1, Div: 1000},
		},
	},
	{
		ID:    "volume_%s_latency",
		Title: "Volume Latency",
		Units: "microseconds",
		Fam:   "volume %s",
		Ctx:   "powerstore.volume_latency",
		Dims: Dims{
			{ID: "volume_%s_perf_avg_read_latency", Name: "read", Div: 1000},
			{ID: "volume_%s_perf_avg_write_latency", Name: "write", Div: 1000},
			{ID: "volume_%s_perf_avg_latency", Name: "avg", Div: 1000},
		},
	},
	{
		ID:    "volume_%s_space",
		Title: "Volume Space Usage",
		Units: "bytes",
		Fam:   "volume %s",
		Ctx:   "powerstore.volume_space",
		Dims: Dims{
			{ID: "volume_%s_space_logical_provisioned", Name: "provisioned"},
			{ID: "volume_%s_space_logical_used", Name: "used"},
		},
	},
}

// --- Node charts (per-node, dynamic) ---

var nodeChartsTmpl = Charts{
	{
		ID:    "node_%s_iops",
		Title: "Node IOPS",
		Units: "ops/s",
		Fam:   "node %s",
		Type:  collectorapi.Area,
		Ctx:   "powerstore.node_iops",
		Dims: Dims{
			{ID: "node_%s_perf_read_iops", Name: "read", Div: 1000},
			{ID: "node_%s_perf_write_iops", Name: "write", Mul: -1, Div: 1000},
		},
	},
	{
		ID:    "node_%s_bandwidth",
		Title: "Node Bandwidth",
		Units: "bytes/s",
		Fam:   "node %s",
		Type:  collectorapi.Area,
		Ctx:   "powerstore.node_bandwidth",
		Dims: Dims{
			{ID: "node_%s_perf_read_bandwidth", Name: "read", Div: 1000},
			{ID: "node_%s_perf_write_bandwidth", Name: "write", Mul: -1, Div: 1000},
		},
	},
	{
		ID:    "node_%s_latency",
		Title: "Node Latency",
		Units: "microseconds",
		Fam:   "node %s",
		Ctx:   "powerstore.node_latency",
		Dims: Dims{
			{ID: "node_%s_perf_avg_read_latency", Name: "read", Div: 1000},
			{ID: "node_%s_perf_avg_write_latency", Name: "write", Div: 1000},
			{ID: "node_%s_perf_avg_latency", Name: "avg", Div: 1000},
		},
	},
	{
		ID:    "node_%s_logins",
		Title: "Node Current Logins",
		Units: "logins",
		Fam:   "node %s",
		Ctx:   "powerstore.node_logins",
		Dims: Dims{
			{ID: "node_%s_current_logins", Name: "logins"},
		},
	},
}

// --- FC Port charts (per-port, dynamic) ---

var fcPortChartsTmpl = Charts{
	{
		ID:    "fc_port_%s_iops",
		Title: "FC Port IOPS",
		Units: "ops/s",
		Fam:   "fc port %s",
		Type:  collectorapi.Area,
		Ctx:   "powerstore.fc_port_iops",
		Dims: Dims{
			{ID: "fc_port_%s_perf_read_iops", Name: "read", Div: 1000},
			{ID: "fc_port_%s_perf_write_iops", Name: "write", Mul: -1, Div: 1000},
		},
	},
	{
		ID:    "fc_port_%s_bandwidth",
		Title: "FC Port Bandwidth",
		Units: "bytes/s",
		Fam:   "fc port %s",
		Type:  collectorapi.Area,
		Ctx:   "powerstore.fc_port_bandwidth",
		Dims: Dims{
			{ID: "fc_port_%s_perf_read_bandwidth", Name: "read", Div: 1000},
			{ID: "fc_port_%s_perf_write_bandwidth", Name: "write", Mul: -1, Div: 1000},
		},
	},
	{
		ID:    "fc_port_%s_link_status",
		Title: "FC Port Link Status",
		Units: "status",
		Fam:   "fc port %s",
		Ctx:   "powerstore.fc_port_link_status",
		Dims: Dims{
			{ID: "fc_port_%s_link_up", Name: "up"},
		},
	},
}

// --- ETH Port charts (per-port, dynamic) ---

var ethPortChartsTmpl = Charts{
	{
		ID:    "eth_port_%s_bytes",
		Title: "Ethernet Port Bytes Rate",
		Units: "bytes/s",
		Fam:   "eth port %s",
		Type:  collectorapi.Area,
		Ctx:   "powerstore.eth_port_bytes",
		Dims: Dims{
			{ID: "eth_port_%s_bytes_rx_ps", Name: "received", Div: 1000},
			{ID: "eth_port_%s_bytes_tx_ps", Name: "sent", Mul: -1, Div: 1000},
		},
	},
	{
		ID:    "eth_port_%s_packets",
		Title: "Ethernet Port Packets Rate",
		Units: "packets/s",
		Fam:   "eth port %s",
		Ctx:   "powerstore.eth_port_packets",
		Type:  collectorapi.Area,
		Dims: Dims{
			{ID: "eth_port_%s_pkt_rx_ps", Name: "received", Div: 1000},
			{ID: "eth_port_%s_pkt_tx_ps", Name: "sent", Mul: -1, Div: 1000},
		},
	},
	{
		ID:    "eth_port_%s_errors",
		Title: "Ethernet Port Errors Rate",
		Units: "errors/s",
		Fam:   "eth port %s",
		Ctx:   "powerstore.eth_port_errors",
		Dims: Dims{
			{ID: "eth_port_%s_pkt_rx_crc_error_ps", Name: "rx_crc", Div: 1000},
			{ID: "eth_port_%s_pkt_rx_no_buffer_error_ps", Name: "rx_no_buffer", Div: 1000},
			{ID: "eth_port_%s_pkt_tx_error_ps", Name: "tx_error", Div: 1000},
		},
	},
	{
		ID:    "eth_port_%s_link_status",
		Title: "Ethernet Port Link Status",
		Units: "status",
		Fam:   "eth port %s",
		Ctx:   "powerstore.eth_port_link_status",
		Dims: Dims{
			{ID: "eth_port_%s_link_up", Name: "up"},
		},
	},
}

// --- File System charts (per-filesystem, dynamic) ---

var fileSystemChartsTmpl = Charts{
	{
		ID:    "filesystem_%s_iops",
		Title: "File System IOPS",
		Units: "ops/s",
		Fam:   "filesystem %s",
		Type:  collectorapi.Area,
		Ctx:   "powerstore.filesystem_iops",
		Dims: Dims{
			{ID: "file_system_%s_perf_read_iops", Name: "read", Div: 1000},
			{ID: "file_system_%s_perf_write_iops", Name: "write", Mul: -1, Div: 1000},
		},
	},
	{
		ID:    "filesystem_%s_bandwidth",
		Title: "File System Bandwidth",
		Units: "bytes/s",
		Fam:   "filesystem %s",
		Type:  collectorapi.Area,
		Ctx:   "powerstore.filesystem_bandwidth",
		Dims: Dims{
			{ID: "file_system_%s_perf_read_bandwidth", Name: "read", Div: 1000},
			{ID: "file_system_%s_perf_write_bandwidth", Name: "write", Mul: -1, Div: 1000},
		},
	},
	{
		ID:    "filesystem_%s_latency",
		Title: "File System Latency",
		Units: "microseconds",
		Fam:   "filesystem %s",
		Ctx:   "powerstore.filesystem_latency",
		Dims: Dims{
			{ID: "file_system_%s_perf_avg_read_latency", Name: "read", Div: 1000},
			{ID: "file_system_%s_perf_avg_write_latency", Name: "write", Div: 1000},
			{ID: "file_system_%s_perf_avg_latency", Name: "avg", Div: 1000},
		},
	},
}

// --- Drive charts (per-drive, dynamic) ---

var driveChartsTmpl = Charts{
	{
		ID:    "drive_%s_endurance",
		Title: "Drive Endurance Remaining",
		Units: "percentage",
		Fam:   "drive %s",
		Ctx:   "powerstore.drive_endurance",
		Dims: Dims{
			{ID: "drive_%s_endurance_remaining", Name: "remaining", Div: 1000},
		},
	},
}

// --- Dynamic chart creation helpers ---

func newApplianceCharts(a client.Appliance) *Charts {
	charts := applianceChartsTmpl.Copy()
	for i, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, a.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, a.Name)
		chart.Priority = prioAppliance + i
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, a.ID)
		}
	}
	return charts
}

func newVolumeCharts(v client.Volume) *Charts {
	charts := volumeChartsTmpl.Copy()
	for i, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, v.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, v.Name)
		chart.Priority = prioVolume + i
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, v.ID)
		}
	}
	return charts
}

func newNodeCharts(n client.Node) *Charts {
	charts := nodeChartsTmpl.Copy()
	for i, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, n.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, n.Name)
		chart.Priority = prioNode + i
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, n.ID)
		}
	}
	return charts
}

func newFcPortCharts(p client.FcPort) *Charts {
	charts := fcPortChartsTmpl.Copy()
	for i, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, p.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, p.Name)
		chart.Priority = prioFcPort + i
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, p.ID)
		}
	}
	return charts
}

func newEthPortCharts(p client.EthPort) *Charts {
	charts := ethPortChartsTmpl.Copy()
	for i, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, p.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, p.Name)
		chart.Priority = prioEthPort + i
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, p.ID)
		}
	}
	return charts
}

func newFileSystemCharts(fs client.FileSystem) *Charts {
	charts := fileSystemChartsTmpl.Copy()
	for i, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, fs.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, fs.Name)
		chart.Priority = prioFileSystem + i
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, fs.ID)
		}
	}
	return charts
}

func newDriveCharts(d client.Hardware) *Charts {
	charts := driveChartsTmpl.Copy()
	for i, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, d.ID)
		chart.Fam = fmt.Sprintf(chart.Fam, d.Name)
		chart.Priority = prioDrive + i
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, d.ID)
		}
	}
	return charts
}

// --- Update dynamic charts ---

func (c *Collector) updateCharts() {
	for _, a := range c.discovered.appliances {
		if c.charted[a.ID] {
			continue
		}
		c.charted[a.ID] = true
		if err := c.Charts().Add(*newApplianceCharts(a)...); err != nil {
			c.Warningf("error adding charts for appliance %s: %v", a.Name, err)
		}
	}
	for _, v := range c.discovered.volumes {
		if c.charted[v.ID] {
			continue
		}
		c.charted[v.ID] = true
		if err := c.Charts().Add(*newVolumeCharts(v)...); err != nil {
			c.Warningf("error adding charts for volume %s: %v", v.Name, err)
		}
	}
	for _, n := range c.discovered.nodes {
		if c.charted[n.ID] {
			continue
		}
		c.charted[n.ID] = true
		if err := c.Charts().Add(*newNodeCharts(n)...); err != nil {
			c.Warningf("error adding charts for node %s: %v", n.Name, err)
		}
	}
	for _, p := range c.discovered.fcPorts {
		if c.charted[p.ID] {
			continue
		}
		c.charted[p.ID] = true
		if err := c.Charts().Add(*newFcPortCharts(p)...); err != nil {
			c.Warningf("error adding charts for FC port %s: %v", p.Name, err)
		}
	}
	for _, p := range c.discovered.ethPorts {
		if c.charted[p.ID] {
			continue
		}
		c.charted[p.ID] = true
		if err := c.Charts().Add(*newEthPortCharts(p)...); err != nil {
			c.Warningf("error adding charts for ETH port %s: %v", p.Name, err)
		}
	}
	for _, fs := range c.discovered.fileSystems {
		if c.charted[fs.ID] {
			continue
		}
		c.charted[fs.ID] = true
		if err := c.Charts().Add(*newFileSystemCharts(fs)...); err != nil {
			c.Warningf("error adding charts for filesystem %s: %v", fs.Name, err)
		}
	}
	for _, d := range c.discovered.drives {
		if c.charted[d.ID] {
			continue
		}
		c.charted[d.ID] = true
		if err := c.Charts().Add(*newDriveCharts(d)...); err != nil {
			c.Warningf("error adding charts for drive %s: %v", d.Name, err)
		}
	}
}
