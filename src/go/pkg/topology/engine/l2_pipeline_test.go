// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildL2ResultFromObservations_LLDPAndCDP(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "switch-a",
			Hostname:     "switch-a.example.net",
			ManagementIP: "10.0.0.1",
			ChassisID:    "aa:bb:cc:dd:ee:ff",
			Interfaces: []ObservedInterface{
				{IfIndex: 8, IfName: "Gi0/0", IfDescr: "Gi0/0"},
			},
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum: "8",
					RemoteIndex:  "1",
					LocalPortID:  "Gi0/0",
					ChassisID:    "bb:cc:dd:ee:ff:00",
					SysName:      "switch-b.example.net",
					PortID:       "Gi0/1",
					ManagementIP: "10.0.0.2",
				},
			},
			CDPRemotes: []CDPRemoteObservation{
				{
					LocalIfIndex: 8,
					LocalIfName:  "Gi0/0",
					DeviceIndex:  "1",
					DeviceID:     "switch-b.example.net",
					DevicePort:   "Gi0/1",
				},
			},
		},
		{
			DeviceID:     "switch-b",
			Hostname:     "switch-b.example.net",
			ManagementIP: "10.0.0.2",
			ChassisID:    "bb:cc:dd:ee:ff:00",
			Interfaces: []ObservedInterface{
				{IfIndex: 9, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableLLDP: true, EnableCDP: true})
	require.NoError(t, err)
	require.Len(t, result.Devices, 2)
	require.Len(t, result.Interfaces, 2)
	require.Len(t, result.Adjacencies, 2)
	require.Equal(t, 2, result.Stats["links_total"])
	require.Equal(t, 1, result.Stats["links_lldp"])
	require.Equal(t, 1, result.Stats["links_cdp"])

	require.Equal(t, "cdp", result.Adjacencies[0].Protocol)
	require.Equal(t, "switch-a", result.Adjacencies[0].SourceID)
	require.Equal(t, "switch-b", result.Adjacencies[0].TargetID)
	require.Equal(t, "lldp", result.Adjacencies[1].Protocol)
	require.Equal(t, "switch-a", result.Adjacencies[1].SourceID)
	require.Equal(t, "switch-b", result.Adjacencies[1].TargetID)
}

func TestBuildL2ResultFromObservations_DefaultProtocols(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "switch-a",
			Hostname:     "switch-a",
			ManagementIP: "10.0.0.1",
			CDPRemotes: []CDPRemoteObservation{
				{
					LocalIfIndex: 8,
					DeviceIndex:  "1",
					Address:      "0a000002",
				},
			},
		},
		{
			DeviceID:     "switch-b",
			Hostname:     "switch-b",
			ManagementIP: "10.0.0.2",
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{})
	require.NoError(t, err)
	require.Len(t, result.Adjacencies, 1)
	require.Equal(t, "switch-b", result.Adjacencies[0].TargetID)
	require.Equal(t, 0, result.Stats["links_lldp"])
	require.Equal(t, 1, result.Stats["links_cdp"])
}

func TestBuildL2ResultFromObservations_UsesProvidedCollectedAt(t *testing.T) {
	collectedAt := time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC)

	result, err := BuildL2ResultFromObservations([]L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a.example.net",
		},
	}, DiscoverOptions{CollectedAt: collectedAt})
	require.NoError(t, err)
	require.Equal(t, collectedAt, result.CollectedAt)
}

func TestBuildL2ResultFromObservations_InterfaceStatusLabels(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a",
			Interfaces: []ObservedInterface{
				{
					IfIndex:       8,
					IfName:        "Gi0/0",
					IfDescr:       "Gi0/0",
					IfAlias:       "uplink-a",
					MAC:           "AA:BB:CC:DD:EE:FF",
					SpeedBps:      1_000_000_000,
					LastChange:    12345,
					Duplex:        "full",
					InterfaceType: "ethernetcsmacd",
					AdminStatus:   "up",
					OperStatus:    "lowerLayerDown",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{})
	require.NoError(t, err)
	require.Len(t, result.Interfaces, 1)
	require.Equal(t, "ethernetcsmacd", result.Interfaces[0].Labels["if_type"])
	require.Equal(t, "up", result.Interfaces[0].Labels["admin_status"])
	require.Equal(t, "lowerLayerDown", result.Interfaces[0].Labels["oper_status"])
	require.Equal(t, "uplink-a", result.Interfaces[0].Labels["if_alias"])
	require.Equal(t, "aa:bb:cc:dd:ee:ff", result.Interfaces[0].Labels["mac"])
	require.Equal(t, "1000000000", result.Interfaces[0].Labels["speed_bps"])
	require.Equal(t, "12345", result.Interfaces[0].Labels["last_change"])
	require.Equal(t, "full", result.Interfaces[0].Labels["duplex"])
}

func TestBuildL2ResultFromObservations_DeviceProtocolsObservedLabel(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "switch-a",
			Hostname:     "switch-a",
			ManagementIP: "10.0.0.1",
			BridgePorts: []BridgePortObservation{
				{BasePort: "3", IfIndex: 3},
			},
			FDBEntries: []FDBObservation{
				{MAC: "70:49:a2:65:72:cd", BridgePort: "3", Status: "learned"},
			},
			ARPNDEntries: []ARPNDObservation{
				{IfIndex: 3, IP: "10.0.0.20", MAC: "70:49:a2:65:72:cd", Protocol: "arp"},
			},
			// Collected STP row that does not form a usable topology edge.
			STPPorts: []STPPortObservation{
				{Port: "3", DesignatedBridge: "00:11:22:33:44:55"},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{
		EnableBridge: true,
		EnableARP:    true,
		EnableSTP:    true,
	})
	require.NoError(t, err)
	require.Len(t, result.Devices, 1)
	require.Equal(t, "arp,bridge,fdb,stp", result.Devices[0].Labels["protocols_observed"])
}

func TestBuildL2ResultFromObservations_MergesDuplicateDeviceObservations(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "switch-a",
			Hostname:     "switch-a.example.net",
			ManagementIP: "10.0.0.1",
			SysObjectID:  "1.3.6.1.4.1.9.1.1",
			ChassisID:    "aa:bb:cc:dd:ee:ff",
			BridgePorts: []BridgePortObservation{
				{BasePort: "3", IfIndex: 3},
			},
		},
		{
			DeviceID:     "switch-a",
			ManagementIP: "10.0.0.2",
			ARPNDEntries: []ARPNDObservation{
				{IfIndex: 3, IP: "10.0.0.20", MAC: "70:49:a2:65:72:cd", Protocol: "arp"},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{
		EnableBridge: true,
		EnableARP:    true,
	})
	require.NoError(t, err)

	device := findDeviceByID(result.Devices, "switch-a")
	require.NotNil(t, device)
	require.Equal(t, "switch-a.example.net", device.Hostname)
	require.Equal(t, "1.3.6.1.4.1.9.1.1", device.SysObject)
	require.Equal(t, "aa:bb:cc:dd:ee:ff", device.ChassisID)
	require.Equal(t, []string{"10.0.0.1", "10.0.0.2"}, deviceAddressStrings(*device))
	require.Equal(t, "arp,bridge", device.Labels["protocols_observed"])
}

func TestBuildL2ResultFromObservations_SkipsSelfAdjacencies(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "dw",
			Hostname:     "dw",
			ManagementIP: "10.104.133.114",
			ChassisID:    "cf",
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum: "1",
					RemoteIndex:  "1",
					LocalPortID:  "CF",
					ChassisID:    "cf",
					SysName:      "dw",
					PortID:       "CF",
				},
			},
			CDPRemotes: []CDPRemoteObservation{
				{
					LocalIfIndex: 1,
					LocalIfName:  "CF",
					DeviceIndex:  "1",
					DeviceID:     "dw",
					SysName:      "dw",
					DevicePort:   "CF",
					Address:      "10.104.133.114",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableLLDP: true, EnableCDP: true})
	require.NoError(t, err)
	require.Len(t, result.Devices, 1)
	require.Empty(t, result.Adjacencies)
	require.Equal(t, 0, result.Stats["links_total"])
	require.Equal(t, 0, result.Stats["links_lldp"])
	require.Equal(t, 0, result.Stats["links_cdp"])
}

