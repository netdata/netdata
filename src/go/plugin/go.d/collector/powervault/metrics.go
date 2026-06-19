// SPDX-License-Identifier: GPL-3.0-or-later

package powervault

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

type collectorMetrics struct {
	controller controllerGauges
	volume     volumeGauges
	port       portGauges
	phy        phyGauges
	pool       poolGauges
	drive      driveGauges
	sensor     sensorGauges
	hardware   hardwareGauges
	system     systemGauges
}

// Controller performance: mix of gauges (instant) and counters (cumulative).
type controllerGauges struct {
	iops             metrix.SnapshotGaugeVec
	throughput       metrix.SnapshotGaugeVec
	cpuLoad          metrix.SnapshotGaugeVec
	writeCacheUsed   metrix.SnapshotGaugeVec
	forwardedCmds    metrix.SnapshotGaugeVec
	dataRead         metrix.SnapshotGaugeVec
	dataWritten      metrix.SnapshotGaugeVec
	readOps          metrix.SnapshotGaugeVec
	writeOps         metrix.SnapshotGaugeVec
	writeCacheHits   metrix.SnapshotGaugeVec
	writeCacheMisses metrix.SnapshotGaugeVec
	readCacheHits    metrix.SnapshotGaugeVec
	readCacheMisses  metrix.SnapshotGaugeVec
}

// Volume performance: same pattern as controller.
type volumeGauges struct {
	iops              metrix.SnapshotGaugeVec
	throughput        metrix.SnapshotGaugeVec
	writeCachePercent metrix.SnapshotGaugeVec
	dataRead          metrix.SnapshotGaugeVec
	dataWritten       metrix.SnapshotGaugeVec
	readOps           metrix.SnapshotGaugeVec
	writeOps          metrix.SnapshotGaugeVec
	writeCacheHits    metrix.SnapshotGaugeVec
	writeCacheMisses  metrix.SnapshotGaugeVec
	readCacheHits     metrix.SnapshotGaugeVec
	readCacheMisses   metrix.SnapshotGaugeVec
	tierSSD           metrix.SnapshotGaugeVec
	tierSAS           metrix.SnapshotGaugeVec
	tierSATA          metrix.SnapshotGaugeVec
}

// Port I/O counters (cumulative).
type portGauges struct {
	readOps     metrix.SnapshotGaugeVec
	writeOps    metrix.SnapshotGaugeVec
	dataRead    metrix.SnapshotGaugeVec
	dataWritten metrix.SnapshotGaugeVec
}

// SAS PHY error counters.
type phyGauges struct {
	disparityErrors metrix.SnapshotGaugeVec
	lostDwords      metrix.SnapshotGaugeVec
	invalidDwords   metrix.SnapshotGaugeVec
}

// Pool capacity.
type poolGauges struct {
	totalBytes     metrix.SnapshotGaugeVec
	availableBytes metrix.SnapshotGaugeVec
}

// Per-drive metrics.
type driveGauges struct {
	temperature  metrix.SnapshotGaugeVec
	powerOnHours metrix.SnapshotGaugeVec
	ssdLifeLeft  metrix.SnapshotGaugeVec
}

// Per-sensor metrics by type.
type sensorGauges struct {
	temperature    metrix.SnapshotGaugeVec
	voltage        metrix.SnapshotGaugeVec
	current        metrix.SnapshotGaugeVec
	chargeCapacity metrix.SnapshotGaugeVec
}

// Hardware health counts (global).
type hardwareGauges struct {
	controllerOK, controllerDegraded, controllerFault, controllerUnknown metrix.SnapshotGauge
	driveOK, driveDegraded, driveFault, driveUnknown                     metrix.SnapshotGauge
	fanOK, fanDegraded, fanFault, fanUnknown                             metrix.SnapshotGauge
	psuOK, psuDegraded, psuFault, psuUnknown                             metrix.SnapshotGauge
	fruOK, fruDegraded, fruFault, fruUnknown                             metrix.SnapshotGauge
	portOK, portDegraded, portFault, portUnknown                         metrix.SnapshotGauge
}

