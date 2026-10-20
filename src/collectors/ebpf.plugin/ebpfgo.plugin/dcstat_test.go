package main

import (
	"path/filepath"
	"testing"
)

func TestBuildDCStatLegacyPlan(t *testing.T) {
	tests := map[string]struct {
		cfg      DCStatLegacyConfig
		want     string
		wantMode LoadMethod
	}{
		"legacy-base": {
			cfg: DCStatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dcstatKernelMask,
				IsRHF:         -1,
				KernelVersion: 328704,
				IsDebian:      false,
				HasBTF:        false,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_dc.5.4.o",
			wantMode: LoadLegacy,
		},
		"buffer-on-6-8": {
			cfg: DCStatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dcstatKernelMask,
				IsRHF:         -1,
				KernelVersion: 395264,
				IsDebian:      false,
				HasBTF:        true,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_dc_buffer.6.8.o",
			wantMode: LoadCore,
		},
		"arena-on-6-12": {
			cfg: DCStatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dcstatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      false,
				HasBTF:        true,
				ObjectFlavor:  "arena",
			},
			want:     "pnetdata_ebpf_dc_arena.6.12.o",
			wantMode: LoadCore,
		},
		"debian-forces-buffer": {
			cfg: DCStatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dcstatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      true,
				HasBTF:        true,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_dc_buffer.6.12.o",
			wantMode: LoadCore,
		},
		"arena-on-debian-falls-back-to-tracing": {
			// Arena is blocked on Debian; the base flavor is chosen and the
			// selector must be capped to dcstatMaxBaseSelector (7 = 5.14),
			// because no base dc object exists beyond 5.14.
			cfg: DCStatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dcstatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      true,
				HasBTF:        true,
				ObjectFlavor:  "arena",
			},
			want:     "pnetdata_ebpf_dc.5.14.o",
			wantMode: LoadCore,
		},
		"tracing-explicit": {
			// "tracing" (= base flavor) on kernel 6.12 must fall back to the
			// highest existing base object (5.14, selector 7) rather than a
			// non-existent pnetdata_ebpf_dc.6.12.o.
			cfg: DCStatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dcstatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      false,
				HasBTF:        true,
				ObjectFlavor:  "tracing",
			},
			want:     "pnetdata_ebpf_dc.5.14.o",
			wantMode: LoadCore,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := BuildDCStatLegacyPlan(tc.cfg)
			want := filepath.Join(defaultPluginsDir(), "ebpf.d", tc.want)
			if got.ObjectPath != want {
				t.Fatalf("BuildDCStatLegacyPlan() = %q, want %q", got.ObjectPath, want)
			}
			if got.LoadMode != tc.wantMode {
				t.Fatalf("BuildDCStatLegacyPlan().LoadMode = %v, want %v", got.LoadMode, tc.wantMode)
			}
		})
	}
}

// TestBuildDCStatFallbackPlansCapBaseSelector guards the one dcstat-specific
// difference from cachestat: the base-flavor object family stops at 5.14, so a
// buffer-flavor primary on a newer kernel must degrade to dc.5.14.o.
func TestBuildDCStatFallbackPlansCapBaseSelector(t *testing.T) {
	cfg := DCStatLegacyConfig{
		PluginsDir:    defaultPluginsDir(),
		Kernels:       dcstatKernelMask,
		IsRHF:         -1,
		KernelVersion: 396288, // 6.12
		HasBTF:        true,
		ObjectFlavor:  "arena",
	}

	plans := buildFallbackPlans(BuildDCStatLegacyPlan(cfg), cfg.PluginsDir, cfg.IsRHF, "dc", dcstatMaxBaseSelector)
	if len(plans) != 3 {
		t.Fatalf("buildFallbackPlans() returned %d plans, want 3 (arena, buffer, base)", len(plans))
	}

	want := []string{
		"pnetdata_ebpf_dc_arena.6.12.o",
		"pnetdata_ebpf_dc_buffer.6.12.o",
		"pnetdata_ebpf_dc.5.14.o",
	}
	for i, base := range want {
		if got := filepath.Base(plans[i].ObjectPath); got != base {
			t.Fatalf("plans[%d] = %q, want %q", i, got, base)
		}
	}
}
