// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"fmt"
	"sort"
	"time"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"

	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/vim25/types"
)

func (d Discoverer) collectMetricLists(res *rs.Resources) error {
	d.Debug("discovering : metric lists : starting resources metric lists collection process")
	t := time.Now()
	perfCounters, err := d.CounterInfoByName()
	if err != nil {
		return fmt.Errorf("load vSphere performance counter registry for metric-list mapping: %w", err)
	}
	d.warnMissingMetricCounters(perfCounters)

	hostML := simpleHostMetricList(perfCounters, d.CollectHostNICPerformance, d.CollectHostDiskPerformance, d.CollectHostStorageAdapterPerformance, d.CollectHostStoragePathPerformance, d.CollectHostCPUInstancePerformance, d.CollectPowerMetrics)
	for _, h := range res.Hosts {
		h.MetricList = hostML
	}
	vmML := simpleVMMetricList(perfCounters, d.CollectVMDiskPerformance, d.CollectVMNICPerformance, d.CollectPowerMetrics)
	for _, v := range res.VMs {
		v.MetricList = vmML
	}
	dsML := simpleDatastoreMetricList(perfCounters)
	for _, ds := range res.Datastores {
		ds.MetricList = dsML
	}
	clusterML := simpleClusterMetricList(perfCounters)
	for _, c := range res.Clusters {
		c.MetricList = clusterML
	}

	d.Infof("discovering : metric lists : collected metric lists for %d clusters, %d hosts, %d vms, %d datastores, process took %s",
		len(res.Clusters),
		len(res.Hosts),
		len(res.VMs),
		len(res.Datastores),
		time.Since(t),
	)

	return nil
}

func (d Discoverer) warnMissingMetricCounters(pci map[string]*types.PerfCounterInfo) {
	names := expectedMetricCounterNames(d)
	for _, name := range names {
		if _, ok := pci[name]; ok {
			continue
		}
		if d.missingPerfCounterWarnings == nil {
			d.missingPerfCounterWarnings = make(map[string]bool)
		}
		if d.missingPerfCounterWarnings[name] {
			continue
		}
		d.missingPerfCounterWarnings[name] = true
		d.Warningf("discovering : metric lists : performance counter %q not found in vCenter registry; metrics using it will be skipped", name)
	}
}

func expectedMetricCounterNames(d Discoverer) []string {
	var names []string
	names = append(names, hostMetrics...)
	if d.CollectPowerMetrics {
		names = append(names, hostPowerMetrics...)
	}
	if d.CollectHostCPUInstancePerformance {
		names = append(names, hostCPUInstancePerformanceMetrics...)
	}
	if d.CollectHostNICPerformance {
		names = append(names, hostNICPerformanceMetrics...)
	}
	if d.CollectHostDiskPerformance {
		names = append(names, hostDiskPerformanceMetrics...)
	}
	if d.CollectHostStorageAdapterPerformance {
		names = append(names, hostStorageAdapterPerformanceMetrics...)
		names = append(names, hostStorageAdapterAggregateMetrics...)
	}
	if d.CollectHostStoragePathPerformance {
		names = append(names, hostStoragePathPerformanceMetrics...)
		names = append(names, hostStoragePathAggregateMetrics...)
	}
	names = append(names, vmMetrics...)
	if d.CollectPowerMetrics {
		names = append(names, vmPowerMetrics...)
	}
	if d.CollectVMDiskPerformance {
		names = append(names, vmDiskPerformanceMetrics...)
	}
	if d.CollectVMNICPerformance {
		names = append(names, vmNICPerformanceMetrics...)
	}
	names = append(names, datastoreMetrics...)
	names = append(names, clusterMetrics...)
	return uniqueSortedStrings(names)
}

func uniqueSortedStrings(in []string) []string {
	sort.Strings(in)
	out := in[:0]
	for _, name := range in {
		if len(out) == 0 || out[len(out)-1] != name {
			out = append(out, name)
		}
	}
	return out
}

func simpleHostMetricList(pci map[string]*types.PerfCounterInfo, collectNICPerformance, collectDiskPerformance, collectStorageAdapterPerformance, collectStoragePathPerformance, collectCPUInstancePerformance, collectPowerMetrics bool) performance.MetricList {
	ml := simpleMetricList(hostMetrics, pci, "")
	if collectPowerMetrics {
		ml = append(ml, simpleMetricList(hostPowerMetrics, pci, "")...)
	}
	if collectCPUInstancePerformance {
		ml = append(ml, simpleMetricList(hostCPUInstancePerformanceMetrics, pci, "*")...)
	}
	if collectNICPerformance {
		ml = append(ml, simpleMetricList(hostNICPerformanceMetrics, pci, "*")...)
	}
	if collectDiskPerformance {
		ml = append(ml, simpleMetricList(hostDiskPerformanceMetrics, pci, "*")...)
	}
	if collectStorageAdapterPerformance {
		ml = append(ml, simpleMetricList(hostStorageAdapterPerformanceMetrics, pci, "*")...)
		ml = append(ml, simpleMetricList(hostStorageAdapterAggregateMetrics, pci, "")...)
	}
	if collectStoragePathPerformance {
		ml = append(ml, simpleMetricList(hostStoragePathPerformanceMetrics, pci, "*")...)
		ml = append(ml, simpleMetricList(hostStoragePathAggregateMetrics, pci, "")...)
	}
	return ml
}

