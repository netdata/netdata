package main

import (
	"sort"
	"sync"
)

// ebpfStaleCycles is the debouncer window before a PID is flagged as a stale
// candidate and the caller performs the authoritative liveness check
// (libbpfloader.PidIsAlive).  Without this debouncer we would run kill() on
// every PID every cycle, which is too expensive.
const ebpfStaleCycles = 3

// ebpfgoSHM* mirrors the EBPFGO_SHM_FLAG_* C constants so Go callers do not
// need to import "C" just to set a flag value.
const (
	ebpfgoSHMFlagCachestat uint32 = 0x01
	ebpfgoSHMFlagSocket    uint32 = 0x02
	ebpfgoSHMFlagDCStat    uint32 = 0x04
	ebpfgoSHMFlagFD        uint32 = 0x08
	// ebpfgoSHMFlagFDErrors is set alongside ebpfgoSHMFlagFD when fd runs with
	// `ebpf load mode = return`.  fd counts open/close errors on every mode (its
	// probes always read the syscall return value), but the C module only created
	// the error charts in `return` mode, and apps.plugin and cgroups.plugin are
	// separate processes that cannot see fd's config.  This bit carries that one
	// decision across the segment; it lives in the existing header `flags` word,
	// so it is not a layout change.
	ebpfgoSHMFlagFDErrors uint32 = 0x10
)

// Production POSIX names for the shared-memory segment and its semaphore.
// Must match NETDATA_EBPFGO_INTEGRATION_NAME / NETDATA_EBPFGO_SHM_INTEGRATION_NAME
// in apps_ebpf_shared_pid_row.h, which is what all consumer plugins open.
const (
	productionSHMName = "/netdata_shm_integration_ebpfgo_v5"
	productionSEMName = "/netdata_sem_integration_ebpfgo_v5"
)

// ebpfFreshnessToken issues the synthetic `ct` stamp a module publishes for its
// per-PID rows.  It replaces the raw BPF ct, which is unusable as a freshness
// signal: only the buffer and arena objects stamp it per event, while the CO-RE
// base and legacy objects write it once at map-entry creation (or never).
//
// Two properties are load-bearing:
//
//   - Store-wide, not per-PID.  apps.plugin keeps a watermark per PID and would
//     not care, but the cgroups consumers keep ONE watermark per cgroup and
//     compare every member PID against it.  With per-PID counters a long-lived
//     PID's high value would sit above a freshly added PID's low value forever,
//     so the new PID's activity would never be counted.
//
//   - Boot-relative, not a plain counter.  cgroups.plugin is compiled into the
//     daemon and its watermark only ever moves forward, deliberately, so a PID
//     leaving a cgroup cannot replay rows.  A counter restarting at 1 after an
//     ebpf-go.plugin restart would sit below the watermark the daemon still
//     holds and freeze the charts for as long as the previous instance ran.
//     bootNanos() keeps rising across plugin restarts and resets only on reboot,
//     which restarts the daemon too.  See bootNanos.
//
// Each publishing module owns its own instance: they collect on independent
// intervals, and their consumers keep separate watermarks.
type ebpfFreshnessToken struct {
	last uint64
}

// next returns this cycle's token.  The clock reading is clamped to be strictly
// increasing so two cycles landing in the same nanosecond still produce distinct
// tokens, which is what the consumers' `ct > last_consumed_ct` gates require.
func (t *ebpfFreshnessToken) next() uint64 {
	token := bootNanos()
	if token <= t.last {
		token = t.last + 1
	}
	t.last = token
	return token
}

// ebpfModuleIdentity carries the per-PID process attributes a module learned
// from its BPF map.  Any module that reads comm/ppid can supply them; the row
// builder takes the first non-empty value in module order.
type ebpfModuleIdentity struct {
	comm [EBPF_MAX_COMPARE_NAME + 1]byte
	ppid uint32
}

// isEmpty reports whether this module learned nothing about the process.
func (i ebpfModuleIdentity) isEmpty() bool {
	return i.ppid == 0 && i.comm[0] == 0
}

