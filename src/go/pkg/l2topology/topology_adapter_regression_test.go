// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/stretchr/testify/require"
)

func TestBackfillPairGroupMissingEndpointPortsCopiesPeerInterfaceAttributes(t *testing.T) {
	entries := []*builtAdjacencyLink{
		{
			adj: Adjacency{
				SourceID: "device-a",
				TargetID: "device-b",
			},
			link: graph.Link{
				Src: graph.LinkEndpoint{},
				Dst: graph.LinkEndpoint{},
			},
		},
		{
			adj: Adjacency{
				SourceID: "device-b",
				TargetID: "device-a",
			},
			link: graph.Link{
				Src: graph.LinkEndpoint{
					IfIndex: 2,
					IfName:  "Gi0/2",
					PortID:  "Gi0/2",
				},
				Dst: graph.LinkEndpoint{
					IfIndex: 1,
					IfName:  "Gi0/1",
					PortID:  "Gi0/1",
				},
			},
		},
	}

	backfillPairGroupMissingEndpointPorts(entries)

	require.Equal(t, 1, entries[0].link.Src.IfIndex)
	require.Equal(t, "Gi0/1", entries[0].link.Src.IfName)
	require.Equal(t, "Gi0/1", entries[0].link.Src.PortID)
	require.Equal(t, 2, entries[0].link.Dst.IfIndex)
	require.Equal(t, "Gi0/2", entries[0].link.Dst.IfName)
	require.Equal(t, "Gi0/2", entries[0].link.Dst.PortID)
}

func TestBackfillPairGroupMissingEndpointPortsSkipsAmbiguousReverseCandidates(t *testing.T) {
	entries := []*builtAdjacencyLink{
		{
			adj: Adjacency{
				SourceID: "device-a",
				TargetID: "device-b",
			},
			link: graph.Link{
				Src: graph.LinkEndpoint{},
				Dst: graph.LinkEndpoint{},
			},
		},
		{
			adj: Adjacency{
				SourceID: "device-b",
				TargetID: "device-a",
			},
			link: graph.Link{
				Src: graph.LinkEndpoint{IfName: "Gi0/2"},
				Dst: graph.LinkEndpoint{IfName: "Gi0/1"},
			},
		},
		{
			adj: Adjacency{
				SourceID: "device-b",
				TargetID: "device-a",
			},
			link: graph.Link{
				Src: graph.LinkEndpoint{IfName: "Gi0/22"},
				Dst: graph.LinkEndpoint{IfName: "Gi0/11"},
			},
		},
	}

	backfillPairGroupMissingEndpointPorts(entries)

	require.Empty(t, entries[0].link.Src.IfName)
	require.Empty(t, entries[0].link.Dst.IfName)
}

func TestBackfillEndpointPortFromPeerPreservesExistingCanonicalPort(t *testing.T) {
	endpoint := graph.LinkEndpoint{
		IfName: "Gi0/10",
	}
	peer := graph.LinkEndpoint{
		IfIndex: 7,
		IfName:  "Gi0/7",
		PortID:  "Gi0/7",
	}

	backfilled := backfillEndpointPortFromPeer(endpoint, peer)

	require.Equal(t, "Gi0/10", backfilled.IfName)
	require.Zero(t, backfilled.IfIndex)
	require.Empty(t, backfilled.PortID)
}

func TestSegmentProjectionBuilderPruneSegmentsWithoutLinksRemovesEmptySegments(t *testing.T) {
	builder := &segmentProjectionBuilder{
		segmentIDs: []string{"segment-a", "segment-b"},
		out: projectedSegments{
			actors: []projectedActor{
				{
					Actor: graph.Actor{
						ActorID:   "segment-a",
						ActorType: "segment",
					},
					Detail: ProjectionActorDetail{
						Segment: ProjectionSegmentActorDetail{SegmentID: "segment-a"},
					},
				},
				{
					Actor: graph.Actor{
						ActorID:   "segment-b",
						ActorType: "segment",
					},
					Detail: ProjectionActorDetail{
						Segment: ProjectionSegmentActorDetail{SegmentID: "segment-b"},
					},
				},
			},
			links: []graph.Link{
				{
					Protocol:  "fdb",
					Direction: "bidirectional",
					L2:        &graph.LinkL2{BridgeDomain: "segment-a"},
				},
				{
					Protocol:  "fdb",
					Direction: "bidirectional",
					L2:        &graph.LinkL2{BridgeDomain: "segment-b"},
				},
			},
		},
	}

	builder.pruneSegmentsWithoutLinks(map[string]struct{}{
		"segment-a": {},
	})

	require.Len(t, builder.out.actors, 1)
	require.Equal(t, "segment-a", builder.out.actors[0].Actor.ActorID)
	require.Len(t, builder.out.links, 1)
	require.Equal(t, "segment-a", topologyLinkBridgeDomain(builder.out.links[0]))
	require.Equal(t, 1, builder.out.linksFdb)
	require.Equal(t, 1, builder.out.bidirectionalCount)
	require.Equal(t, 1, builder.out.endpointLinksEmitted)
}

