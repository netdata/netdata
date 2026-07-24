package main

import (
	"sort"
	"sync"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// cachestatStaleCycles is the debouncer window before a PID is flagged as a
// stale candidate and the caller performs the authoritative liveness check
// (libbpfloader.PidIsAlive).  Without this debouncer we would run kill() on
// every PID every cycle, which is too expensive.
const cachestatStaleCycles = 3

type cachestatSharedMemoryStore struct {
	mu            sync.RWMutex
	entries       []ebpfPidStat
	nextEntries   []ebpfPidStat
	prev          map[uint32]netdataCachestat
	prevCt        map[uint32]uint64 // last observed BPF timestamp per PID
	missCount     map[uint32]int    // consecutive cycles where ct did not advance
	nextPrev      map[uint32]netdataCachestat
	nextPrevCt    map[uint32]uint64
	nextMiss      map[uint32]int
	stalePIDs     []uint32
	socketData    map[uint32]ebpfSocketPublishApps // latest per-PID socket snapshot from tbl_nd_socket
	activeModules uint32                           // EBPFGO_SHM_FLAG_* bits set when a module writes data
	// cachestatOwnsEntries is true when UpdateApps last populated s.entries.
	// When false (socket is the sole publisher), UpdateSocketApps must rebuild
	// s.entries from scratch each cycle rather than merging in-place, so
	// exited PIDs are evicted instead of accumulating indefinitely.
	cachestatOwnsEntries bool
}

func NewCachestatSharedMemoryStore() *cachestatSharedMemoryStore {
	return &cachestatSharedMemoryStore{
		prev:       make(map[uint32]netdataCachestat),
		prevCt:     make(map[uint32]uint64),
		missCount:  make(map[uint32]int),
		nextPrev:   make(map[uint32]netdataCachestat),
		nextPrevCt: make(map[uint32]uint64),
		nextMiss:   make(map[uint32]int),
		socketData: make(map[uint32]ebpfSocketPublishApps),
	}
}

func (s *cachestatSharedMemoryStore) Snapshot() []ebpfPidStat {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// s.entries is always sorted ascending by pid before Publish() is called:
	// UpdateApps sorts when BPF input is unordered, and applySocketDataLocked /
	// buildEntriesFromSocketLocked sort after appending socket-only PIDs.
	copied := make([]ebpfPidStat, len(s.entries))
	copy(copied, s.entries)
	return copied
}

// ebpfgoSHM* mirrors the EBPFGO_SHM_FLAG_* C constants so Go callers
// do not need to import "C" just to set a flag value.
const (
	ebpfgoSHMFlagCachestat uint32 = 0x01
	ebpfgoSHMFlagSocket    uint32 = 0x02
)

// Publish writes the current entries to the shared-memory segment and stamps
// the per-module validity flags.  Only the CACHESTAT bit is cleared after
// each publish; the SOCKET bit persists across cachestat cycles so the C
// consumer does not see socket_ok flap when socket runs at a slower cadence
// than cachestat.  The SOCKET bit is cleared by MarkSocketInactive on goroutine
// exit or per-cycle when collection fails.
//
// The lock is held for the duration of the C memcpy because
// applySocketDataLocked writes s.entries[i].socket in place on the same
// backing array that the C side reads; releasing the lock would expose a
// data race where socket's goroutine could mutate the entries slice while
// publisher.Publish is still reading it.
func (s *cachestatSharedMemoryStore) Publish(publisher *SharedPidMemoryPublisher) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	flags := s.activeModules
	s.activeModules &^= ebpfgoSHMFlagCachestat // socket bit persists until MarkSocketInactive

	if publisher == nil {
		return nil
	}
	return publisher.Publish(s.entries, flags)
}

// MarkSocketInactive clears the SOCKET flag from activeModules.  Called via
// defer when the socket goroutine exits (permanent shutdown) and also per-cycle
// when Snapshot or SnapshotPerPID fails so the consumer does not see
// socket_ok = true with zero data arrays.
func (s *cachestatSharedMemoryStore) MarkSocketInactive() {
	s.mu.Lock()
	s.activeModules &^= ebpfgoSHMFlagSocket
	s.mu.Unlock()
}

func buildCachestatPublish(current, previous netdataCachestat, ct uint64, hasPrevious bool) netdataPublishCachestat {
	publish := netdataPublishCachestat{
		Ct:      ct,
		Current: current,
		Prev:    previous,
	}

	if !hasPrevious {
		return publish
	}

	mpa := diffCounters(uint64(current.MarkPageAccessed), uint64(previous.MarkPageAccessed))
	mbd := diffCounters(uint64(current.MarkBufferDirty), uint64(previous.MarkBufferDirty))
	apcl := diffCounters(uint64(current.AddToPageCacheLru), uint64(previous.AddToPageCacheLru))
	apd := diffCounters(uint64(current.AccountPageDirtied), uint64(previous.AccountPageDirtied))

	publish.Dirty = mbd

	total := mpa - mbd
	if total < 0 {
		total = 0
	}

	misses := apcl - apd
	if misses < 0 {
		misses = 0
	}

	hits := total - misses
	if hits < 0 {
		misses = total
		hits = 0
	}

	if total > 0 {
		publish.Ratio = int64((float64(hits) / float64(total)) * 100)
	} else {
		publish.Ratio = 100
	}

	publish.Hit = hits
	publish.Miss = misses
	return publish
}

