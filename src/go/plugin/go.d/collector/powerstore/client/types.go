// SPDX-License-Identifier: GPL-3.0-or-later

package client

// Cluster represents a PowerStore cluster.
type Cluster struct {
	ID                string `json:"id"`
	GlobalID          string `json:"global_id"`
	Name              string `json:"name"`
	ManagementAddress string `json:"management_address"`
	State             string `json:"state"`
}

// Appliance represents a PowerStore appliance.
type Appliance struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ServiceTag string `json:"service_tag,omitempty"`
}

// Volume represents a PowerStore volume.
type Volume struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	LogicalUsed int64  `json:"logical_used"`
	State       string `json:"state"`
	Type        string `json:"type"`
	ApplianceID string `json:"appliance_id"`
}

// Hardware represents a PowerStore hardware component.
type Hardware struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	LifecycleState string `json:"lifecycle_state"`
	StatusLED      string `json:"status_led,omitempty"`
	ApplianceID    string `json:"appliance_id,omitempty"`
	PartNumber     string `json:"part_number,omitempty"`
	SerialNumber   string `json:"serial_number,omitempty"`
	Slot           int    `json:"slot,omitempty"`
}

// Alert represents a PowerStore alert.
type Alert struct {
	ID                 string `json:"id"`
	Severity           string `json:"severity"`
	State              string `json:"state"`
	ResourceType       string `json:"resource_type"`
	ResourceName       string `json:"resource_name"`
	Description        string `json:"description_l10n"`
	GeneratedTimestamp string `json:"generated_timestamp"`
	IsAcknowledged     bool   `json:"is_acknowledged"`
}

// FcPort represents a PowerStore Fibre Channel port.
type FcPort struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ApplianceID  string `json:"appliance_id"`
	NodeID       string `json:"node_id"`
	IsLinkUp     bool   `json:"is_link_up"`
	CurrentSpeed string `json:"current_speed"`
	Wwn          string `json:"wwn"`
}

// EthPort represents a PowerStore Ethernet port.
type EthPort struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ApplianceID  string `json:"appliance_id"`
	NodeID       string `json:"node_id"`
	IsLinkUp     bool   `json:"is_link_up"`
	CurrentSpeed string `json:"current_speed"`
	MacAddress   string `json:"mac_address"`
	CurrentMTU   int32  `json:"current_mtu"`
}

// FileSystem represents a PowerStore file system.
type FileSystem struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	NasServerID    string `json:"nas_server_id"`
	SizeTotal      int64  `json:"size_total"`
	SizeUsed       int64  `json:"size_used"`
	FilesystemType string `json:"filesystem_type"`
}

// NAS represents a PowerStore NAS server.
type NAS struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	OperationalStatus string `json:"operational_status"`
	CurrentNodeID     string `json:"current_node_id"`
}

// Node represents a PowerStore node (queried from hardware endpoint).
type Node struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ApplianceID string `json:"appliance_id"`
}

// MetricsRequest is the POST body for /api/rest/metrics/generate.
type MetricsRequest struct {
	Entity   string `json:"entity"`
	EntityID string `json:"entity_id"`
	Interval string `json:"interval"`
}

// PerformanceMetrics contains common performance fields shared by most entity types.
type PerformanceMetrics struct {
	ApplianceID string `json:"appliance_id,omitempty"`
	NodeID      string `json:"node_id,omitempty"`
	VolumeID    string `json:"volume_id,omitempty"`
	FePortID    string `json:"fe_port_id,omitempty"`

	// Current values
	ReadIops       float64 `json:"read_iops"`
	WriteIops      float64 `json:"write_iops"`
	TotalIops      float64 `json:"total_iops"`
	ReadBandwidth  float64 `json:"read_bandwidth"`
	WriteBandwidth float64 `json:"write_bandwidth"`
	TotalBandwidth float64 `json:"total_bandwidth"`

	// Averages
	AvgReadLatency  float64 `json:"avg_read_latency"`
	AvgWriteLatency float64 `json:"avg_write_latency"`
	AvgLatency      float64 `json:"avg_latency"`
	AvgIoSize       float64 `json:"avg_io_size"`
	AvgReadIops     float64 `json:"avg_read_iops"`
	AvgWriteIops    float64 `json:"avg_write_iops"`

	// Appliance-specific
	IoWorkloadCPUUtilization    float64 `json:"io_workload_cpu_utilization,omitempty"`
	AvgIoWorkloadCPUUtilization float64 `json:"avg_io_workload_cpu_utilization,omitempty"`

	// Node-specific
	CurrentLogins *int64 `json:"current_logins,omitempty"`
}

// EthPortMetrics contains Ethernet port performance fields.
type EthPortMetrics struct {
	ApplianceID string `json:"appliance_id,omitempty"`
	FePortID    string `json:"fe_port_id,omitempty"`
	NodeID      string `json:"node_id,omitempty"`

	BytesRxPs            float64 `json:"bytes_rx_ps"`
	BytesTxPs            float64 `json:"bytes_tx_ps"`
	PktRxPs              float64 `json:"pkt_rx_ps"`
	PktTxPs              float64 `json:"pkt_tx_ps"`
	PktRxCrcErrorPs      float64 `json:"pkt_rx_crc_error_ps"`
	PktRxNoBufferErrorPs float64 `json:"pkt_rx_no_buffer_error_ps"`
	PktTxErrorPs         float64 `json:"pkt_tx_error_ps"`
}

// SpaceMetrics contains capacity/space fields.
type SpaceMetrics struct {
	ClusterID   string `json:"cluster_id,omitempty"`
	ApplianceID string `json:"appliance_id,omitempty"`
	VolumeID    string `json:"volume_id,omitempty"`

	PhysicalTotal      *int64 `json:"physical_total,omitempty"`
	PhysicalUsed       *int64 `json:"physical_used,omitempty"`
	LogicalProvisioned *int64 `json:"logical_provisioned,omitempty"`
	LogicalUsed        *int64 `json:"logical_used,omitempty"`
	DataPhysicalUsed   *int64 `json:"data_physical_used,omitempty"`
	SharedLogicalUsed  *int64 `json:"shared_logical_used,omitempty"`

	EfficiencyRatio float64 `json:"efficiency_ratio"`
	DataReduction   float64 `json:"data_reduction"`
	SnapshotSavings float64 `json:"snapshot_savings"`
	ThinSavings     float64 `json:"thin_savings"`
}

// WearMetrics contains drive wear/endurance fields.
type WearMetrics struct {
	DriveID                   string  `json:"drive_id"`
	PercentEnduranceRemaining float64 `json:"percent_endurance_remaining"`
}

// CopyMetrics contains replication/copy fields.
type CopyMetrics struct {
	ApplianceID     string  `json:"appliance_id,omitempty"`
	DataRemaining   *int64  `json:"data_remaining,omitempty"`
	DataTransferred *int64  `json:"data_transferred,omitempty"`
	TransferRate    float64 `json:"transfer_rate"`
}

// FileSystemMetrics contains file system performance fields.
type FileSystemMetrics struct {
	FileSystemID string `json:"file_system_id"`

	ReadIops        float64 `json:"read_iops"`
	WriteIops       float64 `json:"write_iops"`
	TotalIops       float64 `json:"total_iops"`
	ReadBandwidth   float64 `json:"read_bandwidth"`
	WriteBandwidth  float64 `json:"write_bandwidth"`
	TotalBandwidth  float64 `json:"total_bandwidth"`
	AvgReadLatency  float64 `json:"avg_read_latency"`
	AvgWriteLatency float64 `json:"avg_write_latency"`
	AvgLatency      float64 `json:"avg_latency"`
}
