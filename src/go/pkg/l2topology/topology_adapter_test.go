// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
	"github.com/stretchr/testify/require"
)

func TestToGraph_ProjectsResult(t *testing.T) {
	collectedAt := time.Date(2026, time.February, 20, 4, 5, 6, 0, time.UTC)

	result := Result{
		CollectedAt: collectedAt,
		Devices: []Device{
			{
				ID:        "local-device",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
				Labels:    map[string]string{"protocols_observed": "bridge,fdb,stp"},
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

	data, stats := toGraphForTest(result, GraphOptions{
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
	require.Equal(t, "Gi0/3", lldpLink.Src.IfName)
	require.Equal(t, "Gi0/3", lldpLink.Src.PortID)
	require.Equal(t, "up", lldpLink.Src.AdminStatus)
	require.Equal(t, "up", lldpLink.Src.OperStatus)
	require.Equal(t, "sw2", lldpLink.Dst.SysName)

	localActor := findActorBySysName(data.Actors, "sw1")
	require.NotNil(t, localActor)
	localDetail := requireActorDetail(t, data, localActor)
	require.False(t, localDetail.Device.Discovered)
	require.False(t, localDetail.Device.Inferred)
	require.Equal(t, []string{"bridge", "fdb", "stp"}, localDetail.Device.Protocols)
	require.Equal(t, []string{"bridge", "fdb", "stp"}, localDetail.Device.ProtocolsCollected)
	require.Equal(t, OptionalValue[int]{Value: 2, Has: true}, localDetail.Device.PortsTotal)
	require.NotNil(t, localDetail.Device.AdminStatusCounts)
	require.NotNil(t, localDetail.Device.OperStatusCounts)
	require.NotNil(t, localDetail.Device.LinkModeCounts)
	require.NotNil(t, localDetail.Device.TopologyRoleCounts)
	require.NotNil(t, localDetail.Device.Ports)
	remoteActor := findActorBySysName(data.Actors, "sw2")
	require.NotNil(t, remoteActor)
	remoteDetail := requireActorDetail(t, data, remoteActor)
	require.True(t, remoteDetail.Device.Inferred)

	endpointActor := findActorByMAC(data.Actors, "70:49:a2:65:72:cd")
	require.NotNil(t, endpointActor)
	require.Equal(t, "endpoint", endpointActor.ActorType)
	require.Equal(t, []string{"10.20.4.84"}, endpointActor.Match.IPAddresses)
	endpointDetail := requireActorDetail(t, data, endpointActor)
	require.Equal(t, []string{"arp", "fdb"}, endpointDetail.Endpoint.LearnedSources)
	require.Equal(t, "single_port_mac", endpointDetail.Endpoint.AttachmentSource)
	require.Equal(t, "sw1", endpointDetail.Endpoint.AttachedDevice)
	require.Equal(t, "Gi0/4", endpointDetail.Endpoint.AttachedPort)

	require.Equal(t, 2, stats.DevicesTotal)
	require.Equal(t, 1, stats.DevicesDiscovered)
	require.Equal(t, 2, stats.LinksTotal)
	require.Equal(t, 1, stats.LinksLLDP)
	require.Equal(t, 0, stats.LinksCDP)
	require.Equal(t, 1, stats.LinksFDB)
	require.Equal(t, 0, stats.LinksARP)
	require.Equal(t, 1, stats.LinksBidirectional)
	require.Equal(t, 1, stats.LinksUnidirectional)
	require.Equal(t, 3, stats.ActorsTotal)
	require.Equal(t, 1, stats.EndpointsTotal)
}

func TestToGraph_ProjectsTypedActorDetailsWithFieldPresence(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "sw1",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
				Labels: map[string]string{
					"capabilities_enabled": "bridge,router",
				},
			},
		},
		Interfaces: []Interface{
			{
				DeviceID: "sw1",
				IfIndex:  1,
				IfName:   "Gi0/1",
				IfDescr:  "GigabitEthernet0/1",
				MAC:      "00:11:22:33:44:56",
				Labels: map[string]string{
					"speed_bps":   "fast(1000000000)",
					"last_change": "ticks(12345)",
					"duplex":      "full",
				},
			},
		},
	}

	projection := ToGraph(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(projection.Graph.Actors, "sw1")
	require.NotNil(t, actor)
	detail, ok := projection.ActorDetails[actor.ActorID]
	require.True(t, ok)

	require.Equal(t, "10.0.0.1", detail.Device.ManagementIP)
	require.Equal(t, []string{"bridge", "router"}, detail.Device.CapabilitiesEnabled)
	require.True(t, detail.Device.PortsTotal.Has)
	require.Equal(t, 1, detail.Device.PortsTotal.Value)
	require.False(t, detail.Device.CDPNeighborCount.Has)
	require.Zero(t, detail.Device.CDPNeighborCount.Value)
	require.Len(t, detail.Device.Ports, 1)

	port := detail.Device.Ports[0]
	require.True(t, port.IfIndex.Has)
	require.Equal(t, 1, port.IfIndex.Value)
	require.True(t, port.Speed.Has)
	require.EqualValues(t, 1000000000, port.Speed.Value)
	require.Equal(t, "12345", port.LastChange)
	require.Equal(t, "full", port.Duplex)
	require.False(t, port.NeighborCount.Has)
	require.Zero(t, port.NeighborCount.Value)
	require.False(t, port.LinkCount.Has)
	require.Zero(t, port.LinkCount.Value)
}

func TestToGraph_ClassifiesPortLinkModesFromFDBAndSTPEvidence(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "sw1",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
			},
			{
				ID:        "sw2",
				Hostname:  "sw2",
				ChassisID: "00:11:22:33:44:66",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "sw1", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "sw1", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
			{DeviceID: "sw1", IfIndex: 3, IfName: "Gi0/3", IfDescr: "Gi0/3"},
			{DeviceID: "sw1", IfIndex: 4, IfName: "Gi0/4", IfDescr: "Gi0/4"},
			{DeviceID: "sw2", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "sw1",
				SourcePort: "Gi0/3",
				TargetID:   "sw2",
				TargetPort: "Gi0/1",
			},
			{
				Protocol:   "stp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "sw2",
				TargetPort: "Gi0/1",
				Labels: map[string]string{
					"vlan_id": "20",
				},
			},
		},
		Attachments: []Attachment{
			{
				DeviceID:   "sw1",
				IfIndex:    1,
				EndpointID: "mac:00:00:00:00:10:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "10",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    1,
				EndpointID: "mac:00:00:00:00:20:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "20",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    2,
				EndpointID: "mac:00:00:00:00:30:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "30",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    3,
				EndpointID: "mac:00:00:00:00:40:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "40",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    4,
				EndpointID: "mac:00:00:00:00:50:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    4,
				EndpointID: "mac:00:00:00:00:50:02",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
				},
			},
		},
	}

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "sw1")
	require.NotNil(t, actor)
	detail := requireActorDetail(t, data, actor)

	require.Equal(t, 1, detail.Device.LinkModeCounts["trunk"])
	require.Equal(t, 1, detail.Device.LinkModeCounts["access"])
	require.Equal(t, 2, detail.Device.LinkModeCounts["unknown"])

	require.Equal(t, 1, detail.Device.TopologyRoleCounts["switch_facing"])
	require.Equal(t, 1, detail.Device.TopologyRoleCounts["host_facing"])
	require.Equal(t, 1, detail.Device.TopologyRoleCounts["host_candidate"])
	require.Equal(t, 1, detail.Device.TopologyRoleCounts["unknown"])

	port1 := findInterfaceStatusByIndex(detail.Device.Ports, 1)
	require.NotNil(t, port1)
	require.Equal(t, "trunk", port1.LinkMode)
	require.Equal(t, "high", port1.LinkModeConfidence)
	require.Equal(t, []string{"fdb", "stp"}, port1.LinkModeSources)
	require.Equal(t, []string{"10", "20"}, port1.VLANIDs)
	require.Equal(t, "unknown", port1.TopologyRole)
	require.Equal(t, "low", port1.TopologyRoleConfidence)
	require.Equal(t, []string{"stp", "fdb"}, port1.TopologyRoleSources)

	port2 := findInterfaceStatusByIndex(detail.Device.Ports, 2)
	require.NotNil(t, port2)
	require.Equal(t, "access", port2.LinkMode)
	require.Equal(t, "medium", port2.LinkModeConfidence)
	require.Equal(t, []string{"fdb"}, port2.LinkModeSources)
	require.Equal(t, []string{"30"}, port2.VLANIDs)
	require.Equal(t, "host_facing", port2.TopologyRole)
	require.Equal(t, "medium", port2.TopologyRoleConfidence)
	require.Equal(t, []string{"fdb"}, port2.TopologyRoleSources)

	port3 := findInterfaceStatusByIndex(detail.Device.Ports, 3)
	require.NotNil(t, port3)
	require.Equal(t, "unknown", port3.LinkMode)
	require.Equal(t, "low", port3.LinkModeConfidence)
	require.Equal(t, []string{"fdb", "peer_link"}, port3.LinkModeSources)
	require.Equal(t, []string{"40"}, port3.VLANIDs)
	require.Equal(t, "switch_facing", port3.TopologyRole)
	require.Equal(t, "high", port3.TopologyRoleConfidence)
	require.Equal(t, []string{"peer_link", "bridge_link", "fdb"}, port3.TopologyRoleSources)

	port4 := findInterfaceStatusByIndex(detail.Device.Ports, 4)
	require.NotNil(t, port4)
	require.Equal(t, "unknown", port4.LinkMode)
	require.Equal(t, "low", port4.LinkModeConfidence)
	require.Equal(t, []string{"fdb"}, port4.LinkModeSources)
	require.Empty(t, port4.VLANIDs)
	require.Equal(t, "host_candidate", port4.TopologyRole)
	require.Equal(t, "low", port4.TopologyRoleConfidence)
	require.Equal(t, []string{"fdb"}, port4.TopologyRoleSources)
}

