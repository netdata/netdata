// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
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

var hostPerfMetricByCounter = map[string]string{
	"cpu.usage.average":           "host_cpu_utilization_used",
	"mem.usage.average":           "host_mem_utilization_used",
	"mem.granted.average":         "host_mem_usage_granted",
	"mem.consumed.average":        "host_mem_usage_consumed",
	"mem.active.average":          "host_mem_usage_active",
	"mem.shared.average":          "host_mem_usage_shared",
	"mem.sharedcommon.average":    "host_mem_usage_sharedcommon",
	"mem.swapinRate.average":      "host_mem_swap_io_in",
	"mem.swapoutRate.average":     "host_mem_swap_io_out",
	"disk.read.average":           "host_disk_io_read",
	"disk.write.average":          "host_disk_io_write",
	"disk.maxTotalLatency.latest": "host_disk_max_latency_latency",
	"net.bytesRx.average":         "host_net_traffic_received",
	"net.bytesTx.average":         "host_net_traffic_sent",
	"net.packetsRx.summation":     "host_net_packets_received",
	"net.packetsTx.summation":     "host_net_packets_sent",
	"net.droppedRx.summation":     "host_net_drops_received",
	"net.droppedTx.summation":     "host_net_drops_sent",
	"net.errorsRx.summation":      "host_net_errors_received",
	"net.errorsTx.summation":      "host_net_errors_sent",
	"sys.uptime.latest":           "host_system_uptime_uptime",
}

var vmPerfMetricByCounter = map[string]string{
	"cpu.usage.average":           "vm_cpu_utilization_used",
	"mem.usage.average":           "vm_mem_utilization_used",
	"mem.granted.average":         "vm_mem_usage_granted",
	"mem.consumed.average":        "vm_mem_usage_consumed",
	"mem.active.average":          "vm_mem_usage_active",
	"mem.shared.average":          "vm_mem_usage_shared",
	"mem.swapped.average":         "vm_mem_swap_usage_swapped",
	"mem.swapinRate.average":      "vm_mem_swap_io_in",
	"mem.swapoutRate.average":     "vm_mem_swap_io_out",
	"disk.read.average":           "vm_disk_io_read",
	"disk.write.average":          "vm_disk_io_write",
	"disk.maxTotalLatency.latest": "vm_disk_max_latency_latency",
	"net.bytesRx.average":         "vm_net_traffic_received",
	"net.bytesTx.average":         "vm_net_traffic_sent",
	"net.packetsRx.summation":     "vm_net_packets_received",
	"net.packetsTx.summation":     "vm_net_packets_sent",
	"net.droppedRx.summation":     "vm_net_drops_received",
	"net.droppedTx.summation":     "vm_net_drops_sent",
	"sys.uptime.latest":           "vm_system_uptime_uptime",
}

var datastorePerfMetricByCounter = map[string]string{
	"datastore.read.average":                "datastore_disk_io_read",
	"datastore.write.average":               "datastore_disk_io_write",
	"datastore.numberReadAveraged.average":  "datastore_disk_iops_reads",
	"datastore.numberWriteAveraged.average": "datastore_disk_iops_writes",
	"datastore.totalReadLatency.average":    "datastore_disk_latency_read",
	"datastore.totalWriteLatency.average":   "datastore_disk_latency_write",
}

