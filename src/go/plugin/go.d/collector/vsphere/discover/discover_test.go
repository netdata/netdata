// SPDX-License-Identifier: GPL-3.0-or-later

package discover

import (
	"crypto/tls"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/performance"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/client"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"
)

func TestDiscoverer_Discover(t *testing.T) {
	d, _, teardown := prepareDiscovererSim(t)
	defer teardown()

	res, err := d.Discover()

	require.NoError(t, err)
	assert.True(t, len(res.DataCenters) > 0)
	assert.True(t, len(res.Folders) > 0)
	assert.True(t, len(res.Clusters) > 0)
	assert.True(t, len(res.Hosts) > 0)
	assert.True(t, len(res.VMs) > 0)
	assert.True(t, len(res.Datastores) > 0)
	assert.True(t, isHierarchySet(res))
	assert.True(t, isMetricListsCollected(res))
}

func TestDiscoverer_DiscoverNetworkTopologyOptIn(t *testing.T) {
	d, _, teardown := prepareDiscovererSim(t)
	defer teardown()
	d.CollectNetworkTopology = true

	res, err := d.Discover()

	require.NoError(t, err)
	require.NotEmpty(t, res.Networks)
	assert.True(t, isHierarchySet(res))
}

func TestDiscoverer_networkPathSetIncludesOverallStatus(t *testing.T) {
	assert.Contains(t, Discoverer{}.networkPathSet(), "overallStatus")
}

func TestDiscoverer_DiscoverFailSoftOptionalSurfaces(t *testing.T) {
	tests := map[string]struct {
		setup func(*Discoverer)
		check func(*testing.T, *rs.Resources)
	}{
		"network topology": {
			setup: func(d *Discoverer) {
				d.CollectNetworkTopology = true
				d.Client = networksErrorClient{Client: d.Client}
			},
			check: func(t *testing.T, res *rs.Resources) {
				require.Empty(t, res.Networks)
				assert.NotEmpty(t, res.Datastores)
			},
		},
		"datastore clusters": {
			setup: func(d *Discoverer) {
				d.CollectDatastoreClusters = true
				d.Client = storagePodsErrorClient{Client: d.Client}
			},
			check: func(t *testing.T, res *rs.Resources) {
				require.Empty(t, res.StoragePods)
				assert.NotEmpty(t, res.Datastores)
			},
		},
		"custom attributes": {
			setup: func(d *Discoverer) {
				d.Client = customFieldsErrorClient{Client: d.Client}
				d.CustomAttributeMatcher = matcher.TRUE()
			},
		},
		"tags": {
			setup: func(d *Discoverer) {
				d.Client = tagsByRefErrorClient{Client: d.Client}
				d.TagCategoryMatcher = matcher.TRUE()
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			d, _, teardown := prepareDiscovererSim(t)
			defer teardown()
			tc.setup(d)

			res, err := d.Discover()

			require.NoError(t, err)
			require.NotNil(t, res)
			assert.NotEmpty(t, res.Hosts)
			assert.NotEmpty(t, res.VMs)
			if tc.check != nil {
				tc.check(t, res)
			}
		})
	}
}

func TestDiscoverer_discover(t *testing.T) {
	d, model, teardown := prepareDiscovererSim(t)
	defer teardown()

	raw, err := d.discover()

	require.NoError(t, err)
	count := model.Count()
	assert.Lenf(t, raw.dcs, count.Datacenter, "datacenters")
	assert.Lenf(t, raw.folders, count.Folder-1, "folders") // minus root folder
	dummyClusters := model.Host * count.Datacenter
	assert.Lenf(t, raw.clusters, count.Cluster+dummyClusters, "clusters")
	assert.Lenf(t, raw.hosts, count.Host, "hosts")
	assert.Lenf(t, raw.vms, count.Machine, "vms")
	assert.Lenf(t, raw.datastores, count.Datastore, "datastores")
}

