package main

import (
	"sort"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// buildDCStatPublish converts one PID's raw directory-cache counters into the
// shared-memory row.
//
// Curr/Prev carry the raw cumulative counters so consumers can derive
// per-interval deltas for any of the three counters themselves (the C struct has
// only one scalar delta field).  Ratio and CacheAccess are the per-interval
// values, matching the semantics the C collector gave those two fields.
//
// On the first sample for a PID there is no previous reading, so Prev is set
// equal to Curr rather than left zero: every consumer derives its deltas as
// Curr - Prev, and a zero Prev would publish the process's entire pre-existing
// counter history as one interval's activity.  This also repeats whenever a PID
// re-enters the BPF map or the producer restarts, so it has to be handled here,
// once, rather than in each consumer.
func buildDCStatPublish(current, previous netdataPublishDCStatPid, ct uint64, hasPrevious bool) netdataPublishDCStat {
	if !hasPrevious {
		return netdataPublishDCStat{Ct: ct, Curr: current, Prev: current}
	}

	reference := diffCounters(current.CacheAccess, previous.CacheAccess)

	return netdataPublishDCStat{
		Ct:          ct,
		Ratio:       dcstatHitRatio(reference, diffCounters(current.NotFound, previous.NotFound)),
		CacheAccess: reference,
		Curr:        current,
		Prev:        previous,
	}
}

// UpdateDCStatApps updates the in-memory snapshot from the latest dcstat BPF
// snapshot.  It returns the PIDs whose ct has not advanced for ebpfStaleCycles
// consecutive cycles; the caller performs the authoritative liveness check
// (libbpfloader.PidIsAlive) before removing them from the kernel BPF map.
//
// dcstat keeps its own ct/miss bookkeeping because its BPF map advances
// independently of cachestat's.
func (s *ebpfSharedMemoryStore) UpdateDCStatApps(apps []libbpfloader.DCStatAppSnapshot) []uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.dcstatData)
	clear(s.dcstatIdent)
	clear(s.nextDcstat)
	clear(s.nextDcstatCt)
	clear(s.nextDcstatMs)
	pids := s.dcstatPIDs[:0]
	stalePIDs := s.dcstatStale[:0]
	ordered := true

	for _, app := range apps {
		lastCt, seen := s.dcstatPrevCt[app.Pid]
		stale := false
		if seen && app.Ct == lastCt {
			miss := s.dcstatMiss[app.Pid] + 1
			if miss >= ebpfStaleCycles {
				stale = true
				stalePIDs = append(stalePIDs, app.Pid)
			} else {
				s.nextDcstatMs[app.Pid] = miss
			}
		}
		// New PID or ct advanced: miss count stays 0 (Go zero-value).  A stale
		// candidate also resets it, so it is re-flagged every ebpfStaleCycles
		// instead of forcing a kill() probe on every cycle.

		s.nextDcstatCt[app.Pid] = app.Ct

		current := netdataPublishDCStatPid{
			CacheAccess: app.CacheAccess,
			FileSystem:  app.FileSystem,
			NotFound:    app.NotFound,
		}
		previous, hasPrevious := s.dcstatPrev[app.Pid]

		// Carry the counter baseline forward even for a stale candidate.  The
		// process may simply be idle-but-alive: dropping its previous counters
		// would make its next burst of activity look like a first sample and be
		// suppressed entirely.  The state is bounded by the BPF map contents, so
		// it disappears on its own once the PID is confirmed dead and deleted.
		s.nextDcstat[app.Pid] = current

		if stale {
			// No row this cycle: the caller decides whether the PID is dead.
			continue
		}

		var ident ebpfModuleIdentity
		copy(ident.comm[:], app.Comm[:])
		ident.ppid = app.Ppid

		s.dcstatData[app.Pid] = buildDCStatPublish(current, previous, app.Ct, hasPrevious)
		s.dcstatIdent[app.Pid] = ident
		pids, ordered = appendAscending(pids, app.Pid, ordered)
	}

	// The native snapshot path already sorts by pid; only a test or fallback
	// caller supplying unordered input pays for the sort.
	if !ordered {
		sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })
	}

	s.dcstatPIDs = pids
	s.dcstatPrev, s.nextDcstat = s.nextDcstat, s.dcstatPrev
	s.dcstatPrevCt, s.nextDcstatCt = s.nextDcstatCt, s.dcstatPrevCt
	s.dcstatMiss, s.nextDcstatMs = s.nextDcstatMs, s.dcstatMiss
	s.dcstatStale = stalePIDs
	s.activeModules |= ebpfgoSHMFlagDCStat // mark dcstat as an active producer
	s.rebuildEntriesLocked()
	return stalePIDs
}
