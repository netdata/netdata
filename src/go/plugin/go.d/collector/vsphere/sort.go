// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"sort"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func sortedDatacenters(in rs.DataCenters) []*rs.Datacenter {
	return sortedValuesByID(in, func(v *rs.Datacenter) string { return v.ID })
}

func sortedClusters(in rs.Clusters) []*rs.Cluster {
	return sortedValuesByID(in, func(v *rs.Cluster) string { return v.ID })
}

func sortedHosts(in rs.Hosts) []*rs.Host {
	return sortedValuesByID(in, func(v *rs.Host) string { return v.ID })
}

func sortedVMs(in rs.VMs) []*rs.VM {
	return sortedValuesByID(in, func(v *rs.VM) string { return v.ID })
}

func sortedDatastores(in rs.Datastores) []*rs.Datastore {
	return sortedValuesByID(in, func(v *rs.Datastore) string { return v.ID })
}

func sortedNetworks(in rs.Networks) []*rs.Network {
	return sortedValuesByID(in, func(v *rs.Network) string { return v.ID })
}

func sortedStoragePods(in rs.StoragePods) []*rs.StoragePod {
	return sortedValuesByID(in, func(v *rs.StoragePod) string { return v.ID })
}

func sortedResourcePools(in rs.ResourcePools) []*rs.ResourcePool {
	return sortedValuesByID(in, func(v *rs.ResourcePool) string { return v.ID })
}

func sortedHostPowerPerfSamples(samples map[string]*hostPowerPerfSample) []*hostPowerPerfSample {
	return sortedValuesByID(samples, func(v *hostPowerPerfSample) string { return v.host.ID })
}

func sortedVMPowerPerfSamples(samples map[string]*vmPowerPerfSample) []*vmPowerPerfSample {
	return sortedValuesByID(samples, func(v *vmPowerPerfSample) string { return v.vm.ID })
}

func sortedValuesByID[M ~map[string]V, V any](in M, id func(V) string) []V {
	out := make([]V, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return id(out[i]) < id(out[j]) })
	return out
}

func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
