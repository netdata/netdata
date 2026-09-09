package main

import (
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

func TestBuildDCStatPublish(t *testing.T) {
	current := netdataPublishDCStatPid{CacheAccess: 1100, FileSystem: 260, NotFound: 60}
	previous := netdataPublishDCStatPid{CacheAccess: 1000, FileSystem: 200, NotFound: 50}

	got := buildDCStatPublish(current, previous, 1234, true, 10)
	want := netdataPublishDCStat{
		Ct:             1234,
		Ratio:          90, // interval: reference +100, not_found +10
		CacheAccess:    100,
		Curr:           current,
		Prev:           previous,
		UpdateEverySec: 10,
	}

	if got != want {
		t.Fatalf("buildDCStatPublish() = %+v, want %+v", got, want)
	}
}

func TestBuildDCStatPublishIdleRatioIsZero(t *testing.T) {
	// The C dcstat collector reports 0 when there were no lookups this interval;
	// cachestat deliberately reports 100 for its idle case, so the two must not
	// be unified.
	current := netdataPublishDCStatPid{CacheAccess: 1000, FileSystem: 200, NotFound: 50}

	got := buildDCStatPublish(current, current, 1234, true, 10)
	if got.Ratio != 0 {
		t.Fatalf("idle ratio = %d, want 0", got.Ratio)
	}
	if got.CacheAccess != 0 {
		t.Fatalf("idle cache_access delta = %d, want 0", got.CacheAccess)
	}
}

// TestBuildDCStatPublishFirstSampleSelfBaselines pins the anti-spike rule: with
// no previous reading, Prev must equal Curr so a consumer computing Curr-Prev
// sees zero instead of the process's entire pre-existing counter history.
func TestBuildDCStatPublishFirstSampleSelfBaselines(t *testing.T) {
	current := netdataPublishDCStatPid{CacheAccess: 1000, FileSystem: 200, NotFound: 50}

	got := buildDCStatPublish(current, netdataPublishDCStatPid{}, 7, false, 10)
	if got.Ratio != 0 || got.CacheAccess != 0 {
		t.Fatalf("first sample = %+v, want zero ratio and delta", got)
	}
	if got.Curr != current {
		t.Fatalf("first sample must still carry the raw counters, got %+v", got.Curr)
	}
	if got.Prev != current {
		t.Fatalf("first sample Prev = %+v, want it baselined to Curr %+v", got.Prev, current)
	}
}

func TestDCStatStoreUpdateApps(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{
			Pid:         10,
			Ppid:        1,
			Ct:          100,
			CacheAccess: 40,
			FileSystem:  10,
			NotFound:    5,
			Comm:        [libbpfloader.DCStatAppCommLen]byte{'a', 'l', 'p', 'h', 'a'},
		},
		{
			Pid:         20,
			Ppid:        3,
			Ct:          200,
			CacheAccess: 80,
			FileSystem:  20,
			NotFound:    8,
			Comm:        [libbpfloader.DCStatAppCommLen]byte{'b', 'e', 't', 'a'},
		},
	}, 10)

	got := store.Snapshot()
	if len(got) != 2 {
		t.Fatalf("Snapshot() len = %d, want 2", len(got))
	}
	if got[0].pid != 10 || got[1].pid != 20 {
		t.Fatalf("Snapshot() pids = %d,%d, want 10,20", got[0].pid, got[1].pid)
	}
	if got[0].ppid != 1 || got[0].comm[0] != 'a' {
		t.Fatalf("Snapshot()[0] identity not copied: ppid=%d comm=%q", got[0].ppid, got[0].comm[0])
	}
	if got[0].dc.Curr.CacheAccess != 40 || got[1].dc.Curr.NotFound != 8 {
		t.Fatalf("Snapshot() raw counters were not copied: %+v / %+v", got[0].dc, got[1].dc)
	}
	if got[0].dc.Prev != got[0].dc.Curr {
		t.Fatalf("Snapshot()[0] first update must self-baseline Prev to Curr, got %+v", got[0].dc.Prev)
	}
	if store.activeModules&ebpfgoSHMFlagDCStat == 0 {
		t.Fatal("UpdateDCStatApps did not set the DCSTAT flag")
	}
}

