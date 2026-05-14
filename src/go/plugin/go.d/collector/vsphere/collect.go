// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
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

var vmPowerStates = []struct {
	value string
	key   string
}{
	{value: string(types.VirtualMachinePowerStatePoweredOn), key: "poweredOn"},
	{value: string(types.VirtualMachinePowerStatePoweredOff), key: "poweredOff"},
	{value: string(types.VirtualMachinePowerStateSuspended), key: "suspended"},
}

var vmConnectionStates = []struct {
	value string
	key   string
}{
	{value: string(types.VirtualMachineConnectionStateConnected), key: "connected"},
	{value: string(types.VirtualMachineConnectionStateDisconnected), key: "disconnected"},
	{value: string(types.VirtualMachineConnectionStateOrphaned), key: "orphaned"},
	{value: string(types.VirtualMachineConnectionStateInaccessible), key: "inaccessible"},
	{value: string(types.VirtualMachineConnectionStateInvalid), key: "invalid"},
}

var vmToolsRunningStatuses = []struct {
	value string
	key   string
}{
	{value: string(types.VirtualMachineToolsRunningStatusGuestToolsRunning), key: "running"},
	{value: string(types.VirtualMachineToolsRunningStatusGuestToolsNotRunning), key: "notRunning"},
	{value: string(types.VirtualMachineToolsRunningStatusGuestToolsExecutingScripts), key: "executingScripts"},
}

var vmToolsVersionStatuses = []struct {
	value string
	key   string
}{
	{value: string(types.VirtualMachineToolsVersionStatusGuestToolsCurrent), key: "current"},
	{value: string(types.VirtualMachineToolsVersionStatusGuestToolsNeedUpgrade), key: "needUpgrade"},
	{value: string(types.VirtualMachineToolsVersionStatusGuestToolsNotInstalled), key: "notInstalled"},
	{value: string(types.VirtualMachineToolsVersionStatusGuestToolsUnmanaged), key: "unmanaged"},
	{value: string(types.VirtualMachineToolsVersionStatusGuestToolsTooOld), key: "tooOld"},
	{value: string(types.VirtualMachineToolsVersionStatusGuestToolsSupportedOld), key: "supportedOld"},
	{value: string(types.VirtualMachineToolsVersionStatusGuestToolsSupportedNew), key: "supportedNew"},
	{value: string(types.VirtualMachineToolsVersionStatusGuestToolsTooNew), key: "tooNew"},
	{value: string(types.VirtualMachineToolsVersionStatusGuestToolsBlacklisted), key: "blacklisted"},
}

var hostPowerStates = []struct {
	value string
	key   string
}{
	{value: string(types.HostSystemPowerStatePoweredOn), key: "poweredOn"},
	{value: string(types.HostSystemPowerStatePoweredOff), key: "poweredOff"},
	{value: string(types.HostSystemPowerStateStandBy), key: "standBy"},
	{value: string(types.HostSystemPowerStateUnknown), key: "unknown"},
}

var hostConnectionStates = []struct {
	value string
	key   string
}{
	{value: string(types.HostSystemConnectionStateConnected), key: "connected"},
	{value: string(types.HostSystemConnectionStateNotResponding), key: "notResponding"},
	{value: string(types.HostSystemConnectionStateDisconnected), key: "disconnected"},
}

var datastoreMaintenanceModes = []struct {
	value string
	key   string
}{
	{value: string(types.DatastoreSummaryMaintenanceModeStateNormal), key: "normal"},
	{value: string(types.DatastoreSummaryMaintenanceModeStateEnteringMaintenance), key: "enteringMaintenance"},
	{value: string(types.DatastoreSummaryMaintenanceModeStateInMaintenance), key: "inMaintenance"},
}

var clusterDRSModes = []struct {
	value string
	key   string
}{
	{value: string(types.DrsBehaviorManual), key: "manual"},
	{value: string(types.DrsBehaviorPartiallyAutomated), key: "partiallyAutomated"},
	{value: string(types.DrsBehaviorFullyAutomated), key: "fullyAutomated"},
}

