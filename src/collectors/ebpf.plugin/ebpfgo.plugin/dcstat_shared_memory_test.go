package main

import (
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

func TestBuildDCStatPublish(t *testing.T) {
	current := netdataPublishDCStatPid{CacheAccess: 1100, FileSystem: 260, NotFound: 60}
	previous := netdataPublishDCStatPid{CacheAccess: 1000, FileSystem: 200, NotFound: 50}

	got := buildDCStatPublish(current, previous, 1234, true)
	want := netdataPublishDCStat{
		Ct:          1234,
		Ratio:       90, // interval: reference +100, not_found +10
		CacheAccess: 100,
		Curr:        current,
		Prev:        previous,
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

	got := buildDCStatPublish(current, current, 1234, true)
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

	got := buildDCStatPublish(current, netdataPublishDCStatPid{}, 7, false)
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
	})

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
	if stale := store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app}); len(stale) != 0 {
		t.Fatalf("cycle 0 (baseline): unexpected stale %v", stale)
	}

	for i := 1; i < ebpfStaleCycles; i++ {
		if stale := store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app}); len(stale) != 0 {
			t.Fatalf("cycle %d: unexpected stale %v (threshold not yet reached)", i, stale)
		}
		if len(store.Snapshot()) != 1 {
			t.Fatalf("cycle %d: PID 42 should still be present", i)
		}
	}

	stale := store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app})
	if len(stale) != 1 || stale[0] != 42 {
		t.Fatalf("expected stale candidate for PID 42, got stale=%v", stale)
	}
}

func TestDCStatStoreNoFlagWhenCtAdvances(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	app := libbpfloader.DCStatAppSnapshot{Pid: 7, Ct: 100}

	for range ebpfStaleCycles {
		store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app})
	}

	app.Ct = 200
	if stale := store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app}); len(stale) != 0 {
		t.Fatalf("expected no stale flag after ct advance, got stale=%v", stale)
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
	})
	if len(store.Snapshot()) != 2 {
		t.Fatalf("cycle 1: Snapshot() len = %d, want 2", len(store.Snapshot()))
	}

	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
		{Pid: 20, Ct: 200, CacheAccess: 9},
	})
	snap := store.Snapshot()
	if len(snap) != 1 || snap[0].pid != 20 {
		t.Fatalf("cycle 2: Snapshot() = %+v, want only PID 20", snap)
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
	})
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
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{{Pid: 1, Ct: 1}})

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

	// Baseline, then enough unchanged cycles to be flagged as a stale candidate.
	for range ebpfStaleCycles + 1 {
		store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app})
	}
	if len(store.Snapshot()) != 0 {
		t.Fatal("a stale candidate must not be published as a live row")
	}

	// The process was only idle: it wakes up and does 100 more lookups, 10 missed.
	app.Ct = 200
	app.CacheAccess = 1100
	app.NotFound = 110
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{app})

	snap := store.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot() len = %d, want 1", len(snap))
	}
	if snap[0].dc.CacheAccess != 100 {
		t.Fatalf("cache_access delta = %d, want 100 (baseline must survive the idle window)",
			snap[0].dc.CacheAccess)
	}
	if snap[0].dc.Ratio != 90 {
		t.Fatalf("ratio = %d, want 90", snap[0].dc.Ratio)
	}
}
