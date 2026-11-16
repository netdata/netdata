package main

import (
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// processApp builds a snapshot whose BPF ct never changes, which is what the base
// and legacy objects produce: ct is written only when the map entry is created.
func processApp(pid uint32, forks, clones, exits, errors uint32) libbpfloader.ProcessAppSnapshot {
	var comm [libbpfloader.ProcessAppCommLen]byte
	copy(comm[:], "worker")
	return libbpfloader.ProcessAppSnapshot{
		Pid: pid, Ppid: 1, Comm: comm, Ct: 7,
		Forks: forks, Clones: clones, Exits: exits, TaskClose: exits, Errors: errors,
	}
}

// withTaskClose sets release_call, which the BPF programs reset on every fork,
// clone and exec of the same TGID (ebpf-co-re src/process.bpf.c).
func withTaskClose(app libbpfloader.ProcessAppSnapshot, taskClose uint32) libbpfloader.ProcessAppSnapshot {
	app.TaskClose = taskClose
	return app
}

func TestUpdateProcessAppsPublishesIntervalDeltas(t *testing.T) {
	cases := map[string]struct {
		cycles [][]libbpfloader.ProcessAppSnapshot
		want   ebpfProcessStat
	}{
		"first sighting publishes the counters since the entry was created": {
			cycles: [][]libbpfloader.ProcessAppSnapshot{{processApp(42, 4, 1, 2, 0)}},
			want: ebpfProcessStat{
				Tgid: 42, Pid: 42,
				CreateProcess: 4, CreateThread: 1, ExitCall: 2, ReleaseCall: 2,
				ProcessUpdateEverySec: 10,
			},
		},
		"later cycle carries the delta even with a constant BPF ct": {
			cycles: [][]libbpfloader.ProcessAppSnapshot{
				{processApp(42, 4, 1, 2, 0)},
				{processApp(42, 7, 2, 3, 2)},
			},
			want: ebpfProcessStat{
				Tgid: 42, Pid: 42,
				CreateProcess: 3, CreateThread: 1, ExitCall: 1, ReleaseCall: 3, TaskErr: 2,
				ProcessUpdateEverySec: 10,
			},
		},
		"release_call without a reset carries the delta": {
			cycles: [][]libbpfloader.ProcessAppSnapshot{
				{withTaskClose(processApp(42, 4, 1, 2, 0), 1)},
				{withTaskClose(processApp(42, 4, 1, 2, 0), 3)},
			},
			want: ebpfProcessStat{Tgid: 42, Pid: 42, ReleaseCall: 2, ProcessUpdateEverySec: 10},
		},
		"release_call reset below the baseline counts releases since the reset": {
			cycles: [][]libbpfloader.ProcessAppSnapshot{
				{withTaskClose(processApp(42, 4, 1, 2, 0), 5)},
				{withTaskClose(processApp(42, 5, 1, 2, 0), 2)},
			},
			want: ebpfProcessStat{Tgid: 42, Pid: 42, CreateProcess: 1, ReleaseCall: 2, ProcessUpdateEverySec: 10},
		},
		"release_call reset above the baseline counts releases since the reset": {
			cycles: [][]libbpfloader.ProcessAppSnapshot{
				{withTaskClose(processApp(42, 4, 1, 2, 0), 1)},
				{withTaskClose(processApp(42, 5, 1, 2, 0), 3)},
			},
			want: ebpfProcessStat{Tgid: 42, Pid: 42, CreateProcess: 1, ReleaseCall: 3, ProcessUpdateEverySec: 10},
		},
		"failed fork resets release_call": {
			cycles: [][]libbpfloader.ProcessAppSnapshot{
				{withTaskClose(processApp(42, 4, 1, 2, 0), 1)},
				{withTaskClose(processApp(42, 4, 1, 2, 1), 3)},
			},
			want: ebpfProcessStat{Tgid: 42, Pid: 42, ReleaseCall: 3, TaskErr: 1, ProcessUpdateEverySec: 10},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := NewEbpfSharedMemoryStore()
			for _, apps := range tc.cycles {
				store.UpdateProcessApps(apps, false, 10)
			}

			rows := store.Snapshot()
			if len(rows) != 1 {
				t.Fatalf("rows = %d, want 1", len(rows))
			}
			got := rows[0].process
			if got.Ct == 0 {
				t.Fatal("published ct is 0; consumers would never treat the row as fresh")
			}
			if string(got.Name[:6]) != "worker" {
				t.Fatalf("name = %q, want worker", got.Name)
			}
			got.Ct, got.Name = 0, [TASK_COMM_LEN]byte{}
			if got != tc.want {
				t.Fatalf("process row = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestUpdateProcessAppsTokenAdvancesOnlyForActivePIDs(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateProcessApps([]libbpfloader.ProcessAppSnapshot{processApp(1, 1, 0, 0, 0), processApp(2, 1, 0, 0, 0)}, false, 10)
	first := store.processPrevCt[2]

	store.UpdateProcessApps([]libbpfloader.ProcessAppSnapshot{processApp(1, 2, 0, 0, 0), processApp(2, 1, 0, 0, 0)}, false, 10)

	if store.processPrevCt[1] <= first {
		t.Fatalf("active PID ct = %d, want > %d", store.processPrevCt[1], first)
	}
	if store.processPrevCt[2] != first {
		t.Fatalf("idle PID ct = %d, want unchanged %d", store.processPrevCt[2], first)
	}
}

func TestUpdateProcessAppsFlagsStalePIDsAfterDebounce(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	apps := []libbpfloader.ProcessAppSnapshot{processApp(9, 1, 0, 0, 0)}

	var stale []uint32
	for range ebpfStaleCycles + 1 {
		stale = store.UpdateProcessApps(apps, false, 10)
	}

	if len(stale) != 1 || stale[0] != 9 {
		t.Fatalf("stale = %v, want [9]", stale)
	}
	if _, ok := store.processPrev[9]; !ok {
		t.Fatal("stale candidate lost its baseline; its next activity would be dropped")
	}
	if len(store.Snapshot()) != 0 {
		t.Fatal("stale candidate still published a row")
	}
}

func TestUpdateProcessAppsDropsBaselinesOfDepartedPIDs(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateProcessApps([]libbpfloader.ProcessAppSnapshot{processApp(1, 1, 0, 0, 0), processApp(2, 1, 0, 0, 0)}, false, 10)
	store.UpdateProcessApps([]libbpfloader.ProcessAppSnapshot{processApp(1, 2, 0, 0, 0)}, false, 10)

	want := map[uint32]bool{1: true}
	for _, m := range []map[uint32]processCounters{store.processPrev} {
		for pid := range m {
			if !want[pid] {
				t.Fatalf("baseline for departed PID %d retained", pid)
			}
		}
	}
	if _, ok := store.processPrevCt[2]; ok {
		t.Fatal("freshness token for departed PID 2 retained")
	}
}

func TestUpdateProcessAppsEmptySnapshotKeepsBaselines(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateProcessApps([]libbpfloader.ProcessAppSnapshot{processApp(5, 1, 0, 0, 0)}, false, 10)
	store.UpdateProcessApps(nil, false, 10)

	if _, ok := store.processPrev[5]; !ok {
		t.Fatal("an empty snapshot discarded the baselines")
	}
}

func TestProcessErrorsFlagFollowsLoadMode(t *testing.T) {
	cases := map[string]struct {
		reportErrors bool
		want         uint32
	}{
		"entry":  {reportErrors: false, want: ebpfgoSHMFlagProcess},
		"return": {reportErrors: true, want: ebpfgoSHMFlagProcess | ebpfgoSHMFlagProcessErrors},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := NewEbpfSharedMemoryStore()
			store.UpdateProcessApps([]libbpfloader.ProcessAppSnapshot{processApp(1, 1, 0, 0, 0)}, tc.reportErrors, 10)
			if got := store.activeModules & processSHMFlags; got != tc.want {
				t.Fatalf("flags = %#x, want %#x", got, tc.want)
			}

			store.ClearProcessApps()
			if store.activeModules&processSHMFlags != 0 || len(store.Snapshot()) != 0 {
				t.Fatal("ClearProcessApps left process rows or flags")
			}
		})
	}
}

func TestRemoveProcessPIDsForgetsTheExitedProcess(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	store.UpdateProcessApps([]libbpfloader.ProcessAppSnapshot{processApp(3, 5, 0, 0, 0)}, false, 10)
	store.RemoveProcessPIDs([]uint32{3})

	// A reused PID has a new BPF entry: publish its counters, not a delta
	// against the exited process' counters.
	store.UpdateProcessApps([]libbpfloader.ProcessAppSnapshot{processApp(3, 2, 0, 0, 0)}, false, 10)
	if got := store.Snapshot()[0].process.CreateProcess; got != 2 {
		t.Fatalf("reused PID create_process = %d, want 2", got)
	}
}
