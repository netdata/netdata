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
// The idle convention is the C collector's: with no directory-cache lookups this
// interval the ratio is 0, not 100.  cachestat deliberately uses 100 for its
// idle case; the two metrics are not interchangeable.
func buildDCStatPublish(current, previous netdataPublishDCStatPid, ct uint64, hasPrevious bool) netdataPublishDCStat {
	publish := netdataPublishDCStat{
		Ct:   ct,
		Curr: current,
		Prev: previous,
	}

	if !hasPrevious {
		return publish
	}

	reference := diffCounters(current.CacheAccess, previous.CacheAccess)
	notFound := diffCounters(current.NotFound, previous.NotFound)

	successful := max(reference-notFound, 0)

	if reference > 0 {
		publish.Ratio = int64((float64(successful) / float64(reference)) * 100)
	}

	publish.CacheAccess = reference
	return publish
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
		if seen && app.Ct == lastCt {
			miss := s.dcstatMiss[app.Pid] + 1
			if miss >= ebpfStaleCycles {
				stalePIDs = append(stalePIDs, app.Pid)
				continue
			}
			s.nextDcstatMs[app.Pid] = miss
		}
		// New PID or ct advanced: miss count stays 0 (Go zero-value).

		s.nextDcstatCt[app.Pid] = app.Ct

		current := netdataPublishDCStatPid{
			CacheAccess: app.CacheAccess,
			FileSystem:  app.FileSystem,
			NotFound:    app.NotFound,
		}
		previous, hasPrevious := s.dcstatPrev[app.Pid]

		var ident ebpfModuleIdentity
		copy(ident.comm[:], app.Comm[:])
		ident.ppid = app.Ppid

		s.dcstatData[app.Pid] = buildDCStatPublish(current, previous, app.Ct, hasPrevious)
		s.dcstatIdent[app.Pid] = ident
		s.nextDcstat[app.Pid] = current
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