func TestBuildL2ResultFromObservations_CDPSysNameAndDeviceID(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "switch-a",
			Hostname:     "switch-a",
			ManagementIP: "10.0.0.1",
			CDPRemotes: []CDPRemoteObservation{
				{
					LocalIfIndex: 1,
					DeviceIndex:  "1",
					DeviceID:     "SEP001122334455",
					SysName:      "distribution-sw",
					Address:      "0a000002",
					DevicePort:   "Gi0/48",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableCDP: true})
	require.NoError(t, err)
	require.Len(t, result.Devices, 2)
	require.Len(t, result.Adjacencies, 1)
	require.Equal(t, "distribution-sw", result.Adjacencies[0].TargetID)

	var remote Device
	var local Device
	for _, dev := range result.Devices {
		if dev.ID == "distribution-sw" {
			remote = dev
		}
		if dev.ID == "switch-a" {
			local = dev
		}
	}
	require.Equal(t, "distribution-sw", remote.ID)
	require.Equal(t, "distribution-sw", remote.Hostname)
	require.Equal(t, "SEP001122334455", remote.ChassisID)
	require.Equal(t, "10.0.0.2", remote.Addresses[0].String())
	require.Equal(t, "true", remote.Labels["inferred"])
	require.Equal(t, "false", local.Labels["inferred"])
}

func TestBuildL2ResultFromObservations_FDBAttachments(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a",
			Interfaces: []ObservedInterface{
				{IfIndex: 3, IfName: "Port3", IfDescr: "Port3"},
			},
			BridgePorts: []BridgePortObservation{
				{BasePort: "7", IfIndex: 3},
			},
			FDBEntries: []FDBObservation{
				{MAC: "7049a26572cd", BridgePort: "7", Status: "learned"},
				{MAC: "70:49:a2:65:72:ce", BridgePort: "7", Status: "learned"},
				{MAC: "70:49:a2:65:72:cd", BridgePort: "7", Status: "learned"},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableBridge: true})
	require.NoError(t, err)
	require.Empty(t, result.Adjacencies)
	require.Len(t, result.Attachments, 2)

	first := result.Attachments[0]
	require.Equal(t, "switch-a", first.DeviceID)
	require.Equal(t, 3, first.IfIndex)
	require.Equal(t, "mac:70:49:a2:65:72:cd", first.EndpointID)
	require.Equal(t, "fdb", first.Method)
	require.Equal(t, "bridge-domain:switch-a:if:3", first.Labels["bridge_domain"])
	require.Equal(t, "7", first.Labels["bridge_port"])
	require.Equal(t, "Port3", first.Labels["if_name"])

	require.Equal(t, 2, result.Stats["attachments_total"])
	require.Equal(t, 2, result.Stats["attachments_fdb"])
	require.Equal(t, 1, result.Stats["bridge_domains_total"])
	require.Equal(t, 2, result.Stats["endpoints_total"])
}

func TestBuildL2ResultFromObservations_FDBDropsDuplicateMACAcrossPorts(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a",
			BridgePorts: []BridgePortObservation{
				{BasePort: "1", IfIndex: 1},
				{BasePort: "2", IfIndex: 2},
			},
			FDBEntries: []FDBObservation{
				{MAC: "70:49:a2:65:72:cd", BridgePort: "1", Status: "learned"},
				{MAC: "70:49:a2:65:72:cd", BridgePort: "2", Status: "learned"},
				{MAC: "70:49:a2:65:72:ce", BridgePort: "2", Status: "learned"},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableBridge: true})
	require.NoError(t, err)
	require.Len(t, result.Attachments, 1)
	require.Equal(t, "mac:70:49:a2:65:72:ce", result.Attachments[0].EndpointID)
	require.Equal(t, 1, result.Stats["attachments_fdb"])
}

func TestBuildL2ResultFromObservations_FDBKeepsSameMACAcrossPortsWhenVLANDiffers(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a",
			BridgePorts: []BridgePortObservation{
				{BasePort: "1", IfIndex: 1},
				{BasePort: "2", IfIndex: 2},
			},
			FDBEntries: []FDBObservation{
				{MAC: "70:49:a2:65:72:cd", BridgePort: "1", Status: "learned", VLANID: "10"},
				{MAC: "70:49:a2:65:72:cd", BridgePort: "2", Status: "learned", VLANID: "20"},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableBridge: true})
	require.NoError(t, err)
	require.Len(t, result.Attachments, 2)

	first := result.Attachments[0]
	second := result.Attachments[1]
	require.Equal(t, "mac:70:49:a2:65:72:cd", first.EndpointID)
	require.Equal(t, "mac:70:49:a2:65:72:cd", second.EndpointID)
	require.NotEqual(t, first.Labels["vlan_id"], second.Labels["vlan_id"])
	require.Equal(t, 2, result.Stats["attachments_fdb"])
}

