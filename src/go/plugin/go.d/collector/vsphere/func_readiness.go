// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

const (
	readinessMethodID   = "readiness"
	readinessMethodHelp = "Reports vSphere collector readiness, cached discovery state, and optional capability gates."
)

type readinessStatus string

const (
	readinessStatusOK       readinessStatus = "ok"
	readinessStatusWarning  readinessStatus = "warning"
	readinessStatusDisabled readinessStatus = "disabled"
	readinessStatusNotReady readinessStatus = "not_ready"
)

type readinessRow struct {
	check   string
	scope   string
	status  readinessStatus
	details string
}

type readinessColumn struct {
	funcapi.ColumnMeta
	value func(readinessRow) any
}

var readinessColumns = []readinessColumn{
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "check",
			Tooltip:       "Check",
			Type:          funcapi.FieldTypeString,
			Visible:       true,
			Sortable:      true,
			Sticky:        true,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterMultiselect,
			Transform:     funcapi.FieldTransformText,
			Visualization: funcapi.FieldVisualValue,
			UniqueKey:     true,
		},
		value: func(row readinessRow) any { return row.check },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "scope",
			Tooltip:       "Scope",
			Type:          funcapi.FieldTypeString,
			Visible:       true,
			Sortable:      true,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterMultiselect,
			Transform:     funcapi.FieldTransformText,
			Visualization: funcapi.FieldVisualValue,
		},
		value: func(row readinessRow) any { return row.scope },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "status",
			Tooltip:       "Status",
			Type:          funcapi.FieldTypeString,
			Visible:       true,
			Sortable:      true,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterMultiselect,
			Transform:     funcapi.FieldTransformText,
			Visualization: funcapi.FieldVisualPill,
		},
		value: func(row readinessRow) any { return string(row.status) },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "details",
			Tooltip:       "Details",
			Type:          funcapi.FieldTypeString,
			Visible:       true,
			Sortable:      false,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterNone,
			Transform:     funcapi.FieldTransformText,
			Visualization: funcapi.FieldVisualValue,
			FullWidth:     true,
			Wrap:          true,
		},
		value: func(row readinessRow) any { return row.details },
	},
}

type funcReadiness struct {
	collector *Collector
}

var _ funcapi.MethodHandler = (*funcReadiness)(nil)

func vsphereMethods() []funcapi.MethodConfig {
	return []funcapi.MethodConfig{{
		ID:           readinessMethodID,
		Name:         "vSphere Readiness",
		UpdateEvery:  30,
		Help:         readinessMethodHelp,
		RequireCloud: true,
	}, vsphereTopologyMethodConfig()}
}

func vsphereMethodHandler(job collectorapi.RuntimeJob) funcapi.MethodHandler {
	c, ok := job.Collector().(*Collector)
	if !ok {
		return nil
	}
	return &funcVSphere{
		readiness: &funcReadiness{collector: c},
		topology:  &funcTopology{collector: c, agentID: job.FullName()},
	}
}

type funcVSphere struct {
	readiness *funcReadiness
	topology  *funcTopology
}

var _ funcapi.MethodHandler = (*funcVSphere)(nil)

func (f *funcVSphere) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	switch method {
	case readinessMethodID:
		return f.readiness.MethodParams(ctx, method)
	case topologyMethodID:
		return f.topology.MethodParams(ctx, method)
	default:
		return nil, nil
	}
}

func (f *funcVSphere) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	switch method {
	case readinessMethodID:
		return f.readiness.Handle(ctx, method, params)
	case topologyMethodID:
		return f.topology.Handle(ctx, method, params)
	default:
		return funcapi.NotFoundResponse(method)
	}
}

func (f *funcVSphere) Cleanup(ctx context.Context) {
	f.readiness.Cleanup(ctx)
	f.topology.Cleanup(ctx)
}

func (f *funcReadiness) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (f *funcReadiness) Handle(_ context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != readinessMethodID {
		return funcapi.NotFoundResponse(method)
	}
	if f.collector == nil {
		return funcapi.UnavailableResponse("collector is not initialized")
	}

	rows := f.collector.readinessRows()
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].scope != rows[j].scope {
			return rows[i].scope < rows[j].scope
		}
		return rows[i].check < rows[j].check
	})

	cs := funcapi.Columns(readinessColumns, func(col readinessColumn) funcapi.ColumnMeta {
		return col.ColumnMeta
	})
	data := make([][]any, 0, len(rows))
	for _, row := range rows {
		values := make([]any, len(readinessColumns))
		for i, col := range readinessColumns {
			values[i] = col.value(row)
		}
		data = append(data, values)
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              readinessMethodHelp,
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: "status",
	}
}

func (f *funcReadiness) Cleanup(context.Context) {
	// No per-invocation resources are allocated by the readiness function.
}