func TestDCStatStoreFlagsStaleAfterStaleCycles(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	app := libbpfloader.DCStatAppSnapshot{Pid: 42, Ppid: 1, Ct: 100, CacheAccess: 10}

	// First call establishes the ct baseline — no stale PIDs yet.
	if stale := store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app}, 10); len(stale) != 0 {
		t.Fatalf("cycle 0 (baseline): unexpected stale %v", stale)
	}

	for i := 1; i < ebpfStaleCycles; i++ {
		if stale := store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app}, 10); len(stale) != 0 {
			t.Fatalf("cycle %d: unexpected stale %v (threshold not yet reached)", i, stale)
		}
		if len(store.Snapshot()) != 1 {
			t.Fatalf("cycle %d: PID 42 should still be present", i)
		}
	}

	stale := store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app}, 10)
	if len(stale) != 1 || stale[0] != 42 {
		t.Fatalf("expected stale candidate for PID 42, got stale=%v", stale)
	}
}

func TestDCStatStoreNoFlagWhenCountersAdvance(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	// The BPF ct is held fixed on purpose: it is not the freshness signal, so only
	// counter movement may clear the stale flag.  See TestDCStatFreshnessIgnoresBPFCt.
	app := libbpfloader.DCStatAppSnapshot{Pid: 7, Ct: 100, CacheAccess: 10}

	for range ebpfStaleCycles {
		store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app}, 10)
	}

	app.CacheAccess = 20
	if stale := store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app}, 10); len(stale) != 0 {
		t.Fatalf("expected no stale flag after the counters advanced, got stale=%v", stale)
	}
}

// TestDCStatStoreEvictsExitedPIDs verifies that the entry rebuild drops a PID
// that disappears from the dcstat snapshot, instead of leaving it in shared
// memory forever.
func TestDCStatStoreEvictsExitedPIDs(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 10, Ct: 100, CacheAccess: 5},
		{Pid: 20, Ct: 100, CacheAccess: 5},
	}, 10)
	if len(store.Snapshot()) != 2 {
		t.Fatalf("cycle 1: Snapshot() len = %d, want 2", len(store.Snapshot()))
	}

	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 20, Ct: 200, CacheAccess: 9},
	}, 10)
	snap := store.Snapshot()
	if len(snap) != 1 || snap[0].pid != 20 {
		t.Fatalf("cycle 2: Snapshot() = %+v, want only PID 20", snap)
	}
}

func TestDCStatStoreRemovesDeletedPIDBaseline(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{{Pid: 10, CacheAccess: 1000, NotFound: 100}}, 10)
	store.RemoveDCStatPIDs([]uint32{10})

	// PID 10 has been reused by a new process with lower counters. It must be
	// self-baselined rather than clamped forever against the exited process.
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{{Pid: 10, CacheAccess: 5, NotFound: 1}}, 10)
	snap := store.Snapshot()
	if len(snap) != 1 || snap[0].dc.Prev != snap[0].dc.Curr {
		t.Fatalf("reused PID was not self-baselined: %+v", snap)
	}

	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{{Pid: 10, CacheAccess: 15, NotFound: 2}}, 10)
	snap = store.Snapshot()
	if snap[0].dc.CacheAccess != 10 || snap[0].dc.Ratio != 90 {
		t.Fatalf("reused PID interval = %+v, want delta 10 and ratio 90", snap[0].dc)
	}
}

