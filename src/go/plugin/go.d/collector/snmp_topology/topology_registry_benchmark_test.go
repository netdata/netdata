// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
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

func BenchmarkSNMPTopologyFDBSnapshotMapTypeScaling(b *testing.B) {
	tests := []struct {
		devices             int
		fdbEntriesPerDevice int
		sharedEndpoints     bool
		mapType             string
	}{
		{devices: 8, fdbEntriesPerDevice: 128},
		{devices: 8, fdbEntriesPerDevice: 128, sharedEndpoints: true},
		{devices: 40, fdbEntriesPerDevice: 1600},
		{devices: 40, fdbEntriesPerDevice: 1600, sharedEndpoints: true},
		{devices: 40, fdbEntriesPerDevice: 1600, mapType: topologyoptions.MapTypeLLDPCDPManaged},
		{devices: 40, fdbEntriesPerDevice: 1600, sharedEndpoints: true, mapType: topologyoptions.MapTypeLLDPCDPManaged},
	}

	for _, tc := range tests {
		mapType := tc.mapType
		if mapType == "" {
			mapType = topologyoptions.MapTypeManagedFabric
		}
		name := fmt.Sprintf(
			"map_type=%s/devices=%d/fdb_entries_per_device=%d/shared_endpoints=%t",
			mapType,
			tc.devices,
			tc.fdbEntriesPerDevice,
			tc.sharedEndpoints,
		)
		b.Run(name, func(b *testing.B) {
			registry := benchmarkManagedFabricFDBTopologyRegistry(tc.devices, tc.fdbEntriesPerDevice, tc.sharedEndpoints)
			options := topologyoptions.DefaultQueryOptions()
			options.MapType = mapType
			snapshot := func() (topologymodel.Data, bool, error) {
				candidates := registry.reverseDNSCandidateCollector()
				data, ok, err := registry.snapshotWithEnvironment(
					options,
					topologyGraphBuildEnvironment{resolveDNSName: candidates.lookupCached},
				)
				runtime.KeepAlive(candidates.collectedCandidates())
				return data, ok, err
			}

			probe, ok, err := snapshot()
			if err != nil || !ok {
				b.Fatalf("default managed-fabric snapshot ok=%t err=%v", ok, err)
			}
			if mapType == topologyoptions.MapTypeManagedFabric && len(probe.Links) == 0 {
				b.Fatal("default managed-fabric snapshot emitted no links")
			}
			if mapType == topologyoptions.MapTypeLLDPCDPManaged && len(probe.Links) != 0 {
				b.Fatalf("legacy LLDP/CDP map emitted %d FDB links", len(probe.Links))
			}
			actorCount := len(probe.Actors)
			linkCount := len(probe.Links)
			probe = topologymodel.Data{}
			payloadBytes, compressedPayloadBytes := benchmarkTopologyPayloadSizes(b, registry, options)

			retainedBytes := benchmarkTopologyRetainedSnapshotBytes(b, snapshot)

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				data, ok, err := snapshot()
				if err != nil || !ok {
					b.Fatalf("default managed-fabric snapshot ok=%t err=%v", ok, err)
				}
				runtime.KeepAlive(data)
			}
			b.ReportMetric(float64(actorCount), "actors/op")
			b.ReportMetric(float64(linkCount), "links/op")
			b.ReportMetric(float64(payloadBytes), "payload-B")
			b.ReportMetric(float64(compressedPayloadBytes), "payload-gzip-B")
			b.ReportMetric(float64(retainedBytes), "retained-B")
		})
	}
}

