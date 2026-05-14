// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/match"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	logKeyNetworkTopologyDiscoveryError  = "vsphere-discover-network-topology-error"
	logKeyDatastoreClusterDiscoveryError = "vsphere-discover-datastore-cluster-error"
	logKeyCustomAttributeDiscoveryError  = "vsphere-discover-custom-attribute-error"
	logKeyTagDiscoveryError              = "vsphere-discover-tag-error"
)

type Client interface {
	Datacenters(pathSet ...string) ([]mo.Datacenter, error)
	Folders(pathSet ...string) ([]mo.Folder, error)
	ComputeResources(pathSet ...string) ([]mo.ComputeResource, error)
	Hosts(pathSet ...string) ([]mo.HostSystem, error)
	VirtualMachines(pathSet ...string) ([]mo.VirtualMachine, error)
	Datastores(pathSet ...string) ([]mo.Datastore, error)
	Networks(pathSet ...string) ([]mo.Network, error)
	StoragePods(pathSet ...string) ([]mo.StoragePod, error)
	ResourcePools(pathSet ...string) ([]mo.ResourcePool, error)

	CustomFields() ([]types.CustomFieldDef, error)
	TagsByRef(refs []types.ManagedObjectReference) (map[types.ManagedObjectReference]map[string][]string, error)
	CounterInfoByName() (map[string]*types.PerfCounterInfo, error)
}

func New(client Client) *Discoverer {
	return &Discoverer{
		Client:                     client,
		HostPowerStates:            []string{poweredOn},
		VMPowerStates:              []string{poweredOn},
		missingPerfCounterWarnings: make(map[string]bool),
	}
}

type Discoverer struct {
	*logger.Logger
	Client
	match.HostMatcher
	match.VMMatcher
	match.DatastoreMatcher
	match.ClusterMatcher
	HostPowerStates                      []string
	VMPowerStates                        []string
	CollectDatastoreClusters             bool
	CollectVMDisks                       bool
	CollectVMDiskPerformance             bool
	CollectVMNICPerformance              bool
	CollectHostNICPerformance            bool
	CollectHostDiskPerformance           bool
	CollectHostStorageAdapterPerformance bool
	CollectHostStoragePathPerformance    bool
	CollectHostCPUInstancePerformance    bool
	CollectPowerMetrics                  bool
	CollectVSAN                          bool
	CollectNetworkTopology               bool
	TagCategoryMatcher                   matcher.Matcher
	CustomAttributeMatcher               matcher.Matcher
	MaxUserMetadataLabels                int
	missingPerfCounterWarnings           map[string]bool
}

type resources struct {
	dcs           []mo.Datacenter
	folders       []mo.Folder
	clusters      []mo.ComputeResource
	hosts         []mo.HostSystem
	vms           []mo.VirtualMachine
	datastores    []mo.Datastore
	networks      []mo.Network
	storagePods   []mo.StoragePod
	resourcePools []mo.ResourcePool
}

func (d Discoverer) Discover() (*rs.Resources, error) {
	startTime := time.Now()
	raw, err := d.discover()
	if err != nil {
		return nil, fmt.Errorf("discover vSphere inventory resources: %w", err)
	}

	res := d.build(raw)

	err = d.setHierarchy(res)
	if err != nil {
		// TODO: handle objects w/o hier?
		d.Error(err)
	}

	numC := len(res.Clusters)
	numH := len(res.Hosts)
	numV := len(res.VMs)
	numD := len(res.Datastores)
	numSP := len(res.StoragePods)
	d.removeUnmatched(res)
	if len(res.Clusters)+len(res.Hosts)+len(res.VMs)+len(res.Datastores)+len(res.StoragePods) == 0 {
		return nil, fmt.Errorf("all resources were filtered (%d clusters, %d hosts, %d vms, %d datastores, %d datastore clusters)", numC, numH, numV, numD, numSP)
	}

	d.collectEnrichmentLabels(res)

	err = d.collectMetricLists(res)
	if err != nil {
		return nil, fmt.Errorf("collect vSphere performance metric lists: %w", err)
	}

	d.Infof("discovering : discovered %d/%d clusters, %d/%d hosts, %d/%d vms, %d/%d datastores, %d/%d datastore clusters, %d resource pools, the whole process took %s",
		len(res.Clusters),
		numOfRealClusters(raw.clusters),
		len(res.Hosts),
		len(raw.hosts),
		len(res.VMs),
		len(raw.vms),
		len(res.Datastores),
		len(raw.datastores),
		len(res.StoragePods),
		len(raw.storagePods),
		len(res.ResourcePools),
		time.Since(startTime))

	return res, nil
}

