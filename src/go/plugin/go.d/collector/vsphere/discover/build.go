// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"fmt"
	"time"

	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
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
	res.Datastores = d.buildDatastores(raw.datastores)
	res.Networks = d.buildNetworks(raw.networks)
	res.StoragePods = d.buildStoragePods(raw.storagePods)
	res.ResourcePools = d.buildResourcePools(raw.resourcePools, res.Clusters)

	d.Infof("discovering : building : built %d/%d dcs, %d/%d folders, %d/%d clusters, %d/%d hosts, %d/%d vms, %d/%d datastores, %d/%d networks, %d/%d datastore clusters, %d/%d resource pools, process took %s",
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
		len(res.Datastores),
		len(raw.datastores),
		len(res.Networks),
		len(raw.networks),
		len(res.StoragePods),
		len(raw.storagePods),
		len(res.ResourcePools),
		len(raw.resourcePools),
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
	return findFolderRootID(parentID, folders)
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
		if f := newFolder(d); f != nil {
			fs.Put(f)
		}
	}
	return fs
}

func newFolder(raw mo.Folder) *rs.Folder {
	parentID := parentRefValue(raw.Parent)
	if parentID == "" {
		return nil
	}
	// vm group-v55 datacenter-54
	// host group-h56 datacenter-54
	// datastore group-s57 datacenter-54
	// network group-n58 datacenter-54
	return &rs.Folder{
		Name:     raw.Name,
		ID:       raw.Reference().Value,
		ParentID: parentID,
	}
}

func (Discoverer) buildClusters(raw []mo.ComputeResource) rs.Clusters {
	clusters := make(rs.Clusters)
	for _, c := range raw {
		if cluster := newCluster(c); cluster != nil {
			clusters.Put(cluster)
		}
	}
	return clusters
}

func newCluster(raw mo.ComputeResource) *rs.Cluster {
	parentID := parentRefValue(raw.Parent)
	if parentID == "" {
		return nil
	}
	// s - dummy cluster, c - created by user cluster
	// 192.168.0.201 domain-s61 group-h4
	// New Cluster1 domain-c52 group-h67
	cluster := &rs.Cluster{
		Name:         raw.Name,
		ID:           raw.Reference().Value,
		ParentID:     parentID,
		CustomValues: customFieldValues(raw.CustomValue),
		Ref:          raw.Reference(),
	}
	rs.SetClusterVSANInfo(cluster, raw.ConfigurationEx)
	return cluster
}

func (d Discoverer) buildHosts(raw []mo.HostSystem) rs.Hosts {
	hosts := make(rs.Hosts)
	for _, h := range raw {
		// connected | notResponding | disconnected
		//if v.Runtime.ConnectionState == "" {
		//
		//}
		if host := newHost(h); host != nil {
			hosts.Put(host)
		}
	}
	return hosts
}

func newHost(raw mo.HostSystem) *rs.Host {
	parentID := parentRefValue(raw.Parent)
	if parentID == "" {
		return nil
	}
	// 192.168.0.201 host-22 domain-s61
	// 192.168.0.202 host-28 domain-c52
	// 192.168.0.203 host-33 domain-c52
	host := &rs.Host{
		Name:              raw.Name,
		ID:                raw.Reference().Value,
		ParentID:          parentID,
		CustomValues:      customFieldValues(raw.CustomValue),
		ConnectionState:   string(raw.Runtime.ConnectionState),
		PowerState:        string(raw.Runtime.PowerState),
		InMaintenanceMode: raw.Runtime.InMaintenanceMode,
		OverallStatus:     string(raw.Summary.OverallStatus),
		Ref:               raw.Reference(),
	}
	if raw.Config != nil && raw.Config.VsanHostConfig != nil && raw.Config.VsanHostConfig.ClusterInfo != nil {
		host.VSANNodeUUID = raw.Config.VsanHostConfig.ClusterInfo.NodeUuid
	}
	return host
}

func (d Discoverer) buildVMs(raw []mo.VirtualMachine) rs.VMs {
	vms := make(rs.VMs)
	for _, v := range raw {
		// connected | disconnected | orphaned | inaccessible | invalid
		//if v.Runtime.ConnectionState == "" {
		//
		//}
		vms.Put(newVM(v))
	}
	return vms
}

