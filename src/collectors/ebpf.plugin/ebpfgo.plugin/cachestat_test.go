package main

import "testing"

func TestBuildCachestatLegacyPlan(t *testing.T) {
	tests := map[string]struct {
		cfg  CachestatLegacyConfig
		want string
	}{
		"legacy-base": {
			cfg: CachestatLegacyConfig{
				PluginsDir:    "/opt/netdata/plugins.d",
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 328704,
				IsDebian:      false,
			},
			want: "/opt/netdata/plugins.d/ebpf.d/pnetdata_ebpf_cachestat.5.4.o",
		},
		"buffer-on-6-8": {
			cfg: CachestatLegacyConfig{
				PluginsDir:    "/opt/netdata/plugins.d",
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 395264,
				IsDebian:      false,
			},
			want: "/opt/netdata/plugins.d/ebpf.d/pnetdata_ebpf_cachestat_buffer.6.8.o",
		},
		"arena-on-6-12": {
			cfg: CachestatLegacyConfig{
				PluginsDir:    "/opt/netdata/plugins.d",
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      false,
			},
			want: "/opt/netdata/plugins.d/ebpf.d/pnetdata_ebpf_cachestat_arena.6.12.o",
		},
		"debian-forces-buffer": {
			cfg: CachestatLegacyConfig{
				PluginsDir:    "/opt/netdata/plugins.d",
				Kernels:       cachestatKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      true,
			},
			want: "/opt/netdata/plugins.d/ebpf.d/pnetdata_ebpf_cachestat_buffer.6.12.o",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := BuildCachestatLegacyPlan(tc.cfg)
			if got.ObjectPath != tc.want {
				t.Fatalf("BuildCachestatLegacyPlan() = %q, want %q", got.ObjectPath, tc.want)
			}
		})
	}
}
