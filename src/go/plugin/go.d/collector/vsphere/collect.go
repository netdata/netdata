// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"errors"
	"fmt"
	"time"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/oldmetrix"

	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/vim25/mo"
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
	c.collectClusters(mx)
	c.collectResourcePools(mx)

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
		used = max(capacity-freeSpace, 0)
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

func (c *Collector) collectClusters(mx map[string]int64) {
	if len(c.resources.Clusters) == 0 {
		return
	}

	refreshed := c.refreshClusterProperties()

	metrics := c.ScrapeClusters(c.resources.Clusters)
	c.collectClustersMetrics(mx, metrics, refreshed)
}

func (c *Collector) refreshClusterProperties() map[string]bool {
	refreshed := make(map[string]bool)

	if c.clusterPropertyCollector == nil {
		return refreshed
	}

	refs := make([]types.ManagedObjectReference, 0, len(c.resources.Clusters))
	for _, cl := range c.resources.Clusters {
		refs = append(refs, cl.Ref)
	}

	clusters, err := c.clusterPropertyCollector.ClustersByRef(refs, "name", "summary", "configurationEx", "overallStatus")
	if err != nil {
		c.Warningf("failed to refresh cluster properties: %v", err)
		return refreshed
	}

	for _, raw := range clusters {
		cl := c.resources.Clusters.Get(raw.Reference().Value)
		if cl == nil {
			continue
		}
		refreshed[cl.ID] = true
		updateClusterFromProperties(cl, raw)
	}

	return refreshed
}

func updateClusterFromProperties(cl *rs.Cluster, raw mo.ClusterComputeResource) {
	cl.OverallStatus = string(raw.OverallStatus)

	var s *types.ComputeResourceSummary
	if raw.Summary != nil {
		s = raw.Summary.GetComputeResourceSummary()
	}
	if s != nil {
		cl.TotalCpu = s.TotalCpu
		cl.TotalMemory = s.TotalMemory
		cl.NumCpuCores = s.NumCpuCores
		cl.NumCpuThreads = s.NumCpuThreads
		cl.EffectiveCpu = s.EffectiveCpu
		cl.EffectiveMemory = s.EffectiveMemory
		cl.NumHosts = s.NumHosts
		cl.NumEffectiveHosts = s.NumEffectiveHosts
	}

	// Reset conditional fields to avoid stale values
	cl.NumVmotions = 0
	cl.CurrentBalance = 0
	cl.TargetBalance = 0
	cl.DrsScore = 0
	cl.UsageCpuDemandMhz = 0
	cl.UsageMemDemandMB = 0
	cl.UsageCpuEntitledMhz = 0
	cl.UsageMemEntitledMB = 0
	cl.UsageCpuReservationMhz = 0
	cl.UsageMemReservationMB = 0
	cl.UsageTotalVmCount = 0
	cl.UsagePoweredOffVmCount = 0
	cl.DrsEnabled = false
	cl.DrsMode = ""
	cl.HaEnabled = false
	cl.HaAdmCtrlEnabled = false

	// Cluster-specific summary fields
	if cs, ok := raw.Summary.(*types.ClusterComputeResourceSummary); ok {
		cl.NumVmotions = cs.NumVmotions
		cl.CurrentBalance = cs.CurrentBalance
		cl.TargetBalance = cs.TargetBalance
		cl.DrsScore = cs.DrsScore
		if cs.UsageSummary != nil {
			cl.UsageCpuDemandMhz = cs.UsageSummary.CpuDemandMhz
			cl.UsageMemDemandMB = cs.UsageSummary.MemDemandMB
			cl.UsageCpuEntitledMhz = cs.UsageSummary.CpuEntitledMhz
			cl.UsageMemEntitledMB = cs.UsageSummary.MemEntitledMB
			cl.UsageCpuReservationMhz = cs.UsageSummary.CpuReservationMhz
			cl.UsageMemReservationMB = cs.UsageSummary.MemReservationMB
			cl.UsageTotalVmCount = cs.UsageSummary.TotalVmCount
			cl.UsagePoweredOffVmCount = cs.UsageSummary.PoweredOffVmCount
		}
	}

	// DRS and HA config from configurationEx
	if cfg, ok := raw.ConfigurationEx.(*types.ClusterConfigInfoEx); ok {
		if cfg.DrsConfig.Enabled != nil {
			cl.DrsEnabled = *cfg.DrsConfig.Enabled
		}
		cl.DrsMode = string(cfg.DrsConfig.DefaultVmBehavior)
		if cfg.DasConfig.Enabled != nil {
			cl.HaEnabled = *cfg.DasConfig.Enabled
		}
		if cfg.DasConfig.AdmissionControlEnabled != nil {
			cl.HaAdmCtrlEnabled = *cfg.DasConfig.AdmissionControlEnabled
		}
	}
}

