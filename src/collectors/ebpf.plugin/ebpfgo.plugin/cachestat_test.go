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
			// Arena is blocked on Debian; base flavor is chosen.  The selector must be
			// capped to cachestatMaxBaseSelector (9 = 5.16) because no base object
			// exists for kernels beyond 5.16.
			cfg: CachestatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      true,
				HasBTF:        true,
				ObjectFlavor:  "arena",
			},
			want:     "pnetdata_ebpf_cachestat.5.16.o",
			wantMode: LoadCore,
		},
		"tracing-explicit": {
			// "tracing" (= legacy / base flavor) on kernel 6.12 must fall back to the
			// highest existing base object (5.16, selector 9) rather than a non-existent
			// pnetdata_ebpf_cachestat.6.12.o.
			cfg: CachestatLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      false,
				HasBTF:        true,
				ObjectFlavor:  "tracing",
			},
			want:     "pnetdata_ebpf_cachestat.5.16.o",
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