// TestSharedStoreMergesAllThreeModules verifies that cachestat, dcstat, and
// socket rows land on the same PID row, and that PIDs seen by only one module
// still appear.
func TestSharedStoreMergesAllThreeModules(t *testing.T) {
	store := NewEbpfSharedMemoryStore()

	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{
		{Pid: 10, Ppid: 1, Ct: 100, MarkPageAccessed: 50},
		{Pid: 30, Ppid: 1, Ct: 100, MarkPageAccessed: 10},
	})
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 10, Ppid: 1, Ct: 100, CacheAccess: 7},
		{Pid: 20, Ppid: 1, Ct: 100, CacheAccess: 9},
	}, 10)
	// Two cycles so the socket delta is non-zero on the second one.
	store.UpdateSocketApps([]libbpfloader.SocketPIDEntry{{PID: 10}, {PID: 40}}, 10)
	store.UpdateSocketApps([]libbpfloader.SocketPIDEntry{{PID: 10, BytesSent: 1000}, {PID: 40, BytesSent: 5}}, 10)

	snap := store.Snapshot()
	if len(snap) != 4 {
		t.Fatalf("Snapshot() len = %d, want 4 (pids 10,20,30,40)", len(snap))
	}
	for i, pid := range []uint32{10, 20, 30, 40} {
		if snap[i].pid != pid {
			t.Fatalf("Snapshot()[%d].pid = %d, want %d (entries must stay sorted)", i, snap[i].pid, pid)
		}
	}

	if snap[0].cachestat.Current.MarkPageAccessed != 50 {
		t.Fatalf("PID 10 lost its cachestat data: %+v", snap[0].cachestat)
	}
	if snap[0].dc.Curr.CacheAccess != 7 {
		t.Fatalf("PID 10 lost its dcstat data: %+v", snap[0].dc)
	}
	if snap[0].socket.BytesSent != 1000 {
		t.Fatalf("PID 10 lost its socket data: %+v", snap[0].socket)
	}
	if snap[1].dc.Curr.CacheAccess != 9 || snap[1].cachestat != (netdataPublishCachestat{}) {
		t.Fatalf("PID 20 should carry dcstat data only, got %+v / %+v", snap[1].dc, snap[1].cachestat)
	}
	if snap[3].socket.BytesSent != 5 {
		t.Fatalf("PID 40 (socket-only) socket data = %+v, want bytes_sent=5", snap[3].socket)
	}

	// Every module flag must be set, and Publish must clear only the caller's.
	wantFlags := ebpfgoSHMFlagCachestat | ebpfgoSHMFlagDCStat | ebpfgoSHMFlagSocket
	if store.activeModules != wantFlags {
		t.Fatalf("activeModules = %#x, want %#x", store.activeModules, wantFlags)
	}
	if err := store.Publish(nil, ebpfgoSHMFlagDCStat); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if store.activeModules != (ebpfgoSHMFlagCachestat | ebpfgoSHMFlagSocket) {
		t.Fatalf("activeModules after dcstat publish = %#x, want cachestat|socket", store.activeModules)
	}
}

func TestMarkDCStatInactiveClearsOnlyDCStat(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{{Pid: 1, Ct: 1}})
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{{Pid: 1, Ct: 1}}, 10)

	store.MarkDCStatInactive()
	if store.activeModules&ebpfgoSHMFlagDCStat != 0 {
		t.Fatal("MarkDCStatInactive did not clear the DCSTAT flag")
	}
	if store.activeModules&ebpfgoSHMFlagCachestat == 0 {
		t.Fatal("MarkDCStatInactive cleared the CACHESTAT flag")
	}
}

// TestDCStatStoreKeepsBaselineForIdlePID guards the idle-but-alive case: a PID
// flagged stale must keep its counter baseline, so the first activity after the
// idle window is reported as a delta instead of being swallowed as a "first
// sample".
func TestDCStatStoreKeepsBaselineForIdlePID(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	app := libbpfloader.DCStatAppSnapshot{Pid: 42, Ct: 100, CacheAccess: 1000, NotFound: 100}
	// A second, continuously active PID keeps the next-cycle map non-empty. Without
	// it the baseline-rotation guard skips the rotation entirely and the assertion
	// below would hold even if the stale branch dropped the baseline.
	busy := libbpfloader.DCStatAppSnapshot{Pid: 43, CacheAccess: 1}

	// Baseline, then enough unchanged cycles to be flagged as a stale candidate.
	for range ebpfStaleCycles + 1 {
		busy.CacheAccess += 10
		store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app, busy}, 10)
	}
	if snap := store.Snapshot(); len(snap) != 1 || snap[0].pid != 43 {
		t.Fatalf("a stale candidate must not be published as a live row, got %+v", snap)
	}

	// The process was only idle: it wakes up and does 100 more lookups, 10 missed.
	// The BPF ct stays where it was — the counters alone must revive the row.
	app.CacheAccess = 1100
	app.NotFound = 110
	busy.CacheAccess += 10
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app, busy}, 10)

	snap := store.Snapshot()
	if len(snap) != 2 || snap[0].pid != 42 {
		t.Fatalf("Snapshot() = %+v, want PID 42 back as a live row", snap)
	}
	if snap[0].dc.CacheAccess != 100 {
		t.Fatalf("cache_access delta = %d, want 100 (baseline must survive the idle window)",
			snap[0].dc.CacheAccess)
	}
	if snap[0].dc.Ratio != 90 {
		t.Fatalf("ratio = %d, want 90", snap[0].dc.Ratio)
	}
}

