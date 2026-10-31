package main

import (
	"slices"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// processStaleCycles is the process-facing name of the shared ct-stagnation
// debouncer window (see ebpfStaleCycles).
const processStaleCycles = ebpfStaleCycles

func buildProcessPublish(current, previous netdataProcess, ct uint64, hasPrevious bool) netdataPublishProcess {
	publish := netdataPublishProcess{
		Ct:      ct,
		Current: current,
		Prev:    previous,
	}

	if !hasPrevious {
		return publish
	}

	publish.Exits = diffCounters(uint64(current.Exits), uint64(previous.Exits))
	publish.TaskClose = diffCounters(uint64(current.TaskClose), uint64(previous.TaskClose))
	publish.Forks = diffCounters(uint64(current.Forks), uint64(previous.Forks))
	publish.Clones = diffCounters(uint64(current.Clones), uint64(previous.Clones))
	publish.Errors = diffCounters(uint64(current.Errors), uint64(previous.Errors))

	return publish
}

// UpdateApps updates the in-memory snapshot from the latest process BPF
// snapshot. It returns the PIDs whose ct has not advanced for
// processStaleCycles consecutive cycles; the caller is responsible for the
// authoritative liveness check (libbpfloader.PidIsAlive) before removing them
// from the kernel BPF map.
func (s *ebpfSharedMemoryStore) UpdateAppsProcess(apps []libbpfloader.ProcessAppSnapshot) []uint32 {
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

	for _, app := range apps {
		lastCt, seen := s.processPrevCt[app.Pid]
		stale := false
		if seen && app.Ct == lastCt {
			miss := s.processMiss[app.Pid] + 1
			s.nextProcessMs[app.Pid] = miss
			if miss >= processStaleCycles {
				stale = true
				stalePIDs = append(stalePIDs, app.Pid)
			}
		}

		current := netdataProcess{
			Exits:     app.Exits,
			TaskClose: app.TaskClose,
			Forks:     app.Forks,
			Clones:    app.Clones,
			Errors:    app.Errors,
		}

		hasPrevious := seen && !stale
		publish := buildProcessPublish(current, s.processPrev[app.Pid], app.Ct, hasPrevious)

		s.processData[app.Pid] = publish
		s.nextProcessCt[app.Pid] = app.Ct
		s.processPrev[app.Pid] = current
		s.processPrevCt[app.Pid] = app.Ct

		ident := ebpfModuleIdentity{}
		copy(ident.comm[:], app.Comm[:])
		ident.ppid = app.Ppid
		s.processIdent[app.Pid] = ident

		pids = append(pids, app.Pid)

		if !seen || (len(pids) > 1 && pids[len(pids)-1] < pids[len(pids)-2]) {
			ordered = false
		}
	}

	if !ordered {
		slices.Sort(pids)
	}

	s.processPIDs = pids
	s.processStale = stalePIDs
	s.processMiss, s.nextProcessMs = s.nextProcessMs, s.processMiss
	clear(s.nextProcessMs)
	// Mark process data as active before the shared-memory publisher snapshots
	// the store. Without this bit, a process-owned publisher emits no valid
	// module flags and apps.plugin suppresses all eBPF chart announcements.
	s.activeModules |= ebpfgoSHMFlagProcess
	s.rebuildEntriesLocked()
	return stalePIDs
}

func (s *ebpfSharedMemoryStore) ClearProcessApps() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.processData)
	clear(s.processIdent)
	s.processPIDs = s.processPIDs[:0]
	s.activeModules &^= ebpfgoSHMFlagProcess
	s.rebuildEntriesLocked()
}

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

// ProcessSnapshot is the public interface for retrieving process data.
func (s *ebpfSharedMemoryStore) ProcessSnapshot() (map[uint32]netdataPublishProcess, map[uint32]ebpfModuleIdentity) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dataCopy := make(map[uint32]netdataPublishProcess, len(s.processData))
	for pid, data := range s.processData {
		dataCopy[pid] = data
	}

	identCopy := make(map[uint32]ebpfModuleIdentity, len(s.processIdent))
	for pid, ident := range s.processIdent {
		identCopy[pid] = ident
	}

	return dataCopy, identCopy
}
