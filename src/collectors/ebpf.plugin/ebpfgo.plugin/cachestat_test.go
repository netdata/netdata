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
			},
			want:     "pnetdata_ebpf_cachestat_buffer.6.12.o",
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