// ebpfSharedMemoryStore holds one row per PID, merged from every eBPF module
// that publishes per-PID data (cachestat, dcstat, socket).
//
// Each module keeps its own current-cycle contribution in a dedicated map, plus
// the previous raw counters and the ct-stagnation debouncer it needs to compute
// deltas and detect exited PIDs.  After any module update, s.entries is rebuilt
// from the union of those maps, so a PID that disappears from every module's
// snapshot is evicted automatically rather than lingering in shared memory.
//
// The cachestat and dcstat pid lists arrive sorted by PID from the native
// snapshot, so the rebuild is a linear two-way merge; socket-only PIDs come
// from a map and are appended, which is the only case that needs a sort.
type ebpfSharedMemoryStore struct {
	mu sync.RWMutex

	entries     []ebpfPidStat
	nextEntries []ebpfPidStat

	// ---- cachestat ----
	cachestatData   map[uint32]netdataPublishCachestat
	cachestatIdent  map[uint32]ebpfModuleIdentity
	cachestatPIDs   []uint32 // ascending; drives the merge
	cachestatPrev   map[uint32]netdataCachestat
	nextCachestat   map[uint32]netdataCachestat
	cachestatPrevCt map[uint32]uint64
	nextCachestatCt map[uint32]uint64
	cachestatMiss   map[uint32]int
	nextCachestatMs map[uint32]int
	cachestatStale  []uint32

	// ---- dcstat ----
	dcstatData  map[uint32]netdataPublishDCStat
	dcstatIdent map[uint32]ebpfModuleIdentity
	dcstatPIDs  []uint32 // ascending; drives the merge
	dcstatPrev  map[uint32]netdataPublishDCStatPid
	nextDcstat  map[uint32]netdataPublishDCStatPid
	// dcstatPrevCt/nextDcstatCt hold the *synthetic* freshness token this store
	// publishes for dcstat, not the BPF `ct` field.  See UpdateDCStatApps for why
	// the BPF value cannot be used.
	dcstatPrevCt map[uint32]uint64
	nextDcstatCt map[uint32]uint64
	// dcstatToken issues the synthetic freshness stamp; see ebpfFreshnessToken.
	dcstatToken  ebpfFreshnessToken
	dcstatMiss   map[uint32]int
	nextDcstatMs map[uint32]int
	dcstatStale  []uint32

	// ---- fd (file descriptor) ----
	//
	// fdData holds PER-INTERVAL deltas, not cumulative counters: struct
	// ebpf_publish_fd_stat has a single counter set with no curr/prev pair, so the
	// diff has to happen here.  apps.plugin accumulates them into monotonic
	// totals for its `incremental` charts (the cachestat pattern); the cgroups
	// consumer sums them per interval.
	fdData  map[uint32]netdataPublishFDStat
	fdIdent map[uint32]ebpfModuleIdentity
	fdPIDs  []uint32 // ascending; drives the merge
	fdPrev  map[uint32]fdCounters
	nextFd  map[uint32]fdCounters
	// fdPrevCt/nextFdCt hold the synthetic freshness token, not the BPF `ct`.
	fdPrevCt map[uint32]uint64
	nextFdCt map[uint32]uint64
	fdToken  ebpfFreshnessToken
	fdMiss   map[uint32]int
	nextFdMs map[uint32]int
	fdStale  []uint32

	// ---- socket ----
	socketData         map[uint32]ebpfSocketPublishApps // per-interval deltas written to SHM this cycle
	prevSocketData     map[uint32]ebpfSocketPublishApps // raw cumulative counters from the previous cycle
	nextPrevSocketData map[uint32]ebpfSocketPublishApps // scratch buffer for the next prevSocketData

	activeModules uint32 // EBPFGO_SHM_FLAG_* bits set when a module writes data
}

func NewEbpfSharedMemoryStore() *ebpfSharedMemoryStore {
	return &ebpfSharedMemoryStore{
		cachestatData:      make(map[uint32]netdataPublishCachestat),
		cachestatIdent:     make(map[uint32]ebpfModuleIdentity),
		cachestatPrev:      make(map[uint32]netdataCachestat),
		nextCachestat:      make(map[uint32]netdataCachestat),
		cachestatPrevCt:    make(map[uint32]uint64),
		nextCachestatCt:    make(map[uint32]uint64),
		cachestatMiss:      make(map[uint32]int),
		nextCachestatMs:    make(map[uint32]int),
		dcstatData:         make(map[uint32]netdataPublishDCStat),
		dcstatIdent:        make(map[uint32]ebpfModuleIdentity),
		dcstatPrev:         make(map[uint32]netdataPublishDCStatPid),
		nextDcstat:         make(map[uint32]netdataPublishDCStatPid),
		dcstatPrevCt:       make(map[uint32]uint64),
		nextDcstatCt:       make(map[uint32]uint64),
		dcstatMiss:         make(map[uint32]int),
		nextDcstatMs:       make(map[uint32]int),
		fdData:             make(map[uint32]netdataPublishFDStat),
		fdIdent:            make(map[uint32]ebpfModuleIdentity),
		fdPrev:             make(map[uint32]fdCounters),
		nextFd:             make(map[uint32]fdCounters),
		fdPrevCt:           make(map[uint32]uint64),
		nextFdCt:           make(map[uint32]uint64),
		fdMiss:             make(map[uint32]int),
		nextFdMs:           make(map[uint32]int),
		socketData:         make(map[uint32]ebpfSocketPublishApps),
		prevSocketData:     make(map[uint32]ebpfSocketPublishApps),
		nextPrevSocketData: make(map[uint32]ebpfSocketPublishApps),
	}
}

