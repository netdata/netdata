//go:build !netdata_ebpf_libbpf

package main

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// TestFDRuntimeDisabledBuild verifies every fd runtime entry point returns the
// disabled sentinel when the plugin is built without libbpf, so the collector
// degrades instead of panicking on a nil runtime.
func TestFDRuntimeDisabledBuild(t *testing.T) {
	if libbpfloader.FDSupportsCore() {
		t.Fatal("FDSupportsCore must be false without libbpf")
	}

	rt, err := libbpfloader.NewFDRuntime("/nonexistent/fd.o", true)
	if rt != nil || !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("NewFDRuntime() = (%v, %v), want (nil, ErrDisabled)", rt, err)
	}

	var nilRT *libbpfloader.FDRuntime
	for name, call := range map[string]func() error{
		"Prepare":          func() error { return nilRT.Prepare(1024, true, "close_fd") },
		"Load":             func() error { return nilRT.Load() },
		"Attach":           func() error { return nilRT.Attach("do_sys_openat2", "close_fd") },
		"UpdateController": func() error { return nilRT.UpdateController(true, 0) },
		"DeletePid":        func() error { return nilRT.DeletePid(1) },
		"DeletePids":       func() error { return nilRT.DeletePids([]uint32{1, 2}) },
	} {
		if err := call(); !errors.Is(err, libbpfloader.ErrDisabled) {
			t.Fatalf("%s() = %v, want ErrDisabled", name, err)
		}
	}

	if _, err := nilRT.Snapshot(true); !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("Snapshot() = %v, want ErrDisabled", err)
	}
	if _, err := nilRT.SnapshotApps(true); !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("SnapshotApps() = %v, want ErrDisabled", err)
	}

	// Close must be safe on a nil runtime (the collector always defers it).
	nilRT.Close()
}

// TestLoadFDLegacyDisabledBuild verifies the plugin-level loader reports the
// disabled sentinel rather than attempting a load.  The config carries resolved
// targets so the test exercises the disabled path and not the earlier
// unresolved-targets guard.
func TestLoadFDLegacyDisabledBuild(t *testing.T) {
	cfg := defaultFDLegacyConfig()
	cfg.Targets = FDTargets{Open: "do_sys_openat2", Close: "close_fd"}

	handle, err := LoadFDLegacy(cfg)
	if handle != nil || !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("LoadFDLegacy() = (%v, %v), want (nil, ErrDisabled)", handle, err)
	}
}

// TestFDHandleCloseIsNilSafe guards the collector's deferred Close path.
func TestFDHandleCloseIsNilSafe(t *testing.T) {
	var handle *FDLegacyHandle
	handle.Close()

	handle = &FDLegacyHandle{}
	handle.Close()
}