func newVM(raw mo.VirtualMachine) *rs.VM {
	// deb91 vm-25 group-v3 host-22
	var hostID string
	if raw.Runtime.Host != nil {
		hostID = raw.Runtime.Host.Value
	}
	var folderID string
	if raw.Parent != nil {
		folderID = raw.Parent.Value
	}
	var toolsRunningStatus, toolsVersionStatus, guestHostName, guestIPAddress, guestFullName string
	if raw.Summary.Guest != nil {
		toolsRunningStatus = raw.Summary.Guest.ToolsRunningStatus
		toolsVersionStatus = raw.Summary.Guest.ToolsVersionStatus2
		guestHostName = raw.Summary.Guest.HostName
		guestIPAddress = raw.Summary.Guest.IpAddress
		guestFullName = raw.Summary.Guest.GuestFullName
	}
	var committed, uncommitted, unshared int64
	if raw.Summary.Storage != nil {
		committed = raw.Summary.Storage.Committed
		uncommitted = raw.Summary.Storage.Uncommitted
		unshared = raw.Summary.Storage.Unshared
	}
	var instanceUUID string
	if raw.Config != nil {
		instanceUUID = raw.Config.InstanceUuid
	}
	snapshot := summarizeSnapshotInfo(raw.Snapshot)
	return &rs.VM{
		Name:                     raw.Name,
		ID:                       raw.Reference().Value,
		ParentID:                 hostID,
		FolderParentID:           folderID,
		CustomValues:             customFieldValues(raw.CustomValue),
		ConnectionState:          string(raw.Runtime.ConnectionState),
		PowerState:               string(raw.Runtime.PowerState),
		ToolsRunningStatus:       toolsRunningStatus,
		ToolsVersionStatus:       toolsVersionStatus,
		GuestHostName:            guestHostName,
		GuestIPAddress:           guestIPAddress,
		GuestFullName:            guestFullName,
		InstanceUUID:             instanceUUID,
		ConsolidationNeeded:      raw.Runtime.ConsolidationNeeded,
		ConfigCPU:                int64(raw.Summary.Config.NumCpu),
		ConfigMemory:             int64(raw.Summary.Config.MemorySizeMB),
		ConfigDisks:              int64(raw.Summary.Config.NumVirtualDisks),
		ConfigNICs:               int64(raw.Summary.Config.NumEthernetCards),
		StorageCommitted:         committed,
		StorageUncommitted:       uncommitted,
		StorageUnshared:          unshared,
		OverallStatus:            string(raw.Summary.OverallStatus),
		SnapshotCount:            snapshot.count,
		SnapshotMaxChainDepth:    snapshot.maxChainDepth,
		SnapshotOldestCreateTime: snapshot.oldestCreateTime,
		Disks:                    newVMDisks(raw.Config),
		Ref:                      raw.Reference(),
	}
}

func newVMDisks(config *types.VirtualMachineConfigInfo) []rs.VMDisk {
	if config == nil {
		return nil
	}
	var disks []rs.VMDisk
	for _, device := range config.Hardware.Device {
		disk, ok := device.(*types.VirtualDisk)
		if !ok {
			continue
		}
		label := fmt.Sprintf("disk-%d", disk.Key)
		if disk.DeviceInfo != nil && disk.DeviceInfo.GetDescription() != nil && disk.DeviceInfo.GetDescription().Label != "" {
			label = disk.DeviceInfo.GetDescription().Label
		}
		capacity := disk.CapacityInBytes
		if capacity == 0 && disk.CapacityInKB > 0 {
			capacity = disk.CapacityInKB * 1024
		}
		disks = append(disks, rs.VMDisk{
			Key:           disk.Key,
			Label:         label,
			CapacityBytes: capacity,
		})
	}
	return disks
}

func (d Discoverer) buildDatastores(raw []mo.Datastore) rs.Datastores {
	datastores := make(rs.Datastores)
	for _, ds := range raw {
		if datastore := newDatastore(ds); datastore != nil {
			datastores.Put(datastore)
		}
	}
	return datastores
}

func newDatastore(raw mo.Datastore) *rs.Datastore {
	parentID := parentRefValue(raw.Parent)
	if parentID == "" {
		return nil
	}
	return &rs.Datastore{
		Name:               raw.Name,
		ID:                 raw.Reference().Value,
		ParentID:           parentID,
		CustomValues:       customFieldValues(raw.CustomValue),
		OverallStatus:      string(raw.OverallStatus),
		Type:               raw.Summary.Type,
		Capacity:           raw.Summary.Capacity,
		FreeSpace:          raw.Summary.FreeSpace,
		Uncommitted:        raw.Summary.Uncommitted,
		Accessible:         raw.Summary.Accessible,
		MaintenanceMode:    raw.Summary.MaintenanceMode,
		MultipleHostAccess: raw.Summary.MultipleHostAccess,
		Ref:                raw.Reference(),
	}
}