var clusterHAServiceStates = []struct {
	value string
	key   string
}{
	{value: string(types.ClusterDasConfigInfoServiceStateEnabled), key: "enabled"},
	{value: string(types.ClusterDasConfigInfoServiceStateDisabled), key: "disabled"},
}

var clusterHAVMMonitoringStates = []struct {
	value string
	key   string
}{
	{value: string(types.ClusterDasConfigInfoVmMonitoringStateVmMonitoringDisabled), key: "vmMonitoringDisabled"},
	{value: string(types.ClusterDasConfigInfoVmMonitoringStateVmMonitoringOnly), key: "vmMonitoringOnly"},
	{value: string(types.ClusterDasConfigInfoVmMonitoringStateVmAndAppMonitoring), key: "vmAndAppMonitoring"},
}

const (
	recurringLogEvery = time.Hour

	logKeyHostNoPerfSamples             = "vsphere:host-no-perf-samples"
	logKeyVMNoPerfSamples               = "vsphere:vm-no-perf-samples"
	logKeyDatastorePropertyRefreshError = "vsphere:datastore-property-refresh-error"
	logKeyClusterPropertyRefreshError   = "vsphere:cluster-property-refresh-error"
	logKeyResourcePoolRefreshError      = "vsphere:resource-pool-property-refresh-error"
	logKeyDiscoveryError                = "vsphere:periodic-discovery-error"
)

func (c *Collector) collect() (map[string]int64, error) {
	c.collectionLock.Lock()
	defer c.collectionLock.Unlock()

	return c.collectLocked()
}

func (c *Collector) collectLocked() (map[string]int64, error) {
	c.Debug("starting collection process")
	t := time.Now()
	mx := make(map[string]int64)
	c.vmDiskPerfSamples = nil
	c.vmNICPerfSamples = nil
	c.hostNICPerfSamples = nil
	c.hostDiskPerfSamples = nil
	c.hostStorageAdapterPerfSamples = nil
	c.hostStorageAdapterAggregatePerfSamples = nil
	c.hostStoragePathPerfSamples = nil
	c.hostStoragePathAggregatePerfSamples = nil
	c.hostCPUInstancePerfSamples = nil
	c.hostPowerPerfSamples = nil
	c.vmPowerPerfSamples = nil
	c.vsanMetrics = nil

	c.collectInventory(mx)

	err := c.collectHosts(mx)
	if err != nil {
		return nil, fmt.Errorf("collect host metrics from vSphere resources: %w", err)
	}

	err = c.collectVMs(mx)
	if err != nil {
		return nil, fmt.Errorf("collect VM metrics from vSphere resources: %w", err)
	}

	c.collectDatastores(mx)
	c.collectClusters(mx)
	c.collectResourcePools(mx)
	c.collectVSAN()

	c.updateCharts()

	c.Debugf("metrics collected, process took %s", time.Since(t))

	return mx, nil
}

func (c *Collector) collectVSAN() {
	if !c.CollectVSAN || c.resources == nil {
		return
	}
	clusters, hosts, vms := c.vsanResources()
	c.vsanMetrics = c.ScrapeVSAN(clusters, hosts, vms)
}

func (c *Collector) collectInventory(mx map[string]int64) {
	if c.resources == nil {
		return
	}

	mx["inventory_datacenters"] = int64(len(c.resources.DataCenters))
	mx["inventory_folders"] = int64(len(c.resources.Folders))
	mx["inventory_clusters"] = int64(len(c.resources.Clusters))
	mx["inventory_hosts"] = int64(len(c.resources.Hosts))
	mx["inventory_vms"] = int64(len(c.resources.VMs))
	mx["inventory_datastores"] = int64(len(c.resources.Datastores))
	mx["inventory_resource_pools"] = int64(len(c.resources.ResourcePools))
}

func (c *Collector) collectHosts(mx map[string]int64) error {
	if len(c.resources.Hosts) == 0 {
		return nil
	}
	c.collectHostsPropertyMetrics(mx)

	poweredOnHosts := numPoweredOnHosts(c.resources.Hosts)
	if poweredOnHosts == 0 {
		return nil
	}

	// NOTE: returns unsorted if at least one types.PerfMetricId Instance is not ""
	metrics := c.ScrapeHosts(c.resources.Hosts)
	if len(metrics) == 0 {
		c.Limit(logKeyHostNoPerfSamples, 1, recurringLogEvery).
			Warningf("collect host performance metrics: vSphere returned no samples for %d powered-on host(s) out of %d discovered host(s)", poweredOnHosts, len(c.resources.Hosts))
		return nil
	}

	c.collectHostsMetrics(mx, metrics)

	return nil
}

