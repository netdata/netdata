// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
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
		return err
	}

	hostML := simpleHostMetricList(perfCounters)
	for _, h := range res.Hosts {
		h.MetricList = hostML
	}
	vmML := simpleVMMetricList(perfCounters)
	for _, v := range res.VMs {
		v.MetricList = vmML
	}

	d.Infof("discovering : metric lists : collected metric lists for %d/%d hosts, %d/%d vms, process took %s",
		len(res.Hosts),
		len(res.Hosts),
		len(res.VMs),
		len(res.VMs),
		time.Since(t),
	)

	return nil
}

func simpleHostMetricList(pci map[string]*types.PerfCounterInfo) performance.MetricList {
	return simpleMetricList(hostMetrics, pci)
}

func simpleVMMetricList(pci map[string]*types.PerfCounterInfo) performance.MetricList {
	return simpleMetricList(vmMetrics, pci)
}

func simpleMetricList(metrics []string, pci map[string]*types.PerfCounterInfo) performance.MetricList {
	sort.Strings(metrics)

	var pml performance.MetricList
	for _, v := range metrics {
		m, ok := pci[v]
		if !ok {
			// TODO: should be logged
			continue
		}
		// TODO: only summary metrics for now
		// TODO: some metrics only appear if Instance is *, for example
		// virtualDisk.totalWriteLatency.average.scsi0:0
		// virtualDisk.numberWriteAveraged.average.scsi0:0
		// virtualDisk.write.average.scsi0:0
		// virtualDisk.totalReadLatency.average.scsi0:0
		// virtualDisk.numberReadAveraged.average.scsi0:0
		// virtualDisk.read.average.scsi0:0
		// disk.numberReadAveraged.average
		// disk.numberWriteAveraged.average
		// TODO: metrics will be unsorted after if at least one Instance is *
		pml = append(pml, types.PerfMetricId{CounterId: m.Key, Instance: ""})
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
)
