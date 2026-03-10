// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

type metrics struct {
	Cluster    clusterMetrics                   `stm:"cluster"`
	Appliance  map[string]applianceMetrics      `stm:"appliance"`
	Volume     map[string]volumeMetrics         `stm:"volume"`
	Node       map[string]nodeMetrics           `stm:"node"`
	FcPort     map[string]fcPortMetrics         `stm:"fc_port"`
	EthPort    map[string]ethPortMetrics        `stm:"eth_port"`
	FileSystem map[string]fileSystemMetrics     `stm:"file_system"`
	Hardware   hardwareMetrics                  `stm:"hardware"`
	Alerts     alertMetrics                     `stm:"alerts"`
	Drive      map[string]driveMetrics          `stm:"drive"`
	NAS        nasStatusMetrics                 `stm:"nas"`
	Copy       copyFields                       `stm:"copy"`
}

type clusterMetrics struct {
	Space spaceFields `stm:"space"`
}

type spaceFields struct {
	PhysicalTotal      int64   `stm:"physical_total"`
	PhysicalUsed       int64   `stm:"physical_used"`
	LogicalProvisioned int64   `stm:"logical_provisioned"`
	LogicalUsed        int64   `stm:"logical_used"`
	DataPhysicalUsed   int64   `stm:"data_physical_used"`
	SharedLogicalUsed  int64   `stm:"shared_logical_used"`
	EfficiencyRatio    float64 `stm:"efficiency_ratio,1000,1"`
	DataReduction      float64 `stm:"data_reduction,1000,1"`
	SnapshotSavings    float64 `stm:"snapshot_savings,1000,1"`
	ThinSavings        float64 `stm:"thin_savings,1000,1"`
}

type perfFields struct {
	ReadIops       float64 `stm:"read_iops,1000,1"`
	WriteIops      float64 `stm:"write_iops,1000,1"`
	TotalIops      float64 `stm:"total_iops,1000,1"`
	ReadBandwidth  float64 `stm:"read_bandwidth,1000,1"`
	WriteBandwidth float64 `stm:"write_bandwidth,1000,1"`
	TotalBandwidth float64 `stm:"total_bandwidth,1000,1"`
	AvgReadLatency float64 `stm:"avg_read_latency,1000,1"`
	AvgWriteLatency float64 `stm:"avg_write_latency,1000,1"`
	AvgLatency     float64 `stm:"avg_latency,1000,1"`
}

type applianceMetrics struct {
	Perf  perfFields  `stm:"perf"`
	Space spaceFields `stm:"space"`
	CPU   float64     `stm:"cpu_utilization,1000,1"`
}

type volumeSpaceFields struct {
	LogicalProvisioned int64   `stm:"logical_provisioned"`
	LogicalUsed        int64   `stm:"logical_used"`
	ThinSavings        float64 `stm:"thin_savings,1000,1"`
}

type volumeMetrics struct {
	Perf  perfFields       `stm:"perf"`
	Space volumeSpaceFields `stm:"space"`
}

type nodeMetrics struct {
	Perf          perfFields `stm:"perf"`
	CurrentLogins int64      `stm:"current_logins"`
}

type fcPortMetrics struct {
	Perf   perfFields `stm:"perf"`
	LinkUp int64      `stm:"link_up"`
}

type ethPortMetrics struct {
	BytesRxPs            float64 `stm:"bytes_rx_ps,1000,1"`
	BytesTxPs            float64 `stm:"bytes_tx_ps,1000,1"`
	PktRxPs              float64 `stm:"pkt_rx_ps,1000,1"`
	PktTxPs              float64 `stm:"pkt_tx_ps,1000,1"`
	PktRxCrcErrorPs      float64 `stm:"pkt_rx_crc_error_ps,1000,1"`
	PktRxNoBufferErrorPs float64 `stm:"pkt_rx_no_buffer_error_ps,1000,1"`
	PktTxErrorPs         float64 `stm:"pkt_tx_error_ps,1000,1"`
	LinkUp               int64   `stm:"link_up"`
}

type fileSystemMetrics struct {
	Perf perfFields `stm:"perf"`
}

type hardwareMetrics struct {
	Fan   hwStateCount `stm:"fan"`
	PSU   hwStateCount `stm:"psu"`
	Drive hwStateCount `stm:"drive"`
	Batt  hwStateCount `stm:"battery"`
	Node  hwStateCount `stm:"node"`
}

type hwStateCount struct {
	OK       int64 `stm:"ok"`
	Degraded int64 `stm:"degraded"`
	Failed   int64 `stm:"failed"`
	Unknown  int64 `stm:"unknown"`
}

type alertMetrics struct {
	Critical int64 `stm:"critical"`
	Major    int64 `stm:"major"`
	Minor    int64 `stm:"minor"`
	Info     int64 `stm:"info"`
}

type driveMetrics struct {
	EnduranceRemaining float64 `stm:"endurance_remaining,1000,1"`
}

type nasStatusMetrics struct {
	Started  int64 `stm:"started"`
	Stopped  int64 `stm:"stopped"`
	Degraded int64 `stm:"degraded"`
	Unknown  int64 `stm:"unknown"`
}

type copyFields struct {
	DataRemaining   int64   `stm:"data_remaining"`
	DataTransferred int64   `stm:"data_transferred"`
	TransferRate    float64 `stm:"transfer_rate,1000,1"`
}
