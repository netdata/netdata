// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

const (
	testIfIndexOIDPrefix = "1.3.6.1.2.1.2.2.1.1"
	testIfNameOIDPrefix  = "1.3.6.1.2.1.31.1.1.1.1"
)

func TestEnrichTrapEntryHostnamePriority(t *testing.T) {
	c, store := newTestTrapEnrichmentCollector(nil)
	regKey := "key:10.1.2.3:162"
	store.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname:      "10.1.2.3",
		SysName:       "core-sw-01",
		VnodeHostname: "core-sw.mydc.example.com",
		Vendor:        "cisco",
		VnodeGUID:     "8f72c1e2-3a4b-5c6d-7e8f-9a0b1c2d3e4f",
	})

	tests := map[string]struct {
		sourceIP     string
		wantHostname string
		wantVendor   string
		wantVnodeID  string
	}{
		"vnode_hostname_wins_over_sysname": {
			sourceIP: "10.1.2.3", wantHostname: "core-sw.mydc.example.com",
			wantVendor: "cisco", wantVnodeID: "8f72c1e2-3a4b-5c6d-7e8f-9a0b1c2d3e4f",
		},
		"vnode_hostname_without_sysname": {
			sourceIP: "10.2.3.4", wantHostname: "",
		},
		"empty_source_ip": {
			sourceIP: "", wantHostname: "",
		},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			entry := &TrapEntry{
				SourceIP: tc.sourceIP,
			}
			c.enrichTrapEntry(entry)

			if entry.DeviceHostname != tc.wantHostname {
				t.Errorf("DeviceHostname = %q, want %q", entry.DeviceHostname, tc.wantHostname)
			}
			if tc.wantVendor != "" && entry.DeviceVendor != tc.wantVendor {
				t.Errorf("DeviceVendor = %q, want %q", entry.DeviceVendor, tc.wantVendor)
			}
			if tc.wantVnodeID != "" && entry.SourceVnodeID != tc.wantVnodeID {
				t.Errorf("SourceVnodeID = %q, want %q", entry.SourceVnodeID, tc.wantVnodeID)
			}
		})
	}
}

func TestEnrichTrapEntryRegistryHostnameWinsOverTopologyAndReverseDNS(t *testing.T) {
	tests := map[string]struct {
		info         ddsnmp.DeviceConnectionInfo
		wantHost     string
		wantVendor   string
		wantVnodeID  string
		wantIface    string
		wantNeighbor string
	}{
		"vnode_hostname_wins": {
			info: ddsnmp.DeviceConnectionInfo{
				Hostname:      "10.1.2.6",
				SysName:       "registry-sysname",
				VnodeHostname: "registry-vnode-name",
				Vendor:        "registry-vendor",
				VnodeGUID:     "registry-vnode-id",
			},
			wantHost:     "registry-vnode-name",
			wantVendor:   "registry-vendor",
			wantVnodeID:  "registry-vnode-id",
			wantIface:    "Gi0/1",
			wantNeighbor: "topo-neighbor",
		},
		"sysname_wins_when_vnode_hostname_unknown": {
			info: ddsnmp.DeviceConnectionInfo{
				Hostname:      "10.1.2.7",
				SysName:       "registry-sysname",
				VnodeHostname: "unknown",
			},
			wantHost:     "registry-sysname",
			wantVendor:   "topology-vendor",
			wantVnodeID:  "topology-vnode-id",
			wantIface:    "Gi0/1",
			wantNeighbor: "topo-neighbor",
		},
	}

	topologyEnricher := testTrapTopologyEnricher(func(ip, ifIndex string) *snmptopology.TrapTopologyEnrichment {
		vnodeID := "topology-vnode-id"
		if ip == "10.1.2.6" {
			vnodeID = "registry-vnode-id"
		}
		return &snmptopology.TrapTopologyEnrichment{
			DeviceStatus:    "matched",
			DeviceMethod:    "management_ip",
			DeviceMatches:   1,
			DeviceHostname:  "topology-sysname",
			DeviceVendor:    "topology-vendor",
			SourceVnodeID:   vnodeID,
			InterfaceIndex:  ifIndex,
			InterfaceStatus: "matched",
			Interface:       "Gi0/1",
			NeighborStatus:  "matched",
			Neighbors:       []string{"topo-neighbor"},
		}
	})

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			dns := newTestReverseDNS(map[string]string{tc.info.Hostname: "reverse.example.com"})
			c, store := newTestTrapEnrichmentCollector(topologyEnricher, dns)
			c.reverseDNSEnabled = true
			regKey := "key:" + tc.info.Hostname + ":162"
			store.Register(regKey, tc.info)

			entry := &TrapEntry{
				SourceIP: tc.info.Hostname,
				Varbinds: []VarbindValue{
					{Name: "ifIndex", OID: testIfIndexOIDPrefix + ".1", Type: "InterfaceIndex", Value: int64(1)},
				},
			}
			c.enrichTrapEntry(entry)

			if entry.DeviceHostname != tc.wantHost {
				t.Errorf("DeviceHostname = %q, want %q", entry.DeviceHostname, tc.wantHost)
			}
			if entry.DeviceVendor != tc.wantVendor {
				t.Errorf("DeviceVendor = %q, want %q", entry.DeviceVendor, tc.wantVendor)
			}
			if entry.SourceVnodeID != tc.wantVnodeID {
				t.Errorf("SourceVnodeID = %q, want %q", entry.SourceVnodeID, tc.wantVnodeID)
			}
			if entry.TopologyInterface != tc.wantIface {
				t.Errorf("TopologyInterface = %q, want %q", entry.TopologyInterface, tc.wantIface)
			}
			if entry.TopologyNeighbors != tc.wantNeighbor {
				t.Errorf("TopologyNeighbors = %q, want %q", entry.TopologyNeighbors, tc.wantNeighbor)
			}
		})
	}
}

