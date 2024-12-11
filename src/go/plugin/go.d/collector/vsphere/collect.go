// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"errors"
	"fmt"
	"time"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"

	"github.com/vmware/govmomi/performance"
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
		mx[key] = metrix.Bool(host.OverallStatus == v)
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
		mx[key] = metrix.Bool(vm.OverallStatus == v)
	}
}