func TestBuildL2ResultFromObservations_FDBSkipsSelfAndNonLearned(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a",
			BridgePorts: []BridgePortObservation{
				{BasePort: "1", IfIndex: 1},
				{BasePort: "2", IfIndex: 2},
				{BasePort: "3", IfIndex: 3},
			},
			FDBEntries: []FDBObservation{
				{MAC: "00:11:22:33:44:55", BridgePort: "1", Status: "self"},
				{MAC: "00:11:22:33:44:55", BridgePort: "2", Status: "learned"},
				{MAC: "00:aa:bb:cc:dd:ee", BridgePort: "2", Status: "mgmt"},
				{MAC: "00:ff:ee:dd:cc:bb", BridgePort: "3", Status: "learned"},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableBridge: true})
	require.NoError(t, err)
	require.Len(t, result.Attachments, 1)
	require.Equal(t, "mac:00:ff:ee:dd:cc:bb", result.Attachments[0].EndpointID)
	require.Equal(t, "3", result.Attachments[0].Labels["bridge_port"])
}

func TestBuildL2ResultFromObservations_STPAdjacency(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			BaseBridgeAddress: "00:11:22:33:44:55",
			Interfaces: []ObservedInterface{
				{IfIndex: 3, IfName: "Port3", IfDescr: "Port3"},
			},
			BridgePorts: []BridgePortObservation{
				{BasePort: "3", IfIndex: 3},
			},
			STPPorts: []STPPortObservation{
				{
					Port:             "3",
					VLANID:           "200",
					VLANName:         "servers",
					DesignatedBridge: "66:77:88:99:aa:bb",
					DesignatedPort:   "8001",
					State:            "forwarding",
				},
			},
		},
		{
			DeviceID:          "switch-b",
			Hostname:          "switch-b",
			BaseBridgeAddress: "66:77:88:99:aa:bb",
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableSTP: true})
	require.NoError(t, err)
	require.Len(t, result.Adjacencies, 1)
	require.Equal(t, "stp", result.Adjacencies[0].Protocol)
	require.Equal(t, "switch-a", result.Adjacencies[0].SourceID)
	require.Equal(t, "switch-b", result.Adjacencies[0].TargetID)
	require.Equal(t, "Port3", result.Adjacencies[0].SourcePort)
	require.Equal(t, "200", result.Adjacencies[0].Labels["vlan_id"])
	require.Equal(t, "servers", result.Adjacencies[0].Labels["vlan_name"])
	require.Equal(t, 1, result.Stats["links_stp"])
}

func TestBuildL2ResultFromObservations_STPDoesNotCreateSyntheticActors(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:          "switch-a",
			Hostname:          "switch-a",
			BaseBridgeAddress: "00:11:22:33:44:55",
			Interfaces: []ObservedInterface{
				{IfIndex: 3, IfName: "Port3", IfDescr: "Port3"},
			},
			BridgePorts: []BridgePortObservation{
				{BasePort: "3", IfIndex: 3},
			},
			STPPorts: []STPPortObservation{
				{
					Port:             "3",
					DesignatedBridge: "66:77:88:99:aa:bb",
					DesignatedPort:   "8001",
					State:            "forwarding",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableSTP: true})
	require.NoError(t, err)
	require.Len(t, result.Devices, 1)
	require.Empty(t, result.Adjacencies)
	require.Equal(t, 0, result.Stats["links_stp"])
}

func TestBuildL2ResultFromObservations_FDBBridgeDomainFallbackToBridgePort(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a",
			FDBEntries: []FDBObservation{
				{MAC: "70:49:a2:65:72:cd", BridgePort: "77", Status: "learned"},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableBridge: true})
	require.NoError(t, err)
	require.Len(t, result.Attachments, 1)
	require.Equal(t, 0, result.Attachments[0].IfIndex)
	require.Equal(t, "bridge-domain:switch-a:bp:77", result.Attachments[0].Labels["bridge_domain"])
}

func TestBuildL2ResultFromObservations_FDBVLANNameLabel(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a",
			BridgePorts: []BridgePortObservation{
				{BasePort: "7", IfIndex: 3},
			},
			FDBEntries: []FDBObservation{
				{MAC: "70:49:a2:65:72:cd", BridgePort: "7", Status: "learned", VLANID: "200", VLANName: "servers"},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableBridge: true})
	require.NoError(t, err)
	require.Len(t, result.Attachments, 1)
	require.Equal(t, "servers", result.Attachments[0].Labels["vlan_name"])
}

func TestBuildL2ResultFromObservations_BridgeOnlyDoesNotAutoEnableDiscoveryProtocols(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a",
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum: "1",
					LocalPortID:  "Gi0/1",
					SysName:      "switch-b",
					PortID:       "Gi0/2",
				},
			},
			BridgePorts: []BridgePortObservation{
				{BasePort: "1", IfIndex: 1},
			},
			FDBEntries: []FDBObservation{
				{MAC: "70:49:a2:65:72:cd", BridgePort: "1"},
			},
		},
		{
			DeviceID: "switch-b",
			Hostname: "switch-b",
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableBridge: true})
	require.NoError(t, err)
	require.Empty(t, result.Adjacencies)
	require.Len(t, result.Attachments, 1)
}

func TestBuildL2ResultFromObservations_ARPNDEnrichment(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a",
			ARPNDEntries: []ARPNDObservation{
				{
					Protocol: "arp",
					IfIndex:  3,
					IfName:   "Port3",
					IP:       "10.20.4.84",
					MAC:      "7049a26572cd",
					State:    "reachable",
					AddrType: "ipv4",
				},
				{
					Protocol: "arp",
					IfIndex:  5,
					IfName:   "Port5",
					IP:       "10.20.4.85",
					MAC:      "70:49:a2:65:72:cd",
					State:    "reachable",
					AddrType: "ipv4",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableARP: true})
	require.NoError(t, err)
	require.Empty(t, result.Adjacencies)
	require.Empty(t, result.Attachments)
	require.Len(t, result.Enrichments, 1)

	enrichment := result.Enrichments[0]
	require.Equal(t, "mac:70:49:a2:65:72:cd", enrichment.EndpointID)
	require.Equal(t, "70:49:a2:65:72:cd", enrichment.MAC)
	require.Len(t, enrichment.IPs, 2)
	require.Equal(t, "10.20.4.84", enrichment.IPs[0].String())
	require.Equal(t, "10.20.4.85", enrichment.IPs[1].String())
	require.Equal(t, "arp", enrichment.Labels["sources"])
	require.Equal(t, "switch-a", enrichment.Labels["device_ids"])
	require.Equal(t, "3,5", enrichment.Labels["if_indexes"])
	require.Equal(t, "Port3,Port5", enrichment.Labels["if_names"])
	require.Equal(t, "reachable", enrichment.Labels["states"])
	require.Equal(t, "ipv4", enrichment.Labels["addr_types"])
	require.Equal(t, 1, result.Stats["enrichments_total"])
	require.Equal(t, 1, result.Stats["enrichments_arp_nd"])
}