func TestToGraph_IgnoresIgnoredFDBStatusForLinkModeClassification(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:       "sw1",
				Hostname: "sw1",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "sw1", IfIndex: 10, IfName: "Gi0/10", IfDescr: "Gi0/10"},
		},
		Attachments: []Attachment{
			{
				DeviceID:   "sw1",
				IfIndex:    10,
				EndpointID: "mac:00:00:00:00:50:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "ignored",
					"vlan_id":    "50",
				},
			},
		},
	}

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "sw1")
	require.NotNil(t, actor)
	detail := requireActorDetail(t, data, actor)
	port := findInterfaceStatusByIndex(detail.Device.Ports, 10)
	require.NotNil(t, port)
	require.Equal(t, "unknown", port.LinkMode)
	require.Equal(t, "low", port.LinkModeConfidence)
	require.Empty(t, port.LinkModeSources)
	require.Empty(t, port.VLANIDs)
	require.Equal(t, "unknown", port.TopologyRole)
	require.Equal(t, "low", port.TopologyRoleConfidence)
	require.Empty(t, port.TopologyRoleSources)
}

func TestToGraph_ClassifiesSTPCorroboratedManagedAliasAsSwitchFacing(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "sw1",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
			},
			{
				ID:        "sw2",
				Hostname:  "sw2",
				ChassisID: "00:11:22:33:44:66",
			},
		},
		Interfaces: []Interface{
			{DeviceID: "sw1", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "stp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "stp-root",
				Labels: map[string]string{
					"vlan_id": "10",
				},
			},
		},
		Attachments: []Attachment{
			{
				DeviceID:   "sw1",
				IfIndex:    1,
				EndpointID: "mac:00:11:22:33:44:66",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "10",
				},
			},
		},
	}

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "sw1")
	require.NotNil(t, actor)
	detail := requireActorDetail(t, data, actor)
	port1 := findInterfaceStatusByIndex(detail.Device.Ports, 1)
	require.NotNil(t, port1)
	require.Equal(t, "switch_facing", port1.TopologyRole)
	require.Equal(t, "medium", port1.TopologyRoleConfidence)
	require.Equal(t, []string{"stp", "fdb", "fdb_managed_alias"}, port1.TopologyRoleSources)
}

func TestToGraph_EnrichesPortStatusesWithNeighborsFDBAndSTP(t *testing.T) {
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
				ChassisID: "aa:bb:cc:dd:ee:ff",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
				Labels: map[string]string{
					"capabilities_enabled": "bridge,router",
				},
			},
		},
		Interfaces: []Interface{
			{
				DeviceID: "sw1",
				IfIndex:  1,
				IfName:   "Gi0/1",
				IfDescr:  "Gi0/1",
				MAC:      "00:11:22:33:44:55",
				Labels: map[string]string{
					"admin_status": "up",
					"oper_status":  "up",
					"if_alias":     "uplink-core",
					"speed_bps":    "1000000000",
					"last_change":  "12345",
					"duplex":       "full",
				},
			},
			{
				DeviceID: "sw1",
				IfIndex:  2,
				IfName:   "Gi0/2",
				IfDescr:  "Gi0/2",
				MAC:      "00:11:22:33:44:66",
				Labels: map[string]string{
					"admin_status": "down",
					"oper_status":  "down",
					"if_alias":     "server-a",
					"speed_bps":    "100000000",
					"last_change":  "54321",
					"duplex":       "half",
				},
			},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "sw2",
				TargetPort: "Gi0/24",
			},
			{
				Protocol:   "cdp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "sw2",
				TargetPort: "Gi0/24",
			},
			{
				Protocol:   "stp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "sw2",
				TargetPort: "Gi0/24",
				Labels: map[string]string{
					"stp_state": "forwarding",
					"vlan_id":   "10",
				},
			},
			{
				Protocol:   "stp",
				SourceID:   "sw1",
				SourcePort: "Gi0/1",
				TargetID:   "sw2",
				TargetPort: "Gi0/24",
				Labels: map[string]string{
					"stp_state": "blocking",
					"vlan_id":   "20",
				},
			},
		},
		Attachments: []Attachment{
			{
				DeviceID:   "sw1",
				IfIndex:    1,
				EndpointID: "mac:00:00:00:00:10:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "10",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    1,
				EndpointID: "mac:00:00:00:00:20:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
					"vlan_id":    "20",
				},
			},
			{
				DeviceID:   "sw1",
				IfIndex:    2,
				EndpointID: "mac:00:00:00:00:30:01",
				Method:     "fdb",
				Labels: map[string]string{
					"fdb_status": "learned",
				},
			},
		},
	}

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "sw1")
	require.NotNil(t, actor)
	detail := requireActorDetail(t, data, actor)
	require.Equal(t, OptionalValue[int]{Value: 1, Has: true}, detail.Device.PortsUp)
	require.Equal(t, OptionalValue[int]{Value: 1, Has: true}, detail.Device.PortsDown)
	require.Equal(t, OptionalValue[int]{Value: 1, Has: true}, detail.Device.PortsAdminDown)
	require.Equal(t, OptionalValue[int64]{Value: 1_000_000_000, Has: true}, detail.Device.TotalBandwidthBps)
	require.Equal(t, OptionalValue[int]{Value: 3, Has: true}, detail.Device.FDBTotalMACs)
	require.Equal(t, OptionalValue[int]{Value: 2, Has: true}, detail.Device.VLANCount)
	require.Equal(t, OptionalValue[int]{Value: 1, Has: true}, detail.Device.LLDPNeighborCount)
	require.Equal(t, OptionalValue[int]{Value: 1, Has: true}, detail.Device.CDPNeighborCount)

	port1 := findInterfaceStatusByIndex(detail.Device.Ports, 1)
	require.NotNil(t, port1)
	require.Equal(t, "Gi0/1", port1.IfDescr)
	require.Equal(t, "uplink-core", port1.IfAlias)
	require.Equal(t, "00:11:22:33:44:55", port1.MAC)
	require.Equal(t, OptionalValue[int64]{Value: 1_000_000_000, Has: true}, port1.Speed)
	require.Equal(t, "12345", port1.LastChange)
	require.Equal(t, "full", port1.Duplex)
	require.Equal(t, OptionalValue[int]{Value: 2, Has: true}, port1.FDBMACCount)
	require.Equal(t, "blocking", port1.STPState)
	require.Len(t, port1.VLANs, 2)
	require.Equal(t, "10", port1.VLANs[0].VLANID)
	require.True(t, port1.VLANs[0].Tagged)
	require.Equal(t, "20", port1.VLANs[1].VLANID)
	require.True(t, port1.VLANs[1].Tagged)
	require.Len(t, port1.Neighbors, 2)

	cdpNeighbor := findNeighborByProtocol(port1.Neighbors, "cdp")
	require.NotNil(t, cdpNeighbor)
	require.Equal(t, "sw2", cdpNeighbor.RemoteDevice)
	require.Equal(t, "Gi0/24", cdpNeighbor.RemotePort)
	require.Equal(t, "10.0.0.2", cdpNeighbor.RemoteIP)
	require.Equal(t, "aa:bb:cc:dd:ee:ff", cdpNeighbor.RemoteChassisID)
	require.Equal(t, []string{"bridge", "router"}, cdpNeighbor.RemoteCapabilities)

	lldpNeighbor := findNeighborByProtocol(port1.Neighbors, "lldp")
	require.NotNil(t, lldpNeighbor)
	require.Equal(t, "sw2", lldpNeighbor.RemoteDevice)
	require.Equal(t, "Gi0/24", lldpNeighbor.RemotePort)
	require.Equal(t, "10.0.0.2", lldpNeighbor.RemoteIP)
	require.Equal(t, "aa:bb:cc:dd:ee:ff", lldpNeighbor.RemoteChassisID)
	require.Equal(t, []string{"bridge", "router"}, lldpNeighbor.RemoteCapabilities)

	port2 := findInterfaceStatusByIndex(detail.Device.Ports, 2)
	require.NotNil(t, port2)
	require.Equal(t, "server-a", port2.IfAlias)
	require.Equal(t, "00:11:22:33:44:66", port2.MAC)
	require.Equal(t, OptionalValue[int64]{Value: 100_000_000, Has: true}, port2.Speed)
	require.Equal(t, "54321", port2.LastChange)
	require.Equal(t, "half", port2.Duplex)
	require.Equal(t, OptionalValue[int]{Value: 1, Has: true}, port2.FDBMACCount)
	require.Empty(t, port2.Neighbors)
}

