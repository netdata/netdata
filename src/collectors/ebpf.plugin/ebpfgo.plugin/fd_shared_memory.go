package main

import (
	"slices"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// fdCounters is the per-PID baseline the store carries between cycles.
//
// Only the four counters, not the whole libbpfloader.FDAppSnapshot: the snapshot
// also carries comm[96] and ppid, which no baseline reader touches.  Retaining
// them would cost ~112 extra bytes per PID in EACH of the two baseline maps —
// about 7 MB at the default pid table size of 32768 — for nothing.  cachestat and
// dcstat keep their baselines the same way.
type fdCounters struct {
	OpenCall  uint32
	CloseCall uint32
	OpenErr   uint32
	CloseErr  uint32
}

func fdCountersOf(app libbpfloader.FDAppSnapshot) fdCounters {
	return fdCounters{
		OpenCall:  app.OpenCall,
		CloseCall: app.CloseCall,
		OpenErr:   app.OpenErr,
		CloseErr:  app.CloseErr,
	}
}

// fdDeltaU32 diffs two 32-bit BPF counters.
//
// A counter that went backwards means the map entry was recreated (PID reuse) or
// the accumulator was rebuilt, so the new reading IS the interval's activity.
// Returning it rather than 0 matters more here than for the 64-bit counters:
// these are uint32 and the BPF programs increment them without saturation, so a
// very busy process wraps in normal operation, and treating a wrap as "no
// activity" would silently drop a whole interval.
func fdDeltaU32(current, previous uint32) uint32 {
	if current < previous {
		return current
	}

	return current - previous
}

// buildFDPublish converts one PID's raw file-descriptor counters into the
// shared-memory row.
//
// The row carries PER-INTERVAL deltas because struct ebpf_publish_fd_stat has a
// single counter set and no curr/prev pair to derive them from — unlike
// ebpf_publish_dcstat.  Consumers therefore never diff: apps.plugin accumulates
// the deltas into monotonic totals for its `incremental` charts, and the cgroups
// consumer sums them for the interval.
//
// On the first sample for a PID there is no previous reading, so the deltas are
// zero rather than the process's entire pre-existing counter history: fd entries
// can be created long before this plugin starts reading them, and publishing
// that backlog as one interval's activity would spike every chart.  The same
// applies whenever a PID re-enters the BPF map or the producer restarts, so it is
// handled here, once, rather than in each consumer.
func buildFDPublish(
	current fdCounters,
	previous fdCounters,
	ct uint64,
	hasPrevious bool,
	updateEvery uint32,
) netdataPublishFDStat {
	if !hasPrevious {
		return netdataPublishFDStat{Ct: ct, UpdateEverySec: updateEvery}
	}

	return netdataPublishFDStat{
		Ct:             ct,
		OpenCall:       fdDeltaU32(current.OpenCall, previous.OpenCall),
		CloseCall:      fdDeltaU32(current.CloseCall, previous.CloseCall),
		OpenErr:        fdDeltaU32(current.OpenErr, previous.OpenErr),
		CloseErr:       fdDeltaU32(current.CloseErr, previous.CloseErr),
		UpdateEverySec: updateEvery,
	}
}

// fdCountersAdvanced reports whether any of the four counters moved.
//
// Freshness is derived from the counters, never from app.Ct.  Only the buffer
// and arena BPF objects stamp ct on every event; the CO-RE base and legacy
// objects write it once when the map entry is created and never again.  Gating on
// ct would therefore suppress every delta on those flavors, which are reachable
// as the last loader fallback and as the only path on hosts without BTF.  The
// four counters are monotonic per map entry, so "any counter moved" is an exact
// activity signal on every flavor.
func fdCountersAdvanced(current, previous fdCounters) bool {
	return current != previous
}

// ClearFDApps drops fd's contribution after a failed collection cycle: the flag
// is cleared and the rows are emptied, so whichever module owns the segment
// publishes neither an fd_ok header nor last cycle's values.
//
// It is a separate entry point rather than UpdateFDApps(nil) because SnapshotApps
// returns a nil slice for a successfully-read but empty BPF map, and an empty map
// is a valid cycle, not a failed one.
//
// The counter baselines (fdPrev/fdPrevCt) are intentionally preserved: the BPF
// counters keep advancing while collection is broken, so keeping them lets the
// first recovered cycle publish a real delta instead of re-baselining.
func (s *ebpfSharedMemoryStore) ClearFDApps() {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.fdData)
	clear(s.fdIdent)
	s.fdPIDs = s.fdPIDs[:0]
	s.activeModules &^= ebpfgoSHMFlagFD | ebpfgoSHMFlagFDErrors
	s.rebuildEntriesLocked()
}