func (c *Collector) readinessRows() []readinessRow {
	c.collectionLock.RLock()
	defer c.collectionLock.RUnlock()

	rows := []readinessRow{
		configReadinessRow("target_url", "target", c.URL != "", "target URL is configured", "target URL is not configured"),
		configReadinessRow("credentials", "target", c.Username != "" && c.Password != "", "credentials are configured", "username or password is not configured"),
		configReadinessRow("client", "target", c.discoverer != nil && c.scraper != nil, "vSphere client, discoverer, and scraper are initialized", "collector has not completed initialization"),
	}

	if c.resources == nil {
		rows = append(rows,
			readinessRow{
				check:   "inventory_cache",
				scope:   "discovery",
				status:  readinessStatusNotReady,
				details: "initial discovery has not completed successfully yet",
			},
			readinessRow{
				check:   "performance_counters",
				scope:   "discovery",
				status:  readinessStatusNotReady,
				details: "performance counter lists are unavailable until discovery succeeds",
			},
		)
	} else {
		rows = append(rows,
			readinessRow{
				check:   "inventory_cache",
				scope:   "discovery",
				status:  readinessStatusOK,
				details: resourceCounts(c.resources),
			},
			c.performanceCountersReadinessRow(),
		)
	}

	rows = append(rows,
		c.booleanReadinessRow("inventory_path_label", "labels", c.InventoryPathLabel, "inventory_path label collection is enabled", "inventory_path label collection is disabled"),
		c.userMetadataReadinessRow(),
		c.vmGuestLabelsReadinessRow(),
		c.optionalMetricReadinessRow("datastore_clusters", "metrics", c.CollectDatastoreClusters, c.MaxDatastoreClusters, c.DatastoreClustersInclude),
		c.optionalMetricReadinessRow("vm_disks", "metrics", c.CollectVMDisks || c.CollectVMDiskPerformance, c.MaxVMDisks, c.VMDisksInclude),
		c.optionalMetricReadinessRow("vm_nics", "metrics", c.CollectVMNICPerformance, c.MaxVMNICs, c.VMNICsInclude),
		c.optionalMetricReadinessRow("host_nics", "metrics", c.CollectHostNICPerformance, c.MaxHostNICs, c.HostNICsInclude),
		c.optionalMetricReadinessRow("host_disks", "metrics", c.CollectHostDiskPerformance, c.MaxHostDisks, c.HostDisksInclude),
		c.optionalMetricReadinessRow("host_storage_adapters", "metrics", c.CollectHostStorageAdapterPerformance, c.MaxHostStorageAdapters, c.HostStorageAdaptersInclude),
		c.optionalMetricReadinessRow("host_storage_paths", "metrics", c.CollectHostStoragePathPerformance, c.MaxHostStoragePaths, c.HostStoragePathsInclude),
		c.optionalMetricReadinessRow("host_cpu_instances", "metrics", c.CollectHostCPUInstancePerformance, c.MaxHostCPUInstances, c.HostCPUInstancesInclude),
		c.booleanReadinessRow("power_metrics", "metrics", c.CollectPowerMetrics, "host and VM power metrics are enabled", "host and VM power metrics are disabled"),
		c.booleanReadinessRow("network_topology", "topology", c.CollectNetworkTopology, "vSphere Network topology discovery is enabled", "vSphere Network topology discovery is disabled"),
		c.vsanReadinessRow(),
	)

	return rows
}

func configReadinessRow(check, scope string, ok bool, okDetails, notOKDetails string) readinessRow {
	if ok {
		return readinessRow{check: check, scope: scope, status: readinessStatusOK, details: okDetails}
	}
	return readinessRow{check: check, scope: scope, status: readinessStatusNotReady, details: notOKDetails}
}

func (c *Collector) booleanReadinessRow(check, scope string, enabled bool, enabledDetails, disabledDetails string) readinessRow {
	if enabled {
		return readinessRow{check: check, scope: scope, status: readinessStatusOK, details: enabledDetails}
	}
	return readinessRow{check: check, scope: scope, status: readinessStatusDisabled, details: disabledDetails}
}

func (c *Collector) optionalMetricReadinessRow(check, scope string, enabled bool, max int, includes []string) readinessRow {
	if !enabled {
		return readinessRow{
			check:   check,
			scope:   scope,
			status:  readinessStatusDisabled,
			details: "optional collection is disabled",
		}
	}
	return readinessRow{
		check:   check,
		scope:   scope,
		status:  readinessStatusOK,
		details: fmt.Sprintf("enabled with max=%d and include_patterns=%d", max, len(includes)),
	}
}

