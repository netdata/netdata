package main

import (
	"path/filepath"
	"testing"
)

func TestBuildDNSLegacyPlan(t *testing.T) {
	tests := map[string]struct {
		cfg      DNSLegacyConfig
		want     string
		wantMode LoadMethod
	}{
		"base-on-old-kernel": {
			cfg: DNSLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dnsKernelMask,
				IsRHF:         -1,
				KernelVersion: 328704, // 5.4
				IsDebian:      false,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_dns.5.4.o",
			wantMode: LoadCore,
		},
		"buffer-on-5-10": {
			// 5.10 uses the matching buffer artifact family.
			cfg: DNSLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dnsKernelMask,
				IsRHF:         -1,
				KernelVersion: 330240, // 5.10 = 5*65536 + 10*256
				IsDebian:      false,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_dns_buffer.5.10.o",
			wantMode: LoadCore,
		},
		"arena-on-6-12": {
			cfg: DNSLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dnsKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288, // 6.12
				IsDebian:      false,
				ObjectFlavor:  "arena",
			},
			want:     "pnetdata_ebpf_dns_arena.6.12.o",
			wantMode: LoadCore,
		},
		"arena-on-debian-falls-back-to-base": {
			// Arena is blocked on Debian, so choose the generic 6.12 base object.
			cfg: DNSLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dnsKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288, // 6.12
				IsDebian:      true,
				ObjectFlavor:  "arena",
			},
			want:     "pnetdata_ebpf_dns.6.12.o",
			wantMode: LoadCore,
		},
		"debian-explicit-buffer": {
			cfg: DNSLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dnsKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288, // 6.12
				IsDebian:      true,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_dns_buffer.6.12.o",
			wantMode: LoadCore,
		},
		"buffer-on-old-kernel-falls-back-to-base": {
			cfg: DNSLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       dnsKernelMask,
				IsRHF:         -1,
				KernelVersion: 328704, // 5.4 — below minimumKernelVersionBuffer (5.10)
				IsDebian:      false,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_dns.5.4.o",
			wantMode: LoadCore,
		},
		"generic-7-buffer-uses-matched-flavor": {
			cfg:  DNSLegacyConfig{PluginsDir: defaultPluginsDir(), Kernels: dnsKernelMask, IsRHF: -1, KernelVersion: minimumKernelVersionSeven, ObjectFlavor: "buffer"},
			want: "pnetdata_ebpf_dns_buffer.7.0.o", wantMode: LoadCore,
		},
		"generic-7-arena-uses-matched-flavor": {
			cfg:  DNSLegacyConfig{PluginsDir: defaultPluginsDir(), Kernels: dnsKernelMask, IsRHF: -1, KernelVersion: minimumKernelVersionSeven, ObjectFlavor: "arena"},
			want: "pnetdata_ebpf_dns_arena.7.0.o", wantMode: LoadCore,
		},
		"generic-7-base-walks-down-to-6-12": {
			cfg:  DNSLegacyConfig{PluginsDir: defaultPluginsDir(), Kernels: dnsKernelMask, IsRHF: -1, KernelVersion: minimumKernelVersionSeven, ObjectFlavor: "tracing"},
			want: "pnetdata_ebpf_dns.6.12.o", wantMode: LoadCore,
		},
		"generic-6-16-2-buffer-uses-matched-flavor": {
			cfg:  DNSLegacyConfig{PluginsDir: defaultPluginsDir(), Kernels: dnsKernelMask, IsRHF: -1, KernelVersion: exactKernelVersion6162, ObjectFlavor: "buffer"},
			want: "pnetdata_ebpf_dns_buffer.6.16.2.o", wantMode: LoadCore,
		},
		"generic-6-16-2-arena-uses-matched-flavor": {
			cfg:  DNSLegacyConfig{PluginsDir: defaultPluginsDir(), Kernels: dnsKernelMask, IsRHF: -1, KernelVersion: exactKernelVersion6162, ObjectFlavor: "arena"},
			want: "pnetdata_ebpf_dns_arena.6.16.2.o", wantMode: LoadCore,
		},
		"generic-6-16-2-base-uses-6-12-family": {
			cfg:  DNSLegacyConfig{PluginsDir: defaultPluginsDir(), Kernels: dnsKernelMask, IsRHF: -1, KernelVersion: exactKernelVersion6162, ObjectFlavor: "tracing"},
			want: "pnetdata_ebpf_dns.6.12.o", wantMode: LoadCore,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := BuildDNSLegacyPlan(tc.cfg)
			want := filepath.Join(defaultPluginsDir(), "ebpf.d", tc.want)
			if got.ObjectPath != want {
				t.Fatalf("BuildDNSLegacyPlan() = %q, want %q", got.ObjectPath, want)
			}
			if got.LoadMode != tc.wantMode {
				t.Fatalf("BuildDNSLegacyPlan().LoadMode = %v, want %v", got.LoadMode, tc.wantMode)
			}
		})
	}
}