func TestEnrichTrapEntrySysNameOverVnodeUnknown(t *testing.T) {
	c, store := newTestTrapEnrichmentCollector(nil)
	regKey := "key:10.1.2.4:162"
	store.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname:      "10.1.2.4",
		SysName:       "real-switch",
		VnodeHostname: "unknown",
	})

	entry := &TrapEntry{SourceIP: "10.1.2.4"}
	c.enrichTrapEntry(entry)

	if entry.DeviceHostname != "real-switch" {
		t.Errorf("DeviceHostname = %q, want real-switch (unknown vnode hostname treated as unresolved)", entry.DeviceHostname)
	}
}

func TestEnrichTrapEntryEmptySysNameSkipped(t *testing.T) {
	c, store := newTestTrapEnrichmentCollector(nil)
	regKey := "key:10.1.2.5:162"
	store.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname: "10.1.2.5",
		SysName:  "",
	})

	entry := &TrapEntry{SourceIP: "10.1.2.5"}
	c.enrichTrapEntry(entry)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty (empty sysName treated as unresolved)", entry.DeviceHostname)
	}
}

func TestEnrichTrapEntryNoDeviceStoreMatch(t *testing.T) {
	c, _ := newTestTrapEnrichmentCollector(nil)
	entry := &TrapEntry{SourceIP: "172.16.0.99"}
	c.enrichTrapEntry(entry)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty for unknown device", entry.DeviceHostname)
	}
	if entry.DeviceVendor != "" {
		t.Errorf("DeviceVendor = %q, want empty", entry.DeviceVendor)
	}
	if entry.SourceVnodeID != "" {
		t.Errorf("SourceVnodeID = %q, want empty", entry.SourceVnodeID)
	}
}

func TestEnrichTrapEntryAmbiguousDeviceStoreMatchDoesNotEnrich(t *testing.T) {
	c, store := newTestTrapEnrichmentCollector(nil)
	store.Register("job-a:10.9.9.1:162", ddsnmp.DeviceConnectionInfo{
		Hostname: "10.9.9.1",
		SysName:  "switch-a",
		Vendor:   "vendor-a",
	})
	store.Register("job-b:10.9.9.1:162", ddsnmp.DeviceConnectionInfo{
		Hostname: "10.9.9.1",
		SysName:  "switch-b",
		Vendor:   "vendor-b",
	})

	entry := &TrapEntry{SourceIP: "10.9.9.1"}
	c.enrichTrapEntry(entry)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty for ambiguous registry source", entry.DeviceHostname)
	}
	if entry.DeviceVendor != "" {
		t.Errorf("DeviceVendor = %q, want empty for ambiguous registry source", entry.DeviceVendor)
	}
	if entry.Enrichment == nil || entry.Enrichment.Registry == nil {
		t.Fatal("missing enrichment registry audit")
	}
	if entry.Enrichment.Registry.Status != "ambiguous" || entry.Enrichment.Registry.Matches != 2 {
		t.Fatalf("registry audit = %+v, want ambiguous with 2 matches", entry.Enrichment.Registry)
	}
}