func (c *Collector) userMetadataReadinessRow() readinessRow {
	tags := len(c.VSphereTagCategories)
	attrs := len(c.CustomAttributes)
	if tags == 0 && attrs == 0 {
		return readinessRow{
			check:   "user_metadata_labels",
			scope:   "labels",
			status:  readinessStatusDisabled,
			details: "vSphere tag and custom-attribute labels are disabled",
		}
	}
	return readinessRow{
		check:   "user_metadata_labels",
		scope:   "labels",
		status:  readinessStatusOK,
		details: fmt.Sprintf("enabled for %d tag category pattern(s), %d custom attribute pattern(s), cap=%d labels per resource", tags, attrs, c.MaxUserMetadataLabels),
	}
}

func (c *Collector) vmGuestLabelsReadinessRow() readinessRow {
	if len(c.VMGuestLabels) == 0 {
		return readinessRow{
			check:   "vm_guest_labels",
			scope:   "labels",
			status:  readinessStatusDisabled,
			details: "VM guest labels are disabled",
		}
	}
	return readinessRow{
		check:   "vm_guest_labels",
		scope:   "labels",
		status:  readinessStatusOK,
		details: fmt.Sprintf("enabled labels: %s", strings.Join(c.VMGuestLabels, ", ")),
	}
}

func (c *Collector) vsanReadinessRow() readinessRow {
	if !c.CollectVSAN {
		return readinessRow{
			check:   "vsan",
			scope:   "metrics",
			status:  readinessStatusDisabled,
			details: "vSAN collection is disabled",
		}
	}
	if c.resources == nil {
		return readinessRow{
			check:   "vsan",
			scope:   "metrics",
			status:  readinessStatusNotReady,
			details: "vSAN collection is enabled, but discovery has not completed",
		}
	}

	clusters, hosts, vms := c.vsanResources()
	if len(clusters) == 0 {
		return readinessRow{
			check:   "vsan",
			scope:   "metrics",
			status:  readinessStatusWarning,
			details: "vSAN collection is enabled, but no vSAN-enabled clusters match the vSAN selectors",
		}
	}
	if c.vsanMetrics == nil {
		return readinessRow{
			check:   "vsan",
			scope:   "metrics",
			status:  readinessStatusNotReady,
			details: fmt.Sprintf("vSAN collection is enabled for %d cluster(s), %d host(s), and %d VM(s) after selectors/caps, but no vSAN scrape data is cached yet", len(clusters), len(hosts), len(vms)),
		}
	}

	return readinessRow{
		check:  "vsan",
		scope:  "metrics",
		status: readinessStatusOK,
		details: fmt.Sprintf(
			"vSAN data cached for %d cluster metric group(s), %d host metric group(s), %d VM metric group(s), %d space result(s), and %d health result(s)",
			len(c.vsanMetrics.Clusters),
			len(c.vsanMetrics.Hosts),
			len(c.vsanMetrics.VMs),
			len(c.vsanMetrics.Space),
			len(c.vsanMetrics.Health),
		),
	}
}

func (c *Collector) performanceCountersReadinessRow() readinessRow {
	var hosts, hostsWithMetrics, vms, vmsWithMetrics, datastores, datastoresWithMetrics, clusters, clustersWithMetrics int
	for _, host := range c.resources.Hosts {
		hosts++
		if len(host.MetricList) > 0 {
			hostsWithMetrics++
		}
	}
	for _, vm := range c.resources.VMs {
		vms++
		if len(vm.MetricList) > 0 {
			vmsWithMetrics++
		}
	}
	for _, datastore := range c.resources.Datastores {
		datastores++
		if len(datastore.MetricList) > 0 {
			datastoresWithMetrics++
		}
	}
	for _, cluster := range c.resources.Clusters {
		clusters++
		if len(cluster.MetricList) > 0 {
			clustersWithMetrics++
		}
	}

	status := readinessStatusOK
	if (hosts > 0 && hostsWithMetrics == 0) ||
		(vms > 0 && vmsWithMetrics == 0) ||
		(datastores > 0 && datastoresWithMetrics == 0) ||
		(clusters > 0 && clustersWithMetrics == 0) {
		status = readinessStatusWarning
	}

	return readinessRow{
		check:  "performance_counters",
		scope:  "discovery",
		status: status,
		details: fmt.Sprintf(
			"metric lists: %d/%d hosts, %d/%d VMs, %d/%d datastores, %d/%d clusters",
			hostsWithMetrics,
			hosts,
			vmsWithMetrics,
			vms,
			datastoresWithMetrics,
			datastores,
			clustersWithMetrics,
			clusters,
		),
	}
}

func resourceCounts(resources *rs.Resources) string {
	return fmt.Sprintf(
		"discovered %d datacenter(s), %d folder(s), %d cluster(s), %d host(s), %d VM(s), %d datastore(s), %d network(s), %d datastore cluster(s), and %d resource pool(s)",
		len(resources.DataCenters),
		len(resources.Folders),
		len(resources.Clusters),
		len(resources.Hosts),
		len(resources.VMs),
		len(resources.Datastores),
		len(resources.Networks),
		len(resources.StoragePods),
		len(resources.ResourcePools),
	)
}