var (
	// properties to set
	datacenterPathSet       = []string{"name", "parent"}
	folderPathSet           = []string{"name", "parent"}
	clusterPathSetBase      = []string{"name", "parent"}
	hostPathSetBase         = []string{"name", "parent", "runtime.connectionState", "runtime.powerState", "runtime.inMaintenanceMode", "summary.overallStatus"}
	vmPathSetBase           = []string{"name", "parent", "runtime.host", "runtime.connectionState", "runtime.powerState", "runtime.consolidationNeeded", "summary.guest", "summary.config", "summary.storage", "summary.overallStatus", "snapshot"}
	datastorePathSetBase    = []string{"name", "parent", "summary", "overallStatus"}
	networkPathSetBase      = []string{"name", "parent", "summary", "host", "vm"}
	storagePodPathSetBase   = []string{"name", "parent", "summary", "overallStatus", "podStorageDrsEntry.storageDrsConfig.podConfig.enabled"}
	resourcePoolPathSetBase = []string{"name", "owner"}
)

func (d Discoverer) clusterPathSet() []string {
	pathSet := append([]string(nil), clusterPathSetBase...)
	if d.CollectVSAN {
		pathSet = append(pathSet, "configurationEx.vsanConfigInfo")
	}
	return d.withCustomValues(pathSet)
}

func (d Discoverer) hostPathSet() []string {
	pathSet := append([]string(nil), hostPathSetBase...)
	if d.CollectVSAN {
		pathSet = append(pathSet, "config.vsanHostConfig.clusterInfo.nodeUuid")
	}
	return d.withCustomValues(pathSet)
}

func (d Discoverer) vmPathSet() []string {
	pathSet := append([]string(nil), vmPathSetBase...)
	if d.CollectVMDisks || d.CollectVMDiskPerformance {
		pathSet = append(pathSet, "config.hardware.device")
	}
	if d.CollectVSAN {
		pathSet = append(pathSet, "config.instanceUuid")
	}
	return d.withCustomValues(pathSet)
}

func (d Discoverer) datastorePathSet() []string {
	return d.withCustomValues(datastorePathSetBase)
}

func (d Discoverer) networkPathSet() []string {
	return d.withCustomValues(networkPathSetBase)
}

func (d Discoverer) storagePodPathSet() []string {
	return d.withCustomValues(storagePodPathSetBase)
}

func (d Discoverer) resourcePoolPathSet() []string {
	return d.withCustomValues(resourcePoolPathSetBase)
}

func (d Discoverer) withCustomValues(pathSet []string) []string {
	out := append([]string(nil), pathSet...)
	if d.CustomAttributeMatcher != nil {
		out = append(out, "customValue")
	}
	return out
}

