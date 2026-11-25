// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

const (
	interfaceLabel = "interface"
	driveLabel     = "drive"
)

type collectorMetrics struct {
	meter           metrix.SnapshotMeter
	cpuBusy         metrix.SnapshotGauge
	memoryUsed      metrix.SnapshotGauge
	memoryAvailable metrix.SnapshotGauge
	networkReceived metrix.SnapshotGauge
	networkSent     metrix.SnapshotGauge
	diskUsed        metrix.SnapshotGauge
	diskFree        metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) collectorMetrics {
	m := store.Write().SnapshotMeter("")
	return collectorMetrics{
		meter:           m,
		cpuBusy:         m.Gauge("cpu_busy_percent"),
		memoryUsed:      m.Gauge("memory_used_bytes"),
		memoryAvailable: m.Gauge("memory_available_bytes"),
		networkReceived: m.Gauge("network_received_bytes_per_second"),
		networkSent:     m.Gauge("network_sent_bytes_per_second"),
		diskUsed:        m.Gauge("disk_used_bytes"),
		diskFree:        m.Gauge("disk_free_bytes"),
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
		labels := m.meter.LabelSet(metrix.Label{Key: interfaceLabel, Value: nic.name})
		if nic.received.valid {
			m.networkReceived.Observe(float64(nic.received.value), labels)
		}
		if nic.sent.valid {
			m.networkSent.Observe(float64(nic.sent.value), labels)
		}
	}
	for _, drive := range snapshot.drives {
		labels := m.meter.LabelSet(metrix.Label{Key: driveLabel, Value: drive.drive})
		if drive.used.valid {
			m.diskUsed.Observe(float64(drive.used.value), labels)
		}
		if drive.free.valid {
			m.diskFree.Observe(float64(drive.free.value), labels)
		}
	}
}
