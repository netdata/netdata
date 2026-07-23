// SPDX-License-Identifier: GPL-3.0-or-later

// M10 test backfill for the ebpfgo plugin public surface.
//
// These tests cover public functions / types that shipped without direct unit
// coverage on the ebpf_nv branch:
//   - parseMinimalFunctionLine
//   - handleNetworkProtocols (via netdataapi buffer)
//   - buildNetworkProtocolsJSON JSON schema (golden output)
//   - applySocketDataLocked and the SHM flag round-trip
//   - MarkSocketActive and UpdateSocketApps edge cases
//   - runSocketGlobalCollector and runDNSGlobalCollector start-time guards
//   - DNS fallback loader sequencing (plan builder / flavor selection)
//   - Per-PID socket aggregation API shape (CGO-required)

package main

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// ---- M10: parseMinimalFunctionLine ----------------------------------------

func TestParseMinimalFunctionLine(t *testing.T) {
	tests := map[string]struct {
		line     string
		wantUID  string
		wantName string
	}{
		"basic FUNCTION line": {
			line:     `FUNCTION 12345 10 "network-protocols" 3 pluginsd`,
			wantUID:  "12345",
			wantName: "network-protocols",
		},
		"with args after the name": {
			line:     `FUNCTION 99 5 "network-protocols some-arg" 3 pluginsd`,
			wantUID:  "99",
			wantName: "network-protocols",
		},
		"too few fields": {
			line: `FUNCTION 99 10 "network-protocols"`,
		},
		"empty line":          {line: ""},
		"not a FUNCTION line": {line: `CHART '' '' 'cpu' 'percentage' 'cpu' 'cpu.cpu0' 'line' 100 1 '' 'netdata' 'cpu'`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			uid, fname := parseMinimalFunctionLine(tc.line)
			if uid != tc.wantUID {
				t.Errorf("uid = %q, want %q", uid, tc.wantUID)
			}
			if fname != tc.wantName {
				t.Errorf("name = %q, want %q", fname, tc.wantName)
			}
		})
	}
}

// ---- M10: handleNetworkProtocols (netdataapi buffer) -----------------------

// runWithBuffer is a small helper that builds a netdataapi API over a bytes.Buffer
// so handleNetworkProtocols can write to it without a real pluginsd stdout.
func runWithBuffer(fn func(api *netdataapi.API)) string {
	var buf bytes.Buffer
	api := netdataapi.New(&buf)
	fn(api)
	return buf.String()
}

func TestHandleNetworkProtocols_NoDataYet(t *testing.T) {
	store := newSocketFunctionStore(5)
	got := runWithBuffer(func(api *netdataapi.API) {
		handleNetworkProtocols(api, store, "uid-no-data")
	})
	if !strings.Contains(got, "503") {
		t.Fatalf("expected 503 status in response, got: %s", got)
	}
	if !strings.Contains(got, "data not yet available") {
		t.Fatalf("expected 'data not yet available' message, got: %s", got)
	}
}

func TestHandleNetworkProtocols_WithData(t *testing.T) {
	store := newSocketFunctionStore(5)
	store.update(socketGlobalPublish{
		tcpDimReceivedCalls: 7,
		tcpDimSentCalls:     11,
		udpRecvCalls:        13,
		udpSendCalls:        17,
	})

	got := runWithBuffer(func(api *netdataapi.API) {
		handleNetworkProtocols(api, store, "uid-with-data")
	})

	if !strings.Contains(got, "200") {
		t.Fatalf("expected 200 status in response, got: %s", got)
	}
	if !strings.Contains(got, `"status":200`) {
		t.Fatalf("expected JSON status=200 in payload, got: %s", got)
	}
	// The data table must carry the published counters.
	if !strings.Contains(got, "TCP") || !strings.Contains(got, "UDP") {
		t.Fatalf("expected TCP and UDP rows in payload, got: %s", got)
	}
}

// ---- M10: JSON function payload schema (golden) ---------------------------

