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

func (d *Discoverer) collectMetricLists(res *rs.Resources) error {
	d.Debug("discovering : metric lists : starting resources metric lists collection process")
	t := time.Now()
	perfCounters, err := d.CounterInfoByName()
	if err != nil {
		return fmt.Errorf("load vSphere performance counter registry for metric-list mapping: %w", err)
	}
	d.warnMissingMetricCounters(perfCounters)

	hostML := simpleHostMetricList(perfCounters)
	for _, h := range res.Hosts {
		h.MetricList = hostML
	}
	vmML := simpleVMMetricList(perfCounters)
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

func (d *Discoverer) warnMissingMetricCounters(pci map[string]*types.PerfCounterInfo) {
	names := expectedMetricCounterNames()
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

func expectedMetricCounterNames() []string {
	var names []string
	names = append(names, hostMetrics...)
	names = append(names, vmMetrics...)
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

func simpleHostMetricList(pci map[string]*types.PerfCounterInfo) performance.MetricList {
	ml := simpleMetricList(hostMetrics, pci, "")
	ml = append(ml, simpleMetricList(hostPowerMetrics, pci, "")...)
	return ml
}

func simpleVMMetricList(pci map[string]*types.PerfCounterInfo) performance.MetricList {
	ml := simpleMetricList(vmMetrics, pci, "")
	ml = append(ml, simpleMetricList(vmPowerMetrics, pci, "")...)
	return ml
}

func simpleDatastoreMetricList(pci map[string]*types.PerfCounterInfo) performance.MetricList {
	return simpleMetricList(datastoreMetrics, pci, "")
}

func simpleClusterMetricList(pci map[string]*types.PerfCounterInfo) performance.MetricList {
	ml := simpleMetricList(clusterMetrics, pci, "")
	ml = append(ml, simpleMetricList(clusterOptionalMetrics, pci, "")...)
	return ml
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
	}

	clusterOptionalMetrics = []string{
		// vSphere 7.0+ only — automatically skipped if not available
		"clusterServices.clusterDrsScore.latest",
		"clusterServices.vmDrsScore.latest",
	}
)
