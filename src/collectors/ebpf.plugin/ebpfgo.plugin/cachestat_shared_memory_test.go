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
