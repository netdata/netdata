// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/stretchr/testify/require"
)

func TestAugmentTopologySnapshotLocalsMaterializesMissingPolledManagedActor(t *testing.T) {
	data := topologymodel.Data{
		Actors: []topologymodel.Actor{
			{
				ActorID:   "existing",
				ActorType: "switch",
				Match:     topologymodel.Match{SysName: "existing-switch"},
			},
		},
	}
	snapshots := []topologymodel.ObservationSnapshot{
		{
			LocalDeviceID: "router-b-id",
			LocalDevice: topologymodel.Device{
				SysName:      "router-b",
				SysObjectID:  "1.3.6.1.4.1.8072.3.2.1",
				ChassisID:    "02:00:00:00:01:02",
				ManagementIP: "192.0.2.2",
				Labels: map[string]string{
					" type ": " router ",
					"empty":  "   ",
				},
			},
		},
	}

	snapshots = append(snapshots, snapshots[0])
	augmentTopologySnapshotLocals(&data, snapshots)

	require.Len(t, data.Actors, 2)
	actor := data.Actors[1]
	require.Equal(t, "router-b-id", actor.ActorID)
	require.Equal(t, "router", actor.ActorType)
	require.Equal(t, "network", actor.Layer)
	require.Equal(t, "snmp", actor.Source)
	require.Equal(t, map[string]string{"type": "router"}, actor.Labels)
	require.Equal(t, "router-b", actor.Match.SysName)
	require.Equal(t, "1.3.6.1.4.1.8072.3.2.1", actor.Match.SysObjectID)
	require.Equal(t, []string{"02:00:00:00:01:02"}, actor.Match.ChassisIDs)
	require.Equal(t, []string{"02:00:00:00:01:02"}, actor.Match.MacAddresses)
	require.Equal(t, []string{"192.0.2.2"}, actor.Match.IPAddresses)
	require.Equal(t, "192.0.2.2", actor.Detail.SNMP.ManagementIP)
}

func TestTopologyLocalActorFromCacheUsesOnlySelectedManagementIPForIdentity(t *testing.T) {
	local := topologymodel.Device{
		SysName:      "router-b",
		ManagementIP: "192.0.2.2",
		ManagementAddresses: []topologymodel.ManagementAddress{
			{Address: "192.0.2.3", AddressType: "ipv4", Source: "ip_mib"},
			{Address: "c0000263", AddressType: "16", Source: "lldp_local"},
		},
	}

	actor, ok := topologyLocalActorFromCache("router-b-id", local)
	require.True(t, ok)
	require.Equal(t, []string{"192.0.2.2"}, actor.Match.IPAddresses)
	require.Equal(t, local.ManagementAddresses, actor.Detail.SNMP.ManagementAddresses)
}

func TestAugmentTopologySnapshotLocalsPreservesActorOrderAndEligibility(t *testing.T) {
	data := topologymodel.Data{Actors: []topologymodel.Actor{
		{
			ActorID:   "segment-a",
			ActorType: topologymodel.L3SubnetSegmentActorType,
			Match:     topologymodel.Match{IPAddresses: []string{"192.0.2.1"}},
		},
		{
			ActorID:   "router-a",
			ActorType: "router",
			Match:     topologymodel.Match{IPAddresses: []string{"192.0.2.1"}},
		},
		{
			ActorID:   "router-b",
			ActorType: "router",
			Match:     topologymodel.Match{ChassisIDs: []string{"chassis-b"}},
		},
	}}
	local := topologymodel.Device{
		ChassisID:    "chassis-b",
		ManagementIP: "192.0.2.1",
		SysDescr:     "selected first eligible actor",
	}

	augmentTopologySnapshotLocals(&data, []topologymodel.ObservationSnapshot{{LocalDevice: local}})

	require.Empty(t, data.Actors[0].Detail.SNMP.SysDescr)
	require.Equal(t, local.SysDescr, data.Actors[1].Detail.SNMP.SysDescr)
	require.Empty(t, data.Actors[2].Detail.SNMP.SysDescr)
}

func TestAugmentTopologySnapshotLocalsKeepsFallbackActorIDExistenceGlobal(t *testing.T) {
	data := topologymodel.Data{Actors: []topologymodel.Actor{{
		ActorID:   "existing-id",
		ActorType: topologymodel.L3SubnetSegmentActorType,
	}}}
	local := topologymodel.Device{
		ManagementIP: "192.0.2.2",
		ManagementAddresses: []topologymodel.ManagementAddress{
			{Address: "192.0.2.1", AddressType: "ipv4", Source: "ip_mib"},
		},
	}

	augmentTopologySnapshotLocals(&data, []topologymodel.ObservationSnapshot{{
		LocalDeviceID: "existing-id",
		LocalDevice:   local,
	}})

	require.Len(t, data.Actors, 1)
}

func TestAugmentTopologySnapshotLocalsDoesNotMatchDiagnosticManagementAddress(t *testing.T) {
	data := topologymodel.Data{Actors: []topologymodel.Actor{{
		ActorID:   "router-a",
		ActorType: "router",
		Match:     topologymodel.Match{IPAddresses: []string{"192.0.2.1"}},
	}}}
	local := topologymodel.Device{
		ManagementIP: "192.0.2.2",
		ManagementAddresses: []topologymodel.ManagementAddress{
			{Address: "192.0.2.1", AddressType: "ipv4", Source: "ip_mib"},
		},
	}

	augmentTopologySnapshotLocals(&data, []topologymodel.ObservationSnapshot{{
		LocalDeviceID: "router-b",
		LocalDevice:   local,
	}})

	require.Len(t, data.Actors, 2)
	require.Equal(t, "router-b", data.Actors[1].ActorID)
}

func TestEnrichLocalActorChartReferencesAddsTypedPortDetails(t *testing.T) {
	actor := &topologymodel.Actor{
		Detail: topologymodel.ActorDetail{
			L2: topologyengine.ProjectionActorDetail{
				Device: topologyengine.ProjectionDeviceActorDetail{
					Ports: []topologyengine.ProjectionPortDetail{
						{Name: "Gi0/1"},
						{IfName: "Gi0/2"},
						{Name: "Gi0/3"},
					},
				},
			},
		},
	}

	enrichLocalActorChartReferences(actor, map[string]topologymodel.InterfaceChartRef{
		"Gi0/1": {
			ChartIDSuffix:    "gi0_1",
			AvailableMetrics: []string{"errors", "traffic", "traffic"},
		},
		"gi0/2": {
			AvailableMetrics: []string{"drops"},
		},
	})

	tests := map[string]struct {
		port        topologyengine.ProjectionPortDetail
		wantSuffix  string
		wantMetrics []string
	}{
		"name-match":    {port: actor.Detail.L2.Device.Ports[0], wantSuffix: "gi0_1", wantMetrics: []string{"errors", "traffic"}},
		"if-name-match": {port: actor.Detail.L2.Device.Ports[1], wantSuffix: "gi0/2", wantMetrics: []string{"drops"}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.wantSuffix, tc.port.ChartIDSuffix)
			require.Equal(t, tc.wantMetrics, tc.port.AvailableMetrics)
		})
	}

	require.Empty(t, actor.Detail.L2.Device.Ports[2].ChartIDSuffix)
	require.Empty(t, actor.Detail.L2.Device.Ports[2].AvailableMetrics)
}
