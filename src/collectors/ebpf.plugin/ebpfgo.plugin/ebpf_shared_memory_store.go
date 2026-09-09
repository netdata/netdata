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
)

// Production POSIX names for the shared-memory segment and its semaphore.
// Must match NETDATA_EBPFGO_INTEGRATION_NAME / NETDATA_EBPFGO_SHM_INTEGRATION_NAME
// in apps_ebpf_shared_pid_row.h, which is what all consumer plugins open.
const (
	productionSHMName = "/netdata_shm_integration_ebpfgo_v5"
	productionSEMName = "/netdata_sem_integration_ebpfgo_v5"
)

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
	// dcstatLastToken is the last freshness token stamped on an active PID.  A
	// single store-wide token keeps values comparable ACROSS PIDs: cgroups.plugin
	// holds one watermark per cgroup and compares every member PID against it, so a
	// per-PID sequence would let a long-running PID's high value permanently mask a
	// newly added one.  The token comes from bootNanos() rather than a counter so
	// it also survives a plugin restart — see bootNanos.
	dcstatLastToken uint64
	dcstatMiss      map[uint32]int
	nextDcstatMs    map[uint32]int
	dcstatStale     []uint32

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

// MarkSocketInactive clears the SOCKET flag from activeModules.  Called via
// defer when the socket goroutine exits (permanent shutdown) and also per-cycle
// when Snapshot or SnapshotPerPID fails so the consumer does not see
// socket_ok = true with zero data arrays.
func (s *ebpfSharedMemoryStore) MarkSocketInactive() {
	s.mu.Lock()
	s.activeModules &^= ebpfgoSHMFlagSocket
	s.mu.Unlock()
}

// nextDCStatTokenLocked returns the freshness token for this cycle.  The clock
// reading is clamped to be strictly increasing so two cycles that land in the
// same nanosecond still produce distinct tokens, which is what the consumers'
// `ct > last_consumed_ct` gates require.
func (s *ebpfSharedMemoryStore) nextDCStatTokenLocked() uint64 {
	token := bootNanos()
	if token <= s.dcstatLastToken {
		token = s.dcstatLastToken + 1
	}
	s.dcstatLastToken = token
	return token
}

// rebuildEntriesLocked recomputes s.entries from every module's current-cycle
// contribution.  Must be called with s.mu held for writing.
func (s *ebpfSharedMemoryStore) rebuildEntriesLocked() {
	nextEntries := s.nextEntries[:0]
	upper := len(s.cachestatPIDs) + len(s.dcstatPIDs) + len(s.socketData)
	if cap(nextEntries) < upper {
		nextEntries = make([]ebpfPidStat, 0, upper)
	}

	// Two-way merge of the ascending cachestat and dcstat PID lists.
	// Each iteration emits the smaller-or-equal PID and advances whichever
	// cursor(s) hold it; equality advances both because PIDs are unique within
	// each list.
	i, j := 0, 0
	for i < len(s.cachestatPIDs) || j < len(s.dcstatPIDs) {
		var pid uint32
		// Take from cachestat when it still has elements and (dcstat is empty or
		// cachestat's current PID is smaller-or-equal).  Explicit length checks
		// keep the indexed reads inside the loop invariant.  On equality the
		// pid already came from cachestat, so the inner `if` advances j too.
		if i < len(s.cachestatPIDs) && (j >= len(s.dcstatPIDs) || s.cachestatPIDs[i] <= s.dcstatPIDs[j]) {
			pid = s.cachestatPIDs[i]
			i++
			if j < len(s.dcstatPIDs) && pid == s.dcstatPIDs[j] {
				j++
			}
		} else {
			// dcstat holds the smaller-or-only PID.
			pid = s.dcstatPIDs[j]
			j++
		}
		nextEntries = append(nextEntries, s.buildRowLocked(pid))
	}

	// Append PIDs seen only by socket, so services with network activity but no
	// page-cache or directory-cache activity are still visible to consumers.
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
// data for pid.  Identity (comm/ppid) is taken from cachestat first, then
// dcstat; socket rows carry no identity.
//
// Presence in a module's identity map is not enough: a BPF entry created
// between process start and the first bpf_get_current_comm() carries an empty
// comm, and the upstream BPF sources skip that call entirely on kernels below
// 4.11.  An empty identity from either module must therefore never shadow a
// populated one from the other.
//
// ppid is always 0 today — neither module's snapshot populates it — so isEmpty()
// effectively tests comm alone.  It still checks both fields so this stays
// correct if a module starts reporting a parent.
func (s *ebpfSharedMemoryStore) buildRowLocked(pid uint32) ebpfPidStat {
	row := ebpfPidStat{pid: pid}

	// Pick the identity to publish: cachestat wins when it actually learned
	// something, then dcstat on the same terms.  Neither module may publish an
	// empty identity while the other holds a populated one; when both are empty
	// (or absent) the row stays at zero, which is what a single module with no
	// identity produced before.
	csIdent, csOK := s.cachestatIdent[pid]
	dcIdent, dcOK := s.dcstatIdent[pid]
	var ident ebpfModuleIdentity
	switch {
	case csOK && !csIdent.isEmpty():
		ident = csIdent
	case dcOK && !dcIdent.isEmpty():
		ident = dcIdent
	}
	row.comm = ident.comm
	row.ppid = ident.ppid

	row.cachestat = s.cachestatData[pid]
	row.dc = s.dcstatData[pid]
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

// appendAscending appends pid to dst, reporting whether dst stayed ascending.
// Callers use the result to decide whether a sort is needed; the native
// snapshot paths already emit PIDs in ascending order.
func appendAscending(dst []uint32, pid uint32, ordered bool) ([]uint32, bool) {
	if ordered && len(dst) > 0 && pid < dst[len(dst)-1] {
		ordered = false
	}
	return append(dst, pid), ordered
}
