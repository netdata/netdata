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
				Labels:       map[string]string{"type": "router"},
			},
		},
	}

	augmentTopologySnapshotLocals(&data, snapshots)
	augmentTopologySnapshotLocals(&data, snapshots)

	require.Len(t, data.Actors, 2)
	actor := data.Actors[1]
	require.Equal(t, "router-b-id", actor.ActorID)
	require.Equal(t, "router", actor.ActorType)
	require.Equal(t, "network", actor.Layer)
	require.Equal(t, "snmp", actor.Source)
	require.Equal(t, "router-b", actor.Match.SysName)
	require.Equal(t, "1.3.6.1.4.1.8072.3.2.1", actor.Match.SysObjectID)
	require.Equal(t, []string{"02:00:00:00:01:02"}, actor.Match.ChassisIDs)
	require.Equal(t, []string{"02:00:00:00:01:02"}, actor.Match.MacAddresses)
	require.Equal(t, []string{"192.0.2.2"}, actor.Match.IPAddresses)
	require.Equal(t, "192.0.2.2", actor.Detail.SNMP.ManagementIP)
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
