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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/reversedns"
)

func TestCollectorsShareBorrowedReverseDNSCacheAcrossCleanup(t *testing.T) {
	manager := setMinimalProfileDir(t)
	withTestCacheDir(t)

	var lookups atomic.Int64
	shared := reversedns.New(reversedns.Config{Lookup: func(context.Context, string) ([]string, error) {
		lookups.Add(1)
		return []string{"shared.example.test."}, nil
	}})
	store := ddsnmp.NewDeviceStore()
	topology := snmptopology.NewTrapEnrichmentHandle()
	first := New(store, topology, shared)
	second := New(store, topology, shared)
	first.services.catalog = manager
	first.Name = "first"
	first.Listen.Endpoints = []EndpointConfig{{Protocol: "udp", Address: "127.0.0.1", Port: freeUDPPort(t)}}
	if err := first.Init(context.Background()); err != nil {
		t.Fatalf("initialize first collector: %v", err)
	}
	t.Cleanup(func() { first.Cleanup(context.Background()) })

	firstEntry := &model.TrapEntry{SourceIP: "192.0.2.10"}
	first.services.enricher.Enrich(firstEntry, true)
	requireReverseDNSState(t, shared, netip.MustParseAddr("192.0.2.10"), reversedns.StatePositive)
	if first.job == nil {
		t.Fatal("first collector did not reach a running Job before cleanup")
	}
	first.Cleanup(context.Background())

	secondEntry := &model.TrapEntry{SourceIP: "192.0.2.10"}
	second.services.enricher.Enrich(secondEntry, true)
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