func copyCommFromSnapshot(dst *[EBPF_MAX_COMPARE_NAME + 1]byte, src [libbpfloader.CachestatAppCommLen]byte) {
	copy(dst[:], src[:])
}

func buildCachestatPidStat(app libbpfloader.CachestatAppSnapshot, previous netdataCachestat, hasPrevious bool) (ebpfPidStat, netdataCachestat) {
	current := netdataCachestat{
		AddToPageCacheLru:  app.AddToPageCacheLru,
		MarkPageAccessed:   app.MarkPageAccessed,
		AccountPageDirtied: app.AccountPageDirtied,
		MarkBufferDirty:    app.MarkBufferDirty,
	}

	var comm [EBPF_MAX_COMPARE_NAME + 1]byte
	copyCommFromSnapshot(&comm, app.Comm)

	return ebpfPidStat{
		pid:       app.Pid,
		comm:      comm,
		ppid:      app.Ppid,
		cachestat: buildCachestatPublish(current, previous, app.Ct, hasPrevious),
	}, current
}

// UpdateApps updates the in-memory snapshot from the latest BPF snapshot.
// It returns the PIDs whose ct has not advanced for cachestatStaleCycles
// consecutive cycles; the caller is responsible for the authoritative
// liveness check (libbpfloader.PidIsAlive) before removing them from the
// kernel BPF map.
//
// The store itself is liveness-agnostic so it can be unit-tested without a
// running /proc and without pulling libbpf into the test binary.
func (s *cachestatSharedMemoryStore) UpdateApps(apps []libbpfloader.CachestatAppSnapshot) []uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	nextEntries := s.nextEntries[:0]
	if cap(nextEntries) < len(apps) {
		nextEntries = make([]ebpfPidStat, 0, len(apps))
	}
	clear(s.nextPrev)
	clear(s.nextPrevCt)
	clear(s.nextMiss)
	stalePIDs := s.stalePIDs[:0]
	ordered := true
	seenPID := false
	var previousPID uint32

	for _, app := range apps {
		if seenPID && app.Pid < previousPID {
			ordered = false
		}
		seenPID = true
		previousPID = app.Pid

		lastCt, seen := s.prevCt[app.Pid]
		if seen && app.Ct == lastCt {
			miss := s.missCount[app.Pid] + 1
			if miss >= cachestatStaleCycles {
				// ct stagnation threshold reached.  The caller will
				// confirm liveness via libbpfloader.PidIsAlive and
				// delete from the BPF map only if the process is gone.
				stalePIDs = append(stalePIDs, app.Pid)
				continue
			}
			s.nextMiss[app.Pid] = miss
		}
		// New PID or ct advanced: miss count stays 0 (Go zero-value).

		s.nextPrevCt[app.Pid] = app.Ct

		previous, hasPrevious := s.prev[app.Pid]
		stat, current := buildCachestatPidStat(app, previous, hasPrevious)
		nextEntries = append(nextEntries, stat)
		s.nextPrev[app.Pid] = current
	}

	// Ensure entries are sorted by pid so Snapshot() callers always see a
	// consistent ordering. The native snapshot path already sorts by pid, so
	// only pay for sort when a test or fallback caller provides unordered input.
	if !ordered {
		sort.Slice(nextEntries, func(i, j int) bool {
			return nextEntries[i].pid < nextEntries[j].pid
		})
	}

	s.entries, s.nextEntries = nextEntries, s.entries
	s.prev, s.nextPrev = s.nextPrev, s.prev
	s.prevCt, s.nextPrevCt = s.nextPrevCt, s.prevCt
	s.missCount, s.nextMiss = s.nextMiss, s.missCount
	s.stalePIDs = stalePIDs
	s.activeModules |= ebpfgoSHMFlagCachestat // mark cachestat as an active producer
	// Track ownership so UpdateSocketApps knows to merge rather than rebuild.
	s.cachestatOwnsEntries = len(s.entries) > 0
	s.applySocketDataLocked()
	return stalePIDs
}

