// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"errors"
	"fmt"
	"time"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/modules/vsphere/resources"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"

	"github.com/vmware/govmomi/performance"
)

// ManagedEntityStatus
var overallStatuses = []string{"green", "red", "yellow", "gray"}

func (vs *VSphere) collect() (map[string]int64, error) {
	vs.collectionLock.Lock()
	defer vs.collectionLock.Unlock()

	vs.Debug("starting collection process")
	t := time.Now()
	mx := make(map[string]int64)

	err := vs.collectHosts(mx)
	if err != nil {
		return nil, err
	}

	err = vs.collectVMs(mx)
	if err != nil {
		return nil, err
	}

	vs.updateCharts()

	vs.Debugf("metrics collected, process took %s", time.Since(t))

	return mx, nil
}

func (vs *VSphere) collectHosts(mx map[string]int64) error {
	if len(vs.resources.Hosts) == 0 {
		return nil
	}
	// NOTE: returns unsorted if at least one types.PerfMetricId Instance is not ""
	metrics := vs.ScrapeHosts(vs.resources.Hosts)
	if len(metrics) == 0 {
		return errors.New("failed to scrape hosts metrics")
	}

	vs.collectHostsMetrics(mx, metrics)

	return nil
}

func (vs *VSphere) collectHostsMetrics(mx map[string]int64, metrics []performance.EntityMetric) {
	for k := range vs.discoveredHosts {
		vs.discoveredHosts[k]++
	}

	for _, metric := range metrics {
		if host := vs.resources.Hosts.Get(metric.Entity.Value); host != nil {
			vs.discoveredHosts[host.ID] = 0
			writeHostMetrics(mx, host, metric.Value)
		}
	}
}

func writeHostMetrics(mx map[string]int64, host *rs.Host, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		key := fmt.Sprintf("%s_%s", host.ID, metric.Name)
		mx[key] = metric.Value[0]
	}
	for _, v := range overallStatuses {
		key := fmt.Sprintf("%s_overall.status.%s", host.ID, v)
		mx[key] = metrix.Bool(host.OverallStatus == v)
	}
}

func (vs *VSphere) collectVMs(mx map[string]int64) error {
	if len(vs.resources.VMs) == 0 {
		return nil
	}
	// NOTE: returns unsorted if at least one types.PerfMetricId Instance is not ""
	ems := vs.ScrapeVMs(vs.resources.VMs)
	if len(ems) == 0 {
		return errors.New("failed to scrape vms metrics")
	}

	vs.collectVMsMetrics(mx, ems)

	return nil
}

func (vs *VSphere) collectVMsMetrics(mx map[string]int64, metrics []performance.EntityMetric) {
	for id := range vs.discoveredVMs {
		vs.discoveredVMs[id]++
	}

	for _, metric := range metrics {
		if vm := vs.resources.VMs.Get(metric.Entity.Value); vm != nil {
			writeVMMetrics(mx, vm, metric.Value)
			vs.discoveredVMs[vm.ID] = 0
		}
	}
}

func writeVMMetrics(mx map[string]int64, vm *rs.VM, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		key := fmt.Sprintf("%s_%s", vm.ID, metric.Name)
		mx[key] = metric.Value[0]
	}
	for _, v := range overallStatuses {
		key := fmt.Sprintf("%s_overall.status.%s", vm.ID, v)
		mx[key] = metrix.Bool(vm.OverallStatus == v)
	}
}
