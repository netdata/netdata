package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestKernelVersionFromRelease(t *testing.T) {
	tests := map[string]struct {
		release string
		want    uint32
	}{
		"plain": {
			release: "6.18.26",
			want:    397850,
		},
		"suffix": {
			release: "5.14.0-362.13.1.el9_3.x86_64",
			want:    331264,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := KernelVersionFromRelease(tc.release)
			if err != nil {
				t.Fatalf("KernelVersionFromRelease(%q) returned error: %v", tc.release, err)
			}
			if got != tc.want {
				t.Fatalf("KernelVersionFromRelease(%q) = %d, want %d", tc.release, got, tc.want)
			}
		})
	}
}

func TestRedHatReleaseFromFile(t *testing.T) {
	tests := map[string]struct {
		contents string
		want     int
	}{
		"rhel8": {
			contents: "Red Hat Enterprise Linux release 8.6 (Ootpa)",
			want:     2054,
		},
		"rhel9": {
			contents: "Red Hat Enterprise Linux release 9.4 (Plow)",
			want:     2308,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := RedHatReleaseFromFile(tc.contents)
			if err != nil {
				t.Fatalf("RedHatReleaseFromFile(%q) returned error: %v", tc.contents, err)
			}
			if got != tc.want {
				t.Fatalf("RedHatReleaseFromFile(%q) = %d, want %d", tc.contents, got, tc.want)
			}
		})
	}
}