// System-level health.
type systemGauges struct {
	health metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) *collectorMetrics {
	meter := store.Write().SnapshotMeter("")

	ctrlVec := meter.Vec("controller")
	volVec := meter.Vec("volume")
	portVec := meter.Vec("port")
	phyVec := meter.Vec("port") // PHY errors aggregated per port
	poolVec := meter.Vec("pool")
	driveVec := meter.Vec("drive")
	sensorVec := meter.Vec("sensor")

	return &collectorMetrics{
		controller: controllerGauges{
			iops:             ctrlVec.Gauge("controller_iops"),
			throughput:       ctrlVec.Gauge("controller_throughput"),
			cpuLoad:          ctrlVec.Gauge("controller_cpu_load"),
			writeCacheUsed:   ctrlVec.Gauge("controller_write_cache_used"),
			forwardedCmds:    ctrlVec.Gauge("controller_forwarded_cmds"),
			dataRead:         ctrlVec.Gauge("controller_data_read"),
			dataWritten:      ctrlVec.Gauge("controller_data_written"),
			readOps:          ctrlVec.Gauge("controller_read_ops"),
			writeOps:         ctrlVec.Gauge("controller_write_ops"),
			writeCacheHits:   ctrlVec.Gauge("controller_write_cache_hits"),
			writeCacheMisses: ctrlVec.Gauge("controller_write_cache_misses"),
			readCacheHits:    ctrlVec.Gauge("controller_read_cache_hits"),
			readCacheMisses:  ctrlVec.Gauge("controller_read_cache_misses"),
		},
		volume: volumeGauges{
			iops:              volVec.Gauge("volume_iops"),
			throughput:        volVec.Gauge("volume_throughput"),
			writeCachePercent: volVec.Gauge("volume_write_cache_percent"),
			dataRead:          volVec.Gauge("volume_data_read"),
			dataWritten:       volVec.Gauge("volume_data_written"),
			readOps:           volVec.Gauge("volume_read_ops"),
			writeOps:          volVec.Gauge("volume_write_ops"),
			writeCacheHits:    volVec.Gauge("volume_write_cache_hits"),
			writeCacheMisses:  volVec.Gauge("volume_write_cache_misses"),
			readCacheHits:     volVec.Gauge("volume_read_cache_hits"),
			readCacheMisses:   volVec.Gauge("volume_read_cache_misses"),
			tierSSD:           volVec.Gauge("volume_tier_ssd"),
			tierSAS:           volVec.Gauge("volume_tier_sas"),
			tierSATA:          volVec.Gauge("volume_tier_sata"),
		},
		port: portGauges{
			readOps:     portVec.Gauge("port_read_ops"),
			writeOps:    portVec.Gauge("port_write_ops"),
			dataRead:    portVec.Gauge("port_data_read"),
			dataWritten: portVec.Gauge("port_data_written"),
		},
		phy: phyGauges{
			disparityErrors: phyVec.Gauge("phy_disparity_errors"),
			lostDwords:      phyVec.Gauge("phy_lost_dwords"),
			invalidDwords:   phyVec.Gauge("phy_invalid_dwords"),
		},
		pool: poolGauges{
			totalBytes:     poolVec.Gauge("pool_total_bytes"),
			availableBytes: poolVec.Gauge("pool_available_bytes"),
		},
		drive: driveGauges{
			temperature:  driveVec.Gauge("drive_temperature"),
			powerOnHours: driveVec.Gauge("drive_power_on_hours"),
			ssdLifeLeft:  driveVec.Gauge("drive_ssd_life_left"),
		},
		sensor: sensorGauges{
			temperature:    sensorVec.Gauge("sensor_temperature"),
			voltage:        sensorVec.Gauge("sensor_voltage"),
			current:        sensorVec.Gauge("sensor_current"),
			chargeCapacity: sensorVec.Gauge("sensor_charge_capacity"),
		},
		hardware: hardwareGauges{
			controllerOK:       meter.Gauge("hw_controller_ok"),
			controllerDegraded: meter.Gauge("hw_controller_degraded"),
			controllerFault:    meter.Gauge("hw_controller_fault"),
			controllerUnknown:  meter.Gauge("hw_controller_unknown"),
			driveOK:            meter.Gauge("hw_drive_ok"),
			driveDegraded:      meter.Gauge("hw_drive_degraded"),
			driveFault:         meter.Gauge("hw_drive_fault"),
			driveUnknown:       meter.Gauge("hw_drive_unknown"),
			fanOK:              meter.Gauge("hw_fan_ok"),
			fanDegraded:        meter.Gauge("hw_fan_degraded"),
			fanFault:           meter.Gauge("hw_fan_fault"),
			fanUnknown:         meter.Gauge("hw_fan_unknown"),
			psuOK:              meter.Gauge("hw_psu_ok"),
			psuDegraded:        meter.Gauge("hw_psu_degraded"),
			psuFault:           meter.Gauge("hw_psu_fault"),
			psuUnknown:         meter.Gauge("hw_psu_unknown"),
			fruOK:              meter.Gauge("hw_fru_ok"),
			fruDegraded:        meter.Gauge("hw_fru_degraded"),
			fruFault:           meter.Gauge("hw_fru_fault"),
			fruUnknown:         meter.Gauge("hw_fru_unknown"),
			portOK:             meter.Gauge("hw_port_ok"),
			portDegraded:       meter.Gauge("hw_port_degraded"),
			portFault:          meter.Gauge("hw_port_fault"),
			portUnknown:        meter.Gauge("hw_port_unknown"),
		},
		system: systemGauges{
			health: meter.Gauge("system_health"),
		},
	}
}
