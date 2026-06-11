// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	snmptopology "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology"
)

func TestEnrichTrapEntryHostnamePriority(t *testing.T) {
	regKey := "key:10.1.2.3:162"
	ddsnmp.DeviceRegistry.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname:      "10.1.2.3",
		SysName:       "core-sw-01",
		VnodeHostname: "core-sw.mydc.example.com",
		Vendor:        "cisco",
		VnodeGUID:     "8f72c1e2-3a4b-5c6d-7e8f-9a0b1c2d3e4f",
	})
	defer ddsnmp.DeviceRegistry.Unregister(regKey)

	dns := newReverseDNSResolver()

	tests := map[string]struct {
		sourceIP      string
		useReverseDNS bool
		wantHostname  string
		wantVendor    string
		wantVnodeID   string
		dnsCache      map[string]string
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
			enrichTrapEntry(entry, tc.useReverseDNS, dns)

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

	prev := trapTopologyEnrichmentForSource
	trapTopologyEnrichmentForSource = func(ip, ifIndex string) *snmptopology.TrapTopologyEnrichment {
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
	}
	t.Cleanup(func() { trapTopologyEnrichmentForSource = prev })

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			regKey := "key:" + tc.info.Hostname + ":162"
			ddsnmp.DeviceRegistry.Register(regKey, tc.info)
			defer ddsnmp.DeviceRegistry.Unregister(regKey)

			dns := newReverseDNSResolver()
			dns.cache[tc.info.Hostname] = reverseDNSCacheEntry{
				name:      "reverse.example.com",
				expiresAt: farFuture(),
			}
			defer dns.Close()

			entry := &TrapEntry{
				SourceIP: tc.info.Hostname,
				Varbinds: []VarbindValue{
					{Name: "ifIndex", OID: ifIndexOIDPrefix + ".1", Type: "InterfaceIndex", Value: int64(1)},
				},
			}
			enrichTrapEntry(entry, true, dns)

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
	regKey := "key:10.1.2.4:162"
	ddsnmp.DeviceRegistry.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname:      "10.1.2.4",
		SysName:       "real-switch",
		VnodeHostname: "unknown",
	})
	defer ddsnmp.DeviceRegistry.Unregister(regKey)

	entry := &TrapEntry{SourceIP: "10.1.2.4"}
	enrichTrapEntry(entry, false, nil)

	if entry.DeviceHostname != "real-switch" {
		t.Errorf("DeviceHostname = %q, want real-switch (unknown vnode hostname treated as unresolved)", entry.DeviceHostname)
	}
}

func TestEnrichTrapEntryEmptySysNameSkipped(t *testing.T) {
	regKey := "key:10.1.2.5:162"
	ddsnmp.DeviceRegistry.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname: "10.1.2.5",
		SysName:  "",
	})
	defer ddsnmp.DeviceRegistry.Unregister(regKey)

	entry := &TrapEntry{SourceIP: "10.1.2.5"}
	enrichTrapEntry(entry, false, nil)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty (empty sysName treated as unresolved)", entry.DeviceHostname)
	}
}

func TestEnrichTrapEntryNoDeviceRegistryMatch(t *testing.T) {
	entry := &TrapEntry{SourceIP: "172.16.0.99"}
	enrichTrapEntry(entry, false, nil)

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

func TestEnrichTrapEntryAmbiguousDeviceRegistryMatchDoesNotEnrich(t *testing.T) {
	ddsnmp.DeviceRegistry.Register("job-a:10.9.9.1:162", ddsnmp.DeviceConnectionInfo{
		Hostname: "10.9.9.1",
		SysName:  "switch-a",
		Vendor:   "vendor-a",
	})
	defer ddsnmp.DeviceRegistry.Unregister("job-a:10.9.9.1:162")
	ddsnmp.DeviceRegistry.Register("job-b:10.9.9.1:162", ddsnmp.DeviceConnectionInfo{
		Hostname: "10.9.9.1",
		SysName:  "switch-b",
		Vendor:   "vendor-b",
	})
	defer ddsnmp.DeviceRegistry.Unregister("job-b:10.9.9.1:162")

	entry := &TrapEntry{SourceIP: "10.9.9.1"}
	enrichTrapEntry(entry, false, nil)

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
	prev := trapTopologyEnrichmentForSource
	trapTopologyEnrichmentForSource = func(_, ifIndex string) *snmptopology.TrapTopologyEnrichment {
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
	}
	t.Cleanup(func() { trapTopologyEnrichmentForSource = prev })

	ddsnmp.DeviceRegistry.Register("job-a:10.9.9.2:162", ddsnmp.DeviceConnectionInfo{
		Hostname:  "10.9.9.2",
		SysName:   "registry-switch",
		VnodeGUID: "registry-vnode-id",
	})
	defer ddsnmp.DeviceRegistry.Unregister("job-a:10.9.9.2:162")

	entry := &TrapEntry{
		SourceIP: "10.9.9.2",
		Varbinds: []VarbindValue{
			{Name: "ifIndex", OID: ifIndexOIDPrefix + ".1", Type: "InterfaceIndex", Value: int64(1)},
		},
	}
	enrichTrapEntry(entry, false, nil)

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
	prev := trapTopologyEnrichmentForSource
	trapTopologyEnrichmentForSource = func(_, _ string) *snmptopology.TrapTopologyEnrichment {
		return nil
	}
	t.Cleanup(func() { trapTopologyEnrichmentForSource = prev })

	entry := &TrapEntry{
		SourceIP: "10.9.9.3",
		Varbinds: []VarbindValue{
			{Name: "ifIndex", OID: ifIndexOIDPrefix + ".29", Type: "InterfaceIndex", Value: int64(29)},
			{Name: "ifName", OID: ifNameOIDPrefix + ".29", Type: "OctetString", Value: "uplink-29"},
		},
	}
	enrichTrapEntry(entry, false, nil)

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
	entry := &TrapEntry{SourceUDPPeer: "192.168.1.1"}
	enrichTrapEntry(entry, false, nil)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty (no device match)", entry.DeviceHostname)
	}
}

