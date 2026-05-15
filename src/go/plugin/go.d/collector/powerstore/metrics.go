// SPDX-License-Identifier: GPL-3.0-or-later

package powerstore

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

type collectorMetrics struct {
	cluster     clusterGauges
	appliance   applianceGauges
	volume      volumeGauges
	node        nodeGauges
	fcPort      fcPortGauges
	ethPort     ethPortGauges
	fileSystem  fileSystemGauges
	hardware    hardwareGauges
	alerts      alertGauges
	drive       driveGauges
	nas         nasGauges
	replication replicationGauges
}

type clusterGauges struct {
	spacePhysicalTotal      metrix.SnapshotGauge
	spacePhysicalUsed       metrix.SnapshotGauge
	spaceLogicalProvisioned metrix.SnapshotGauge
	spaceLogicalUsed        metrix.SnapshotGauge
	spaceDataPhysicalUsed   metrix.SnapshotGauge
	spaceSharedLogicalUsed  metrix.SnapshotGauge
	spaceEfficiencyRatio    metrix.SnapshotGauge
	spaceDataReduction      metrix.SnapshotGauge
	spaceSnapshotSavings    metrix.SnapshotGauge
	spaceThinSavings        metrix.SnapshotGauge
}

type perfGaugeVecs struct {
	readIops        metrix.SnapshotGaugeVec
	writeIops       metrix.SnapshotGaugeVec
	readBandwidth   metrix.SnapshotGaugeVec
	writeBandwidth  metrix.SnapshotGaugeVec
	avgReadLatency  metrix.SnapshotGaugeVec
	avgWriteLatency metrix.SnapshotGaugeVec
	avgLatency      metrix.SnapshotGaugeVec
}

type spaceGaugeVecs struct {
	physicalTotal      metrix.SnapshotGaugeVec
	physicalUsed       metrix.SnapshotGaugeVec
	logicalProvisioned metrix.SnapshotGaugeVec
	logicalUsed        metrix.SnapshotGaugeVec
	dataPhysicalUsed   metrix.SnapshotGaugeVec
	sharedLogicalUsed  metrix.SnapshotGaugeVec
	efficiencyRatio    metrix.SnapshotGaugeVec
	dataReduction      metrix.SnapshotGaugeVec
	snapshotSavings    metrix.SnapshotGaugeVec
	thinSavings        metrix.SnapshotGaugeVec
}

type applianceGauges struct {
	perf  perfGaugeVecs
	space spaceGaugeVecs
	cpu   metrix.SnapshotGaugeVec
}

type volumeGauges struct {
	perf             perfGaugeVecs
	spaceLogicalProv metrix.SnapshotGaugeVec
	spaceLogicalUsed metrix.SnapshotGaugeVec
	spaceThinSavings metrix.SnapshotGaugeVec
}

type nodeGauges struct {
	perf          perfGaugeVecs
	currentLogins metrix.SnapshotGaugeVec
}

type fcPortGauges struct {
	perf   perfGaugeVecs
	linkUp metrix.SnapshotGaugeVec
}

type ethPortGauges struct {
	bytesRxPs            metrix.SnapshotGaugeVec
	bytesTxPs            metrix.SnapshotGaugeVec
	pktRxPs              metrix.SnapshotGaugeVec
	pktTxPs              metrix.SnapshotGaugeVec
	pktRxCrcErrorPs      metrix.SnapshotGaugeVec
	pktRxNoBufferErrorPs metrix.SnapshotGaugeVec
	pktTxErrorPs         metrix.SnapshotGaugeVec
	linkUp               metrix.SnapshotGaugeVec
}

type fileSystemGauges struct {
	perf perfGaugeVecs
}

type hardwareGauges struct {
	fanOK, fanDegraded, fanFailed, fanUnknown                 metrix.SnapshotGauge
	psuOK, psuDegraded, psuFailed, psuUnknown                 metrix.SnapshotGauge
	driveOK, driveDegraded, driveFailed, driveUnknown         metrix.SnapshotGauge
	batteryOK, batteryDegraded, batteryFailed, batteryUnknown metrix.SnapshotGauge
	nodeOK, nodeDegraded, nodeFailed, nodeUnknown             metrix.SnapshotGauge
}