// Snapshot returns a copy of the current in-memory entries.
// Used only by tests to inspect store state; production code reads via Publish.
func (s *ebpfSharedMemoryStore) Snapshot() []ebpfPidStat {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// s.entries is always sorted ascending by pid: rebuildEntriesLocked merges
	// two ascending PID lists and sorts whenever socket-only PIDs are appended.
	copied := make([]ebpfPidStat, len(s.entries))
	copy(copied, s.entries)
	return copied
}

// Publish writes the current entries to the shared-memory segment and stamps the
// per-module validity flags.
//
// Only the flag of the publishing module is cleared after each publish; the
// other modules' bits persist so a consumer does not see them flap when the
// modules run at different cadences.  The SOCKET bit is cleared by
// MarkSocketInactive on goroutine exit or per-cycle when collection fails.
//
// selfFlag is the caller's own EBPFGO_SHM_FLAG_* bit.
//
// The lock is held for the duration of the C memcpy because of scratch-buffer
// reuse, NOT because anything writes s.entries in place: rebuildEntriesLocked
// builds a fresh slice and swaps it in, which hands the array the C side is
// reading back to s.nextEntries.  The very next rebuild overwrites that array, so
// releasing the lock around the memcpy would expose a data race.  Do not drop the
// lock on the grounds that the swap makes it unnecessary.
func (s *ebpfSharedMemoryStore) Publish(publisher *SharedPidMemoryPublisher, selfFlag uint32) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	flags := s.activeModules
	s.activeModules &^= selfFlag

	if publisher == nil {
		return nil
	}
	return publisher.Publish(s.entries, flags)
}

// MarkDCStatInactive clears the DCSTAT flag from activeModules, so a consumer
// does not treat stale rows as live.
//
// The only production caller is the dcstat goroutine's exit path (main.go).  A
// per-cycle snapshot failure goes through ClearDCStatApps instead, which clears
// the flag AND empties the rows; this entry point exists for shutdown, where the
// rows are about to become unreachable anyway.
func (s *ebpfSharedMemoryStore) MarkDCStatInactive() {
	s.mu.Lock()
	s.activeModules &^= ebpfgoSHMFlagDCStat
	s.mu.Unlock()
}

// MarkFDInactive clears the FD flag from activeModules, so a consumer does not
// treat stale rows as live.
//
// The only production caller is the fd goroutine's exit path (main.go).  A
// per-cycle snapshot failure goes through ClearFDApps instead, which clears the
// flag AND empties the rows; this entry point exists for shutdown, where the
// rows are about to become unreachable anyway.
func (s *ebpfSharedMemoryStore) MarkFDInactive() {
	s.mu.Lock()
	s.activeModules &^= ebpfgoSHMFlagFD | ebpfgoSHMFlagFDErrors
	s.mu.Unlock()
}

// MarkSocketInactive clears the SOCKET flag from activeModules.  Called via
// defer when the socket goroutine exits (permanent shutdown) and also per-cycle
// when Snapshot or SnapshotPerPID fails so the consumer does not see
// socket_ok = true with zero data arrays.
func (s *ebpfSharedMemoryStore) MarkSocketInactive() {
	s.mu.Lock()
	s.activeModules &^= ebpfgoSHMFlagSocket
	s.mu.Unlock()
}

// ebpfSortedPIDLists is the number of modules that contribute an ASCENDING
// per-PID list to the row merge (cachestat, dcstat, fd).  Socket contributes a
// map instead and is appended separately.
const ebpfSortedPIDLists = 3

// mergedPIDIterator walks several ascending PID lists as one ascending sequence
// with duplicates collapsed.  It is a value type with fixed-size arrays so the
// per-cycle rebuild allocates nothing.
type mergedPIDIterator struct {
	lists   [ebpfSortedPIDLists][]uint32
	cursors [ebpfSortedPIDLists]int
}

// next returns the smallest PID not yet emitted, or false when every list is
// exhausted.  Every cursor sitting on that PID advances, because PIDs are unique
// within a single list.
func (it *mergedPIDIterator) next() (uint32, bool) {
	var pid uint32
	found := false
	for i := range it.lists {
		if it.cursors[i] >= len(it.lists[i]) {
			continue
		}
		if candidate := it.lists[i][it.cursors[i]]; !found || candidate < pid {
			pid, found = candidate, true
		}
	}
	if !found {
		return 0, false
	}

	for i := range it.lists {
		if it.cursors[i] < len(it.lists[i]) && it.lists[i][it.cursors[i]] == pid {
			it.cursors[i]++
		}
	}

	return pid, true
}