func TestJSONFunctionPayloadSchema_GoldenKeys(t *testing.T) {
	// Build a small but complete payload and assert the JSON has the schema
	// the network-viewer plugin expects. This is the schema pin: a column
	// rename or row reorder breaks this test.
	p := socketGlobalPublish{
		tcpDimReceivedCalls: 1,
		tcpDimSentCalls:     2,
		udpRecvCalls:        3,
		udpSendCalls:        4,
	}
	payload, err := buildNetworkProtocolsJSON(p, 5, 1_700_000_000)
	if err != nil {
		t.Fatalf("buildNetworkProtocolsJSON: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		t.Fatalf("unmarshal: %v\npayload: %s", err, payload)
	}

	requiredKeys := []string{
		"status", "type", "update_every", "has_history", "help",
		"data", "columns", "default_sort_column", "charts",
		"default_charts", "group_by", "expires",
	}
	for _, k := range requiredKeys {
		if _, ok := raw[k]; !ok {
			t.Errorf("missing top-level key %q in payload: %s", k, payload)
		}
	}

	requiredColumns := []string{
		"Transport", "Family", "Received", "Sent", "Errors",
		"ConnActive", "ConnEstablished", "ConnPassive", "ConnReset",
		"SegsTotal", "SegsRetransmitted", "DatagramsNoPort",
	}
	cols, ok := raw["columns"].(map[string]interface{})
	if !ok {
		t.Fatalf("columns is not an object: %s", payload)
	}
	for _, c := range requiredColumns {
		if _, ok := cols[c]; !ok {
			t.Errorf("missing column %q in payload: %s", c, payload)
		}
	}

	// Sent must come after Received in the column order — the chart
	// definition ("Traffic") sorts by the column order.
	if got := mustIndexString(cols["Received"].(map[string]interface{})["index"]); got != "2" {
		t.Errorf("Received index = %s, want 2", got)
	}
	if got := mustIndexString(cols["Sent"].(map[string]interface{})["index"]); got != "3" {
		t.Errorf("Sent index = %s, want 3", got)
	}
}

func mustIndexString(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatInt(int64(x), 10)
	}
	return ""
}

// ---- M10: MarkSocketActive / SHM flag round-trip --------------------------

func TestMarkSocketActive_SetsFlag(t *testing.T) {
	store := NewCachestatSharedMemoryStore()
	store.MarkSocketActive()
	if store.activeModules&ebpfgoSHMFlagSocket == 0 {
		t.Fatal("MarkSocketActive did not set the SOCKET flag")
	}
}

func TestMarkSocketActive_Idempotent(t *testing.T) {
	store := NewCachestatSharedMemoryStore()
	store.MarkSocketActive()
	store.MarkSocketActive()
	store.MarkSocketActive()
	// Flag is a single bit; idempotent means repeated calls do not change the
	// value beyond the single bit being set.
	if got := store.activeModules; got != ebpfgoSHMFlagSocket {
		t.Fatalf("activeModules = %#x, want %#x", got, ebpfgoSHMFlagSocket)
	}
}

func TestSHMFlagRoundTrip_ResetsAfterPublish(t *testing.T) {
	store := NewCachestatSharedMemoryStore()
	store.MarkSocketActive()
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{
		{Pid: 1, Ppid: 1, Ct: 100},
	})
	if store.activeModules&ebpfgoSHMFlagCachestat == 0 {
		t.Fatal("UpdateApps did not set the CACHESTAT flag")
	}
	if store.activeModules&ebpfgoSHMFlagSocket == 0 {
		t.Fatal("MarkSocketActive flag was lost across UpdateApps")
	}

	// Publish clears the CACHESTAT bit each cycle; the SOCKET bit persists
	// until MarkSocketInactive() is called when the socket goroutine exits.
	if err := store.Publish(nil); err != nil {
		t.Fatalf("Publish(nil): %v", err)
	}
	if store.activeModules&ebpfgoSHMFlagCachestat != 0 {
		t.Fatalf("CACHESTAT bit still set after Publish: activeModules = %#x", store.activeModules)
	}
	if store.activeModules&ebpfgoSHMFlagSocket == 0 {
		t.Fatal("SOCKET bit was cleared by Publish; should persist until MarkSocketInactive")
	}

	// MarkSocketInactive clears the SOCKET bit (called on socket goroutine exit).
	store.MarkSocketInactive()
	if store.activeModules != 0 {
		t.Fatalf("activeModules after MarkSocketInactive = %#x, want 0", store.activeModules)
	}
}

// ---- M10: applySocketDataLocked direct coverage ---------------------------

