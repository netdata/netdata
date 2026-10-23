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
			// Arena is blocked on Debian; this exact generic 6.12 stream has a
			// base object, so the plan uses the matching 6.12 artifact.
			cfg: DCStatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dcstatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      true,
				HasBTF:        true,
				ObjectFlavor:  "arena",
			},
			want:     "pnetdata_ebpf_dc.6.12.o",
			wantMode: LoadCore,
		},
		"tracing-explicit": {
			// "tracing" (= base flavor) on kernel 6.12 must fall back to the
			// generic 6.12 object rather than a stale earlier kernel layout.
			cfg: DCStatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dcstatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      false,
				HasBTF:        true,
				ObjectFlavor:  "tracing",
			},
			want:     "pnetdata_ebpf_dc.6.12.o",
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

func TestDCStat6162PlansUseShippedFlavors(t *testing.T) {
	tests := []struct {
		requested string
		want      string
	}{
		{requested: "buffer", want: "pnetdata_ebpf_dc_buffer.6.16.2.o"},
		{requested: "arena", want: "pnetdata_ebpf_dc_arena.6.16.2.o"},
		{requested: "tracing", want: "pnetdata_ebpf_dc.6.12.o"},
	}

	for _, tc := range tests {
		cfg := DCStatLegacyConfig{
			PluginsDir:    defaultPluginsDir(),
			Kernels:       dcstatKernelMask,
			IsRHF:         -1,
			KernelVersion: exactKernelVersion6162,
			HasBTF:        false,
			ObjectFlavor:  tc.requested,
		}
		got := BuildDCStatLegacyPlan(cfg)
		if path := filepath.Base(got.ObjectPath); path != tc.want {
			t.Errorf("BuildDCStatLegacyPlan(%q) = %q, want %q", tc.requested, path, tc.want)
		}
	}
}

// TestBuildDCStatFallbackPlansCapBaseSelector verifies that 6.16.2 flavored
// artifacts are selected while base fallback uses the producer-backed 6.12 family.
func TestBuildDCStatFallbackPlansCapBaseSelector(t *testing.T) {
	cfg := DCStatLegacyConfig{
		PluginsDir:    defaultPluginsDir(),
		Kernels:       dcstatKernelMask,
		IsRHF:         -1,
		KernelVersion: exactKernelVersion6162,
		HasBTF:        true,
		ObjectFlavor:  "arena",
	}

	plans := buildFallbackPlans(BuildDCStatLegacyPlan(cfg), cfg.PluginsDir, cfg.IsRHF, "dc", dcstatMaxBaseSelector)
	if len(plans) != 3 {
		t.Fatalf("buildFallbackPlans() returned %d plans, want 3 (arena, buffer, base)", len(plans))
	}

	want := []string{
		"pnetdata_ebpf_dc_arena.6.16.2.o",
		"pnetdata_ebpf_dc_buffer.6.16.2.o",
		"pnetdata_ebpf_dc.6.12.o",
	}
	for i, base := range want {
		if got := filepath.Base(plans[i].ObjectPath); got != base {
			t.Fatalf("plans[%d] = %q, want %q", i, got, base)
		}
	}
}