func (c *Collector) collectClustersMetrics(mx map[string]int64, metrics []performance.EntityMetric, refreshed map[string]bool) {
	for id := range c.discoveredClusters {
		c.discoveredClusters[id]++
	}

	for _, cl := range c.resources.Clusters {
		if refreshed[cl.ID] {
			c.discoveredClusters[cl.ID] = 0
		}
		writeClusterPropertyMetrics(mx, cl)
	}

	for _, metric := range metrics {
		if cl := c.resources.Clusters.Get(metric.Entity.Value); cl != nil {
			c.discoveredClusters[cl.ID] = 0
			c.clusterPerfReceived[cl.ID] = true
			writeClusterPerfMetrics(mx, cl, metric.Value)
		}
	}
}

func writeClusterPropertyMetrics(mx map[string]int64, cl *rs.Cluster) {
	mx[fmt.Sprintf("%s_num_hosts", cl.ID)] = int64(cl.NumHosts)
	mx[fmt.Sprintf("%s_num_effective_hosts", cl.ID)] = int64(cl.NumEffectiveHosts)
	mx[fmt.Sprintf("%s_total_cpu", cl.ID)] = int64(cl.TotalCpu)
	mx[fmt.Sprintf("%s_effective_cpu", cl.ID)] = int64(cl.EffectiveCpu)
	mx[fmt.Sprintf("%s_total_memory", cl.ID)] = cl.TotalMemory
	// EffectiveMemory is MB from API, convert to bytes for consistency with TotalMemory
	mx[fmt.Sprintf("%s_effective_memory", cl.ID)] = cl.EffectiveMemory * 1024 * 1024
	mx[fmt.Sprintf("%s_num_cpu_cores", cl.ID)] = int64(cl.NumCpuCores)
	mx[fmt.Sprintf("%s_num_cpu_threads", cl.ID)] = int64(cl.NumCpuThreads)
	mx[fmt.Sprintf("%s_num_vmotions", cl.ID)] = int64(cl.NumVmotions)
	mx[fmt.Sprintf("%s_drs_score", cl.ID)] = int64(cl.DrsScore)
	mx[fmt.Sprintf("%s_current_balance", cl.ID)] = int64(cl.CurrentBalance)
	mx[fmt.Sprintf("%s_target_balance", cl.ID)] = int64(cl.TargetBalance)

	mx[fmt.Sprintf("%s_drs_enabled", cl.ID)] = oldmetrix.Bool(cl.DrsEnabled)
	mx[fmt.Sprintf("%s_ha_enabled", cl.ID)] = oldmetrix.Bool(cl.HaEnabled)
	mx[fmt.Sprintf("%s_ha_adm_ctrl_enabled", cl.ID)] = oldmetrix.Bool(cl.HaAdmCtrlEnabled)

	mx[fmt.Sprintf("%s_usage_cpu_demand_mhz", cl.ID)] = int64(cl.UsageCpuDemandMhz)
	mx[fmt.Sprintf("%s_usage_mem_demand_mb", cl.ID)] = int64(cl.UsageMemDemandMB)
	mx[fmt.Sprintf("%s_usage_cpu_entitled_mhz", cl.ID)] = int64(cl.UsageCpuEntitledMhz)
	mx[fmt.Sprintf("%s_usage_mem_entitled_mb", cl.ID)] = int64(cl.UsageMemEntitledMB)
	mx[fmt.Sprintf("%s_usage_cpu_reservation_mhz", cl.ID)] = int64(cl.UsageCpuReservationMhz)
	mx[fmt.Sprintf("%s_usage_mem_reservation_mb", cl.ID)] = int64(cl.UsageMemReservationMB)
	mx[fmt.Sprintf("%s_usage_total_vm_count", cl.ID)] = int64(cl.UsageTotalVmCount)
	mx[fmt.Sprintf("%s_usage_powered_off_vm_count", cl.ID)] = int64(cl.UsagePoweredOffVmCount)

	for _, v := range overallStatuses {
		key := fmt.Sprintf("%s_overall.status.%s", cl.ID, v)
		mx[key] = oldmetrix.Bool(cl.OverallStatus == v)
	}
}