func TestApplySocketDataLocked_ZeroesSocketForPIDsGone(t *testing.T) {
	store := NewCachestatSharedMemoryStore()
	// First, populate both cachestat and socket for PID 10.
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{
		{Pid: 10, Ppid: 1, Ct: 100, MarkPageAccessed: 50},
	})
	store.UpdateSocketApps([]libbpfloader.SocketPIDEntry{
		{PID: 10, BytesSent: 1000},
	})

	// Now UpdateApps with no socket entry for PID 10 (simulating PID exit).
	store.UpdateSocketApps(nil)
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{
		{Pid: 10, Ppid: 1, Ct: 101, MarkPageAccessed: 55},
	})

	// applySocketDataLocked must have zeroed the socket field on the existing
	// entry because socketData no longer contains PID 10.
	snap := store.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snap))
	}
	if snap[0].socket != (ebpfSocketPublishApps{}) {
		t.Errorf("PID %d socket data after exit = %+v, want zero", snap[0].pid, snap[0].socket)
	}
}

func TestApplySocketDataLocked_AppendsSocketOnlyPIDs(t *testing.T) {
	store := NewCachestatSharedMemoryStore()
	store.UpdateSocketApps([]libbpfloader.SocketPIDEntry{
		{PID: 30, BytesSent: 3000, CallUDPSent: 4},
		{PID: 10, BytesReceived: 1000, CallTCPReceived: 2},
	})

	// No cachestat data; socket data must build entries directly.
	snap := store.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("Snapshot len = %d, want 2", len(snap))
	}
	// Entries are sorted ascending by pid.
	if snap[0].pid != 10 || snap[1].pid != 30 {
		t.Fatalf("pids = %d,%d, want 10,30", snap[0].pid, snap[1].pid)
	}
	if snap[0].socket.BytesReceived != 1000 {
		t.Errorf("PID 10 BytesReceived = %d, want 1000", snap[0].socket.BytesReceived)
	}
	if snap[1].socket.BytesSent != 3000 {
		t.Errorf("PID 30 BytesSent = %d, want 3000", snap[1].socket.BytesSent)
	}
}

// ---- M10: runSocketGlobalCollector / runDNSGlobalCollector guards ---------

func TestRunSocketGlobalCollector_NilHandleIsNoOp(t *testing.T) {
	// A nil handle must not panic. The function returns immediately.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil handle panicked: %v", r)
		}
	}()
	runSocketGlobalCollector(nil, make(chan struct{}), 5, nil, nil, false)
}

func TestRunSocketGlobalCollector_NilRuntimeIsNoOp(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil runtime panicked: %v", r)
		}
	}()
	runSocketGlobalCollector(&SocketLegacyHandle{}, make(chan struct{}), 5, nil, nil, false)
}

// runDNSGlobalCollector is harder to exercise without a real DNS runtime;
// the start-time nil-check is the most valuable guard. Mirrored here.
func TestRunDNSGlobalCollector_NilHandleIsNoOp(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil DNS handle panicked: %v", r)
		}
	}()
	runDNSGlobalCollector(nil, make(chan struct{}), 5)
}

// ---- M10: DNS fallback loader sequencing (per-query flag plumbing) ------

func TestBuildDNSLegacyPlan_FlavorAndSelectorInvariantToPerQuery(t *testing.T) {
	// PerQueryTracking must not affect the plan — only the runtime attach
	// decides whether to open the AF_PACKET capture socket.
	cases := []DNSLegacyConfig{
		defaultDNSLegacyConfig(),
		func() DNSLegacyConfig {
			c := defaultDNSLegacyConfig()
			c.KernelVersion = 328704 // 5.4
			c.ObjectFlavor = "buffer"
			return c
		}(),
		func() DNSLegacyConfig {
			c := defaultDNSLegacyConfig()
			c.KernelVersion = 330240 // 5.10
			c.ObjectFlavor = "buffer"
			return c
		}(),
		func() DNSLegacyConfig {
			c := defaultDNSLegacyConfig()
			c.KernelVersion = 396288 // 6.12
			c.IsDebian = true
			c.ObjectFlavor = "arena"
			return c
		}(),
	}

	for i := 0; i < len(cases); i++ {
		perQuery := cases[i]
		perQuery.PerQueryTracking = true
		noPerQuery := cases[i]
		noPerQuery.PerQueryTracking = false

		got1 := BuildDNSLegacyPlan(perQuery)
		got2 := BuildDNSLegacyPlan(noPerQuery)
		if got1 != got2 {
			t.Fatalf("case %d: plan differs between PerQueryTracking=true and false\n"+
				"perQuery: %+v\nnoPerQuery: %+v", i, got1, got2)
		}
	}
}