func TestBuildBridgeSegmentActorMarksKnownZeroCountsPresent(t *testing.T) {
	segment := &bridgeDomainSegment{
		ports:       map[string]bridgePortRef{},
		endpointIDs: map[string]struct{}{},
		methods:     map[string]struct{}{},
	}

	_, actor := buildBridgeSegmentActor("empty-segment", segment, "2", "snmp")

	require.Equal(t, OptionalValue[int]{Has: true}, actor.Detail.Segment.PortsTotal)
	require.Equal(t, OptionalValue[int]{Has: true}, actor.Detail.Segment.EndpointsTotal)
}

func TestCollapseActorsByIPMergesTypedActorDetail(t *testing.T) {
	actors := []projectedActor{
		{
			Actor: graph.Actor{
				ActorType: "device",
				Match: graph.Match{
					MacAddresses: []string{"00:00:00:00:00:01"},
					IPAddresses:  []string{"192.0.2.10"},
				},
			},
			Detail: ProjectionActorDetail{
				Device: ProjectionDeviceActorDetail{
					DeviceID: "rep",
				},
			},
		},
		{
			Actor: graph.Actor{
				ActorType: "device",
				Match: graph.Match{
					MacAddresses: []string{"ff:ff:ff:ff:ff:ff"},
					IPAddresses:  []string{"192.0.2.10"},
				},
			},
			Detail: ProjectionActorDetail{
				Device: ProjectionDeviceActorDetail{
					DeviceID:                 "member",
					Discovered:               true,
					Inferred:                 true,
					ManagementIP:             "10.0.0.10",
					ManagementAddresses:      []string{"10.0.0.10"},
					Protocols:                []string{"lldp"},
					ProtocolsCollected:       []string{"lldp"},
					Capabilities:             []string{"bridge"},
					CapabilitiesSupported:    []string{"router"},
					CapabilitiesEnabled:      []string{"bridge"},
					Vendor:                   "Example Vendor",
					VendorSource:             "labels",
					VendorConfidence:         "high",
					VendorDerived:            "Derived Vendor",
					VendorDerivedSource:      "mac_oui",
					VendorDerivedConfidence:  "low",
					VendorDerivedMatchPrefix: "ffffff",
					VendorMatchPrefix:        "ffffff",
				},
				Endpoint: ProjectionEndpointActorDetail{
					Vendor:                   "Endpoint Vendor",
					VendorSource:             "mac_oui",
					VendorConfidence:         "low",
					VendorMatchPrefix:        "ffffff",
					VendorDerived:            "Endpoint Vendor",
					VendorDerivedSource:      "mac_oui",
					VendorDerivedConfidence:  "low",
					VendorDerivedMatchPrefix: "ffffff",
				},
			},
		},
	}

	collapsed := collapseActorsByIP(actors)

	require.Len(t, collapsed, 1)
	detail := collapsed[0].Detail
	require.True(t, detail.CollapsedByIP)
	require.Equal(t, 2, detail.CollapsedCount)
	require.Equal(t, "rep", detail.Device.DeviceID)
	require.True(t, detail.Device.Discovered)
	require.True(t, detail.Device.Inferred)
	require.Equal(t, "10.0.0.10", detail.Device.ManagementIP)
	require.Equal(t, []string{"10.0.0.10"}, detail.Device.ManagementAddresses)
	require.Equal(t, []string{"lldp"}, detail.Device.Protocols)
	require.Equal(t, []string{"lldp"}, detail.Device.ProtocolsCollected)
	require.Equal(t, []string{"bridge"}, detail.Device.Capabilities)
	require.Equal(t, []string{"router"}, detail.Device.CapabilitiesSupported)
	require.Equal(t, []string{"bridge"}, detail.Device.CapabilitiesEnabled)
	require.Equal(t, "Example Vendor", detail.Device.Vendor)
	require.Equal(t, "labels", detail.Device.VendorSource)
	require.Equal(t, "high", detail.Device.VendorConfidence)
	require.Equal(t, "Derived Vendor", detail.Device.VendorDerived)
	require.Equal(t, "mac_oui", detail.Device.VendorDerivedSource)
	require.Equal(t, "low", detail.Device.VendorDerivedConfidence)
	require.Equal(t, "ffffff", detail.Device.VendorDerivedMatchPrefix)
	require.Equal(t, "ffffff", detail.Device.VendorMatchPrefix)
	require.Equal(t, "Endpoint Vendor", detail.Endpoint.Vendor)
	require.Equal(t, "mac_oui", detail.Endpoint.VendorSource)
	require.Equal(t, "low", detail.Endpoint.VendorConfidence)
	require.Equal(t, "ffffff", detail.Endpoint.VendorMatchPrefix)
	require.Equal(t, "Endpoint Vendor", detail.Endpoint.VendorDerived)
	require.Equal(t, "mac_oui", detail.Endpoint.VendorDerivedSource)
	require.Equal(t, "low", detail.Endpoint.VendorDerivedConfidence)
	require.Equal(t, "ffffff", detail.Endpoint.VendorDerivedMatchPrefix)
}