func TestDiscoverer_build(t *testing.T) {
	d, _, teardown := prepareDiscovererSim(t)
	defer teardown()

	raw, err := d.discover()
	require.NoError(t, err)

	res := d.build(raw)

	assert.Lenf(t, res.DataCenters, len(raw.dcs), "datacenters")
	assert.Lenf(t, res.Folders, len(raw.folders), "folders")
	assert.Lenf(t, res.Clusters, len(raw.clusters), "clusters")
	assert.Lenf(t, res.Hosts, len(raw.hosts), "hosts")
	assert.Lenf(t, res.VMs, len(raw.vms), "vms")
	assert.Lenf(t, res.Datastores, len(raw.datastores), "datastores")
}

func TestResourceBuildersSkipMissingParent(t *testing.T) {
	tests := map[string]struct {
		build func() any
	}{
		"folder":      {build: func() any { return newFolder(mo.Folder{}) }},
		"cluster":     {build: func() any { return newCluster(mo.ComputeResource{}) }},
		"host":        {build: func() any { return newHost(mo.HostSystem{}) }},
		"datastore":   {build: func() any { return newDatastore(mo.Datastore{}) }},
		"storage pod": {build: func() any { return newStoragePod(mo.StoragePod{}) }},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Nil(t, tc.build())
		})
	}
}

func TestDiscoverer_buildHostsKeepsNonPoweredHosts(t *testing.T) {
	raw := []mo.HostSystem{
		{
			ManagedEntity: mo.ManagedEntity{
				ExtensibleManagedObject: mo.ExtensibleManagedObject{
					Self: types.ManagedObjectReference{Type: "HostSystem", Value: "host-1"},
				},
				Parent: &types.ManagedObjectReference{Type: "ComputeResource", Value: "domain-c1"},
				Name:   "host1",
			},
			Runtime: types.HostRuntimeInfo{PowerState: types.HostSystemPowerStatePoweredOn},
		},
		{
			ManagedEntity: mo.ManagedEntity{
				ExtensibleManagedObject: mo.ExtensibleManagedObject{
					Self: types.ManagedObjectReference{Type: "HostSystem", Value: "host-2"},
				},
				Parent: &types.ManagedObjectReference{Type: "ComputeResource", Value: "domain-c1"},
				Name:   "host2",
			},
			Runtime: types.HostRuntimeInfo{
				ConnectionState:   types.HostSystemConnectionStateNotResponding,
				PowerState:        types.HostSystemPowerStatePoweredOff,
				InMaintenanceMode: true,
			},
		},
	}

	hosts := Discoverer{}.buildHosts(raw)
	assert.NotNil(t, hosts.Get("host-1"))
	assert.NotNil(t, hosts.Get("host-2"))
	host := hosts.Get("host-2")
	require.NotNil(t, host)
	assert.Equal(t, "poweredOff", host.PowerState)
	assert.Equal(t, "notResponding", host.ConnectionState)
	assert.True(t, host.InMaintenanceMode)
}