func TestDebianFlavorFromOSRelease(t *testing.T) {
	tests := map[string]struct {
		contents string
		want     bool
	}{
		"debian": {
			contents: "ID=debian\nNAME=\"Debian GNU/Linux\"\n",
			want:     true,
		},
		"ubuntu-like-not-debian": {
			contents: "ID=ubuntu\nID_LIKE=debian\nNAME=\"Ubuntu\"\n",
			want:     false,
		},
		"other-distro": {
			contents: "ID=fedora\nID_LIKE=\"rhel fedora\"\n",
			want:     false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := DebianFlavorFromOSRelease(tc.contents)
			if got != tc.want {
				t.Fatalf("DebianFlavorFromOSRelease() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSelectHelpers(t *testing.T) {
	tests := map[string]struct {
		kernels uint32
		isRHF   int
		kver    uint32
		wantIdx uint32
		wantObj string
	}{
		"kernel-org": {
			kernels: 1<<1 | 1<<2 | 1<<10,
			isRHF:   -1,
			kver:    397850,
			wantIdx: 10,
			wantObj: "pnetdata_ebpf_process.6.8.o",
		},
		"kernel-612": {
			kernels: 1 << 11,
			isRHF:   -1,
			kver:    397850,
			wantIdx: 11,
			wantObj: "pnetdata_ebpf_process.6.12.o",
		},
		"rhf": {
			kernels: 1<<3 | 1<<4 | 1<<7,
			isRHF:   1,
			kver:    331264,
			wantIdx: 7,
			wantObj: "pnetdata_ebpf_process.5.14.rhf.o",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotIdx := SelectIndex(tc.kernels, tc.isRHF, tc.kver)
			if gotIdx != tc.wantIdx {
				t.Fatalf("SelectIndex() = %d, want %d", gotIdx, tc.wantIdx)
			}

			got := BuildObjectPath(defaultPluginsDir(), gotIdx, "process", false, tc.isRHF)
			want := filepath.Join(defaultPluginsDir(), "ebpf.d", tc.wantObj)
			if got != want {
				t.Fatalf("BuildObjectPath() = %q, want %q", got, want)
			}
		})
	}
}

func TestSelectLoadAndAttachModes(t *testing.T) {
	tests := map[string]struct {
		hasBTF bool
		load   LoadMethod
		kver   uint32
		isRH   int
		mode   RunMode
		attach string
		wantL  LoadMethod
		wantA  ProgramMode
	}{
		"legacy-fallback": {
			hasBTF: false,
			load:   LoadPlayDice,
			kver:   397850,
			isRH:   0,
			mode:   RunModeEntry,
			attach: "probe",
			wantL:  LoadLegacy,
			wantA:  LoadProbe,
		},
		"core-trampoline": {
			hasBTF: true,
			load:   LoadCore,
			kver:   397850,
			isRH:   0,
			mode:   RunModeReturn,
			attach: "tracepoint",
			wantL:  LoadCore,
			wantA:  LoadTracepoint,
		},
		"rhf-oracle-legacy": {
			hasBTF: true,
			load:   LoadPlayDice,
			kver:   328800,
			isRH:   1,
			mode:   RunModeReturn,
			attach: "probe",
			wantL:  LoadLegacy,
			wantA:  LoadRetprobe,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := SelectLoadMode(tc.hasBTF, tc.load, tc.kver, tc.isRH); got != tc.wantL {
				t.Fatalf("SelectLoadMode() = %v, want %v", got, tc.wantL)
			}

			if got := ConvertCoreType(tc.attach, tc.mode); got != tc.wantA {
				t.Fatalf("ConvertCoreType() = %v, want %v", got, tc.wantA)
			}
		})
	}
}

func TestBuildLoadPlan(t *testing.T) {
	tests := map[string]struct {
		kernels          uint32
		isRHF            int
		kver             uint32
		name             string
		isReturn         bool
		hasResizableMaps bool
		isDebian         bool
		hasBTF           bool
		load             LoadMethod
		coreAttach       string
		mode             RunMode
		want             LoadPlan
	}{
		"core-mode": {
			kernels:          1<<1 | 1<<2 | 1<<10,
			isRHF:            -1,
			kver:             397850,
			name:             "process",
			isReturn:         false,
			hasResizableMaps: false,
			isDebian:         false,
			hasBTF:           true,
			load:             LoadCore,
			coreAttach:       "probe",
			mode:             RunModeEntry,
			want: LoadPlan{
				KernelVersion: 397850,
				IsRHF:         -1,
				Selector:      10,
				Flavor:        ObjectFlavorBase,
				ObjectPath:    "pnetdata_ebpf_process.6.8.o",
				LoadMode:      LoadCore,
				ProgramMode:   LoadProbe,
			},
		},
		"buffer-mode": {
			kernels:          1<<1 | 1<<2 | 1<<10,
			isRHF:            -1,
			kver:             395264,
			name:             "process",
			isReturn:         false,
			hasResizableMaps: true,
			isDebian:         false,
			hasBTF:           true,
			load:             LoadCore,
			coreAttach:       "probe",
			mode:             RunModeEntry,
			want: LoadPlan{
				KernelVersion: 395264,
				IsRHF:         -1,
				Selector:      10,
				Flavor:        ObjectFlavorBuffer,
				ObjectPath:    "pnetdata_ebpf_process_buffer.6.8.o",
				LoadMode:      LoadCore,
				ProgramMode:   LoadProbe,
			},
		},
		"arena-mode": {
			kernels:          1 << 11,
			isRHF:            -1,
			kver:             397850,
			name:             "process",
			isReturn:         false,
			hasResizableMaps: true,
			isDebian:         false,
			hasBTF:           true,
			load:             LoadCore,
			coreAttach:       "probe",
			mode:             RunModeEntry,
			want: LoadPlan{
				KernelVersion: 397850,
				IsRHF:         -1,
				Selector:      11,
				Flavor:        ObjectFlavorArena,
				ObjectPath:    "pnetdata_ebpf_process_arena.6.12.o",
				LoadMode:      LoadCore,
				ProgramMode:   LoadProbe,
			},
		},
		"debian-buffer-only": {
			kernels:          1 << 11,
			isRHF:            -1,
			kver:             397850,
			name:             "process",
			isReturn:         false,
			hasResizableMaps: true,
			isDebian:         true,
			hasBTF:           true,
			load:             LoadCore,
			coreAttach:       "probe",
			mode:             RunModeEntry,
			want: LoadPlan{
				KernelVersion: 397850,
				IsRHF:         -1,
				Selector:      11,
				Flavor:        ObjectFlavorBuffer,
				ObjectPath:    "pnetdata_ebpf_process_buffer.6.12.o",
				LoadMode:      LoadCore,
				ProgramMode:   LoadProbe,
			},
		},
		"legacy-fallback": {
			kernels:          1<<3 | 1<<4 | 1<<7,
			isRHF:            1,
			kver:             328800,
			name:             "process",
			isReturn:         true,
			hasResizableMaps: false,
			isDebian:         false,
			hasBTF:           false,
			load:             LoadPlayDice,
			coreAttach:       "tracepoint",
			mode:             RunModeReturn,
			want: LoadPlan{
				KernelVersion: 328800,
				IsRHF:         1,
				Selector:      4,
				Flavor:        ObjectFlavorBase,
				ObjectPath:    "rnetdata_ebpf_process.5.4.rhf.o",
				LoadMode:      LoadLegacy,
				ProgramMode:   LoadTracepoint,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := BuildLoadPlan(LoadPlanRequest{
				PluginsDir:       defaultPluginsDir(),
				Kernels:          tc.kernels,
				IsRHF:            tc.isRHF,
				KernelVersion:    tc.kver,
				Name:             tc.name,
				IsReturn:         tc.isReturn,
				HasResizableMaps: tc.hasResizableMaps,
				IsDebian:         tc.isDebian,
				HasBTF:           tc.hasBTF,
				Load:             tc.load,
				CoreAttach:       tc.coreAttach,
				Mode:             tc.mode,
			})

			tc.want.ObjectPath = filepath.Join(defaultPluginsDir(), "ebpf.d", tc.want.ObjectPath)

			if got != tc.want {
				t.Fatalf("BuildLoadPlan() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestKernelRejected(t *testing.T) {
	dir := t.TempDir()
	reject := filepath.Join(dir, "reject.txt")
	if err := os.WriteFile(reject, []byte("6.18.26\n# comment\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	rejected, err := KernelRejected("6.18.26-1", reject)
	if err != nil {
		t.Fatalf("KernelRejected() returned error: %v", err)
	}
	if !rejected {
		t.Fatalf("KernelRejected() = false, want true")
	}
}