func BenchmarkSNMPTopologyFDBFunctionRenderingScaling(b *testing.B) {
	tests := []struct {
		devices             int
		fdbEntriesPerDevice int
		sharedEndpoints     bool
	}{
		{devices: 8, fdbEntriesPerDevice: 128},
		{devices: 8, fdbEntriesPerDevice: 128, sharedEndpoints: true},
		{devices: 40, fdbEntriesPerDevice: 1600},
		{devices: 40, fdbEntriesPerDevice: 1600, sharedEndpoints: true},
	}

	for _, tc := range tests {
		name := fmt.Sprintf(
			"devices=%d/fdb_entries_per_device=%d/shared_endpoints=%t",
			tc.devices,
			tc.fdbEntriesPerDevice,
			tc.sharedEndpoints,
		)
		b.Run(name, func(b *testing.B) {
			registry := benchmarkManagedFabricFDBTopologyRegistry(tc.devices, tc.fdbEntriesPerDevice, tc.sharedEndpoints)
			deps := funcDepsAdapter{registry: registry}
			options := topologyoptions.DefaultQueryOptions()

			probe, ok, err := deps.Snapshot(options)
			if err != nil || !ok || probe.Links.Rows == 0 {
				b.Fatalf("Function probe links=%d ok=%t err=%v", probe.Links.Rows, ok, err)
			}
			encoded, err := json.Marshal(probe)
			if err != nil {
				b.Fatalf("encode Function probe: %v", err)
			}
			b.ReportMetric(float64(probe.Actors.Rows), "actors/op")
			b.ReportMetric(float64(probe.Links.Rows), "links/op")
			b.ReportMetric(float64(len(encoded)), "payload-B")

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				payload, ok, err := deps.Snapshot(options)
				if err != nil || !ok {
					b.Fatalf("Function snapshot ok=%t err=%v", ok, err)
				}
				encoded, err := json.Marshal(payload)
				if err != nil {
					b.Fatalf("encode Function payload: %v", err)
				}
				runtime.KeepAlive(encoded)
			}
		})
	}
}

func TestBenchmarkManagedFabricFDBSegmentDensityRegistry(t *testing.T) {
	const (
		deviceCount  = 9
		fdbEntries   = 32
		segmentCount = 8
	)
	registry := benchmarkManagedFabricFDBSegmentDensityRegistry(deviceCount, fdbEntries, segmentCount)
	data, ok, err := registry.snapshotWithOptions(topologyoptions.DefaultQueryOptions())
	require.NoError(t, err)
	require.True(t, ok)

	segments := 0
	for _, actor := range data.Actors {
		if topologymodel.ActorSegmentKind(actor) == topologymodel.SegmentKindBroadcastDomain {
			segments++
		}
	}
	require.Equal(t, segmentCount, segments)
	require.Equal(t, segmentCount*2, testCountTopologyLinksByType(data.Links, "bridge")+testCountTopologyLinksByType(data.Links, "fdb"))
}

func BenchmarkSNMPTopologyFDBSegmentDensityEndToEnd(b *testing.B) {
	tests := []struct {
		devices  int
		fdb      int
		segments int
	}{
		{devices: 129, fdb: 2048, segments: 128},
		{devices: 1025, fdb: 2048, segments: 128},
		{devices: 1025, fdb: 8192, segments: 128},
		{devices: 1025, fdb: 8192, segments: 1024},
	}

	for _, tc := range tests {
		name := fmt.Sprintf("devices=%d/fdb=%d/segments=%d", tc.devices, tc.fdb, tc.segments)
		b.Run(name, func(b *testing.B) {
			registry := benchmarkManagedFabricFDBSegmentDensityRegistry(tc.devices, tc.fdb, tc.segments)
			options := topologyoptions.DefaultQueryOptions()
			snapshot := func() (topologymodel.Data, bool, error) {
				return registry.snapshotWithOptions(options)
			}

			probe, ok, err := snapshot()
			if err != nil || !ok {
				b.Fatalf("high-segment snapshot ok=%t err=%v", ok, err)
			}
			segments := 0
			for _, actor := range probe.Actors {
				if topologymodel.ActorSegmentKind(actor) == topologymodel.SegmentKindBroadcastDomain {
					segments++
				}
			}
			if segments != tc.segments {
				b.Fatalf("broadcast segments=%d, want %d", segments, tc.segments)
			}
			actorCount := len(probe.Actors)
			linkCount := len(probe.Links)
			probe = topologymodel.Data{}

			payloadBytes, compressedPayloadBytes := benchmarkTopologyPayloadSizes(b, registry, options)
			retainedBytes := benchmarkTopologyRetainedSnapshotBytes(b, snapshot)
			deps := funcDepsAdapter{registry: registry}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				payload, ok, err := deps.Snapshot(options)
				if err != nil || !ok {
					b.Fatalf("high-segment Function snapshot ok=%t err=%v", ok, err)
				}
				encoded, err := json.Marshal(payload)
				if err != nil {
					b.Fatalf("encode high-segment Function payload: %v", err)
				}
				runtime.KeepAlive(encoded)
			}
			b.ReportMetric(float64(actorCount), "actors/op")
			b.ReportMetric(float64(linkCount), "links/op")
			b.ReportMetric(float64(tc.segments), "segments/op")
			b.ReportMetric(float64(payloadBytes), "payload-B")
			b.ReportMetric(float64(compressedPayloadBytes), "payload-gzip-B")
			b.ReportMetric(float64(retainedBytes), "retained-B")
		})
	}
}

