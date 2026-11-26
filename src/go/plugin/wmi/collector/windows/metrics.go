// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import "github.com/netdata/netdata/go/plugins/pkg/metrix"

// Chart label keys and fixed values of the native Windows Agent's charts.
const (
	cpuLabel           = "cpu"
	deviceLabel        = "device"
	interfaceTypeLabel = "interface_type"
	mountPointLabel    = "mount_point"
	driveTypeLabel     = "drive_type"
	fileSystemLabel    = "filesystem"
	serialNumberLabel  = "serial_number"

	// The Network Interface counter set lists the interfaces the native agent
	// calls real; it calls the extra Network Adapter instances virtual.
	realInterface = "real"
	// The disk query reads fixed drives only.
	fixedDrive = "fixed"
)

type collectorMetrics struct {
	meter       metrix.SnapshotMeter
	cpuTotal    [len(cpuModes)]metrix.SnapshotGauge
	cpuCore     [len(cpuModes)]metrix.SnapshotGauge
	memoryFree  metrix.SnapshotGauge
	memoryUsed  metrix.SnapshotGauge
	netReceived metrix.SnapshotCounter
	netSent     metrix.SnapshotCounter
	diskAvail   metrix.SnapshotGauge
	diskUsed    metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) collectorMetrics {
	m := store.Write().SnapshotMeter("")
	metrics := collectorMetrics{
		meter:       m,
		memoryFree:  m.Gauge("memory_free_bytes"),
		memoryUsed:  m.Gauge("memory_used_bytes"),
		netReceived: m.Counter("network_received_bytes"),
		netSent:     m.Counter("network_sent_bytes"),
		diskAvail:   m.Gauge("disk_avail_bytes"),
		diskUsed:    m.Gauge("disk_used_bytes"),
	}
	for i, mode := range cpuModes {
		metrics.cpuTotal[i] = m.Gauge("cpu_total_"+mode+"_percent", metrix.WithFloat(true))
		metrics.cpuCore[i] = m.Gauge("cpu_core_"+mode+"_percent", metrix.WithFloat(true))
	}
	return metrics
}

// write records the valid samples. Label sets are built per poll, so a
// processor, interface or drive that disappears stops being observed. Byte
// totals are counters: Netdata computes the traffic rate over each collection
// interval.
func (m collectorMetrics) write(snapshot observations) {
	if snapshot.cpu.hasTotal {
		observeShares(m.cpuTotal, snapshot.cpu.total)
	}
	for cpu, shares := range snapshot.cpu.cores {
		labels := m.meter.LabelSet(metrix.Label{
			Key:   cpuLabel,
			Value: cpu,
		})
		observeShares(m.cpuCore, shares, labels)
	}
	observe(m.memoryFree, snapshot.memoryFree)
	observe(m.memoryUsed, snapshot.memoryUsed)
	for _, nic := range snapshot.interfaces {
		labels := m.meter.LabelSet(
			metrix.Label{
				Key:   deviceLabel,
				Value: nic.device,
			},
			metrix.Label{
				Key:   interfaceTypeLabel,
				Value: realInterface,
			},
		)
		observeTotal(m.netReceived, nic.received, labels)
		observeTotal(m.netSent, nic.sent, labels)
	}
	for _, drive := range snapshot.drives {
		labels := m.meter.LabelSet(drive.labels()...)
		observe(m.diskAvail, drive.avail, labels)
		observe(m.diskUsed, drive.used, labels)
	}
}

func (d diskSpace) labels() []metrix.Label {
	labels := []metrix.Label{
		{
			Key:   mountPointLabel,
			Value: d.mountPoint,
		},
		{
			Key:   driveTypeLabel,
			Value: fixedDrive,
		},
		{
			Key:   fileSystemLabel,
			Value: d.fileSystem,
		},
	}
	if d.serialNumber != "" {
		labels = append(labels, metrix.Label{
			Key:   serialNumberLabel,
			Value: d.serialNumber,
		})
	}
	return labels
}

func observeShares(gauges [len(cpuModes)]metrix.SnapshotGauge, shares cpuShares, labels ...metrix.LabelSet) {
	for i, share := range shares {
		gauges[i].Observe(share, labels...)
	}
}

func observe(gauge metrix.SnapshotGauge, s sample, labels ...metrix.LabelSet) {
	if s.valid {
		gauge.Observe(float64(s.value), labels...)
	}
}

func observeTotal(counter metrix.SnapshotCounter, s sample, labels ...metrix.LabelSet) {
	if s.valid {
		counter.ObserveTotal(float64(s.value), labels...)
	}
}