// TestClearDCStatAppsDropsRowsAndFlag covers the failed-snapshot path: dcstat's
// rows and flag must both go away so the module that owns the segment cannot
// publish last cycle's directory-cache values as live data.
func TestClearDCStatAppsDropsRowsAndFlag(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{{Pid: 10, Ppid: 1, Ct: 100, MarkPageAccessed: 5}})
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 10, Ppid: 1, Ct: 100, CacheAccess: 1000, NotFound: 100},
		{Pid: 20, Ppid: 1, Ct: 100, CacheAccess: 50},
	}, 10)

	store.ClearDCStatApps()

	if store.activeModules&ebpfgoSHMFlagDCStat != 0 {
		t.Fatal("ClearDCStatApps did not clear the DCSTAT flag")
	}
	if store.activeModules&ebpfgoSHMFlagCachestat == 0 {
		t.Fatal("ClearDCStatApps cleared another module's flag")
	}

	snap := store.Snapshot()
	// PID 20 was dcstat-only, so it disappears; PID 10 survives via cachestat but
	// must carry no directory-cache values.
	if len(snap) != 1 || snap[0].pid != 10 {
		t.Fatalf("Snapshot() = %+v, want only PID 10", snap)
	}
	if snap[0].dc != (netdataPublishDCStat{}) {
		t.Fatalf("PID 10 dc data after clear = %+v, want zero", snap[0].dc)
	}
	if snap[0].cachestat.Current.MarkPageAccessed != 5 {
		t.Fatalf("PID 10 lost its cachestat data: %+v", snap[0].cachestat)
	}
}

// TestClearDCStatAppsKeepsBaseline verifies a recovered cycle still publishes a
// real delta: the counter baseline must survive a failed collection cycle.
func TestClearDCStatAppsKeepsBaseline(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 10, Ct: 100, CacheAccess: 1000, NotFound: 100},
	}, 10)

	store.ClearDCStatApps() // failed cycle

	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 10, Ct: 200, CacheAccess: 1100, NotFound: 110},
	}, 10)

	snap := store.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot() len = %d, want 1", len(snap))
	}
	if snap[0].dc.CacheAccess != 100 {
		t.Fatalf("cache_access delta = %d, want 100 (baseline must survive a failed cycle)",
			snap[0].dc.CacheAccess)
	}
	if snap[0].dc.Ratio != 90 {
		t.Fatalf("ratio = %d, want 90", snap[0].dc.Ratio)
	}
}

// TestDCStatStoreEmptySnapshotPreservesBaseline pins the fix for a real data
// loss: SnapshotApps returns a nil slice for an empty BPF map/accumulator, and
// rotating that empty map into the baseline discarded every PID's previous
// counters, so the next active cycle re-baselined and silently dropped one
// interval of activity.
func TestDCStatStoreEmptySnapshotPreservesBaseline(t *testing.T) {
	store := NewEbpfSharedMemoryStore()

	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 10, Ct: 100, CacheAccess: 1000, NotFound: 100},
	}, 10)
	store.UpdateDCStatApps(nil, 10) // empty cycle: nothing to publish, baseline must survive
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 10, Ct: 200, CacheAccess: 1100, NotFound: 110},
	}, 10)

	snap := store.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot() len = %d, want 1", len(snap))
	}
	if snap[0].dc.CacheAccess != 100 {
		t.Fatalf("cache_access delta = %d, want 100 (baseline must survive an empty cycle)",
			snap[0].dc.CacheAccess)
	}
	if snap[0].dc.Ratio != 90 {
		t.Fatalf("ratio = %d, want 90", snap[0].dc.Ratio)
	}
}

