package main

import (
	"path/filepath"
	"testing"
)

func TestSocketDelta(t *testing.T) {
	tests := map[string]struct {
		current, prev uint64
		want          uint64
	}{
		"normal increment": {current: 100, prev: 60, want: 40},
		"no change":        {current: 50, prev: 50, want: 0},
		"counter reset":    {current: 10, prev: 500, want: 0}, // must not return 490
		"counter wrap":     {current: 1, prev: ^uint64(0), want: 0},
		// prev=0 means the counter genuinely started at zero (eBPF maps clear
		// on load); spike suppression is handled by socketGlobalState.Update's
		// !initialized gate, not here.
		"first read (prev=0)": {current: 1000, prev: 0, want: 1000},
		"both zero":           {current: 0, prev: 0, want: 0},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := socketDelta(tc.current, tc.prev); got != tc.want {
				t.Fatalf("socketDelta(%d, %d) = %d, want %d", tc.current, tc.prev, got, tc.want)
			}
		})
	}
}

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
			// Arena is blocked on Debian; base flavor is chosen.  The selector must be
			// capped to socketMaxBaseSelector (7 = 5.14) because no base object exists
			// for kernels beyond 5.14.
			cfg: SocketLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       socketKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      true,
				HasBTF:        true,
				ObjectFlavor:  "arena",
			},
			want:     "pnetdata_ebpf_socket.5.14.o",
			wantMode: LoadCore,
		},
		"tracing-explicit": {
			// "tracing" (= legacy / base flavor) on kernel 6.12 must fall back to the
			// highest existing base object (5.14, selector 7) rather than a non-existent
			// pnetdata_ebpf_socket.6.12.o.
			cfg: SocketLegacyConfig{
				PluginsDir:    defaultPluginsDir(),
				Kernels:       socketKernelMask,
				IsRHF:         -1,
				KernelVersion: 396288,
				IsDebian:      false,
				HasBTF:        true,
				ObjectFlavor:  "tracing",
			},
			want:     "pnetdata_ebpf_socket.5.14.o",
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
