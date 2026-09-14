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
	assertRuntimeDisabledStubs(t, "DCStatRuntime",
		libbpfloader.DCStatSupportsCore,
		func() (*libbpfloader.DCStatRuntime, error) {
			return libbpfloader.NewDCStatRuntime("/nonexistent/dc.o", true)
		},
		runtimeDisabledOps{
			Mutators: []struct {
				Name string
				Call func() error
			}{
				{"Prepare", func() error { return (*libbpfloader.DCStatRuntime)(nil).Prepare(1024, true) }},
				{"Load", func() error { return (*libbpfloader.DCStatRuntime)(nil).Load() }},
				{"Attach", func() error { return (*libbpfloader.DCStatRuntime)(nil).Attach("lookup_fast", "d_lookup") }},
				{"UpdateController", func() error { return (*libbpfloader.DCStatRuntime)(nil).UpdateController(true, 0) }},
				{"DeletePid", func() error { return (*libbpfloader.DCStatRuntime)(nil).DeletePid(1) }},
				{"DeletePids", func() error { return (*libbpfloader.DCStatRuntime)(nil).DeletePids([]uint32{1, 2}) }},
			},
			Snapshot:     func() error { _, err := (*libbpfloader.DCStatRuntime)(nil).Snapshot(true); return err },
			SnapshotApps: func() error { _, err := (*libbpfloader.DCStatRuntime)(nil).SnapshotApps(true); return err },
			Close:        func() { (*libbpfloader.DCStatRuntime)(nil).Close() },
		})
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