func TestEnrichTrapEntryNilEntry(t *testing.T) {
	enrichTrapEntry(nil, false, nil)
}

func TestEnrichTrapEntryNoSource(t *testing.T) {
	entry := &TrapEntry{}
	enrichTrapEntry(entry, false, nil)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty", entry.DeviceHostname)
	}
}

func TestEnrichTrapEntryReverseDNSDefaultOff(t *testing.T) {
	regKey := "key:10.5.5.1:162"
	ddsnmp.DeviceRegistry.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname: "10.5.5.1",
	})
	defer ddsnmp.DeviceRegistry.Unregister(regKey)

	dns := newReverseDNSResolver()
	dns.cache["10.5.5.1"] = reverseDNSCacheEntry{
		name:      "core-sw.mydc.example.com",
		expiresAt: farFuture(),
	}

	entry := &TrapEntry{SourceIP: "10.5.5.1"}
	enrichTrapEntry(entry, false, dns)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty (reverse DNS disabled, no vnode/sysName)", entry.DeviceHostname)
	}
}

func TestEnrichTrapEntryReverseDNSEnabledNoSNMPState(t *testing.T) {
	dns := newReverseDNSResolver()
	dns.cache["10.6.6.1"] = reverseDNSCacheEntry{
		name:      "peer.mydc.example.com",
		expiresAt: farFuture(),
	}

	entry := &TrapEntry{SourceIP: "10.6.6.1"}
	enrichTrapEntry(entry, true, dns)

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
	regKey := "key:10.7.7.1:162"
	ddsnmp.DeviceRegistry.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname: "10.7.7.1",
	})
	defer ddsnmp.DeviceRegistry.Unregister(regKey)

	dns := newReverseDNSResolver()
	dns.cache["10.7.7.1"] = reverseDNSCacheEntry{
		name:      "cached.example.com",
		expiresAt: farFuture(),
	}

	entry := &TrapEntry{SourceIP: "10.7.7.1"}
	enrichTrapEntry(entry, false, dns)

	if entry.DeviceHostname != "" {
		t.Errorf("DeviceHostname = %q, want empty (reverse DNS disabled, no SNMP state)", entry.DeviceHostname)
	}
}

func TestEnrichTrapEntryReverseDNSDoesNotReplaceKnownHostname(t *testing.T) {
	regKey := "key:10.7.7.2:162"
	ddsnmp.DeviceRegistry.Register(regKey, ddsnmp.DeviceConnectionInfo{
		Hostname: "10.7.7.2",
		SysName:  "known-switch",
	})
	defer ddsnmp.DeviceRegistry.Unregister(regKey)

	dns := newReverseDNSResolver()
	dns.cache["10.7.7.2"] = reverseDNSCacheEntry{
		name:      "reverse-known.example.com",
		expiresAt: farFuture(),
	}
	entry := &TrapEntry{SourceIP: "10.7.7.2"}
	enrichTrapEntry(entry, true, dns)

	if entry.DeviceHostname != "known-switch" {
		t.Errorf("DeviceHostname = %q, want known-switch", entry.DeviceHostname)
	}
	if entry.ReverseDNS != "reverse-known.example.com" {
		t.Errorf("ReverseDNS = %q, want reverse-known.example.com", entry.ReverseDNS)
	}
}