// TestCachestatStoreEmptySnapshotPreservesBaseline is the cachestat half of the
// same defect — it rotated the empty baseline in exactly the same way.
func TestCachestatStoreEmptySnapshotPreservesBaseline(t *testing.T) {
	store := NewEbpfSharedMemoryStore()

	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{
		{Pid: 10, Ct: 100, MarkPageAccessed: 1000, AddToPageCacheLru: 100},
	})
	store.UpdateApps(nil)
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{
		{Pid: 10, Ct: 200, MarkPageAccessed: 1100, AddToPageCacheLru: 110},
	})

	snap := store.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot() len = %d, want 1", len(snap))
	}
	// interval: mark_page_accessed +100 (total), add_to_page_cache_lru +10 (misses)
	if snap[0].cachestat.Hit != 90 || snap[0].cachestat.Miss != 10 {
		t.Fatalf("hit/miss = %d/%d, want 90/10 (baseline must survive an empty cycle)",
			snap[0].cachestat.Hit, snap[0].cachestat.Miss)
	}
}

// TestSharedStoreModulesUpdateIndependently pins the merge asymmetry: a cycle in
// which only one module updates must not drop the other module's rows.
func TestSharedStoreModulesUpdateIndependently(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{{Pid: 10, Ppid: 1, Ct: 100, MarkPageAccessed: 50}})
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{{Pid: 20, Ppid: 1, Ct: 100, CacheAccess: 9}}, 10)

	// cachestat alone updates: PID 20's dcstat row must survive.
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{{Pid: 10, Ppid: 1, Ct: 200, MarkPageAccessed: 90}})
	snap := store.Snapshot()
	if len(snap) != 2 || snap[1].pid != 20 || snap[1].dc.Curr.CacheAccess != 9 {
		t.Fatalf("dcstat row lost when only cachestat updated: %+v", snap)
	}

	// dcstat alone updates: PID 10's cachestat row must survive.
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{{Pid: 20, Ppid: 1, Ct: 200, CacheAccess: 19}}, 10)
	snap = store.Snapshot()
	if len(snap) != 2 || snap[0].pid != 10 || snap[0].cachestat.Current.MarkPageAccessed != 90 {
		t.Fatalf("cachestat row lost when only dcstat updated: %+v", snap)
	}
}

// TestDCStatFreshnessIgnoresBPFCt pins the fix for the dcstat `ct` contract: the
// CO-RE base object never writes ct (it stays 0) and the legacy object writes it
// once at map-entry creation.  A ct-based gate suppressed every delta on those two
// flavors, so the store now derives freshness from the counters and publishes its
// own monotonic token.
func TestDCStatFreshnessIgnoresBPFCt(t *testing.T) {
	cases := map[string]uint64{
		"co-re base object never writes ct": 0,
		"legacy object stamps ct once":      1234567890,
	}

	for name, bpfCt := range cases {
		t.Run(name, func(t *testing.T) {
			store := NewEbpfSharedMemoryStore()

			// Cycle 1: first sample. Prev == Curr, so the delta is 0, but the token
			// must already be non-zero or the consumers' `ct > 0` gate rejects it.
			store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
				{Pid: 7, Ct: bpfCt, CacheAccess: 100, FileSystem: 10, NotFound: 5},
			}, 10)
			first := store.Snapshot()[0].dc
			if first.Ct == 0 {
				t.Fatalf("first sample published ct = 0; consumers gate on ct > 0")
			}

			// Cycles 2 and 3: the counters advance while the BPF ct stays put. Each
			// cycle must publish a strictly greater token and the real delta.
			prevCt := first.Ct
			for cycle, access := range []uint64{160, 190} {
				store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
					{Pid: 7, Ct: bpfCt, CacheAccess: access, FileSystem: 10, NotFound: 5},
				}, 10)
				got := store.Snapshot()[0].dc
				if got.Ct <= prevCt {
					t.Fatalf("cycle %d: ct = %d, want > %d (activity must advance the token)",
						cycle+2, got.Ct, prevCt)
				}
				prevCt = got.Ct
			}
			if want := int64(30); prevCt != 0 && store.Snapshot()[0].dc.CacheAccess != want {
				t.Fatalf("cache_access delta = %d, want %d",
					store.Snapshot()[0].dc.CacheAccess, want)
			}

			// Idle cycle: counters unchanged, so the token must be held. A rising
			// token here would make the consumers re-add the already-counted delta.
			store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
				{Pid: 7, Ct: bpfCt, CacheAccess: 190, FileSystem: 10, NotFound: 5},
			}, 10)
			if got := store.Snapshot()[0].dc; got.Ct != prevCt {
				t.Fatalf("idle cycle: ct = %d, want it held at %d", got.Ct, prevCt)
			}
		})
	}
}