// rebuildEntriesLocked recomputes s.entries from every module's current-cycle
// contribution.  Must be called with s.mu held for writing.
func (s *ebpfSharedMemoryStore) rebuildEntriesLocked() {
	nextEntries := s.nextEntries[:0]
	upper := len(s.cachestatPIDs) + len(s.dcstatPIDs) + len(s.fdPIDs) + len(s.socketData)
	if cap(nextEntries) < upper {
		nextEntries = make([]ebpfPidStat, 0, upper)
	}

	// k-way merge of the ascending per-module PID lists.
	merge := mergedPIDIterator{lists: [ebpfSortedPIDLists][]uint32{
		s.cachestatPIDs, s.dcstatPIDs, s.fdPIDs,
	}}
	for {
		pid, ok := merge.next()
		if !ok {
			break
		}
		nextEntries = append(nextEntries, s.buildRowLocked(pid))
	}

	// Append PIDs seen only by socket, so services with network activity but no
	// page-cache, directory-cache or file-descriptor activity are still visible
	// to consumers.
	prevLen := len(nextEntries)
	for pid := range s.socketData {
		if !sortedEntriesContainPID(nextEntries[:prevLen], pid) {
			nextEntries = append(nextEntries, s.buildRowLocked(pid))
		}
	}
	if len(nextEntries) > prevLen {
		sort.Slice(nextEntries, func(a, b int) bool {
			return nextEntries[a].pid < nextEntries[b].pid
		})
	}

	s.entries, s.nextEntries = nextEntries, s.entries
}

// buildRowLocked assembles one shared-memory row from whichever modules have
// data for pid.  Identity (comm/ppid) is taken from cachestat first, then dcstat,
// then fd; socket rows carry no identity.
//
// Presence in a module's identity map is not enough: a BPF entry created between
// process start and the first bpf_get_current_comm() carries an empty comm, and
// the upstream BPF sources skip that call entirely on kernels below 4.11.  An
// empty identity from one module must therefore never shadow a populated one
// from another.
//
// ppid is always 0 today — no module's snapshot populates it — so isEmpty()
// effectively tests comm alone.  It still checks both fields so this stays
// correct if a module starts reporting a parent.
func (s *ebpfSharedMemoryStore) buildRowLocked(pid uint32) ebpfPidStat {
	row := ebpfPidStat{pid: pid}

	// Pick the identity to publish: the first module that actually learned
	// something wins.  No module may publish an empty identity while another
	// holds a populated one; when all are empty (or absent) the row stays at
	// zero, which is what a single module with no identity produced before.
	for _, ident := range [...]ebpfModuleIdentity{
		s.cachestatIdent[pid],
		s.dcstatIdent[pid],
		s.fdIdent[pid],
	} {
		if !ident.isEmpty() {
			row.comm = ident.comm
			row.ppid = ident.ppid
			break
		}
	}

	row.cachestat = s.cachestatData[pid]
	row.dc = s.dcstatData[pid]
	row.fd = s.fdData[pid]
	row.socket = s.socketData[pid]

	return row
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

// removeFromSortedPIDs drops every pid in remove from an ascending list,
// in place and order-preserving.
//
// Every Remove*PIDs entry point needs it: deleting a PID only from the module's
// data maps leaves its entry in the module's PID list, so the next
// rebuildEntriesLocked still emits an all-zero row for it and the PID keeps a
// slot in the fixed-size shared-memory segment until the module's next snapshot
// rebuilds the list.  Consumers ignore such a row (its ct is 0, which never
// passes their `ct > last_consumed_ct` gate), but on a busy host the wasted slots
// can crowd out live processes.
//
// remove is an eviction batch — a handful of PIDs confirmed dead — so a set
// lookup per surviving element is cheaper than sorting and merging.
func removeFromSortedPIDs(list []uint32, remove []uint32) []uint32 {
	if len(list) == 0 || len(remove) == 0 {
		return list
	}

	dead := make(map[uint32]struct{}, len(remove))
	for _, pid := range remove {
		dead[pid] = struct{}{}
	}

	kept := list[:0]
	for _, pid := range list {
		if _, ok := dead[pid]; !ok {
			kept = append(kept, pid)
		}
	}

	return kept
}

// appendAscending appends pid to dst, reporting whether dst stayed ascending.
// Callers use the result to decide whether a sort is needed; the native
// snapshot paths already emit PIDs in ascending order.
func appendAscending(dst []uint32, pid uint32, ordered bool) ([]uint32, bool) {
	if ordered && len(dst) > 0 && pid < dst[len(dst)-1] {
		ordered = false
	}
	return append(dst, pid), ordered
}