func (c *Collector) collectHostsPropertyMetrics(mx map[string]int64) {
	for k := range c.discoveredHosts {
		c.discoveredHosts[k]++
	}

	for _, host := range c.resources.Hosts {
		c.discoveredHosts[host.ID] = 0
		writeHostPropertyMetrics(mx, host)
	}
}

func (c *Collector) collectHostsMetrics(mx map[string]int64, metrics []performance.EntityMetric) {
	for _, metric := range metrics {
		if host := c.resources.Hosts.Get(metric.Entity.Value); host != nil {
			c.discoveredHosts[host.ID] = 0
			writeHostPerfMetrics(mx, host, metric.Value)
			if c.CollectHostNICPerformance {
				c.collectHostNICPerformanceMetrics(host, metric.Value)
			}
			if c.CollectHostDiskPerformance {
				c.collectHostDiskPerformanceMetrics(host, metric.Value)
			}
			if c.CollectHostStorageAdapterPerformance {
				c.collectHostStorageAdapterPerformanceMetrics(host, metric.Value)
			}
			if c.CollectHostStoragePathPerformance {
				c.collectHostStoragePathPerformanceMetrics(host, metric.Value)
			}
			if c.CollectHostCPUInstancePerformance {
				c.collectHostCPUInstancePerformanceMetrics(host, metric.Value)
			}
			if c.CollectPowerMetrics {
				c.collectHostPowerMetrics(host, metric.Value)
			}
		}
	}
}

func numPoweredOnHosts(hosts rs.Hosts) (num int) {
	for _, host := range hosts {
		if host.IsPoweredOn() {
			num++
		}
	}
	return num
}

func writeHostPerfMetrics(mx map[string]int64, host *rs.Host, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		if metric.Instance != "" {
			continue
		}
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		key := fmt.Sprintf("%s_%s", host.ID, metric.Name)
		mx[key] = metric.Value[0]
	}
}

func writeHostPropertyMetrics(mx map[string]int64, host *rs.Host) {
	for _, v := range overallStatuses {
		key := fmt.Sprintf("%s_overall.status.%s", host.ID, v)
		mx[key] = oldmetrix.Bool(host.OverallStatus == v)
	}
	for _, v := range hostPowerStates {
		key := fmt.Sprintf("%s_power_state.%s", host.ID, v.key)
		mx[key] = oldmetrix.Bool(host.PowerState == v.value)
	}
	for _, v := range hostConnectionStates {
		key := fmt.Sprintf("%s_connection_state.%s", host.ID, v.key)
		mx[key] = oldmetrix.Bool(host.ConnectionState == v.value)
	}
	mx[fmt.Sprintf("%s_maintenance_status.inMaintenance", host.ID)] = oldmetrix.Bool(host.InMaintenanceMode)
	mx[fmt.Sprintf("%s_maintenance_status.normal", host.ID)] = oldmetrix.Bool(!host.InMaintenanceMode)
}

func (c *Collector) collectVMs(mx map[string]int64) error {
	if len(c.resources.VMs) == 0 {
		return nil
	}
	c.collectVMsPropertyMetrics(mx)

	poweredOnVMs := numPoweredOnVMs(c.resources.VMs)
	if poweredOnVMs == 0 {
		return nil
	}

	// NOTE: returns unsorted if at least one types.PerfMetricId Instance is not ""
	ems := c.ScrapeVMs(c.resources.VMs)
	if len(ems) == 0 {
		c.Limit(logKeyVMNoPerfSamples, 1, recurringLogEvery).
			Warningf("collect VM performance metrics: vSphere returned no samples for %d powered-on VM(s) out of %d discovered VM(s)", poweredOnVMs, len(c.resources.VMs))
		return nil
	}

	c.collectVMsMetrics(mx, ems)

	return nil
}

