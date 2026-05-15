// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/match"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"

	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

type Client interface {
	Datacenters(pathSet ...string) ([]mo.Datacenter, error)
	Folders(pathSet ...string) ([]mo.Folder, error)
	ComputeResources(pathSet ...string) ([]mo.ComputeResource, error)
	Hosts(pathSet ...string) ([]mo.HostSystem, error)
	VirtualMachines(pathSet ...string) ([]mo.VirtualMachine, error)
	Datastores(pathSet ...string) ([]mo.Datastore, error)
	ResourcePools(pathSet ...string) ([]mo.ResourcePool, error)

	CounterInfoByName() (map[string]*types.PerfCounterInfo, error)
}

func New(client Client) *Discoverer {
	return &Discoverer{
		Client: client,
	}
}

type Discoverer struct {
	*logger.Logger
	Client
	match.HostMatcher
	match.VMMatcher
	match.DatastoreMatcher
	match.ClusterMatcher
}

type resources struct {
	dcs           []mo.Datacenter
	folders       []mo.Folder
	clusters      []mo.ComputeResource
	hosts         []mo.HostSystem
	vms           []mo.VirtualMachine
	datastores    []mo.Datastore
	resourcePools []mo.ResourcePool
}

func (d Discoverer) Discover() (*rs.Resources, error) {
	startTime := time.Now()
	raw, err := d.discover()
	if err != nil {
		return nil, fmt.Errorf("discovering resources : %v", err)
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
	removed := d.removeUnmatched(res)
	if removed == (numC + numH + numV + numD) {
		return nil, fmt.Errorf("all resources were filtered (%d clusters, %d hosts, %d vms, %d datastores)", numC, numH, numV, numD)
	}

	err = d.collectMetricLists(res)
	if err != nil {
		return nil, fmt.Errorf("collecting metric lists : %v", err)
	}

	d.Infof("discovering : discovered %d/%d clusters, %d/%d hosts, %d/%d vms, %d/%d datastores, %d resource pools, the whole process took %s",
		len(res.Clusters),
		numOfRealClusters(raw.clusters),
		len(res.Hosts),
		len(raw.hosts),
		len(res.VMs),
		len(raw.vms),
		len(res.Datastores),
		len(raw.datastores),
		len(res.ResourcePools),
		time.Since(startTime))

	return res, nil
}

var (
	// properties to set
	datacenterPathSet   = []string{"name", "parent"}
	folderPathSet       = []string{"name", "parent"}
	clusterPathSet      = []string{"name", "parent"}
	hostPathSet         = []string{"name", "parent", "runtime.powerState", "summary.overallStatus"}
	vmPathSet           = []string{"name", "runtime.host", "runtime.powerState", "summary.overallStatus"}
	datastorePathSet    = []string{"name", "parent", "summary", "overallStatus"}
	resourcePoolPathSet = []string{"name", "owner"}
)

func (d Discoverer) discover() (*resources, error) {
	d.Debug("discovering : starting resource discovering process")

	start := time.Now()
	t := start
	datacenters, err := d.Datacenters(datacenterPathSet...)
	if err != nil {
		return nil, err
	}
	d.Debugf("discovering : found %d dcs, process took %s", len(datacenters), time.Since(t))

	t = time.Now()
	folders, err := d.Folders(folderPathSet...)
	if err != nil {
		return nil, err
	}
	d.Debugf("discovering : found %d folders, process took %s", len(folders), time.Since(t))

	t = time.Now()
	clusters, err := d.ComputeResources(clusterPathSet...)
	if err != nil {
		return nil, err
	}
	d.Debugf("discovering : found %d clusters, process took %s", len(clusters), time.Since(t))

	t = time.Now()
	hosts, err := d.Hosts(hostPathSet...)
	if err != nil {
		return nil, err
	}
	d.Debugf("discovering : found %d hosts, process took %s", len(hosts), time.Since(t))

	t = time.Now()
	vms, err := d.VirtualMachines(vmPathSet...)
	if err != nil {
		return nil, err
	}
	d.Debugf("discovering : found %d vms, process took %s", len(vms), time.Since(t))

	t = time.Now()
	datastores, err := d.Datastores(datastorePathSet...)
	if err != nil {
		return nil, err
	}
	d.Debugf("discovering : found %d datastores, process took %s", len(datastores), time.Since(t))

	t = time.Now()
	resourcePools, err := d.ResourcePools(resourcePoolPathSet...)
	if err != nil {
		return nil, err
	}
	d.Debugf("discovering : found %d resource pools, process took %s", len(resourcePools), time.Since(t))

	raw := resources{
		dcs:           datacenters,
		folders:       folders,
		clusters:      clusters,
		hosts:         hosts,
		vms:           vms,
		datastores:    datastores,
		resourcePools: resourcePools,
	}

	d.Infof("discovering : found %d dcs, %d folders, %d clusters (%d dummy), %d hosts, %d vms, %d datastores, %d resource pools, process took %s",
		len(raw.dcs),
		len(raw.folders),
		len(clusters),
		numOfDummyClusters(clusters),
		len(raw.hosts),
		len(raw.vms),
		len(raw.datastores),
		len(raw.resourcePools),
		time.Since(start),
	)

	return &raw, nil
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