func TestToGraph_InfersVendorFromMACOUI(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "sw1",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
			},
			{
				ID:        "remote-device",
				Hostname:  "edge-remote",
				ChassisID: "28:6f:b9:00:00:22",
				Labels:    map[string]string{"inferred": "true"},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "sw1", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
		},
		Attachments: []Attachment{
			{DeviceID: "sw1", IfIndex: 1, EndpointID: "mac:08:ea:44:11:22:33", Method: "fdb"},
		},
		Enrichments: []Enrichment{
			{EndpointID: "mac:08:ea:44:11:22:33", MAC: "08:ea:44:11:22:33"},
		},
	}

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	remote := findActorBySysName(data.Actors, "edge-remote")
	require.NotNil(t, remote)
	remoteDetail := requireActorDetail(t, data, remote)
	require.Equal(t, "Nokia Shanghai Bell Co., Ltd.", remoteDetail.Device.Vendor)
	require.Equal(t, "mac_oui", remoteDetail.Device.VendorSource)
	require.Equal(t, "low", remoteDetail.Device.VendorConfidence)
	require.Equal(t, "Nokia Shanghai Bell Co., Ltd.", remoteDetail.Device.VendorDerived)
	require.Equal(t, "mac_oui", remoteDetail.Device.VendorDerivedSource)
	require.Equal(t, "low", remoteDetail.Device.VendorDerivedConfidence)
	require.NotEmpty(t, remoteDetail.Device.VendorDerivedMatchPrefix)

	endpoint := findActorByMAC(data.Actors, "08:ea:44:11:22:33")
	require.NotNil(t, endpoint)
	require.Equal(t, "endpoint", endpoint.ActorType)
	endpointDetail := requireActorDetail(t, data, endpoint)
	require.Equal(t, "Extreme Networks Headquarters", endpointDetail.Endpoint.Vendor)
	require.Equal(t, "mac_oui", endpointDetail.Endpoint.VendorSource)
	require.Equal(t, "low", endpointDetail.Endpoint.VendorConfidence)
	require.Equal(t, "Extreme Networks Headquarters", endpointDetail.Endpoint.VendorDerived)
	require.Equal(t, "mac_oui", endpointDetail.Endpoint.VendorDerivedSource)
	require.Equal(t, "low", endpointDetail.Endpoint.VendorDerivedConfidence)
	require.NotEmpty(t, endpointDetail.Endpoint.VendorDerivedMatchPrefix)
}

func TestDeviceToTopologyActor_DoesNotOverrideExplicitVendor(t *testing.T) {
	actor := deviceToTopologyActor(
		Device{
			ID:        "switch-a",
			Hostname:  "switch-a",
			ChassisID: "08:ea:44:99:88:77",
			Labels: map[string]string{
				"vendor": "Explicit Vendor",
			},
		},
		"snmp",
		"2",
		"",
		topologyDeviceInterfaceSummary{},
		nil,
	)

	require.Equal(t, "Explicit Vendor", actor.Detail.Device.Vendor)
	require.Equal(t, "labels", actor.Detail.Device.VendorSource)
	require.Equal(t, "high", actor.Detail.Device.VendorConfidence)
	require.Equal(t, "Extreme Networks Headquarters", actor.Detail.Device.VendorDerived)
	require.Equal(t, "mac_oui", actor.Detail.Device.VendorDerivedSource)
	require.Equal(t, "low", actor.Detail.Device.VendorDerivedConfidence)
	require.NotEmpty(t, actor.Detail.Device.VendorDerivedMatchPrefix)
}

func TestToGraph_DefaultDiscoveredCountWithoutLocalID(t *testing.T) {
	result := Result{
		Devices: []Device{
			{ID: "a", Hostname: "a"},
			{ID: "b", Hostname: "b"},
			{ID: "c", Hostname: "c"},
		},
	}

	_, stats := toGraphForTest(result, GraphOptions{})
	require.Equal(t, 2, stats.DevicesDiscovered)
}

func TestToGraph_AssignsDeterministicActorIDsAndLinkActorIDs(t *testing.T) {
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

	data, _ := toGraphForTest(result, GraphOptions{
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

func TestToGraph_DeterministicAcrossRepeatedCalls(t *testing.T) {
	collectedAt := time.Date(2026, time.February, 20, 4, 5, 6, 0, time.UTC)

	result := Result{
		CollectedAt: collectedAt,
		Devices: []Device{
			{
				ID:        "local-device",
				Hostname:  "sw1",
				ChassisID: "00:11:22:33:44:55",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
				Labels:    map[string]string{"protocols_observed": "bridge,fdb,stp"},
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

	opts := GraphOptions{
		SchemaVersion: "2.0",
		Source:        "snmp",
		Layer:         "2",
		View:          "summary",
		AgentID:       "agent-1",
		LocalDeviceID: "local-device",
	}

	baseline, _ := toGraphForTest(result, opts)
	for range 10 {
		next, _ := toGraphForTest(result, opts)
		require.Equal(t, baseline, next)
	}
}

func TestToGraph_DeduplicatesEndpointActorOverlappingManagedDevice(t *testing.T) {
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

	data, stats := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Actors, 1)
	require.Equal(t, "device", data.Actors[0].ActorType)
	require.Equal(t, 0, stats.EndpointsTotal)
	require.Equal(t, 1, stats.ActorsTotal)
	require.Equal(t, 0, stats.LinksTotal)
	require.Equal(t, 0, stats.LinksFDBEndpointEmitted)
	require.Equal(t, 1, stats.SegmentsSuppressed)
}

func TestCanonicalTopologyMatchKey_NormalizesEquivalentMACRepresentations(t *testing.T) {
	raw := graph.Match{
		ChassisIDs: []string{"7049a26572cd"},
	}
	colon := graph.Match{
		ChassisIDs: []string{"70:49:A2:65:72:CD"},
	}

	require.Equal(t, canonicalTopologyMatchKey(raw), canonicalTopologyMatchKey(colon))
	require.Equal(t, "mac:70:49:a2:65:72:cd", canonicalTopologyMatchKey(raw))
}

func TestToGraph_UsesDeterministicPrimaryManagementIP(t *testing.T) {
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

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "device-a")
	require.NotNil(t, actor)
	detail := requireActorDetail(t, data, actor)
	require.Equal(t, "10.0.0.2", detail.Device.ManagementIP)
	require.Equal(t, []string{"10.0.0.2", "10.0.0.9"}, detail.Device.ManagementAddresses)
}

func TestToGraph_KeepsDistinctActorsWhenMACDiffersDespiteSameSecondaryIdentity(t *testing.T) {
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

	data, _ := toGraphForTest(result, GraphOptions{
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

func TestToGraph_MergesPairedAdjacenciesIntoBidirectionalLink(t *testing.T) {
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
					adjacencyLabelPairPass: lldpMatchPassDefault,
				},
			},
		},
	}

	data, stats := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "lldp", link.Protocol)
	require.Equal(t, "bidirectional", link.Direction)
	require.Equal(t, "Gi0/1", link.Src.IfName)
	require.Equal(t, "Gi0/1", link.Src.PortID)
	require.Equal(t, "Gi0/2", link.Dst.IfName)
	require.Equal(t, "Gi0/2", link.Dst.PortID)

	require.NotNil(t, link.L2)
	require.Equal(t, "lldp:pair-a-b", link.L2.PairID)
	require.Equal(t, lldpMatchPassDefault, link.L2.PairPass)
	require.True(t, link.L2.PairConsistent)

	require.Equal(t, 1, stats.LinksTotal)
	require.Equal(t, 1, stats.LinksLLDP)
	require.Equal(t, 1, stats.LinksBidirectional)
	require.Equal(t, 0, stats.LinksUnidirectional)
}

func TestToGraph_MergesPairedAdjacenciesPreservesRawAddressHints(t *testing.T) {
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
					adjacencyLabelPairPass: cdpMatchPassDefault,
					"remote_address_raw":   "10.0.0.1",
				},
			},
		},
	}

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "cdp", link.Protocol)
	require.Equal(t, "bidirectional", link.Direction)
	require.Contains(t, link.Dst.Match.IPAddresses, "edge-sw3.mgmt.local")
	require.Contains(t, link.Src.Match.IPAddresses, "10.0.0.1")
}

