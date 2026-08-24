package main

import (
	"testing"
	"unsafe"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

func fdApp(pid uint32, open, closed, openErr, closeErr uint32) libbpfloader.FDAppSnapshot {
	return libbpfloader.FDAppSnapshot{
		Pid:       pid,
		OpenCall:  open,
		CloseCall: closed,
		OpenErr:   openErr,
		CloseErr:  closeErr,
	}
}

func fdRow(t *testing.T, store *ebpfSharedMemoryStore, pid uint32) netdataPublishFDStat {
	t.Helper()

	for _, entry := range store.Snapshot() {
		if entry.pid == pid {
			return entry.fd
		}
	}
	t.Fatalf("no shared-memory row for pid %d", pid)
	return netdataPublishFDStat{}
}

// TestFDCountersBaselineIsMinimal pins the baseline shape: the store keeps only
// the four counters, not the whole snapshot.  At the default pid table size the
// difference is megabytes of resident memory across the two baseline maps.
func TestFDCountersBaselineIsMinimal(t *testing.T) {
	if got, want := unsafe.Sizeof(fdCounters{}), uintptr(16); got != want {
		t.Fatalf("unsafe.Sizeof(fdCounters{}) = %d, want %d (four uint32 counters and nothing else)",
			got, want)
	}

	app := fdApp(1, 2, 3, 4, 5)
	copy(app.Comm[:], "netdata")
	app.Ppid = 9
	app.Ct = 12345

	// comm/ppid/ct are deliberately NOT carried: identity comes from fdIdent and
	// freshness from the synthetic token.
	if got := fdCountersOf(app); got != (fdCounters{OpenCall: 2, CloseCall: 3, OpenErr: 4, CloseErr: 5}) {
		t.Fatalf("fdCountersOf() = %+v", got)
	}
}

func TestFDDeltaU32(t *testing.T) {
	tests := map[string]struct {
		current, previous, want uint32
	}{
		"normal increase":        {current: 10, previous: 4, want: 6},
		"no change":              {current: 7, previous: 7, want: 0},
		"first sample from zero": {current: 5, previous: 0, want: 5},
		// A counter that went backwards means the entry was recreated (PID reuse)
		// or the accumulator was rebuilt, so the new reading IS the interval's
		// activity.  Returning 0 would silently drop it — these are uint32
		// counters that a busy process really does wrap.
		"regression returns the new reading": {current: 3, previous: 4_000_000_000, want: 3},
		"wrap to zero":                       {current: 0, previous: 4_294_967_295, want: 0},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := fdDeltaU32(tc.current, tc.previous); got != tc.want {
				t.Fatalf("fdDeltaU32(%d, %d) = %d, want %d", tc.current, tc.previous, got, tc.want)
			}
		})
	}
}

// TestUpdateFDAppsFirstCycleSuppressesBacklog is the invariant that keeps the
// charts honest across a plugin restart: the BPF entry may already hold hours of
// counters, and publishing them as one interval would spike every consumer.
func TestUpdateFDAppsFirstCycleSuppressesBacklog(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(42, 5_000, 4_900, 12, 3)}, false)

	row := fdRow(t, store, 42)
	if row.OpenCall != 0 || row.CloseCall != 0 || row.OpenErr != 0 || row.CloseErr != 0 {
		t.Fatalf("first cycle published %+v, want all-zero deltas", row)
	}
	if row.Ct == 0 {
		t.Fatal("first cycle must still stamp a freshness token")
	}
}

func TestUpdateFDAppsPublishesIntervalDeltas(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(42, 100, 90, 4, 1)}, false)
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(42, 130, 120, 6, 1)}, false)

	row := fdRow(t, store, 42)
	want := netdataPublishFDStat{OpenCall: 30, CloseCall: 30, OpenErr: 2, CloseErr: 0}
	if row.OpenCall != want.OpenCall || row.CloseCall != want.CloseCall ||
		row.OpenErr != want.OpenErr || row.CloseErr != want.CloseErr {
		t.Fatalf("second cycle published %+v, want %+v", row, want)
	}
}