func simpleVMMetricList(pci map[string]*types.PerfCounterInfo, collectDiskPerformance, collectNICPerformance, collectPowerMetrics bool) performance.MetricList {
	ml := simpleMetricList(vmMetrics, pci, "")
	if collectPowerMetrics {
		ml = append(ml, simpleMetricList(vmPowerMetrics, pci, "")...)
	}
	if collectDiskPerformance {
		ml = append(ml, simpleMetricList(vmDiskPerformanceMetrics, pci, "*")...)
	}
	if collectNICPerformance {
		ml = append(ml, simpleMetricList(vmNICPerformanceMetrics, pci, "*")...)
	}
	return ml
}

func simpleDatastoreMetricList(pci map[string]*types.PerfCounterInfo) performance.MetricList {
	return simpleMetricList(datastoreMetrics, pci, "")
}

func simpleClusterMetricList(pci map[string]*types.PerfCounterInfo) performance.MetricList {
	return simpleMetricList(clusterMetrics, pci, "")
}

func simpleMetricList(metrics []string, pci map[string]*types.PerfCounterInfo, instance string) performance.MetricList {
	metrics = append([]string(nil), metrics...)
	sort.Strings(metrics)

	var pml performance.MetricList
	for _, v := range metrics {
		m, ok := pci[v]
		if !ok {
			continue
		}
		pml = append(pml, types.PerfMetricId{CounterId: m.Key, Instance: instance})
	}
	return pml
}

