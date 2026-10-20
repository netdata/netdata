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
	assertRuntimeDisabledStubs(t, "FDRuntime",
		libbpfloader.FDSupportsCore,
		func() (*libbpfloader.FDRuntime, error) {
			return libbpfloader.NewFDRuntime("/nonexistent/fd.o", true, "/custom/btf/vmlinux")
		},
		runtimeDisabledOps{
			Mutators: []struct {
				Name string
				Call func() error
			}{
				{"Prepare", func() error { return (*libbpfloader.FDRuntime)(nil).Prepare(1024, true) }},
				{"Load", func() error { return (*libbpfloader.FDRuntime)(nil).Load() }},
				{"Attach", func() error { return (*libbpfloader.FDRuntime)(nil).Attach("do_sys_openat2", "close_fd") }},
				{"UpdateController", func() error { return (*libbpfloader.FDRuntime)(nil).UpdateController(true, 0) }},
				{"DeletePid", func() error { return (*libbpfloader.FDRuntime)(nil).DeletePid(1) }},
				{"DeletePids", func() error { return (*libbpfloader.FDRuntime)(nil).DeletePids([]uint32{1, 2}) }},
			},
			Snapshot:     func() error { _, err := (*libbpfloader.FDRuntime)(nil).Snapshot(true); return err },
			SnapshotApps: func() error { _, err := (*libbpfloader.FDRuntime)(nil).SnapshotApps(true); return err },
			Close:        func() { (*libbpfloader.FDRuntime)(nil).Close() },
		})
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