func benchmarkTopologyPayloadSizes(b *testing.B, registry *topologyRegistry, options topologyoptions.QueryOptions) (int, int) {
	b.Helper()
	payload, ok, err := (funcDepsAdapter{registry: registry}).Snapshot(options)
	if err != nil || !ok {
		b.Fatalf("topology Function payload ok=%t err=%v", ok, err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		b.Fatalf("marshal topology Function payload: %v", err)
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(encoded); err != nil {
		b.Fatalf("compress topology Function payload: %v", err)
	}
	if err := writer.Close(); err != nil {
		b.Fatalf("finish topology Function payload compression: %v", err)
	}
	return len(encoded), compressed.Len()
}

func benchmarkTopologyRetainedSnapshotBytes(
	b *testing.B,
	snapshot func() (topologymodel.Data, bool, error),
) int64 {
	b.Helper()
	// Two cycles clear prior json/gzip sync.Pool victims before measuring the
	// graph objects retained by the snapshots below.
	runtime.GC()
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	const retainedCopies = 4
	retained := make([]topologymodel.Data, retainedCopies)
	for i := range retained {
		var ok bool
		var err error
		retained[i], ok, err = snapshot()
		if err != nil || !ok {
			b.Fatalf("retained-heap snapshot ok=%t err=%v", ok, err)
		}
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	retainedBytes := max(int64(after.HeapAlloc)-int64(before.HeapAlloc), 0)
	retainedBytes /= retainedCopies
	runtime.KeepAlive(retained)
	runtime.GC()
	return retainedBytes
}

func BenchmarkTopologyGenerationFDBSnapshotRead(b *testing.B) {
	registry := benchmarkManagedFabricFDBTopologyRegistry(1, 1600, false)
	generation := registry.acquireGeneration()
	if generation == nil || len(generation.devices) != 1 || !generation.devices[0].hasObservation {
		b.Fatal("benchmark generation is not renderable")
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		generation := registry.acquireGeneration()
		snapshot := generation.devices[0].observation
		runtime.KeepAlive(snapshot)
	}
}

func BenchmarkTopologyGenerationAliasSnapshotRead(b *testing.B) {
	for _, aliases := range []int{256, 4096} {
		b.Run(fmt.Sprintf("aliases=%d", aliases), func(b *testing.B) {
			now := time.Now()
			cache := newTopologyBuilder()
			cache.updateTime = now
			cache.staleAfter = time.Hour
			cache.agentID = "benchmark-agent"
			cache.localDevice = topologymodel.Device{
				ChassisID:     "02:00:00:00:00:01",
				ChassisIDType: "macAddress",
				SysName:       "benchmark-router",
				ManagementIP:  "10.1.0.1",
			}
			for i := range aliases {
				ip := benchmarkAliasIPAddress(0, i)
				cache.localDevice.ManagementAddresses = append(cache.localDevice.ManagementAddresses, topologymodel.ManagementAddress{
					Address:     ip,
					AddressType: "ipv4",
					Source:      "ip_mib",
				})
				cache.ipAddressesByIP[ip] = resolvedIPAddress{
					ifIndex: fmt.Sprintf("%d", i+1),
					netmask: "255.255.255.0",
				}
			}
			generation := freezeTestTopologyBuilder(1, cache)
			if generation == nil || !generation.hasObservation {
				b.Fatal("benchmark cache is not renderable")
			}

			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				snapshot := generation.observation
				runtime.KeepAlive(snapshot)
			}
		})
	}
}

func BenchmarkTopologyBuilderBuildAndFreeze(b *testing.B) {
	for _, fdbEntries := range []int{0, 1600} {
		b.Run(fmt.Sprintf("fdb_entries=%d", fdbEntries), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				now := time.Now()
				builder := newTopologyBuilder()
				builder.updateTime = now
				builder.staleAfter = time.Hour
				builder.agentID = "benchmark-agent"
				builder.localDevice = topologymodel.Device{
					ChassisID:     "02:00:00:00:00:01",
					ChassisIDType: "macAddress",
					SysName:       "benchmark-switch",
					ManagementIP:  "192.0.2.1",
				}
				builder.bridgePortToIf["1"] = "1"
				builder.ifNamesByIndex["1"] = "uplink"
				for entryIndex := range fdbEntries {
					mac := benchmarkManagedFabricEndpointMAC(0, entryIndex+1, false)
					builder.fdbEntries[mac+"|1||"] = &fdbEntry{
						mac: mac, bridgePort: "1", status: "learned",
					}
				}

				snapshot, _ := freezeTopologyBuilder(builder)
				if snapshot == nil || !snapshot.hasObservation {
					b.Fatal("frozen snapshot is not renderable")
				}
				runtime.KeepAlive(snapshot)
			}
		})
	}
}