// TestDCStatStaleUsesCountersNotCt pins that the stale-PID debouncer follows the
// same counters-derived signal: a PID whose BPF ct never moves but whose counters
// do must never be flagged stale.
func TestDCStatStaleUsesCountersNotCt(t *testing.T) {
	store := NewEbpfSharedMemoryStore()

	for cycle := range ebpfStaleCycles + 2 {
		stale := store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
			// Ct pinned at 0, as the CO-RE base object leaves it.
			{Pid: 7, Ct: 0, CacheAccess: uint64(100 + cycle*10)},
		}, 10)
		if len(stale) != 0 {
			t.Fatalf("cycle %d: active PID flagged stale %v", cycle, stale)
		}
	}

	// Now go idle with the counters frozen: the debouncer must fire on schedule.
	var flagged []int
	for cycle := range ebpfStaleCycles + 1 {
		stale := store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
			{Pid: 7, Ct: 0, CacheAccess: uint64(100 + (ebpfStaleCycles+1)*10)},
		}, 10)
		if len(stale) > 0 {
			flagged = append(flagged, cycle)
		}
	}
	if len(flagged) != 1 || flagged[0] != ebpfStaleCycles-1 {
		t.Fatalf("stale flagged at cycles %v, want exactly [%d]", flagged, ebpfStaleCycles-1)
	}
}

// TestCachestatStoreKeepsBaselineForIdlePID pins that a stale *candidate* keeps its
// ct and counter baselines. Most candidates are idle-but-alive processes that the
// caller's PidIsAlive check keeps in the BPF map; dropping their baseline made the
// next burst of activity look like a first sample, losing one interval of deltas
// and publishing a wrong hit ratio. Mirrors TestDCStatStoreKeepsBaselineForIdlePID.
func TestCachestatStoreKeepsBaselineForIdlePID(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	app := libbpfloader.CachestatAppSnapshot{
		Pid: 42, Ct: 100,
		MarkPageAccessed: 1000, AddToPageCacheLru: 100,
	}
	// A second, continuously active PID. Without it the baseline-rotation guard
	// (see TestCachestatStoreEmptySnapshotPreservesBaseline) would skip the whole
	// rotation and mask the defect, which only shows when some other PID keeps the
	// next-cycle map non-empty.
	busy := libbpfloader.CachestatAppSnapshot{Pid: 43, MarkPageAccessed: 1}

	// Baseline, then enough unchanged cycles to be flagged as a stale candidate.
	for cycle := range cachestatStaleCycles + 1 {
		busy.Ct = uint64(cycle + 1)
		busy.MarkPageAccessed += 10
		store.UpdateApps([]libbpfloader.CachestatAppSnapshot{app, busy})
	}
	if snap := store.Snapshot(); len(snap) != 1 || snap[0].pid != 43 {
		t.Fatalf("a stale candidate must not be published as a live row, got %+v", snap)
	}

	// The process was only idle: it wakes up with 100 more accesses, 10 of which
	// missed the page cache.
	app.Ct = 200
	app.MarkPageAccessed = 1100
	app.AddToPageCacheLru = 110

	busy.Ct++
	busy.MarkPageAccessed += 10
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{app, busy})

	snap := store.Snapshot()
	if len(snap) != 2 || snap[0].pid != 42 {
		t.Fatalf("Snapshot() = %+v, want PID 42 back as a live row", snap)
	}
	// 100 accesses, 10 misses -> 90 hits, ratio 90. A dropped baseline yields a
	// first sample: Prev == Curr, so hit/miss are 0 and the ratio is the idle 100.
	if got := snap[0].cachestat; got.Hit != 90 || got.Miss != 10 || got.Ratio != 90 {
		t.Fatalf("hit/miss/ratio = %d/%d/%d, want 90/10/90 (baseline must survive the idle window)",
			got.Hit, got.Miss, got.Ratio)
	}
}

