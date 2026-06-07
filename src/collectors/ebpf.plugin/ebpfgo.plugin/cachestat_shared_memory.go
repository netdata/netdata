package main

import (
	"sort"
	"sync"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// cachestatStaleCycles is the number of consecutive collection cycles with no
// ct advance before a PID entry is evicted from both the BPF map and the SHM.
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
}

func NewCachestatSharedMemoryStore() *cachestatSharedMemoryStore {
	return &cachestatSharedMemoryStore{
		prev:       make(map[uint32]netdataCachestat),
		prevCt:     make(map[uint32]uint64),
		missCount:  make(map[uint32]int),
		nextPrev:   make(map[uint32]netdataCachestat),
		nextPrevCt: make(map[uint32]uint64),
		nextMiss:   make(map[uint32]int),
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
// It returns the PIDs that should be deleted from the kernel BPF map because
// their ct has not advanced for cachestatStaleCycles consecutive cycles.
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
				// ct has not advanced for cachestatStaleCycles cycles:
				// treat the PID as exited and evict it.
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
	return stalePIDs
}