func BenchmarkSNMPTopologyFunctionDefaultManagedFabricConcurrent(b *testing.B) {
	registry := benchmarkManagedFabricFDBTopologyRegistry(8, 128, true)
	deps := funcDepsAdapter{registry: registry}
	options := topologyoptions.DefaultQueryOptions()
	if payload, ok, err := deps.Snapshot(options); err != nil || !ok || payload.Links.Rows == 0 {
		b.Fatalf("default managed-fabric Function probe rows=%d ok=%t err=%v", payload.Links.Rows, ok, err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	var failureOnce sync.Once
	var failure string
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			payload, ok, err := deps.Snapshot(options)
			if err != nil || !ok {
				failureOnce.Do(func() {
					failure = fmt.Sprintf("concurrent Function snapshot ok=%t err=%v", ok, err)
				})
				return
			}
			runtime.KeepAlive(payload)
		}
	})
	if failure != "" {
		b.Fatal(failure)
	}
}

func BenchmarkSNMPTopologyFunctionDefaultManagedFabricMixedReadPublish(b *testing.B) {
	registry := benchmarkManagedFabricFDBTopologyRegistry(8, 128, true)
	deps := funcDepsAdapter{registry: registry}
	options := topologyoptions.DefaultQueryOptions()
	published := registry.acquireGeneration()
	if published == nil {
		b.Fatal("benchmark generation is missing")
	}
	replacement := &topologyGeneration{
		sequence:          published.sequence + 1,
		publishedAt:       published.publishedAt.Add(time.Second),
		producerScopeID:   published.producerScopeID,
		devices:           append([]*topologyDeviceGeneration(nil), published.devices...),
		renderableDevices: append([]*topologyDeviceGeneration(nil), published.renderableDevices...),
		diagnostic:        published.diagnostic,
	}

	if payload, ok, err := deps.Snapshot(options); err != nil || !ok || payload.Links.Rows == 0 {
		b.Fatalf("default managed-fabric Function probe rows=%d ok=%t err=%v", payload.Links.Rows, ok, err)
	}

	var operations atomic.Uint64
	var publications atomic.Uint64
	var failureOnce sync.Once
	var failure string
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if operations.Add(1)%128 == 0 {
				registry.publishGeneration(replacement)
				publications.Add(1)
			}
			payload, ok, err := deps.Snapshot(options)
			if err != nil || !ok {
				failureOnce.Do(func() {
					failure = fmt.Sprintf("mixed Function snapshot ok=%t err=%v", ok, err)
				})
				return
			}
			runtime.KeepAlive(payload)
		}
	})
	if failure != "" {
		b.Fatal(failure)
	}
	b.ReportMetric(float64(publications.Load()), "publications")
}