// TestDCStatTokensAreComparableAcrossPIDs pins that the published freshness token
// is drawn from a store-wide sequence, not a per-PID counter.
//
// cgroup_ebpfgo_dcstat_sum_pids() keeps ONE watermark per cgroup and compares every
// member PID's token against it. With per-PID counters a long-lived PID accumulates
// a high count while a newly added PID starts near zero, so the new PID would sit
// below the cgroup watermark and its activity would never be counted. This
// simulates that consumer to prove a late joiner is admitted immediately.
func TestDCStatTokensAreComparableAcrossPIDs(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	old := libbpfloader.DCStatAppSnapshot{Pid: 100, CacheAccess: 0}

	// A long-lived PID is active on its own for many cycles.
	for range 50 {
		old.CacheAccess += 10
		store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{old}, 10)
	}

	// The cgroup consumer's watermark after consuming those cycles.
	watermark := uint64(0)
	for _, row := range store.Snapshot() {
		if row.dc.Ct > watermark {
			watermark = row.dc.Ct
		}
	}
	if watermark == 0 {
		t.Fatal("the long-lived PID never published a token")
	}

	// A brand-new PID joins the same cgroup and is immediately active.
	newcomer := libbpfloader.DCStatAppSnapshot{Pid: 200, CacheAccess: 7}
	old.CacheAccess += 10
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{old, newcomer}, 10)
	newcomer.CacheAccess += 5
	old.CacheAccess += 10
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{old, newcomer}, 10)

	var newRow netdataPublishDCStat
	for _, row := range store.Snapshot() {
		if row.pid == 200 {
			newRow = row.dc
		}
	}
	if newRow.Ct <= watermark {
		t.Fatalf("newcomer token = %d, cgroup watermark = %d: the consumer's "+
			"`ct > watermark` gate would skip this PID forever", newRow.Ct, watermark)
	}
	if newRow.CacheAccess != 5 {
		t.Fatalf("newcomer delta = %d, want 5", newRow.CacheAccess)
	}
}

// TestDCStatTokensSurviveProducerRestart pins that the freshness token comes from
// a boot-relative clock rather than a counter starting at zero.
//
// cgroups.plugin is compiled into the netdata daemon, so its per-cgroup watermark
// (cg->dcstat.ct) outlives an ebpf-go.plugin restart, and it only ever moves
// forward. A restarted plugin whose tokens began at 1 would sit below that stored
// watermark for as many cycles as the previous instance ran, freezing the cgroup
// charts for hours. The replacement store's very first token must therefore exceed
// every token the previous one issued.
func TestDCStatTokensSurviveProducerRestart(t *testing.T) {
	app := libbpfloader.DCStatAppSnapshot{Pid: 100, CacheAccess: 0}

	before := NewEbpfSharedMemoryStore()
	watermark := uint64(0)
	for range 5 {
		app.CacheAccess += 10
		before.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app}, 10)
		for _, row := range before.Snapshot() {
			if row.dc.Ct > watermark {
				watermark = row.dc.Ct
			}
		}
	}
	if watermark == 0 {
		t.Fatal("the first producer never published a token")
	}

	// ebpf-go.plugin restarts: a brand-new store, while the daemon still holds the
	// watermark above. The BPF counters are unchanged because the kernel maps
	// survived, so the first cycle re-baselines and the second one is the live test.
	after := NewEbpfSharedMemoryStore()
	after.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app}, 10)
	app.CacheAccess += 10
	after.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app}, 10)

	snap := after.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot() len = %d, want 1", len(snap))
	}
	if snap[0].dc.Ct <= watermark {
		t.Fatalf("token after restart = %d, stale cgroup watermark = %d: the "+
			"consumer would skip every PID until the token caught up",
			snap[0].dc.Ct, watermark)
	}
	if snap[0].dc.CacheAccess != 10 {
		t.Fatalf("delta after restart = %d, want 10", snap[0].dc.CacheAccess)
	}
}

