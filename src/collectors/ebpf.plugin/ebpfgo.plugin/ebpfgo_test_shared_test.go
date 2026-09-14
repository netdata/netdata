package main

import (
	"errors"
	"testing"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// mergedCommonExpect carries the post-merge values the four shared
// `[global]` keys produce on every collector.  Fields use concrete types so a
// test failure message is readable; CollectPidLevel is -1 to mean "this
// collector does not consume collect pid (cachestat)".
type mergedCommonExpect struct {
	UpdateEvery     int
	PidTable        uint32
	MapsPerCore     bool
	BTFPath         string
	ObjectFlavor    string
	CollectPidLevel int // -1 to skip
}

// assertMergedCommonConfig walks the merged pluginConfigFile fields every
// collector exercises after a four-file merge and compares them against the
// expected values.  Cachestat does not set CollectPidLevel; fd and dcstat do.
// Pass CollectPidLevel: -1 to skip the collect-pid assertion.
func assertMergedCommonConfig(t *testing.T, cfg pluginConfigFile, want mergedCommonExpect) {
	t.Helper()

	if cfg.UpdateEvery == nil || *cfg.UpdateEvery != want.UpdateEvery {
		t.Fatalf("UpdateEvery = %#v, want %d", cfg.UpdateEvery, want.UpdateEvery)
	}
	if cfg.PidTable == nil || *cfg.PidTable != want.PidTable {
		t.Fatalf("PidTable = %#v, want %d", cfg.PidTable, want.PidTable)
	}
	if cfg.MapsPerCore == nil || *cfg.MapsPerCore != want.MapsPerCore {
		t.Fatalf("MapsPerCore = %#v, want %v", cfg.MapsPerCore, want.MapsPerCore)
	}
	if cfg.BTFPath == nil || *cfg.BTFPath != want.BTFPath {
		t.Fatalf("BTFPath = %#v, want %q", cfg.BTFPath, want.BTFPath)
	}
	if cfg.ObjectFlavor == nil || *cfg.ObjectFlavor != want.ObjectFlavor {
		t.Fatalf("ObjectFlavor = %#v, want %q", cfg.ObjectFlavor, want.ObjectFlavor)
	}
	if want.CollectPidLevel >= 0 {
		if cfg.CollectPidLevel == nil || *cfg.CollectPidLevel != want.CollectPidLevel {
			t.Fatalf("CollectPidLevel = %#v, want %d", cfg.CollectPidLevel, want.CollectPidLevel)
		}
	}
}

// runtimeDisabledOps is the bag of closures a collector's "disabled libbpf
// build" test wires into the shared driver.  Per-runtime method signatures
// differ (cachestat's Prepare/Attach carry extra parameters; fd/dcstat's
// Attach takes two targets, cachestat's takes one), so the caller supplies
// the closures that already match its type.  The driver only knows the
// operation must return libbpfloader.ErrDisabled.
type runtimeDisabledOps struct {
	// Mutators is the six mutating entry points the fd and dcstat runtimes
	// share (Prepare, Load, Attach, UpdateController, DeletePid, DeletePids).
	// The slice is intentionally ordered to mirror the per-collector test
	// layout that existed before this helper.
	Mutators []struct {
		Name string
		Call func() error
	}
	Snapshot     func() error
	SnapshotApps func() error
	Close        func()
}

// assertRuntimeDisabledStubs exercises every disabled-build runtime entry
// point the collector exposes through the libbpfloader surface and confirms
// each returns ErrDisabled. Cachestat has a different method shape, so it uses
// a separate hand-rolled test instead of going through this helper.
//
// T is parameterised over the runtime type so the disabled-build New*
// constructor's typed-nil return compares as a true nil pointer (an `any`
// interface wrapping a typed nil would compare non-nil here).
func assertRuntimeDisabledStubs[T any](t *testing.T, module string, supportsCore func() bool, newRT func() (*T, error), ops runtimeDisabledOps) {
	t.Helper()

	if supportsCore() {
		t.Fatalf("%s: SupportsCore() = true, want false without libbpf", module)
	}

	rt, err := newRT()
	if rt != nil || !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("%s: New() = (%v, %v), want (nil, ErrDisabled)", module, rt, err)
	}

	for _, e := range ops.Mutators {
		if err := e.Call(); !errors.Is(err, libbpfloader.ErrDisabled) {
			t.Fatalf("%s.%s() = %v, want ErrDisabled", module, e.Name, err)
		}
	}

	if err := ops.Snapshot(); !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("%s.Snapshot() = %v, want ErrDisabled", module, err)
	}
	if err := ops.SnapshotApps(); !errors.Is(err, libbpfloader.ErrDisabled) {
		t.Fatalf("%s.SnapshotApps() = %v, want ErrDisabled", module, err)
	}

	// Close must be safe on a nil runtime (the collector always defers it).
	ops.Close()
}