func TestToGraph_MergesReversePairsWithoutDirectionalPairLabels(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "router-a",
				Hostname:  "MikroTik-router",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "XS1930",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "router-a", IfIndex: 3, IfName: "ether3", IfDescr: "ether3"},
			{DeviceID: "switch-b", IfIndex: 8, IfName: "swp07", IfDescr: "swp07"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "router-a",
				SourcePort: "ether3",
				TargetID:   "switch-b",
				TargetPort: "",
				Labels: map[string]string{
					adjacencyLabelPairID:   "lldp:pair-router-xs",
					adjacencyLabelPairPass: lldpMatchPassPortDesc,
				},
			},
			{
				Protocol:   "lldp",
				SourceID:   "switch-b",
				SourcePort: "8",
				TargetID:   "router-a",
				TargetPort: "ether3",
				Labels: map[string]string{
					adjacencyLabelPairID:   "lldp:pair-router-xs",
					adjacencyLabelPairPass: lldpMatchPassPortDesc,
				},
			},
		},
	}

	data, stats := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Equal(t, 1, stats.LinksTotal)
	require.Equal(t, 1, stats.LinksLLDP)
	require.Equal(t, 1, stats.LinksBidirectional)
	require.Equal(t, 0, stats.LinksUnidirectional)
	require.Len(t, data.Links, 1)

	link := data.Links[0]
	require.Equal(t, "lldp", link.Protocol)
	require.Equal(t, "bidirectional", link.Direction)
	require.Equal(t, "MikroTik-router", link.Src.SysName)
	require.Equal(t, "XS1930", link.Dst.SysName)
	require.Equal(t, "ether3", link.Src.IfName)
	require.Equal(t, "swp07", link.Dst.IfName)
	require.Equal(t, "8", link.Dst.PortID)
	require.Equal(t, "swp07", link.Dst.PortName)
}

func TestToGraph_UnknownAdjacencyPortsRemainUnsetWithoutZeroFallback(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "router-a",
				Hostname:  "MikroTik-router",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "switch-b",
				Hostname:  "XS1930",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "router-a", IfIndex: 3, IfName: "ether3", IfDescr: "ether3"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "router-a",
				SourcePort: "ether3",
				TargetID:   "switch-b",
				TargetPort: "",
			},
		},
	}

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Links, 1)
	link := data.Links[0]
	require.Equal(t, "lldp", link.Protocol)
	require.Equal(t, "unidirectional", link.Direction)
	require.Empty(t, link.Dst.PortName)
	require.NotNil(t, link.Display)
	require.Empty(t, strings.TrimSpace(link.Display.DstPortName))
	require.Contains(t, link.Display.Name, ":[unset]")
}

func TestToGraph_DropsAmbiguousEndpointSegmentLinks(t *testing.T) {
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

	data, stats := toGraphForTest(result, GraphOptions{
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
	require.Equal(t, 0, stats.LinksTotal)
	require.Equal(t, 0, stats.LinksFDB)
	require.Equal(t, 2, stats.LinksFDBEndpointCandidates)
	require.Equal(t, 0, stats.LinksFDBEndpointEmitted)
	require.Equal(t, 2, stats.LinksFDBEndpointSuppressed)
	require.Equal(t, 1, stats.EndpointsAmbiguousSegments)
	require.Equal(t, 2, stats.SegmentsSuppressed)
}

func TestToGraph_ProbableConnectivityConnectsAmbiguousEndpoint(t *testing.T) {
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

	strictData, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Len(t, findFDBLinksByEndpointMAC(strictData.Links, "70:49:a2:65:72:cd"), 0)

	data, stats := toGraphForTest(result, GraphOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointMAC(data.Links, "70:49:a2:65:72:cd")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(fdbLinks[0].State)))
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(topologyLinkInference(fdbLinks[0]))))
	require.Equal(t, "probable_segment", topologyLinkAttachmentMode(fdbLinks[0]))
	require.Equal(t, "low", topologyLinkConfidence(fdbLinks[0]))

	require.Equal(t, 1, stats.LinksProbable)
	require.Equal(t, 1, stats.LinksFDBEndpointEmitted)
	require.Equal(t, 1, stats.LinksFDBEndpointSuppressed)
}

func TestToGraph_ProbableConnectivityDoesNotReclassifyStrictSinglePortEndpoint(t *testing.T) {
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
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
		},
	}

	strictData, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	strictFDBLinks := findFDBLinksByEndpointMAC(strictData.Links, "dd:dd:dd:dd:dd:dd")
	require.Len(t, strictFDBLinks, 1)
	require.Equal(t, "", strings.TrimSpace(strictFDBLinks[0].State))
	require.Equal(t, "", strings.TrimSpace(topologyLinkInference(strictFDBLinks[0])))

	data, stats := toGraphForTest(result, GraphOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointMAC(data.Links, "dd:dd:dd:dd:dd:dd")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "", strings.TrimSpace(fdbLinks[0].State))
	require.Equal(t, "", strings.TrimSpace(topologyLinkInference(fdbLinks[0])))
	require.Equal(t, "direct", topologyLinkAttachmentMode(fdbLinks[0]))
	require.Equal(t, 0, stats.LinksProbable)
}

func TestToGraph_ProbableConnectivityConnectsUnlinkedLLDPEndpoint(t *testing.T) {
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
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:70:49:a2:65:72:cf", Method: "lldp"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:70:49:a2:65:72:cf", Method: "lldp"},
		},
	}

	strictData, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Len(t, findFDBLinksByEndpointMAC(strictData.Links, "70:49:a2:65:72:cf"), 0)

	data, _ := toGraphForTest(result, GraphOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointMAC(data.Links, "70:49:a2:65:72:cf")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(fdbLinks[0].State)))
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(topologyLinkInference(fdbLinks[0]))))
	require.Equal(t, "probable_segment", topologyLinkAttachmentMode(fdbLinks[0]))
}

func TestToGraph_ProbableConnectivityAvoidsExtraBridgePathForLLDPPeers(t *testing.T) {
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
			{DeviceID: "switch-a", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-b", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "switch-b",
				TargetPort: "Gi0/1",
			},
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/2",
				TargetID:   "switch-b",
				TargetPort: "Gi0/2",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:70:49:a2:65:72:aa", Method: "lldp"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:70:49:a2:65:72:aa", Method: "lldp"},
			{DeviceID: "switch-a", IfIndex: 2, EndpointID: "mac:70:49:a2:65:72:aa", Method: "lldp"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:70:49:a2:65:72:aa", Method: "lldp"},
		},
	}

	strictData, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Len(t, findFDBLinksByEndpointMAC(strictData.Links, "70:49:a2:65:72:aa"), 0)

	data, _ := toGraphForTest(result, GraphOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointMAC(data.Links, "70:49:a2:65:72:aa")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(fdbLinks[0].State)))

	bridgeLinksBySegment := make(map[string]map[string]struct{})
	for _, link := range data.Links {
		if !strings.EqualFold(strings.TrimSpace(link.Protocol), "bridge") {
			continue
		}
		segmentActorID := strings.TrimSpace(link.DstActorID)
		if segmentActorID == "" {
			continue
		}
		devices := bridgeLinksBySegment[segmentActorID]
		if devices == nil {
			devices = make(map[string]struct{})
			bridgeLinksBySegment[segmentActorID] = devices
		}
		devices[strings.TrimSpace(link.SrcActorID)] = struct{}{}
	}
	for _, devices := range bridgeLinksBySegment {
		require.LessOrEqual(t, len(devices), 1)
	}
}

func TestToGraph_ProbableConnectivityConnectsZeroCandidateEndpointUsingReporterHints(t *testing.T) {
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
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "ip:10.0.0.99",
				IPs:        []netip.Addr{netip.MustParseAddr("10.0.0.99")},
				Labels: map[string]string{
					"sources":    "arp",
					"device_ids": "switch-a",
					"if_indexes": "1",
					"if_names":   "Gi0/1",
				},
			},
		},
	}

	strictData, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Len(t, findFDBLinksByEndpointIP(strictData.Links, "10.0.0.99"), 0)

	data, _ := toGraphForTest(result, GraphOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointIP(data.Links, "10.0.0.99")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(fdbLinks[0].State)))
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(topologyLinkInference(fdbLinks[0]))))
	require.Equal(t, "probable_segment", topologyLinkAttachmentMode(fdbLinks[0]))
	require.Equal(t, "low", topologyLinkConfidence(fdbLinks[0]))

	strictSignatures := topologyLinkSignatures(strictData.Links)
	probableSignatures := topologyLinkSignatures(data.Links)
	for signature := range strictSignatures {
		_, ok := probableSignatures[signature]
		require.Truef(t, ok, "strict link signature missing in probable output: %s", signature)
	}
}

func TestToGraph_ProbableConnectivityCreatesPortlessAttachmentForZeroCandidateEndpoint(t *testing.T) {
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
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "ip:10.0.0.199",
				IPs:        []netip.Addr{netip.MustParseAddr("10.0.0.199")},
				Labels: map[string]string{
					"sources":    "arp",
					"device_ids": "switch-a",
					"if_indexes": "999",
					"if_names":   "Gi0/999",
				},
			},
		},
	}

	data, _ := toGraphForTest(result, GraphOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointIP(data.Links, "10.0.0.199")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(fdbLinks[0].State)))
	require.Equal(t, "probable_portless", topologyLinkAttachmentMode(fdbLinks[0]))

	segmentActor := findActorByMatch(data.Actors, fdbLinks[0].Src.Match)
	require.NotNil(t, segmentActor)
	segmentDetail := requireActorDetail(t, data, segmentActor)
	require.Contains(t, segmentDetail.Segment.SegmentID, "bridge-domain:probable:")
	require.Equal(t, []string{"switch-a"}, segmentDetail.Segment.ParentDevices)

	bridgeCount := 0
	for _, link := range data.Links {
		if !strings.EqualFold(strings.TrimSpace(link.Protocol), "bridge") {
			continue
		}
		if canonicalTopologyMatchKey(link.Dst.Match) != canonicalTopologyMatchKey(fdbLinks[0].Src.Match) {
			continue
		}
		bridgeCount++
	}
	require.Equal(t, 1, bridgeCount)
}

