// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

const (
	interfaceLabel = "interface"
	driveLabel     = "drive"
)

type collectorMetrics struct {
	cpuBusy         metrix.SnapshotGauge
	memoryUsed      metrix.SnapshotGauge
	memoryAvailable metrix.SnapshotGauge
	networkReceived metrix.SnapshotGaugeVec
	networkSent     metrix.SnapshotGaugeVec
	diskUsed        metrix.SnapshotGaugeVec
	diskFree        metrix.SnapshotGaugeVec
}

func newCollectorMetrics(store metrix.CollectorStore) collectorMetrics {
	m := store.Write().SnapshotMeter("")
	network := m.Vec(interfaceLabel)
	disk := m.Vec(driveLabel)
	return collectorMetrics{
		cpuBusy:         m.Gauge("cpu_busy_percent"),
		memoryUsed:      m.Gauge("memory_used_bytes"),
		memoryAvailable: m.Gauge("memory_available_bytes"),
		networkReceived: network.Gauge("network_received_bytes_per_second"),
		networkSent:     network.Gauge("network_sent_bytes_per_second"),
		diskUsed:        disk.Gauge("disk_used_bytes"),
		diskFree:        disk.Gauge("disk_free_bytes"),
	}
}

func (m collectorMetrics) write(snapshot observations) {
	if snapshot.cpu.valid {
		m.cpuBusy.Observe(float64(snapshot.cpu.value))
	}
	if snapshot.memoryUsed.valid {
		m.memoryUsed.Observe(float64(snapshot.memoryUsed.value) * 1024)
	}
	if snapshot.memoryAvailable.valid {
		m.memoryAvailable.Observe(float64(snapshot.memoryAvailable.value) * 1024)
	}
	for _, nic := range snapshot.interfaces {
		if nic.received.valid {
			m.networkReceived.WithLabelValues(nic.name).Observe(float64(nic.received.value))
		}
		if nic.sent.valid {
			m.networkSent.WithLabelValues(nic.name).Observe(float64(nic.sent.value))
		}
	}
	for _, drive := range snapshot.drives {
		if drive.used.valid {
			m.diskUsed.WithLabelValues(drive.drive).Observe(float64(drive.used.value))
		}
		if drive.free.valid {
			m.diskFree.WithLabelValues(drive.drive).Observe(float64(drive.free.value))
		}
	}
}
