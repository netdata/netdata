// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	topologyv1 "github.com/netdata/netdata/go/plugins/pkg/topology/v1"
)

const (
	topologyMethodID   = "topology:vsphere"
	topologyMethodHelp = "Reports cached vSphere inventory topology for datacenters, clusters, hosts, VMs, datastores, networks, datastore clusters, and resource pools."

	vsphereTopologySource = "vsphere"
	vsphereTopologyLayer  = "virtualization"
	vsphereTopologyView   = "vsphere-inventory"

	vsphereTopologyDetailTable       = "vsphere_object_detail"
	vsphereTopologyLabelsTable       = "actor_labels"
	vsphereTopologyOwnershipLink     = "vsphere_ownership"
	vsphereTopologyRunsOnLink        = "vsphere_runs_on"
	vsphereTopologyNetworkLink       = "vsphere_connected_to"
	vsphereOwnershipEvidenceType     = "vsphere_ownership_evidence"
	vsphereRunsOnEvidenceType        = "vsphere_runs_on_evidence"
	vsphereNetworkConnectionEvidence = "vsphere_network_connection_evidence"

	vsphereDatastoreUtilizationOverlay = "datastore_space_utilization"
	vsphereOverlaySelectorCollectJob   = "collect_job"
	vsphereOverlaySelectorID           = "id"
)

type funcTopology struct {
	collector *Collector
	agentID   string
	jobName   string
}

var _ funcapi.MethodHandler = (*funcTopology)(nil)

func vsphereTopologyMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:           topologyMethodID,
		Aliases:      []string{topologyMethodID},
		Name:         "vSphere Topology",
		UpdateEvery:  30,
		Help:         topologyMethodHelp,
		RequireCloud: true,
		ResponseType: "topology",
	}
}

func (f *funcTopology) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (f *funcTopology) Handle(_ context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != topologyMethodID {
		return funcapi.NotFoundResponse(method)
	}
	if f.collector == nil {
		return funcapi.UnavailableResponse("collector is not initialized")
	}

	data, ok := f.collector.topologyData(f.agentID, f.jobName)
	if !ok {
		return funcapi.UnavailableResponse("topology data not available yet, please retry after discovery")
	}

	return &funcapi.FunctionResponse{
		Status:       200,
		Help:         topologyMethodHelp,
		ResponseType: "topology",
		Data:         data,
	}
}

func (f *funcTopology) Cleanup(context.Context) {
	// No per-invocation resources are allocated by the topology function.
}