var clusterPerfMetricByCounter = map[string]string{
	"cpu.usage.average":                    "cluster_cpu_utilization_used",
	"cpu.usagemhz.average":                 "cluster_cpu_usage_used",
	"cpu.totalmhz.average":                 "cluster_cpu_usage_total",
	"mem.usage.average":                    "cluster_mem_utilization_used",
	"mem.consumed.average":                 "cluster_mem_usage_consumed",
	"mem.active.average":                   "cluster_mem_usage_active",
	"mem.granted.average":                  "cluster_mem_usage_granted",
	"mem.shared.average":                   "cluster_mem_usage_shared",
	"mem.overhead.average":                 "cluster_mem_usage_overhead",
	"mem.swapused.average":                 "cluster_mem_usage_swap_used",
	"clusterServices.effectivecpu.average": "cluster_services_effective_cpu_effective_cpu",
	"clusterServices.effectivemem.average": "cluster_services_effective_mem_effective_mem",
	"clusterServices.cpufairness.latest":   "cluster_services_fairness_cpu",
	"clusterServices.memfairness.latest":   "cluster_services_fairness_memory",
	"clusterServices.failover.latest":      "cluster_services_failover_failures_tolerable",
	"vmop.numVMotion.latest":               "cluster_vm_migrations_vmotion",
	"vmop.numSVMotion.latest":              "cluster_vm_migrations_svmotion",
	"vmop.numXVMotion.latest":              "cluster_vm_migrations_xvmotion",
	"vmop.numPoweron.latest":               "cluster_vm_lifecycle_poweron",
	"vmop.numPoweroff.latest":              "cluster_vm_lifecycle_poweroff",
	"vmop.numCreate.latest":                "cluster_vm_lifecycle_create",
	"vmop.numDestroy.latest":               "cluster_vm_lifecycle_destroy",
	"vmop.numClone.latest":                 "cluster_vm_lifecycle_clone",
	"vmop.numDeploy.latest":                "cluster_vm_lifecycle_deploy",
	"vmop.numReconfigure.latest":           "cluster_vm_management_reconfigure",
	"vmop.numReset.latest":                 "cluster_vm_management_reset",
	"vmop.numSuspend.latest":               "cluster_vm_management_suspend",
	"vmop.numRegister.latest":              "cluster_vm_management_register",
	"vmop.numUnregister.latest":            "cluster_vm_management_unregister",
	"vmop.numRebootGuest.latest":           "cluster_vm_guest_ops_reboot",
	"vmop.numShutdownGuest.latest":         "cluster_vm_guest_ops_shutdown",
	"vmop.numStandbyGuest.latest":          "cluster_vm_guest_ops_standby",
	"vmop.numChangeDS.latest":              "cluster_vm_cold_migrations_change_ds",
	"vmop.numChangeHost.latest":            "cluster_vm_cold_migrations_change_host",
	"vmop.numChangeHostDS.latest":          "cluster_vm_cold_migrations_change_host_ds",
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

func (c *Collector) collectLocked() error {
	c.Debug("starting collection process")
	t := time.Now()
	c.hostPowerPerfSamples = nil
	c.vmPowerPerfSamples = nil
	c.vsanMetrics = nil

	c.collectInventory()

	err := c.collectHosts()
	if err != nil {
		return fmt.Errorf("collect host metrics from vSphere resources: %w", err)
	}

	err = c.collectVMs()
	if err != nil {
		return fmt.Errorf("collect VM metrics from vSphere resources: %w", err)
	}

	c.collectDatastores()
	c.collectClusters()
	c.collectResourcePools()
	c.collectVSAN()
	c.writeDatastoreClusterMetrics()
	c.writePowerMetrics()
	c.writeVSANMetrics()

	c.Debugf("metrics collected, process took %s", time.Since(t))

	return nil
}

func (c *Collector) collectVSAN() {
	if !c.CollectVSAN || c.resources == nil {
		return
	}
	clusters, hosts, vms := c.vsanResources()
	c.vsanMetrics = c.ScrapeVSAN(clusters, hosts, vms)
}

func (c *Collector) collectInventory() {
	if c.resources == nil {
		return
	}

	labels := c.inventoryLabelSet()
	c.observeGauge("inventory_objects_datacenters", int64(len(c.resources.DataCenters)), labels)
	c.observeGauge("inventory_objects_folders", int64(len(c.resources.Folders)), labels)
	c.observeGauge("inventory_objects_clusters", int64(len(c.resources.Clusters)), labels)
	c.observeGauge("inventory_objects_hosts", int64(len(c.resources.Hosts)), labels)
	c.observeGauge("inventory_objects_vms", int64(len(c.resources.VMs)), labels)
	c.observeGauge("inventory_objects_datastores", int64(len(c.resources.Datastores)), labels)
	c.observeGauge("inventory_objects_resource_pools", int64(len(c.resources.ResourcePools)), labels)
}

func (c *Collector) collectHosts() error {
	if len(c.resources.Hosts) == 0 {
		return nil
	}
	c.collectHostsPropertyMetrics()

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

	c.collectHostsMetrics(metrics)

	return nil
}

func (c *Collector) collectHostsPropertyMetrics() {
	for _, host := range c.resources.Hosts {
		c.writeHostPropertyMetrics(host)
	}
}

func (c *Collector) collectHostsMetrics(metrics []performance.EntityMetric) {
	for _, metric := range metrics {
		if host := c.resources.Hosts.Get(metric.Entity.Value); host != nil {
			c.writeHostPerfMetrics(host, metric.Value)
			c.collectHostPowerMetrics(host, metric.Value)
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

func (c *Collector) writeHostPerfMetrics(host *rs.Host, metrics []performance.MetricSeries) {
	labels := c.hostLabelSet(host)
	for _, metric := range metrics {
		if _, ok := hostPowerMetricByCounter[metric.Name]; ok {
			continue
		}
		if metric.Instance != "" {
			continue
		}
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		name := hostPerfMetricByCounter[metric.Name]
		if name == "" {
			continue
		}
		c.observeGauge(name, metric.Value[0], labels)
	}
}

func (c *Collector) writeHostPropertyMetrics(host *rs.Host) {
	labels := c.hostLabelSet(host)
	for _, v := range overallStatuses {
		c.observeGauge("host_overall_status_"+v, oldmetrix.Bool(host.OverallStatus == v), labels)
	}
	for _, v := range hostPowerStates {
		c.observeGauge("host_power_state_"+snakeStatus(v.key), oldmetrix.Bool(host.PowerState == v.value), labels)
	}
	for _, v := range hostConnectionStates {
		c.observeGauge("host_connection_state_"+snakeStatus(v.key), oldmetrix.Bool(host.ConnectionState == v.value), labels)
	}
	c.observeGauge("host_maintenance_status_in_maintenance", oldmetrix.Bool(host.InMaintenanceMode), labels)
	c.observeGauge("host_maintenance_status_normal", oldmetrix.Bool(!host.InMaintenanceMode), labels)
}

func (c *Collector) collectVMs() error {
	if len(c.resources.VMs) == 0 {
		return nil
	}
	c.collectVMsPropertyMetrics()

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

	c.collectVMsMetrics(ems)

	return nil
}

func (c *Collector) collectVMsPropertyMetrics() {
	for _, vm := range c.resources.VMs {
		c.writeVMPropertyMetrics(vm)
	}
}

func (c *Collector) collectVMsMetrics(metrics []performance.EntityMetric) {
	for _, metric := range metrics {
		if vm := c.resources.VMs.Get(metric.Entity.Value); vm != nil {
			c.writeVMPerfMetrics(vm, metric.Value)
			c.collectVMPowerMetrics(vm, metric.Value)
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

func (c *Collector) writeVMPerfMetrics(vm *rs.VM, metrics []performance.MetricSeries) {
	labels := c.vmLabelSet(vm)
	for _, metric := range metrics {
		if _, ok := vmPowerMetricByCounter[metric.Name]; ok {
			continue
		}
		if metric.Instance != "" {
			continue
		}
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		name := vmPerfMetricByCounter[metric.Name]
		if name == "" {
			continue
		}
		c.observeGauge(name, metric.Value[0], labels)
	}
}

func (c *Collector) writeVMPropertyMetrics(vm *rs.VM) {
	labels := c.vmLabelSet(vm)
	for _, v := range overallStatuses {
		c.observeGauge("vm_overall_status_"+v, oldmetrix.Bool(vm.OverallStatus == v), labels)
	}
	for _, v := range vmPowerStates {
		c.observeGauge("vm_power_state_"+snakeStatus(v.key), oldmetrix.Bool(vm.PowerState == v.value), labels)
	}
	for _, v := range vmConnectionStates {
		c.observeGauge("vm_connection_state_"+snakeStatus(v.key), oldmetrix.Bool(vm.ConnectionState == v.value), labels)
	}
	toolsRunningStatusKnown := false
	for _, v := range vmToolsRunningStatuses {
		ok := vm.ToolsRunningStatus == v.value
		c.observeGauge("vm_tools_running_status_"+snakeStatus(v.key), oldmetrix.Bool(ok), labels)
		toolsRunningStatusKnown = toolsRunningStatusKnown || ok
	}
	c.observeGauge("vm_tools_running_status_unknown", oldmetrix.Bool(!toolsRunningStatusKnown), labels)

	toolsVersionStatusKnown := false
	for _, v := range vmToolsVersionStatuses {
		ok := vm.ToolsVersionStatus == v.value
		c.observeGauge("vm_tools_version_status_"+snakeStatus(v.key), oldmetrix.Bool(ok), labels)
		toolsVersionStatusKnown = toolsVersionStatusKnown || ok
	}
	c.observeGauge("vm_tools_version_status_unknown", oldmetrix.Bool(!toolsVersionStatusKnown), labels)

	c.observeGauge("vm_consolidation_needed_needed", oldmetrix.Bool(vm.ConsolidationNeeded), labels)
	c.observeGauge("vm_consolidation_needed_not_needed", oldmetrix.Bool(!vm.ConsolidationNeeded), labels)
	c.observeGauge("vm_config_cpu_vcpus", vm.ConfigCPU, labels)
	c.observeGauge("vm_config_memory_memory", vm.ConfigMemory, labels)
	c.observeGauge("vm_config_devices_disks", vm.ConfigDisks, labels)
	c.observeGauge("vm_config_devices_nics", vm.ConfigNICs, labels)
	c.observeGauge("vm_storage_usage_committed", vm.StorageCommitted, labels)
	c.observeGauge("vm_storage_usage_uncommitted", vm.StorageUncommitted, labels)
	c.observeGauge("vm_storage_usage_unshared", vm.StorageUnshared, labels)
	c.observeGauge("vm_snapshot_count_count", vm.SnapshotCount, labels)
	c.observeGauge("vm_snapshot_max_chain_depth_depth", vm.SnapshotMaxChainDepth, labels)
	c.observeGauge("vm_snapshot_max_age_age", snapshotMaxAgeSeconds(vm.SnapshotOldestCreateTime), labels)
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

func snakeStatus(s string) string {
	switch s {
	// Chart selectors use the VMware UI spelling, not the enum's camel-case suffix.
	case "standBy":
		return "standby"
	// The chart dimension is "disabled"; the enum name includes the monitored subsystem.
	case "vmMonitoringDisabled":
		return "disabled"
	}

	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (c *Collector) collectDatastores() {
	if len(c.resources.Datastores) == 0 {
		return
	}

	refreshed := c.refreshDatastoreProperties()

	metrics := c.ScrapeDatastores(c.resources.Datastores)
	// Datastore perf counters may return empty for vSAN or when no historical data is available yet.
	// This is not an error — we still collect capacity and status from properties.
	c.collectDatastoresMetrics(metrics, refreshed)
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

func (c *Collector) collectDatastoresMetrics(metrics []performance.EntityMetric, refreshed map[string]bool) {
	// Property metrics reflect cached resource fields, so emit them only after
	// the current refresh succeeds. Perf samples are fetched separately.
	for _, ds := range c.resources.Datastores {
		if refreshed[ds.ID] {
			c.writeDatastoreMetrics(ds)
		}
	}

	for _, metric := range metrics {
		if ds := c.resources.Datastores.Get(metric.Entity.Value); ds != nil {
			c.writeDatastorePerfMetrics(ds, metric.Value)
		}
	}
}

func (c *Collector) writeDatastoreMetrics(ds *rs.Datastore) {
	// VMware docs: Capacity and FreeSpace are guaranteed valid only when Accessible is true.
	var capacity, freeSpace, used, uncommitted int64
	if ds.Accessible {
		capacity = ds.Capacity
		freeSpace = ds.FreeSpace
		used = max(capacity-freeSpace, 0)
		uncommitted = ds.Uncommitted
	}

	labels := c.datastoreLabelSet(ds)
	c.observeGauge("datastore_space_usage_capacity", capacity, labels)
	c.observeGauge("datastore_space_usage_free", freeSpace, labels)
	c.observeGauge("datastore_space_usage_used", used, labels)
	c.observeGauge("datastore_space_usage_uncommitted", uncommitted, labels)

	if capacity > 0 {
		// use float64 to avoid int64 overflow on datastores larger than 922 TB
		c.observeGauge("datastore_space_utilization_used", int64(float64(used)/float64(capacity)*scaledPercent), labels)
	} else {
		c.observeGauge("datastore_space_utilization_used", 0, labels)
	}

	for _, v := range overallStatuses {
		c.observeGauge("datastore_overall_status_"+v, oldmetrix.Bool(ds.OverallStatus == v), labels)
	}

	c.observeGauge("datastore_accessibility_status_accessible", oldmetrix.Bool(ds.Accessible), labels)
	c.observeGauge("datastore_accessibility_status_inaccessible", oldmetrix.Bool(!ds.Accessible), labels)

	maintenanceModeKnown := false
	for _, mode := range datastoreMaintenanceModes {
		ok := ds.MaintenanceMode == mode.value
		maintenanceModeKnown = maintenanceModeKnown || ok
		c.observeGauge("datastore_maintenance_status_"+snakeStatus(mode.key), oldmetrix.Bool(ok), labels)
	}
	c.observeGauge("datastore_maintenance_status_unknown", oldmetrix.Bool(!maintenanceModeKnown), labels)

	c.observeGauge("datastore_multiple_host_access_enabled", oldmetrix.Bool(ds.MultipleHostAccess != nil && *ds.MultipleHostAccess), labels)
	c.observeGauge("datastore_multiple_host_access_disabled", oldmetrix.Bool(ds.MultipleHostAccess != nil && !*ds.MultipleHostAccess), labels)
	c.observeGauge("datastore_multiple_host_access_unknown", oldmetrix.Bool(ds.MultipleHostAccess == nil), labels)
}

func (c *Collector) writeDatastorePerfMetrics(ds *rs.Datastore, metrics []performance.MetricSeries) {
	labels := c.datastoreLabelSet(ds)
	for _, metric := range metrics {
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		name := datastorePerfMetricByCounter[metric.Name]
		if name == "" {
			continue
		}
		c.observeGauge(name, metric.Value[0], labels)
	}
}

func (c *Collector) collectClusters() {
	if len(c.resources.Clusters) == 0 {
		return
	}

	refreshed := c.refreshClusterProperties()

	metrics := c.ScrapeClusters(c.resources.Clusters)
	c.collectClustersMetrics(metrics, refreshed)
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

func (c *Collector) collectClustersMetrics(metrics []performance.EntityMetric, refreshed map[string]bool) {
	// Property metrics reflect cached resource fields, so emit them only after
	// the current refresh succeeds. Perf samples are fetched separately.
	for _, cl := range c.resources.Clusters {
		if refreshed[cl.ID] {
			c.writeClusterPropertyMetrics(cl)
		}
	}

	for _, metric := range metrics {
		if cl := c.resources.Clusters.Get(metric.Entity.Value); cl != nil {
			c.writeClusterPerfMetrics(cl, metric.Value)
		}
	}
}

func (c *Collector) writeClusterPropertyMetrics(cl *rs.Cluster) {
	labels := c.clusterLabelSet(cl)
	c.observeGauge("cluster_hosts_total", int64(cl.NumHosts), labels)
	c.observeGauge("cluster_hosts_effective", int64(cl.NumEffectiveHosts), labels)
	c.observeGauge("cluster_cpu_capacity_total", int64(cl.TotalCpu), labels)
	c.observeGauge("cluster_cpu_capacity_effective", int64(cl.EffectiveCpu), labels)
	c.observeGauge("cluster_mem_capacity_total", cl.TotalMemory, labels)
	// EffectiveMemory is MB from API, convert to bytes for consistency with TotalMemory
	c.observeGauge("cluster_mem_capacity_effective", cl.EffectiveMemory*1024*1024, labels)
	c.observeGauge("cluster_cpu_topology_cores", int64(cl.NumCpuCores), labels)
	c.observeGauge("cluster_cpu_topology_threads", int64(cl.NumCpuThreads), labels)
	c.observeGauge("cluster_vmotions_vmotions", int64(cl.NumVmotions), labels)
	c.observeGauge("cluster_drs_score_score", int64(cl.DrsScore), labels)
	c.observeGauge("cluster_drs_balance_current", int64(cl.CurrentBalance), labels)
	c.observeGauge("cluster_drs_balance_target", int64(cl.TargetBalance), labels)

	c.observeGauge("cluster_drs_config_enabled", oldmetrix.Bool(cl.DrsEnabled), labels)
	drsModeKnown := false
	for _, v := range clusterDRSModes {
		ok := cl.DrsMode == v.value
		c.observeGauge("cluster_drs_mode_"+snakeStatus(v.key), oldmetrix.Bool(ok), labels)
		drsModeKnown = drsModeKnown || ok
	}
	c.observeGauge("cluster_drs_mode_unknown", oldmetrix.Bool(!drsModeKnown), labels)
	c.observeGauge("cluster_drs_vmotion_rate_rate", int64(cl.DrsVmotionRate), labels)

	c.observeGauge("cluster_ha_config_enabled", oldmetrix.Bool(cl.HaEnabled), labels)
	c.observeGauge("cluster_ha_config_admission_control", oldmetrix.Bool(cl.HaAdmCtrlEnabled), labels)
	c.writeClusterHAServiceState("cluster_ha_host_monitoring", cl.HaHostMonitoring, labels)
	c.writeClusterHAVMMonitoringState(cl.HaVMMonitoring, labels)
	c.writeClusterHAServiceState("cluster_ha_vm_component_protection", cl.HaVMComponentProtection, labels)

	c.observeGauge("cluster_usage_cpu_demand", int64(cl.UsageCpuDemandMhz), labels)
	c.observeGauge("cluster_usage_mem_demand", int64(cl.UsageMemDemandMB), labels)
	c.observeGauge("cluster_usage_cpu_entitled", int64(cl.UsageCpuEntitledMhz), labels)
	c.observeGauge("cluster_usage_mem_entitled", int64(cl.UsageMemEntitledMB), labels)
	c.observeGauge("cluster_usage_cpu_reserved", int64(cl.UsageCpuReservationMhz), labels)
	c.observeGauge("cluster_usage_mem_reserved", int64(cl.UsageMemReservationMB), labels)
	c.observeGauge("cluster_vm_count_total", int64(cl.UsageTotalVmCount), labels)
	c.observeGauge("cluster_vm_count_powered_off", int64(cl.UsagePoweredOffVmCount), labels)

	for _, v := range overallStatuses {
		c.observeGauge("cluster_overall_status_"+v, oldmetrix.Bool(cl.OverallStatus == v), labels)
	}
}

func (c *Collector) writeClusterHAServiceState(prefix, state string, labels metrix.LabelSet) {
	known := false
	for _, v := range clusterHAServiceStates {
		ok := state == v.value
		c.observeGauge(prefix+"_"+snakeStatus(v.key), oldmetrix.Bool(ok), labels)
		known = known || ok
	}
	c.observeGauge(prefix+"_unknown", oldmetrix.Bool(!known), labels)
}

func (c *Collector) writeClusterHAVMMonitoringState(state string, labels metrix.LabelSet) {
	known := false
	for _, v := range clusterHAVMMonitoringStates {
		ok := state == v.value
		c.observeGauge("cluster_ha_vm_monitoring_"+snakeStatus(v.key), oldmetrix.Bool(ok), labels)
		known = known || ok
	}
	c.observeGauge("cluster_ha_vm_monitoring_unknown", oldmetrix.Bool(!known), labels)
}

func (c *Collector) writeClusterPerfMetrics(cl *rs.Cluster, metrics []performance.MetricSeries) {
	labels := c.clusterLabelSet(cl)
	for _, metric := range metrics {
		if len(metric.Value) == 0 || metric.Value[0] == -1 {
			continue
		}
		name := clusterPerfMetricByCounter[metric.Name]
		if name == "" {
			continue
		}
		c.observeGauge(name, metric.Value[0], labels)
	}
}

func (c *Collector) collectResourcePools() {
	if len(c.resources.ResourcePools) == 0 {
		return
	}

	refreshed := c.refreshResourcePoolProperties()

	c.collectResourcePoolsMetrics(refreshed)
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

func (c *Collector) collectResourcePoolsMetrics(refreshed map[string]bool) {
	// Property metrics reflect cached resource fields, so emit them only after
	// the current refresh succeeds.
	for _, rp := range c.resources.ResourcePools {
		if refreshed[rp.ID] {
			c.writeResourcePoolMetrics(rp)
		}
	}
}

func (c *Collector) writeResourcePoolMetrics(rp *rs.ResourcePool) {
	labels := c.resourcePoolLabelSet(rp)
	c.observeGauge("resource_pool_cpu_usage_usage", rp.OverallCpuUsage, labels)
	c.observeGauge("resource_pool_cpu_usage_demand", rp.OverallCpuDemand, labels)
	c.observeGauge("resource_pool_cpu_entitlement_distributed", rp.DistributedCpuEntitlement, labels)
	c.observeGauge("resource_pool_mem_usage_guest", rp.GuestMemoryUsage, labels)
	c.observeGauge("resource_pool_mem_usage_host", rp.HostMemoryUsage, labels)
	c.observeGauge("resource_pool_mem_entitlement_distributed", rp.DistributedMemoryEntitlement, labels)

	c.observeGauge("resource_pool_mem_breakdown_private", rp.PrivateMemory, labels)
	c.observeGauge("resource_pool_mem_breakdown_shared", rp.SharedMemory, labels)
	c.observeGauge("resource_pool_mem_breakdown_swapped", rp.SwappedMemory, labels)
	c.observeGauge("resource_pool_mem_breakdown_ballooned", rp.BalloonedMemory, labels)
	c.observeGauge("resource_pool_mem_breakdown_overhead", rp.OverheadMemory, labels)
	c.observeGauge("resource_pool_mem_breakdown_consumed_overhead", rp.ConsumedOverheadMemory, labels)
	// vSphere reports CompressedMemory in KiB; the chart keeps V1's MB display scale.
	c.observeGauge("resource_pool_mem_breakdown_compressed", rp.CompressedMemory, labels)

	c.observeGauge("resource_pool_cpu_allocation_reservation_used", rp.CpuReservationUsed, labels)
	c.observeGauge("resource_pool_cpu_allocation_max_usage", rp.CpuMaxUsage, labels)
	c.observeGauge("resource_pool_cpu_allocation_unreserved_for_vm", rp.CpuUnreservedForVm, labels)
	c.observeGauge("resource_pool_mem_allocation_reservation_used", rp.MemReservationUsed, labels)
	c.observeGauge("resource_pool_mem_allocation_max_usage", rp.MemMaxUsage, labels)
	c.observeGauge("resource_pool_mem_allocation_unreserved_for_vm", rp.MemUnreservedForVm, labels)

	c.observeGauge("resource_pool_cpu_config_reservation", rp.CpuReservation, labels)
	c.observeGauge("resource_pool_cpu_config_limit", rp.CpuLimit, labels)
	c.observeGauge("resource_pool_mem_config_reservation", rp.MemReservation, labels)
	c.observeGauge("resource_pool_mem_config_limit", rp.MemLimit, labels)

	for _, v := range overallStatuses {
		c.observeGauge("resource_pool_overall_status_"+v, oldmetrix.Bool(rp.OverallStatus == v), labels)
	}
}
