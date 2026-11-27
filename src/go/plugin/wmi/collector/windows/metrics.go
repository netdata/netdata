// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
)

// Chart label keys and fixed values of the native Windows Agent's charts.
const (
	cpuLabel           = "cpu"
	deviceLabel        = "device"
	interfaceTypeLabel = "interface_type"
	mountPointLabel    = "mount_point"
	driveTypeLabel     = "drive_type"
	fileSystemLabel    = "filesystem"
	readWriteModeLabel = "rw_mode"
	serialNumberLabel  = "serial_number"
	pagefileLabel      = "pagefile"
	modelLabel         = "model"
	deviceIDLabel      = "device_id"
	numaNodeLabel      = "node"
	// The go.d ping collector's label: the target as configured.
	hostLabel = "host"

	realInterface    = "real"
	virtualInterface = "virtual"
	// The disk query reads fixed drives only and leaves out read-only ones.
	fixedDrive    = "fixed"
	readWriteMode = "rw"
)

type collectorMetrics struct {
	meter            metrix.SnapshotMeter
	cpuTotal         [len(cpuModes)]metrix.SnapshotGauge
	cpuCore          [len(cpuModes)]metrix.SnapshotGauge
	memoryFree       metrix.SnapshotGauge
	memoryUsed       metrix.SnapshotGauge
	swapFree         metrix.SnapshotGauge
	swapUsed         metrix.SnapshotGauge
	memoryCounters   [len(memoryCounters)]counterInstrument
	pageFaultsMinor  metrix.SnapshotCounter
	pageFaultsMajor  metrix.SnapshotCounter
	pagefileFree     metrix.SnapshotGauge
	pagefileUsed     metrix.SnapshotGauge
	numaFree         metrix.SnapshotGauge
	numaStandby      metrix.SnapshotGauge
	processes        metrix.SnapshotGauge
	threads          metrix.SnapshotGauge
	contextSwitches  metrix.SnapshotCounter
	processorQueue   metrix.SnapshotGauge
	uptime           metrix.SnapshotGauge
	netInterface     [len(interfaceCounters)]counterInstrument
	netSpeedKbps     metrix.SnapshotGauge
	netTotalReceived metrix.SnapshotCounter
	netTotalSent     metrix.SnapshotCounter
	tcp              [len(tcpCounters)]protocolInstruments
	udp              [len(udpCounters)]protocolInstruments
	diskAvail        metrix.SnapshotGauge
	diskUsed         metrix.SnapshotGauge
	diskTotalRead    metrix.SnapshotCounter
	diskTotalWritten metrix.SnapshotCounter
	disk             [len(diskCounters)]counterInstrument
	diskUtilization  metrix.SnapshotGauge
	diskReadAwait    metrix.SnapshotGauge
	diskWriteAwait   metrix.SnapshotGauge
	diskServiceTime  metrix.SnapshotGauge
	diskReadSize     metrix.SnapshotGauge
	diskWriteSize    metrix.SnapshotGauge
	pingRTTMin       metrix.SnapshotGauge
	pingRTTMax       metrix.SnapshotGauge
	pingRTTAvg       metrix.SnapshotGauge
	pingRTTDev       metrix.SnapshotGauge
	pingLoss         metrix.SnapshotGauge
}

