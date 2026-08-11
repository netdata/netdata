package main

import (
	"sort"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// cachestatStaleCycles is the cachestat-facing name of the shared ct-stagnation
// debouncer window (see ebpfStaleCycles).
const cachestatStaleCycles = ebpfStaleCycles

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

	total := max(mpa-mbd, 0)

	misses := max(apcl-apd, 0)

	hits := total - misses
	if hits < 0 {
		misses = total
		hits = 0
	}

	if total > 0 {
		publish.Ratio = int64((float64(hits) / float64(total)) * 100)
	} else {
		// No page-cache activity this interval; 100 = full hit rate (nothing missed).
		// Matches the idle-path convention in apps_ebpf_shared_memory.c and
		// cgroup_ebpfgo_cachestat.c so SHM consumers stay consistent.
		publish.Ratio = 100
	}

	publish.Hit = hits
	publish.Miss = misses
	return publish
}

func copyCommFromSnapshot(dst *[EBPF_MAX_COMPARE_NAME + 1]byte, src [libbpfloader.CachestatAppCommLen]byte) {
	copy(dst[:], src[:])
}

// UpdateApps updates the in-memory snapshot from the latest cachestat BPF
// snapshot.  It returns the PIDs whose ct has not advanced for
// cachestatStaleCycles consecutive cycles; the caller is responsible for the
// authoritative liveness check (libbpfloader.PidIsAlive) before removing them
// from the kernel BPF map.
//
// The store itself is liveness-agnostic so it can be unit-tested without a
// running /proc and without pulling libbpf into the test binary.
func (s *ebpfSharedMemoryStore) UpdateApps(apps []libbpfloader.CachestatAppSnapshot) []uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.cachestatData)
	clear(s.cachestatIdent)
	clear(s.nextCachestat)
	clear(s.nextCachestatCt)
	clear(s.nextCachestatMs)
	pids := s.cachestatPIDs[:0]
	stalePIDs := s.cachestatStale[:0]
	ordered := true

	for _, app := range apps {
		lastCt, seen := s.cachestatPrevCt[app.Pid]
		if seen && app.Ct == lastCt {
			miss := s.cachestatMiss[app.Pid] + 1
			if miss >= cachestatStaleCycles {
				// ct stagnation threshold reached.  The caller will confirm
				// liveness via libbpfloader.PidIsAlive and delete from the BPF
				// map only if the process is gone.
				stalePIDs = append(stalePIDs, app.Pid)
				continue
			}
			s.nextCachestatMs[app.Pid] = miss
		}
		// New PID or ct advanced: miss count stays 0 (Go zero-value).

		s.nextCachestatCt[app.Pid] = app.Ct

		current := netdataCachestat{
			AddToPageCacheLru:  app.AddToPageCacheLru,
			MarkPageAccessed:   app.MarkPageAccessed,
			AccountPageDirtied: app.AccountPageDirtied,
			MarkBufferDirty:    app.MarkBufferDirty,
		}
		previous, hasPrevious := s.cachestatPrev[app.Pid]

		var ident ebpfModuleIdentity
		copyCommFromSnapshot(&ident.comm, app.Comm)
		ident.ppid = app.Ppid

		s.cachestatData[app.Pid] = buildCachestatPublish(current, previous, app.Ct, hasPrevious)
		s.cachestatIdent[app.Pid] = ident
		s.nextCachestat[app.Pid] = current
		pids, ordered = appendAscending(pids, app.Pid, ordered)
	}

	// The native snapshot path already sorts by pid; only a test or fallback
	// caller supplying unordered input pays for the sort.
	if !ordered {
		sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })
	}

	s.cachestatPIDs = pids
	s.cachestatPrev, s.nextCachestat = s.nextCachestat, s.cachestatPrev
	s.cachestatPrevCt, s.nextCachestatCt = s.nextCachestatCt, s.cachestatPrevCt
	s.cachestatMiss, s.nextCachestatMs = s.nextCachestatMs, s.cachestatMiss
	s.cachestatStale = stalePIDs
	s.activeModules |= ebpfgoSHMFlagCachestat // mark cachestat as an active producer
	s.rebuildEntriesLocked()
	return stalePIDs
}
