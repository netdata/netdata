package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// baseFDConfig is the shared starting point for the plan tests: only the fields
// each case varies are overridden.
func baseFDConfig() FDLegacyConfig {
	return FDLegacyConfig{
		PluginsDir: defaultPluginsDir(),
		Kernels:    fdKernelMask,
		IsRHF:      -1,
	}
}

func TestBuildFDLegacyPlan(t *testing.T) {
	tests := map[string]struct {
		kernelVersion uint32
		isDebian      bool
		hasBTF        bool
		objectFlavor  string
		want          string
		wantMode      LoadMethod
	}{
		"legacy-base": {
			kernelVersion: 328704, // 5.4
			objectFlavor:  "buffer",
			want:          "rnetdata_ebpf_fd.5.4.o",
			wantMode:      LoadLegacy,
		},
		"buffer-on-6-8": {
			kernelVersion: 395264, // 6.8
			hasBTF:        true,
			objectFlavor:  "buffer",
			want:          "rnetdata_ebpf_fd_buffer.6.8.o",
			wantMode:      LoadCore,
		},
		"arena-on-6-12": {
			kernelVersion: 396288, // 6.12
			hasBTF:        true,
			objectFlavor:  "arena",
			want:          "rnetdata_ebpf_fd_arena.6.12.o",
			wantMode:      LoadCore,
		},
		"debian-forces-buffer": {
			kernelVersion: 396288,
			isDebian:      true,
			hasBTF:        true,
			objectFlavor:  "buffer",
			want:          "rnetdata_ebpf_fd_buffer.6.12.o",
			wantMode:      LoadCore,
		},
		"arena-on-debian-falls-back-to-tracing": {
			// Arena is blocked on Debian; the base flavor is chosen and the
			// selector must be capped to fdMaxBaseSelector (7 = 5.14), because no
			// base fd object exists beyond 5.14.
			kernelVersion: 396288,
			isDebian:      true,
			hasBTF:        true,
			objectFlavor:  "arena",
			want:          "rnetdata_ebpf_fd.5.14.o",
			wantMode:      LoadCore,
		},
		"tracing-explicit": {
			kernelVersion: 396288,
			hasBTF:        true,
			objectFlavor:  "tracing",
			want:          "rnetdata_ebpf_fd.5.14.o",
			wantMode:      LoadCore,
		},
		// fd ships no base object for "5.10".  A 5.10 host asking for the base
		// flavor must therefore not land on rnetdata_ebpf_fd.5.10.o; the selector
		// resolves to 5.11, which does exist.
		"kernel-5-10-avoids-the-missing-base-object": {
			kernelVersion: 330240, // 5.10
			objectFlavor:  "tracing",
			want:          "rnetdata_ebpf_fd.5.11.o",
			wantMode:      LoadLegacy,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := baseFDConfig()
			cfg.KernelVersion = tc.kernelVersion
			cfg.IsDebian = tc.isDebian
			cfg.HasBTF = tc.hasBTF
			cfg.ObjectFlavor = tc.objectFlavor

			got := BuildFDLegacyPlan(cfg)
			want := filepath.Join(defaultPluginsDir(), "ebpf.d", tc.want)
			if got.ObjectPath != want {
				t.Fatalf("BuildFDLegacyPlan() = %q, want %q", got.ObjectPath, want)
			}
			if got.LoadMode != tc.wantMode {
				t.Fatalf("BuildFDLegacyPlan().LoadMode = %v, want %v", got.LoadMode, tc.wantMode)
			}
		})
	}
}

func TestFDCustomBTFPath(t *testing.T) {
	if got, want := fdCustomBTFPath("/custom/btf"), "/custom/btf/vmlinux"; got != want {
		t.Fatalf("fdCustomBTFPath() = %q, want %q", got, want)
	}
}

// TestBuildFDLegacyPlanAlwaysSelectsReturnObjects pins the D1 decision: fd loads
// the 'r'-prefixed object family in BOTH modes, because those are the objects
// whose programs read PT_REGS_RC.  `ebpf load mode` gates the error charts only.
func TestBuildFDLegacyPlanAlwaysSelectsReturnObjects(t *testing.T) {
	for _, reportErrors := range []bool{false, true} {
		cfg := baseFDConfig()
		cfg.KernelVersion = 395264 // 6.8
		cfg.HasBTF = true
		cfg.ObjectFlavor = "buffer"
		cfg.ReportErrors = reportErrors

		plan := BuildFDLegacyPlan(cfg)
		if !plan.IsReturn {
			t.Fatalf("ReportErrors=%t: plan.IsReturn = false, want true", reportErrors)
		}
		if got := filepath.Base(plan.ObjectPath); !strings.HasPrefix(got, "r") {
			t.Fatalf("ReportErrors=%t: object %q does not use the return-probe prefix", reportErrors, got)
		}
	}
}