func TestEnrichTrapEntryDoesNotUseTopologyOnVnodeConflict(t *testing.T) {
	topologyEnricher := testTrapTopologyEnricher(func(_, ifIndex string) *snmptopology.TrapTopologyEnrichment {
		return &snmptopology.TrapTopologyEnrichment{
			DeviceStatus:    "matched",
			DeviceMethod:    "management_ip",
			DeviceMatches:   1,
			DeviceHostname:  "topology-sysname",
			DeviceVendor:    "topology-vendor",
			SourceVnodeID:   "topology-vnode-id",
			InterfaceIndex:  ifIndex,
			InterfaceStatus: "matched",
			Interface:       "Gi0/1",
			NeighborStatus:  "matched",
			Neighbors:       []string{"dist-a"},
		}
	})

	c, store := newTestTrapEnrichmentCollector(topologyEnricher)
	store.Register("job-a:10.9.9.2:162", ddsnmp.DeviceConnectionInfo{
		Hostname:  "10.9.9.2",
		SysName:   "registry-switch",
		VnodeGUID: "registry-vnode-id",
	})

	entry := &TrapEntry{
		SourceIP: "10.9.9.2",
		Varbinds: []VarbindValue{
			{Name: "ifIndex", OID: testIfIndexOIDPrefix + ".1", Type: "InterfaceIndex", Value: int64(1)},
		},
	}
	c.enrichTrapEntry(entry)

	if entry.DeviceHostname != "registry-switch" {
		t.Errorf("DeviceHostname = %q, want registry-switch", entry.DeviceHostname)
	}
	if entry.TopologyInterface != "" {
		t.Errorf("TopologyInterface = %q, want empty on vnode conflict", entry.TopologyInterface)
	}
	if entry.TopologyNeighbors != "" {
		t.Errorf("TopologyNeighbors = %q, want empty on vnode conflict", entry.TopologyNeighbors)
	}
	if entry.Enrichment == nil || entry.Enrichment.Topology == nil {
		t.Fatal("missing topology audit")
	}
	if entry.Enrichment.Topology.Status != "conflict" || entry.Enrichment.Topology.Reason != "vnode_mismatch" {
		t.Fatalf("topology audit = %+v, want vnode conflict", entry.Enrichment.Topology)
	}
}

func TestEnrichTrapEntryUsesTrapVarbindInterfaceWithoutTopology(t *testing.T) {
	topologyEnricher := testTrapTopologyEnricher(func(_, _ string) *snmptopology.TrapTopologyEnrichment {
		return nil
	})
	c, _ := newTestTrapEnrichmentCollector(topologyEnricher)

	entry := &TrapEntry{
		SourceIP: "10.9.9.3",
		Varbinds: []VarbindValue{
			{Name: "ifIndex", OID: testIfIndexOIDPrefix + ".29", Type: "InterfaceIndex", Value: int64(29)},
			{Name: "ifName", OID: testIfNameOIDPrefix + ".29", Type: "OctetString", Value: "uplink-29"},
		},
	}
	c.enrichTrapEntry(entry)

	if entry.TopologyInterface != "uplink-29" {
		t.Errorf("TopologyInterface = %q, want uplink-29", entry.TopologyInterface)
	}
	if entry.TopologyNeighbors != "" {
		t.Errorf("TopologyNeighbors = %q, want empty without exact topology device", entry.TopologyNeighbors)
	}
	if entry.Enrichment == nil || entry.Enrichment.Interface == nil {
		t.Fatal("missing interface audit")
	}
	if entry.Enrichment.Interface.Method != "trap_varbind" || entry.Enrichment.Interface.Status != "matched" {
		t.Fatalf("interface audit = %+v, want trap_varbind matched", entry.Enrichment.Interface)
	}
	if entry.Enrichment.Neighbors == nil || entry.Enrichment.Neighbors.Reason != "no_exact_topology_device_match" {
		t.Fatalf("neighbors audit = %+v, want skipped without exact topology device", entry.Enrichment.Neighbors)
	}
}

