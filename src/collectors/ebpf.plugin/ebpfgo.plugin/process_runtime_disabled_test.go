//go:build !netdata_ebpf_libbpf

package main

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// TestProcessRuntimeDisabledBuild verifies every process runtime entry point
// returns the disabled sentinel when the plugin is built without libbpf, so the
// collector degrades instead of panicking on a nil runtime.
func TestProcessRuntimeDisabledBuild(t *testing.T) {
	assertRuntimeDisabledStubs(t, "ProcessRuntime",
		libbpfloader.ProcessRuntimeSupportsCore,
		func() (*libbpfloader.ProcessRuntime, error) {
			return libbpfloader.ProcessRuntimeOpenMode("/nonexistent/process.o", true, "/custom/btf/vmlinux")
		},
		runtimeDisabledOps{
			Mutators: []struct {
				Name string
				Call func() error
			}{
				{"Prepare", func() error { return (*libbpfloader.ProcessRuntime)(nil).Prepare(1024, true) }},
				{"Load", func() error { return (*libbpfloader.ProcessRuntime)(nil).Load() }},
				{"Attach", func() error { return (*libbpfloader.ProcessRuntime)(nil).Attach() }},
				{"UpdateController", func() error { return (*libbpfloader.ProcessRuntime)(nil).UpdateController(true, 0) }},
				{"DeletePids", func() error { return (*libbpfloader.ProcessRuntime)(nil).DeletePids([]uint32{1, 2}) }},
			},
			Snapshot:     func() error { _, err := (*libbpfloader.ProcessRuntime)(nil).Snapshot(true); return err },
			SnapshotApps: func() error { _, err := (*libbpfloader.ProcessRuntime)(nil).SnapshotApps(true); return err },
			Close:        func() { (*libbpfloader.ProcessRuntime)(nil).Close() },
		})
}

// TestLoadProcessLegacyDisabledBuild verifies the plugin-level loader reports
// the disabled sentinel, like every sibling module, rather than a bespoke error.
func TestLoadProcessLegacyDisabledBuild(t *testing.T) {
	handle, err := LoadProcessLegacy(defaultProcessLegacyConfig())
	if handle != nil || !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("LoadProcessLegacy() = (%v, %v), want (nil, ErrDisabled)", handle, err)
	}
}