type alertGauges struct {
	critical metrix.SnapshotGauge
	major    metrix.SnapshotGauge
	minor    metrix.SnapshotGauge
	info     metrix.SnapshotGauge
}

type driveGauges struct {
	enduranceRemaining metrix.SnapshotGaugeVec
}

type nasGauges struct {
	started  metrix.SnapshotGauge
	stopped  metrix.SnapshotGauge
	degraded metrix.SnapshotGauge
	unknown  metrix.SnapshotGauge
}

type replicationGauges struct {
	dataRemaining   metrix.SnapshotGauge
	dataTransferred metrix.SnapshotGauge
	transferRate    metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	meter := store.Write().SnapshotMeter("")

	appVec := meter.Vec("appliance")
	volVec := meter.Vec("volume")
	nodeVec := meter.Vec("node")
	fcVec := meter.Vec("fc_port")
	ethVec := meter.Vec("eth_port")
	fsVec := meter.Vec("filesystem")
	driveVec := meter.Vec("drive")

	newPerfVecs := func(vec metrix.SnapshotVecMeter, prefix string) perfGaugeVecs {
		return perfGaugeVecs{
			readIops:        vec.Gauge(prefix + "_perf_read_iops"),
			writeIops:       vec.Gauge(prefix + "_perf_write_iops"),
			readBandwidth:   vec.Gauge(prefix + "_perf_read_bandwidth"),
			writeBandwidth:  vec.Gauge(prefix + "_perf_write_bandwidth"),
			avgReadLatency:  vec.Gauge(prefix + "_perf_avg_read_latency"),
			avgWriteLatency: vec.Gauge(prefix + "_perf_avg_write_latency"),
			avgLatency:      vec.Gauge(prefix + "_perf_avg_latency"),
		}
	}

	newSpaceVecs := func(vec metrix.SnapshotVecMeter, prefix string) spaceGaugeVecs {
		return spaceGaugeVecs{
			physicalTotal:      vec.Gauge(prefix + "_space_physical_total"),
			physicalUsed:       vec.Gauge(prefix + "_space_physical_used"),
			logicalProvisioned: vec.Gauge(prefix + "_space_logical_provisioned"),
			logicalUsed:        vec.Gauge(prefix + "_space_logical_used"),
			dataPhysicalUsed:   vec.Gauge(prefix + "_space_data_physical_used"),
			sharedLogicalUsed:  vec.Gauge(prefix + "_space_shared_logical_used"),
			efficiencyRatio:    vec.Gauge(prefix + "_space_efficiency_ratio"),
			dataReduction:      vec.Gauge(prefix + "_space_data_reduction"),
			snapshotSavings:    vec.Gauge(prefix + "_space_snapshot_savings"),
			thinSavings:        vec.Gauge(prefix + "_space_thin_savings"),
		}
	}

	return &collectorMetrics{
		cluster: clusterGauges{
			spacePhysicalTotal:      meter.Gauge("cluster_space_physical_total"),
			spacePhysicalUsed:       meter.Gauge("cluster_space_physical_used"),
			spaceLogicalProvisioned: meter.Gauge("cluster_space_logical_provisioned"),
			spaceLogicalUsed:        meter.Gauge("cluster_space_logical_used"),
			spaceDataPhysicalUsed:   meter.Gauge("cluster_space_data_physical_used"),
			spaceSharedLogicalUsed:  meter.Gauge("cluster_space_shared_logical_used"),
			spaceEfficiencyRatio:    meter.Gauge("cluster_space_efficiency_ratio"),
			spaceDataReduction:      meter.Gauge("cluster_space_data_reduction"),
			spaceSnapshotSavings:    meter.Gauge("cluster_space_snapshot_savings"),
			spaceThinSavings:        meter.Gauge("cluster_space_thin_savings"),
		},
		appliance: applianceGauges{
			perf:  newPerfVecs(appVec, "appliance"),
			space: newSpaceVecs(appVec, "appliance"),
			cpu:   appVec.Gauge("appliance_cpu_utilization"),
		},
		volume: volumeGauges{
			perf:             newPerfVecs(volVec, "volume"),
			spaceLogicalProv: volVec.Gauge("volume_space_logical_provisioned"),
			spaceLogicalUsed: volVec.Gauge("volume_space_logical_used"),
			spaceThinSavings: volVec.Gauge("volume_space_thin_savings"),
		},
		node: nodeGauges{
			perf:          newPerfVecs(nodeVec, "node"),
			currentLogins: nodeVec.Gauge("node_current_logins"),
		},
		fcPort: fcPortGauges{
			perf:   newPerfVecs(fcVec, "fc_port"),
			linkUp: fcVec.Gauge("fc_port_link_up"),
		},
		ethPort: ethPortGauges{
			bytesRxPs:            ethVec.Gauge("eth_port_bytes_rx_ps"),
			bytesTxPs:            ethVec.Gauge("eth_port_bytes_tx_ps"),
			pktRxPs:              ethVec.Gauge("eth_port_pkt_rx_ps"),
			pktTxPs:              ethVec.Gauge("eth_port_pkt_tx_ps"),
			pktRxCrcErrorPs:      ethVec.Gauge("eth_port_pkt_rx_crc_error_ps"),
			pktRxNoBufferErrorPs: ethVec.Gauge("eth_port_pkt_rx_no_buffer_error_ps"),
			pktTxErrorPs:         ethVec.Gauge("eth_port_pkt_tx_error_ps"),
			linkUp:               ethVec.Gauge("eth_port_link_up"),
		},
		fileSystem: fileSystemGauges{
			perf: newPerfVecs(fsVec, "file_system"),
		},
		hardware: hardwareGauges{
			fanOK: meter.Gauge("hardware_fan_ok"), fanDegraded: meter.Gauge("hardware_fan_degraded"),
			fanFailed: meter.Gauge("hardware_fan_failed"), fanUnknown: meter.Gauge("hardware_fan_unknown"),
			psuOK: meter.Gauge("hardware_psu_ok"), psuDegraded: meter.Gauge("hardware_psu_degraded"),
			psuFailed: meter.Gauge("hardware_psu_failed"), psuUnknown: meter.Gauge("hardware_psu_unknown"),
			driveOK: meter.Gauge("hardware_drive_ok"), driveDegraded: meter.Gauge("hardware_drive_degraded"),
			driveFailed: meter.Gauge("hardware_drive_failed"), driveUnknown: meter.Gauge("hardware_drive_unknown"),
			batteryOK: meter.Gauge("hardware_battery_ok"), batteryDegraded: meter.Gauge("hardware_battery_degraded"),
			batteryFailed: meter.Gauge("hardware_battery_failed"), batteryUnknown: meter.Gauge("hardware_battery_unknown"),
			nodeOK: meter.Gauge("hardware_node_ok"), nodeDegraded: meter.Gauge("hardware_node_degraded"),
			nodeFailed: meter.Gauge("hardware_node_failed"), nodeUnknown: meter.Gauge("hardware_node_unknown"),
		},
		alerts: alertGauges{
			critical: meter.Gauge("alerts_critical"),
			major:    meter.Gauge("alerts_major"),
			minor:    meter.Gauge("alerts_minor"),
			info:     meter.Gauge("alerts_info"),
		},
		drive: driveGauges{
			enduranceRemaining: driveVec.Gauge("drive_endurance_remaining"),
		},
		nas: nasGauges{
			started:  meter.Gauge("nas_started"),
			stopped:  meter.Gauge("nas_stopped"),
			degraded: meter.Gauge("nas_degraded"),
			unknown:  meter.Gauge("nas_unknown"),
		},
		replication: replicationGauges{
			dataRemaining:   meter.Gauge("copy_data_remaining"),
			dataTransferred: meter.Gauge("copy_data_transferred"),
			transferRate:    meter.Gauge("copy_transfer_rate"),
		},
	}
}
