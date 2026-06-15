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
	mu          sync.RWMutex
	entries     []ebpfPidStat
	nextEntries []ebpfPidStat
	prev        map[uint32]netdataCachestat
	prevCt      map[uint32]uint64 // last observed BPF timestamp per PID
	missCount   map[uint32]int    // consecutive cycles where ct did not advance
	nextPrev    map[uint32]netdataCachestat
	nextPrevCt  map[uint32]uint64
	nextMiss    map[uint32]int
	stalePIDs   []uint32
	socketData  map[uint32]ebpfSocketPublishApps // latest per-PID socket snapshot from tbl_nd_socket
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

	// s.entries is already sorted ascending by pid: the C layer qsorts its
	// output, SnapshotApps preserves that order, and UpdateApps iterates in
	// order while only skipping (not reordering) stale entries.
	copied := make([]ebpfPidStat, len(s.entries))
	copy(copied, s.entries)
	return copied
}

func (s *cachestatSharedMemoryStore) Publish(publisher *SharedPidMemoryPublisher) error {
	if publisher == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return publisher.Publish(s.entries)
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
	s.applySocketDataLocked()
	return stalePIDs
}

// applySocketDataLocked merges the latest socket snapshot into s.entries.
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
}

// UpdateSocketApps stores the latest per-PID socket snapshot and applies it to
// the current entries.  Called by the socket collector each cycle.
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
	s.applySocketDataLocked()
}