func TestToGraph_InferenceStrategy_STPParentDoesNotSuppressFDBEndpointOwnership(t *testing.T) {
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
				Protocol:   "stp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "switch-b",
				TargetPort: "Gi0/2",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:00:00:00:00:00:11", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 2, EndpointID: "mac:00:00:00:00:00:22", Method: "fdb"},
		},
	}

	_, baselineStats := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Equal(t, topologyInferenceStrategyFDBMinimumKnowledge, baselineStats.InferenceStrategy)
	require.Greater(t, baselineStats.LinksFDBEndpointEmitted, 0)

	_, stpStats := toGraphForTest(result, GraphOptions{
		Source:            "snmp",
		Layer:             "2",
		View:              "summary",
		InferenceStrategy: topologyInferenceStrategySTPParentTree,
	})
	require.Equal(t, topologyInferenceStrategySTPParentTree, stpStats.InferenceStrategy)
	require.Greater(t, stpStats.LinksFDBEndpointEmitted, 0)
}

func TestToGraph_InferenceStrategy_CDPHybridPrefersCDPBridgeLinks(t *testing.T) {
	result := Result{
		Devices: []Device{
			{ID: "sw-a", Hostname: "sw-a", ChassisID: "00:00:00:00:00:aa"},
			{ID: "sw-b", Hostname: "sw-b", ChassisID: "00:00:00:00:00:bb"},
			{ID: "sw-c", Hostname: "sw-c", ChassisID: "00:00:00:00:00:cc"},
		},
		Interfaces: []Interface{
			{DeviceID: "sw-a", IfIndex: 1, IfName: "Gi0/1"},
			{DeviceID: "sw-a", IfIndex: 2, IfName: "Gi0/2"},
			{DeviceID: "sw-b", IfIndex: 1, IfName: "Gi0/1"},
			{DeviceID: "sw-c", IfIndex: 1, IfName: "Gi0/1"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "sw-a",
				SourcePort: "Gi0/1",
				TargetID:   "sw-b",
				TargetPort: "Gi0/1",
			},
			{
				Protocol:   "cdp",
				SourceID:   "sw-a",
				SourcePort: "Gi0/2",
				TargetID:   "sw-c",
				TargetPort: "Gi0/1",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "sw-a", IfIndex: 1, EndpointID: "mac:00:00:00:00:10:01", Method: "fdb"},
			{DeviceID: "sw-a", IfIndex: 2, EndpointID: "mac:00:00:00:00:10:02", Method: "fdb"},
		},
	}

	_, baselineStats := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Equal(t, topologyInferenceStrategyFDBMinimumKnowledge, baselineStats.InferenceStrategy)
	require.Equal(t, 0, baselineStats.LinksFDBEndpointEmitted)

	_, stats := toGraphForTest(result, GraphOptions{
		Source:            "snmp",
		Layer:             "2",
		View:              "summary",
		InferenceStrategy: topologyInferenceStrategyCDPFDBHybrid,
	})

	require.Equal(t, topologyInferenceStrategyCDPFDBHybrid, stats.InferenceStrategy)
	require.Equal(t, 1, stats.LinksCDP)
	require.Equal(t, 0, stats.LinksFDBEndpointEmitted)
}

func TestPickProbableSegmentAnchorPortID_PrefersManagedPortWhenOwnerPointsToUnmanaged(t *testing.T) {
	unmanagedPort := bridgePortRef{
		deviceID: "ghost-switch",
		ifIndex:  900,
		ifName:   "ghost0",
	}
	managedPort := bridgePortRef{
		deviceID: "managed-switch",
		ifIndex:  7,
		ifName:   "swp06",
	}

	segment := newBridgeDomainSegment(unmanagedPort)
	segment.addPort(managedPort)

	endpointID := "mac:50:2c:c6:a6:fc:35"
	owner := fdbEndpointOwner{
		portKey:     bridgePortObservationKey(unmanagedPort),
		portVLANKey: bridgePortObservationVLANKey(unmanagedPort),
		port:        unmanagedPort,
		source:      "single_port_mac",
	}

	picked := pickProbableSegmentAnchorPortID(
		segment,
		map[string]struct{}{endpointID: {}},
		map[string]fdbEndpointOwner{endpointID: owner},
		map[string]struct{}{"managed-switch": {}},
	)

	require.NotEmpty(t, picked)
	require.Equal(t, bridgePortRefSortKey(managedPort), bridgePortRefSortKey(segment.ports[picked]))
}

func TestSelectProbableEndpointReporterHint_PrefersManagedHintsOverUnmanagedOwner(t *testing.T) {
	endpointLabels := map[string]string{
		"learned_device_ids": "ghost-switch,managed-switch",
		"learned_if_indexes": "7",
		"learned_if_names":   "swp06",
	}
	reporterHints := map[string][]bridgePortRef{
		"ghost-switch": {
			{deviceID: "ghost-switch", ifIndex: 900, ifName: "ghost0"},
		},
		"managed-switch": {
			{deviceID: "managed-switch", ifIndex: 7, ifName: "swp06"},
		},
	}
	owner := fdbEndpointOwner{
		port: bridgePortRef{
			deviceID: "ghost-switch",
			ifIndex:  900,
			ifName:   "ghost0",
		},
		source: "single_port_mac",
	}

	hint := selectProbableEndpointReporterHint(
		endpointLabels,
		reporterHints,
		owner,
		nil,
		map[string]struct{}{"managed-switch": {}},
	)

	require.Equal(t, "managed-switch", hint.deviceID)
	require.Equal(t, 7, hint.ifIndex)
	require.Equal(t, "swp06", hint.ifName)
}

func TestProbableCandidateSegmentsFromReporterHints_PrefersManagedReporterSegments(t *testing.T) {
	index := segmentReporterIndex{
		byDevice: map[string]map[string]struct{}{
			"ghost-switch":   {"segment:ghost": {}},
			"managed-switch": {"segment:managed": {}},
		},
		byDeviceIfIndex: map[string]map[string]struct{}{
			"ghost-switch\x007":   {"segment:ghost": {}},
			"managed-switch\x007": {"segment:managed": {}},
		},
		byDeviceIfName: map[string]map[string]struct{}{
			"ghost-switch\x00swp06":   {"segment:ghost": {}},
			"managed-switch\x00swp06": {"segment:managed": {}},
		},
	}

	segments := probableCandidateSegmentsFromReporterHints(
		map[string]string{
			"learned_device_ids": "ghost-switch,managed-switch",
			"learned_if_indexes": "7",
			"learned_if_names":   "swp06",
		},
		nil,
		index,
		nil,
		map[string]struct{}{"managed-switch": {}},
	)

	require.Equal(t, []string{"segment:managed"}, segments)
}

func TestEnsureManagedProbableReporterHint_UpgradesUnmanagedHint(t *testing.T) {
	hint := probableEndpointReporterHint{
		deviceID: "ghost-switch",
	}
	endpointLabels := map[string]string{
		"learned_device_ids": "macAddress:18:fd:74:7e:c5:80",
		"learned_if_indexes": "7",
		"learned_if_names":   "swp06",
	}
	aliasOwnerIDs := map[string]map[string]struct{}{
		"mac:18:fd:74:7e:c5:80": {
			"managed-switch": {},
		},
	}

	updated := ensureManagedProbableReporterHint(
		hint,
		endpointLabels,
		nil,
		aliasOwnerIDs,
		map[string]struct{}{"managed-switch": {}},
		[]string{"managed-switch"},
	)

	require.Equal(t, "managed-switch", updated.deviceID)
	require.Equal(t, 7, updated.ifIndex)
	require.Equal(t, "swp06", updated.ifName)
}

func TestEnsureManagedProbableReporterHint_FallsBackToFirstManagedDevice(t *testing.T) {
	updated := ensureManagedProbableReporterHint(
		probableEndpointReporterHint{deviceID: "ghost-switch"},
		nil,
		nil,
		nil,
		map[string]struct{}{"managed-a": {}, "managed-b": {}},
		[]string{"managed-a", "managed-b"},
	)

	require.Equal(t, "managed-a", updated.deviceID)
	require.Equal(t, 0, updated.ifIndex)
	require.Equal(t, "0", updated.ifName)
}