func TestDiscoverer_buildVMsKeepsNonPoweredVMsAndNilHost(t *testing.T) {
	raw := []mo.VirtualMachine{
		{
			ManagedEntity: mo.ManagedEntity{
				ExtensibleManagedObject: mo.ExtensibleManagedObject{
					Self: types.ManagedObjectReference{Type: "VirtualMachine", Value: "vm-1"},
				},
				Parent: &types.ManagedObjectReference{Type: "Folder", Value: "group-v1"},
				Name:   "vm1",
			},
			Runtime: types.VirtualMachineRuntimeInfo{PowerState: types.VirtualMachinePowerStatePoweredOn},
		},
		{
			ManagedEntity: mo.ManagedEntity{
				ExtensibleManagedObject: mo.ExtensibleManagedObject{
					Self: types.ManagedObjectReference{Type: "VirtualMachine", Value: "vm-2"},
				},
				Parent: &types.ManagedObjectReference{Type: "Folder", Value: "group-v1"},
				Name:   "vm2",
				CustomValue: []types.BaseCustomFieldValue{
					&types.CustomFieldStringValue{
						CustomFieldValue: types.CustomFieldValue{Key: 7},
						Value:            "owner-a",
					},
				},
			},
			Runtime: types.VirtualMachineRuntimeInfo{
				ConnectionState:     types.VirtualMachineConnectionStateInaccessible,
				PowerState:          types.VirtualMachinePowerStatePoweredOff,
				ConsolidationNeeded: true,
			},
			Summary: types.VirtualMachineSummary{
				Guest: &types.VirtualMachineGuestSummary{
					ToolsRunningStatus:  string(types.VirtualMachineToolsRunningStatusGuestToolsRunning),
					ToolsVersionStatus2: string(types.VirtualMachineToolsVersionStatusGuestToolsTooOld),
				},
				Config: types.VirtualMachineConfigSummary{
					NumCpu:           4,
					MemorySizeMB:     8192,
					NumVirtualDisks:  2,
					NumEthernetCards: 3,
				},
				Storage: &types.VirtualMachineStorageSummary{
					Committed:   100,
					Uncommitted: 200,
					Unshared:    300,
				},
			},
		},
	}
	hostRef := types.ManagedObjectReference{Type: "HostSystem", Value: "host-1"}
	raw[0].Runtime.Host = &hostRef

	vms := Discoverer{}.buildVMs(raw)
	assert.NotNil(t, vms.Get("vm-1"))
	vm := vms.Get("vm-2")
	require.NotNil(t, vm)
	assert.Empty(t, vm.ParentID)
	assert.Equal(t, "group-v1", vm.FolderParentID)
	assert.Equal(t, "poweredOff", vm.PowerState)
	assert.Equal(t, "inaccessible", vm.ConnectionState)
	assert.Equal(t, string(types.VirtualMachineToolsRunningStatusGuestToolsRunning), vm.ToolsRunningStatus)
	assert.Equal(t, string(types.VirtualMachineToolsVersionStatusGuestToolsTooOld), vm.ToolsVersionStatus)
	assert.True(t, vm.ConsolidationNeeded)
	assert.EqualValues(t, 4, vm.ConfigCPU)
	assert.EqualValues(t, 8192, vm.ConfigMemory)
	assert.EqualValues(t, 2, vm.ConfigDisks)
	assert.EqualValues(t, 3, vm.ConfigNICs)
	assert.EqualValues(t, 100, vm.StorageCommitted)
	assert.EqualValues(t, 200, vm.StorageUncommitted)
	assert.EqualValues(t, 300, vm.StorageUnshared)
	assert.Equal(t, map[int32]string{7: "owner-a"}, vm.CustomValues)
}

func TestDiscoverer_buildDatastoresKeepsInaccessible(t *testing.T) {
	yes := true
	raw := []mo.Datastore{
		{
			ManagedEntity: mo.ManagedEntity{
				ExtensibleManagedObject: mo.ExtensibleManagedObject{
					Self: types.ManagedObjectReference{Type: "Datastore", Value: "datastore-1"},
				},
				Parent:        &types.ManagedObjectReference{Type: "Folder", Value: "group-s1"},
				Name:          "Datastore1",
				OverallStatus: types.ManagedEntityStatusGray,
			},
			Summary: types.DatastoreSummary{
				Capacity:           1000,
				FreeSpace:          400,
				Uncommitted:        250,
				Accessible:         false,
				MultipleHostAccess: &yes,
				Type:               "VMFS",
				MaintenanceMode:    string(types.DatastoreSummaryMaintenanceModeStateInMaintenance),
			},
		},
	}

	datastores := Discoverer{}.buildDatastores(raw)

	ds := datastores.Get("datastore-1")
	require.NotNil(t, ds)
	assert.False(t, ds.Accessible)
	assert.EqualValues(t, 250, ds.Uncommitted)
	assert.Equal(t, string(types.DatastoreSummaryMaintenanceModeStateInMaintenance), ds.MaintenanceMode)
	require.NotNil(t, ds.MultipleHostAccess)
	assert.True(t, *ds.MultipleHostAccess)
}

