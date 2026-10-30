package main

import (
	"path/filepath"
	"testing"
)

func TestBuildCachestatLegacyPlan(t *testing.T) {
	tests := map[string]struct {
		cfg      CachestatLegacyConfig
		want     string
		wantMode LoadMethod
	}{
		"legacy-base": {
			cfg: CachestatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 328704,
				IsDebian:      false,
				HasBTF:        false,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_cachestat.5.4.o",
			wantMode: LoadLegacy,
		},
		"buffer-on-6-8": {
			cfg: CachestatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 395264,
				IsDebian:      false,
				HasBTF:        true,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_cachestat_buffer.6.8.o",
			wantMode: LoadCore,
		},
		"arena-on-6-12": {
			cfg: CachestatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      false,
				HasBTF:        true,
				ObjectFlavor:  "arena",
			},
			want:     "pnetdata_ebpf_cachestat_arena.6.12.o",
			wantMode: LoadCore,
		},
		"debian-forces-buffer": {
			cfg: CachestatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      true,
				HasBTF:        true,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_cachestat_buffer.6.12.o",
			wantMode: LoadCore,
		},
		"buffer-falls-back-to-tracing": {
			cfg: CachestatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 328704,
				IsDebian:      false,
				HasBTF:        false,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_cachestat.5.4.o",
			wantMode: LoadLegacy,
		},
		"arena-on-debian-falls-back-to-tracing": {
			// Arena is blocked on Debian; the generic 6.12 build now provides a base
			// object for this exact kernel stream.
			cfg: CachestatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      true,
				HasBTF:        true,
				ObjectFlavor:  "arena",
			},
			want:     "pnetdata_ebpf_cachestat.6.12.o",
			wantMode: LoadCore,
		},
		"tracing-explicit": {
			// "tracing" (= legacy / base flavor) can use the generic 6.12 artifact.
			cfg: CachestatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      false,
				HasBTF:        true,
				ObjectFlavor:  "tracing",
			},
			want:     "pnetdata_ebpf_cachestat.6.12.o",
			wantMode: LoadCore,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := BuildCachestatLegacyPlan(tc.cfg)
			want := filepath.Join(defaultPluginsDir(), "ebpf.d", tc.want)
			if got.ObjectPath != want {
				t.Fatalf("BuildCachestatLegacyPlan() = %q, want %q", got.ObjectPath, want)
			}
			if got.LoadMode != tc.wantMode {
				t.Fatalf("BuildCachestatLegacyPlan().LoadMode = %v, want %v", got.LoadMode, tc.wantMode)
			}
		})
	}
}

func TestCachestat6162PlansUseShippedFlavors(t *testing.T) {
	tests := map[string]struct {
		requested string
		want      string
	}{
		"buffer uses exact rung":      {requested: "buffer", want: "pnetdata_ebpf_cachestat_buffer.6.16.2.o"},
		"arena uses exact rung":       {requested: "arena", want: "pnetdata_ebpf_cachestat_arena.6.16.2.o"},
		"tracing uses 6.12 base rung": {requested: "tracing", want: "pnetdata_ebpf_cachestat.6.12.o"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := CachestatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: exactKernelVersion6162,
				HasBTF:        false,
				ObjectFlavor:  tc.requested,
			}
			got := BuildCachestatLegacyPlan(cfg)
			if path := filepath.Base(got.ObjectPath); path != tc.want {
				t.Errorf("BuildCachestatLegacyPlan(%q) = %q, want %q", tc.requested, path, tc.want)
			}
		})
	}
}

func TestCachestatFallbackPlanOrderAcrossFlavors(t *testing.T) {
	cfg := CachestatLegacyConfig{
		PluginsDir:    defaultPluginsDir(),
		Kernels:       cachestatKernelMask,
		IsRHF:         -1,
		KernelVersion: exactKernelVersion6162,
		HasBTF:        false,
		ObjectFlavor:  "arena",
	}
	primary := BuildCachestatLegacyPlan(cfg)
	primary.LoadMode = LoadLegacy
	plans := buildFallbackPlans(primary, cfg.PluginsDir, cfg.IsRHF, "cachestat", cachestatMaxBaseSelector)
	want := []string{
		"pnetdata_ebpf_cachestat_arena.6.16.2.o",
		"pnetdata_ebpf_cachestat_buffer.6.16.2.o",
		"pnetdata_ebpf_cachestat.6.12.o",
	}
	if len(plans) != len(want) {
		t.Fatalf("buildFallbackPlans() returned %d plans, want %d: %+v", len(plans), len(want), plans)
	}
	for i := range want {
		if got := filepath.Base(plans[i].ObjectPath); got != want[i] {
			t.Errorf("plans[%d] = %q, want %q", i, got, want[i])
		}
		if plans[i].LoadMode != LoadLegacy {
			t.Errorf("plans[%d].LoadMode = %v, want LoadLegacy", i, plans[i].LoadMode)
		}
	}
}
