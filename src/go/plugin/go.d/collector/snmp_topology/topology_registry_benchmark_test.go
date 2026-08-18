// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologyoptions"
	"github.com/stretchr/testify/require"
)

func TestSNMPTopologyRegistryKeepsLogicalLinkEndpointIPHintsConstantSized(t *testing.T) {
	const aliasCount = 256

	registry := benchmarkAliasRichTopologyRegistry(2, aliasCount, false, false, 1)
	data, ok, err := registry.snapshotWithOptions(topologyoptions.DefaultQueryOptions())
	require.NoError(t, err)
	require.True(t, ok)

	selectedIPByActorHandle := make(map[topologymodel.ActorHandle]string)
	managedActors := 0
	for _, actor := range data.Actors {
		if !topologymodel.IsManagedSNMPDeviceActor(actor) {
			continue
		}
		managedActors++
		require.Len(t, actor.Match.IPAddresses, aliasCount)
		selectedIPByActorHandle[actor.ActorHandle] = topologymodel.ActorDetailManagementIP(actor)
	}
	require.Equal(t, 2, managedActors)

	seen := make(map[string]int)
	for _, link := range data.Links {
		switch link.LinkType {
		case topologymodel.L3SubnetLinkType, topologymodel.L3SubnetMembershipLinkType,
			topologymodel.OSPFAdjacencyLinkType, topologymodel.BGPAdjacencyLinkType:
			seen[link.LinkType]++
			require.LessOrEqual(t, len(link.Src.Match.IPAddresses), 1, link.LinkType)
			require.LessOrEqual(t, len(link.Dst.Match.IPAddresses), 1, link.LinkType)
			if selected := selectedIPByActorHandle[link.SrcActorHandle]; selected != "" {
				require.Equal(t, []string{selected}, link.Src.Match.IPAddresses, link.LinkType)
			}
			if selected := selectedIPByActorHandle[link.DstActorHandle]; selected != "" {
				require.Equal(t, []string{selected}, link.Dst.Match.IPAddresses, link.LinkType)
			}
		}
	}
	require.Equal(t, map[string]int{
		topologymodel.L3SubnetLinkType:           1,
		topologymodel.L3SubnetMembershipLinkType: 2,
		topologymodel.OSPFAdjacencyLinkType:      1,
		topologymodel.BGPAdjacencyLinkType:       1,
	}, seen)
}

func TestSNMPTopologyRegistryPreservesInferredLLDPAliasesWithCompactLinkHandles(t *testing.T) {
	const (
		linkCount  = 4
		aliasCount = 4
	)
	registry := benchmarkInferredLLDPTopologyRegistry(linkCount, aliasCount)
	options := topologyoptions.DefaultQueryOptions()
	options.MapType = topologyoptions.MapTypeAllDevicesLowConfidence

	data, ok, err := registry.snapshotWithOptions(options)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, data.Links, linkCount)

	actorsByHandle := make(map[topologymodel.ActorHandle]topologymodel.Actor, len(data.Actors))
	var remote *topologymodel.Actor
	for i := range data.Actors {
		actor := &data.Actors[i]
		require.False(t, actor.ActorHandle.IsZero())
		actorsByHandle[actor.ActorHandle] = *actor
		if actor.Match.SysName == "weak-remote" {
			remote = actor
		}
	}
	require.NotNil(t, remote)
	require.Equal(t, []string{"10.1.0.1", "10.1.0.2", "10.1.0.3", "10.1.0.4"}, remote.Match.IPAddresses)
	require.Equal(t, "ip:10.1.0.1,10.1.0.2,10.1.0.3,10.1.0.4", remote.ActorID)

	for _, link := range data.Links {
		require.Contains(t, actorsByHandle, link.SrcActorHandle)
		require.Contains(t, actorsByHandle, link.DstActorHandle)
		require.Equal(t, remote.ActorHandle, link.DstActorHandle)
	}

	payload, ok, err := (funcDepsAdapter{registry: registry}).Snapshot(options)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, linkCount, payload.Links.Rows)
}