func TestNewNetwork(t *testing.T) {
	network := newNetwork(mo.Network{
		ManagedEntity: mo.ManagedEntity{
			ExtensibleManagedObject: mo.ExtensibleManagedObject{
				Self: types.ManagedObjectReference{Type: "Network", Value: "network-1"},
			},
			Parent:        &types.ManagedObjectReference{Type: "Folder", Value: "group-n1"},
			OverallStatus: types.ManagedEntityStatusGreen,
		},
		Name: "VM Network",
		Summary: &types.NetworkSummary{
			Accessible: true,
			IpPoolName: "pool1",
		},
		Host: []types.ManagedObjectReference{{Type: "HostSystem", Value: "host-1"}},
		Vm:   []types.ManagedObjectReference{{Type: "VirtualMachine", Value: "vm-1"}},
	})

	require.NotNil(t, network)
	assert.Equal(t, "network-1", network.ID)
	assert.Equal(t, "VM Network", network.Name)
	assert.Equal(t, "Network", network.Type)
	assert.Equal(t, "group-n1", network.ParentID)
	assert.True(t, network.Accessible)
	assert.Equal(t, "pool1", network.IPPoolName)
	assert.Equal(t, "green", network.OverallStatus)
	assert.Equal(t, []string{"host-1"}, network.HostIDs)
	assert.Equal(t, []string{"vm-1"}, network.VMIDs)
}

func TestNewStoragePod(t *testing.T) {
	pod := newStoragePod(mo.StoragePod{
		Folder: mo.Folder{
			ManagedEntity: mo.ManagedEntity{
				ExtensibleManagedObject: mo.ExtensibleManagedObject{
					Self: types.ManagedObjectReference{Type: "StoragePod", Value: "group-p1"},
				},
				Parent:        &types.ManagedObjectReference{Type: "Folder", Value: "group-s1"},
				Name:          "DC0_POD0",
				OverallStatus: types.ManagedEntityStatusYellow,
			},
		},
		Summary: &types.StoragePodSummary{
			Capacity:  1000,
			FreeSpace: 400,
		},
		PodStorageDrsEntry: &types.PodStorageDrsEntry{
			StorageDrsConfig: types.StorageDrsConfigInfo{
				PodConfig: types.StorageDrsPodConfigInfo{Enabled: true},
			},
		},
	})

	require.NotNil(t, pod)
	assert.Equal(t, "group-p1", pod.ID)
	assert.Equal(t, "DC0_POD0", pod.Name)
	assert.Equal(t, "group-s1", pod.ParentID)
	assert.EqualValues(t, 1000, pod.Capacity)
	assert.EqualValues(t, 400, pod.FreeSpace)
	require.NotNil(t, pod.StorageDRSEnabled)
	assert.True(t, *pod.StorageDRSEnabled)
	assert.Equal(t, "yellow", pod.OverallStatus)
}

func TestDiscoverer_setHierarchy(t *testing.T) {
	d, _, teardown := prepareDiscovererSim(t)
	defer teardown()

	raw, err := d.discover()
	require.NoError(t, err)
	res := d.build(raw)

	err = d.setHierarchy(res)

	require.NoError(t, err)
	assert.True(t, isHierarchySet(res))
}

func TestDiscoverer_setVMHierarchyUsesFolderDatacenterWhenHostMissing(t *testing.T) {
	res := &rs.Resources{
		DataCenters: rs.DataCenters{
			"datacenter-1": &rs.Datacenter{ID: "datacenter-1", Name: "DC1"},
		},
		Folders: rs.Folders{
			"group-v1": &rs.Folder{ID: "group-v1", ParentID: "datacenter-1", Name: "vm"},
		},
		Hosts: rs.Hosts{},
		VMs: rs.VMs{
			"vm-1": &rs.VM{ID: "vm-1", FolderParentID: "group-v1"},
		},
	}

	assert.True(t, setVMHierarchy(res.VMs.Get("vm-1"), res))
	assert.Equal(t, "DC1", res.VMs.Get("vm-1").Hier.DC.Name)
	assert.Empty(t, res.VMs.Get("vm-1").Hier.Cluster.Name)
	assert.Empty(t, res.VMs.Get("vm-1").Hier.Host.Name)
}

