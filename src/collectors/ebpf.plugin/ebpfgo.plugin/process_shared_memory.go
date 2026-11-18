package main

import (
	"slices"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// processCounters is one TGID's cumulative per-PID counters.
type processCounters struct {
	Exits     uint32
	TaskClose uint32
	Forks     uint32
	Clones    uint32
	Errors    uint32
}

func processCountersOf(app libbpfloader.ProcessAppSnapshot) processCounters {
	return processCounters{
		Exits:     app.Exits,
		TaskClose: app.TaskClose,
		Forks:     app.Forks,
		Clones:    app.Clones,
		Errors:    app.Errors,
	}
}

// netdataPublishProcess is one PID's per-interval delta, written to the SHM row.
// Ct is the store-wide freshness token, not the BPF ct.
type netdataPublishProcess struct {
	Ct             uint64
	Exits          uint32
	TaskClose      uint32
	Forks          uint32
	Clones         uint32
	Errors         uint32
	UpdateEverySec uint32
}

func processDelta(current, previous uint32) uint32 {
	if current < previous {
		return 0
	}
	return current - previous
}

// processTaskCloseDelta handles release_call, the one counter the base and
// legacy objects reset: on every fork, clone and exec of the TGID (ebpf-co-re
// src/process.bpf.c), but only in the slot of the CPU that ran it, while the
// snapshot folds every CPU's slot.  A fork elsewhere therefore says nothing about
// the folded value, and the buffer/arena accumulators never reset at all, so only
// a decrease is taken as a reset.  Releases lost to a reset are not recoverable;
// treating a fork as a reset would instead republish other slots' releases.
func processTaskCloseDelta(current, previous processCounters) uint32 {
	if current.TaskClose < previous.TaskClose {
		return current.TaskClose
	}
	return current.TaskClose - previous.TaskClose
}

// buildProcessPublish publishes the full counters of a PID seen for the first
// time.  Unlike fd, this is exact: tbl_pid_stats is not pinned, so it starts
// empty at load and every entry is created by an event this runtime observed,
// and a PID re-enters the store only after its entry was deleted.  Without this,
// a process that starts and exits within one interval would never be counted.
func buildProcessPublish(current, previous processCounters, ct uint64, hasPrevious bool, updateEvery uint32) netdataPublishProcess {
	if !hasPrevious {
		return netdataPublishProcess{
			Ct:             ct,
			UpdateEverySec: updateEvery,
			Exits:          current.Exits,
			TaskClose:      current.TaskClose,
			Forks:          current.Forks,
			Clones:         current.Clones,
			Errors:         current.Errors,
		}
	}
	return netdataPublishProcess{
		Ct:             ct,
		UpdateEverySec: updateEvery,
		Exits:          processDelta(current.Exits, previous.Exits),
		TaskClose:      processTaskCloseDelta(current, previous),
		Forks:          processDelta(current.Forks, previous.Forks),
		Clones:         processDelta(current.Clones, previous.Clones),
		Errors:         processDelta(current.Errors, previous.Errors),
	}
}

// UpdateProcessApps updates the in-memory snapshot from the latest process BPF
// snapshot and returns the PIDs whose counters have not advanced for
// ebpfStaleCycles consecutive cycles; the caller performs the authoritative
// liveness check before removing them from the BPF map.
//
// Freshness comes from the counters, never from app.Ct: the base and legacy
// objects write ct only when the map entry is created (ebpf-co-re
// src/process.bpf.c), so a ct gate would suppress every later delta.
//
// reportErrors is `ebpf load mode = return`; it only tells consumers whether they
// may show the error charts.  See ebpfgoSHMFlagProcessErrors.  updateEvery is the
// interval the deltas cover; consumers divide by it.
func (s *ebpfSharedMemoryStore) UpdateProcessApps(apps []libbpfloader.ProcessAppSnapshot, reportErrors bool, updateEvery uint32) []uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.processData)
	clear(s.processIdent)
	clear(s.nextProcess)
	clear(s.nextProcessCt)
	clear(s.nextProcessMs)
	pids := s.processPIDs[:0]
	stalePIDs := s.processStale[:0]
	ordered := true
	publishToken := s.processToken.next()

	for _, app := range apps {
		current := processCountersOf(app)
		previous, hasPrevious := s.processPrev[app.Pid]
		advanced := !hasPrevious || current != previous

		lastCt, seen := s.processPrevCt[app.Pid]
		publishCt := lastCt
		if advanced {
			publishCt = publishToken
		}

		stale := false
		if seen && !advanced {
			miss := s.processMiss[app.Pid] + 1
			if miss >= ebpfStaleCycles {
				stale = true
				stalePIDs = append(stalePIDs, app.Pid)
			} else {
				s.nextProcessMs[app.Pid] = miss
			}
		}

		s.nextProcessCt[app.Pid] = publishCt
		// Keep the baseline of an idle-but-alive PID so its next activity is a
		// real delta; the state disappears once the PID leaves the BPF map.
		s.nextProcess[app.Pid] = current

		if stale {
			continue
		}

		var ident ebpfModuleIdentity
		copy(ident.comm[:], app.Comm[:])
		ident.ppid = app.Ppid

		s.processData[app.Pid] = buildProcessPublish(current, previous, publishCt, hasPrevious, updateEvery)
		s.processIdent[app.Pid] = ident
		pids, ordered = appendAscending(pids, app.Pid, ordered)
	}

	if !ordered {
		slices.Sort(pids)
	}

	s.processPIDs = pids
	// An empty snapshot must not discard every baseline (see UpdateFDApps).
	if len(s.nextProcess) > 0 {
		s.processPrev, s.nextProcess = s.nextProcess, s.processPrev
		s.processPrevCt, s.nextProcessCt = s.nextProcessCt, s.processPrevCt
		s.processMiss, s.nextProcessMs = s.nextProcessMs, s.processMiss
	}
	s.processStale = stalePIDs
	s.activeModules |= ebpfgoSHMFlagProcess
	if reportErrors {
		s.activeModules |= ebpfgoSHMFlagProcessErrors
	} else {
		s.activeModules &^= ebpfgoSHMFlagProcessErrors
	}
	s.rebuildEntriesLocked()
	return stalePIDs
}

// ClearProcessApps drops process's contribution after a failed collection cycle.
// Baselines are kept so the first recovered cycle publishes a real delta.
func (s *ebpfSharedMemoryStore) ClearProcessApps() {
	s.mu.Lock()
	defer s.mu.Unlock()

	clear(s.processData)
	clear(s.processIdent)
	s.processPIDs = s.processPIDs[:0]
	s.activeModules &^= processSHMFlags
	s.rebuildEntriesLocked()
}

// MarkProcessInactive clears the process flags on goroutine exit.
func (s *ebpfSharedMemoryStore) MarkProcessInactive() {
	s.mu.Lock()
	s.activeModules &^= processSHMFlags
	s.mu.Unlock()
}

// RemoveProcessPIDs drops state only after the runtime removed the same PIDs, so
// a reused PID publishes its new entry's counters instead of a delta against the
// exited process' counters.
func (s *ebpfSharedMemoryStore) RemoveProcessPIDs(pids []uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pid := range pids {
		delete(s.processData, pid)
		delete(s.processIdent, pid)
		delete(s.processPrev, pid)
		delete(s.processPrevCt, pid)
		delete(s.processMiss, pid)
	}
	s.processPIDs = removeFromSortedPIDs(s.processPIDs, pids)
	s.rebuildEntriesLocked()
}
