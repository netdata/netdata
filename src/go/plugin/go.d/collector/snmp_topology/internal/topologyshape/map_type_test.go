// SPDX-License-Identifier: GPL-3.0-or-later

package topologyshape

import (
	"sort"
	"testing"

	topologyengine "github.com/netdata/netdata/go/plugins/pkg/l2topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/stretchr/testify/require"
)

func TestManagedFabricMapPolicyKeepsQualifiedManagedFabric(t *testing.T) {
	data := &topologymodel.Data{
		Actors: []topologymodel.Actor{
			{ActorID: "managed-a", ActorType: "switch", Source: "snmp"},
			{ActorID: "managed-b", ActorType: "switch", Source: "snmp"},
			{ActorID: "managed-c", ActorType: "switch", Source: "snmp"},
			{ActorID: "managed-isolated", ActorType: "switch", Source: "snmp"},
			{
				ActorID:   "lldp-neighbor",
				ActorType: "switch",
				Source:    "snmp",
				Detail: topologymodel.ActorDetail{
					L2: topologyengine.ProjectionActorDetail{
						Device: topologyengine.ProjectionDeviceActorDetail{Inferred: true},
					},
				},
			},
			{ActorID: "qualified-segment", ActorType: "segment", SegmentKind: topologymodel.SegmentKindBroadcastDomain},
			{ActorID: "sparse-segment", ActorType: "segment", SegmentKind: topologymodel.SegmentKindBroadcastDomain},
			{ActorID: "endpoint", ActorType: "endpoint", Source: "snmp"},
		},
	}
	h := assignTopologyShapeTestHandles(t, data)
	data.Links = []topologymodel.Link{
		{Protocol: "lldp", SrcActorHandle: h["managed-a"], DstActorHandle: h["lldp-neighbor"]},
		{Protocol: "cdp", SrcActorHandle: h["managed-b"], DstActorHandle: h["managed-a"]},
		{Protocol: "stp", SrcActorHandle: h["managed-a"], DstActorHandle: h["managed-b"]},
		{Protocol: "stp", SrcActorHandle: h["managed-a"], DstActorHandle: h["lldp-neighbor"]},
		// Multiple bridge legs from one managed device count as one distinct neighbor.
		{Protocol: "bridge", SrcActorHandle: h["managed-a"], DstActorHandle: h["qualified-segment"]},
		{Protocol: "bridge", SrcActorHandle: h["managed-a"], DstActorHandle: h["qualified-segment"]},
		{Protocol: "fdb", SrcActorHandle: h["qualified-segment"], DstActorHandle: h["managed-b"]},
		{Protocol: "fdb", SrcActorHandle: h["qualified-segment"], DstActorHandle: h["managed-c"]},
		{Protocol: "fdb", SrcActorHandle: h["qualified-segment"], DstActorHandle: h["endpoint"]},
		{Protocol: "bridge", SrcActorHandle: h["managed-isolated"], DstActorHandle: h["sparse-segment"]},
		{Protocol: "fdb", SrcActorHandle: h["sparse-segment"], DstActorHandle: h["endpoint"]},
		{Protocol: "fdb", SrcActorHandle: h["managed-a"], DstActorHandle: h["managed-b"]},
		{Protocol: "fdb", SrcActorHandle: h["qualified-segment"], DstActorHandle: h["sparse-segment"]},
	}

	removed := applyMapTypePolicy(data, topologyoptions.MapTypeManagedFabric)
	require.Equal(t, 2, removed)
	require.Equal(t, []string{"lldp-neighbor", "managed-a", "managed-b", "managed-c", "managed-isolated", "qualified-segment"}, actorIDs(data.Actors))
	require.Equal(t, []string{"bridge", "bridge", "cdp", "fdb", "fdb", "lldp", "stp"}, linkProtocols(data.Links))
}

func actorIDs(actors []topologymodel.Actor) []string {
	out := make([]string, 0, len(actors))
	for _, actor := range actors {
		out = append(out, actor.ActorID)
	}
	sort.Strings(out)
	return out
}

func linkProtocols(links []topologymodel.Link) []string {
	out := make([]string, 0, len(links))
	for _, link := range links {
		out = append(out, link.Protocol)
	}
	sort.Strings(out)
	return out
}
