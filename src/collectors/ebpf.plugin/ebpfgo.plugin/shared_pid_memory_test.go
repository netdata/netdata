//go:build linux && cgo

package main

import (
	"testing"
	"unsafe"
)

func TestSharedPidMemoryLayoutMatchesGo(t *testing.T) {
	if got, want := unsafe.Sizeof(ebpfPidStat{}), uintptr(sharedPidMemoryRowSize); got != want {
		t.Fatalf("ebpfPidStat size = %d, want %d", got, want)
	}
}

// TestSharedPidMemoryPublisher_PrevCountInvariant documents and exercises the
// prev_count == 0 invariant that holds both after shared_pid_memory_open and
// after pid_shm_replace_generation.  pid_shm_replace_generation explicitly
// resets prev_count to 0 so the first publish after replace does not fire a
// spurious tail-memset on a kernel-zero-filled segment.
//
// The test exercises the three relevant prev_count transitions:
//
//	0 → 0  (no memset; no-op on a fresh or replaced segment)
//	0 → N  (no memset; only memcpy of new entries)
//	N → M  (conditional memset zeros vacated slots [M, N))
func TestSharedPidMemoryPublisher_PrevCountInvariant(t *testing.T) {
	const total = 8
	pub, err := NewSharedPidMemoryPublisher(total, 1)
	if err != nil {
		t.Skipf("SHM unavailable (not Linux or /dev/shm missing): %v", err)
	}
	defer pub.Close()

	// 0 → 0: prev_count(0) > count(0) is false; no memset, no OOB write.
	if err := pub.Publish(nil, 0); err != nil {
		t.Fatalf("Publish(0 entries): %v", err)
	}

	// 0 → 4: advance prev_count; only memcpy, no memset.
	entries := make([]ebpfPidStat, 4)
	for i := range entries {
		entries[i].pid = uint32(i + 1)
	}
	if err := pub.Publish(entries, 1); err != nil {
		t.Fatalf("Publish(4 entries): %v", err)
	}

	// 4 → 2: conditional memset zeros slots [2, 3]; must not overrun entries[].
	if err := pub.Publish(entries[:2], 1); err != nil {
		t.Fatalf("Publish(shrink to 2 entries): %v", err)
	}

	// 2 → 0: conditional memset zeros slots [0, 1].
	if err := pub.Publish(nil, 0); err != nil {
		t.Fatalf("Publish(shrink to 0 entries): %v", err)
	}
}