func TestBuildL2ResultFromObservations_SkipsMACLessARPNDEntries(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a",
			ARPNDEntries: []ARPNDObservation{
				{
					Protocol: "arp",
					IfIndex:  7,
					IfName:   "Port7",
					IP:       "10.20.4.99",
					State:    "reachable",
					AddrType: "ipv4",
				},
				{
					Protocol: "arp",
					IfIndex:  7,
					IfName:   "Port7",
					IP:       "10.20.4.100",
					MAC:      "7049a26572cf",
					State:    "reachable",
					AddrType: "ipv4",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableARP: true})
	require.NoError(t, err)
	require.Len(t, result.Enrichments, 1)
	require.Equal(t, "mac:70:49:a2:65:72:cf", result.Enrichments[0].EndpointID)
	require.Equal(t, []string{"10.20.4.100"}, addressStrings(result.Enrichments[0].IPs))
	require.Equal(t, 1, result.Stats["endpoints_total"])
}

func TestBuildL2ResultFromObservations_ReconcilesARPAliasIntoLLDPDeviceIdentity(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "mikrotik-router",
			Hostname:     "MikroTik-router",
			ManagementIP: "10.20.4.1",
			ChassisID:    "18:fd:74:7e:c5:80",
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum:       "5",
					LocalPortID:        "ether5",
					LocalPortIDSubtype: "interfaceName",
					ChassisID:          "d8:5e:d3:0e:c5:e6",
					SysName:            "costa-desktop",
					PortID:             "enp6s0",
					PortIDSubtype:      "interfaceName",
					ManagementIP:       "fc00:f853:ccd:e793::1",
				},
			},
			ARPNDEntries: []ARPNDObservation{
				{
					Protocol: "arp",
					IfIndex:  5,
					IfName:   "ether5",
					IP:       "10.20.4.205",
					MAC:      "d8:5e:d3:0e:c5:e6",
					State:    "reachable",
					AddrType: "ipv4",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableLLDP: true, EnableARP: true})
	require.NoError(t, err)

	costaDesktop := findDeviceByHostname(result.Devices, "costa-desktop")
	require.NotNil(t, costaDesktop)
	require.ElementsMatch(
		t,
		[]string{"10.20.4.205", "fc00:f853:ccd:e793::1"},
		deviceAddressStrings(*costaDesktop),
	)
	require.Equal(t, 1, result.Stats["identity_alias_endpoints_mapped"])
	require.Equal(t, 1, result.Stats["identity_alias_ips_merged"])
}

func TestBuildL2ResultFromObservations_SkipsConflictingARPAliases(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "mikrotik-router",
			Hostname:     "MikroTik-router",
			ManagementIP: "10.20.4.1",
			ChassisID:    "18:fd:74:7e:c5:80",
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum:       "5",
					LocalPortID:        "ether5",
					LocalPortIDSubtype: "interfaceName",
					ChassisID:          "d8:5e:d3:0e:c5:e6",
					SysName:            "costa-desktop",
					PortID:             "enp6s0",
					PortIDSubtype:      "interfaceName",
					ManagementIP:       "fc00:f853:ccd:e793::1",
				},
			},
			ARPNDEntries: []ARPNDObservation{
				{
					Protocol: "arp",
					IfIndex:  5,
					IfName:   "ether5",
					IP:       "10.20.4.205",
					MAC:      "d8:5e:d3:0e:c5:e6",
					State:    "reachable",
					AddrType: "ipv4",
				},
				{
					Protocol: "arp",
					IfIndex:  7,
					IfName:   "ether7",
					IP:       "10.20.4.205",
					MAC:      "70:49:a2:65:72:cd",
					State:    "reachable",
					AddrType: "ipv4",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableLLDP: true, EnableARP: true})
	require.NoError(t, err)

	costaDesktop := findDeviceByHostname(result.Devices, "costa-desktop")
	require.NotNil(t, costaDesktop)
	require.Equal(t, []string{"fc00:f853:ccd:e793::1"}, deviceAddressStrings(*costaDesktop))
	require.Equal(t, 1, result.Stats["identity_alias_endpoints_mapped"])
	require.Equal(t, 0, result.Stats["identity_alias_ips_merged"])
	require.Equal(t, 1, result.Stats["identity_alias_ips_conflict_skipped"])
}

