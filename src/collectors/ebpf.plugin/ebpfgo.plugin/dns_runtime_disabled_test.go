//go:build !netdata_ebpf_libbpf

package main

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// TestDNSRuntimeDisabledBuild verifies that all DNSRuntime methods return
// ErrDisabled in the non-libbpf build, preventing silent no-ops masking load
// failures when libbpf is unavailable.
func TestDNSRuntimeDisabledBuild(t *testing.T) {
	_, err := libbpfloader.NewDNSRuntime("/dev/null", false, false)
	if !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("NewDNSRuntime: got %v, want ErrDisabled", err)
	}

	var rt *libbpfloader.DNSRuntime

	if err := rt.Prepare(); !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("Prepare: got %v, want ErrDisabled", err)
	}
	if err := rt.Load(); !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("Load: got %v, want ErrDisabled", err)
	}
	if err := rt.Attach(); !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("Attach: got %v, want ErrDisabled", err)
	}
	if _, err := rt.Snapshot(); !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("Snapshot: got %v, want ErrDisabled", err)
	}
	rt.Close() // must not panic
}