// TestSharedStoreIdentityPrefersPopulatedModule pins that neither module may
// publish an empty identity while the other holds a populated one.
//
// A BPF entry created between process start and the first bpf_get_current_comm()
// carries an empty comm — and on kernels below 4.11 the upstream BPF sources skip
// that call entirely — so an empty identity from the preferred module is a real
// occurrence, not a theoretical one. Preferring it discards usable metadata that
// the other module already has.
func TestSharedStoreIdentityPrefersPopulatedModule(t *testing.T) {
	named := [libbpfloader.DCStatAppCommLen]byte{'d', 'c', 'p', 'r', 'o', 'c'}
	var unnamed [libbpfloader.CachestatAppCommLen]byte

	store := NewEbpfSharedMemoryStore()
	// cachestat sees the PID but learned no name; dcstat has one.
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{
		{Pid: 77, Comm: unnamed, MarkPageAccessed: 5},
	})
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 77, Comm: named, CacheAccess: 5},
	}, 10)

	snap := store.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot() len = %d, want 1", len(snap))
	}
	if snap[0].comm[0] == 0 {
		t.Fatal("an empty cachestat identity shadowed dcstat's populated one")
	}
	if got := string(snap[0].comm[:6]); got != "dcproc" {
		t.Fatalf("comm = %q, want %q", got, "dcproc")
	}
}

// TestSharedStoreIdentityKeepsCachestatPriority pins the other direction: when
// cachestat does know the name it stays the preferred source, so the fix above
// did not invert the documented precedence.
func TestSharedStoreIdentityKeepsCachestatPriority(t *testing.T) {
	csName := [libbpfloader.CachestatAppCommLen]byte{'c', 's', 'p', 'r', 'o', 'c'}
	dcName := [libbpfloader.DCStatAppCommLen]byte{'d', 'c', 'p', 'r', 'o', 'c'}

	store := NewEbpfSharedMemoryStore()
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{
		{Pid: 77, Comm: csName, MarkPageAccessed: 5},
	})
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 77, Comm: dcName, CacheAccess: 5},
	}, 10)

	snap := store.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot() len = %d, want 1", len(snap))
	}
	if got := string(snap[0].comm[:6]); got != "csproc" {
		t.Fatalf("comm = %q, want cachestat's %q", got, "csproc")
	}
}

// TestDCStatPublishesCollectionInterval pins that every row carries the dcstat
// collection interval that produced its deltas.
//
// Consumers must divide by that interval, not by their own tick rate:
// cgroups.plugin ticks every second while dcstat collects every 10s by default,
// so dividing a 10-second total by one second inflated the published rate ~10x.
// The SHM header's update_every_s belongs to whichever module owns the segment,
// which need not be dcstat, so the interval has to travel per row — the same
// reason ebpf_socket_publish_apps carries socket_update_every_s.
func TestDCStatPublishesCollectionInterval(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	const interval = uint32(10)

	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 5, CacheAccess: 100, NotFound: 10},
	}, interval)
	// Second cycle so the row carries real deltas rather than a self-baseline.
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 5, CacheAccess: 200, NotFound: 20},
	}, interval)

	snap := store.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot() len = %d, want 1", len(snap))
	}
	if got := snap[0].dc.UpdateEverySec; got != interval {
		t.Fatalf("UpdateEverySec = %d, want %d: consumers cannot scale the deltas without it", got, interval)
	}
	if got := snap[0].dc.CacheAccess; got != 100 {
		t.Fatalf("cache_access delta = %d, want 100", got)
	}
}