// TestUpdateFDAppsTokenAdvancesOnlyForActivePIDs pins the freshness contract the
// C consumers gate on: an active PID gets this cycle's token, an idle one keeps
// its previous stamp so the consumer skips it instead of re-counting a delta.
func TestUpdateFDAppsTokenAdvancesOnlyForActivePIDs(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(1, 10, 10, 0, 0), fdApp(2, 20, 20, 0, 0)}, false)
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(1, 11, 10, 0, 0), fdApp(2, 20, 20, 0, 0)}, false)

	activeCt := fdRow(t, store, 1).Ct
	idleCt := fdRow(t, store, 2).Ct

	// Cycle 2 stamped PID 1; PID 2 kept cycle 1's stamp.
	if activeCt <= idleCt {
		t.Fatalf("active pid ct = %d, idle pid ct = %d; active must be strictly greater", activeCt, idleCt)
	}

	// A third cycle where neither moved must not advance either stamp.
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(1, 11, 10, 0, 0), fdApp(2, 20, 20, 0, 0)}, false)
	if got := fdRow(t, store, 1).Ct; got != activeCt {
		t.Fatalf("idle cycle advanced pid 1 ct from %d to %d", activeCt, got)
	}
}

// TestUpdateFDAppsFlagsStalePIDsAfterDebounce pins the debouncer: a PID is only
// offered for removal after ebpfStaleCycles quiet cycles, so the caller's kill()
// probe does not run on every PID every cycle.
func TestUpdateFDAppsFlagsStalePIDsAfterDebounce(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	apps := []libbpfloader.FDAppSnapshot{fdApp(7, 3, 3, 0, 0)}

	// Cycle 1 establishes the baseline; a PID seen for the first time is never a
	// stale candidate.
	if stale := store.UpdateFDApps(apps, false); len(stale) != 0 {
		t.Fatalf("cycle 1 flagged %v, want none", stale)
	}

	for cycle := 2; cycle < 2+ebpfStaleCycles-1; cycle++ {
		if stale := store.UpdateFDApps(apps, false); len(stale) != 0 {
			t.Fatalf("cycle %d flagged %v before the debounce window elapsed", cycle, stale)
		}
	}

	stale := store.UpdateFDApps(apps, false)
	if len(stale) != 1 || stale[0] != 7 {
		t.Fatalf("stale = %v, want [7]", stale)
	}

	// A stale candidate publishes no row this cycle: the caller decides whether
	// the PID is really dead.
	for _, entry := range store.Snapshot() {
		if entry.pid == 7 {
			t.Fatal("a stale candidate must not publish a row")
		}
	}
}

// TestUpdateFDAppsStaleCandidateKeepsItsBaseline covers the idle-but-alive case:
// the PID was flagged, the caller found it alive, and its next burst of activity
// must still produce a real delta rather than being suppressed as a first sample.
func TestUpdateFDAppsStaleCandidateKeepsItsBaseline(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	apps := []libbpfloader.FDAppSnapshot{fdApp(7, 3, 3, 0, 0)}
	for range ebpfStaleCycles + 1 {
		store.UpdateFDApps(apps, false)
	}

	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(7, 9, 3, 0, 0)}, false)
	if got := fdRow(t, store, 7).OpenCall; got != 6 {
		t.Fatalf("OpenCall after the PID woke up = %d, want 6 (9 - 3)", got)
	}
}

func TestRemoveFDPIDsForcesReBaseline(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(11, 100, 100, 0, 0)}, false)
	store.RemoveFDPIDs([]uint32{11})

	if len(store.Snapshot()) != 0 {
		t.Fatalf("Snapshot after RemoveFDPIDs = %+v, want empty", store.Snapshot())
	}

	// A reused PID must self-baseline rather than inherit the exited process's
	// counters, so its first cycle publishes zero deltas again.
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(11, 5, 5, 0, 0)}, false)
	if row := fdRow(t, store, 11); row.OpenCall != 0 {
		t.Fatalf("reused pid published OpenCall = %d, want 0", row.OpenCall)
	}
}