func (c *Collector) collectVMsPropertyMetrics(mx map[string]int64) {
	for id := range c.discoveredVMs {
		c.discoveredVMs[id]++
	}

	for _, vm := range c.resources.VMs {
		c.discoveredVMs[vm.ID] = 0
		writeVMPropertyMetrics(mx, vm)
	}
}

func (c *Collector) collectVMsMetrics(mx map[string]int64, metrics []performance.EntityMetric) {
	for _, metric := range metrics {
		if vm := c.resources.VMs.Get(metric.Entity.Value); vm != nil {
			writeVMPerfMetrics(mx, vm, metric.Value)
			if c.CollectVMDiskPerformance {
				c.collectVMDiskPerformanceMetrics(vm, metric.Value)
			}
			if c.CollectVMNICPerformance {
				c.collectVMNICPerformanceMetrics(vm, metric.Value)
			}
			if c.CollectPowerMetrics {
				c.collectVMPowerMetrics(vm, metric.Value)
			}
			c.discoveredVMs[vm.ID] = 0
		}
	}
}

func numPoweredOnVMs(vms rs.VMs) (num int) {
	for _, vm := range vms {
		if vm.IsPoweredOn() {
			num++
		}
	}
	return num
}

func writeVMPerfMetrics(mx map[string]int64, vm *rs.VM, metrics []performance.MetricSeries) {
	for _, metric := range metrics {
		if metric.Instance != "" || isVMDiskPerformanceMetric(metric.Name) {
			continue
		}
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		key := fmt.Sprintf("%s_%s", vm.ID, metric.Name)
		mx[key] = metric.Value[0]
	}
}

func writeVMPropertyMetrics(mx map[string]int64, vm *rs.VM) {
	for _, v := range overallStatuses {
		key := fmt.Sprintf("%s_overall.status.%s", vm.ID, v)
		mx[key] = oldmetrix.Bool(vm.OverallStatus == v)
	}
	for _, v := range vmPowerStates {
		key := fmt.Sprintf("%s_power_state.%s", vm.ID, v.key)
		mx[key] = oldmetrix.Bool(vm.PowerState == v.value)
	}
	for _, v := range vmConnectionStates {
		key := fmt.Sprintf("%s_connection_state.%s", vm.ID, v.key)
		mx[key] = oldmetrix.Bool(vm.ConnectionState == v.value)
	}
	toolsRunningStatusKnown := false
	for _, v := range vmToolsRunningStatuses {
		ok := vm.ToolsRunningStatus == v.value
		key := fmt.Sprintf("%s_tools_running_status.%s", vm.ID, v.key)
		mx[key] = oldmetrix.Bool(ok)
		toolsRunningStatusKnown = toolsRunningStatusKnown || ok
	}
	mx[fmt.Sprintf("%s_tools_running_status.unknown", vm.ID)] = oldmetrix.Bool(!toolsRunningStatusKnown)

	toolsVersionStatusKnown := false
	for _, v := range vmToolsVersionStatuses {
		ok := vm.ToolsVersionStatus == v.value
		key := fmt.Sprintf("%s_tools_version_status.%s", vm.ID, v.key)
		mx[key] = oldmetrix.Bool(ok)
		toolsVersionStatusKnown = toolsVersionStatusKnown || ok
	}
	mx[fmt.Sprintf("%s_tools_version_status.unknown", vm.ID)] = oldmetrix.Bool(!toolsVersionStatusKnown)

	mx[fmt.Sprintf("%s_consolidation_needed.needed", vm.ID)] = oldmetrix.Bool(vm.ConsolidationNeeded)
	mx[fmt.Sprintf("%s_consolidation_needed.notNeeded", vm.ID)] = oldmetrix.Bool(!vm.ConsolidationNeeded)
	mx[fmt.Sprintf("%s_config_cpu", vm.ID)] = vm.ConfigCPU
	mx[fmt.Sprintf("%s_config_memory", vm.ID)] = vm.ConfigMemory
	mx[fmt.Sprintf("%s_config_devices.disks", vm.ID)] = vm.ConfigDisks
	mx[fmt.Sprintf("%s_config_devices.nics", vm.ID)] = vm.ConfigNICs
	mx[fmt.Sprintf("%s_storage.committed", vm.ID)] = vm.StorageCommitted
	mx[fmt.Sprintf("%s_storage.uncommitted", vm.ID)] = vm.StorageUncommitted
	mx[fmt.Sprintf("%s_storage.unshared", vm.ID)] = vm.StorageUnshared
	mx[fmt.Sprintf("%s_snapshot_count", vm.ID)] = vm.SnapshotCount
	mx[fmt.Sprintf("%s_snapshot_max_chain_depth", vm.ID)] = vm.SnapshotMaxChainDepth
	mx[fmt.Sprintf("%s_snapshot_max_age", vm.ID)] = snapshotMaxAgeSeconds(vm.SnapshotOldestCreateTime)
}