// applySocketDataLocked merges the latest socket snapshot into s.entries.
// Existing entries' socket field is updated from s.socketData.  PIDs present
// in s.socketData but absent from s.entries are appended and the slice is
// re-sorted, so that services with socket activity but no page-cache activity
// (absent from the cachestat snapshot) are still visible to the C consumer.
// Must be called with s.mu held for writing.
func (s *cachestatSharedMemoryStore) applySocketDataLocked() {
	for i := range s.entries {
		if data, ok := s.socketData[s.entries[i].pid]; ok {
			s.entries[i].socket = data
		} else {
			// PID absent from the latest socket snapshot: reset to zero so
			// consumers never see values from a prior cycle.
			s.entries[i].socket = ebpfSocketPublishApps{}
		}
	}

	if len(s.socketData) == 0 {
		return
	}

	// Append socket-only PIDs not already covered by a cachestat entry.
	// s.entries is sorted ascending so binary search is O(log n) per PID.
	prevLen := len(s.entries)
	for pid, data := range s.socketData {
		if !sortedEntriesContainPID(s.entries[:prevLen], pid) {
			s.entries = append(s.entries, ebpfPidStat{pid: pid, socket: data})
		}
	}

	if len(s.entries) > prevLen {
		sort.Slice(s.entries, func(i, j int) bool {
			return s.entries[i].pid < s.entries[j].pid
		})
	}
}

// sortedEntriesContainPID reports whether the sorted entries slice contains pid.
func sortedEntriesContainPID(entries []ebpfPidStat, pid uint32) bool {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if entries[mid].pid < pid {
			lo = mid + 1
		} else if entries[mid].pid > pid {
			hi = mid
		} else {
			return true
		}
	}
	return false
}

// UpdateSocketApps stores the latest per-PID socket snapshot and applies it to
// the current entries.  Called by the socket collector each cycle.
//
// When cachestat is also running as publisher (cachestatOwnsEntries == true),
// socket data is merged into the cachestat-populated entries via
// applySocketDataLocked so both modules' data appear on the same rows.
//
// When socket is the sole publisher (cachestat disabled or has no apps/cgroups
// consumers), s.entries is rebuilt from scratch each cycle via
// buildEntriesFromSocketLocked.  This mirrors the double-buffer pattern used
// by UpdateApps and ensures that exited PIDs are evicted automatically — if we
// merged in-place the way the co-publisher path does, entries would accumulate
// every PID that ever had socket activity and never shrink, eventually exceeding
// the 32768-row SHM capacity and silently dropping high-PID processes.
func (s *cachestatSharedMemoryStore) UpdateSocketApps(entries []libbpfloader.SocketPIDEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for k := range s.socketData {
		delete(s.socketData, k)
	}
	for _, e := range entries {
		s.socketData[e.PID] = ebpfSocketPublishApps{
			BytesSent:           e.BytesSent,
			BytesReceived:       e.BytesReceived,
			CallTCPSent:         e.CallTCPSent,
			CallTCPReceived:     e.CallTCPReceived,
			Retransmit:          e.Retransmit,
			CallUDPSent:         e.CallUDPSent,
			CallUDPReceived:     e.CallUDPReceived,
			CallClose:           e.CallClose,
			CallTCPV4Connection: e.CallTCPV4Connection,
			CallTCPV6Connection: e.CallTCPV6Connection,
		}
	}
	// Set the SOCKET flag only on the success path (entries != nil).
	// clearSocketApps() passes nil to signal a failed collection cycle;
	// the caller clears the flag via MarkSocketInactive() before calling here.
	if entries != nil {
		s.activeModules |= ebpfgoSHMFlagSocket
	}
	if len(s.socketData) == 0 {
		s.clearSocketDataFromEntriesLocked()
		return
	}
	if s.cachestatOwnsEntries {
		s.applySocketDataLocked()
	} else {
		s.buildEntriesFromSocketLocked()
	}
}

// clearSocketDataFromEntriesLocked removes the socket contribution from the
// current snapshot while preserving real cachestat rows.
func (s *cachestatSharedMemoryStore) clearSocketDataFromEntriesLocked() {
	nextEntries := s.entries[:0]
	for i := range s.entries {
		s.entries[i].socket = ebpfSocketPublishApps{}
		if entryHasCachestatData(s.entries[i]) {
			nextEntries = append(nextEntries, s.entries[i])
		}
	}
	s.entries = nextEntries
}

func entryHasCachestatData(entry ebpfPidStat) bool {
	if entry.ppid != 0 || entry.cachestat != (netdataPublishCachestat{}) {
		return true
	}
	return entry.comm != ([EBPF_MAX_COMPARE_NAME + 1]byte{})
}

// buildEntriesFromSocketLocked populates s.entries from the latest socket
// snapshot when cachestat has no entries to merge into.  Must be called with
// s.mu held for writing.
func (s *cachestatSharedMemoryStore) buildEntriesFromSocketLocked() {
	entries := make([]ebpfPidStat, 0, len(s.socketData))
	for pid, data := range s.socketData {
		entries = append(entries, ebpfPidStat{pid: pid, socket: data})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].pid < entries[j].pid
	})
	s.entries = entries
}