func TestClearFDAppsDropsRowsAndFlag(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(3, 1, 1, 0, 0)}, false)
	if store.activeModules&ebpfgoSHMFlagFD == 0 {
		t.Fatal("UpdateFDApps did not set the FD flag")
	}

	store.ClearFDApps()
	if store.activeModules&ebpfgoSHMFlagFD != 0 {
		t.Fatalf("ClearFDApps left activeModules = %#x", store.activeModules)
	}
	if len(store.Snapshot()) != 0 {
		t.Fatalf("ClearFDApps left rows: %+v", store.Snapshot())
	}

	// The baselines survive, so the first recovered cycle publishes a real delta
	// instead of re-baselining.
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(3, 4, 1, 0, 0)}, false)
	if got := fdRow(t, store, 3).OpenCall; got != 3 {
		t.Fatalf("OpenCall after recovery = %d, want 3 (4 - 1)", got)
	}
}

// TestUpdateFDAppsEmptySnapshotKeepsBaselines guards the rotation gate: an empty
// BPF map is a valid cycle, and swapping in the empty baseline map would make the
// next active cycle look like a first sample and drop one interval of activity.
func TestUpdateFDAppsEmptySnapshotKeepsBaselines(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(5, 50, 50, 0, 0)}, false)
	store.UpdateFDApps(nil, false)
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(5, 60, 50, 0, 0)}, false)

	if got := fdRow(t, store, 5).OpenCall; got != 10 {
		t.Fatalf("OpenCall = %d, want 10 (60 - 50); the baseline was lost", got)
	}
}

func TestMarkFDInactiveClearsOnlyTheFDFlag(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(1, 1, 1, 0, 0)}, false)
	store.UpdateSocketApps([]libbpfloader.SocketPIDEntry{{PID: 1, BytesSent: 1}}, 10)

	store.MarkFDInactive()
	if store.activeModules&ebpfgoSHMFlagFD != 0 {
		t.Fatal("MarkFDInactive did not clear the FD flag")
	}
	if store.activeModules&ebpfgoSHMFlagSocket == 0 {
		t.Fatal("MarkFDInactive cleared another module's flag")
	}
}

// TestFDPublishClearsOnlyItsOwnFlag pins the per-module publish contract: other
// modules' bits persist so a consumer does not see them flap when the modules run
// at different cadences.
func TestFDPublishClearsOnlyItsOwnFlag(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(1, 1, 1, 0, 0)}, false)
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{{Pid: 1, CacheAccess: 1}}, 10)

	if err := store.Publish(nil, ebpfgoSHMFlagFD); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if store.activeModules&ebpfgoSHMFlagFD != 0 {
		t.Fatalf("FD bit still set after Publish: activeModules = %#x", store.activeModules)
	}
	if store.activeModules&ebpfgoSHMFlagDCStat == 0 {
		t.Fatal("Publish(FD) cleared the DCSTAT bit")
	}
}

// TestRebuildEntriesMergesThreeAscendingLists exercises the k-way merge that
// replaced the hand-written two-way one: every PID must appear exactly once,
// ascending, carrying whichever modules hold data for it.
func TestRebuildEntriesMergesThreeAscendingLists(t *testing.T) {
	store := NewEbpfSharedMemoryStore()

	// Overlapping and disjoint PIDs across all four producers.
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{{Pid: 2, Ct: 1}, {Pid: 6, Ct: 1}})
	store.UpdateDCStatApps([]libbpfloader.DCStatAppSnapshot{{Pid: 2, CacheAccess: 1}, {Pid: 4, CacheAccess: 1}}, 10)
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(1, 1, 1, 0, 0), fdApp(4, 1, 1, 0, 0), fdApp(9, 1, 1, 0, 0)}, false)
	store.UpdateSocketApps([]libbpfloader.SocketPIDEntry{{PID: 3}, {PID: 6}}, 10)

	snap := store.Snapshot()
	wantPIDs := []uint32{1, 2, 3, 4, 6, 9}
	if len(snap) != len(wantPIDs) {
		t.Fatalf("Snapshot has %d entries, want %d: %+v", len(snap), len(wantPIDs), snap)
	}
	for i, want := range wantPIDs {
		if snap[i].pid != want {
			t.Fatalf("entry %d pid = %d, want %d (entries must be ascending and deduplicated)",
				i, snap[i].pid, want)
		}
	}
}