func TestToGraph_ProbableConnectivityRecoversUnmanagedOverlapSuppression(t *testing.T) {
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
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-a", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "remote-peer",
				TargetPort: "cc:cc:cc:cc:cc:cc",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 2, EndpointID: "mac:cc:cc:cc:cc:cc:cc", Method: "fdb"},
		},
	}

	strictData, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.Len(t, findFDBLinksByEndpointMAC(strictData.Links, "cc:cc:cc:cc:cc:cc"), 0)

	data, _ := toGraphForTest(result, GraphOptions{
		Source:                    "snmp",
		Layer:                     "2",
		View:                      "summary",
		ProbabilisticConnectivity: true,
	})

	fdbLinks := findFDBLinksByEndpointMAC(data.Links, "cc:cc:cc:cc:cc:cc")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(fdbLinks[0].State)))
	require.Equal(t, "probable", strings.ToLower(strings.TrimSpace(topologyLinkInference(fdbLinks[0]))))
	require.Equal(t, "probable_direct", topologyLinkAttachmentMode(fdbLinks[0]))
	require.Equal(t, "low", topologyLinkConfidence(fdbLinks[0]))
}

func TestToGraph_CollapseByIPPrunesSuppressedManagedOverlapEndpoint(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "nova",
				Hostname:  "nova",
				ChassisID: "9c:6b:00:7b:98:c6",
				Addresses: []netip.Addr{netip.MustParseAddr("172.22.0.1")},
				Labels:    map[string]string{"inferred": "true"},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			{DeviceID: "switch-a", IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "nova",
				TargetPort: "9c:6b:00:7b:98:c7",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 2, EndpointID: "mac:9c:6b:00:7b:98:c7", Method: "fdb"},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "mac:9c:6b:00:7b:98:c7",
				MAC:        "9c:6b:00:7b:98:c7",
				IPs:        []netip.Addr{netip.MustParseAddr("10.20.4.22")},
				Labels: map[string]string{
					"sources": "arp",
				},
			},
		},
	}

	withoutCollapse, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})
	require.NotNil(t, findActorByMAC(withoutCollapse.Actors, "9c:6b:00:7b:98:c7"))

	withCollapse, withCollapseStats := toGraphForTest(result, GraphOptions{
		Source:             "snmp",
		Layer:              "2",
		View:               "summary",
		CollapseActorsByIP: true,
	})
	require.NotNil(t, findActorByMAC(withCollapse.Actors, "9c:6b:00:7b:98:c6"))
	require.Nil(t, findActorByMAC(withCollapse.Actors, "9c:6b:00:7b:98:c7"))
	require.Equal(t, 1, withCollapseStats.ActorsUnlinkedSuppressed)
}

func TestToGraph_ReplacesKnownDeviceEndpointWithManagedDeviceEdge(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "router-a",
				Hostname:  "router-a",
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
			{DeviceID: "router-a", IfIndex: 1, IfName: "ether1"},
		},
		Attachments: []Attachment{
			{DeviceID: "router-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
		},
	}

	data, stats := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	fdbLinks := findFDBLinksByDstSysName(data.Links, "switch-b")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "managed_device_overlap", topologyLinkAttachmentMode(fdbLinks[0]))
	require.Equal(t, 1, stats.LinksFDBEndpointEmitted)
	require.Equal(t, 0, stats.LinksFDBEndpointSuppressed)

	for _, actor := range data.Actors {
		if actor.ActorType != "endpoint" {
			continue
		}
		for _, mac := range actor.Match.MacAddresses {
			require.NotEqual(t, "bb:bb:bb:bb:bb:bb", mac)
		}
	}
}

func TestToGraph_KnownDeviceOverlapUsesInterfaceMACAlias(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "router-a",
				Hostname:  "router-a",
				ChassisID: "aa:aa:aa:aa:aa:80",
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
			{DeviceID: "router-a", IfIndex: 1, IfName: "ether1", MAC: "aa:aa:aa:aa:aa:8c"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "ether1"},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:8c", Method: "fdb"},
		},
	}

	data, stats := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	fdbLinks := findFDBLinksByDstSysName(data.Links, "router-a")
	require.Len(t, fdbLinks, 1)
	require.Equal(t, "managed_device_overlap", topologyLinkAttachmentMode(fdbLinks[0]))
	require.Equal(t, 1, stats.LinksFDBEndpointEmitted)
	require.Equal(t, 0, stats.LinksFDBEndpointSuppressed)
}

func TestToGraph_DeviceActorIncludesInterfaceMACAliases(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:01",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", MAC: "aa:aa:aa:aa:aa:11"},
			{DeviceID: "switch-a", IfIndex: 2, IfName: "Gi0/2", MAC: "aa:aa:aa:aa:aa:12"},
			{DeviceID: "switch-a", IfIndex: 3, IfName: "Gi0/3", MAC: "AA-AA-AA-AA-AA-12"},
		},
	}

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	actor := findActorBySysName(data.Actors, "switch-a")
	require.NotNil(t, actor)
	require.ElementsMatch(
		t,
		[]string{"aa:aa:aa:aa:aa:01", "aa:aa:aa:aa:aa:11", "aa:aa:aa:aa:aa:12"},
		actor.Match.MacAddresses,
	)
}

func TestToGraph_KeepsUnlinkedEndpointsAndDevices(t *testing.T) {
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

	data, stats := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.Len(t, data.Actors, 2)
	deviceCount := 0
	endpointCount := 0
	for _, actor := range data.Actors {
		switch actor.ActorType {
		case "device":
			deviceCount++
		case "endpoint":
			endpointCount++
		}
	}
	require.Equal(t, 1, deviceCount)
	require.Equal(t, 1, endpointCount)
	require.Equal(t, 0, stats.LinksTotal)
	require.Equal(t, 0, stats.ActorsUnlinkedSuppressed)
}