func benchmarkManagedFabricFDBTopologyRegistry(deviceCount, fdbEntriesPerDevice int, sharedEndpoints bool) *topologyRegistry {
	registry := newTopologyRegistry()
	now := time.Now()
	for deviceIndex := range deviceCount {
		cache := newTopologyBuilder()
		cache.lastUpdate = now
		cache.updateTime = now
		cache.agentID = fmt.Sprintf("benchmark-agent-%d", deviceIndex)
		cache.localDevice = topologymodel.Device{
			ChassisID:     benchmarkManagedFabricDeviceMAC(deviceIndex),
			ChassisIDType: "macAddress",
			SysName:       fmt.Sprintf("benchmark-switch-%d", deviceIndex),
			ManagementIP:  fmt.Sprintf("192.0.2.%d", deviceIndex+1),
		}
		cache.bridgePortToIf["1"] = "1"
		cache.ifNamesByIndex["1"] = "uplink"

		if fdbEntriesPerDevice > 0 {
			peerMAC := benchmarkManagedFabricDeviceMAC((deviceIndex + 1) % deviceCount)
			cache.fdbEntries[peerMAC+"|1||"] = &fdbEntry{
				mac:        peerMAC,
				bridgePort: "1",
				status:     "learned",
			}
		}
		for entryIndex := 1; entryIndex < fdbEntriesPerDevice; entryIndex++ {
			mac := benchmarkManagedFabricEndpointMAC(deviceIndex, entryIndex, sharedEndpoints)
			cache.fdbEntries[mac+"|1||"] = &fdbEntry{
				mac:        mac,
				bridgePort: "1",
				status:     "learned",
			}
		}
		publishTestTopologyBuilder(registry, cache)
	}
	return registry
}

