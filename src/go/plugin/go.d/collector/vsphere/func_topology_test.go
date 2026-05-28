// SPDX-License-Identifier: GPL-3.0-or-later

package vsphere

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
	rs "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/vsphere/resources"

	"github.com/stretchr/testify/require"
)

func TestFuncTopology_Handle(t *testing.T) {
	tests := map[string]struct {
		method    string
		collector func() *Collector
		want      int
		check     func(*testing.T, any)
	}{
		"without discovery": {
			method:    "topology:vsphere",
			collector: New,
			want:      503,
		},
		"with empty inventory cache": {
			method: "topology:vsphere",
			collector: func() *Collector {
				collr := New()
				collr.resources = &rs.Resources{}
				return collr
			},
			want: 200,
			check: func(t *testing.T, raw any) {
				data, ok := raw.(topology.Data)
				require.True(t, ok)
				require.Empty(t, data.Actors)
				require.Empty(t, data.Links)
				require.EqualValues(t, 0, data.Stats["hosts"])
				require.EqualValues(t, 0, data.Stats["vms"])
			},
		},
		"unknown method": {
			method:    "unknown",
			collector: New,
			want:      404,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			handler := &funcTopology{collector: tc.collector(), agentID: "vsphere_vcenter1"}

			resp := handler.Handle(context.Background(), tc.method, nil)

			require.Equal(t, tc.want, resp.Status)
			if tc.check != nil {
				tc.check(t, resp.Data)
			}
		})
	}
}

func TestFuncTopology_HandleWithInventoryCache(t *testing.T) {
	collr := New()
	collr.resources = &rs.Resources{
		DataCenters: rs.DataCenters{
			"datacenter-1": {ID: "datacenter-1", Name: "DC1"},
		},
		Clusters: rs.Clusters{
			"domain-c1": {
				ID:            "domain-c1",
				Name:          "Cluster1",
				Hier:          rs.ClusterHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}},
				OverallStatus: "green",
				DrsEnabled:    true,
				HaEnabled:     true,
			},
		},
		Hosts: rs.Hosts{
			"host-1": {
				ID:              "host-1",
				Name:            "Host1",
				Hier:            rs.HostHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c1", Name: "Cluster1"}},
				ConnectionState: "connected",
				PowerState:      "poweredOn",
				OverallStatus:   "green",
			},
		},
		VMs: rs.VMs{
			"vm-1": {
				ID:                    "vm-1",
				Name:                  "VM1",
				Hier:                  rs.VMHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c1", Name: "Cluster1"}, Host: rs.HierarchyValue{ID: "host-1", Name: "Host1"}},
				ConnectionState:       "connected",
				PowerState:            "poweredOn",
				OverallStatus:         "green",
				SnapshotCount:         2,
				SnapshotMaxChainDepth: 3,
			},
		},
		Datastores: rs.Datastores{
			"datastore-1": {
				ID:              "datastore-1",
				Name:            "Datastore1",
				Hier:            rs.DatastoreHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}},
				Type:            "VMFS",
				Accessible:      true,
				MaintenanceMode: "normal",
				Capacity:        1000,
				FreeSpace:       400,
			},
		},
		Networks: rs.Networks{
			"network-1": {
				ID:            "network-1",
				Name:          "VM Network",
				Type:          "Network",
				Hier:          rs.NetworkHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}},
				Accessible:    true,
				HostIDs:       []string{"host-1"},
				VMIDs:         []string{"vm-1"},
				OverallStatus: "green",
			},
		},
		StoragePods: rs.StoragePods{
			"group-p1": {
				ID:                "group-p1",
				Name:              "Pod1",
				Hier:              rs.StoragePodHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}},
				StorageDRSEnabled: new(true),
			},
		},
		ResourcePools: rs.ResourcePools{
			"resgroup-1": {
				ID:            "resgroup-1",
				Name:          "Resources",
				Hier:          rs.ResourcePoolHierarchy{DC: rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"}, Cluster: rs.HierarchyValue{ID: "domain-c1", Name: "Cluster1"}},
				OverallStatus: "green",
			},
		},
	}
	handler := &funcTopology{collector: collr, agentID: "vsphere_vcenter1"}

	resp := handler.Handle(context.Background(), "topology:vsphere", nil)

	require.Equal(t, 200, resp.Status)
	require.Equal(t, "topology", resp.ResponseType)
	data, ok := resp.Data.(topology.Data)
	require.True(t, ok)
	require.Equal(t, "vsphere", data.Source)
	require.Equal(t, "virtualization", data.Layer)
	require.Equal(t, "inventory", data.View)
	require.Equal(t, "vsphere_vcenter1", data.AgentID)
	require.Len(t, data.Actors, 8)
	require.Len(t, data.Links, 9)

	actors := topologyActorsByID(data.Actors)
	require.Contains(t, actors, "vsphere_datacenter:datacenter-1")
	require.Contains(t, actors, "vsphere_cluster:domain-c1")
	require.Contains(t, actors, "vsphere_host:host-1")
	require.Contains(t, actors, "vsphere_vm:vm-1")
	require.Contains(t, actors, "vsphere_network:network-1")
	require.Equal(t, "VM1", actors["vsphere_vm:vm-1"].Attributes["name"])
	require.EqualValues(t, 2, actors["vsphere_vm:vm-1"].Attributes["snapshot_count"])
	require.Equal(t, "VM Network", actors["vsphere_network:network-1"].Attributes["name"])

	require.Contains(t, topologyLinkKeys(data.Links), "vsphere_host:host-1->vsphere_vm:vm-1:runs")
	require.Contains(t, topologyLinkKeys(data.Links), "vsphere_host:host-1->vsphere_network:network-1:connects")
	require.Contains(t, topologyLinkKeys(data.Links), "vsphere_vm:vm-1->vsphere_network:network-1:connects")
}