func TestEnrichTrapEntrySourceUDPPeerFallback(t *testing.T) {
	c, _ := newTestTrapEnrichmentCollector(nil)
	entry := &TrapEntry{SourceUDPPeer: "192.168.1.1"}
	c.enrichTrapEntry(entry)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty (no device match)", entry.DeviceHostname)
	}
}

func TestEnrichTrapEntryNilEntry(t *testing.T) {
	c, _ := newTestTrapEnrichmentCollector(nil)
	c.enrichTrapEntry(nil)
}

func TestEnrichTrapEntryNoSource(t *testing.T) {
	c, _ := newTestTrapEnrichmentCollector(nil)
	entry := &TrapEntry{}
	c.enrichTrapEntry(entry)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty", entry.DeviceHostname)
	}
}

func TestEnrichTrapEntryReverseDNSDefaultOff(t *testing.T) {
	dns := newTestReverseDNS(map[string]string{"10.5.5.1": "core-sw.mydc.example.com"})
	c, store := newTestTrapEnrichmentCollector(nil, dns)
	regKey := "key:10.5.5.1:162"
	store.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname: "10.5.5.1",
	})

	entry := &TrapEntry{SourceIP: "10.5.5.1"}
	c.enrichTrapEntry(entry)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty (reverse DNS disabled, no vnode/sysName)", entry.DeviceHostname)
	}
	if lookups, schedules := dns.callCounts(); lookups != 0 || schedules != 0 {
		t.Fatalf("reverse DNS calls = (%d lookups, %d schedules), want none while disabled", lookups, schedules)
	}
}

func TestEnrichTrapEntryReverseDNSEnabledNoSNMPState(t *testing.T) {
	dns := newTestReverseDNS(map[string]string{"10.6.6.1": "peer.mydc.example.com"})
	c, _ := newTestTrapEnrichmentCollector(nil, dns)
	c.reverseDNSEnabled = true

	entry := &TrapEntry{SourceIP: "10.6.6.1"}
	c.enrichTrapEntry(entry)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty because reverse DNS is not authoritative identity", entry.DeviceHostname)
	}
	if entry.ReverseDNS != "peer.mydc.example.com" {
		t.Errorf("ReverseDNS = %q, want peer.mydc.example.com", entry.ReverseDNS)
	}
	if entry.Enrichment == nil || entry.Enrichment.ReverseDNS == nil {
		t.Fatal("missing reverse DNS audit")
	}
	if entry.Enrichment.ReverseDNS.Value != "peer.mydc.example.com" {
		t.Fatalf("reverse DNS audit = %+v, want cached value", entry.Enrichment.ReverseDNS)
	}
}

func TestEnrichTrapEntryReverseDNSDisabledNoCacheUse(t *testing.T) {
	dns := newTestReverseDNS(map[string]string{"10.7.7.1": "cached.example.com"})
	c, store := newTestTrapEnrichmentCollector(nil, dns)
	regKey := "key:10.7.7.1:162"
	store.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname: "10.7.7.1",
	})

	entry := &TrapEntry{SourceIP: "10.7.7.1"}
	c.enrichTrapEntry(entry)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty (reverse DNS disabled, no SNMP state)", entry.DeviceHostname)
	}
	if lookups, schedules := dns.callCounts(); lookups != 0 || schedules != 0 {
		t.Fatalf("reverse DNS calls = (%d lookups, %d schedules), want none while disabled", lookups, schedules)
	}
}

func TestEnrichTrapEntryReverseDNSDoesNotReplaceKnownHostname(t *testing.T) {
	dns := newTestReverseDNS(map[string]string{"10.7.7.2": "reverse-known.example.com"})
	c, store := newTestTrapEnrichmentCollector(nil, dns)
	c.reverseDNSEnabled = true
	regKey := "key:10.7.7.2:162"
	store.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname: "10.7.7.2",
		SysName:  "known-switch",
	})

	entry := &TrapEntry{SourceIP: "10.7.7.2"}
	c.enrichTrapEntry(entry)

	if entry.DeviceHostname != "known-switch" {
		t.Errorf("DeviceHostname = %q, want known-switch", entry.DeviceHostname)
	}
	if entry.ReverseDNS != "reverse-known.example.com" {
		t.Errorf("ReverseDNS = %q, want reverse-known.example.com", entry.ReverseDNS)
	}
}