func (d Discoverer) discover() (*resources, error) {
	d.Debug("discovering : starting resource discovering process")

	start := time.Now()
	t := start
	datacenters, err := d.Datacenters(datacenterPathSet...)
	if err != nil {
		return nil, discoverRetrieveError("datacenters", datacenterPathSet, err)
	}
	d.Debugf("discovering : found %d dcs, process took %s", len(datacenters), time.Since(t))

	t = time.Now()
	folders, err := d.Folders(folderPathSet...)
	if err != nil {
		return nil, discoverRetrieveError("folders", folderPathSet, err)
	}
	d.Debugf("discovering : found %d folders, process took %s", len(folders), time.Since(t))

	t = time.Now()
	clusters, err := d.ComputeResources(d.clusterPathSet()...)
	if err != nil {
		return nil, discoverRetrieveError("compute resources", d.clusterPathSet(), err)
	}
	d.Debugf("discovering : found %d clusters, process took %s", len(clusters), time.Since(t))

	t = time.Now()
	hosts, err := d.Hosts(d.hostPathSet()...)
	if err != nil {
		return nil, discoverRetrieveError("hosts", d.hostPathSet(), err)
	}
	d.Debugf("discovering : found %d hosts, process took %s", len(hosts), time.Since(t))

	t = time.Now()
	vms, err := d.VirtualMachines(d.vmPathSet()...)
	if err != nil {
		return nil, discoverRetrieveError("virtual machines", d.vmPathSet(), err)
	}
	d.Debugf("discovering : found %d vms, process took %s", len(vms), time.Since(t))

	t = time.Now()
	datastores, err := d.Datastores(d.datastorePathSet()...)
	if err != nil {
		return nil, discoverRetrieveError("datastores", d.datastorePathSet(), err)
	}
	d.Debugf("discovering : found %d datastores, process took %s", len(datastores), time.Since(t))

	var networks []mo.Network
	if d.CollectNetworkTopology {
		t = time.Now()
		networks, err = d.Networks(d.networkPathSet()...)
		if err != nil {
			d.warnLimited(logKeyNetworkTopologyDiscoveryError, "discovering : failed to discover networks for topology: %v", discoverRetrieveError("networks", d.networkPathSet(), err))
		} else {
			d.Debugf("discovering : found %d networks, process took %s", len(networks), time.Since(t))
		}
	}

	var storagePods []mo.StoragePod
	if d.CollectDatastoreClusters {
		t = time.Now()
		storagePods, err = d.StoragePods(d.storagePodPathSet()...)
		if err != nil {
			d.warnLimited(logKeyDatastoreClusterDiscoveryError, "discovering : failed to discover datastore clusters: %v", discoverRetrieveError("datastore clusters", d.storagePodPathSet(), err))
		} else {
			d.Debugf("discovering : found %d datastore clusters, process took %s", len(storagePods), time.Since(t))
		}
	}

	t = time.Now()
	resourcePools, err := d.ResourcePools(d.resourcePoolPathSet()...)
	if err != nil {
		return nil, discoverRetrieveError("resource pools", d.resourcePoolPathSet(), err)
	}
	d.Debugf("discovering : found %d resource pools, process took %s", len(resourcePools), time.Since(t))

	raw := resources{
		dcs:           datacenters,
		folders:       folders,
		clusters:      clusters,
		hosts:         hosts,
		vms:           vms,
		datastores:    datastores,
		networks:      networks,
		storagePods:   storagePods,
		resourcePools: resourcePools,
	}

	d.Infof("discovering : found %d dcs, %d folders, %d clusters (%d dummy), %d hosts, %d vms, %d datastores, %d networks, %d datastore clusters, %d resource pools, process took %s",
		len(raw.dcs),
		len(raw.folders),
		len(clusters),
		numOfDummyClusters(clusters),
		len(raw.hosts),
		len(raw.vms),
		len(raw.datastores),
		len(raw.networks),
		len(raw.storagePods),
		len(raw.resourcePools),
		time.Since(start),
	)

	return &raw, nil
}

func (d Discoverer) warnLimited(key, format string, args ...any) {
	d.Limit(key, 1, time.Hour).Warningf(format, args...)
}

func discoverRetrieveError(resource string, pathSet []string, err error) error {
	return fmt.Errorf("retrieve %s from vSphere inventory pathSet=[%s]: %w", resource, strings.Join(pathSet, ","), err)
}

func numOfDummyClusters(clusters []mo.ComputeResource) (num int) {
	for _, c := range clusters {
		// domain-s61 | domain-c52
		if strings.HasPrefix(c.Reference().Value, "domain-s") {
			num++
		}
	}
	return num
}

func numOfRealClusters(clusters []mo.ComputeResource) int {
	return len(clusters) - numOfDummyClusters(clusters)
}

// isDummyCluster returns true for standalone-host placeholder clusters (domain-s*).
func isDummyCluster(id string) bool {
	return strings.HasPrefix(id, "domain-s")
}
