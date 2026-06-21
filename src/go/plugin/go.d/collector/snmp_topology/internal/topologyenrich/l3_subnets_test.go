// SPDX-License-Identifier: GPL-3.0-or-later

package topologyenrich

import (
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestBuildTopologyL3SubnetAdjacencies(t *testing.T) {
	tests := map[string]struct {
		rows      []topologymodel.L3Interface
		wantLinks []topologyL3SubnetAdjacency
		wantStats topologymodel.L3SubnetBuildStats
	}{
		"ipv4-point-to-point": {
			rows: []topologymodel.L3Interface{
				l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.252", "3"),
				l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
			},
			wantLinks: []topologyL3SubnetAdjacency{
				{
					Subnet:  "198.51.100.0/30",
					Network: "198.51.100.0",
					Netmask: "255.255.255.252",
					Prefix:  30,
					A:       l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
					B:       l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.252", "3"),
				},
			},
			wantStats: topologymodel.L3SubnetBuildStats{
				CandidateSubnets: 1,
				CandidateLinks:   1,
			},
		},
		"ipv4-thirty-one": {
			rows: []topologymodel.L3Interface{
				l3InterfaceForTest("device-a", "198.51.100.0", "255.255.255.254", "2"),
				l3InterfaceForTest("device-b", "198.51.100.1", "255.255.255.254", "3"),
			},
			wantLinks: []topologyL3SubnetAdjacency{
				{
					Subnet:  "198.51.100.0/31",
					Network: "198.51.100.0",
					Netmask: "255.255.255.254",
					Prefix:  31,
					A:       l3InterfaceForTest("device-a", "198.51.100.0", "255.255.255.254", "2"),
					B:       l3InterfaceForTest("device-b", "198.51.100.1", "255.255.255.254", "3"),
				},
			},
			wantStats: topologymodel.L3SubnetBuildStats{
				CandidateSubnets: 1,
				CandidateLinks:   1,
			},
		},
		"unsupported-prefix": {
			rows: []topologymodel.L3Interface{
				l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.0", "2"),
				l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.0", "3"),
			},
			wantStats: topologymodel.L3SubnetBuildStats{
				SuppressedUnsupportedPrefix: 2,
			},
		},
		"duplicate-ip-ownership": {
			rows: []topologymodel.L3Interface{
				l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
				l3InterfaceForTest("device-b", "198.51.100.1", "255.255.255.252", "3"),
			},
			wantStats: topologymodel.L3SubnetBuildStats{
				CandidateSubnets:      1,
				SuppressedDuplicateIP: 1,
			},
		},
		"same-device-self-link": {
			rows: []topologymodel.L3Interface{
				l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
				l3InterfaceForTest("device-a", "198.51.100.2", "255.255.255.252", "3"),
			},
			wantStats: topologymodel.L3SubnetBuildStats{
				CandidateSubnets:   1,
				SuppressedSelfLink: 1,
			},
		},
		"unmatched-subnet": {
			rows: []topologymodel.L3Interface{
				l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
			},
			wantStats: topologymodel.L3SubnetBuildStats{
				CandidateSubnets:    1,
				SuppressedUnmatched: 1,
			},
		},
		"multi-access-rows": {
			rows: []topologymodel.L3Interface{
				l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
				l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.252", "3"),
				l3InterfaceForTest("device-c", "198.51.100.3", "255.255.255.252", "4"),
			},
			wantStats: topologymodel.L3SubnetBuildStats{
				CandidateSubnets:      1,
				SuppressedMultiAccess: 1,
			},
		},
		"invalid-rows": {
			rows: []topologymodel.L3Interface{
				l3InterfaceForTest("", "198.51.100.1", "255.255.255.252", "2"),
				l3InterfaceForTest("device-a", "not-an-ip", "255.255.255.252", "2"),
				l3InterfaceForTest("device-a", "2001:db8::1", "ffff:ffff:ffff:ffff::", "2"),
				l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", ""),
			},
			wantStats: topologymodel.L3SubnetBuildStats{
				SuppressedInvalid: 4,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			links, stats := buildTopologyL3SubnetAdjacencies(tc.rows)

			if len(tc.wantLinks) == 0 {
				require.Empty(t, links)
			} else {
				require.Equal(t, tc.wantLinks, links)
			}
			require.Equal(t, tc.wantStats, stats)
		})
	}
}

func TestBuildTopologyL3SubnetAdjacenciesDeterministic(t *testing.T) {
	rows := []topologymodel.L3Interface{
		l3InterfaceForTest("device-d", "198.51.100.6", "255.255.255.252", "6"),
		l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.252", "3"),
		l3InterfaceForTest("device-c", "198.51.100.5", "255.255.255.252", "5"),
		l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
	}
	reversed := slices.Clone(rows)
	slices.Reverse(reversed)

	links, stats := buildTopologyL3SubnetAdjacencies(rows)
	reversedLinks, reversedStats := buildTopologyL3SubnetAdjacencies(reversed)

	require.Equal(t, links, reversedLinks)
	require.Equal(t, stats, reversedStats)
	require.Len(t, links, 2)
	require.Equal(t, "198.51.100.0/30", links[0].Subnet)
	require.Equal(t, "198.51.100.4/30", links[1].Subnet)
}

func l3InterfaceForTest(deviceID, ip, netmask, ifIndex string) topologymodel.L3Interface {
	return topologymodel.L3Interface{
		DeviceID: deviceID,
		IP:       ip,
		Netmask:  netmask,
		IfIndex:  ifIndex,
	}
}
