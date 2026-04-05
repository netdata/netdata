// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubNetworkLifecycleAndHelpers(t *testing.T) {
	subnet, err := NewSubNetwork(2, netip.MustParseAddr("192.0.2.2"), netip.MustParseAddr("255.255.255.252"))
	require.NoError(t, err)
	require.Equal(t, netip.MustParseAddr("192.0.2.0"), subnet.Network())
	require.Equal(t, netip.MustParseAddr("255.255.255.252"), subnet.Netmask())
	require.Equal(t, "192.0.2.0/30", subnet.CIDR())
	require.Equal(t, 30, subnet.NetworkPrefix())
	require.True(t, subnet.IsIPv4Subnetwork())
	require.Equal(t, []int{2}, subnet.NodeIDs())

	require.True(t, subnet.IsInRange(netip.MustParseAddr("192.0.2.1")))
	require.False(t, subnet.IsInRange(netip.MustParseAddr("192.0.2.8")))

	require.True(t, subnet.Add(1, netip.MustParseAddr("192.0.2.1")))
	require.False(t, subnet.Add(1, netip.MustParseAddr("192.0.2.1")))
	require.False(t, subnet.Add(3, netip.MustParseAddr("192.0.2.8")))
	require.Equal(t, []int{1, 2}, subnet.NodeIDs())

	require.True(t, subnet.Add(3, netip.MustParseAddr("192.0.2.1")))
	require.True(t, subnet.HasDuplicatedAddress())
	require.True(t, subnet.Remove(3, netip.MustParseAddr("192.0.2.1")))
	require.False(t, subnet.HasDuplicatedAddress())
	require.False(t, subnet.Remove(3, netip.MustParseAddr("192.0.2.1")))

	cloned := subnet.clone()
	require.NotNil(t, cloned)
	require.True(t, cloned.Remove(1, netip.MustParseAddr("192.0.2.1")))
	require.Equal(t, []int{1, 2}, subnet.NodeIDs())
	require.Equal(t, []int{2}, cloned.NodeIDs())

	require.Equal(t, "192.0.2.0\x00255.255.255.252", subnet.key())
	require.Equal(t, "192.0.2.0\x00255.255.255.252", subnetKey(subnet.Network(), subnet.Netmask()))
	require.Equal(t, []string{"a", "b"}, sortedSubnetworkKeys(map[string]*SubNetwork{
		"b": subnet,
		"a": cloned,
	}))

	require.Equal(t, -1, compareAddr(netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2")))
	require.Equal(t, 1, compareAddr(netip.MustParseAddr("2001:db8::2"), netip.MustParseAddr("2001:db8::1")))
	require.Equal(t, 0, compareAddr(netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.1")))

	require.True(t, IsPointToPointMask(netip.MustParseAddr("255.255.255.252")))
	require.True(t, IsPointToPointMask(netip.MustParseAddr("ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe")))
	require.True(t, IsLoopbackMask(netip.MustParseAddr("255.255.255.255")))
	require.True(t, InSameNetwork(
		netip.MustParseAddr("10.0.0.1"),
		netip.MustParseAddr("10.0.0.2"),
		netip.MustParseAddr("255.255.255.252"),
	))
	require.False(t, InSameNetwork(
		netip.MustParseAddr("10.0.0.1"),
		netip.MustParseAddr("10.0.0.5"),
		netip.MustParseAddr("255.255.255.252"),
	))

	network, ok := NetworkAddress(netip.MustParseAddr("10.0.0.2"), netip.MustParseAddr("255.255.255.252"))
	require.True(t, ok)
	require.Equal(t, netip.MustParseAddr("10.0.0.0"), network)

	prefix, err := MaskToCIDRPrefix(netip.MustParseAddr("255.255.255.252"))
	require.NoError(t, err)
	require.Equal(t, 30, prefix)
	_, err = MaskToCIDRPrefix(netip.MustParseAddr("255.0.255.0"))
	require.Error(t, err)
}

func TestNodeTopologyServiceAndRouterTopology(t *testing.T) {
	service := NewNodeTopologyService(
		[]NodeTopologyEntity{
			{ID: 3, Label: "node-3", Address: netip.MustParseAddr("192.168.1.3")},
			{ID: 1, Label: "node-1", Address: netip.MustParseAddr("10.0.0.1")},
			{ID: 2, Label: "node-2", Address: netip.MustParseAddr("10.0.0.2")},
		},
		[]IPInterfaceTopologyEntity{
			{ID: 5, NodeID: 2, IPAddress: netip.MustParseAddr("192.168.1.2"), NetMask: netip.MustParseAddr("255.255.255.0"), IsManaged: true, SnmpInterfaceID: 103},
			{ID: 2, NodeID: 1, IPAddress: netip.MustParseAddr("10.0.0.1"), NetMask: netip.MustParseAddr("255.255.255.252"), IsManaged: true, IsSnmpPrimary: true, SnmpInterfaceID: 101},
			{ID: 6, NodeID: 3, IPAddress: netip.MustParseAddr("127.0.0.1"), NetMask: netip.MustParseAddr("255.255.255.255"), IsManaged: true},
			{ID: 4, NodeID: 3, IPAddress: netip.MustParseAddr("192.168.1.3"), NetMask: netip.MustParseAddr("255.255.255.0"), IsManaged: true, IsSnmpPrimary: true, SnmpInterfaceID: 104},
			{ID: 3, NodeID: 2, IPAddress: netip.MustParseAddr("10.0.0.2"), NetMask: netip.MustParseAddr("255.255.255.252"), IsManaged: true, IsSnmpPrimary: true, SnmpInterfaceID: 102},
			{ID: 1, NodeID: 1, IPAddress: netip.MustParseAddr("10.10.10.1"), IsManaged: false},
		},
		[]SnmpInterfaceTopologyEntity{
			{ID: 104, NodeID: 3, IfIndex: 4, IfName: "eth4"},
			{ID: 101, NodeID: 1, IfIndex: 1, IfName: "eth1"},
			{ID: 103, NodeID: 2, IfIndex: 3, IfName: "eth3"},
			{ID: 102, NodeID: 2, IfIndex: 2, IfName: "eth2"},
		},
	)

	nodes := service.FindAllNode()
	require.Equal(t, []int{1, 2, 3}, []int{nodes[0].ID, nodes[1].ID, nodes[2].ID})

	ips := service.FindAllIP()
	require.Equal(t, []int{1, 2, 3, 4, 5, 6}, []int{ips[0].ID, ips[1].ID, ips[2].ID, ips[3].ID, ips[4].ID, ips[5].ID})

	snmp := service.FindAllSnmp()
	require.Equal(t, []int{101, 102, 103, 104}, []int{snmp[0].ID, snmp[1].ID, snmp[2].ID, snmp[3].ID})

	allSubnets := service.FindAllSubNetwork()
	require.Len(t, allSubnets, 3)
	require.Equal(t, []string{"10.0.0.0/30", "127.0.0.1/32", "192.168.1.0/24"}, []string{
		allSubnets[0].CIDR(),
		allSubnets[1].CIDR(),
		allSubnets[2].CIDR(),
	})

	legal := service.FindAllLegalSubNetwork()
	require.Len(t, legal, 2)
	require.Equal(t, []string{"10.0.0.0/30", "192.168.1.0/24"}, []string{legal[0].CIDR(), legal[1].CIDR()})

	ptp := service.FindAllPointToPointSubNetwork()
	require.Len(t, ptp, 1)
	require.Equal(t, "10.0.0.0/30", ptp[0].CIDR())

	legalPTP := service.FindAllLegalPointToPointSubNetwork()
	require.Len(t, legalPTP, 1)
	require.Equal(t, []int{1, 2}, legalPTP[0].NodeIDs())

	loopbacks := service.FindAllLoopbacks()
	require.Len(t, loopbacks, 1)
	require.Equal(t, "127.0.0.1/32", loopbacks[0].CIDR())

	legalLoopbacks := service.FindAllLegalLoopbacks()
	require.Len(t, legalLoopbacks, 1)
	require.Equal(t, []int{3}, legalLoopbacks[0].NodeIDs())

	multibet := service.FindSubNetworkByNetworkPrefixLessThen(30, 126)
	require.Len(t, multibet, 1)
	require.Equal(t, "192.168.1.0/24", multibet[0].CIDR())

	require.Equal(t, map[int]int{
		1: 0,
		2: 0,
		3: 1,
	}, service.GetNodeIDPriorityMap())

	router := BuildNetworkRouterTopology(service, 30, 126)
	require.Equal(t, "1", router.DefaultVertex)
	require.Len(t, router.Vertices, 4)
	require.Len(t, router.Edges, 3)

	require.Equal(t, []string{"1", "192.168.1.0/24", "2", "3"}, []string{
		router.Vertices[0].ID,
		router.Vertices[1].ID,
		router.Vertices[2].ID,
		router.Vertices[3].ID,
	})

	require.Equal(t, []string{
		"1\x0010.0.0.1->2\x0010.0.0.2",
		"192.168.1.0/24\x00192.168.1.0/24to:192.168.1.2->2\x00192.168.1.2",
		"192.168.1.0/24\x00192.168.1.0/24to:192.168.1.3->3\x00192.168.1.3",
	}, []string{
		router.Edges[0].ID,
		router.Edges[1].ID,
		router.Edges[2].ID,
	})
	require.Equal(t, "1", router.Edges[0].SourcePort.Vertex)
	require.Equal(t, "2", router.Edges[0].TargetPort.Vertex)
	require.Equal(t, "eth1", router.Edges[0].SourcePort.IfName)
	require.Equal(t, "eth2", router.Edges[0].TargetPort.IfName)
	require.Equal(t, "192.168.1.0/24", router.Edges[1].SourcePort.Vertex)
	require.Equal(t, "2", router.Edges[1].TargetPort.Vertex)
	require.Equal(t, "", router.Edges[1].SourcePort.IfName)
	require.Equal(t, "eth3", router.Edges[1].TargetPort.IfName)
	require.Equal(t, "192.168.1.0/24", router.Edges[2].SourcePort.Vertex)
	require.Equal(t, "3", router.Edges[2].TargetPort.Vertex)
}