func TestToGraph_KeepsUnlinkedEndpointWhenIdentityOverlapsLinkedDevice(t *testing.T) {
	result := Result{
		Devices: []Device{
			{
				ID:        "switch-a",
				Hostname:  "switch-a",
				ChassisID: "aa:aa:aa:aa:aa:aa",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.1")},
			},
			{
				ID:        "mega",
				Hostname:  "mega",
				ChassisID: "bb:bb:bb:bb:bb:bb",
				Addresses: []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1"},
			{DeviceID: "mega", IfIndex: 1, IfName: "eth0"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "Gi0/1",
				TargetID:   "mega",
				TargetPort: "eth0",
			},
		},
		Enrichments: []Enrichment{
			{
				EndpointID: "mac:cc:cc:cc:cc:cc:cc",
				IPs:        []netip.Addr{netip.MustParseAddr("10.0.0.2")},
			},
		},
	}

	data, stats := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	require.NotNil(t, findActorByMAC(data.Actors, "cc:cc:cc:cc:cc:cc"))
	require.Equal(t, 3, stats.ActorsTotal)
	require.Equal(t, 1, stats.LinksTotal)
	require.Equal(t, 0, stats.ActorsUnlinkedSuppressed)
}

func TestToGraph_DisplayNamesPreferDNSThenIPThenMAC(t *testing.T) {
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

	data, _ := toGraphForTest(result, GraphOptions{
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
	deviceDetail := requireActorDetail(t, data, device)
	require.Equal(t, "switch-a.example.net", deviceDetail.DisplayName)
	require.Equal(t, "dns", deviceDetail.DisplaySource)

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
		require.NotEmpty(t, link.Src.DisplayName)
		require.NotEmpty(t, link.Dst.DisplayName)
	}
}

func TestTopologyDisplayNameFromMatch_PrefersSysNameBeforeIP(t *testing.T) {
	display := topologyDisplayNameFromMatch(graph.Match{
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
	display := topologyDisplayNameFromMatch(graph.Match{
		Hostnames:   []string{"nova"},
		IPAddresses: []string{"10.20.4.22"},
	}, &topologyDisplayNameResolver{
		lookup: func(string) string { return "" },
		cache:  map[string]string{},
	})

	require.Equal(t, "nova", display.name)
	require.Equal(t, "hostname", display.source)
}

func TestToGraph_SegmentDisplayNameUsesParentPortPattern(t *testing.T) {
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

	data, _ := toGraphForTest(result, GraphOptions{
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
	segmentDetail := requireActorDetail(t, data, segment)
	require.Equal(t, "switch-a.example.net.gi0/3.segment", segmentDetail.DisplayName)
	require.Equal(t, "segment", segmentDetail.DisplaySource)
}

func TestToGraph_FDBOwnerInferencePrefersNonLLDPSide(t *testing.T) {
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

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	targetLinks := findFDBLinksByEndpointMAC(data.Links, "70:49:a2:65:72:cd")
	require.Len(t, targetLinks, 1)

	segmentActor := findActorByMatch(data.Actors, targetLinks[0].Src.Match)
	require.NotNil(t, segmentActor)
	segmentDetail := requireActorDetail(t, data, segmentActor)
	require.Equal(t, []string{"switch-b"}, segmentDetail.Segment.ParentDevices)
	require.Equal(t, []string{"Gi0/2"}, segmentDetail.Segment.IfNames)
}

func TestToGraph_FDBOwnerInferenceUsesSingleMACPortRule(t *testing.T) {
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

	data, _ := toGraphForTest(result, GraphOptions{
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
	endpointDetail := requireActorDetail(t, data, endpointActor)
	require.Equal(t, "single_port_mac", endpointDetail.Endpoint.AttachmentSource)
	require.Equal(t, "switch-a", endpointDetail.Endpoint.AttachedDevice)
	require.Equal(t, "Gi0/1", endpointDetail.Endpoint.AttachedPort)
	require.Equal(t, "single_port_mac", endpointDetail.Endpoint.AttachedBy)
}

func TestToGraph_FDBOwnerInferenceSuppressesManagedAliasSwitchFacingPorts(t *testing.T) {
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
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1"},
			{DeviceID: "switch-a", IfIndex: 2, IfName: "Gi0/2"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "Gi0/1"},
		},
		Attachments: []Attachment{
			// Managed-device aliases learned on the same port mark it as switch-facing.
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},

			// Candidate endpoint appears behind the managed-link port and must be suppressed.
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:dd:dd:dd:dd:dd:dd", Method: "fdb"},

			// Control endpoint on non-switch-facing port remains directly attachable.
			{DeviceID: "switch-a", IfIndex: 2, EndpointID: "mac:ee:ee:ee:ee:ee:ee", Method: "fdb"},
		},
	}

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	ddLinks := findFDBLinksByEndpointMAC(data.Links, "dd:dd:dd:dd:dd:dd")
	require.Len(t, ddLinks, 0)
	ddActor := findActorByMAC(data.Actors, "dd:dd:dd:dd:dd:dd")
	require.NotNil(t, ddActor)
	ddDetail := requireActorDetail(t, data, ddActor)
	require.Empty(t, ddDetail.Endpoint.AttachmentSource)
	require.Empty(t, ddDetail.Endpoint.AttachedDevice)

	eeLinks := findFDBLinksByEndpointMAC(data.Links, "ee:ee:ee:ee:ee:ee")
	require.Len(t, eeLinks, 1)
	eeSrc := findActorByMatch(data.Actors, eeLinks[0].Src.Match)
	require.NotNil(t, eeSrc)
	require.Equal(t, "device", eeSrc.ActorType)
	require.Equal(t, "switch-a", eeSrc.Match.SysName)
}

func TestToGraph_SuppressesFDBEndpointsOnLLDPPorts(t *testing.T) {
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

	data, stats := toGraphForTest(result, GraphOptions{
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
	require.Equal(t, 1, stats.LinksTotal)
	require.Equal(t, 0, stats.LinksFDB)
	require.Equal(t, 0, stats.LinksFDBEndpointCandidates)
	require.Equal(t, 0, stats.LinksFDBEndpointEmitted)
	require.Equal(t, 0, stats.LinksFDBEndpointSuppressed)
}

func TestToGraph_KeepsChassisPlaceholderDevicesAsDevices(t *testing.T) {
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

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	placeholder := findActorBySysName(data.Actors, "chassis-788cb595dfcc")
	require.NotNil(t, placeholder)
	require.Equal(t, "device", placeholder.ActorType)
}

func TestPruneSegmentArtifacts_SuppressesLLDPDuplicateSegmentPath(t *testing.T) {
	actors := []projectedActor{
		{
			Actor: graph.Actor{
				ActorType: "device",
				Match:     graph.Match{IPAddresses: []string{"10.0.0.1"}, SysName: "switch-a"},
			},
		},
		{
			Actor: graph.Actor{
				ActorType: "device",
				Match:     graph.Match{IPAddresses: []string{"10.0.0.2"}, SysName: "switch-b"},
			},
		},
		{
			Actor: graph.Actor{
				ActorType: "segment",
				Match:     graph.Match{Hostnames: []string{"segment:dup"}},
			},
		},
	}

	links := []graph.Link{
		{
			Protocol: "lldp",
			Src:      graph.LinkEndpoint{Match: graph.Match{IPAddresses: []string{"10.0.0.1"}}},
			Dst:      graph.LinkEndpoint{Match: graph.Match{IPAddresses: []string{"10.0.0.2"}}},
		},
		{
			Protocol: "bridge",
			Src:      graph.LinkEndpoint{Match: graph.Match{IPAddresses: []string{"10.0.0.1"}}},
			Dst:      graph.LinkEndpoint{Match: graph.Match{Hostnames: []string{"segment:dup"}}},
		},
		{
			Protocol: "bridge",
			Src:      graph.LinkEndpoint{Match: graph.Match{Hostnames: []string{"segment:dup"}}},
			Dst:      graph.LinkEndpoint{Match: graph.Match{IPAddresses: []string{"10.0.0.2"}}},
		},
	}

	filteredActors, filteredLinks, suppressed := pruneSegmentArtifacts(actors, links)
	require.Equal(t, 1, suppressed)
	require.Len(t, filteredActors, 2)
	require.Len(t, filteredLinks, 1)
	require.Equal(t, "lldp", filteredLinks[0].Protocol)
}

func TestPruneSegmentArtifacts_SuppressesCDPDuplicateSegmentPath(t *testing.T) {
	actors := []projectedActor{
		{
			Actor: graph.Actor{
				ActorType: "device",
				Match:     graph.Match{IPAddresses: []string{"10.0.1.1"}, SysName: "switch-a"},
			},
		},
		{
			Actor: graph.Actor{
				ActorType: "device",
				Match:     graph.Match{IPAddresses: []string{"10.0.1.2"}, SysName: "switch-b"},
			},
		},
		{
			Actor: graph.Actor{
				ActorType: "segment",
				Match:     graph.Match{Hostnames: []string{"segment:dup-cdp"}},
			},
		},
	}

	links := []graph.Link{
		{
			Protocol: "cdp",
			Src:      graph.LinkEndpoint{Match: graph.Match{IPAddresses: []string{"10.0.1.1"}}},
			Dst:      graph.LinkEndpoint{Match: graph.Match{IPAddresses: []string{"10.0.1.2"}}},
		},
		{
			Protocol: "bridge",
			Src:      graph.LinkEndpoint{Match: graph.Match{IPAddresses: []string{"10.0.1.1"}}},
			Dst:      graph.LinkEndpoint{Match: graph.Match{Hostnames: []string{"segment:dup-cdp"}}},
		},
		{
			Protocol: "bridge",
			Src:      graph.LinkEndpoint{Match: graph.Match{Hostnames: []string{"segment:dup-cdp"}}},
			Dst:      graph.LinkEndpoint{Match: graph.Match{IPAddresses: []string{"10.0.1.2"}}},
		},
	}

	filteredActors, filteredLinks, suppressed := pruneSegmentArtifacts(actors, links)
	require.Equal(t, 1, suppressed)
	require.Len(t, filteredActors, 2)
	require.Len(t, filteredLinks, 1)
	require.Equal(t, "cdp", filteredLinks[0].Protocol)
}

func TestPruneSegmentArtifacts_SuppressesSegmentsWithSingleNeighbor(t *testing.T) {
	actors := []projectedActor{
		{
			Actor: graph.Actor{
				ActorType: "device",
				Match:     graph.Match{IPAddresses: []string{"10.0.0.1"}, SysName: "router-a"},
			},
		},
		{
			Actor: graph.Actor{
				ActorType: "segment",
				Match:     graph.Match{Hostnames: []string{"segment:orphan"}},
			},
		},
	}

	links := []graph.Link{
		{
			Protocol: "bridge",
			Src:      graph.LinkEndpoint{Match: graph.Match{IPAddresses: []string{"10.0.0.1"}}},
			Dst:      graph.LinkEndpoint{Match: graph.Match{Hostnames: []string{"segment:orphan"}}},
		},
	}

	filteredActors, filteredLinks, suppressed := pruneSegmentArtifacts(actors, links)
	require.Equal(t, 1, suppressed)
	require.Len(t, filteredActors, 1)
	require.Len(t, filteredLinks, 0)
}

func TestToGraph_DeterministicTransitRuleSuppressesFDBOnLLDPPortInExperimental(t *testing.T) {
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
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:00:00:00:00:00:11", Method: "fdb"},
		},
	}

	data, stats := toGraphForTest(result, GraphOptions{
		Source:            "snmp",
		Layer:             "2",
		View:              "summary",
		InferenceStrategy: topologyInferenceStrategyFDBMinimumKnowledge,
	})

	require.Equal(t, topologyInferenceStrategyFDBMinimumKnowledge, stats.InferenceStrategy)
	require.Equal(t, 0, stats.LinksFDBEndpointEmitted)
	require.Nil(t, findActorByType(data.Actors, "segment"))
}

func TestToGraph_DeterministicTransitRuleMatchesNumericLLDPPortToIfIndex(t *testing.T) {
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
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 2, IfName: "GigabitEthernet2", IfDescr: "GigabitEthernet2"},
			{DeviceID: "switch-b", IfIndex: 4, IfName: "ether4", IfDescr: "ether4"},
		},
		Adjacencies: []Adjacency{
			{
				Protocol:   "lldp",
				SourceID:   "switch-a",
				SourcePort: "2",
				TargetID:   "switch-b",
				TargetPort: "ether4",
			},
		},
		Attachments: []Attachment{
			{DeviceID: "switch-a", IfIndex: 2, EndpointID: "mac:00:00:00:00:00:11", Method: "fdb"},
		},
	}

	data, stats := toGraphForTest(result, GraphOptions{
		Source:            "snmp",
		Layer:             "2",
		View:              "summary",
		InferenceStrategy: topologyInferenceStrategyFDBMinimumKnowledge,
	})

	require.Equal(t, 0, stats.LinksFDBEndpointEmitted)
	require.Nil(t, findActorByType(data.Actors, "segment"))
	lldpLink := findLinkByProtocol(data.Links, "lldp")
	require.NotNil(t, lldpLink)
	require.Equal(t, 2, lldpLink.Src.IfIndex)
	require.Equal(t, "GigabitEthernet2", lldpLink.Src.IfName)
	require.Equal(t, "2", lldpLink.Src.PortID)
	require.Equal(t, 4, lldpLink.Dst.IfIndex)
	require.Equal(t, "ether4", lldpLink.Dst.IfName)
	require.Equal(t, "ether4", lldpLink.Dst.PortID)
}

func TestToGraph_SwitchFacingPortDoesNotSuppressEndpointOwnership(t *testing.T) {
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
		},
		Interfaces: []Interface{
			{DeviceID: "switch-a", IfIndex: 1, IfName: "Gi0/1", MAC: "aa:aa:aa:aa:aa:aa"},
			{DeviceID: "switch-b", IfIndex: 1, IfName: "Gi0/1", MAC: "bb:bb:bb:bb:bb:bb"},
		},
		Attachments: []Attachment{
			// Reciprocal managed-alias observations make this a switch-facing bridge pair.
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:bb:bb:bb:bb:bb:bb", Method: "fdb"},
			{DeviceID: "switch-b", IfIndex: 1, EndpointID: "mac:aa:aa:aa:aa:aa:aa", Method: "fdb"},
			// Host endpoint learned on the same switch-facing port.
			{DeviceID: "switch-a", IfIndex: 1, EndpointID: "mac:00:00:00:00:00:11", Method: "fdb"},
		},
	}

	data, _ := toGraphForTest(result, GraphOptions{
		Source:            "snmp",
		Layer:             "2",
		View:              "summary",
		InferenceStrategy: topologyInferenceStrategyFDBPairwise,
	})

	actor := findActorBySysName(data.Actors, "switch-a")
	require.NotNil(t, actor)

	detail := requireActorDetail(t, data, actor)
	port1 := findInterfaceStatusByIndex(detail.Device.Ports, 1)
	require.NotNil(t, port1)
	require.Equal(t, "switch_facing", port1.TopologyRole)

	targetLinks := findFDBLinksByEndpointMAC(data.Links, "00:00:00:00:00:11")
	require.Len(t, targetLinks, 1)
	segmentActor := findActorByMatch(data.Actors, targetLinks[0].Src.Match)
	require.NotNil(t, segmentActor)
	segmentDetail := requireActorDetail(t, data, segmentActor)
	require.Contains(t, segmentDetail.Segment.ParentDevices, "switch-a")
	require.Contains(t, segmentDetail.Segment.IfNames, "Gi0/1")
}