func TestBuildL2ResultFromObservations_SkipsAmbiguousMACAliasOwnership(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "switch-a",
			Hostname:     "switch-a",
			ManagementIP: "10.0.0.1",
			ChassisID:    "00:11:22:33:44:55",
			Interfaces: []ObservedInterface{
				{IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1", MAC: "aa:aa:aa:aa:aa:aa"},
			},
		},
		{
			DeviceID:     "switch-b",
			Hostname:     "switch-b",
			ManagementIP: "10.0.0.2",
			ChassisID:    "00:11:22:33:44:66",
			Interfaces: []ObservedInterface{
				{IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1", MAC: "aa:aa:aa:aa:aa:aa"},
			},
		},
		{
			DeviceID: "observer",
			Hostname: "observer",
			ARPNDEntries: []ARPNDObservation{
				{
					Protocol: "arp",
					IfIndex:  9,
					IfName:   "Gi0/9",
					IP:       "10.0.0.50",
					MAC:      "aa:aa:aa:aa:aa:aa",
					State:    "reachable",
					AddrType: "ipv4",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableARP: true})
	require.NoError(t, err)

	switchA := findDeviceByID(result.Devices, "switch-a")
	switchB := findDeviceByID(result.Devices, "switch-b")
	require.NotNil(t, switchA)
	require.NotNil(t, switchB)
	require.NotContains(t, deviceAddressStrings(*switchA), "10.0.0.50")
	require.NotContains(t, deviceAddressStrings(*switchB), "10.0.0.50")
	require.Equal(t, 0, result.Stats["identity_alias_ips_merged"])
	require.Equal(t, 1, result.Stats["identity_alias_endpoints_ambiguous_mac"])
}

func TestNormalizeMAC_PadsSingleNibbleTokens(t *testing.T) {
	require.Equal(t, "00:15:99:9f:07:ef", normalizeMAC("0:15:99:9f:7:ef"))
	require.Equal(t, "60:33:4b:08:17:a8", normalizeMAC("60:33:4b:8:17:a8"))
	require.Equal(t, "00:90:1a:42:22:f8", normalizeMAC("0:90:1a:42:22:f8"))
	require.Equal(t, "00:11:22:33:44:55", normalizeMAC("0011.2233.4455"))
}

func TestBuildL2ResultFromObservations_DeterministicOrderingAndDedup(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID: "switch-a",
			Hostname: "switch-a",
			Interfaces: []ObservedInterface{
				{IfIndex: 7, IfName: "Port7", IfDescr: "Port7"},
			},
			BridgePorts: []BridgePortObservation{
				{BasePort: "2", IfIndex: 7},
			},
			FDBEntries: []FDBObservation{
				{MAC: "70:49:a2:65:72:ce", BridgePort: "2"},
				{MAC: "70:49:a2:65:72:cd", BridgePort: "2"},
				{MAC: "7049a26572ce", BridgePort: "2"}, // duplicate MAC, different format
			},
			ARPNDEntries: []ARPNDObservation{
				{Protocol: "arp", IfIndex: 7, IfName: "Port7", IP: "10.20.4.86", MAC: "70:49:a2:65:72:ce"},
				{Protocol: "arp", IfIndex: 7, IfName: "Port7", IP: "10.20.4.85", MAC: "70:49:a2:65:72:ce"},
				{Protocol: "arp", IfIndex: 7, IfName: "Port7", IP: "10.20.4.85", MAC: "7049a26572ce"}, // duplicate
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableBridge: true, EnableARP: true})
	require.NoError(t, err)

	require.Len(t, result.Attachments, 2)
	require.Equal(t, "mac:70:49:a2:65:72:cd", result.Attachments[0].EndpointID)
	require.Equal(t, "mac:70:49:a2:65:72:ce", result.Attachments[1].EndpointID)

	require.Len(t, result.Enrichments, 1)
	require.Equal(t, "mac:70:49:a2:65:72:ce", result.Enrichments[0].EndpointID)
	require.Len(t, result.Enrichments[0].IPs, 2)
	require.Equal(t, "10.20.4.85", result.Enrichments[0].IPs[0].String())
	require.Equal(t, "10.20.4.86", result.Enrichments[0].IPs[1].String())
}

func TestMatchLLDPLinksEnlinkdPassOrder_Precedence(t *testing.T) {
	links := []lldpMatchLink{
		{
			index:               0,
			sourceDeviceID:      "node-a",
			localChassisID:      "chassis-a",
			remoteChassisID:     "chassis-b",
			localSysName:        "node-a",
			remoteSysName:       "node-b",
			localPortID:         "Gi0/1",
			localPortIDSubtype:  "5",
			remotePortID:        "Gi0/2",
			remotePortIDSubtype: "5",
			localPortDescr:      "GigabitEthernet0/1",
			remotePortDescr:     "GigabitEthernet0/2",
		},
		{
			index:               1,
			sourceDeviceID:      "node-b",
			localChassisID:      "chassis-b",
			remoteChassisID:     "chassis-a",
			localSysName:        "node-b",
			remoteSysName:       "node-a",
			localPortID:         "Gi0/2",
			localPortIDSubtype:  "5",
			remotePortID:        "Gi0/1",
			remotePortIDSubtype: "5",
			localPortDescr:      "GigabitEthernet0/2",
			remotePortDescr:     "GigabitEthernet0/1",
		},
	}

	pairs := matchLLDPLinksEnlinkdPassOrder(links)
	require.Len(t, pairs, 1)
	require.Equal(t, 0, pairs[0].sourceIndex)
	require.Equal(t, 1, pairs[0].targetIndex)
	require.NotEmpty(t, pairs[0].pass)
}

func TestMatchLLDPLinksEnlinkdPassOrder_FallbackPasses(t *testing.T) {
	tests := []struct {
		name        string
		pass        string
		left, right lldpMatchLink
	}{
		{
			name: "port-description",
			pass: lldpMatchPassPortDesc,
			left: lldpMatchLink{
				index:               0,
				sourceDeviceID:      "a",
				localChassisID:      "A",
				remoteChassisID:     "B",
				localSysName:        "a",
				remoteSysName:       "b",
				localPortID:         "Gi0/1",
				localPortIDSubtype:  "5",
				remotePortID:        "wrong-b",
				remotePortIDSubtype: "5",
				localPortDescr:      "PORT-A",
				remotePortDescr:     "PORT-B",
			},
			right: lldpMatchLink{
				index:               1,
				sourceDeviceID:      "b",
				localChassisID:      "B",
				remoteChassisID:     "A",
				localSysName:        "b",
				remoteSysName:       "a",
				localPortID:         "Gi0/2",
				localPortIDSubtype:  "5",
				remotePortID:        "wrong-a",
				remotePortIDSubtype: "5",
				localPortDescr:      "PORT-B",
				remotePortDescr:     "PORT-A",
			},
		},
		{
			name: "sysname",
			pass: lldpMatchPassSysName,
			left: lldpMatchLink{
				index:               0,
				sourceDeviceID:      "a",
				localChassisID:      "A-LOCAL",
				remoteChassisID:     "B-REMOTE",
				localSysName:        "sys-a",
				remoteSysName:       "sys-b",
				localPortID:         "xe-0/0/1",
				localPortIDSubtype:  "5",
				remotePortID:        "xe-0/0/2",
				remotePortIDSubtype: "5",
				localPortDescr:      "left-a",
				remotePortDescr:     "left-b",
			},
			right: lldpMatchLink{
				index:               1,
				sourceDeviceID:      "b",
				localChassisID:      "B-LOCAL",
				remoteChassisID:     "A-REMOTE",
				localSysName:        "sys-b",
				remoteSysName:       "sys-a",
				localPortID:         "xe-0/0/2",
				localPortIDSubtype:  "5",
				remotePortID:        "xe-0/0/1",
				remotePortIDSubtype: "5",
				localPortDescr:      "right-b",
				remotePortDescr:     "right-a",
			},
		},
		{
			name: "chassis-port-subtype",
			pass: lldpMatchPassChassisPort,
			left: lldpMatchLink{
				index:               0,
				sourceDeviceID:      "a",
				localChassisID:      "A",
				remoteChassisID:     "B",
				localSysName:        "a-local",
				remoteSysName:       "b-remote",
				localPortID:         "A1",
				localPortIDSubtype:  "5",
				remotePortID:        "wrong-b",
				remotePortIDSubtype: "5",
				localPortDescr:      "DA",
				remotePortDescr:     "RA",
			},
			right: lldpMatchLink{
				index:               1,
				sourceDeviceID:      "b",
				localChassisID:      "B",
				remoteChassisID:     "A",
				localSysName:        "b-local",
				remoteSysName:       "a-remote",
				localPortID:         "B1",
				localPortIDSubtype:  "5",
				remotePortID:        "A1",
				remotePortIDSubtype: "5",
				localPortDescr:      "DB",
				remotePortDescr:     "RB",
			},
		},
		{
			name: "chassis-port-description",
			pass: lldpMatchPassChassisDescr,
			left: lldpMatchLink{
				index:               0,
				sourceDeviceID:      "a",
				localChassisID:      "A",
				remoteChassisID:     "B",
				localSysName:        "a-local",
				remoteSysName:       "b-remote",
				localPortID:         "A1",
				localPortIDSubtype:  "5",
				remotePortID:        "wrong-b",
				remotePortIDSubtype: "5",
				localPortDescr:      "PORT-A",
				remotePortDescr:     "REMOTE-A",
			},
			right: lldpMatchLink{
				index:               1,
				sourceDeviceID:      "b",
				localChassisID:      "B",
				remoteChassisID:     "A",
				localSysName:        "b-local",
				remoteSysName:       "a-remote",
				localPortID:         "B1",
				localPortIDSubtype:  "5",
				remotePortID:        "wrong-a",
				remotePortIDSubtype: "5",
				localPortDescr:      "PORT-B",
				remotePortDescr:     "PORT-A",
			},
		},
		{
			name: "chassis-only",
			pass: lldpMatchPassChassis,
			left: lldpMatchLink{
				index:               0,
				sourceDeviceID:      "a",
				localChassisID:      "A",
				remoteChassisID:     "B",
				localSysName:        "a-local",
				remoteSysName:       "b-remote",
				localPortID:         "A1",
				localPortIDSubtype:  "5",
				remotePortID:        "wrong-b",
				remotePortIDSubtype: "5",
				localPortDescr:      "PORT-A",
				remotePortDescr:     "REMOTE-A",
			},
			right: lldpMatchLink{
				index:               1,
				sourceDeviceID:      "b",
				localChassisID:      "B",
				remoteChassisID:     "A",
				localSysName:        "b-local",
				remoteSysName:       "a-remote",
				localPortID:         "B1",
				localPortIDSubtype:  "5",
				remotePortID:        "wrong-a",
				remotePortIDSubtype: "5",
				localPortDescr:      "PORT-B",
				remotePortDescr:     "REMOTE-B",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pairs := matchLLDPLinksEnlinkdPassOrder([]lldpMatchLink{tc.left, tc.right})
			require.Len(t, pairs, 1)
			require.Equal(t, 0, pairs[0].sourceIndex)
			require.Equal(t, 1, pairs[0].targetIndex)
			require.Equal(t, tc.pass, pairs[0].pass)
		})
	}
}

func TestBuildL2ResultFromObservations_LLDPPairsAcrossChassisRepresentations(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "router-a",
			Hostname:     "MikroTik-router",
			ManagementIP: "10.20.4.1",
			ChassisID:    "18:FD:74:7E:C5:80",
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum:       "3",
					LocalPortID:        "ether3",
					LocalPortIDSubtype: "interfaceName",
					ChassisID:          "7049A26572CD",
					PortID:             "",
					PortIDSubtype:      "interfaceName",
					SysName:            "XS1930",
					ManagementIP:       "10.20.4.84",
				},
			},
		},
		{
			DeviceID:     "switch-b",
			Hostname:     "XS1930",
			ManagementIP: "10.20.4.84",
			ChassisID:    "70:49:a2:65:72:cd",
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum:       "8",
					LocalPortID:        "8",
					LocalPortIDSubtype: "local",
					ChassisID:          "18fd747ec580",
					PortID:             "ether3",
					PortIDSubtype:      "interfaceName",
					SysName:            "MikroTik-router",
					ManagementIP:       "10.20.4.1",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableLLDP: true})
	require.NoError(t, err)
	require.Len(t, result.Adjacencies, 2)

	var pairID string
	for _, adj := range result.Adjacencies {
		require.Equal(t, "lldp", adj.Protocol)
		require.NotEmpty(t, adj.Labels[adjacencyLabelPairID])
		require.NotEmpty(t, adj.Labels[adjacencyLabelPairPass])
		if pairID == "" {
			pairID = adj.Labels[adjacencyLabelPairID]
		}
		require.Equal(t, pairID, adj.Labels[adjacencyLabelPairID])
	}
}