func newCollectorMetrics(store metrix.CollectorStore) collectorMetrics {
	m := store.Write().SnapshotMeter("")
	metrics := collectorMetrics{
		meter:            m,
		memoryFree:       m.Gauge("memory_free_bytes"),
		memoryUsed:       m.Gauge("memory_used_bytes"),
		swapFree:         m.Gauge("memory_swap_free_bytes"),
		swapUsed:         m.Gauge("memory_swap_used_bytes"),
		pageFaultsMinor:  m.Counter("memory_page_faults_minor"),
		pageFaultsMajor:  m.Counter("memory_page_faults_major"),
		pagefileFree:     m.Gauge("pagefile_free_bytes"),
		pagefileUsed:     m.Gauge("pagefile_used_bytes"),
		numaFree:         m.Gauge("numa_node_free_and_zero_mebibytes"),
		numaStandby:      m.Gauge("numa_node_standby_mebibytes"),
		processes:        m.Gauge("system_processes"),
		threads:          m.Gauge("system_threads"),
		contextSwitches:  m.Counter("system_context_switches"),
		processorQueue:   m.Gauge("system_processor_queue_length"),
		uptime:           m.Gauge("system_uptime_milliseconds"),
		netTotalReceived: m.Counter("network_total_received_bytes"),
		netTotalSent:     m.Counter("network_total_sent_bytes"),
		diskAvail:        m.Gauge("disk_avail_bytes"),
		diskUsed:         m.Gauge("disk_used_bytes"),
		diskTotalRead:    m.Counter("disk_total_read_bytes"),
		diskTotalWritten: m.Counter("disk_total_written_bytes"),
		diskUtilization:  m.Gauge("disk_utilization_percent"),
		diskReadAwait:    m.Gauge("disk_read_await_microseconds"),
		diskWriteAwait:   m.Gauge("disk_write_await_microseconds"),
		diskServiceTime:  m.Gauge("disk_service_time_microseconds"),
		diskReadSize:     m.Gauge("disk_read_average_bytes"),
		diskWriteSize:    m.Gauge("disk_write_average_bytes"),
		pingRTTMin:       m.Gauge("ping_rtt_min"),
		pingRTTMax:       m.Gauge("ping_rtt_max"),
		pingRTTAvg:       m.Gauge("ping_rtt_avg"),
		pingRTTDev:       m.Gauge("ping_rtt_std_dev"),
		pingLoss:         m.Gauge("ping_packet_loss"),
	}
	for i, mode := range cpuModes {
		metrics.cpuTotal[i] = m.Gauge("cpu_total_"+mode+"_percent", metrix.WithFloat(true))
		metrics.cpuCore[i] = m.Gauge("cpu_core_"+mode+"_percent", metrix.WithFloat(true))
	}
	for i, counter := range memoryCounters {
		metrics.memoryCounters[i] = newCounterInstrument(m, counter.metric, counter.total)
	}
	for i, counter := range interfaceCounters {
		metrics.netInterface[i] = newCounterInstrument(m, counter.metric, counter.total)
	}
	for i, counter := range tcpCounters {
		metrics.tcp[i] = newProtocolInstruments(m, counter)
	}
	for i, counter := range udpCounters {
		metrics.udp[i] = newProtocolInstruments(m, counter)
	}
	for i, counter := range diskCounters {
		metrics.disk[i] = newCounterInstrument(m, counter.metric, counter.total)
	}
	// Chart variables are sent unscaled, so the speed variable has its own gauge.
	metrics.netSpeedKbps = m.Gauge("network_speed_kilobits_per_second", metrix.WithFloat(true))
	return metrics
}

// counterInstrument records one raw performance counter: a count since the
// counter started, or a current value.
type counterInstrument struct {
	total metrix.SnapshotCounter
	gauge metrix.SnapshotGauge
}

func newCounterInstrument(m metrix.SnapshotMeter, metric string, total bool) counterInstrument {
	if total {
		return counterInstrument{
			total: m.Counter(metric),
		}
	}
	return counterInstrument{
		gauge: m.Gauge(metric),
	}
}

func (i counterInstrument) observe(s sample, labels ...metrix.LabelSet) {
	if i.total != nil {
		observeTotal(i.total, s, labels...)
	} else {
		observe(i.gauge, s, labels...)
	}
}

// protocolInstruments records a TCP or UDP counter as its protocolCounter
// selects: per IP version, for both versions summed, or both.
type protocolInstruments struct {
	counter  protocolCounter
	versions [ipVersions]counterInstrument
	summed   counterInstrument
}

func newProtocolInstruments(m metrix.SnapshotMeter, counter protocolCounter) protocolInstruments {
	p := protocolInstruments{
		counter: counter,
	}
	if counter.perVersion {
		for version, prefix := range ipVersionPrefixes {
			p.versions[version] = newCounterInstrument(m, prefix+counter.metric, counter.total)
		}
	}
	if counter.summed {
		p.summed = newCounterInstrument(m, counter.metric, counter.total)
	}
	return p
}

func (p protocolInstruments) observe(values [ipVersions]sample) {
	if p.counter.perVersion {
		for version, s := range values {
			p.versions[version].observe(s)
		}
	}
	if p.counter.summed {
		p.summed.observe(bothVersions(values))
	}
}