func TestSuppressInferredBridgeLinksOnDeterministicDiscovery(t *testing.T) {
	deterministic := make(map[string]struct{})
	addBridgePortObservationKeys(deterministic, bridgePortRef{
		deviceID: "switch-a",
		ifIndex:  1,
		ifName:   "Gi0/1",
	})
	discoveryPairs := map[string]struct{}{
		topologyUndirectedPairKey("switch-a", "switch-b"): {},
	}

	links := []bridgeBridgeLinkRecord{
		{
			designatedPort: bridgePortRef{deviceID: "switch-a", ifIndex: 1, ifName: "Gi0/1"},
			port:           bridgePortRef{deviceID: "switch-b", ifIndex: 1, ifName: "Gi0/1"},
			method:         "lldp",
		},
		{
			designatedPort: bridgePortRef{deviceID: "switch-a", ifIndex: 2, ifName: "Gi0/2"},
			port:           bridgePortRef{deviceID: "switch-b", ifIndex: 2, ifName: "Gi0/2"},
			method:         "fdb_pairwise",
		},
		{
			designatedPort: bridgePortRef{deviceID: "switch-a", ifIndex: 1, ifName: "Gi0/1"},
			port:           bridgePortRef{deviceID: "switch-c", ifIndex: 7, ifName: "Gi0/7"},
			method:         "stp",
		},
		{
			designatedPort: bridgePortRef{deviceID: "switch-a", ifIndex: 3, ifName: "Gi0/3"},
			port:           bridgePortRef{deviceID: "switch-c", ifIndex: 3, ifName: "Gi0/3"},
			method:         "fdb_pairwise",
		},
	}

	filtered := suppressInferredBridgeLinksOnDeterministicDiscovery(links, deterministic, discoveryPairs)
	require.Len(t, filtered, 2)
	require.Equal(t, "lldp", filtered[0].method)
	require.Equal(t, "fdb_pairwise", filtered[1].method)
	require.Equal(t, "switch-c", filtered[1].port.deviceID)
}

func TestToGraph_FDBOwnerInferenceUsesReporterMatrixRule(t *testing.T) {
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

	data, _ := toGraphForTest(result, GraphOptions{
		Source: "snmp",
		Layer:  "2",
		View:   "summary",
	})

	targetLinks := findFDBLinksByEndpointMAC(data.Links, "dd:dd:dd:dd:dd:dd")
	require.Len(t, targetLinks, 1)

	segmentActor := findActorByMatch(data.Actors, targetLinks[0].Src.Match)
	require.NotNil(t, segmentActor)
	segmentDetail := requireActorDetail(t, data, segmentActor)
	require.Equal(t, []string{"switch-c"}, segmentDetail.Segment.ParentDevices)
	require.Equal(t, []string{"Gi0/2"}, segmentDetail.Segment.IfNames)
}

func findInterfaceStatusByIndex(statuses []ProjectionPortDetail, ifIndex int) *ProjectionPortDetail {
	for _, status := range statuses {
		if status.IfIndex.Has && status.IfIndex.Value == ifIndex {
			return &status
		}
	}
	return nil
}

func findNeighborByProtocol(neighbors []ProjectionPortNeighbor, protocol string) *ProjectionPortNeighbor {
	for _, neighbor := range neighbors {
		if strings.EqualFold(neighbor.Protocol, protocol) {
			return &neighbor
		}
	}
	return nil
}

func findFDBLinksByEndpointMAC(links []graph.Link, mac string) []graph.Link {
	out := make([]graph.Link, 0)
	for _, link := range links {
		if link.Protocol != "fdb" {
			continue
		}
		if slices.Contains(link.Dst.Match.MacAddresses, mac) {
			out = append(out, link)
		}
	}
	return out
}

func findFDBLinksByEndpointIP(links []graph.Link, ip string) []graph.Link {
	out := make([]graph.Link, 0)
	for _, link := range links {
		if link.Protocol != "fdb" {
			continue
		}
		if slices.Contains(link.Dst.Match.IPAddresses, ip) {
			out = append(out, link)
		}
	}
	return out
}

func topologyLinkSignatures(links []graph.Link) map[string]struct{} {
	out := make(map[string]struct{}, len(links))
	for _, link := range links {
		srcKey := canonicalTopologyMatchKey(link.Src.Match)
		dstKey := canonicalTopologyMatchKey(link.Dst.Match)
		if srcKey == "" || dstKey == "" {
			continue
		}
		key := strings.Join([]string{
			strings.ToLower(strings.TrimSpace(link.Protocol)),
			strings.ToLower(strings.TrimSpace(link.Direction)),
			srcKey,
			dstKey,
			strings.ToLower(strings.TrimSpace(link.State)),
			strings.ToLower(topologyLinkAttachmentMode(link)),
		}, keySep)
		out[key] = struct{}{}
	}
	return out
}

func findFDBLinksByDstSysName(links []graph.Link, sysName string) []graph.Link {
	out := make([]graph.Link, 0)
	for _, link := range links {
		if link.Protocol != "fdb" {
			continue
		}
		if link.Dst.Match.SysName != sysName {
			continue
		}
		out = append(out, link)
	}
	return out
}

func findActorByMatch(actors []graph.Actor, match graph.Match) *graph.Actor {
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

func findActorBySysName(actors []graph.Actor, sysName string) *graph.Actor {
	for i := range actors {
		if actors[i].Match.SysName == sysName {
			return &actors[i]
		}
	}
	return nil
}

func findActorByMAC(actors []graph.Actor, mac string) *graph.Actor {
	for i := range actors {
		if slices.Contains(actors[i].Match.MacAddresses, mac) {
			return &actors[i]
		}
	}
	return nil
}

func findActorByIP(actors []graph.Actor, ip string) *graph.Actor {
	for i := range actors {
		if slices.Contains(actors[i].Match.IPAddresses, ip) {
			return &actors[i]
		}
	}
	return nil
}

func findActorByType(actors []graph.Actor, actorType string) *graph.Actor {
	for i := range actors {
		if actors[i].ActorType == actorType {
			return &actors[i]
		}
	}
	return nil
}

func findLinkByProtocol(links []graph.Link, protocol string) *graph.Link {
	for i := range links {
		if links[i].Protocol == protocol {
			return &links[i]
		}
	}
	return nil
}
