// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"errors"
	"fmt"
	"time"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"

	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/vim25/types"
)

// ManagedEntityStatus
var overallStatuses = []string{"green", "red", "yellow", "gray"}

func (c *Collector) collect() (map[string]int64, error) {
	c.collectionLock.Lock()
	defer c.collectionLock.Unlock()

	c.Debug("starting collection process")
	t := time.Now()
	mx := make(map[string]int64)

	err := c.collectHosts(mx)
	if err != nil {
		return nil, err
	}

	err = c.collectVMs(mx)
	if err != nil {
		return nil, err
	}

	c.collectDatastores(mx)

	c.updateCharts()

	c.Debugf("metrics collected, process took %s", time.Since(t))

	return mx, nil
}

func (c *Collector) collectHosts(mx map[string]int64) error {
	if len(c.resources.Hosts) == 0 {
		return nil
	}
	// NOTE: returns unsorted if at least one types.PerfMetricId Instance is not ""
	metrics := c.ScrapeHosts(c.resources.Hosts)
	if len(metrics) == 0 {
		return errors.New("failed to scrape hosts metrics")
	}

	c.collectHostsMetrics(mx, metrics)

	return nil
}

func (c *Collector) collectHostsMetrics(mx map[string]int64, metrics []performance.EntityMetric) {
	for k := range c.discoveredHosts {
		c.discoveredHosts[k]++
	}

	for _, metric := range metrics {
		if host := c.resources.Hosts.Get(metric.Entity.Value); host != nil {
			c.discoveredHosts[host.ID] = 0
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
		mx[key] = oldmetrix.Bool(host.OverallStatus == v)
	}
}

func (c *Collector) collectVMs(mx map[string]int64) error {
	if len(c.resources.VMs) == 0 {
		return nil
	}
	// NOTE: returns unsorted if at least one types.PerfMetricId Instance is not ""
	ems := c.ScrapeVMs(c.resources.VMs)
	if len(ems) == 0 {
		return errors.New("failed to scrape vms metrics")
	}

	c.collectVMsMetrics(mx, ems)

	return nil
}

func (c *Collector) collectVMsMetrics(mx map[string]int64, metrics []performance.EntityMetric) {
	for id := range c.discoveredVMs {
		c.discoveredVMs[id]++
	}

	for _, metric := range metrics {
		if vm := c.resources.VMs.Get(metric.Entity.Value); vm != nil {
			writeVMMetrics(mx, vm, metric.Value)
			c.discoveredVMs[vm.ID] = 0
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
		mx[key] = oldmetrix.Bool(vm.OverallStatus == v)
	}
}

func (c *Collector) collectDatastores(mx map[string]int64) {
	if len(c.resources.Datastores) == 0 {
		return
	}

	refreshed := c.refreshDatastoreProperties()

	metrics := c.ScrapeDatastores(c.resources.Datastores)
	// Datastore perf counters may return empty for vSAN or when no historical data is available yet.
	// This is not an error — we still collect capacity and status from properties.
	c.collectDatastoresMetrics(mx, metrics, refreshed)
}

func (c *Collector) refreshDatastoreProperties() map[string]bool {
	refreshed := make(map[string]bool)

	if c.dsPropertyCollector == nil {
		return refreshed
	}

	refs := make([]types.ManagedObjectReference, 0, len(c.resources.Datastores))
	for _, ds := range c.resources.Datastores {
		refs = append(refs, ds.Ref)
	}

	dsList, err := c.dsPropertyCollector.DatastoresByRef(refs, "summary", "overallStatus")
	if err != nil {
		c.Warningf("failed to refresh datastore properties: %v", err)
		return refreshed
	}

	for _, raw := range dsList {
		ds := c.resources.Datastores.Get(raw.Reference().Value)
		if ds == nil {
			continue
		}
		refreshed[ds.ID] = true
		ds.Capacity = raw.Summary.Capacity
		ds.FreeSpace = raw.Summary.FreeSpace
		ds.Accessible = raw.Summary.Accessible
		ds.OverallStatus = string(raw.OverallStatus)
	}

	return refreshed
}

func (c *Collector) collectDatastoresMetrics(mx map[string]int64, metrics []performance.EntityMetric, refreshed map[string]bool) {
	for id := range c.discoveredDatastores {
		c.discoveredDatastores[id]++
	}

	for _, ds := range c.resources.Datastores {
		if refreshed[ds.ID] {
			c.discoveredDatastores[ds.ID] = 0
		}
		writeDatastoreMetrics(mx, ds)
	}

	for _, metric := range metrics {
		if ds := c.resources.Datastores.Get(metric.Entity.Value); ds != nil {
			c.discoveredDatastores[ds.ID] = 0
			c.datastorePerfReceived[ds.ID] = true
			writeDatastorePerfMetrics(mx, ds, metric.Value)
		}
	}
}

func writeDatastoreMetrics(mx map[string]int64, ds *rs.Datastore) {
	// VMware docs: Capacity and FreeSpace are guaranteed valid only when Accessible is true.
	var capacity, freeSpace, used int64
	if ds.Accessible {
		capacity = ds.Capacity
		freeSpace = ds.FreeSpace
		used = capacity - freeSpace
		if used < 0 {
			used = 0
		}
	}

	mx[fmt.Sprintf("%s_capacity", ds.ID)] = capacity
	mx[fmt.Sprintf("%s_free_space", ds.ID)] = freeSpace
	mx[fmt.Sprintf("%s_used_space", ds.ID)] = used

	if capacity > 0 {
		// use float64 to avoid int64 overflow on datastores larger than 922 TB
		mx[fmt.Sprintf("%s_used_space_pct", ds.ID)] = int64(float64(used) / float64(capacity) * 10000)
	} else {
		mx[fmt.Sprintf("%s_used_space_pct", ds.ID)] = 0
	}

	for _, v := range overallStatuses {
		key := fmt.Sprintf("%s_overall.status.%s", ds.ID, v)
		mx[key] = oldmetrix.Bool(ds.OverallStatus == v)
	}
}

func writeDatastorePerfMetrics(mx map[string]int64, ds *rs.Datastore, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		key := fmt.Sprintf("%s_%s", ds.ID, metric.Name)
		mx[key] = metric.Value[0]
	}
}
