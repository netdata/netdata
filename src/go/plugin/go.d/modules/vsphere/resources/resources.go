// SPDX-License-Identifier: GPL-3.0-or-later

package resources

import (
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/vim25/types"
)

/*

```
Virtual Datacenter Architecture Representation (partial).

<root>
+-DC0 # Virtual datacenter
   +-datastore # Datastore folder (created by system)
   | +-Datastore1
   |
   +-host # Host folder (created by system)
   | +-Folder1 # Host and Cluster folder
   | | +-NestedFolder1
   | | | +-Cluster1
   | | | | +-Host1
   | +-Cluster2
   | | +-Host2
   | | | +-VM1
   | | | +-VM2
   | | | +-hadoop1
   | +-Host3 # Dummy folder for non-clustered host (created by system)
   | | +-Host3
   | | | +-VM3
   | | | +-VM4
   | | |
   +-vm # VM folder (created by system)
   | +-VM1
   | +-VM2
   | +-Folder2 # VM and Template folder
   | | +-hadoop1
   | | +-NestedFolder1
   | | | +-VM3
   | | | +-VM4
```
*/

type Resources struct {
	DataCenters DataCenters
	Folders     Folders
	Clusters    Clusters
	Hosts       Hosts
	VMs         VMs
}

type (
	Datacenter struct {
		Name string
		ID   string
	}

	Folder struct {
		Name     string
		ID       string
		ParentID string
	}

	HierarchyValue struct {
		ID, Name string
	}

	ClusterHierarchy struct {
		DC HierarchyValue
	}
	Cluster struct {
		Name     string
		ID       string
		ParentID string
		Hier     ClusterHierarchy
	}

	HostHierarchy struct {
		DC      HierarchyValue
		Cluster HierarchyValue
	}
	Host struct {
		Name          string
		ID            string
		ParentID      string
		Hier          HostHierarchy
		OverallStatus string
		MetricList    performance.MetricList
		Ref           types.ManagedObjectReference
	}

	VMHierarchy struct {
		DC      HierarchyValue
		Cluster HierarchyValue
		Host    HierarchyValue
	}

	VM struct {
		Name          string
		ID            string
		ParentID      string
		Hier          VMHierarchy
		OverallStatus string
		MetricList    performance.MetricList
		Ref           types.ManagedObjectReference
	}
)

func (v *HierarchyValue) IsSet() bool         { return v.ID != "" && v.Name != "" }
func (v *HierarchyValue) Set(id, name string) { v.ID = id; v.Name = name }

func (h ClusterHierarchy) IsSet() bool { return h.DC.IsSet() }
func (h HostHierarchy) IsSet() bool    { return h.DC.IsSet() && h.Cluster.IsSet() }
func (h VMHierarchy) IsSet() bool      { return h.DC.IsSet() && h.Cluster.IsSet() && h.Host.IsSet() }

type (
	DataCenters map[string]*Datacenter
	Folders     map[string]*Folder
	Clusters    map[string]*Cluster
	Hosts       map[string]*Host
	VMs         map[string]*VM
)

func (dcs DataCenters) Put(dc *Datacenter)        { dcs[dc.ID] = dc }
func (dcs DataCenters) Get(id string) *Datacenter { return dcs[id] }
func (fs Folders) Put(folder *Folder)             { fs[folder.ID] = folder }
func (fs Folders) Get(id string) *Folder          { return fs[id] }
func (cs Clusters) Put(cluster *Cluster)          { cs[cluster.ID] = cluster }
func (cs Clusters) Get(id string) *Cluster        { return cs[id] }
func (hs Hosts) Put(host *Host)                   { hs[host.ID] = host }
func (hs Hosts) Remove(id string)                 { delete(hs, id) }
func (hs Hosts) Get(id string) *Host              { return hs[id] }
func (vs VMs) Put(vm *VM)                         { vs[vm.ID] = vm }
func (vs VMs) Remove(id string)                   { delete(vs, id) }
func (vs VMs) Get(id string) *VM                  { return vs[id] }
