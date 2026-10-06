package main

import (
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

func TestProcessSharedMemoryStorePublishesRowsAndDeltas(t *testing.T) {
	store := NewEbpfSharedMemoryStore()
	var comm [libbpfloader.ProcessAppCommLen]byte
	copy(comm[:], "worker")
	store.UpdateAppsProcess([]libbpfloader.ProcessAppSnapshot{{Pid: 42, Ppid: 1, Comm: comm, Ct: 10, Forks: 4}})
	store.UpdateAppsProcess([]libbpfloader.ProcessAppSnapshot{{Pid: 42, Ppid: 1, Comm: comm, Ct: 20, Forks: 7, Errors: 2}})
	if store.activeModules&ebpfgoSHMFlagProcess == 0 {
		t.Fatal("process update did not mark the process module active")
	}

	rows := store.Snapshot()
	if len(rows) != 1 || rows[0].pid != 42 {
		t.Fatalf("rows = %+v, want one process row for PID 42", rows)
	}
	if rows[0].process.CreateProcess != 3 || rows[0].process.TaskErr != 2 {
		t.Fatalf("process row = %+v, want interval deltas", rows[0].process)
	}
	if rows[0].process.Name[0] != 'w' {
		t.Fatalf("process name = %q, want worker", rows[0].process.Name)
	}
}
