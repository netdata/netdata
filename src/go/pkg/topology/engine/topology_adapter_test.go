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
				Labels:    map[string]string{"inferred": "true"},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "local-device", IfIndex: 3, IfName: "Gi0/3", IfDescr: "Gi0/3", Labels: map[string]string{"admin_status": "up", "oper_status": "up"}},
			{DeviceID: "local-device", IfIndex: 4, IfName: "Gi0/4", IfDescr: "Gi0/4", Labels: map[string]string{"admin_status": "up", "oper_status": "lowerLayerDown"}},
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
			{DeviceID: "local-device", IfIndex: 4, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "mac:70:49:a2:65:72:cd",
				MAC:        "70:49:a2:65:72:cd",
				IPs:        []netip.Addr{netip.MustParseAddr("10.20.4.84")},
				Labels: map[string]string{
					"sources":    "arp",
					"if_indexes": "4",
					"if_names":   "Gi0/4",
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

	require.Len(t, data.Actors, 3)
	require.Len(t, data.Links, 2)
	lldpLink := findLinkByProtocol(data.Links, "lldp")
	require.NotNil(t, lldpLink)
	require.Equal(t, "Gi0/3", lldpLink.Src.Attributes["if_name"])
	require.Equal(t, "Gi0/3", lldpLink.Src.Attributes["port_id"])
	require.Equal(t, "up", lldpLink.Src.Attributes["if_admin_status"])
	require.Equal(t, "up", lldpLink.Src.Attributes["if_oper_status"])
	require.Equal(t, "sw2", lldpLink.Dst.Attributes["sys_name"])

	localActor := findActorBySysName(data.Actors, "sw1")
	require.NotNil(t, localActor)
	require.Equal(t, false, localActor.Attributes["discovered"])
	require.Equal(t, false, localActor.Attributes["inferred"])
	require.Equal(t, 2, localActor.Attributes["ports_total"])
	require.NotNil(t, localActor.Attributes["if_admin_status_counts"])
	require.NotNil(t, localActor.Attributes["if_oper_status_counts"])
	require.NotNil(t, localActor.Attributes["if_statuses"])
	remoteActor := findActorBySysName(data.Actors, "sw2")
	require.NotNil(t, remoteActor)
	require.Equal(t, true, remoteActor.Attributes["inferred"])

	endpointActor := findActorByMAC(data.Actors, "70:49:a2:65:72:cd")
	require.NotNil(t, endpointActor)
	require.Equal(t, "endpoint", endpointActor.ActorType)
	require.Equal(t, []string{"10.20.4.84"}, endpointActor.Match.IPAddresses)
	require.Equal(t, []string{"arp", "fdb"}, endpointActor.Attributes["learned_sources"])
	require.Equal(t, "single_port_mac", endpointActor.Attributes["attachment_source"])
	require.Equal(t, "sw1", endpointActor.Attributes["attached_device"])
	require.Equal(t, "Gi0/4", endpointActor.Attributes["attached_port"])

	require.Equal(t, 2, data.Stats["devices_total"])
	require.Equal(t, 1, data.Stats["devices_discovered"])
	require.Equal(t, 2, data.Stats["links_total"])
	require.Equal(t, 1, data.Stats["links_lldp"])
	require.Equal(t, 0, data.Stats["links_cdp"])
	require.Equal(t, 1, data.Stats["links_fdb"])
	require.Equal(t, 0, data.Stats["links_arp"])
	require.Equal(t, 1, data.Stats["links_bidirectional"])
	require.Equal(t, 1, data.Stats["links_unidirectional"])
	require.Equal(t, 3, data.Stats["actors_total"])
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

func TestToTopologyData_AssignsDeterministicActorIDsAndLinkActorIDs(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "sw1",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "sw2",
				Hostname:  "sw2",
				ChassisID: "00:11:22:33:44:66",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "sw2",
				TargetPort: "Gi0/2",
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Actors, 2)
	actorIDs := make(map[string]struct{}, len(data.Actors))
	for _, actor := range data.Actors {
		require.NotEmpty(t, actor.ActorID)
		_, exists := actorIDs[actor.ActorID]
		require.False(t, exists, "duplicate actor_id %q", actor.ActorID)
		actorIDs[actor.ActorID] = struct{}{}
	}

	require.Len(t, data.Links, 1)
	require.NotEmpty(t, data.Links[0].SrcActorID)
	require.NotEmpty(t, data.Links[0].DstActorID)
	_, srcExists := actorIDs[data.Links[0].SrcActorID]
	_, dstExists := actorIDs[data.Links[0].DstActorID]
	require.True(t, srcExists)
	require.True(t, dstExists)
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

	require.Len(t, data.Actors, 1)
	require.Equal(t, "device", data.Actors[0].ActorType)
	require.Equal(t, 0, data.Stats["endpoints_total"])
	require.Equal(t, 1, data.Stats["actors_total"])
	require.Equal(t, 0, data.Stats["links_total"])
	require.Equal(t, 0, data.Stats["links_fdb"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_emitted"])
	require.Equal(t, 1, data.Stats["segments_suppressed"])
}

func TestCanonicalTopologyMatchKey_NormalizesEquivalentMACRepresentations(t *testing.T) {
	raw := topology.Match{
		ChassisIDs: []string{"7049a26572cd"},
	}
	colon := topology.Match{
		ChassisIDs: []string{"70:49:A2:65:72:CD"},
	}

	require.Equal(t, canonicalTopologyMatchKey(raw), canonicalTopologyMatchKey(colon))
	require.Equal(t, "mac:70:49:a2:65:72:cd", canonicalTopologyMatchKey(raw))
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

func TestToTopologyData_KeepsDistinctActorsWhenMACDiffersDespiteSameSecondaryIdentity(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "device-a",
				Hostname:  "shared-name",
				ChassisID: "00:11:22:33:44:55",
				Addresses: []netip.Addr{netip.MustParseAddr("10.20.30.40")},
			},
			{
				ID:        "device-b",
				Hostname:  "shared-name",
				ChassisID: "00:11:22:33:44:66",
				Addresses: []netip.Addr{netip.MustParseAddr("10.20.30.40")},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Actors, 2)
	macs := make(map[string]struct{}, 2)
	for _, actor := range data.Actors {
		require.Equal(t, "device", actor.ActorType)
		require.NotEmpty(t, actor.Match.MacAddresses)
		macs[actor.Match.MacAddresses[0]] = struct{}{}
	}
	require.Contains(t, macs, "00:11:22:33:44:55")
	require.Contains(t, macs, "00:11:22:33:44:66")
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

func TestToTopologyData_DropsAmbiguousEndpointSegmentLinks(t *testing.T) {
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
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	bridgeLinks := 0
	fdbLinks := 0
	for _, link := range data.Links {
		switch link.Protocol {
		case "bridge":
			bridgeLinks++
		case "fdb":
			fdbLinks++
		}
	}

	require.Equal(t, 0, bridgeLinks)
	require.Equal(t, 0, fdbLinks)
	require.Equal(t, 0, data.Stats["links_total"])
	require.Equal(t, 0, data.Stats["links_fdb"])
	require.Equal(t, 2, data.Stats["links_fdb_endpoint_candidates"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_emitted"])
	require.Equal(t, 2, data.Stats["links_fdb_endpoint_suppressed"])
	require.Equal(t, 1, data.Stats["endpoints_ambiguous_segments"])
	require.Equal(t, 2, data.Stats["segments_suppressed"])
}

func TestToTopologyData_PrunesOnlyUnlinkedEndpointsKeepsDevices(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "device-a",
				Hostname:  "device-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "ip:10.0.0.42",
				IPs:        []netip.Addr{netip.MustParseAddr("10.0.0.42")},
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Actors, 1)
	require.Equal(t, "device", data.Actors[0].ActorType)
	require.Equal(t, 0, data.Stats["links_total"])
	require.Equal(t, 1, data.Stats["actors_unlinked_suppressed"])
}

func TestToTopologyData_DisplayNamesPreferDNSThenIPThenMAC(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 3, IfName: "Gi0/3", IfDescr: "Gi0/3"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 3, EndpointID: "ip:10.0.0.42", Method: "arp"},
			{DeviceID: "switch-a", IfIndex: 3, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
		ResolveDNSName: func(ip string) string {
			switch ip {
			case "10.0.0.1":
				return "switch-a.example.net."
			default:
				return ""
			}
		},
	})

	device := findActorBySysName(data.Actors, "switch-a")
	require.NotNil(t, device)
	require.Equal(t, "switch-a.example.net", device.Labels["display_name"])
	require.Equal(t, "dns", device.Labels["display_source"])
	require.Equal(t, "switch-a.example.net", device.Attributes["display_name"])

	ipEndpoint := findActorByIP(data.Actors, "10.0.0.42")
	require.NotNil(t, ipEndpoint)
	require.Equal(t, "10.0.0.42", ipEndpoint.Labels["display_name"])
	require.Equal(t, "ip", ipEndpoint.Labels["display_source"])

	macEndpoint := findActorByMAC(data.Actors, "70:49:a2:65:72:cd")
	require.NotNil(t, macEndpoint)
	require.Equal(t, "70:49:a2:65:72:cd", macEndpoint.Labels["display_name"])
	require.Equal(t, "mac", macEndpoint.Labels["display_source"])

	require.NotEmpty(t, data.Links)
	for _, link := range data.Links {
		require.NotNil(t, link.Src.Attributes)
		require.NotNil(t, link.Dst.Attributes)
		require.NotEmpty(t, link.Src.Attributes["display_name"])
		require.NotEmpty(t, link.Dst.Attributes["display_name"])
	}
}

func TestTopologyDisplayNameFromMatch_PrefersSysNameBeforeIP(t *testing.T) {
	display := topologyDisplayNameFromMatch(topology.Match{
		SysName:     "MikroTik-router",
		IPAddresses: []string{"10.20.4.1"},
	}, &topologyDisplayNameResolver{
		lookup: func(string) string { return "" },
		cache:  map[string]string{},
	})

	require.Equal(t, "MikroTik-router", display.name)
	require.Equal(t, "sys_name", display.source)
}

func TestTopologyDisplayNameFromMatch_PrefersHostnameBeforeIPWhenSysNameMissing(t *testing.T) {
	display := topologyDisplayNameFromMatch(topology.Match{
		Hostnames:   []string{"nova"},
		IPAddresses: []string{"10.20.4.22"},
	}, &topologyDisplayNameResolver{
		lookup: func(string) string { return "" },
		cache:  map[string]string{},
	})

	require.Equal(t, "nova", display.name)
	require.Equal(t, "hostname", display.source)
}

func TestToTopologyData_SegmentDisplayNameUsesParentPortPattern(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 3, IfName: "Gi0/3", IfDescr: "Gi0/3"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 3, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
			{DeviceID: "switch-a", IfIndex: 3, EndpointID: "mac:70:49:a2:65:72:ce", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
		ResolveDNSName: func(ip string) string {
			if ip == "10.0.0.1" {
				return "switch-a.example.net."
			}
			return ""
		},
	})

	segment := findActorByType(data.Actors, "segment")
	require.NotNil(t, segment)
	require.Equal(t, "switch-a.example.net.gi0/3.segment", segment.Labels["display_name"])
	require.Equal(t, "segment", segment.Labels["display_source"])
	require.Equal(t, "switch-a.example.net.gi0/3.segment", segment.Attributes["display_name"])
}

func TestToTopologyData_FDBOwnerInferencePrefersNonLLDPSide(t *testing.T) {
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
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", MAC: "aa:aa:aa:aa:aa:aa"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "Gi0/1", MAC: "bb:bb:bb:bb:bb:bb"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", MAC: "bb:bb:bb:bb:bb:bc"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "switch-b",
				TargetPort: "Gi0/1",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:70:49:a2:65:72:cd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:ee:ee:ee:ee:ee:ee", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	targetLinks := findFDBLinksByEndpointMAC(data.Links, "70:49:a2:65:72:cd")
	require.Len(t, targetLinks, 1)

	segmentActor := findActorByMatch(data.Actors, targetLinks[0].Src.Match)
	require.NotNil(t, segmentActor)
	require.Equal(t, []string{"switch-b"}, segmentActor.Attributes["parent_devices"])
	require.Equal(t, []string{"Gi0/2"}, segmentActor.Attributes["if_names"])
}

func TestToTopologyData_FDBOwnerInferenceUsesSingleMACPortRule(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:       "switch-a",
				Hostname: "switch-a",
			},
			{
				ID:       "switch-b",
				Hostname: "switch-b",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1"},
			{DeviceID: "switch-a", IfIndex: 2, IfName: "Gi0/2"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "Gi0/1"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
			{DeviceID: "switch-a", IfIndex: 2, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	targetLinks := findFDBLinksByEndpointMAC(data.Links, "dd:dd:dd:dd:dd:dd")
	require.Len(t, targetLinks, 1)

	srcActor := findActorByMatch(data.Actors, targetLinks[0].Src.Match)
	require.NotNil(t, srcActor)
	require.Equal(t, "device", srcActor.ActorType)
	require.Equal(t, "switch-a", srcActor.Match.SysName)

	endpointActor := findActorByMAC(data.Actors, "dd:dd:dd:dd:dd:dd")
	require.NotNil(t, endpointActor)
	require.Equal(t, "endpoint", endpointActor.ActorType)
	require.Equal(t, "single_port_mac", endpointActor.Attributes["attachment_source"])
	require.Equal(t, "switch-a", endpointActor.Attributes["attached_device"])
	require.Equal(t, "Gi0/1", endpointActor.Attributes["attached_port"])
	require.Equal(t, "single_port_mac", endpointActor.Labels["attached_by"])
}

func TestToTopologyData_SuppressesFDBEndpointsOnLLDPPorts(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "host-b",
				Hostname:  "host-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", MAC: "aa:aa:aa:aa:aa:aa"},
			{DeviceID: "host-b", IfIndex: 1, IfName: "eth0", MAC: "bb:bb:bb:bb:bb:bb"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "host-b",
				TargetPort: "eth0",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
			{DeviceID: "host-b", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	bridgeLinks := 0
	fdbLinks := 0
	segmentActors := 0
	for _, actor := range data.Actors {
		if actor.ActorType == "segment" {
			segmentActors++
		}
	}
	for _, link := range data.Links {
		switch link.Protocol {
		case "bridge":
			bridgeLinks++
		case "fdb":
			fdbLinks++
		}
	}

	require.Equal(t, 0, segmentActors)
	require.Equal(t, 0, bridgeLinks)
	require.Equal(t, 0, fdbLinks)
	require.Equal(t, 1, data.Stats["links_total"])
	require.Equal(t, 0, data.Stats["links_fdb"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_candidates"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_emitted"])
	require.Equal(t, 0, data.Stats["links_fdb_endpoint_suppressed"])
}

func TestToTopologyData_KeepsChassisPlaceholderDevicesAsDevices(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "chassis-788cb595dfcc",
				Hostname:  "chassis-788cb595dfcc",
				ChassisID: "78:8c:b5:95:df:cc",
			},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "chassis-788cb595dfcc",
				TargetPort: "eth0",
			},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	placeholder := findActorBySysName(data.Actors, "chassis-788cb595dfcc")
	require.NotNil(t, placeholder)
	require.Equal(t, "device", placeholder.ActorType)
}

func TestPruneSegmentArtifacts_SuppressesLLDPDuplicateSegmentPath(t *testing.T) {
	actors := []topology.Actor{
		{
			ActorType: "device",
			Match:     topology.Match{IPAddresses: []string{"10.0.0.1"}, SysName: "switch-a"},
		},
		{
			ActorType: "device",
			Match:     topology.Match{IPAddresses: []string{"10.0.0.2"}, SysName: "switch-b"},
		},
		{
			ActorType: "segment",
			Match:     topology.Match{Hostnames: []string{"segment:dup"}},
		},
	}

	links := []topology.Link{
		{
			Protocol: "lldp",
			Src:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.0.1"}}},
			Dst:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.0.2"}}},
		},
		{
			Protocol: "bridge",
			Src:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.0.1"}}},
			Dst:      topology.LinkEndpoint{Match: topology.Match{Hostnames: []string{"segment:dup"}}},
		},
		{
			Protocol: "bridge",
			Src:      topology.LinkEndpoint{Match: topology.Match{Hostnames: []string{"segment:dup"}}},
			Dst:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.0.2"}}},
		},
	}

	filteredActors, filteredLinks, suppressed := pruneSegmentArtifacts(actors, links)
	require.Equal(t, 1, suppressed)
	require.Len(t, filteredActors, 2)
	require.Len(t, filteredLinks, 1)
	require.Equal(t, "lldp", filteredLinks[0].Protocol)
}

func TestPruneSegmentArtifacts_SuppressesSegmentsWithSingleNeighbor(t *testing.T) {
	actors := []topology.Actor{
		{
			ActorType: "device",
			Match:     topology.Match{IPAddresses: []string{"10.0.0.1"}, SysName: "router-a"},
		},
		{
			ActorType: "segment",
			Match:     topology.Match{Hostnames: []string{"segment:orphan"}},
		},
	}

	links := []topology.Link{
		{
			Protocol: "bridge",
			Src:      topology.LinkEndpoint{Match: topology.Match{IPAddresses: []string{"10.0.0.1"}}},
			Dst:      topology.LinkEndpoint{Match: topology.Match{Hostnames: []string{"segment:orphan"}}},
		},
	}

	filteredActors, filteredLinks, suppressed := pruneSegmentArtifacts(actors, links)
	require.Equal(t, 1, suppressed)
	require.Len(t, filteredActors, 1)
	require.Len(t, filteredLinks, 0)
}

func TestToTopologyData_FDBOwnerInferenceUsesReporterMatrixRule(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
			},
			{
				ID:        "switch-b",
				Hostname:  "switch-b",
				ChassisID: "bb:bb:bb:bb:bb:bb",
			},
			{
				ID:        "switch-c",
				Hostname:  "switch-c",
				ChassisID: "cc:cc:cc:cc:cc:cc",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", MAC: "aa:aa:aa:aa:aa:aa"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "Gi0/1", MAC: "bb:bb:bb:bb:bb:bb"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", MAC: "bb:bb:bb:bb:bb:bc"},
			{DeviceID: "switch-c", IfIndex: 1, IfName: "Gi0/1", MAC: "cc:cc:cc:cc:cc:cc"},
			{DeviceID: "switch-c", IfIndex: 2, IfName: "Gi0/2", MAC: "cc:cc:cc:cc:cc:cd"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:cc:cc:cc:cc:cc:cc", Method: "fdb"},
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:cc:cc:cc:cc:cc:cc", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
			{DeviceID: "switch-c", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
			{DeviceID: "switch-c", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
			{DeviceID: "switch-c", IfIndex: 2, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
			{DeviceID: "switch-c", IfIndex: 2, EndpointID: "mac:ee:ee:ee:ee:ee:ee", Method: "fdb"},
		},
	}

	data := ToTopologyData(result, TopologyDataOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	targetLinks := findFDBLinksByEndpointMAC(data.Links, "dd:dd:dd:dd:dd:dd")
	require.Len(t, targetLinks, 1)

	segmentActor := findActorByMatch(data.Actors, targetLinks[0].Src.Match)
	require.NotNil(t, segmentActor)
	require.Equal(t, []string{"switch-c"}, segmentActor.Attributes["parent_devices"])
	require.Equal(t, []string{"Gi0/2"}, segmentActor.Attributes["if_names"])
}

func findFDBLinksByEndpointMAC(links []topology.Link, mac string) []topology.Link {
	out := make([]topology.Link, 0)
	for _, link := range links {
		if link.Protocol != "fdb" {
			continue
		}
		for _, candidate := range link.Dst.Match.MacAddresses {
			if candidate == mac {
				out = append(out, link)
				break
			}
		}
	}
	return out
}

func findActorByMatch(actors []topology.Actor, match topology.Match) *topology.Actor {
	target := canonicalTopologyMatchKey(match)
	if target == "" {
		return nil
	}
	for i := range actors {
		if canonicalTopologyMatchKey(actors[i].Match) == target {
			return &actors[i]
		}
	}
	return nil
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

func findActorByIP(actors []topology.Actor, ip string) *topology.Actor {
	for i := range actors {
		for _, candidate := range actors[i].Match.IPAddresses {
			if candidate == ip {
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