func TestBuildL2ResultFromObservations_LLDPPairsAcrossKnownDeviceIdentityDespiteChassisMismatch(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "mikrotik-router",
			Hostname:     "MikroTik-router",
			ManagementIP: "10.20.4.1",
			ChassisID:    "18:FD:74:7E:C5:80",
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum:       "3",
					LocalPortID:        "ether3",
					LocalPortIDSubtype: "interfaceName",
					ChassisID:          "70:49:A2:65:72:D5",
					PortID:             "",
					PortIDSubtype:      "interfaceName",
					SysName:            "XS1930",
					ManagementIP:       "10.20.4.84",
				},
			},
		},
		{
			DeviceID:     "xs1930",
			Hostname:     "XS1930",
			ManagementIP: "10.20.4.84",
			ChassisID:    "70:49:A2:65:72:CD",
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum:       "8",
					LocalPortID:        "8",
					LocalPortIDSubtype: "local",
					ChassisID:          "18:FD:74:7E:C5:80",
					PortID:             "ether3",
					PortIDSubtype:      "interfaceName",
					SysName:            "MikroTik-router",
					ManagementIP:       "10.20.4.1",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableLLDP: true})
	require.NoError(t, err)
	require.Len(t, result.Adjacencies, 2)

	for _, adj := range result.Adjacencies {
		require.Equal(t, "lldp", adj.Protocol)
		require.NotEmpty(t, adj.Labels[adjacencyLabelPairID])
		require.NotEmpty(t, adj.Labels[adjacencyLabelPairPass])
	}
}

func TestBuildL2ResultFromObservations_KeepsDistinctRemotesWhenMACDiffersDespiteSameSecondaryIdentity(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "local-a",
			Hostname:     "local-a",
			ManagementIP: "10.0.0.1",
			ChassisID:    "00:00:00:00:10:01",
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum:       "1",
					LocalPortID:        "eth1",
					LocalPortIDSubtype: "interfaceName",
					ChassisID:          "00:11:22:33:44:55",
					SysName:            "shared-secondary-id",
					ManagementIP:       "10.20.30.40",
				},
			},
		},
		{
			DeviceID:     "local-b",
			Hostname:     "local-b",
			ManagementIP: "10.0.0.2",
			ChassisID:    "00:00:00:00:10:02",
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum:       "1",
					LocalPortID:        "eth1",
					LocalPortIDSubtype: "interfaceName",
					ChassisID:          "00:11:22:33:44:66",
					SysName:            "shared-secondary-id",
					ManagementIP:       "10.20.30.40",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableLLDP: true})
	require.NoError(t, err)

	var remoteCount int
	remoteIDs := make(map[string]struct{})
	for _, device := range result.Devices {
		if device.Hostname != "shared-secondary-id" {
			continue
		}
		remoteCount++
		remoteIDs[device.ID] = struct{}{}
	}
	require.Equal(t, 2, remoteCount)
	require.Len(t, remoteIDs, 2)
}

