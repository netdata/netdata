// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildTopologyL3SubnetAdjacenciesIPv4PointToPoint(t *testing.T) {
	rows := []topologyL3Interface{
		l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.252", "3"),
		l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
	}

	links, stats := buildTopologyL3SubnetAdjacencies(rows)

	require.Len(t, links, 1)
	require.Equal(t, "198.51.100.0/30", links[0].Subnet)
	require.Equal(t, "198.51.100.0", links[0].Network)
	require.Equal(t, "255.255.255.252", links[0].Netmask)
	require.Equal(t, 30, links[0].Prefix)
	require.Equal(t, "device-a", links[0].A.DeviceID)
	require.Equal(t, "device-b", links[0].B.DeviceID)
	require.Equal(t, topologyL3SubnetBuildStats{
		candidateSubnets: 1,
		candidateLinks:   1,
	}, stats)
}

func TestBuildTopologyL3SubnetAdjacenciesSupportsIPv4ThirtyOne(t *testing.T) {
	rows := []topologyL3Interface{
		l3InterfaceForTest("device-a", "198.51.100.0", "255.255.255.254", "2"),
		l3InterfaceForTest("device-b", "198.51.100.1", "255.255.255.254", "3"),
	}

	links, stats := buildTopologyL3SubnetAdjacencies(rows)

	require.Len(t, links, 1)
	require.Equal(t, "198.51.100.0/31", links[0].Subnet)
	require.Equal(t, 31, links[0].Prefix)
	require.Equal(t, topologyL3SubnetBuildStats{
		candidateSubnets: 1,
		candidateLinks:   1,
	}, stats)
}

func TestBuildTopologyL3SubnetAdjacenciesSuppressesUnsupportedPrefix(t *testing.T) {
	rows := []topologyL3Interface{
		l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.0", "2"),
		l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.0", "3"),
	}

	links, stats := buildTopologyL3SubnetAdjacencies(rows)

	require.Empty(t, links)
	require.Equal(t, topologyL3SubnetBuildStats{
		suppressedUnsupportedPrefix: 2,
	}, stats)
}

func TestBuildTopologyL3SubnetAdjacenciesSuppressesDuplicateIPOwnership(t *testing.T) {
	rows := []topologyL3Interface{
		l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
		l3InterfaceForTest("device-b", "198.51.100.1", "255.255.255.252", "3"),
	}

	links, stats := buildTopologyL3SubnetAdjacencies(rows)

	require.Empty(t, links)
	require.Equal(t, topologyL3SubnetBuildStats{
		candidateSubnets:      1,
		suppressedDuplicateIP: 1,
	}, stats)
}

func TestBuildTopologyL3SubnetAdjacenciesSuppressesSameDeviceSelfLink(t *testing.T) {
	rows := []topologyL3Interface{
		l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
		l3InterfaceForTest("device-a", "198.51.100.2", "255.255.255.252", "3"),
	}

	links, stats := buildTopologyL3SubnetAdjacencies(rows)

	require.Empty(t, links)
	require.Equal(t, topologyL3SubnetBuildStats{
		candidateSubnets:   1,
		suppressedSelfLink: 1,
	}, stats)
}

func TestBuildTopologyL3SubnetAdjacenciesSuppressesUnmatchedSubnet(t *testing.T) {
	rows := []topologyL3Interface{
		l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
	}

	links, stats := buildTopologyL3SubnetAdjacencies(rows)

	require.Empty(t, links)
	require.Equal(t, topologyL3SubnetBuildStats{
		candidateSubnets:    1,
		suppressedUnmatched: 1,
	}, stats)
}

func TestBuildTopologyL3SubnetAdjacenciesSuppressesMultiAccessRows(t *testing.T) {
	rows := []topologyL3Interface{
		l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", "2"),
		l3InterfaceForTest("device-b", "198.51.100.2", "255.255.255.252", "3"),
		l3InterfaceForTest("device-c", "198.51.100.3", "255.255.255.252", "4"),
	}

	links, stats := buildTopologyL3SubnetAdjacencies(rows)

	require.Empty(t, links)
	require.Equal(t, topologyL3SubnetBuildStats{
		candidateSubnets:      1,
		suppressedMultiAccess: 1,
	}, stats)
}

func TestBuildTopologyL3SubnetAdjacenciesDeterministic(t *testing.T) {
	rows := []topologyL3Interface{
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

func TestBuildTopologyL3SubnetAdjacenciesSuppressesInvalidRows(t *testing.T) {
	rows := []topologyL3Interface{
		l3InterfaceForTest("", "198.51.100.1", "255.255.255.252", "2"),
		l3InterfaceForTest("device-a", "not-an-ip", "255.255.255.252", "2"),
		l3InterfaceForTest("device-a", "2001:db8::1", "ffff:ffff:ffff:ffff::", "2"),
		l3InterfaceForTest("device-a", "198.51.100.1", "255.255.255.252", ""),
	}

	links, stats := buildTopologyL3SubnetAdjacencies(rows)

	require.Empty(t, links)
	require.Equal(t, topologyL3SubnetBuildStats{
		suppressedInvalid: 4,
	}, stats)
}

func l3InterfaceForTest(deviceID, ip, netmask, ifIndex string) topologyL3Interface {
	return topologyL3Interface{
		DeviceID: deviceID,
		AgentID:  "agent-test",
		IP:       ip,
		Netmask:  netmask,
		IfIndex:  ifIndex,
	}
}