// RemoveFDPIDs drops state only after the runtime has successfully removed the
// same PIDs.  A reused PID must self-baseline instead of inheriting counters from
// the exited process.
func (s *ebpfSharedMemoryStore) RemoveFDPIDs(pids []uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pid := range pids {
		delete(s.fdData, pid)
		delete(s.fdIdent, pid)
		delete(s.fdPrev, pid)
		delete(s.fdPrevCt, pid)
		delete(s.fdMiss, pid)
	}
	s.fdPIDs = removeFromSortedPIDs(s.fdPIDs, pids)
	s.rebuildEntriesLocked()
}

// UpdateFDApps updates the in-memory snapshot from the latest fd BPF snapshot.
// It returns the PIDs whose counters have not advanced for ebpfStaleCycles
// consecutive cycles; the caller performs the authoritative liveness check
// (libbpfloader.PidIsAlive) before removing them from the kernel BPF map.
//
// fd keeps its own ct/miss bookkeeping because its BPF map advances independently
// of cachestat's and dcstat's.
//
// reportErrors is `ebpf load mode = return`.  It does not change what is
// collected — the counters are always populated — only whether the consumers are
// told they may show the error charts.  See ebpfgoSHMFlagFDErrors.
func (s *ebpfSharedMemoryStore) UpdateFDApps(
	apps []libbpfloader.FDAppSnapshot,
	reportErrors bool,
	updateEveryS ...uint32,
) []uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.fdData)
	clear(s.fdIdent)
	clear(s.nextFd)
	clear(s.nextFdCt)
	clear(s.nextFdMs)
	pids := s.fdPIDs[:0]
	stalePIDs := s.fdStale[:0]
	ordered := true

	// One token per cycle, stamped on every PID that is active in it, so tokens
	// issued later are always strictly greater than every token issued before.
	publishToken := s.fdToken.next()
	updateEvery := uint32(fdDefaultUpdateEvery)
	if len(updateEveryS) > 0 && updateEveryS[0] > 0 {
		updateEvery = updateEveryS[0]
	}

	for _, app := range apps {
		current := fdCountersOf(app)
		previous, hasPrevious := s.fdPrev[app.Pid]
		advanced := !hasPrevious || fdCountersAdvanced(current, previous)

		// An active PID is stamped with this cycle's token; an idle one holds its
		// previous stamp, which is exactly the contract the C consumers'
		// `ct > last_consumed_ct` gates expect.  See ebpfFreshnessToken for why the
		// stamp is store-wide and boot-relative.
		lastCt, seen := s.fdPrevCt[app.Pid]
		publishCt := lastCt
		if advanced {
			publishCt = publishToken
		}

		stale := false
		if seen && !advanced {
			miss := s.fdMiss[app.Pid] + 1
			if miss >= ebpfStaleCycles {
				stale = true
				stalePIDs = append(stalePIDs, app.Pid)
			} else {
				s.nextFdMs[app.Pid] = miss
			}
		}
		// New PID or counters advanced: miss count stays 0 (Go zero-value).  A
		// stale candidate also resets it, so it is re-flagged every
		// ebpfStaleCycles instead of forcing a kill() probe on every cycle.

		s.nextFdCt[app.Pid] = publishCt

		// Carry the counter baseline forward even for a stale candidate.  The
		// process may simply be idle-but-alive: dropping its previous counters
		// would make its next burst of activity look like a first sample and be
		// suppressed entirely.  The state is bounded by the BPF map contents, so
		// it disappears on its own once the PID is confirmed dead and deleted.
		s.nextFd[app.Pid] = current

		if stale {
			// No row this cycle: the caller decides whether the PID is dead.
			continue
		}

		var ident ebpfModuleIdentity
		copy(ident.comm[:], app.Comm[:])
		ident.ppid = app.Ppid

		s.fdData[app.Pid] = buildFDPublish(current, previous, publishCt, hasPrevious, updateEvery)
		s.fdIdent[app.Pid] = ident
		pids, ordered = appendAscending(pids, app.Pid, ordered)
	}

	// The native snapshot path already sorts by pid; only a test or fallback
	// caller supplying unordered input pays for the sort.
	if !ordered {
		slices.Sort(pids)
	}

	s.fdPIDs = pids
	// Rotate the baselines only when this cycle actually recorded some.  An empty
	// snapshot (SnapshotApps returns nil for an empty BPF map or accumulator) must
	// not swap in the empty map: that would discard every PID's previous counters,
	// so the next active cycle would look like a first sample and silently drop one
	// interval of activity.
	if len(s.nextFd) > 0 {
		s.fdPrev, s.nextFd = s.nextFd, s.fdPrev
		s.fdPrevCt, s.nextFdCt = s.nextFdCt, s.fdPrevCt
		s.fdMiss, s.nextFdMs = s.nextFdMs, s.fdMiss
	}
	s.fdStale = stalePIDs
	s.activeModules |= ebpfgoSHMFlagFD // mark fd as an active producer
	if reportErrors {
		s.activeModules |= ebpfgoSHMFlagFDErrors
	} else {
		s.activeModules &^= ebpfgoSHMFlagFDErrors
	}
	s.rebuildEntriesLocked()
	return stalePIDs
}
