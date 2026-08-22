package main

import (
	"slices"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// buildDCStatPublish converts one PID's raw directory-cache counters into the
// shared-memory row.
//
// Curr/Prev carry the raw cumulative counters so consumers can derive
// per-interval deltas for any of the three counters themselves (the C struct has
// only one scalar delta field). Ratio and CacheAccess are per-interval values
// for the shared-memory consumers; the legacy C global ratio was cumulative.
//
// On the first sample for a PID there is no previous reading, so Prev is set
// equal to Curr rather than left zero: every consumer derives its deltas as
// Curr - Prev, and a zero Prev would publish the process's entire pre-existing
// counter history as one interval's activity.  This also repeats whenever a PID
// re-enters the BPF map or the producer restarts, so it has to be handled here,
// once, rather than in each consumer.
func buildDCStatPublish(
	current, previous netdataPublishDCStatPid,
	ct uint64,
	hasPrevious bool,
	updateEverySec uint32,
) netdataPublishDCStat {
	if !hasPrevious {
		return netdataPublishDCStat{Ct: ct, Curr: current, Prev: current, UpdateEverySec: updateEverySec}
	}

	reference := diffCounters(current.CacheAccess, previous.CacheAccess)

	return netdataPublishDCStat{
		Ct:             ct,
		Ratio:          dcstatHitRatio(reference, diffCounters(current.NotFound, previous.NotFound)),
		CacheAccess:    reference,
		Curr:           current,
		Prev:           previous,
		UpdateEverySec: updateEverySec,
	}
}

// ClearDCStatApps drops dcstat's contribution after a failed collection cycle:
// the flag is cleared and the rows are emptied, so whichever module owns the
// segment publishes neither a dcstat_ok header nor last cycle's values.
//
// It is a separate entry point rather than UpdateDCStatApps(nil) because
// SnapshotApps returns a nil slice for a successfully-read but empty BPF map,
// and an empty map is a valid cycle, not a failed one.
//
// The counter baselines (dcstatPrev/dcstatPrevCt) are intentionally preserved:
// the BPF counters keep advancing while collection is broken, so keeping them
// lets the first recovered cycle publish a real delta instead of re-baselining.
func (s *ebpfSharedMemoryStore) ClearDCStatApps() {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.dcstatData)
	clear(s.dcstatIdent)
	s.dcstatPIDs = s.dcstatPIDs[:0]
	s.activeModules &^= ebpfgoSHMFlagDCStat
	s.rebuildEntriesLocked()
}

// RemoveDCStatPIDs drops state only after the runtime has successfully removed
// the same PIDs. A reused PID must self-baseline instead of inheriting counters
// from the exited process.
func (s *ebpfSharedMemoryStore) RemoveDCStatPIDs(pids []uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pid := range pids {
		delete(s.dcstatData, pid)
		delete(s.dcstatIdent, pid)
		delete(s.dcstatPrev, pid)
		delete(s.dcstatPrevCt, pid)
		delete(s.dcstatMiss, pid)
	}
	s.rebuildEntriesLocked()
}

// UpdateDCStatApps updates the in-memory snapshot from the latest dcstat BPF
// snapshot.  It returns the PIDs whose ct has not advanced for ebpfStaleCycles
// consecutive cycles; the caller performs the authoritative liveness check
// (libbpfloader.PidIsAlive) before removing them from the kernel BPF map.
//
// dcstat keeps its own ct/miss bookkeeping because its BPF map advances
// independently of cachestat's.
func (s *ebpfSharedMemoryStore) UpdateDCStatApps(
	apps []libbpfloader.DCStatAppSnapshot,
	updateEverySec uint32,
) []uint32 {
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

	// One token per cycle, stamped on every PID that is active in it, so tokens
	// issued later are always strictly greater than every token issued before —
	// the property both consumers rely on and the one the BPF `bpf_ktime_get_ns()`
	// stamp used to provide, including across a plugin restart.
	publishToken := s.nextDCStatTokenLocked()

	for _, app := range apps {
		current := netdataPublishDCStatPid{
			CacheAccess: app.CacheAccess,
			FileSystem:  app.FileSystem,
			NotFound:    app.NotFound,
		}
		previous, hasPrevious := s.dcstatPrev[app.Pid]

		// Freshness is derived from the counters, never from app.Ct.  Only the
		// buffer and arena BPF objects stamp ct on every event; the CO-RE base
		// object never writes it (so it stays 0 forever) and the legacy object
		// writes it once when the map entry is created and never again.  Gating on
		// ct would therefore suppress every delta on those two flavors, which are
		// reachable as the last loader fallback and as the only path on hosts
		// without BTF.  The three counters are monotonic per map entry, so "any
		// counter moved" is an exact activity signal on every flavor.
		advanced := !hasPrevious || current != previous

		// The published token is synthetic: an active PID is stamped with this
		// cycle's token and an idle one holds its previous stamp, which is exactly
		// the contract the C consumers' `ct > last_consumed_ct` gates expect.
		//
		// The stamp MUST come from the store-wide clock rather than from a per-PID
		// increment.  apps.plugin keeps a watermark per PID (`p->ebpf_dcstat_ct`)
		// and would not care, but cgroup_ebpfgo_dcstat_sum_pids() keeps ONE
		// watermark per cgroup (`cg->dcstat.ct`) and compares every member PID
		// against it.  With per-PID counters a long-lived PID's high count would sit
		// above a freshly added PID's low count forever, so the new PID's activity
		// would never be counted.
		lastCt, seen := s.dcstatPrevCt[app.Pid]
		publishCt := lastCt
		if advanced {
			publishCt = publishToken
		}

		stale := false
		if seen && !advanced {
			miss := s.dcstatMiss[app.Pid] + 1
			if miss >= ebpfStaleCycles {
				stale = true
				stalePIDs = append(stalePIDs, app.Pid)
			} else {
				s.nextDcstatMs[app.Pid] = miss
			}
		}
		// New PID or counters advanced: miss count stays 0 (Go zero-value).  A
		// stale candidate also resets it, so it is re-flagged every
		// ebpfStaleCycles instead of forcing a kill() probe on every cycle.

		s.nextDcstatCt[app.Pid] = publishCt

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

		s.dcstatData[app.Pid] = buildDCStatPublish(current, previous, publishCt, hasPrevious, updateEverySec)
		s.dcstatIdent[app.Pid] = ident
		pids, ordered = appendAscending(pids, app.Pid, ordered)
	}

	// The native snapshot path already sorts by pid; only a test or fallback
	// caller supplying unordered input pays for the sort.
	if !ordered {
		slices.Sort(pids)
	}

	s.dcstatPIDs = pids
	// Rotate the baselines only when this cycle actually recorded some.  An empty
	// snapshot (SnapshotApps returns nil for an empty BPF map or accumulator) must
	// not swap in the empty map: that would discard every PID's previous counters,
	// so the next active cycle would look like a first sample and silently drop one
	// interval of activity.  Socket's prevSocketData rotation is gated the same way.
	if len(s.nextDcstat) > 0 {
		s.dcstatPrev, s.nextDcstat = s.nextDcstat, s.dcstatPrev
		s.dcstatPrevCt, s.nextDcstatCt = s.nextDcstatCt, s.dcstatPrevCt
		s.dcstatMiss, s.nextDcstatMs = s.nextDcstatMs, s.dcstatMiss
	}
	s.dcstatStale = stalePIDs
	s.activeModules |= ebpfgoSHMFlagDCStat // mark dcstat as an active producer
	s.rebuildEntriesLocked()
	return stalePIDs
}
