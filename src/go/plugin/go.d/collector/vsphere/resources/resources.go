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
	DataCenters   DataCenters
	Folders       Folders
	Clusters      Clusters
	Hosts         Hosts
	VMs           VMs
	Datastores    Datastores
	ResourcePools ResourcePools
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
		Name              string
		ID                string
		ParentID          string
		Hier              ClusterHierarchy
		OverallStatus     string
		NumHosts          int32
		NumEffectiveHosts int32
		TotalCpu          int32 // MHz
		TotalMemory       int64 // bytes
		EffectiveCpu      int32 // MHz
		EffectiveMemory   int64 // MB
		NumCpuCores       int16
		NumCpuThreads     int16
		NumVmotions       int32 // cumulative count
		DrsEnabled        bool
		DrsMode           string // fullyAutomated/partiallyAutomated/manual
		DrsScore          int32  // 0-100, vSphere 7.0+, 0 if unavailable
		CurrentBalance    int32  // thousandths of std dev
		TargetBalance     int32  // thousandths of std dev
		HaEnabled         bool
		HaAdmCtrlEnabled  bool
		// UsageSummary fields (nil when DRS disabled)
		UsageCpuDemandMhz      int32
		UsageMemDemandMB       int32
		UsageCpuEntitledMhz    int32
		UsageMemEntitledMB     int32
		UsageCpuReservationMhz int32
		UsageMemReservationMB  int32
		UsageTotalVmCount      int32
		UsagePoweredOffVmCount int32
		MetricList             performance.MetricList
		Ref                    types.ManagedObjectReference
	}

	ResourcePoolHierarchy struct {
		DC      HierarchyValue
		Cluster HierarchyValue
	}
	ResourcePool struct {
		Name     string
		ID       string
		ParentID string // owner cluster ref value
		Hier     ResourcePoolHierarchy
		// QuickStats (polled via PropertyCollector)
		OverallCpuUsage              int64 // MHz
		OverallCpuDemand             int64 // MHz
		GuestMemoryUsage             int64 // MB
		HostMemoryUsage              int64 // MB
		DistributedCpuEntitlement    int64 // MHz
		DistributedMemoryEntitlement int64 // MB
		PrivateMemory                int64 // MB
		SharedMemory                 int64 // MB
		SwappedMemory                int64 // MB
		BalloonedMemory              int64 // MB
		OverheadMemory               int64 // MB
		ConsumedOverheadMemory       int64 // MB
		CompressedMemory             int64 // KB
		// Runtime
		CpuReservationUsed int64 // MHz
		CpuMaxUsage        int64 // MHz
		CpuUnreservedForVm int64 // MHz
		MemReservationUsed int64 // bytes
		MemMaxUsage        int64 // bytes
		MemUnreservedForVm int64 // bytes
		// Config
		CpuReservation int64 // MHz, 0 if nil
		CpuLimit       int64 // MHz, -1 = unlimited
		MemReservation int64 // MB, 0 if nil
		MemLimit       int64 // MB, -1 = unlimited
		OverallStatus  string
		Ref            types.ManagedObjectReference
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

	DatastoreHierarchy struct {
		DC HierarchyValue
	}
	Datastore struct {
		Name          string
		ID            string
		ParentID      string
		Hier          DatastoreHierarchy
		OverallStatus string
		Type          string // VMFS, NFS, NFS41, vsan, VVOL, PMEM
		Capacity      int64  // bytes
		FreeSpace     int64  // bytes
		Accessible    bool
		MetricList    performance.MetricList
		Ref           types.ManagedObjectReference
	}
)

func (v *HierarchyValue) IsSet() bool         { return v.ID != "" && v.Name != "" }
func (v *HierarchyValue) Set(id, name string) { v.ID = id; v.Name = name }

func (h ClusterHierarchy) IsSet() bool      { return h.DC.IsSet() }
func (h HostHierarchy) IsSet() bool         { return h.DC.IsSet() && h.Cluster.IsSet() }
func (h VMHierarchy) IsSet() bool           { return h.DC.IsSet() && h.Cluster.IsSet() && h.Host.IsSet() }
func (h DatastoreHierarchy) IsSet() bool    { return h.DC.IsSet() }
func (h ResourcePoolHierarchy) IsSet() bool { return h.DC.IsSet() && h.Cluster.IsSet() }

type (
	DataCenters   map[string]*Datacenter
	Folders       map[string]*Folder
	Clusters      map[string]*Cluster
	Hosts         map[string]*Host
	VMs           map[string]*VM
	Datastores    map[string]*Datastore
	ResourcePools map[string]*ResourcePool
)

func (dcs DataCenters) Put(dc *Datacenter)           { dcs[dc.ID] = dc }
func (dcs DataCenters) Get(id string) *Datacenter    { return dcs[id] }
func (fs Folders) Put(folder *Folder)                { fs[folder.ID] = folder }
func (fs Folders) Get(id string) *Folder             { return fs[id] }
func (cs Clusters) Put(cluster *Cluster)             { cs[cluster.ID] = cluster }
func (cs Clusters) Remove(id string)                 { delete(cs, id) }
func (cs Clusters) Get(id string) *Cluster           { return cs[id] }
func (hs Hosts) Put(host *Host)                      { hs[host.ID] = host }
func (hs Hosts) Remove(id string)                    { delete(hs, id) }
func (hs Hosts) Get(id string) *Host                 { return hs[id] }
func (vs VMs) Put(vm *VM)                            { vs[vm.ID] = vm }
func (vs VMs) Remove(id string)                      { delete(vs, id) }
func (vs VMs) Get(id string) *VM                     { return vs[id] }
func (ds Datastores) Put(d *Datastore)               { ds[d.ID] = d }
func (ds Datastores) Remove(id string)               { delete(ds, id) }
func (ds Datastores) Get(id string) *Datastore       { return ds[id] }
func (rp ResourcePools) Put(p *ResourcePool)         { rp[p.ID] = p }
func (rp ResourcePools) Remove(id string)            { delete(rp, id) }
func (rp ResourcePools) Get(id string) *ResourcePool { return rp[id] }
