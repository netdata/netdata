//go:build !netdata_ebpf_libbpf

package main

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// TestDCStatRuntimeDisabledBuild verifies every dcstat runtime entry point
// returns the disabled sentinel when the plugin is built without libbpf, so the
// collector degrades instead of panicking on a nil runtime.
func TestDCStatRuntimeDisabledBuild(t *testing.T) {
	if libbpfloader.DCStatSupportsCore() {
		t.Fatal("DCStatSupportsCore must be false without libbpf")
	}

	rt, err := libbpfloader.NewDCStatRuntime("/nonexistent/dc.o", true)
	if rt != nil || !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("NewDCStatRuntime() = (%v, %v), want (nil, ErrDisabled)", rt, err)
	}

	var nilRT *libbpfloader.DCStatRuntime
	for name, call := range map[string]func() error{
		"Prepare":          func() error { return nilRT.Prepare(1024, true) },
		"Load":             func() error { return nilRT.Load() },
		"Attach":           func() error { return nilRT.Attach("lookup_fast", "d_lookup") },
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

// TestLoadDCStatLegacyDisabledBuild verifies the plugin-level loader reports the
// disabled sentinel rather than attempting a load.
func TestLoadDCStatLegacyDisabledBuild(t *testing.T) {
	handle, err := LoadDCStatLegacy(defaultDCStatLegacyConfig())
	if handle != nil || !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("LoadDCStatLegacy() = (%v, %v), want (nil, ErrDisabled)", handle, err)
	}
}

// TestDCStatHandleCloseIsNilSafe guards the collector's deferred Close path.
func TestDCStatHandleCloseIsNilSafe(t *testing.T) {
	var handle *DCStatLegacyHandle
	handle.Close()

	handle = &DCStatLegacyHandle{}
	handle.Close()
}