func writeClusterPerfMetrics(mx map[string]int64, cl *rs.Cluster, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		key := fmt.Sprintf("%s_%s", cl.ID, metric.Name)
		mx[key] = metric.Value[0]
	}
}

func (c *Collector) collectResourcePools(mx map[string]int64) {
	if len(c.resources.ResourcePools) == 0 {
		return
	}

	refreshed := c.refreshResourcePoolProperties()

	c.collectResourcePoolsMetrics(mx, refreshed)
}

func (c *Collector) refreshResourcePoolProperties() map[string]bool {
	refreshed := make(map[string]bool)

	if c.rpPropertyCollector == nil {
		return refreshed
	}

	refs := make([]types.ManagedObjectReference, 0, len(c.resources.ResourcePools))
	for _, rp := range c.resources.ResourcePools {
		refs = append(refs, rp.Ref)
	}

	pools, err := c.rpPropertyCollector.ResourcePoolsByRef(refs, "name", "summary", "config", "runtime", "overallStatus")
	if err != nil {
		c.Warningf("failed to refresh resource pool properties: %v", err)
		return refreshed
	}

	for _, raw := range pools {
		rp := c.resources.ResourcePools.Get(raw.Reference().Value)
		if rp == nil {
			continue
		}
		refreshed[rp.ID] = true
		updateResourcePoolFromProperties(rp, raw)
	}

	return refreshed
}

func updateResourcePoolFromProperties(rp *rs.ResourcePool, raw mo.ResourcePool) {
	rp.OverallStatus = string(raw.OverallStatus)

	// Reset conditional fields to avoid stale values
	rp.OverallCpuUsage = 0
	rp.OverallCpuDemand = 0
	rp.GuestMemoryUsage = 0
	rp.HostMemoryUsage = 0
	rp.DistributedCpuEntitlement = 0
	rp.DistributedMemoryEntitlement = 0
	rp.PrivateMemory = 0
	rp.SharedMemory = 0
	rp.SwappedMemory = 0
	rp.BalloonedMemory = 0
	rp.OverheadMemory = 0
	rp.ConsumedOverheadMemory = 0
	rp.CompressedMemory = 0

	var qs *types.ResourcePoolQuickStats
	if raw.Summary != nil {
		if s := raw.Summary.GetResourcePoolSummary(); s != nil {
			qs = s.QuickStats
		}
	}
	if qs != nil {
		rp.OverallCpuUsage = qs.OverallCpuUsage
		rp.OverallCpuDemand = qs.OverallCpuDemand
		rp.GuestMemoryUsage = qs.GuestMemoryUsage
		rp.HostMemoryUsage = qs.HostMemoryUsage
		rp.DistributedCpuEntitlement = qs.DistributedCpuEntitlement
		rp.DistributedMemoryEntitlement = qs.DistributedMemoryEntitlement
		rp.PrivateMemory = qs.PrivateMemory
		rp.SharedMemory = qs.SharedMemory
		rp.SwappedMemory = qs.SwappedMemory
		rp.BalloonedMemory = qs.BalloonedMemory
		rp.OverheadMemory = qs.OverheadMemory
		rp.ConsumedOverheadMemory = qs.ConsumedOverheadMemory
		rp.CompressedMemory = qs.CompressedMemory
	}

	// Runtime resource usage (full "runtime" property requested)
	rp.CpuReservationUsed = raw.Runtime.Cpu.ReservationUsed
	rp.CpuMaxUsage = raw.Runtime.Cpu.MaxUsage
	rp.CpuUnreservedForVm = raw.Runtime.Cpu.UnreservedForVm
	rp.MemReservationUsed = raw.Runtime.Memory.ReservationUsed
	rp.MemMaxUsage = raw.Runtime.Memory.MaxUsage
	rp.MemUnreservedForVm = raw.Runtime.Memory.UnreservedForVm

	// Config — reservation, limit
	rp.CpuReservation = 0
	rp.CpuLimit = -1
	rp.MemReservation = 0
	rp.MemLimit = -1
	if raw.Config.CpuAllocation.Reservation != nil {
		rp.CpuReservation = *raw.Config.CpuAllocation.Reservation
	}
	if raw.Config.CpuAllocation.Limit != nil {
		rp.CpuLimit = *raw.Config.CpuAllocation.Limit
	}
	if raw.Config.MemoryAllocation.Reservation != nil {
		rp.MemReservation = *raw.Config.MemoryAllocation.Reservation
	}
	if raw.Config.MemoryAllocation.Limit != nil {
		rp.MemLimit = *raw.Config.MemoryAllocation.Limit
	}

	// CpuSharesLevel / MemSharesLevel are strings (low/normal/high/custom) — not exported as metrics
}

