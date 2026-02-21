// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"testing"

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
	for _, dev := range result.Devices {
		if dev.ID == "distribution-sw" {
			remote = dev
			break
		}
	}
	require.Equal(t, "distribution-sw", remote.ID)
	require.Equal(t, "distribution-sw", remote.Hostname)
	require.Equal(t, "SEP001122334455", remote.ChassisID)
	require.Equal(t, "10.0.0.2", remote.Addresses[0].String())
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

func TestBuildL2ResultFromObservations_ARPMergesIPOnlyIntoMACEndpoint(t *testing.T) {
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
					IP:       "10.20.4.99",
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
	require.Equal(t, 1, result.Stats["endpoints_total"])
}

func TestNormalizeMAC_PadsSingleNibbleTokens(t *testing.T) {
	require.Equal(t, "00:15:99:9f:07:ef", normalizeMAC("0:15:99:9f:7:ef"))
	require.Equal(t, "60:33:4b:08:17:a8", normalizeMAC("60:33:4b:8:17:a8"))
	require.Equal(t, "00:90:1a:42:22:f8", normalizeMAC("0:90:1a:42:22:f8"))
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
	require.Equal(t, lldpMatchPassDefault, pairs[0].pass)
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
		require.NotEmpty(t, adj.Labels[adjacencyLabelPairSide])
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
		require.NotEmpty(t, adj.Labels[adjacencyLabelPairSide])
		require.NotEmpty(t, adj.Labels[adjacencyLabelPairPass])
	}
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

	pairSides := make(map[string]map[string]struct{})
	pairPasses := make(map[string]string)
	for _, adj := range result.Adjacencies {
		if adj.Protocol != "lldp" && adj.Protocol != "cdp" {
			continue
		}
		pairID := adj.Labels[adjacencyLabelPairID]
		pairSide := adj.Labels[adjacencyLabelPairSide]
		pairPass := adj.Labels[adjacencyLabelPairPass]

		require.NotEmpty(t, pairID)
		require.NotEmpty(t, pairSide)
		require.Contains(t, []string{adjacencyPairSideSource, adjacencyPairSideTarget}, pairSide)
		require.NotEmpty(t, pairPass)

		if pairSides[adj.Protocol] == nil {
			pairSides[adj.Protocol] = make(map[string]struct{})
		}
		pairSides[adj.Protocol][pairSide] = struct{}{}

		if existingPass, ok := pairPasses[adj.Protocol]; ok {
			require.Equal(t, existingPass, pairPass)
		} else {
			pairPasses[adj.Protocol] = pairPass
		}
	}

	require.Len(t, pairSides["lldp"], 2)
	require.Len(t, pairSides["cdp"], 2)
	require.Equal(t, lldpMatchPassDefault, pairPasses["lldp"])
	require.Equal(t, cdpMatchPassDefault, pairPasses["cdp"])
}

func TestBuildL2ResultFromObservations_ErrorsOnEmptyInput(t *testing.T) {
	_, err := BuildL2ResultFromObservations(nil, DiscoverOptions{EnableLLDP: true})
	require.Error(t, err)
}
