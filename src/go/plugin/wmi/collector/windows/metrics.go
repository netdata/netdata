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

// write records the valid samples. Label sets are built per poll, so an
// interface or drive that disappears stops being observed.
func (m collectorMetrics) write(snapshot observations) {
	observe(m.cpuBusy, snapshot.cpuBusy)
	observe(m.memoryUsed, snapshot.memoryUsed)
	observe(m.memoryAvailable, snapshot.memoryAvailable)
	for _, nic := range snapshot.interfaces {
		labels := m.meter.LabelSet(metrix.Label{
			Key:   interfaceLabel,
			Value: nic.name,
		})
		observe(m.networkReceived, nic.received, labels)
		observe(m.networkSent, nic.sent, labels)
	}
	for _, drive := range snapshot.drives {
		labels := m.meter.LabelSet(metrix.Label{
			Key:   driveLabel,
			Value: drive.drive,
		})
		observe(m.diskUsed, drive.used, labels)
		observe(m.diskFree, drive.free, labels)
	}
}

func observe(gauge metrix.SnapshotGauge, s sample, labels ...metrix.LabelSet) {
	if s.valid {
		gauge.Observe(float64(s.value), labels...)
	}
}