func (d Discoverer) buildNetworks(raw []mo.Network) rs.Networks {
	networks := make(rs.Networks)
	for _, network := range raw {
		networks.Put(newNetwork(network))
	}
	return networks
}

func newNetwork(raw mo.Network) *rs.Network {
	var accessible bool
	var ipPoolName string
	if raw.Summary != nil {
		if summary := raw.Summary.GetNetworkSummary(); summary != nil {
			accessible = summary.Accessible
			ipPoolName = summary.IpPoolName
		}
	}

	networkType := raw.Reference().Type
	if networkType == "" {
		networkType = "Network"
	}

	var parentID string
	if raw.Parent != nil {
		parentID = raw.Parent.Value
	}

	return &rs.Network{
		Name:          raw.Name,
		ID:            raw.Reference().Value,
		Type:          networkType,
		ParentID:      parentID,
		CustomValues:  customFieldValues(raw.CustomValue),
		Accessible:    accessible,
		IPPoolName:    ipPoolName,
		HostIDs:       refValues(raw.Host),
		VMIDs:         refValues(raw.Vm),
		OverallStatus: string(raw.OverallStatus),
		Ref:           raw.Reference(),
	}
}

func refValues(refs []types.ManagedObjectReference) []string {
	if len(refs) == 0 {
		return nil
	}
	values := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Value != "" {
			values = append(values, ref.Value)
		}
	}
	return values
}

func (d Discoverer) buildStoragePods(raw []mo.StoragePod) rs.StoragePods {
	pods := make(rs.StoragePods)
	for _, pod := range raw {
		if storagePod := newStoragePod(pod); storagePod != nil {
			pods.Put(storagePod)
		}
	}
	return pods
}

func newStoragePod(raw mo.StoragePod) *rs.StoragePod {
	parentID := parentRefValue(raw.Parent)
	if parentID == "" {
		return nil
	}
	var capacity, freeSpace int64
	if raw.Summary != nil {
		capacity = raw.Summary.Capacity
		freeSpace = raw.Summary.FreeSpace
	}
	var storageDRSEnabled bool
	if raw.PodStorageDrsEntry != nil {
		storageDRSEnabled = raw.PodStorageDrsEntry.StorageDrsConfig.PodConfig.Enabled
	}
	return &rs.StoragePod{
		Name:              raw.Name,
		ID:                raw.Reference().Value,
		ParentID:          parentID,
		CustomValues:      customFieldValues(raw.CustomValue),
		Capacity:          capacity,
		FreeSpace:         freeSpace,
		StorageDRSEnabled: storageDRSEnabled,
		OverallStatus:     string(raw.OverallStatus),
		Ref:               raw.Reference(),
	}
}

func (d Discoverer) buildResourcePools(raw []mo.ResourcePool, clusters rs.Clusters) rs.ResourcePools {
	pools := make(rs.ResourcePools)
	for _, rp := range raw {
		// owner is the cluster that owns this pool
		ownerID := rp.Owner.Value
		if ownerID == "" {
			continue
		}
		// skip pools whose owner is a dummy cluster (standalone host)
		if isDummyCluster(ownerID) {
			continue
		}
		// skip pools whose owner cluster wasn't discovered
		if clusters.Get(ownerID) == nil {
			continue
		}
		pools.Put(newResourcePool(rp))
	}
	return pools
}

func newResourcePool(raw mo.ResourcePool) *rs.ResourcePool {
	return &rs.ResourcePool{
		Name:         raw.Name,
		ID:           raw.Reference().Value,
		ParentID:     raw.Owner.Value, // owner cluster ref
		CustomValues: customFieldValues(raw.CustomValue),
		Ref:          raw.Reference(),
	}
}

func customFieldValues(values []types.BaseCustomFieldValue) map[int32]string {
	if len(values) == 0 {
		return nil
	}

	out := make(map[int32]string, len(values))
	for _, value := range values {
		stringValue, ok := value.(*types.CustomFieldStringValue)
		if !ok || stringValue == nil || stringValue.Value == "" {
			continue
		}
		out[stringValue.Key] = stringValue.Value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parentRefValue(ref *types.ManagedObjectReference) string {
	if ref == nil {
		return ""
	}
	return ref.Value
}