func BenchmarkSNMPTopologyFunctionInferredActorIDScaling(b *testing.B) {
	tests := []struct {
		links   int
		aliases int
	}{
		{links: 512, aliases: 1},
		{links: 512, aliases: 512},
		{links: 1024, aliases: 1},
		{links: 1024, aliases: 512},
		{links: 1024, aliases: 1024},
	}

	for _, tc := range tests {
		b.Run(fmt.Sprintf("links=%d/aliases=%d", tc.links, tc.aliases), func(b *testing.B) {
			registry := benchmarkInferredLLDPTopologyRegistry(tc.links, tc.aliases)
			options := topologyoptions.DefaultQueryOptions()
			options.MapType = topologyoptions.MapTypeAllDevicesLowConfidence

			data, ok, err := registry.snapshotWithOptions(options)
			if err != nil {
				b.Fatalf("build topology snapshot: %v", err)
			}
			if !ok {
				b.Fatal("topology snapshot is unavailable")
			}
			seenRemote := false
			for _, actor := range data.Actors {
				if actor.Match.SysName != "weak-remote" {
					continue
				}
				seenRemote = true
				if got := len(actor.Match.IPAddresses); got != tc.aliases {
					b.Fatalf("remote aliases=%d, want %d", got, tc.aliases)
				}
				if tc.aliases > 1 && len(actor.ActorID) < tc.aliases*8 {
					b.Fatalf("remote ActorID length=%d, unexpectedly short for %d aliases", len(actor.ActorID), tc.aliases)
				}
			}
			if !seenRemote {
				b.Fatal("inferred remote actor was not emitted")
			}
			if got := len(data.Links); got != tc.links {
				b.Fatalf("links=%d, want %d", got, tc.links)
			}

			deps := funcDepsAdapter{registry: registry}
			probe, ok, err := deps.Snapshot(options)
			if err != nil || !ok || probe.Links.Rows != tc.links {
				b.Fatalf("Function probe rows=%d ok=%t err=%v", probe.Links.Rows, ok, err)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				payload, ok, err := deps.Snapshot(options)
				if err != nil || !ok {
					b.Fatalf("Function snapshot ok=%t err=%v", ok, err)
				}
				runtime.KeepAlive(payload)
			}
		})
	}
}

func benchmarkInferredLLDPTopologyRegistry(linkCount, aliasCount int) *topologyRegistry {
	now := time.Now()
	cache := newTopologyCache()
	cache.lastUpdate = now
	cache.updateTime = now
	cache.agentID = "benchmark-agent"
	cache.localDevice = topologymodel.Device{
		ChassisID:     "02:00:00:00:00:01",
		ChassisIDType: "macAddress",
		SysName:       "reporter",
		ManagementIP:  "192.0.2.1",
	}

	for i := range linkCount {
		port := fmt.Sprintf("Ethernet%d", i+1)
		aliasIndex := i % aliasCount
		ip := fmt.Sprintf("10.%d.%d.%d", aliasIndex/64516+1, (aliasIndex/254)%254, aliasIndex%254+1)
		cache.lldpLocPorts[port] = &lldpLocPort{portNum: port, portID: port}
		cache.lldpRemotes[fmt.Sprintf("%s:1", port)] = &lldpRemote{
			localPortNum: port,
			remIndex:     "1",
			sysName:      "weak-remote",
			portID:       fmt.Sprintf("remote-%d", i+1),
			managementAddrs: []topologymodel.ManagementAddress{{
				Address:     ip,
				AddressType: "ipv4",
				Source:      "lldp_remote",
			}},
		}
	}

	return &topologyRegistry{
		caches:          map[*topologyCache]struct{}{cache: {}},
		producerScopeID: "benchmark-producer",
	}
}

func BenchmarkSNMPTopologyFunctionAliasScaling(b *testing.B) {
	tests := []struct {
		devices       int
		aliases       int
		sharedPrimary bool
		ipOnly        bool
		logicalPeers  int
	}{
		{devices: 16, aliases: 1},
		{devices: 64, aliases: 1},
		{devices: 16, aliases: 64},
		{devices: 32, aliases: 64},
		{devices: 64, aliases: 64},
		{devices: 64, aliases: 256},
		{devices: 16, aliases: 64, sharedPrimary: true},
		{devices: 32, aliases: 64, sharedPrimary: true},
		{devices: 64, aliases: 64, sharedPrimary: true},
		{devices: 64, aliases: 256, sharedPrimary: true},
		{devices: 64, aliases: 256, logicalPeers: 63},
		{devices: 64, aliases: 1, ipOnly: true},
		{devices: 64, aliases: 1, ipOnly: true, logicalPeers: 63},
		{devices: 64, aliases: 256, ipOnly: true},
		{devices: 64, aliases: 256, ipOnly: true, logicalPeers: 63},
	}

	for _, tc := range tests {
		b.Run(fmt.Sprintf("devices=%d/aliases=%d/shared_primary=%t/ip_only=%t/logical_peers=%d", tc.devices, tc.aliases, tc.sharedPrimary, tc.ipOnly, tc.logicalPeers), func(b *testing.B) {
			logicalPeers := tc.logicalPeers
			if logicalPeers == 0 {
				logicalPeers = 1
			}
			registry := benchmarkAliasRichTopologyRegistry(tc.devices, tc.aliases, tc.sharedPrimary, tc.ipOnly, logicalPeers)
			deps := funcDepsAdapter{registry: registry}
			options := topologyoptions.DefaultQueryOptions()
			options.MapType = topologyoptions.MapTypeAllDevicesLowConfidence

			probe, ok, err := deps.Snapshot(options)
			if err != nil || !ok {
				b.Fatalf("topology snapshot probe failed: ok=%t err=%v", ok, err)
			}
			if minimumLinks := logicalPeers * 3; !tc.sharedPrimary && probe.Links.Rows < minimumLinks {
				b.Fatalf("topology snapshot emitted %d links, want at least %d", probe.Links.Rows, minimumLinks)
			}
			if tc.sharedPrimary && probe.Links.Rows != 0 {
				b.Fatalf("shared-primary topology emitted %d links after actor collapse, want 0", probe.Links.Rows)
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				payload, ok, err := deps.Snapshot(options)
				if err != nil {
					b.Fatal(err)
				}
				if !ok {
					b.Fatal("topology snapshot is unavailable")
				}
				runtime.KeepAlive(payload)
			}
		})
	}
}