func TestBuildFDLegacyPlanForcedLegacyUsesBaseObject(t *testing.T) {
	cfg := baseFDConfig()
	cfg.KernelVersion = 395264 // 6.8
	cfg.HasBTF = true
	cfg.ObjectFlavor = "arena"
	cfg.LoadMethod = LoadLegacy

	plan := BuildFDLegacyPlan(cfg)
	if plan.LoadMode != LoadLegacy {
		t.Fatalf("forced legacy LoadMode = %v, want %v", plan.LoadMode, LoadLegacy)
	}
	if plan.Flavor != ObjectFlavorBase {
		t.Fatalf("forced legacy Flavor = %q, want base", plan.Flavor)
	}
	if got, want := filepath.Base(plan.ObjectPath), "rnetdata_ebpf_fd.5.14.o"; got != want {
		t.Fatalf("forced legacy object = %q, want %q", got, want)
	}
}

// TestBuildFDFallbackPlansCapBaseSelector guards two things at once: the base
// object family stops at 5.14, and the whole fallback chain stays in the
// return-probe ('r') family.
func TestBuildFDFallbackPlansCapBaseSelector(t *testing.T) {
	cfg := baseFDConfig()
	cfg.KernelVersion = 396288 // 6.12
	cfg.HasBTF = true
	cfg.ObjectFlavor = "arena"

	plans := buildFallbackPlans(BuildFDLegacyPlan(cfg), cfg.PluginsDir, cfg.IsRHF, "fd", fdMaxBaseSelector)
	if len(plans) != 3 {
		t.Fatalf("buildFallbackPlans() returned %d plans, want 3 (arena, buffer, base)", len(plans))
	}

	want := []string{
		"rnetdata_ebpf_fd_arena.6.12.o",
		"rnetdata_ebpf_fd_buffer.6.12.o",
		"rnetdata_ebpf_fd.5.14.o",
	}
	for i, base := range want {
		if got := filepath.Base(plans[i].ObjectPath); got != base {
			t.Fatalf("plans[%d] = %q, want %q", i, got, base)
		}
	}
}

// TestBuildFDFallbackPlansRHFKeepsReturnPrefix covers the RHF branch of
// buildFallbackPlans, which builds a second path per flavor without the .rhf
// suffix.  Those generic paths are constructed separately, so they are the ones
// that would silently drop the 'r' prefix if the plan field were ignored.
func TestBuildFDFallbackPlansRHFKeepsReturnPrefix(t *testing.T) {
	cfg := baseFDConfig()
	cfg.IsRHF = 9*256 + 0
	cfg.KernelVersion = 331264 // 5.15 — lowest kernel at which SelectMaxIndex reaches 5.14 on RHF
	cfg.HasBTF = true
	cfg.ObjectFlavor = "buffer"

	plans := buildFallbackPlans(BuildFDLegacyPlan(cfg), cfg.PluginsDir, cfg.IsRHF, "fd", fdMaxBaseSelector)
	if len(plans) == 0 {
		t.Fatal("buildFallbackPlans() returned no plans")
	}

	want := []string{
		"rnetdata_ebpf_fd_buffer.5.14.rhf.o",
		"rnetdata_ebpf_fd_buffer.5.14.o",
		"rnetdata_ebpf_fd.5.14.rhf.o",
		"rnetdata_ebpf_fd.5.14.o",
	}
	if len(plans) != len(want) {
		t.Fatalf("buildFallbackPlans() returned %d plans, want %d: %+v", len(plans), len(want), plans)
	}
	for i, base := range want {
		if got := filepath.Base(plans[i].ObjectPath); got != base {
			t.Fatalf("plans[%d] = %q, want %q", i, got, base)
		}
	}
}

// TestSelectMaxIndexNeverReturnsTheMissingFDBaseSelector guards the assumption
// fdMaxBaseSelector rests on: fd ships no base object for selector 5 ("5.10"),
// and no cap protects against it, because SelectMaxIndex jumps from 4 to 6 at
// that kernel boundary.  If that ladder ever grows a 5.10 rung, fd needs an
// explicit exclusion.
func TestSelectMaxIndexNeverReturnsTheMissingFDBaseSelector(t *testing.T) {
	kernels := []uint32{
		328448, // 5.2
		328704, // 5.4
		330239, // just below 5.10
		330240, // 5.10
		330496, // 5.14 numeric boundary used by the ladder
		395264, // 6.8
		396288, // 6.12
	}

	for _, kver := range kernels {
		for _, isRHF := range []int{-1, 8 * 256, 9 * 256} {
			if got := SelectMaxIndex(isRHF, kver); got == 5 {
				t.Fatalf("SelectMaxIndex(isRHF=%d, kver=%d) = 5 (\"5.10\"); fd ships no base object for it",
					isRHF, kver)
			}
		}
	}
}