func TestFindFolderRootIDStopsOnCycles(t *testing.T) {
	folders := rs.Folders{
		"group-v1": &rs.Folder{ID: "group-v1", ParentID: "group-v2", Name: "vm"},
		"group-v2": &rs.Folder{ID: "group-v2", ParentID: "group-v1", Name: "nested"},
	}

	assert.Empty(t, findVMDcID("group-v1", folders))
}

func TestDiscoverer_removeUnmatched(t *testing.T) {
	d, _, teardown := prepareDiscovererSim(t)
	defer teardown()

	d.HostMatcher = falseHostMatcher{}
	d.VMMatcher = falseVMMatcher{}
	raw, err := d.discover()
	require.NoError(t, err)
	res := d.build(raw)

	numClusters := len(res.Clusters)
	numVMs, numHosts := len(res.VMs), len(res.Hosts)
	removed := d.removeUnmatched(res)
	// dummy clusters are removed + all hosts/VMs rejected by matchers
	remainingClusters := len(res.Clusters)
	assert.Equal(t, (numClusters-remainingClusters)+numVMs+numHosts, removed)
	assert.Lenf(t, res.Hosts, 0, "hosts")
	assert.Lenf(t, res.VMs, 0, "vms")
}

func TestDiscoverer_removeUnmatchedStoragePods(t *testing.T) {
	pods := rs.StoragePods{
		"group-p1": &rs.StoragePod{ID: "group-p1"},
		"group-p2": &rs.StoragePod{ID: "group-p2"},
	}
	d := Discoverer{DatastoreClusterMatcher: falseStoragePodMatcher{}}

	removed := d.removeUnmatchedStoragePods(pods)

	require.Equal(t, 2, removed)
	require.Empty(t, pods)
}

func TestDiscoverer_collectMetricLists(t *testing.T) {
	d, _, teardown := prepareDiscovererSim(t)
	defer teardown()

	raw, err := d.discover()
	require.NoError(t, err)

	res := d.build(raw)
	err = d.collectMetricLists(res)

	require.NoError(t, err)
	assert.True(t, isMetricListsCollected(res))
}

func TestDiscoverer_warnMissingMetricCountersInitializesWarningMap(t *testing.T) {
	d := &Discoverer{Logger: logger.New()}

	d.warnMissingMetricCounters(map[string]*types.PerfCounterInfo{})

	require.NotNil(t, d.missingPerfCounterWarnings)
	require.NotEmpty(t, d.missingPerfCounterWarnings)
}

func TestSimpleMetricListIncludesPowerMetrics(t *testing.T) {
	tests := map[string]struct {
		counters map[string]*types.PerfCounterInfo
		build    func(map[string]*types.PerfCounterInfo) performance.MetricList
		wantLen  int
	}{
		"host": {
			counters: map[string]*types.PerfCounterInfo{
				"cpu.usage.average":                  {Key: 1},
				"power.power.average":                {Key: 2},
				"power.powerCap.average":             {Key: 3},
				"power.energy.summation":             {Key: 4},
				"power.capacity.usage.average":       {Key: 5},
				"power.capacity.usagePct.average":    {Key: 6},
				"power.capacity.usageIdle.average":   {Key: 7},
				"power.capacity.usageSystem.average": {Key: 8},
				"power.capacity.usageVm.average":     {Key: 9},
			},
			build:   simpleHostMetricList,
			wantLen: 9,
		},
		"VM": {
			counters: map[string]*types.PerfCounterInfo{
				"cpu.usage.average":      {Key: 1},
				"power.power.average":    {Key: 2},
				"power.energy.summation": {Key: 3},
			},
			build:   simpleVMMetricList,
			wantLen: 3,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ml := tc.build(tc.counters)

			require.Len(t, ml, tc.wantLen)
			for _, metric := range ml {
				assert.Empty(t, metric.Instance)
			}
		})
	}
}