func (c *Collector) topologyData(agentID, jobName string) (topologyv1.Data, bool) {
	c.collectionLock.RLock()
	defer c.collectionLock.RUnlock()

	if c.resources == nil {
		return topologyv1.Data{}, false
	}

	builder := newVSphereTopologyBuilder(jobName)

	for _, dc := range sortedDatacenters(c.resources.DataCenters) {
		builder.addActor("vsphere_datacenter", dc.ID, dc.Name, "", vsphereActorDetail{
			objectType: "datacenter",
		}, nil)
	}
	for _, cluster := range sortedClusters(c.resources.Clusters) {
		builder.addActor("vsphere_cluster", cluster.ID, cluster.Name, cluster.Hier.DC.ID, vsphereActorDetail{
			objectType:        "cluster",
			datacenter:        cluster.Hier.DC.Name,
			overallStatus:     cluster.OverallStatus,
			drsEnabled:        cluster.DrsEnabled,
			haEnabled:         cluster.HaEnabled,
			vsanEnabled:       cluster.VSANEnabled,
			cpuCapacityMHz:    int64(cluster.TotalCpu),
			memoryCapacityMiB: cluster.TotalMemory / 1024 / 1024,
		}, mergeStringMaps(cluster.Labels, map[string]string{
			"datacenter": cluster.Hier.DC.Name,
		}))
		if c.resources.DataCenters.Get(cluster.Hier.DC.ID) != nil {
			builder.addLink(cluster.Hier.DC.ID, cluster.ID, vsphereTopologyOwnershipLink, "contains")
		}
	}
	for _, host := range sortedHosts(c.resources.Hosts) {
		parentID := firstExistingResourceID(func(id string) bool {
			if id == host.Hier.Cluster.ID {
				return c.resources.Clusters.Get(id) != nil
			}
			return c.resources.DataCenters.Get(id) != nil
		}, host.Hier.Cluster.ID, host.Hier.DC.ID)
		builder.addActor("vsphere_host", host.ID, host.Name, parentID, vsphereActorDetail{
			objectType:        "host",
			datacenter:        host.Hier.DC.Name,
			cluster:           host.Hier.Cluster.Name,
			connectionState:   host.ConnectionState,
			powerState:        host.PowerState,
			inMaintenanceMode: host.InMaintenanceMode,
			overallStatus:     host.OverallStatus,
		}, mergeStringMaps(host.Labels, map[string]string{
			"datacenter": host.Hier.DC.Name,
			"cluster":    host.Hier.Cluster.Name,
		}))
		if c.resources.Clusters.Get(host.Hier.Cluster.ID) != nil {
			builder.addLink(host.Hier.Cluster.ID, host.ID, vsphereTopologyOwnershipLink, "contains")
		} else if c.resources.DataCenters.Get(host.Hier.DC.ID) != nil {
			builder.addLink(host.Hier.DC.ID, host.ID, vsphereTopologyOwnershipLink, "contains")
		}
	}
	for _, vm := range sortedVMs(c.resources.VMs) {
		parentID := firstExistingResourceID(func(id string) bool {
			switch id {
			case vm.Hier.Host.ID:
				return c.resources.Hosts.Get(id) != nil
			case vm.Hier.Cluster.ID:
				return c.resources.Clusters.Get(id) != nil
			default:
				return c.resources.DataCenters.Get(id) != nil
			}
		}, vm.Hier.Host.ID, vm.Hier.Cluster.ID, vm.Hier.DC.ID)
		builder.addActor("vsphere_vm", vm.ID, vm.Name, parentID, vsphereActorDetail{
			objectType:          "vm",
			datacenter:          vm.Hier.DC.Name,
			cluster:             vm.Hier.Cluster.Name,
			host:                vm.Hier.Host.Name,
			connectionState:     vm.ConnectionState,
			powerState:          vm.PowerState,
			overallStatus:       vm.OverallStatus,
			toolsRunningStatus:  vm.ToolsRunningStatus,
			toolsVersionStatus:  vm.ToolsVersionStatus,
			consolidationNeeded: vm.ConsolidationNeeded,
			snapshotCount:       vm.SnapshotCount,
			snapshotChainDepth:  vm.SnapshotMaxChainDepth,
			configuredVCPUs:     vm.ConfigCPU,
			configuredMemoryMiB: vm.ConfigMemory,
		}, mergeStringMaps(vm.Labels, map[string]string{
			"datacenter": vm.Hier.DC.Name,
			"cluster":    vm.Hier.Cluster.Name,
			"host":       vm.Hier.Host.Name,
		}))
		switch {
		case c.resources.Hosts.Get(vm.Hier.Host.ID) != nil:
			builder.addLink(vm.ID, vm.Hier.Host.ID, vsphereTopologyRunsOnLink, "runs_on")
		case c.resources.Clusters.Get(vm.Hier.Cluster.ID) != nil:
			builder.addLink(vm.Hier.Cluster.ID, vm.ID, vsphereTopologyOwnershipLink, "contains")
		case c.resources.DataCenters.Get(vm.Hier.DC.ID) != nil:
			builder.addLink(vm.Hier.DC.ID, vm.ID, vsphereTopologyOwnershipLink, "contains")
		}
	}
	for _, datastore := range sortedDatastores(c.resources.Datastores) {
		actor := builder.addActor("vsphere_datastore", datastore.ID, datastore.Name, datastore.Hier.DC.ID, vsphereActorDetail{
			objectType:         "datastore",
			datacenter:         datastore.Hier.DC.Name,
			overallStatus:      datastore.OverallStatus,
			datastoreType:      datastore.Type,
			accessible:         datastore.Accessible,
			maintenanceMode:    datastore.MaintenanceMode,
			capacityBytes:      datastore.Capacity,
			freeSpaceBytes:     datastore.FreeSpace,
			uncommittedBytes:   datastore.Uncommitted,
			multipleHostAccess: optionalBool(datastore.MultipleHostAccess),
		}, mergeStringMaps(datastore.Labels, map[string]string{
			"datacenter": datastore.Hier.DC.Name,
			"type":       datastore.Type,
		}))
		builder.addDatastoreUtilizationOverlay(actor, datastore.ID)
		if c.resources.DataCenters.Get(datastore.Hier.DC.ID) != nil {
			builder.addLink(datastore.Hier.DC.ID, datastore.ID, vsphereTopologyOwnershipLink, "contains")
		}
	}
	for _, network := range sortedNetworks(c.resources.Networks) {
		builder.addActor("vsphere_network", network.ID, network.Name, network.Hier.DC.ID, vsphereActorDetail{
			objectType:    "network",
			datacenter:    network.Hier.DC.Name,
			networkType:   network.Type,
			accessible:    network.Accessible,
			ipPoolName:    network.IPPoolName,
			overallStatus: network.OverallStatus,
			networkHosts:  len(network.HostIDs),
			networkVMs:    len(network.VMIDs),
		}, mergeStringMaps(network.Labels, map[string]string{
			"datacenter": network.Hier.DC.Name,
			"type":       network.Type,
		}))
		if c.resources.DataCenters.Get(network.Hier.DC.ID) != nil {
			builder.addLink(network.Hier.DC.ID, network.ID, vsphereTopologyOwnershipLink, "contains")
		}
		for _, hostID := range sortedStrings(network.HostIDs) {
			if c.resources.Hosts.Get(hostID) != nil {
				builder.addLink(hostID, network.ID, vsphereTopologyNetworkLink, "connected_to")
			}
		}
		for _, vmID := range sortedStrings(network.VMIDs) {
			if c.resources.VMs.Get(vmID) != nil {
				builder.addLink(vmID, network.ID, vsphereTopologyNetworkLink, "connected_to")
			}
		}
	}
	for _, pod := range sortedStoragePods(c.resources.StoragePods) {
		builder.addActor("vsphere_datastore_cluster", pod.ID, pod.Name, pod.Hier.DC.ID, vsphereActorDetail{
			objectType:        "datastore_cluster",
			datacenter:        pod.Hier.DC.Name,
			overallStatus:     pod.OverallStatus,
			capacityBytes:     pod.Capacity,
			freeSpaceBytes:    pod.FreeSpace,
			storageDRSEnabled: optionalBool(pod.StorageDRSEnabled),
		}, mergeStringMaps(pod.Labels, map[string]string{
			"datacenter": pod.Hier.DC.Name,
		}))
		if c.resources.DataCenters.Get(pod.Hier.DC.ID) != nil {
			builder.addLink(pod.Hier.DC.ID, pod.ID, vsphereTopologyOwnershipLink, "contains")
		}
	}
	for _, pool := range sortedResourcePools(c.resources.ResourcePools) {
		builder.addActor("vsphere_resource_pool", pool.ID, pool.Name, pool.Hier.Cluster.ID, vsphereActorDetail{
			objectType:     "resource_pool",
			datacenter:     pool.Hier.DC.Name,
			cluster:        pool.Hier.Cluster.Name,
			resourcePool:   pool.Name,
			overallStatus:  pool.OverallStatus,
			cpuLimitMHz:    pool.CpuLimit,
			memoryLimitMiB: pool.MemLimit,
			cpuReservation: pool.CpuReservation,
			memReservation: pool.MemReservation,
		}, mergeStringMaps(pool.Labels, map[string]string{
			"datacenter":    pool.Hier.DC.Name,
			"cluster":       pool.Hier.Cluster.Name,
			"resource_pool": pool.Name,
		}))
		if c.resources.Clusters.Get(pool.Hier.Cluster.ID) != nil {
			builder.addLink(pool.Hier.Cluster.ID, pool.ID, vsphereTopologyOwnershipLink, "contains")
		}
	}

	data, err := builder.data(strings.TrimSpace(agentID), time.Now().UTC(), map[string]any{
		"datacenters":        len(c.resources.DataCenters),
		"clusters":           len(c.resources.Clusters),
		"hosts":              len(c.resources.Hosts),
		"vms":                len(c.resources.VMs),
		"datastores":         len(c.resources.Datastores),
		"networks":           len(c.resources.Networks),
		"datastore_clusters": len(c.resources.StoragePods),
		"resource_pools":     len(c.resources.ResourcePools),
	})
	if err != nil {
		return topologyv1.Data{}, false
	}
	return data, true
}

func optionalBool(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}
