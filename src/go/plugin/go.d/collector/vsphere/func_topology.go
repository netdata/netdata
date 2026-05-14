// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/topology"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

const (
	topologyMethodID   = "topology:vsphere"
	topologyMethodHelp = "Reports cached vSphere inventory topology for datacenters, clusters, hosts, VMs, datastores, networks, datastore clusters, and resource pools."

	vsphereTopologySchemaVersion = "2.0"
	vsphereTopologySource        = "vsphere"
	vsphereTopologyLayer         = "virtualization"
	vsphereTopologyView          = "inventory"
)

type funcTopology struct {
	collector *Collector
	agentID   string
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
	}.WithPresentation(vsphereTopologyPresentation())
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

	data, ok := f.collector.topologyData(f.agentID)
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

func (c *Collector) topologyData(agentID string) (topology.Data, bool) {
	c.collectionLock.RLock()
	defer c.collectionLock.RUnlock()

	if c.resources == nil {
		return topology.Data{}, false
	}

	actors := make([]topology.Actor, 0, topologyActorCount(c.resources))
	links := make([]topology.Link, 0, topologyLinkCount(c.resources))

	for _, dc := range sortedDatacenters(c.resources.DataCenters) {
		actors = append(actors, vsphereTopologyActor("vsphere_datacenter", dc.ID, dc.Name, nil, nil))
	}
	for _, cluster := range sortedClusters(c.resources.Clusters) {
		actors = append(actors, vsphereTopologyActor("vsphere_cluster", cluster.ID, cluster.Name, map[string]any{
			"overall_status": cluster.OverallStatus,
			"drs_enabled":    cluster.DrsEnabled,
			"ha_enabled":     cluster.HaEnabled,
			"vsan_enabled":   cluster.VSANEnabled,
		}, map[string]string{
			"datacenter": cluster.Hier.DC.Name,
		}))
		if cluster.Hier.DC.ID != "" {
			links = append(links, vsphereTopologyLink(cluster.Hier.DC.ID, cluster.ID, "contains", "Datacenter contains cluster"))
		}
	}
	for _, host := range sortedHosts(c.resources.Hosts) {
		actors = append(actors, vsphereTopologyActor("vsphere_host", host.ID, host.Name, map[string]any{
			"connection_state":    host.ConnectionState,
			"power_state":         host.PowerState,
			"in_maintenance_mode": host.InMaintenanceMode,
			"overall_status":      host.OverallStatus,
		}, map[string]string{
			"datacenter": host.Hier.DC.Name,
			"cluster":    host.Hier.Cluster.Name,
		}))
		if host.Hier.Cluster.ID != "" {
			links = append(links, vsphereTopologyLink(host.Hier.Cluster.ID, host.ID, "contains", "Cluster contains ESXi host"))
		}
	}
	for _, vm := range sortedVMs(c.resources.VMs) {
		actors = append(actors, vsphereTopologyActor("vsphere_vm", vm.ID, vm.Name, map[string]any{
			"connection_state":      vm.ConnectionState,
			"power_state":           vm.PowerState,
			"overall_status":        vm.OverallStatus,
			"tools_running_status":  vm.ToolsRunningStatus,
			"tools_version_status":  vm.ToolsVersionStatus,
			"consolidation_needed":  vm.ConsolidationNeeded,
			"snapshot_count":        vm.SnapshotCount,
			"snapshot_chain_depth":  vm.SnapshotMaxChainDepth,
			"configured_vcpus":      vm.ConfigCPU,
			"configured_memory_mib": vm.ConfigMemory,
		}, map[string]string{
			"datacenter": vm.Hier.DC.Name,
			"cluster":    vm.Hier.Cluster.Name,
			"host":       vm.Hier.Host.Name,
		}))
		switch {
		case vm.Hier.Host.ID != "":
			links = append(links, vsphereTopologyLink(vm.Hier.Host.ID, vm.ID, "runs", "ESXi host runs VM"))
		case vm.Hier.Cluster.ID != "":
			links = append(links, vsphereTopologyLink(vm.Hier.Cluster.ID, vm.ID, "contains", "Cluster contains VM"))
		case vm.Hier.DC.ID != "":
			links = append(links, vsphereTopologyLink(vm.Hier.DC.ID, vm.ID, "contains", "Datacenter contains VM"))
		}
	}
	for _, datastore := range sortedDatastores(c.resources.Datastores) {
		attrs := map[string]any{
			"type":                 datastore.Type,
			"overall_status":       datastore.OverallStatus,
			"accessible":           datastore.Accessible,
			"maintenance_mode":     datastore.MaintenanceMode,
			"capacity_bytes":       datastore.Capacity,
			"free_space_bytes":     datastore.FreeSpace,
			"uncommitted_bytes":    datastore.Uncommitted,
			"multiple_host_access": datastore.MultipleHostAccess,
		}
		actors = append(actors, vsphereTopologyActor("vsphere_datastore", datastore.ID, datastore.Name, attrs, map[string]string{
			"datacenter": datastore.Hier.DC.Name,
			"type":       datastore.Type,
		}))
		if datastore.Hier.DC.ID != "" {
			links = append(links, vsphereTopologyLink(datastore.Hier.DC.ID, datastore.ID, "contains", "Datacenter contains datastore"))
		}
	}
	for _, network := range sortedNetworks(c.resources.Networks) {
		actors = append(actors, vsphereTopologyActor("vsphere_network", network.ID, network.Name, map[string]any{
			"type":           network.Type,
			"accessible":     network.Accessible,
			"ip_pool_name":   network.IPPoolName,
			"overall_status": network.OverallStatus,
			"hosts":          len(network.HostIDs),
			"vms":            len(network.VMIDs),
		}, map[string]string{
			"datacenter": network.Hier.DC.Name,
			"type":       network.Type,
		}))
		if network.Hier.DC.ID != "" {
			links = append(links, vsphereTopologyLink(network.Hier.DC.ID, network.ID, "contains", "Datacenter contains network"))
		}
		for _, hostID := range network.HostIDs {
			if c.resources.Hosts.Get(hostID) != nil {
				links = append(links, vsphereTopologyLink(hostID, network.ID, "connects", "ESXi host connects to network"))
			}
		}
		for _, vmID := range network.VMIDs {
			if c.resources.VMs.Get(vmID) != nil {
				links = append(links, vsphereTopologyLink(vmID, network.ID, "connects", "VM connects to network"))
			}
		}
	}
	for _, pod := range sortedStoragePods(c.resources.StoragePods) {
		actors = append(actors, vsphereTopologyActor("vsphere_datastore_cluster", pod.ID, pod.Name, map[string]any{
			"capacity_bytes":      pod.Capacity,
			"free_space_bytes":    pod.FreeSpace,
			"storage_drs_enabled": pod.StorageDRSEnabled,
		}, map[string]string{
			"datacenter": pod.Hier.DC.Name,
		}))
		if pod.Hier.DC.ID != "" {
			links = append(links, vsphereTopologyLink(pod.Hier.DC.ID, pod.ID, "contains", "Datacenter contains datastore cluster"))
		}
	}
	for _, pool := range sortedResourcePools(c.resources.ResourcePools) {
		actors = append(actors, vsphereTopologyActor("vsphere_resource_pool", pool.ID, pool.Name, map[string]any{
			"overall_status": pool.OverallStatus,
			"cpu_limit_mhz":  pool.CpuLimit,
			"mem_limit_mb":   pool.MemLimit,
		}, map[string]string{
			"datacenter":    pool.Hier.DC.Name,
			"cluster":       pool.Hier.Cluster.Name,
			"resource_pool": pool.Name,
		}))
		if pool.Hier.Cluster.ID != "" {
			links = append(links, vsphereTopologyLink(pool.Hier.Cluster.ID, pool.ID, "contains", "Cluster contains resource pool"))
		}
	}

	sort.SliceStable(actors, func(i, j int) bool { return actors[i].ActorID < actors[j].ActorID })
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].SrcActorID != links[j].SrcActorID {
			return links[i].SrcActorID < links[j].SrcActorID
		}
		if links[i].DstActorID != links[j].DstActorID {
			return links[i].DstActorID < links[j].DstActorID
		}
		return links[i].LinkType < links[j].LinkType
	})

	return topology.Data{
		SchemaVersion: vsphereTopologySchemaVersion,
		Source:        vsphereTopologySource,
		Layer:         vsphereTopologyLayer,
		AgentID:       strings.TrimSpace(agentID),
		CollectedAt:   time.Now().UTC(),
		View:          vsphereTopologyView,
		Actors:        actors,
		Links:         links,
		Stats: map[string]any{
			"datacenters":        len(c.resources.DataCenters),
			"clusters":           len(c.resources.Clusters),
			"hosts":              len(c.resources.Hosts),
			"vms":                len(c.resources.VMs),
			"datastores":         len(c.resources.Datastores),
			"networks":           len(c.resources.Networks),
			"datastore_clusters": len(c.resources.StoragePods),
			"resource_pools":     len(c.resources.ResourcePools),
			"actors":             len(actors),
			"links":              len(links),
		},
	}, true
}