var (
	vmMetrics = []string{
		"cpu.usage.average",

		"mem.usage.average",
		"mem.granted.average",
		"mem.consumed.average",
		"mem.active.average",
		"mem.shared.average",
		// Refers to VMkernel swapping!
		"mem.swapinRate.average",
		"mem.swapoutRate.average",
		"mem.swapped.average",

		"net.bytesRx.average",
		"net.bytesTx.average",
		"net.packetsRx.summation",
		"net.packetsTx.summation",
		"net.droppedRx.summation",
		"net.droppedTx.summation",

		// the only summary disk metrics
		"disk.read.average",
		"disk.write.average",
		"disk.maxTotalLatency.latest",

		"sys.uptime.latest",
	}

	vmDiskPerformanceMetrics = []string{
		"virtualDisk.numberReadAveraged.average",
		"virtualDisk.numberWriteAveraged.average",
		"virtualDisk.read.average",
		"virtualDisk.readOIO.latest",
		"virtualDisk.totalReadLatency.average",
		"virtualDisk.totalWriteLatency.average",
		"virtualDisk.write.average",
		"virtualDisk.writeOIO.latest",
	}

	vmNICPerformanceMetrics = []string{
		"net.broadcastRx.summation",
		"net.broadcastTx.summation",
		"net.bytesRx.average",
		"net.bytesTx.average",
		"net.droppedRx.summation",
		"net.droppedTx.summation",
		"net.multicastRx.summation",
		"net.multicastTx.summation",
		"net.packetsRx.summation",
		"net.packetsTx.summation",
	}

	datastoreMetrics = []string{
		// Level 1 - available at default vCenter settings
		"datastore.numberReadAveraged.average",
		"datastore.numberWriteAveraged.average",
		"datastore.totalReadLatency.average",
		"datastore.totalWriteLatency.average",

		// Level 2 - requires elevated stats level
		"datastore.read.average",
		"datastore.write.average",
	}

	hostMetrics = []string{
		"cpu.usage.average",

		"mem.usage.average",
		"mem.granted.average",
		"mem.consumed.average",
		"mem.active.average",
		"mem.shared.average",
		"mem.sharedcommon.average",
		// Refers to VMkernel swapping!
		"mem.swapinRate.average",
		"mem.swapoutRate.average",

		"net.bytesRx.average",
		"net.bytesTx.average",
		"net.packetsRx.summation",
		"net.packetsTx.summation",
		"net.droppedRx.summation",
		"net.droppedTx.summation",
		"net.errorsRx.summation",
		"net.errorsTx.summation",

		// the only summary disk metrics
		"disk.read.average",
		"disk.write.average",
		"disk.maxTotalLatency.latest",

		"sys.uptime.latest",
	}

	hostNICPerformanceMetrics = []string{
		"net.broadcastRx.summation",
		"net.broadcastTx.summation",
		"net.bytesRx.average",
		"net.bytesTx.average",
		"net.droppedRx.summation",
		"net.droppedTx.summation",
		"net.errorsRx.summation",
		"net.errorsTx.summation",
		"net.multicastRx.summation",
		"net.multicastTx.summation",
		"net.packetsRx.summation",
		"net.packetsTx.summation",
		"net.unknownProtos.summation",
		"net.usage.average",
	}

	hostDiskPerformanceMetrics = []string{
		"disk.busResets.summation",
		"disk.commands.summation",
		"disk.commandsAborted.summation",
		"disk.commandsAveraged.average",
		"disk.deviceLatency.average",
		"disk.deviceReadLatency.average",
		"disk.deviceWriteLatency.average",
		"disk.kernelLatency.average",
		"disk.kernelReadLatency.average",
		"disk.kernelWriteLatency.average",
		"disk.maxQueueDepth.average",
		"disk.numberRead.summation",
		"disk.numberReadAveraged.average",
		"disk.numberWrite.summation",
		"disk.numberWriteAveraged.average",
		"disk.queueLatency.average",
		"disk.queueReadLatency.average",
		"disk.queueWriteLatency.average",
		"disk.read.average",
		"disk.scsiReservationCnflctsPct.average",
		"disk.scsiReservationConflicts.summation",
		"disk.totalLatency.average",
		"disk.totalReadLatency.average",
		"disk.totalWriteLatency.average",
		"disk.write.average",
	}

	hostStorageAdapterPerformanceMetrics = []string{
		"storageAdapter.OIOsPct.average",
		"storageAdapter.commandsAveraged.average",
		"storageAdapter.numberReadAveraged.average",
		"storageAdapter.numberWriteAveraged.average",
		"storageAdapter.outstandingIOs.average",
		"storageAdapter.queueDepth.average",
		"storageAdapter.queueLatency.average",
		"storageAdapter.queued.average",
		"storageAdapter.read.average",
		"storageAdapter.throughput.cont.average",
		"storageAdapter.throughput.usag.average",
		"storageAdapter.totalReadLatency.average",
		"storageAdapter.totalWriteLatency.average",
		"storageAdapter.write.average",
	}

	hostStorageAdapterAggregateMetrics = []string{
		"storageAdapter.maxTotalLatency.latest",
	}

	hostStoragePathPerformanceMetrics = []string{
		"storagePath.busResets.summation",
		"storagePath.commandsAborted.summation",
		"storagePath.commandsAveraged.average",
		"storagePath.numberReadAveraged.average",
		"storagePath.numberWriteAveraged.average",
		"storagePath.read.average",
		"storagePath.throughput.cont.average",
		"storagePath.throughput.usage.average",
		"storagePath.totalReadLatency.average",
		"storagePath.totalWriteLatency.average",
		"storagePath.write.average",
	}

	hostStoragePathAggregateMetrics = []string{
		"storagePath.maxTotalLatency.latest",
	}

	hostCPUInstancePerformanceMetrics = []string{
		"cpu.coreUtilization.average",
		"cpu.idle.summation",
		"cpu.usage.average",
		"cpu.used.summation",
		"cpu.utilization.average",
	}

	hostPowerMetrics = []string{
		"power.capacity.usable.average",
		"power.capacity.usage.average",
		"power.capacity.usageIdle.average",
		"power.capacity.usagePct.average",
		"power.capacity.usageSystem.average",
		"power.capacity.usageVm.average",
		"power.energy.summation",
		"power.power.average",
		"power.powerCap.average",
	}

	vmPowerMetrics = []string{
		"power.energy.summation",
		"power.power.average",
	}

	// All Level 1 (available at default vCenter settings), IntervalId=300 (historical)
	clusterMetrics = []string{
		// clusterServices counters
		"clusterServices.effectivecpu.average",
		"clusterServices.effectivemem.average",
		"clusterServices.cpufairness.latest",
		"clusterServices.memfairness.latest",
		"clusterServices.failover.latest",

		// cpu/mem aggregate counters
		"cpu.usage.average",
		"cpu.usagemhz.average",
		"cpu.totalmhz.average",
		"mem.usage.average",
		"mem.consumed.average",
		"mem.overhead.average",
		"mem.active.average",
		"mem.granted.average",
		"mem.shared.average",
		"mem.swapused.average",

		// vmop counters — VM operations per cluster
		"vmop.numVMotion.latest",
		"vmop.numSVMotion.latest",
		"vmop.numXVMotion.latest",
		"vmop.numPoweron.latest",
		"vmop.numPoweroff.latest",
		"vmop.numCreate.latest",
		"vmop.numDestroy.latest",
		"vmop.numClone.latest",
		"vmop.numDeploy.latest",
		"vmop.numReset.latest",
		"vmop.numSuspend.latest",
		"vmop.numReconfigure.latest",
		"vmop.numRegister.latest",
		"vmop.numUnregister.latest",
		"vmop.numChangeDS.latest",
		"vmop.numChangeHost.latest",
		"vmop.numChangeHostDS.latest",
		"vmop.numRebootGuest.latest",
		"vmop.numShutdownGuest.latest",
		"vmop.numStandbyGuest.latest",

		// vSphere 7.0+ only — automatically skipped if not available
		"clusterServices.clusterDrsScore.latest",
		"clusterServices.vmDrsScore.latest",
	}
)