func TestEnrichTrapEntryReverseDNSEnabledSchedulesCacheMiss(t *testing.T) {
	dns := newTestReverseDNS(nil)
	c, _ := newTestTrapEnrichmentCollector(nil, dns)
	c.reverseDNSEnabled = true
	entry := &TrapEntry{SourceIP: "203.0.113.10"}
	c.enrichTrapEntry(entry)

	if entry.Enrichment == nil || entry.Enrichment.ReverseDNS == nil {
		t.Fatal("missing reverse DNS audit")
	}
	if got := entry.Enrichment.ReverseDNS.Status; got != "pending" {
		t.Fatalf("reverse DNS status = %q, want pending", got)
	}
	if lookups, schedules := dns.callCounts(); lookups != 1 || schedules != 1 {
		t.Fatalf("reverse DNS calls = (%d lookups, %d schedules), want (1, 1)", lookups, schedules)
	}
}

func TestCollectorsShareBorrowedReverseDNSCacheAcrossCleanup(t *testing.T) {
	var lookups atomic.Int64
	shared := reversedns.New(reversedns.Config{Lookup: func(context.Context, string) ([]string, error) {
		lookups.Add(1)
		return []string{"shared.example.test."}, nil
	}})
	store := ddsnmp.NewDeviceStore()
	topology := snmptopology.NewTrapEnrichmentHandle()
	first := New(store, topology, shared)
	second := New(store, topology, shared)
	first.reverseDNSEnabled = true
	second.reverseDNSEnabled = true

	firstEntry := &TrapEntry{SourceIP: "192.0.2.10"}
	first.enrichTrapEntry(firstEntry)
	requireReverseDNSState(t, shared, netip.MustParseAddr("192.0.2.10"), reversedns.StatePositive)
	first.Cleanup(context.Background())

	secondEntry := &TrapEntry{SourceIP: "192.0.2.10"}
	second.enrichTrapEntry(secondEntry)
	if got := secondEntry.ReverseDNS; got != "shared.example.test" {
		t.Fatalf("ReverseDNS = %q, want shared.example.test", got)
	}
	if got := lookups.Load(); got != 1 {
		t.Fatalf("PTR lookups = %d, want one shared lookup", got)
	}
}

func requireReverseDNSState(t *testing.T, resolver *reversedns.Resolver, addr netip.Addr, want reversedns.State) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if result := resolver.Lookup(addr); result.State == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("reverse DNS state = %v, want %v", resolver.Lookup(addr).State, want)
}

func TestEnrichTrapEntryVendorAndVnodeEnrichment(t *testing.T) {
	tests := map[string]struct {
		hostname    string
		sysName     string
		vendor      string
		vnodeGUID   string
		wantVendor  string
		wantVnodeID string
	}{
		"vendor_and_vnode_set": {
			hostname: "10.8.8.1", sysName: "switch1",
			vendor: "juniper", vnodeGUID: "abcd-1234",
			wantVendor: "juniper", wantVnodeID: "abcd-1234",
		},
		"vendor_empty_omitted": {
			hostname: "10.8.8.2", sysName: "switch2",
			vendor: "", vnodeGUID: "efgh-5678",
			wantVendor: "", wantVnodeID: "efgh-5678",
		},
		"vnode_empty_omitted": {
			hostname: "10.8.8.3", sysName: "switch3",
			vendor: "arista", vnodeGUID: "",
			wantVendor: "arista", wantVnodeID: "",
		},
		"both_empty": {
			hostname: "10.8.8.4", sysName: "switch4",
			vendor: "", vnodeGUID: "",
			wantVendor: "", wantVnodeID: "",
		},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			c, store := newTestTrapEnrichmentCollector(nil)
			regKey := "key:" + tc.hostname + ":162"
			store.Register(regKey, ddsnmp.DeviceConnectionInfo{
				Hostname:  tc.hostname,
				SysName:   tc.sysName,
				Vendor:    tc.vendor,
				VnodeGUID: tc.vnodeGUID,
			})

			entry := &TrapEntry{SourceIP: tc.hostname}
			c.enrichTrapEntry(entry)

			if entry.DeviceVendor != tc.wantVendor {
				t.Errorf("DeviceVendor = %q, want %q", entry.DeviceVendor, tc.wantVendor)
			}
			if entry.SourceVnodeID != tc.wantVnodeID {
				t.Errorf("SourceVnodeID = %q, want %q", entry.SourceVnodeID, tc.wantVnodeID)
			}
		})
	}
}