func TestVSphereTopologyActorIDForResource(t *testing.T) {
	tests := map[string]struct {
		id   string
		want string
	}{
		"opaque network": {
			id:   "opaqueNetwork-1",
			want: "vsphere_network:opaqueNetwork-1",
		},
		"standard network": {
			id:   "network-1",
			want: "vsphere_network:network-1",
		},
		"distributed port group": {
			id:   "dvportgroup-1",
			want: "vsphere_network:dvportgroup-1",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, vsphereTopologyActorIDForResource(tc.id))
		})
	}
}

func TestFuncTopology_DoesNotLinkToFilteredActors(t *testing.T) {
	collr := New()
	collr.resources = &rs.Resources{
		DataCenters: rs.DataCenters{
			"datacenter-1": {ID: "datacenter-1", Name: "DC1"},
		},
		Clusters: rs.Clusters{},
		Hosts: rs.Hosts{
			"host-1": {
				ID:   "host-1",
				Name: "Host1",
				Hier: rs.HostHierarchy{
					DC:      rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"},
					Cluster: rs.HierarchyValue{ID: "domain-c-filtered", Name: "Filtered"},
				},
			},
		},
		VMs: rs.VMs{
			"vm-1": {
				ID:   "vm-1",
				Name: "VM1",
				Hier: rs.VMHierarchy{
					DC:      rs.HierarchyValue{ID: "datacenter-1", Name: "DC1"},
					Cluster: rs.HierarchyValue{ID: "domain-c-filtered", Name: "Filtered"},
					Host:    rs.HierarchyValue{ID: "host-filtered", Name: "FilteredHost"},
				},
			},
		},
	}

	data, ok := collr.topologyData("agent")

	require.True(t, ok)
	require.Len(t, data.Actors, 3)
	keys := topologyLinkKeys(data.Links)
	require.Contains(t, keys, "vsphere_datacenter:datacenter-1->vsphere_host:host-1:contains")
	require.Contains(t, keys, "vsphere_datacenter:datacenter-1->vsphere_vm:vm-1:contains")
	require.NotContains(t, keys, "vsphere_cluster:domain-c-filtered->vsphere_host:host-1:contains")
	require.NotContains(t, keys, "vsphere_host:host-filtered->vsphere_vm:vm-1:runs")
}

func topologyActorsByID(actors []topology.Actor) map[string]topology.Actor {
	out := make(map[string]topology.Actor, len(actors))
	for _, actor := range actors {
		out[actor.ActorID] = actor
	}
	return out
}

func topologyLinkKeys(links []topology.Link) map[string]struct{} {
	out := make(map[string]struct{}, len(links))
	for _, link := range links {
		out[link.SrcActorID+"->"+link.DstActorID+":"+link.LinkType] = struct{}{}
	}
	return out
}