func benchmarkManagedFabricFDBSegmentDensityRegistry(deviceCount, fdbEntryCount, segmentCount int) *topologyRegistry {
	if deviceCount < segmentCount+1 {
		panic("segment-density benchmark needs one managed peer per segment")
	}
	if segmentCount <= 0 || fdbEntryCount < segmentCount {
		panic("segment-density benchmark needs at least one FDB row per segment")
	}

	registry := newTopologyRegistry()
	now := time.Now()
	caches := make([]*topologyBuilder, 0, deviceCount)
	for deviceIndex := range deviceCount {
		cache := newTopologyBuilder()
		cache.lastUpdate = now
		cache.updateTime = now
		cache.agentID = fmt.Sprintf("segment-benchmark-agent-%d", deviceIndex)
		cache.localDevice = topologymodel.Device{
			ChassisID:     benchmarkManagedFabricDeviceMAC(deviceIndex),
			ChassisIDType: "macAddress",
			SysName:       fmt.Sprintf("segment-benchmark-switch-%d", deviceIndex),
			ManagementIP:  fmt.Sprintf("198.18.%d.%d", deviceIndex/254, deviceIndex%254+1),
		}
		caches = append(caches, cache)
	}

	hub := caches[0]
	for segmentIndex := range segmentCount {
		port := fmt.Sprintf("%d", segmentIndex+1)
		hub.bridgePortToIf[port] = port
		hub.ifNamesByIndex[port] = "uplink-" + port
		peerMAC := caches[segmentIndex+1].localDevice.ChassisID
		key := fmt.Sprintf("managed|%s|%s", peerMAC, port)
		hub.fdbEntries[key] = &fdbEntry{
			mac:        peerMAC,
			bridgePort: port,
			status:     "learned",
		}
	}
	for entryIndex := segmentCount; entryIndex < fdbEntryCount; entryIndex++ {
		segmentIndex := entryIndex % segmentCount
		port := fmt.Sprintf("%d", segmentIndex+1)
		mac := benchmarkManagedFabricEndpointMAC(segmentIndex+1, entryIndex+1, false)
		key := fmt.Sprintf("endpoint|%s|%s|%d", mac, port, entryIndex)
		hub.fdbEntries[key] = &fdbEntry{
			mac:        mac,
			bridgePort: port,
			status:     "learned",
		}
	}
	for _, cache := range caches {
		publishTestTopologyBuilder(registry, cache)
	}
	return registry
}

func benchmarkManagedFabricDeviceMAC(deviceIndex int) string {
	return fmt.Sprintf("02:10:00:00:%02x:%02x", deviceIndex/256, deviceIndex%256)
}

func benchmarkManagedFabricEndpointMAC(deviceIndex, entryIndex int, shared bool) string {
	if shared {
		deviceIndex = 0
	}
	return fmt.Sprintf(
		"02:20:%02x:%02x:%02x:%02x",
		deviceIndex/256,
		deviceIndex%256,
		entryIndex/256,
		entryIndex%256,
	)
}

func benchmarkInferredLLDPTopologyRegistry(linkCount, aliasCount int) *topologyRegistry {
	now := time.Now()
	cache := newTopologyBuilder()
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

	registry := newTopologyRegistry()
	registry.producerScopeID = "benchmark-producer"
	publishTestTopologyBuilder(registry, cache)
	return registry
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
	registry := newTopologyRegistry()
	registry.producerScopeID = "benchmark-producer"
	now := time.Now()
	selectedIPs := make([]string, deviceCount)
	caches := make([]*topologyBuilder, 0, deviceCount)

	for deviceIndex := range deviceCount {
		cache := newTopologyBuilder()
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
		caches = append(caches, cache)
	}

	for deviceIndex, cache := range caches {
		ip := fmt.Sprintf("203.0.113.%d", deviceIndex+1)
		cache.ipAddressesByIP[ip] = resolvedIPAddress{ifIndex: "100", netmask: "255.255.255.0"}
	}

	logicalPeerCount = min(logicalPeerCount, len(caches)-1)
	for peerIndex := 1; peerIndex <= logicalPeerCount; peerIndex++ {
		networkOffset := (peerIndex - 1) * 4
		localIP := fmt.Sprintf("198.51.100.%d", networkOffset+1)
		remoteIP := fmt.Sprintf("198.51.100.%d", networkOffset+2)
		caches[0].ipAddressesByIP[localIP] = resolvedIPAddress{
			ifIndex: fmt.Sprintf("%d", peerIndex),
			netmask: "255.255.255.252",
		}
		caches[peerIndex].ipAddressesByIP[remoteIP] = resolvedIPAddress{
			ifIndex: "1",
			netmask: "255.255.255.252",
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
	for _, cache := range caches {
		cache.finalize()
		publishTestTopologyBuilder(registry, cache)
	}

	return registry
}

func benchmarkAliasIPAddress(deviceIndex, aliasIndex int) string {
	return fmt.Sprintf("10.%d.%d.%d", deviceIndex+1, aliasIndex/254, aliasIndex%254+1)
}