func TestMatchLLDPLinksEnlinkdPassOrder_DoesNotDropLinksWhenSysNamesAreEmpty(t *testing.T) {
	links := []lldpMatchLink{
		{
			index:               0,
			localChassisID:      "00:11:22:33:44:55",
			localMatchID:        "device-a",
			localPortID:         "Gi0/1",
			localPortIDSubtype:  "interfaceName",
			remoteChassisID:     "00:11:22:33:44:66",
			remotePortID:        "Gi0/2",
			remotePortIDSubtype: "interfaceName",
		},
		{
			index:               1,
			localChassisID:      "00:11:22:33:44:66",
			localMatchID:        "device-b",
			localPortID:         "Gi0/2",
			localPortIDSubtype:  "interfaceName",
			remoteChassisID:     "00:11:22:33:44:55",
			remotePortID:        "Gi0/1",
			remotePortIDSubtype: "interfaceName",
		},
	}

	pairs := matchLLDPLinksEnlinkdPassOrder(links)

	require.Len(t, pairs, 1)
	require.Equal(t, 0, pairs[0].sourceIndex)
	require.Equal(t, 1, pairs[0].targetIndex)
	require.NotEmpty(t, pairs[0].pass)
}

func TestResolveKnownRemote_RejectsHostnameMatchWhenMACMismatchesWithoutMgmtIP(t *testing.T) {
	state := newL2BuildState(1)
	state.devices["known-device"] = Device{
		ID:        "known-device",
		Hostname:  "shared-host",
		ChassisID: "00:11:22:33:44:55",
	}
	state.hostToID["shared-host"] = "known-device"

	require.Empty(t, state.resolveKnownRemote("shared-host", "", "", "00:11:22:33:44:66"))
}

func TestResolveRemote_UsesMACDerivedIDWhenManagedHostnameCollidesWithoutMgmtIP(t *testing.T) {
	state := newL2BuildState(1)
	state.devices["known-device"] = Device{
		ID:        "known-device",
		Hostname:  "shared-host",
		ChassisID: "00:11:22:33:44:55",
	}
	state.managedObservationByDeviceID["known-device"] = true
	state.hostToID["shared-host"] = "known-device"

	id := state.resolveRemote("shared-host", "00:11:22:33:44:66", "", "")

	require.Equal(t, "chassis-001122334466", id)
	require.Equal(t, "known-device", state.hostToID["shared-host"])
	require.Equal(t, id, state.macToID["00:11:22:33:44:66"])
}

func TestResolveRemote_ReusesHostnameIDForUnmanagedRemoteCollisions(t *testing.T) {
	state := newL2BuildState(1)

	firstID := state.resolveRemote("shared-host", "00:11:22:33:44:55", "", "")
	secondID := state.resolveRemote("shared-host", "00:11:22:33:44:66", "", "")

	require.Equal(t, "shared-host", firstID)
	require.Equal(t, "shared-host", secondID)
	require.Equal(t, "shared-host", state.hostToID["shared-host"])
}

func TestResolveRemoteEnforcingHostnameMACGuard_SplitsUnmanagedHostnameCollisions(t *testing.T) {
	state := newL2BuildState(1)

	firstID := state.resolveRemoteEnforcingHostnameMACGuard("shared-host", "00:11:22:33:44:55", "", "")
	secondID := state.resolveRemoteEnforcingHostnameMACGuard("shared-host", "00:11:22:33:44:66", "", "")

	require.Equal(t, "shared-host", firstID)
	require.Equal(t, "chassis-001122334466", secondID)
	require.Equal(t, "shared-host", state.hostToID["shared-host"])
	require.Equal(t, secondID, state.macToID["00:11:22:33:44:66"])
}

func TestRegisterObservation_ReinitializesLabelsAfterEmptyMerge(t *testing.T) {
	state := newL2BuildState(1)
	state.devices["switch-a"] = Device{ID: "switch-a", Hostname: "switch-a"}

	err := state.registerObservation(L2Observation{
		DeviceID: "switch-a",
		LLDPRemotes: []LLDPRemoteObservation{
			{LocalPortNum: "1", ChassisID: "00:11:22:33:44:55"},
		},
	})

	require.NoError(t, err)
	require.Equal(t, "lldp", state.devices["switch-a"].Labels["protocols_observed"])
}

func TestMatchLLDPLinksEnlinkdPassOrder_SkipsEmptyPortDescriptions(t *testing.T) {
	links := []lldpMatchLink{
		{
			index:               0,
			sourceDeviceID:      "a",
			localChassisID:      "A",
			remoteChassisID:     "B",
			localPortID:         "A-1",
			localPortIDSubtype:  "5",
			remotePortID:        "wrong-b-1",
			remotePortIDSubtype: "5",
		},
		{
			index:               1,
			sourceDeviceID:      "a",
			localChassisID:      "A",
			remoteChassisID:     "B",
			localPortID:         "A-2",
			localPortIDSubtype:  "5",
			remotePortID:        "wrong-b-2",
			remotePortIDSubtype: "5",
		},
		{
			index:               2,
			sourceDeviceID:      "b",
			localChassisID:      "B",
			remoteChassisID:     "A",
			localPortID:         "B-2",
			localPortIDSubtype:  "5",
			remotePortID:        "A-2",
			remotePortIDSubtype: "5",
		},
		{
			index:               3,
			sourceDeviceID:      "b",
			localChassisID:      "B",
			remoteChassisID:     "A",
			localPortID:         "B-1",
			localPortIDSubtype:  "5",
			remotePortID:        "A-1",
			remotePortIDSubtype: "5",
		},
	}

	pairs := matchLLDPLinksEnlinkdPassOrder(links)

	require.Len(t, pairs, 2)
	require.Equal(t, lldpMatchPassChassisPort, pairs[0].pass)
	require.Equal(t, 0, pairs[0].sourceIndex)
	require.Equal(t, 3, pairs[0].targetIndex)
	require.Equal(t, lldpMatchPassChassisPort, pairs[1].pass)
	require.Equal(t, 1, pairs[1].sourceIndex)
	require.Equal(t, 2, pairs[1].targetIndex)
}