func (c *Collector) collectResourcePoolsMetrics(mx map[string]int64, refreshed map[string]bool) {
	for id := range c.discoveredResourcePools {
		c.discoveredResourcePools[id]++
	}

	for _, rp := range c.resources.ResourcePools {
		if refreshed[rp.ID] {
			c.discoveredResourcePools[rp.ID] = 0
		}
		writeResourcePoolMetrics(mx, rp)
	}
}

func writeResourcePoolMetrics(mx map[string]int64, rp *rs.ResourcePool) {
	mx[fmt.Sprintf("%s_cpu_usage", rp.ID)] = rp.OverallCpuUsage
	mx[fmt.Sprintf("%s_cpu_demand", rp.ID)] = rp.OverallCpuDemand
	mx[fmt.Sprintf("%s_cpu_entitlement_distributed", rp.ID)] = rp.DistributedCpuEntitlement
	mx[fmt.Sprintf("%s_mem_usage_guest", rp.ID)] = rp.GuestMemoryUsage
	mx[fmt.Sprintf("%s_mem_usage_host", rp.ID)] = rp.HostMemoryUsage
	mx[fmt.Sprintf("%s_mem_entitlement_distributed", rp.ID)] = rp.DistributedMemoryEntitlement

	mx[fmt.Sprintf("%s_mem_private", rp.ID)] = rp.PrivateMemory
	mx[fmt.Sprintf("%s_mem_shared", rp.ID)] = rp.SharedMemory
	mx[fmt.Sprintf("%s_mem_swapped", rp.ID)] = rp.SwappedMemory
	mx[fmt.Sprintf("%s_mem_ballooned", rp.ID)] = rp.BalloonedMemory
	mx[fmt.Sprintf("%s_mem_overhead", rp.ID)] = rp.OverheadMemory
	mx[fmt.Sprintf("%s_mem_consumed_overhead", rp.ID)] = rp.ConsumedOverheadMemory
	mx[fmt.Sprintf("%s_mem_compressed", rp.ID)] = rp.CompressedMemory

	mx[fmt.Sprintf("%s_cpu_reservation_used", rp.ID)] = rp.CpuReservationUsed
	mx[fmt.Sprintf("%s_cpu_max_usage", rp.ID)] = rp.CpuMaxUsage
	mx[fmt.Sprintf("%s_cpu_unreserved_for_vm", rp.ID)] = rp.CpuUnreservedForVm
	mx[fmt.Sprintf("%s_mem_reservation_used", rp.ID)] = rp.MemReservationUsed
	mx[fmt.Sprintf("%s_mem_max_usage", rp.ID)] = rp.MemMaxUsage
	mx[fmt.Sprintf("%s_mem_unreserved_for_vm", rp.ID)] = rp.MemUnreservedForVm

	mx[fmt.Sprintf("%s_cpu_reservation", rp.ID)] = rp.CpuReservation
	mx[fmt.Sprintf("%s_cpu_limit", rp.ID)] = rp.CpuLimit
	mx[fmt.Sprintf("%s_mem_reservation", rp.ID)] = rp.MemReservation
	mx[fmt.Sprintf("%s_mem_limit", rp.ID)] = rp.MemLimit

	for _, v := range overallStatuses {
		key := fmt.Sprintf("%s_overall.status.%s", rp.ID, v)
		mx[key] = oldmetrix.Bool(rp.OverallStatus == v)
	}
}
