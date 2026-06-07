package main

import (
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

func TestBuildCachestatPublish(t *testing.T) {
	current := netdataCachestat{
		AddToPageCacheLru:  95,
		MarkPageAccessed:   130,
		AccountPageDirtied: 20,
		MarkBufferDirty:    30,
	}
	previous := netdataCachestat{
		AddToPageCacheLru:  80,
		MarkPageAccessed:   100,
		AccountPageDirtied: 10,
		MarkBufferDirty:    20,
	}

	got := buildCachestatPublish(current, previous, 1234, true)
	want := netdataPublishCachestat{
		Ct:      1234,
		Dirty:   10,
		Hit:     15,
		Miss:    5,
		Ratio:   75,
		Current: current,
		Prev:    previous,
	}

	if got != want {
		t.Fatalf("buildCachestatPublish() = %+v, want %+v", got, want)
	}
}

func TestCachestatSharedMemoryStoreEvictsAfterStaleCycles(t *testing.T) {
	store := NewCachestatSharedMemoryStore()
	app := libbpfloader.CachestatAppSnapshot{Pid: 42, Ppid: 1, Ct: 100, MarkPageAccessed: 10}

	// First call establishes the ct baseline — no stale PIDs yet.
	if stale := store.UpdateApps([]libbpfloader.CachestatAppSnapshot{app}); len(stale) != 0 {
		t.Fatalf("cycle 0 (baseline): unexpected stale %v", stale)
	}

	// Each subsequent call with the same ct accumulates a miss.
	// The PID must survive the first cachestatStaleCycles-1 stale cycles.
	for i := 1; i < cachestatStaleCycles; i++ {
		stale := store.UpdateApps([]libbpfloader.CachestatAppSnapshot{app})
		if len(stale) != 0 {
			t.Fatalf("cycle %d: unexpected stale %v (threshold not yet reached)", i, stale)
		}
		if len(store.Snapshot()) != 1 {
			t.Fatalf("cycle %d: PID 42 should still be present", i)
		}
	}

	// One more call with unchanged ct — miss count reaches cachestatStaleCycles → evict.
	stale := store.UpdateApps([]libbpfloader.CachestatAppSnapshot{app})
	if len(stale) != 1 || stale[0] != 42 {
		t.Fatalf("expected eviction of PID 42, got stale=%v", stale)
	}
	if len(store.Snapshot()) != 0 {
		t.Fatalf("expected empty snapshot after eviction")
	}
}

func TestCachestatSharedMemoryStoreNoEvictionWhenCtAdvances(t *testing.T) {
	store := NewCachestatSharedMemoryStore()
	app := libbpfloader.CachestatAppSnapshot{Pid: 7, Ct: 100}

	// Drive the miss count to cachestatStaleCycles-1 (one cycle before eviction).
	for i := 0; i <= cachestatStaleCycles-1; i++ {
		store.UpdateApps([]libbpfloader.CachestatAppSnapshot{app})
	}
	if len(store.Snapshot()) != 1 {
		t.Fatal("PID 7 should still be present before ct advance")
	}

	// Advance ct — miss count must reset to zero.
	app.Ct = 200
	if stale := store.UpdateApps([]libbpfloader.CachestatAppSnapshot{app}); len(stale) != 0 {
		t.Fatalf("expected no eviction after ct advance, got stale=%v", stale)
	}

	// Now drive another cachestatStaleCycles-1 stale cycles — still below threshold.
	for i := 0; i < cachestatStaleCycles-1; i++ {
		stale := store.UpdateApps([]libbpfloader.CachestatAppSnapshot{app})
		if len(stale) != 0 {
			t.Fatalf("cycle %d after ct advance: unexpected eviction", i)
		}
	}

	// The cachestatStaleCycles-th stale cycle triggers eviction.
	stale := store.UpdateApps([]libbpfloader.CachestatAppSnapshot{app})
	if len(stale) != 1 || stale[0] != 7 {
		t.Fatalf("expected eviction of PID 7 after second stale run, got stale=%v", stale)
	}
}

func TestCachestatSharedMemoryStoreUpdateApps(t *testing.T) {
	store := NewCachestatSharedMemoryStore()
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{
		{
			Pid:                20,
			Ppid:               3,
			Ct:                 200,
			AddToPageCacheLru:  80,
			MarkPageAccessed:   100,
			AccountPageDirtied: 10,
			MarkBufferDirty:    20,
			Comm:               [libbpfloader.CachestatAppCommLen]byte{'b', 'e', 't', 'a'},
		},
		{
			Pid:                10,
			Ppid:               1,
			Ct:                 100,
			AddToPageCacheLru:  40,
			MarkPageAccessed:   50,
			AccountPageDirtied: 5,
			MarkBufferDirty:    8,
			Comm:               [libbpfloader.CachestatAppCommLen]byte{'a', 'l', 'p', 'h', 'a'},
		},
	})

	got := store.Snapshot()
	if len(got) != 2 {
		t.Fatalf("Snapshot() len = %d, want 2", len(got))
	}

	if got[0].pid != 10 || got[1].pid != 20 {
		t.Fatalf("Snapshot() pids = %d,%d, want 10,20", got[0].pid, got[1].pid)
	}

	if got[0].ppid != 1 {
		t.Fatalf("Snapshot()[0].ppid = %d, want 1", got[0].ppid)
	}

	if got[0].comm[0] != 'a' || got[1].comm[0] != 'b' {
		t.Fatalf("Snapshot() comms were not copied")
	}

	if got[0].cachestat.Current.AddToPageCacheLru != 40 || got[1].cachestat.Current.AddToPageCacheLru != 80 {
		t.Fatalf("Snapshot() current counters were not copied")
	}

	if got[0].cachestat.Prev != (netdataCachestat{}) || got[1].cachestat.Prev != (netdataCachestat{}) {
		t.Fatalf("Snapshot() previous counters on first update should be zero")
	}
}

func BenchmarkCachestatSharedMemoryStoreUpdateAppsSorted(b *testing.B) {
	const appsCount = 1024
	apps := make([]libbpfloader.CachestatAppSnapshot, appsCount)
	for i := range apps {
		apps[i] = libbpfloader.CachestatAppSnapshot{
			Pid:                uint32(i + 1),
			Ppid:               1,
			Ct:                 uint64(i + 1),
			AddToPageCacheLru:  uint32(i),
			MarkPageAccessed:   uint32(i * 2),
			AccountPageDirtied: uint32(i / 2),
			MarkBufferDirty:    uint32(i / 3),
		}
	}

	store := NewCachestatSharedMemoryStore()
	store.UpdateApps(apps)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for idx := range apps {
			apps[idx].Ct++
		}
		if stale := store.UpdateApps(apps); len(stale) != 0 {
			b.Fatalf("unexpected stale PIDs: %v", stale)
		}
	}
}