func snapshotMaxAgeSeconds(oldest time.Time) int64 {
	if oldest.IsZero() {
		return 0
	}
	age := time.Since(oldest).Seconds()
	if age < 0 {
		return 0
	}
	return int64(age)
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

	pathSet := []string{"summary", "overallStatus"}
	dsList, err := c.dsPropertyCollector.DatastoresByRef(refs, pathSet...)
	if err != nil {
		c.Limit(logKeyDatastorePropertyRefreshError, 1, recurringLogEvery).
			Warningf("collect vSphere datastore properties refresh: refs=%d pathSet=%v: %v", len(refs), pathSet, err)
		return refreshed
	}

	for _, raw := range dsList {
		ds := c.resources.Datastores.Get(raw.Reference().Value)
		if ds == nil {
			continue
		}
		refreshed[ds.ID] = true
		ds.Type = raw.Summary.Type
		ds.Capacity = raw.Summary.Capacity
		ds.FreeSpace = raw.Summary.FreeSpace
		ds.Uncommitted = raw.Summary.Uncommitted
		ds.Accessible = raw.Summary.Accessible
		ds.MaintenanceMode = raw.Summary.MaintenanceMode
		ds.MultipleHostAccess = raw.Summary.MultipleHostAccess
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
	var capacity, freeSpace, used, uncommitted int64
	if ds.Accessible {
		capacity = ds.Capacity
		freeSpace = ds.FreeSpace
		used = max(capacity-freeSpace, 0)
		uncommitted = ds.Uncommitted
	}

	mx[fmt.Sprintf("%s_capacity", ds.ID)] = capacity
	mx[fmt.Sprintf("%s_free_space", ds.ID)] = freeSpace
	mx[fmt.Sprintf("%s_used_space", ds.ID)] = used
	mx[fmt.Sprintf("%s_uncommitted", ds.ID)] = uncommitted

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

	mx[fmt.Sprintf("%s_accessible_status.accessible", ds.ID)] = oldmetrix.Bool(ds.Accessible)
	mx[fmt.Sprintf("%s_accessible_status.inaccessible", ds.ID)] = oldmetrix.Bool(!ds.Accessible)

	maintenanceModeKnown := false
	for _, mode := range datastoreMaintenanceModes {
		ok := ds.MaintenanceMode == mode.value
		maintenanceModeKnown = maintenanceModeKnown || ok
		mx[fmt.Sprintf("%s_maintenance.status.%s", ds.ID, mode.key)] = oldmetrix.Bool(ok)
	}
	mx[fmt.Sprintf("%s_maintenance.status.unknown", ds.ID)] = oldmetrix.Bool(!maintenanceModeKnown)

	mx[fmt.Sprintf("%s_multiple_host_access.enabled", ds.ID)] = oldmetrix.Bool(ds.MultipleHostAccess != nil && *ds.MultipleHostAccess)
	mx[fmt.Sprintf("%s_multiple_host_access.disabled", ds.ID)] = oldmetrix.Bool(ds.MultipleHostAccess != nil && !*ds.MultipleHostAccess)
	mx[fmt.Sprintf("%s_multiple_host_access.unknown", ds.ID)] = oldmetrix.Bool(ds.MultipleHostAccess == nil)
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

	pathSet := []string{"name", "summary", "configurationEx", "overallStatus"}
	clusters, err := c.clusterPropertyCollector.ClustersByRef(refs, pathSet...)
	if err != nil {
		c.Limit(logKeyClusterPropertyRefreshError, 1, recurringLogEvery).
			Warningf("collect vSphere cluster properties refresh: refs=%d pathSet=%v: %v", len(refs), pathSet, err)
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
	cl.DrsVmotionRate = 0
	cl.HaEnabled = false
	cl.HaAdmCtrlEnabled = false
	cl.HaHostMonitoring = ""
	cl.HaVMMonitoring = ""
	cl.HaVMComponentProtection = ""

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
		rs.SetClusterVSANInfo(cl, cfg)
		if cfg.DrsConfig.Enabled != nil {
			cl.DrsEnabled = *cfg.DrsConfig.Enabled
		}
		cl.DrsMode = string(cfg.DrsConfig.DefaultVmBehavior)
		cl.DrsVmotionRate = cfg.DrsConfig.VmotionRate
		if cfg.DasConfig.Enabled != nil {
			cl.HaEnabled = *cfg.DasConfig.Enabled
		}
		if cfg.DasConfig.AdmissionControlEnabled != nil {
			cl.HaAdmCtrlEnabled = *cfg.DasConfig.AdmissionControlEnabled
		}
		cl.HaHostMonitoring = cfg.DasConfig.HostMonitoring
		cl.HaVMMonitoring = cfg.DasConfig.VmMonitoring
		cl.HaVMComponentProtection = cfg.DasConfig.VmComponentProtecting
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
	drsModeKnown := false
	for _, v := range clusterDRSModes {
		ok := cl.DrsMode == v.value
		mx[fmt.Sprintf("%s_drs_mode.%s", cl.ID, v.key)] = oldmetrix.Bool(ok)
		drsModeKnown = drsModeKnown || ok
	}
	mx[fmt.Sprintf("%s_drs_mode.unknown", cl.ID)] = oldmetrix.Bool(!drsModeKnown)
	mx[fmt.Sprintf("%s_drs_vmotion_rate", cl.ID)] = int64(cl.DrsVmotionRate)

	mx[fmt.Sprintf("%s_ha_enabled", cl.ID)] = oldmetrix.Bool(cl.HaEnabled)
	mx[fmt.Sprintf("%s_ha_adm_ctrl_enabled", cl.ID)] = oldmetrix.Bool(cl.HaAdmCtrlEnabled)
	writeClusterHAServiceState(mx, cl.ID, "ha_host_monitoring", cl.HaHostMonitoring)
	writeClusterHAVMMonitoringState(mx, cl.ID, cl.HaVMMonitoring)
	writeClusterHAServiceState(mx, cl.ID, "ha_vm_component_protection", cl.HaVMComponentProtection)

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

func writeClusterHAServiceState(mx map[string]int64, id, prefix, state string) {
	known := false
	for _, v := range clusterHAServiceStates {
		ok := state == v.value
		mx[fmt.Sprintf("%s_%s.%s", id, prefix, v.key)] = oldmetrix.Bool(ok)
		known = known || ok
	}
	mx[fmt.Sprintf("%s_%s.unknown", id, prefix)] = oldmetrix.Bool(!known)
}

func writeClusterHAVMMonitoringState(mx map[string]int64, id, state string) {
	known := false
	for _, v := range clusterHAVMMonitoringStates {
		ok := state == v.value
		mx[fmt.Sprintf("%s_ha_vm_monitoring.%s", id, v.key)] = oldmetrix.Bool(ok)
		known = known || ok
	}
	mx[fmt.Sprintf("%s_ha_vm_monitoring.unknown", id)] = oldmetrix.Bool(!known)
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

	pathSet := []string{"name", "summary", "config", "runtime", "overallStatus"}
	pools, err := c.rpPropertyCollector.ResourcePoolsByRef(refs, pathSet...)
	if err != nil {
		c.Limit(logKeyResourcePoolRefreshError, 1, recurringLogEvery).
			Warningf("collect vSphere resource pool properties refresh: refs=%d pathSet=%v: %v", len(refs), pathSet, err)
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

	// Runtime and Config are value structs in mo.ResourcePool; missing properties
	// decode as zero values rather than nil pointers.
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
	// vSphere reports CompressedMemory in KiB; the chart keeps V1's MB display scale.
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