func TestEnrichTrapEntryReverseDNSEnabledSchedulesAsyncLookup(t *testing.T) {
	dns := newReverseDNSResolver()
	defer dns.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	dns.lookupAddr = func(ctx context.Context, ip string) ([]string, error) {
		close(started)
		select {
		case <-release:
			return []string{"peer.mydc.example.com."}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	entry := &TrapEntry{SourceIP: "203.0.113.10"}
	enrichTrapEntry(entry, true, dns)

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("reverse DNS lookup was not started")
	}

	dns.mu.RLock()
	_, pending := dns.pending["203.0.113.10"]
	dns.mu.RUnlock()
	if !pending {
		t.Fatal("reverse DNS lookup was not marked pending")
	}

	close(release)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if dns.lookupCached("203.0.113.10") == "peer.mydc.example.com" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("reverse DNS cache was not populated, got %q", dns.lookupCached("203.0.113.10"))
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
			regKey := "key:" + tc.hostname + ":162"
			ddsnmp.DeviceRegistry.Register(regKey, ddsnmp.DeviceConnectionInfo{
				Hostname:  tc.hostname,
				SysName:   tc.sysName,
				Vendor:    tc.vendor,
				VnodeGUID: tc.vnodeGUID,
			})
			defer ddsnmp.DeviceRegistry.Unregister(regKey)

			entry := &TrapEntry{SourceIP: tc.hostname}
			enrichTrapEntry(entry, false, nil)

			if entry.DeviceVendor != tc.wantVendor {
				t.Errorf("DeviceVendor = %q, want %q", entry.DeviceVendor, tc.wantVendor)
			}
			if entry.SourceVnodeID != tc.wantVnodeID {
				t.Errorf("SourceVnodeID = %q, want %q", entry.SourceVnodeID, tc.wantVnodeID)
			}
		})
	}
}