func TestExpectedMetricCounterNamesSkipsOptionalCounters(t *testing.T) {
	names := expectedMetricCounterNames()

	assert.Contains(t, names, "cpu.usage.average")
	for name, counter := range map[string]string{
		"host power":        "power.power.average",
		"host energy":       "power.energy.summation",
		"cluster DRS score": "clusterServices.clusterDrsScore.latest",
		"VM DRS score":      "clusterServices.vmDrsScore.latest",
	} {
		t.Run(name, func(t *testing.T) {
			assert.NotContains(t, names, counter)
		})
	}
}

func prepareDiscovererSim(t *testing.T) (d *Discoverer, model *simulator.Model, teardown func()) {
	model, srv := createSim(t)
	teardown = func() { model.Remove(); srv.Close() }
	c := newClient(t, srv.URL)

	return New(c), model, teardown
}

func newClient(t *testing.T, vCenterURL *url.URL) *client.Client {
	c, err := client.New(client.Config{
		URL:       vCenterURL.String(),
		User:      "admin",
		Password:  "password",
		Timeout:   time.Second * 3,
		TLSConfig: tlscfg.TLSConfig{InsecureSkipVerify: true},
	})
	require.NoError(t, err)
	return c
}

func createSim(t *testing.T) (*simulator.Model, *simulator.Server) {
	model := simulator.VPX()
	err := model.Create()
	require.NoError(t, err)
	model.Service.TLS = new(tls.Config)
	return model, model.Service.NewServer()
}

func isHierarchySet(res *rs.Resources) bool {
	for _, c := range res.Clusters {
		if !c.Hier.IsSet() {
			return false
		}
	}
	for _, h := range res.Hosts {
		if !h.Hier.IsSet() {
			return false
		}
	}
	for _, v := range res.VMs {
		if !v.Hier.IsSet() {
			return false
		}
	}
	for _, ds := range res.Datastores {
		if !ds.Hier.IsSet() {
			return false
		}
	}
	for _, network := range res.Networks {
		if !network.Hier.IsSet() {
			return false
		}
	}
	for _, rp := range res.ResourcePools {
		if !rp.Hier.IsSet() {
			return false
		}
	}
	return true
}

func isMetricListsCollected(res *rs.Resources) bool {
	for _, h := range res.Hosts {
		if h.MetricList == nil {
			return false
		}
	}
	for _, v := range res.VMs {
		if v.MetricList == nil {
			return false
		}
	}
	// Datastore metric lists may be empty if perf counters are not available (e.g., vSAN)
	return true
}

type falseHostMatcher struct{}

func (falseHostMatcher) Match(*rs.Host) bool { return false }

type falseVMMatcher struct{}

func (falseVMMatcher) Match(*rs.VM) bool { return false }

type falseStoragePodMatcher struct{}

func (falseStoragePodMatcher) Match(*rs.StoragePod) bool { return false }

type storagePodsErrorClient struct {
	Client
}

func (storagePodsErrorClient) StoragePods(...string) ([]mo.StoragePod, error) {
	return nil, errors.New("storage pod permission denied")
}

type networksErrorClient struct {
	Client
}

func (networksErrorClient) Networks(...string) ([]mo.Network, error) {
	return nil, errors.New("network permission denied")
}

type customFieldsErrorClient struct {
	Client
}

func (customFieldsErrorClient) CustomFields() ([]types.CustomFieldDef, error) {
	return nil, errors.New("custom field permission denied")
}

type tagsByRefErrorClient struct {
	Client
}

func (tagsByRefErrorClient) TagsByRef([]types.ManagedObjectReference) (map[types.ManagedObjectReference]map[string][]string, error) {
	return nil, errors.New("tag permission denied")
}
