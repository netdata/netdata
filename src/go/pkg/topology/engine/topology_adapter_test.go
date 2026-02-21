// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"net/netip"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology"
	"github.com/stretchr/testify/require"
)

func TestToTopologyData_ProjectsResult(t *testing.T) {
	collectedAt := time.Date(2026, time.February, 20, 4, 5, 6, 0, time.UTC)

	result := Result{
		CollectedAt: collectedAt,
		Devices: []Device{
			{
				ID:        "local-device",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "remote-device",
				Hostname:  "sw2",
				ChassisID: "aa:bb:cc:dd:ee:ff",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "local-device", IfIndex: 3, IfName: "Gi0/3", IfDescr: "Gi0/3"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "local-device",
				SourcePort: "Gi0/3",
				TargetID:   "remote-device",
				TargetPort: "Gi0/1",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "local-device", IfIndex: 3, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "mac:70:49:a2:65:72:cd",
				MAC:        "70:49:a2:65:72:cd",
				IPs:        []netip.Addr{netip.MustParseAddr("10.20.4.84")},
				Labels: map[string]string{
					"sources":    "arp",
					"if_indexes": "3",
					"if_names":   "Gi0/3",
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		SchemaVersion: "2.0",
		Source:        "snmp",
		Layer:         "2",
		View:          "summary",
		AgentID:       "agent-1",
		LocalDeviceID: "local-device",
	})

	require.Equal(t, "2.0", data.SchemaVersion)
	require.Equal(t, "snmp", data.Source)
	require.Equal(t, "2", data.Layer)
	require.Equal(t, "agent-1", data.AgentID)
	require.Equal(t, collectedAt, data.CollectedAt)

	require.Len(t, data.Actors, 4)
	require.Len(t, data.Links, 3)
	lldpLink := findLinkByProtocol(data.Links, "lldp")
	require.NotNil(t, lldpLink)
	require.Equal(t, "Gi0/3", lldpLink.Src.Attributes["if_name"])
	require.Equal(t, "Gi0/3", lldpLink.Src.Attributes["port_id"])
	require.Equal(t, "sw2", lldpLink.Dst.Attributes["sys_name"])

	localActor := findActorBySysName(data.Actors, "sw1")
	require.NotNil(t, localActor)
	require.Equal(t, false, localActor.Attributes["discovered"])

	endpointActor := findActorByMAC(data.Actors, "70:49:a2:65:72:cd")
	require.NotNil(t, endpointActor)
	require.Equal(t, "endpoint", endpointActor.ActorType)
	require.Equal(t, []string{"10.20.4.84"}, endpointActor.Match.IPAddresses)
	require.Equal(t, []string{"arp", "fdb"}, endpointActor.Attributes["learned_sources"])
	segmentActor := findActorByType(data.Actors, "segment")
	require.NotNil(t, segmentActor)
	require.Contains(t, segmentActor.Match.Hostnames[0], "segment:")
	require.Contains(t, segmentActor.Match.Hostnames[0], "local-device")

	require.Equal(t, 2, data.Stats["devices_total"])
	require.Equal(t, 1, data.Stats["devices_discovered"])
	require.Equal(t, 3, data.Stats["links_total"])
	require.Equal(t, 1, data.Stats["links_lldp"])
	require.Equal(t, 0, data.Stats["links_cdp"])
	require.Equal(t, 2, data.Stats["links_fdb"])
	require.Equal(t, 0, data.Stats["links_arp"])
	require.Equal(t, 2, data.Stats["links_bidirectional"])
	require.Equal(t, 1, data.Stats["links_unidirectional"])
	require.Equal(t, 4, data.Stats["actors_total"])
	require.Equal(t, 1, data.Stats["endpoints_total"])
}

func TestToTopologyData_DefaultDiscoveredCountWithoutLocalID(t *testing.T) {
	result := Result{
		Devices: []Device{
			{ID: "a", Hostname: "a"},
			{ID: "b", Hostname: "b"},
			{ID: "c", Hostname: "c"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{})
	require.Equal(t, 2, data.Stats["devices_discovered"])
}

func TestToTopologyData_DeduplicatesEndpointActorOverlappingManagedDevice(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "sw1",
				Hostname:  "sw1",
				ChassisID: "7049a26572cd",
				Addresses: []netip.Addr{netip.MustParseAddr("10.20.4.84")},
			},
		},
		Attachments: []Attachment{
			{
				DeviceID:   "sw1",
				IfIndex:    1,
				EndpointID: "mac:70:49:a2:65:72:cd",
				Method:     "fdb",
			},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "mac:70:49:a2:65:72:cd",
				MAC:        "70:49:a2:65:72:cd",
				IPs:        []netip.Addr{netip.MustParseAddr("10.20.4.84")},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Actors, 2)
	require.Equal(t, "device", data.Actors[0].ActorType)
	require.Equal(t, "segment", data.Actors[1].ActorType)
	require.Equal(t, 0, data.Stats["endpoints_total"])
	require.Equal(t, 2, data.Stats["actors_total"])
	require.Equal(t, 2, data.Stats["links_total"])
	require.Equal(t, 2, data.Stats["links_fdb"])
}

func TestCanonicalTopologyMatchKey_NormalizesEquivalentMACRepresentations(t *testing.T) {
	raw := topology.Match{
		ChassisIDs: []string{"7049a26572cd"},
	}
	colon := topology.Match{
		ChassisIDs: []string{"70:49:A2:65:72:CD"},
	}

	require.Equal(t, canonicalTopologyMatchKey(raw), canonicalTopologyMatchKey(colon))
	require.Equal(t, "chassis:70:49:a2:65:72:cd", canonicalTopologyMatchKey(raw))
}

func TestToTopologyData_UsesDeterministicPrimaryManagementIP(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "device-a",
				Hostname:  "device-a",
				ChassisID: "aa:bb:cc:dd:ee:ff",
				Addresses: []netip.Addr{
					netip.MustParseAddr("10.0.0.9"),
					netip.MustParseAddr("10.0.0.2"),
					netip.MustParseAddr("10.0.0.9"),
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "device-a")
	require.NotNil(t, actor)
	require.Equal(t, "10.0.0.2", actor.Attributes["management_ip"])
	require.Equal(t, []string{"10.0.0.2", "10.0.0.9"}, actor.Attributes["management_addresses"])
}

func TestToTopologyData_MergesPairedAdjacenciesIntoBidirectionalLink(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "switch-b",
				TargetPort: "Gi0/2",
				Labels: map[string]string{
					adjacencyLabelPairID:   "lldp:pair-a-b",
					adjacencyLabelPairSide: adjacencyPairSideSource,
					adjacencyLabelPairPass: lldpMatchPassDefault,
				},
			},
			{
				Protocol:   "lldp",
				SourceID:   "switch-b",
				SourcePort: "Gi0/2",
				TargetID:   "switch-a",
				TargetPort: "Gi0/1",
				Labels: map[string]string{
					adjacencyLabelPairID:   "lldp:pair-a-b",
					adjacencyLabelPairSide: adjacencyPairSideTarget,
					adjacencyLabelPairPass: lldpMatchPassDefault,
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "lldp", link.Protocol)
	require.Equal(t, "bidirectional", link.Direction)
	require.Equal(t, "Gi0/1", link.Src.Attributes["if_name"])
	require.Equal(t, "Gi0/1", link.Src.Attributes["port_id"])
	require.Equal(t, "Gi0/2", link.Dst.Attributes["if_name"])
	require.Equal(t, "Gi0/2", link.Dst.Attributes["port_id"])

	require.NotNil(t, link.Metrics)
	require.Equal(t, "lldp:pair-a-b", link.Metrics[adjacencyLabelPairID])
	require.Equal(t, lldpMatchPassDefault, link.Metrics[adjacencyLabelPairPass])
	require.Equal(t, true, link.Metrics["pair_consistent"])

	require.Equal(t, 1, data.Stats["links_total"])
	require.Equal(t, 1, data.Stats["links_lldp"])
	require.Equal(t, 1, data.Stats["links_bidirectional"])
	require.Equal(t, 0, data.Stats["links_unidirectional"])
}

func TestToTopologyData_MergesPairedAdjacenciesPreservesRawAddressHints(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "cdp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "switch-b",
				TargetPort: "Gi0/2",
				Labels: map[string]string{
					adjacencyLabelPairID:   "cdp:pair-a-b",
					adjacencyLabelPairSide: adjacencyPairSideSource,
					adjacencyLabelPairPass: cdpMatchPassDefault,
					"remote_address_raw":   "edge-sw3.mgmt.local",
				},
			},
			{
				Protocol:   "cdp",
				SourceID:   "switch-b",
				SourcePort: "Gi0/2",
				TargetID:   "switch-a",
				TargetPort: "Gi0/1",
				Labels: map[string]string{
					adjacencyLabelPairID:   "cdp:pair-a-b",
					adjacencyLabelPairSide: adjacencyPairSideTarget,
					adjacencyLabelPairPass: cdpMatchPassDefault,
					"remote_address_raw":   "10.0.0.1",
				},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "cdp", link.Protocol)
	require.Equal(t, "bidirectional", link.Direction)
	require.Contains(t, link.Dst.Match.IPAddresses, "edge-sw3.mgmt.local")
	require.Contains(t, link.Metrics, "src_remote_address_raw")
	require.Contains(t, link.Metrics, "dst_remote_address_raw")
}

func findActorBySysName(actors []topology.Actor, sysName string) *topology.Actor {
	for i := range actors {
		if actors[i].Match.SysName == sysName {
			return &actors[i]
		}
	}
	return nil
}

func findActorByMAC(actors []topology.Actor, mac string) *topology.Actor {
	for i := range actors {
		for _, candidate := range actors[i].Match.MacAddresses {
			if candidate == mac {
				return &actors[i]
			}
		}
	}
	return nil
}

func findActorByType(actors []topology.Actor, actorType string) *topology.Actor {
	for i := range actors {
		if actors[i].ActorType == actorType {
			return &actors[i]
		}
	}
	return nil
}

func findLinkByProtocol(links []topology.Link, protocol string) *topology.Link {
	for i := range links {
		if links[i].Protocol == protocol {
			return &links[i]
		}
	}
	return nil
}
