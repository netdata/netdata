//go:build !netdata_ebpf_libbpf

package main

import (
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// TestPidIsAliveDisabledBuild verifies the package-level PidIsAlive stub
// keeps the previous "always alive" behaviour in the disabled libbpf
// build.  This is the conservative default: when no BPF map is open there
// is no eviction to perform, and answering "alive" prevents the
// collector from deleting PIDs from any other shared state.
func TestPidIsAliveDisabledBuild(t *testing.T) {
	if !libbpfloader.PidIsAlive(1) {
		t.Fatalf("PidIsAlive should be conservative-true in the disabled build")
	}
	if !libbpfloader.PidIsAlive(0) {
		// 0 is the placeholder; the disabled build must still report alive
		// because no eviction should occur.
		t.Fatalf("PidIsAlive(0) should report alive in the disabled build")
	}
}

// TestRuntimeDeletePidsDisabledBuild verifies the bulk-delete entry point
// returns the disabled sentinel in the non-libbpf build so callers
// gracefully fall through.
func TestRuntimeDeletePidsDisabledBuild(t *testing.T) {
	var rt *libbpfloader.CachestatRuntime
	if err := rt.DeletePids([]uint32{1, 2, 3}); err == nil {
		t.Fatalf("DeletePids on nil runtime should return an error")
	}
}