func vsphereTopologyActor(actorType, id, name string, attrs map[string]any, labels map[string]string) topology.Actor {
	attrs = cleanTopologyAnyMap(attrs)
	attrs["name"] = name
	attrs["vsphere_id"] = id

	return topology.Actor{
		ActorID:    vsphereTopologyActorID(actorType, id),
		ActorType:  actorType,
		Layer:      vsphereTopologyLayer,
		Source:     vsphereTopologySource,
		Match:      topology.Match{},
		Attributes: attrs,
		Labels:     cleanTopologyStringMap(labels),
	}
}

func vsphereTopologyLink(srcID, dstID, linkType, label string) topology.Link {
	srcActorID := vsphereTopologyActorIDForResource(srcID)
	dstActorID := vsphereTopologyActorIDForResource(dstID)
	return topology.Link{
		Layer:      vsphereTopologyLayer,
		Protocol:   vsphereTopologySource,
		LinkType:   linkType,
		Direction:  "parent_to_child",
		SrcActorID: srcActorID,
		DstActorID: dstActorID,
		Src:        vsphereTopologyEndpoint(srcActorID),
		Dst:        vsphereTopologyEndpoint(dstActorID),
		Metrics: map[string]any{
			"label": label,
		},
	}
}

func vsphereTopologyEndpoint(actorID string) topology.LinkEndpoint {
	return topology.LinkEndpoint{
		Match: topology.Match{},
		Attributes: map[string]any{
			"actor_id": actorID,
		},
	}
}