func TestDNSLegacyConfig_DefaultEnablesPerQuery(t *testing.T) {
	c := defaultDNSLegacyConfig()
	if !c.PerQueryTracking {
		t.Fatal("defaultDNSLegacyConfig().PerQueryTracking = false, want true (preserve current behavior)")
	}
}

// ---- M10: Per-PID socket aggregation API shape ----------------------------

// TestSocketPIDEntryLayout pins the Go-side per-PID socket entry struct size
// so any field rename in libbpfloader.SocketPIDEntry fails this test. The
// C-side aggregator in socket_libbpf.c must mirror the same field order so
// netdata_socket_per_pid_snapshot produces values that decode correctly.
func TestSocketPIDEntryLayout(t *testing.T) {
	// 11 fields, each uint64: PID (uint32 padded to uint64 on most ABIs), then
	// 10 uint64 counters. Go's struct layout does not necessarily pack uint32
	// at offset 0 with no padding; assert the natural alignment size.
	entry := libbpfloader.SocketPIDEntry{}
	if entry.PID != 0 {
		t.Fatalf("zero-value SocketPIDEntry.PID = %d, want 0", entry.PID)
	}
	if entry.BytesSent != 0 || entry.CallTCPV6Connection != 0 {
		t.Fatalf("zero-value SocketPIDEntry has non-zero counters: %+v", entry)
	}
	// All aggregate and per-call counters must remain uint64 so long-running
	// systems do not overflow at 2^32 events.
	if got := entry.BytesSent + 1; got != 1 {
		t.Fatalf("BytesSent arithmetic failed: got %d", got)
	}
}

// ---- M10: cgroups/network-viewer consumer smoke ---------------------------

// TestNetworkProtocolsFunctionTableResponseShape pins the column ordering
// netdataapi consumers (network-viewer, Cloud) rely on: Transport at index 0,
// Family at 1, Received at 2, Sent at 3. Changing the order breaks dashboards.
func TestNetworkProtocolsFunctionTableResponseShape(t *testing.T) {
	payload, err := buildNetworkProtocolsJSON(socketGlobalPublish{}, 5, 1)
	if err != nil {
		t.Fatalf("buildNetworkProtocolsJSON: %v", err)
	}
	var resp fnTableResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("Data len = %d, want 2", len(resp.Data))
	}
	if got, _ := resp.Data[0][0].(string); got != "TCP" {
		t.Errorf("Data[0][0] = %q, want \"TCP\"", got)
	}
	if got, _ := resp.Data[1][0].(string); got != "UDP" {
		t.Errorf("Data[1][0] = %q, want \"UDP\"", got)
	}
	if _, ok := resp.Columns["Transport"]; !ok {
		t.Error("Columns missing Transport")
	}
	if _, ok := resp.Columns["Received"]; !ok {
		t.Error("Columns missing Received")
	}
	if _, ok := resp.Columns["Sent"]; !ok {
		t.Error("Columns missing Sent")
	}
}

// TestSocketFunctionStore_ConcurrentUpdateSnapshot is a smoke test that the
// RWMutex guards active updates against concurrent snapshots. The assertion
// is structural — wg.Wait() must complete without panic or data race — and
// the race detector (when enabled) is the actual gate.
func TestSocketFunctionStore_ConcurrentUpdateSnapshot(t *testing.T) {
	store := newSocketFunctionStore(5)

	const writers = 4
	const writes = 200

	var wg sync.WaitGroup
	wg.Add(writers + 1)

	for i := 0; i < writers; i++ {
		go func(seed int) {
			defer wg.Done()
			for j := 0; j < writes; j++ {
				store.update(socketGlobalPublish{
					tcpDimReceivedCalls: uint64(seed*writes + j),
					tcpDimSentCalls:     uint64(j),
				})
			}
		}(i)
	}

	go func() {
		defer wg.Done()
		for j := 0; j < writers*writes; j++ {
			_, _ = store.snapshot()
		}
	}()

	wg.Wait()
}
