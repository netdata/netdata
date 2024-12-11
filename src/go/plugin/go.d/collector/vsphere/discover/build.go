// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"time"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"

	"github.com/vmware/govmomi/vim25/mo"
)

func (d Discoverer) build(raw *resources) *rs.Resources {
	d.Debug("discovering : building : starting building resources process")
	t := time.Now()

	var res rs.Resources
	res.DataCenters = d.buildDatacenters(raw.dcs)
	res.Folders = d.buildFolders(raw.folders)
	res.Clusters = d.buildClusters(raw.clusters)
	fixClustersParentID(&res)
	res.Hosts = d.buildHosts(raw.hosts)
	res.VMs = d.buildVMs(raw.vms)

	d.Infof("discovering : building : built %d/%d dcs, %d/%d folders, %d/%d clusters, %d/%d hosts, %d/%d vms, process took %s",
		len(res.DataCenters),
		len(raw.dcs),
		len(res.Folders),
		len(raw.folders),
		len(res.Clusters),
		len(raw.clusters),
		len(res.Hosts),
		len(raw.hosts),
		len(res.VMs),
		len(raw.vms),
		time.Since(t),
	)
	return &res
}

// cluster parent is folder by default
// should be called after buildDatacenters, buildFolders and buildClusters
func fixClustersParentID(res *rs.Resources) {
	for _, c := range res.Clusters {
		c.ParentID = findClusterDcID(c.ParentID, res.Folders)
	}
}

func findClusterDcID(parentID string, folders rs.Folders) string {
	f := folders.Get(parentID)
	if f == nil {
		return parentID
	}
	return findClusterDcID(f.ParentID, folders)
}

func (Discoverer) buildDatacenters(raw []mo.Datacenter) rs.DataCenters {
	dcs := make(rs.DataCenters)
	for _, d := range raw {
		dcs.Put(newDC(d))
	}
	return dcs
}

func newDC(raw mo.Datacenter) *rs.Datacenter {
	// Datacenter1 datacenter-2 group-h4 group-v3
	return &rs.Datacenter{
		Name: raw.Name,
		ID:   raw.Reference().Value,
	}
}

func (Discoverer) buildFolders(raw []mo.Folder) rs.Folders {
	fs := make(rs.Folders)
	for _, d := range raw {
		fs.Put(newFolder(d))
	}
	return fs
}

func newFolder(raw mo.Folder) *rs.Folder {
	// vm group-v55 datacenter-54
	// host group-h56 datacenter-54
	// datastore group-s57 datacenter-54
	// network group-n58 datacenter-54
	return &rs.Folder{
		Name:     raw.Name,
		ID:       raw.Reference().Value,
		ParentID: raw.Parent.Value,
	}
}

func (Discoverer) buildClusters(raw []mo.ComputeResource) rs.Clusters {
	clusters := make(rs.Clusters)
	for _, c := range raw {
		clusters.Put(newCluster(c))
	}
	return clusters
}

func newCluster(raw mo.ComputeResource) *rs.Cluster {
	// s - dummy cluster, c - created by user cluster
	// 192.168.0.201 domain-s61 group-h4
	// New Cluster1 domain-c52 group-h67
	return &rs.Cluster{
		Name:     raw.Name,
		ID:       raw.Reference().Value,
		ParentID: raw.Parent.Value,
	}
}

const (
	poweredOn = "poweredOn"
)

func (d Discoverer) buildHosts(raw []mo.HostSystem) rs.Hosts {
	var num int
	hosts := make(rs.Hosts)
	for _, h := range raw {
		//	poweredOn | poweredOff | standBy | unknown
		if h.Runtime.PowerState != poweredOn {
			num++
			continue
		}
		// connected | notResponding | disconnected
		//if v.Runtime.ConnectionState == "" {
		//
		//}
		hosts.Put(newHost(h))
	}
	if num > 0 {
		d.Infof("discovering : building : removed %d hosts (not powered on)", num)
	}
	return hosts
}

func newHost(raw mo.HostSystem) *rs.Host {
	// 192.168.0.201 host-22 domain-s61
	// 192.168.0.202 host-28 domain-c52
	// 192.168.0.203 host-33 domain-c52
	return &rs.Host{
		Name:          raw.Name,
		ID:            raw.Reference().Value,
		ParentID:      raw.Parent.Value,
		OverallStatus: string(raw.Summary.OverallStatus),
		Ref:           raw.Reference(),
	}
}

func (d Discoverer) buildVMs(raw []mo.VirtualMachine) rs.VMs {
	var num int
	vms := make(rs.VMs)
	for _, v := range raw {
		//  poweredOff | poweredOn | suspended
		if v.Runtime.PowerState != poweredOn {
			num++
			continue
		}
		// connected | disconnected | orphaned | inaccessible | invalid
		//if v.Runtime.ConnectionState == "" {
		//
		//}
		vms.Put(newVM(v))
	}
	if num > 0 {
		d.Infof("discovering : building : removed %d vms (not powered on)", num)
	}
	return vms
}

func newVM(raw mo.VirtualMachine) *rs.VM {
	// deb91 vm-25 group-v3 host-22
	return &rs.VM{
		Name:          raw.Name,
		ID:            raw.Reference().Value,
		ParentID:      raw.Runtime.Host.Value,
		OverallStatus: string(raw.Summary.OverallStatus),
		Ref:           raw.Reference(),
	}
}