func vsphereTopologyActorID(actorType, id string) string {
	return actorType + ":" + id
}

func vsphereTopologyActorIDForResource(id string) string {
	switch {
	case strings.HasPrefix(id, "datacenter-"):
		return vsphereTopologyActorID("vsphere_datacenter", id)
	case strings.HasPrefix(id, "domain-"):
		return vsphereTopologyActorID("vsphere_cluster", id)
	case strings.HasPrefix(id, "host-"):
		return vsphereTopologyActorID("vsphere_host", id)
	case strings.HasPrefix(id, "vm-"):
		return vsphereTopologyActorID("vsphere_vm", id)
	case strings.HasPrefix(id, "datastore-"):
		return vsphereTopologyActorID("vsphere_datastore", id)
	case strings.HasPrefix(id, "network-"), strings.HasPrefix(id, "dvportgroup-"):
		return vsphereTopologyActorID("vsphere_network", id)
	case strings.HasPrefix(id, "group-p"):
		return vsphereTopologyActorID("vsphere_datastore_cluster", id)
	case strings.HasPrefix(id, "resgroup-"):
		return vsphereTopologyActorID("vsphere_resource_pool", id)
	default:
		return id
	}
}

func cleanTopologyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cleanTopologyAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+2)
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" || v == nil {
			continue
		}
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			out[k] = s
			continue
		}
		out[k] = v
	}
	return out
}

func topologyActorCount(resources *rs.Resources) int {
	return len(resources.DataCenters) + len(resources.Clusters) + len(resources.Hosts) + len(resources.VMs) +
		len(resources.Datastores) + len(resources.Networks) + len(resources.StoragePods) + len(resources.ResourcePools)
}

func topologyLinkCount(resources *rs.Resources) int {
	count := len(resources.Clusters) + len(resources.Hosts) + len(resources.VMs) +
		len(resources.Datastores) + len(resources.Networks) + len(resources.StoragePods) + len(resources.ResourcePools)
	for _, network := range resources.Networks {
		count += len(network.HostIDs) + len(network.VMIDs)
	}
	return count
}

func sortedDatacenters(in rs.DataCenters) []*rs.Datacenter {
	out := make([]*rs.Datacenter, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedClusters(in rs.Clusters) []*rs.Cluster {
	out := make([]*rs.Cluster, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedHosts(in rs.Hosts) []*rs.Host {
	out := make([]*rs.Host, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedDatastores(in rs.Datastores) []*rs.Datastore {
	out := make([]*rs.Datastore, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedNetworks(in rs.Networks) []*rs.Network {
	out := make([]*rs.Network, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func sortedResourcePools(in rs.ResourcePools) []*rs.ResourcePool {
	out := make([]*rs.ResourcePool, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