func TestIsUnresolvedSysName(t *testing.T) {
	tests := map[string]struct {
		name string
		want bool
	}{
		"empty_string":       {name: "", want: true},
		"literal_unknown":    {name: "unknown", want: true},
		"upper_unknown":      {name: "UNKNOWN", want: true},
		"mixed_case_unknown": {name: "Unknown", want: true},
		"whitespace_unknown": {name: "  unknown  ", want: true},
		"valid_name":         {name: "core-sw-01", want: false},
		"single_char":        {name: "x", want: false},
	}

	for tcName, tc := range tests {
		t.Run(tcName, func(t *testing.T) {
			if got := isUnresolvedSysName(tc.name); got != tc.want {
				t.Errorf("isUnresolvedSysName(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestReverseDNSLookupCached(t *testing.T) {
	dns := newReverseDNSResolver()

	if got := dns.lookupCached("10.1.2.3"); got != "" {
		t.Errorf("empty cache returned %q, want empty", got)
	}

	dns.cache["10.1.2.3"] = reverseDNSCacheEntry{
		name:      "core-sw.example.com",
		expiresAt: farFuture(),
	}
	if got := dns.lookupCached("10.1.2.3"); got != "core-sw.example.com" {
		t.Errorf("lookupCached = %q, want core-sw.example.com", got)
	}
}

func TestReverseDNSLookupCachedNilResolver(t *testing.T) {
	var dns *reverseDNSResolver
	if got := dns.lookupCached("10.1.2.3"); got != "" {
		t.Errorf("nil resolver returned %q, want empty", got)
	}
}

func TestReverseDNSLookupCachedInvalidIP(t *testing.T) {
	dns := newReverseDNSResolver()
	dns.cache["not-an-ip"] = reverseDNSCacheEntry{
		name:      "should-not-return",
		expiresAt: farFuture(),
	}
	if got := dns.lookupCached("not-an-ip"); got != "" {
		t.Errorf("invalid IP returned %q, want empty", got)
	}
}

func TestReverseDNSResolveAsyncSkipsExisting(t *testing.T) {
	dns := newReverseDNSResolver()
	dns.cache["10.1.2.3"] = reverseDNSCacheEntry{
		name:      "existing.example.com",
		expiresAt: farFuture(),
	}
	dns.resolveAsync("10.1.2.3")

	if got := dns.lookupCached("10.1.2.3"); got != "existing.example.com" {
		t.Errorf("existing entry was overwritten, got %q, want existing.example.com", got)
	}
}

func TestReverseDNSResolveAsyncSkipsPending(t *testing.T) {
	dns := newReverseDNSResolver()
	dns.pending["10.1.2.3"] = struct{}{}

	dns.resolveAsync("10.1.2.3")

	dns.mu.RLock()
	_, stillPending := dns.pending["10.1.2.3"]
	dns.mu.RUnlock()
	if !stillPending {
		t.Fatal("pending entry was unexpectedly removed")
	}
}

func TestReverseDNSResolveAsyncLimitsConcurrentLookups(t *testing.T) {
	dns := newReverseDNSResolver()
	defer dns.Close()
	dns.lookupSem = make(chan struct{}, 1)

	started := make(chan string, 1)
	release := make(chan struct{})
	dns.lookupAddr = func(ctx context.Context, ip string) ([]string, error) {
		started <- ip
		select {
		case <-release:
			return []string{"first.example.com."}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	dns.resolveAsync("10.1.2.3")
	select {
	case ip := <-started:
		if ip != "10.1.2.3" {
			t.Fatalf("lookup started for %s, want first IP", ip)
		}
	case <-time.After(time.Second):
		t.Fatal("first reverse DNS lookup was not started")
	}

	dns.resolveAsync("10.1.2.4")

	dns.mu.RLock()
	_, firstPending := dns.pending["10.1.2.3"]
	_, secondPending := dns.pending["10.1.2.4"]
	activeLookups := len(dns.lookupSem)
	dns.mu.RUnlock()
	if !firstPending {
		t.Fatal("first lookup was not marked pending")
	}
	if secondPending {
		t.Fatal("second lookup was marked pending despite full concurrency limit")
	}
	if activeLookups != 1 {
		t.Fatalf("active lookups = %d, want 1", activeLookups)
	}

	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if dns.lookupCached("10.1.2.3") == "first.example.com" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("first reverse DNS cache was not populated, got %q", dns.lookupCached("10.1.2.3"))
}

func TestReverseDNSResolveAsyncNilResolver(t *testing.T) {
	var dns *reverseDNSResolver
	dns.resolveAsync("10.1.2.3")
}

func TestReverseDNSMaybeSweepExpiredAndCapsCache(t *testing.T) {
	now := time.Now()
	dns := newReverseDNSResolver()
	dns.maxEntries = 2
	dns.cache["10.1.2.1"] = reverseDNSCacheEntry{
		name:      "expired.example.com",
		expiresAt: now.Add(-time.Second),
	}
	dns.cache["10.1.2.2"] = reverseDNSCacheEntry{
		name:      "valid-a.example.com",
		expiresAt: now.Add(time.Hour),
	}
	dns.cache["10.1.2.3"] = reverseDNSCacheEntry{
		name:      "valid-b.example.com",
		expiresAt: now.Add(time.Hour),
	}
	dns.cache["10.1.2.4"] = reverseDNSCacheEntry{
		name:      "valid-c.example.com",
		expiresAt: now.Add(time.Hour),
	}

	dns.maybeSweep(now)

	if _, ok := dns.cache["10.1.2.1"]; ok {
		t.Fatal("expired reverse DNS cache entry was not swept")
	}
	if len(dns.cache) > dns.maxEntries {
		t.Fatalf("cache length = %d, want at most %d", len(dns.cache), dns.maxEntries)
	}
}

func TestReverseDNSTrimCachePrefersNegativeThenOldest(t *testing.T) {
	now := time.Now()
	dns := newReverseDNSResolver()
	dns.maxEntries = 2
	dns.cache["10.1.2.1"] = reverseDNSCacheEntry{
		name:      "newer.example.com",
		expiresAt: now.Add(10 * time.Minute),
	}
	dns.cache["10.1.2.2"] = reverseDNSCacheEntry{
		name:      "",
		expiresAt: now.Add(30 * time.Second),
	}
	dns.cache["10.1.2.3"] = reverseDNSCacheEntry{
		name:      "older.example.com",
		expiresAt: now.Add(time.Minute),
	}
	dns.cache["10.1.2.4"] = reverseDNSCacheEntry{
		name:      "middle.example.com",
		expiresAt: now.Add(5 * time.Minute),
	}

	dns.maybeSweep(now)

	if _, ok := dns.cache["10.1.2.2"]; ok {
		t.Fatal("negative reverse DNS cache entry was not evicted first")
	}
	if _, ok := dns.cache["10.1.2.3"]; ok {
		t.Fatal("oldest positive reverse DNS cache entry was not evicted second")
	}
	if _, ok := dns.cache["10.1.2.1"]; !ok {
		t.Fatal("newest positive reverse DNS cache entry was unexpectedly evicted")
	}
	if _, ok := dns.cache["10.1.2.4"]; !ok {
		t.Fatal("middle positive reverse DNS cache entry was unexpectedly evicted")
	}
}

func TestReverseDNSClosePreventsResolveAsync(t *testing.T) {
	dns := newReverseDNSResolver()
	dns.Close()

	dns.resolveAsync("10.1.2.3")

	dns.mu.RLock()
	_, pending := dns.pending["10.1.2.3"]
	dns.mu.RUnlock()
	if pending {
		t.Fatal("reverse DNS lookup was scheduled after resolver close")
	}
	if got := dns.lookupCached("10.1.2.3"); got != "" {
		t.Errorf("lookupCached after close = %q, want empty", got)
	}
}

func farFuture() time.Time {
	return time.Now().Add(24 * time.Hour)
}