// TestBuildRowIdentityFallsBackAcrossModules pins the identity rule: an empty
// comm from one module must never shadow a populated one from another, because a
// BPF entry created before the first bpf_get_current_comm() carries no name.
func TestBuildRowIdentityFallsBackAcrossModules(t *testing.T) {
	store := NewEbpfSharedMemoryStore()

	// cachestat sees the PID first but learned no name; fd learned one.
	store.UpdateApps([]libbpfloader.CachestatAppSnapshot{{Pid: 8, Ct: 1}})

	named := fdApp(8, 1, 1, 0, 0)
	copy(named.Comm[:], "netdata")
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{named}, false)

	snap := store.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("Snapshot has %d entries, want 1", len(snap))
	}
	if got := string(snap[0].comm[:len("netdata")]); got != "netdata" {
		t.Fatalf("row comm = %q, want the name fd learned", got)
	}
}

// TestUpdateFDAppsErrorFlagTracksReportErrors pins the mechanism that carries
// `ebpf load mode` across the segment: apps.plugin and cgroups.plugin are
// separate processes and cannot see fd's config, so the errors bit is the only
// thing telling them whether the error charts may exist.
func TestUpdateFDAppsErrorFlagTracksReportErrors(t *testing.T) {
	store := NewEbpfSharedMemoryStore()

	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(1, 1, 1, 0, 0)}, false)
	if store.activeModules&ebpfgoSHMFlagFDErrors != 0 {
		t.Fatalf("entry mode set the FD errors bit: activeModules = %#x", store.activeModules)
	}

	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(1, 2, 1, 0, 0)}, true)
	if store.activeModules&ebpfgoSHMFlagFDErrors == 0 {
		t.Fatalf("return mode did not set the FD errors bit: activeModules = %#x", store.activeModules)
	}

	// A live config reload back to entry mode must retract it, not leave a stale
	// bit that keeps the consumers' error charts alive.
	store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(1, 3, 1, 0, 0)}, false)
	if store.activeModules&ebpfgoSHMFlagFDErrors != 0 {
		t.Fatalf("errors bit survived a switch back to entry mode: activeModules = %#x", store.activeModules)
	}
}

// TestFDFlagsAreClearedTogether guards the pairing: leaving the errors bit set
// after fd stops publishing would let consumers keep drawing error charts from a
// dead producer.
func TestFDFlagsAreClearedTogether(t *testing.T) {
	for name, stop := range map[string]func(*ebpfSharedMemoryStore){
		"ClearFDApps":    func(s *ebpfSharedMemoryStore) { s.ClearFDApps() },
		"MarkFDInactive": func(s *ebpfSharedMemoryStore) { s.MarkFDInactive() },
		"Publish":        func(s *ebpfSharedMemoryStore) { _ = s.Publish(nil, fdSHMFlags) },
	} {
		t.Run(name, func(t *testing.T) {
			store := NewEbpfSharedMemoryStore()
			store.UpdateFDApps([]libbpfloader.FDAppSnapshot{fdApp(1, 1, 1, 0, 0)}, true)
			if store.activeModules&fdSHMFlags != fdSHMFlags {
				t.Fatalf("setup did not set both fd bits: activeModules = %#x", store.activeModules)
			}

			stop(store)
			if store.activeModules&fdSHMFlags != 0 {
				t.Fatalf("%s left fd bits set: activeModules = %#x", name, store.activeModules)
			}
		})
	}
}