func TestMatchCDPLinksEnlinkdPassOrder_DefaultAndParsedTarget(t *testing.T) {
	links := []cdpMatchLink{
		{
			index:              0,
			sourceDeviceID:     "node-a",
			sourceGlobalID:     "A-GID",
			localInterfaceName: "Gi0/0",
			remoteDeviceID:     "B-GID",
			remoteDevicePort:   "Gi0/1",
		},
		{
			index:              1,
			sourceDeviceID:     "node-b",
			sourceGlobalID:     "B-GID",
			localInterfaceName: "Gi0/1",
			remoteDeviceID:     "A-GID",
			remoteDevicePort:   "Gi0/0",
		},
		{
			index:              2,
			sourceDeviceID:     "node-c",
			sourceGlobalID:     "A-GID",
			localInterfaceName: "Gi0/0",
			remoteDeviceID:     "B-GID",
			remoteDevicePort:   "Gi0/1",
		},
	}

	pairs := matchCDPLinksEnlinkdPassOrder(links)
	require.Len(t, pairs, 1)
	require.Equal(t, 0, pairs[0].sourceIndex)
	require.Equal(t, 1, pairs[0].targetIndex)
	require.Equal(t, cdpMatchPassDefault, pairs[0].pass)
}

func TestBuildCDPLookupMap_PreservesFirstDuplicateKey(t *testing.T) {
	links := []cdpMatchLink{
		{
			index:              0,
			sourceGlobalID:     "A-GID",
			localInterfaceName: "Gi0/0",
			remoteDeviceID:     "B-GID",
			remoteDevicePort:   "Gi0/1",
		},
		{
			index:              1,
			sourceGlobalID:     "A-GID",
			localInterfaceName: "Gi0/0",
			remoteDeviceID:     "B-GID",
			remoteDevicePort:   "Gi0/1",
		},
	}

	lookup := buildCDPLookupMap(links)
	key := topologyMatchCompositeKey("Gi0/1", "Gi0/0", "A-GID", "B-GID")
	value, ok := lookup[key]
	require.True(t, ok)
	require.Equal(t, 0, value)
}

func TestMatchCDPLinksEnlinkdPassOrder_SkipsSelfTarget(t *testing.T) {
	links := []cdpMatchLink{
		{
			index:              0,
			sourceDeviceID:     "node-a",
			sourceGlobalID:     "A-GID",
			localInterfaceName: "Gi0/0",
			remoteDeviceID:     "A-GID",
			remoteDevicePort:   "Gi0/0",
		},
	}

	pairs := matchCDPLinksEnlinkdPassOrder(links)
	require.Empty(t, pairs)
}

func TestBuildL2ResultFromObservations_AnnotatesPairMetadata(t *testing.T) {
	observations := []L2Observation{
		{
			DeviceID:     "switch-a",
			Hostname:     "A-GID",
			ManagementIP: "10.0.0.1",
			ChassisID:    "aa:aa:aa:aa:aa:aa",
			Interfaces: []ObservedInterface{
				{IfIndex: 1, IfName: "Gi0/1", IfDescr: "Gi0/1"},
			},
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum:       "1",
					RemoteIndex:        "1",
					LocalPortID:        "Gi0/1",
					LocalPortIDSubtype: "5",
					ChassisID:          "bb:bb:bb:bb:bb:bb",
					SysName:            "B-GID",
					PortID:             "Gi0/2",
					PortIDSubtype:      "5",
				},
			},
			CDPRemotes: []CDPRemoteObservation{
				{
					LocalIfIndex: 1,
					LocalIfName:  "Gi0/1",
					DeviceIndex:  "1",
					DeviceID:     "B-GID",
					SysName:      "B-GID",
					DevicePort:   "Gi0/2",
				},
			},
		},
		{
			DeviceID:     "switch-b",
			Hostname:     "B-GID",
			ManagementIP: "10.0.0.2",
			ChassisID:    "bb:bb:bb:bb:bb:bb",
			Interfaces: []ObservedInterface{
				{IfIndex: 2, IfName: "Gi0/2", IfDescr: "Gi0/2"},
			},
			LLDPRemotes: []LLDPRemoteObservation{
				{
					LocalPortNum:       "2",
					RemoteIndex:        "1",
					LocalPortID:        "Gi0/2",
					LocalPortIDSubtype: "5",
					ChassisID:          "aa:aa:aa:aa:aa:aa",
					SysName:            "A-GID",
					PortID:             "Gi0/1",
					PortIDSubtype:      "5",
				},
			},
			CDPRemotes: []CDPRemoteObservation{
				{
					LocalIfIndex: 2,
					LocalIfName:  "Gi0/2",
					DeviceIndex:  "1",
					DeviceID:     "A-GID",
					SysName:      "A-GID",
					DevicePort:   "Gi0/1",
				},
			},
		},
	}

	result, err := BuildL2ResultFromObservations(observations, DiscoverOptions{EnableLLDP: true, EnableCDP: true})
	require.NoError(t, err)
	require.Len(t, result.Adjacencies, 4)

	pairIDs := make(map[string]map[string]struct{})
	pairPasses := make(map[string]string)
	for _, adj := range result.Adjacencies {
		if adj.Protocol != "lldp" && adj.Protocol != "cdp" {
			continue
		}
		pairID := adj.Labels[adjacencyLabelPairID]
		pairPass := adj.Labels[adjacencyLabelPairPass]

		require.NotEmpty(t, pairID)
		require.NotEmpty(t, pairPass)

		if pairIDs[adj.Protocol] == nil {
			pairIDs[adj.Protocol] = make(map[string]struct{})
		}
		pairIDs[adj.Protocol][pairID] = struct{}{}

		if existingPass, ok := pairPasses[adj.Protocol]; ok {
			require.Equal(t, existingPass, pairPass)
		} else {
			pairPasses[adj.Protocol] = pairPass
		}
	}

	require.Len(t, pairIDs["lldp"], 1)
	require.Len(t, pairIDs["cdp"], 1)
	require.Equal(t, lldpMatchPassDefault, pairPasses["lldp"])
	require.Equal(t, cdpMatchPassDefault, pairPasses["cdp"])
}

func TestBuildL2ResultFromObservations_ErrorsOnEmptyInput(t *testing.T) {
	_, err := BuildL2ResultFromObservations(nil, DiscoverOptions{EnableLLDP: true})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidRequest)
}

func findDeviceByHostname(devices []Device, hostname string) *Device {
	for i := range devices {
		if devices[i].Hostname == hostname {
			return &devices[i]
		}
	}
	return nil
}

func findDeviceByID(devices []Device, id string) *Device {
	for i := range devices {
		if devices[i].ID == id {
			return &devices[i]
		}
	}
	return nil
}

func deviceAddressStrings(device Device) []string {
	out := make([]string, 0, len(device.Addresses))
	for _, addr := range device.Addresses {
		if !addr.IsValid() {
			continue
		}
		out = append(out, addr.String())
	}
	return out
}
