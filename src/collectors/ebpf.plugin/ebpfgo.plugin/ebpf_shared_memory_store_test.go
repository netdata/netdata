package main

import (
	"sync"
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// TestSharedStoreConcurrentModuleUpdates exercises the store the way production
// does: cachestat, dcstat, and socket each run their own collection goroutine and
// mutate the shared store, while readers take snapshots.
//
// Under `go test -race` this is the only coverage that the store's locking is
// actually sound; every other test drives it from a single goroutine. The
// assertions are limited to invariants that hold under any interleaving — the
// exact contents are racy by construction — namely that entries stay sorted
// ascending by PID and carry no duplicates, which is what the consumers' binary
// search over shared memory depends on.
func TestSharedStoreConcurrentModuleUpdates(t *testing.T) {
	const rounds = 200

	store := NewEbpfSharedMemoryStore()

	var wg sync.WaitGroup
	start := make(chan struct{})

	worker := func(fn func(i int)) {
		wg.Go(func() {
			<-start
			for i := range rounds {
				fn(i)
			}
		})
	}

	worker(func(i int) {
		store.UpdateApps([]libbpfloader.CachestatAppSnapshot{
			{Pid: 10, Ct: uint64(i), MarkPageAccessed: uint32(i)},
			{Pid: 30, Ct: uint64(i), MarkPageAccessed: uint32(i)},
		})
	})
	worker(func(i int) {
		store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{
			{Pid: 10, CacheAccess: uint64(i)},
			{Pid: 20, CacheAccess: uint64(i)},
		}, 10)
	})
	worker(func(i int) {
		store.UpdateSocketApps([]libbpfloader.SocketPIDEntry{
			{PID: 10, BytesSent: uint64(i)},
			{PID: 40, BytesSent: uint64(i)},
		}, 10)
	})
	// The failure paths mutate the store too, so they belong in the race.
	worker(func(i int) {
		if i%16 == 0 {
			store.ClearDCStatApps()
		}
	})

	// Readers.
	for range 3 {
		worker(func(int) {
			for _, row := range store.Snapshot() {
				_ = row.pid
			}
		})
	}

	close(start)
	wg.Wait()

	// Invariants that must hold regardless of interleaving.
	snap := store.Snapshot()
	seen := make(map[uint32]bool, len(snap))
	for i, row := range snap {
		if i > 0 && row.pid <= snap[i-1].pid {
			t.Fatalf("entries are not strictly ascending at %d: %d after %d "+
				"(consumers binary-search this)", i, row.pid, snap[i-1].pid)
		}
		if seen[row.pid] {
			t.Fatalf("duplicate PID %d in the published entries", row.pid)
		}
		seen[row.pid] = true
	}
}