func benchmarkAliasRichTopologyRegistry(deviceCount, aliasCount int, sharedPrimary, ipOnly bool, logicalPeerCount int) *topologyRegistry {
	registry := &topologyRegistry{
		caches:          make(map[*topologyCache]struct{}, deviceCount),
		producerScopeID: "benchmark-producer",
	}
	now := time.Now()
	selectedIPs := make([]string, deviceCount)
	caches := make([]*topologyCache, 0, deviceCount)

	for deviceIndex := range deviceCount {
		cache := newTopologyCache()
		cache.lastUpdate = now
		cache.updateTime = now
		cache.agentID = fmt.Sprintf("benchmark-agent-%d", deviceIndex)

		addresses := make([]topologymodel.ManagementAddress, 0, aliasCount)
		for aliasIndex := range aliasCount {
			ip := benchmarkAliasIPAddress(deviceIndex, aliasIndex)
			addresses = append(addresses, topologymodel.ManagementAddress{
				Address:     ip,
				AddressType: "ipv4",
				Source:      "ip_mib",
			})
			selectedIPs[deviceIndex] = ip
		}
		if sharedPrimary {
			selectedIPs[deviceIndex] = "172.16.0.1"
		}

		chassisID := ""
		if !ipOnly {
			chassisID = fmt.Sprintf("02:00:00:00:%02x:%02x", deviceIndex/256, deviceIndex%256)
		}
		cache.localDevice = topologymodel.Device{
			ChassisID:           chassisID,
			ChassisIDType:       "macAddress",
			SysName:             fmt.Sprintf("benchmark-router-%d", deviceIndex),
			ManagementIP:        selectedIPs[deviceIndex],
			ManagementAddresses: addresses,
			OSPFRouterID:        fmt.Sprintf("192.0.2.%d", deviceIndex+1),
		}
		registry.caches[cache] = struct{}{}
		caches = append(caches, cache)
	}

	for deviceIndex, cache := range caches {
		ip := fmt.Sprintf("203.0.113.%d", deviceIndex+1)
		cache.l3InterfacesByIP[ip] = topologymodel.L3Interface{
			IP:      ip,
			Netmask: "255.255.255.0",
			IfIndex: "100",
		}
	}

	logicalPeerCount = min(logicalPeerCount, len(caches)-1)
	for peerIndex := 1; peerIndex <= logicalPeerCount; peerIndex++ {
		networkOffset := (peerIndex - 1) * 4
		localIP := fmt.Sprintf("198.51.100.%d", networkOffset+1)
		remoteIP := fmt.Sprintf("198.51.100.%d", networkOffset+2)
		caches[0].l3InterfacesByIP[localIP] = topologymodel.L3Interface{
			IP:      localIP,
			Netmask: "255.255.255.252",
			IfIndex: fmt.Sprintf("%d", peerIndex),
		}
		caches[peerIndex].l3InterfacesByIP[remoteIP] = topologymodel.L3Interface{
			IP:      remoteIP,
			Netmask: "255.255.255.252",
			IfIndex: "1",
		}
		caches[0].ospfNeighborsByKey[fmt.Sprintf("benchmark-neighbor-%d", peerIndex)] = topologymodel.OSPFNeighbor{
			LocalRouterID: caches[0].localDevice.OSPFRouterID,
			NeighborIP:    benchmarkAliasIPAddress(peerIndex, 0),
			LocalIP:       selectedIPs[0],
			State:         "full",
		}
		caches[0].bgpPeersByKey[fmt.Sprintf("benchmark-peer-%d", peerIndex)] = topologymodel.BGPPeer{
			NeighborIP:      benchmarkAliasIPAddress(peerIndex, 0),
			LocalIP:         selectedIPs[0],
			LocalIdentifier: caches[0].localDevice.OSPFRouterID,
			State:           "established",
		}
	}

	return registry
}

func benchmarkAliasIPAddress(deviceIndex, aliasIndex int) string {
	return fmt.Sprintf("10.%d.%d.%d", deviceIndex+1, aliasIndex/254, aliasIndex%254+1)
}
