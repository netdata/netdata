// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"time"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

const maxHierarchyParentDepth = 64

func (d Discoverer) setHierarchy(res *rs.Resources) error {
	d.Debug("discovering : hierarchy : start setting resources hierarchy process")
	t := time.Now()

	c := d.setClustersHierarchy(res)
	h := d.setHostsHierarchy(res)
	v := d.setVMsHierarchy(res)
	ds := d.setDatastoresHierarchy(res)
	nw := d.setNetworksHierarchy(res)
	sp := d.setStoragePodsHierarchy(res)
	rp := d.setResourcePoolsHierarchy(res)

	d.Infof("discovering : hierarchy : set %d/%d clusters, %d/%d hosts, %d/%d vms, %d/%d datastores, %d/%d networks, %d/%d datastore clusters, %d/%d resource pools, process took %s",
		c, len(res.Clusters),
		h, len(res.Hosts),
		v, len(res.VMs),
		ds, len(res.Datastores),
		nw, len(res.Networks),
		sp, len(res.StoragePods),
		rp, len(res.ResourcePools),
		time.Since(t),
	)

	return nil
}

func (d Discoverer) setClustersHierarchy(res *rs.Resources) (set int) {
	for _, cluster := range res.Clusters {
		if setClusterHierarchy(cluster, res) {
			set++
		}
	}
	return set
}

func (d Discoverer) setHostsHierarchy(res *rs.Resources) (set int) {
	for _, host := range res.Hosts {
		if setHostHierarchy(host, res) {
			set++
		}
	}
	return set
}

func (d Discoverer) setVMsHierarchy(res *rs.Resources) (set int) {
	for _, vm := range res.VMs {
		if setVMHierarchy(vm, res) {
			set++
		}
	}
	return set
}

func setClusterHierarchy(cluster *rs.Cluster, res *rs.Resources) bool {
	dc := res.DataCenters.Get(cluster.ParentID)
	if dc == nil {
		return false
	}
	cluster.Hier.DC.Set(dc.ID, dc.Name)
	return cluster.Hier.IsSet()
}

func setHostHierarchy(host *rs.Host, res *rs.Resources) bool {
	cr := res.Clusters.Get(host.ParentID)
	if cr == nil {
		return false
	}
	host.Hier.Cluster.Set(cr.ID, cr.Name)

	dc := res.DataCenters.Get(cr.ParentID)
	if dc == nil {
		return false
	}
	host.Hier.DC.Set(dc.ID, dc.Name)
	return host.Hier.IsSet()
}

func setVMHierarchy(vm *rs.VM, res *rs.Resources) bool {
	if setVMHostHierarchy(vm, res) {
		return true
	}
	return setVMFolderHierarchy(vm, res)
}

func setVMHostHierarchy(vm *rs.VM, res *rs.Resources) bool {
	h := res.Hosts.Get(vm.ParentID)
	if h == nil {
		return false
	}
	vm.Hier.Host.Set(h.ID, h.Name)

	cr := res.Clusters.Get(h.ParentID)
	if cr == nil {
		return false
	}
	vm.Hier.Cluster.Set(cr.ID, cr.Name)

	dc := res.DataCenters.Get(cr.ParentID)
	if dc == nil {
		return false
	}
	vm.Hier.DC.Set(dc.ID, dc.Name)
	return vm.Hier.IsSet()
}

func setVMFolderHierarchy(vm *rs.VM, res *rs.Resources) bool {
	dcID := findVMDcID(vm.FolderParentID, res.Folders)
	dc := res.DataCenters.Get(dcID)
	if dc == nil {
		return false
	}
	vm.Hier.DC.Set(dc.ID, dc.Name)
	return vm.Hier.DC.IsSet()
}

func findVMDcID(parentID string, folders rs.Folders) string {
	return findFolderRootID(parentID, folders)
}

func (d Discoverer) setDatastoresHierarchy(res *rs.Resources) (set int) {
	for _, ds := range res.Datastores {
		if setDatastoreHierarchy(ds, res) {
			set++
		}
	}
	return set
}

// Datastore parent is a folder (group-s*) which resolves to a datacenter.
func setDatastoreHierarchy(ds *rs.Datastore, res *rs.Resources) bool {
	dcID := findDatastoreDcID(ds.ParentID, res.Folders)
	dc := res.DataCenters.Get(dcID)
	if dc == nil {
		return false
	}
	ds.Hier.DC.Set(dc.ID, dc.Name)
	return ds.Hier.IsSet()
}

func findDatastoreDcID(parentID string, folders rs.Folders) string {
	return findFolderRootID(parentID, folders)
}

func (d Discoverer) setNetworksHierarchy(res *rs.Resources) (set int) {
	for _, network := range res.Networks {
		if setNetworkHierarchy(network, res) {
			set++
		}
	}
	return set
}

// Network parent is normally a network folder (group-n*) which resolves to a datacenter.
func setNetworkHierarchy(network *rs.Network, res *rs.Resources) bool {
	dcID := findNetworkDcID(network.ParentID, res.Folders)
	dc := res.DataCenters.Get(dcID)
	if dc == nil {
		return false
	}
	network.Hier.DC.Set(dc.ID, dc.Name)
	return network.Hier.IsSet()
}

func findNetworkDcID(parentID string, folders rs.Folders) string {
	return findFolderRootID(parentID, folders)
}

func findFolderRootID(parentID string, folders rs.Folders) string {
	seen := make(map[string]struct{})
	for depth := 0; parentID != "" && depth < maxHierarchyParentDepth; depth++ {
		if _, ok := seen[parentID]; ok {
			return ""
		}
		seen[parentID] = struct{}{}

		f := folders.Get(parentID)
		if f == nil {
			return parentID
		}
		parentID = f.ParentID
	}
	return parentID
}

func (d Discoverer) setStoragePodsHierarchy(res *rs.Resources) (set int) {
	for _, pod := range res.StoragePods {
		if setStoragePodHierarchy(pod, res) {
			set++
		}
	}
	return set
}

// StoragePod parent is a folder (group-s*) which resolves to a datacenter.
func setStoragePodHierarchy(pod *rs.StoragePod, res *rs.Resources) bool {
	dcID := findDatastoreDcID(pod.ParentID, res.Folders)
	dc := res.DataCenters.Get(dcID)
	if dc == nil {
		return false
	}
	pod.Hier.DC.Set(dc.ID, dc.Name)
	return pod.Hier.IsSet()
}

func (d Discoverer) setResourcePoolsHierarchy(res *rs.Resources) (set int) {
	for _, rp := range res.ResourcePools {
		if setResourcePoolHierarchy(rp, res) {
			set++
		}
	}
	return set
}

// Resource pool owner is a cluster. Resolve DC from the cluster.
func setResourcePoolHierarchy(rp *rs.ResourcePool, res *rs.Resources) bool {
	cr := res.Clusters.Get(rp.ParentID)
	if cr == nil {
		return false
	}
	rp.Hier.Cluster.Set(cr.ID, cr.Name)

	dc := res.DataCenters.Get(cr.ParentID)
	if dc == nil {
		return false
	}
	rp.Hier.DC.Set(dc.ID, dc.Name)
	return rp.Hier.IsSet()
}