// write records the valid samples. Label sets are built per poll, so a
// processor, paging file, NUMA node, interface, drive or disk that disappears
// stops being observed. Raw totals are counters: Netdata computes their rate over each
// collection interval.
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
	observe(m.swapFree, snapshot.swapFree)
	observe(m.swapUsed, snapshot.swapUsed)
	for i, s := range snapshot.memoryCounters {
		m.memoryCounters[i].observe(s)
	}
	observeTotal(m.pageFaultsMinor, snapshot.pageFaults.minor)
	observeTotal(m.pageFaultsMajor, snapshot.pageFaults.major)
	for _, file := range snapshot.pagingFiles {
		labels := m.meter.LabelSet(metrix.Label{
			Key:   pagefileLabel,
			Value: file.path,
		})
		observe(m.pagefileFree, file.free, labels)
		observe(m.pagefileUsed, file.used, labels)
	}
	for _, node := range snapshot.numaNodes {
		labels := m.meter.LabelSet(metrix.Label{
			Key:   numaNodeLabel,
			Value: node.name,
		})
		observe(m.numaFree, node.free, labels)
		observe(m.numaStandby, node.standby, labels)
	}
	observe(m.processes, snapshot.system.processes)
	observe(m.threads, snapshot.system.threads)
	observeTotal(m.contextSwitches, snapshot.system.contextSwitches)
	observe(m.processorQueue, snapshot.system.processorQueue)
	observe(m.uptime, snapshot.system.uptime)
	observeTotal(m.netTotalReceived, snapshot.netTotalReceived)
	observeTotal(m.netTotalSent, snapshot.netTotalSent)
	for _, nic := range snapshot.interfaces {
		interfaceType := realInterface
		if nic.virtual {
			interfaceType = virtualInterface
		}
		labels := m.meter.LabelSet(
			metrix.Label{
				Key:   deviceLabel,
				Value: nic.device,
			},
			metrix.Label{
				Key:   interfaceTypeLabel,
				Value: interfaceType,
			},
		)
		for i, s := range nic.values {
			m.netInterface[i].observe(s, labels)
		}
		if speed := nic.values[currentBandwidth]; speed.valid {
			m.netSpeedKbps.Observe(float64(speed.value)/1000, labels)
		}
	}
	for i := range tcpCounters {
		m.tcp[i].observe([ipVersions]sample{
			ipv4: snapshot.tcp[ipv4][i],
			ipv6: snapshot.tcp[ipv6][i],
		})
	}
	for i := range udpCounters {
		m.udp[i].observe([ipVersions]sample{
			ipv4: snapshot.udp[ipv4][i],
			ipv6: snapshot.udp[ipv6][i],
		})
	}
	for _, drive := range snapshot.drives {
		labels := m.meter.LabelSet(drive.labels()...)
		observe(m.diskAvail, drive.avail, labels)
		observe(m.diskUsed, drive.used, labels)
	}
	observeTotal(m.diskTotalRead, snapshot.diskTotalRead)
	observeTotal(m.diskTotalWritten, snapshot.diskTotalWritten)
	for _, disk := range snapshot.physicalDisks {
		labels := m.meter.LabelSet(disk.labels()...)
		for i, s := range disk.values {
			m.disk[i].observe(s, labels)
		}
		observe(m.diskUtilization, disk.interval.utilization, labels)
		observe(m.diskReadAwait, disk.interval.readAwait, labels)
		observe(m.diskWriteAwait, disk.interval.writeAwait, labels)
		observe(m.diskServiceTime, disk.interval.serviceTime, labels)
		observe(m.diskReadSize, disk.interval.readSize, labels)
		observe(m.diskWriteSize, disk.interval.writeSize, labels)
	}
}

// writePing records a probe in the go.d ping collector's units, which the
// chart template scales: round-trip times in microseconds and loss in
// thousandths of a percent. Round-trip times exist only when a reply arrived.
func (m collectorMetrics) writePing(host string, s pinger.Sample) {
	labels := m.meter.LabelSet(metrix.Label{
		Key:   hostLabel,
		Value: host,
	})
	m.pingLoss.Observe(s.PacketLossPct*1000, labels)
	if !s.RTT.Valid {
		return
	}
	m.pingRTTMin.Observe(float64(s.RTT.Min.Microseconds()), labels)
	m.pingRTTMax.Observe(float64(s.RTT.Max.Microseconds()), labels)
	m.pingRTTAvg.Observe(float64(s.RTT.Avg.Microseconds()), labels)
	m.pingRTTDev.Observe(float64(s.RTT.StdDev.Microseconds()), labels)
}

func (d diskSpace) labels() []metrix.Label {
	return []metrix.Label{
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
		{
			Key:   readWriteModeLabel,
			Value: readWriteMode,
		},
		{
			Key:   serialNumberLabel,
			Value: d.serialNumber,
		},
	}
}

// labels are the native Windows Agent's physical disk labels, each left out
// when its value is empty, as the native agent does: the mount point for a
// disk without a lettered volume, the model and device ID for a disk Windows
// does not list among its disk drives.
func (d physicalDisk) labels() []metrix.Label {
	labels := []metrix.Label{
		{
			Key:   deviceLabel,
			Value: d.device,
		},
	}
	if d.mountPoint != "" {
		labels = append(labels, metrix.Label{
			Key:   mountPointLabel,
			Value: d.mountPoint,
		})
	}
	if d.drive.model != "" {
		labels = append(labels, metrix.Label{
			Key:   modelLabel,
			Value: d.drive.model,
		})
	}
	if d.drive.deviceID != "" {
		labels = append(labels, metrix.Label{
			Key:   deviceIDLabel,
			Value: d.drive.deviceID,
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
