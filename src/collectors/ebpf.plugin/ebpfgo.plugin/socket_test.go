package main

import (
	"path/filepath"
	"testing"
)

func TestBuildSocketLegacyPlan(t *testing.T) {
	tests := map[string]struct {
		cfg      SocketLegacyConfig
		want     string
		wantMode LoadMethod
	}{
		"legacy-base": {
			cfg: SocketLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       socketKernelMask,
				IsRHF:         -1,
				KernelVersion: 328704,
				IsDebian:      false,
				HasBTF:        false,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_socket.5.4.o",
			wantMode: LoadLegacy,
		},
		"buffer-on-6-8": {
			cfg: SocketLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       socketKernelMask,
				IsRHF:         -1,
				KernelVersion: 395264,
				IsDebian:      false,
				HasBTF:        true,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_socket_buffer.6.8.o",
			wantMode: LoadCore,
		},
		"arena-on-6-12": {
			cfg: SocketLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       socketKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      false,
				HasBTF:        true,
				ObjectFlavor:  "arena",
			},
			want:     "pnetdata_ebpf_socket_arena.6.12.o",
			wantMode: LoadCore,
		},
		"debian-forces-buffer": {
			cfg: SocketLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       socketKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      true,
				HasBTF:        true,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_socket_buffer.6.12.o",
			wantMode: LoadCore,
		},
		"buffer-falls-back-to-base": {
			cfg: SocketLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       socketKernelMask,
				IsRHF:         -1,
				KernelVersion: 328704,
				IsDebian:      false,
				HasBTF:        false,
				ObjectFlavor:  "buffer",
			},
			want:     "pnetdata_ebpf_socket.5.4.o",
			wantMode: LoadLegacy,
		},
		"arena-on-debian-falls-back-to-base": {
			cfg: SocketLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       socketKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      true,
				HasBTF:        true,
				ObjectFlavor:  "arena",
			},
			want:     "pnetdata_ebpf_socket.6.12.o",
			wantMode: LoadCore,
		},
		"tracing-explicit": {
			cfg: SocketLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       socketKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      false,
				HasBTF:        true,
				ObjectFlavor:  "tracing",
			},
			want:     "pnetdata_ebpf_socket.6.12.o",
			wantMode: LoadCore,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := BuildSocketLegacyPlan(tc.cfg)
			want := filepath.Join(defaultPluginsDir(), "ebpf.d", tc.want)
			if got.ObjectPath != want {
				t.Fatalf("BuildSocketLegacyPlan() = %q, want %q", got.ObjectPath, want)
			}
			if got.LoadMode != tc.wantMode {
				t.Fatalf("BuildSocketLegacyPlan().LoadMode = %v, want %v", got.LoadMode, tc.wantMode)
			}
		})
	}
}
