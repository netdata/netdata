package main

import (
	"errors"
	"fmt"
	"io/fs"
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
		"RHEL 10.2 errata kernel": {
			release: "6.12.0-211.7.1.el10_2.x86_64",
			want:    396288,
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

func TestRHEL10ErrataKernelSelectsRHF612Object(t *testing.T) {
	kver, err := KernelVersionFromRelease("6.12.0-211.7.1.el10_2.x86_64")
	if err != nil {
		t.Fatalf("KernelVersionFromRelease() error = %v", err)
	}

	selector := SelectIndex(1<<11, 10*256, kver)
	if selector != 11 {
		t.Fatalf("SelectIndex() = %d, want 11 (6.12)", selector)
	}

	path := BuildObjectPath("/usr/libexec/netdata/plugins.d", selector, "disk", false, 10*256)
	want := filepath.Join("/usr/libexec/netdata/plugins.d", "ebpf.d", "pnetdata_ebpf_disk.6.12.rhf.o")
	if path != want {
		t.Fatalf("BuildObjectPath() = %q, want %q", path, want)
	}
}

func TestRHEL10612StreamIsBoundedAndDoesNotTryGenericObjects(t *testing.T) {
	if !isRHEL10Kernel612Stream(10*256, minimumKernelVersionArena) ||
		!isRHEL10Kernel612Stream(10*256, minimumKernelVersionArena+211) {
		t.Fatal("RHEL 10 / 6.12 should use the RHF artifact stream")
	}
	if isRHEL10Kernel612Stream(11*256, minimumKernelVersionArena) ||
		isRHEL10Kernel612Stream(10*256, maximumKernelVersionArena) {
		t.Fatal("the RHEL 10 / 6.12 selector must not include other RH majors or kernel bases")
	}

	primary := LoadPlan{
		KernelVersion: minimumKernelVersionArena,
		IsRHF:         10 * 256,
		Selector:      11,
		Flavor:        ObjectFlavorBuffer,
	}
	plans := buildFallbackPlans(primary, defaultPluginsDir(), primary.IsRHF, "socket", socketMaxBaseSelector)
	for _, plan := range plans {
		if plan.IsRHF == -1 {
			t.Fatalf("RHEL 10 / 6.12 must not fall back to generic objects: %#v", plans)
		}
	}
	if len(plans) != 2 {
		t.Fatalf("buildFallbackPlans() returned %d plans, want RHF buffer and base only: %#v", len(plans), plans)
	}

	if SelectMaxIndex(11*256, minimumKernelVersionArena) == 11 {
		t.Fatalf("SelectMaxIndex() selected RHEL 10's 6.12 rung for RHEL 11")
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
		"cloudlinux": {
			contents: "CloudLinux release 8.10 (Shared)",
			want:     2058,
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

func TestRedHatReleaseFromOSRelease(t *testing.T) {
	tests := map[string]struct {
		contents string
		want     int
	}{
		"rhel id": {
			contents: "ID=\"rhel\"\nID_LIKE=\"fedora\"\nVERSION_ID=\"10.1\"\n",
			want:     2561,
		},
		"rocky id": {
			contents: "ID=rocky\nVERSION_ID=9.6\n",
			want:     2310,
		},
		"cloudlinux crlf and patch version": {
			contents: "ID=cloudlinux\r\nVERSION_ID=\"8.10.1\"\r\n",
			want:     2058,
		},
		"invalid patch component": {
			contents: "ID=rocky\nVERSION_ID=9.6.bad\n",
			want:     -1,
		},
		"amazon linux 2 ancestry is not sufficient": {
			contents: "ID=amzn\nID_LIKE=\"centos rhel fedora\"\nVERSION_ID=2\n",
			want:     -1,
		},
		"fedora ancestry is not sufficient": {
			contents: "ID=fedora\nID_LIKE=rhel\nVERSION_ID=42\n",
			want:     -1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := RedHatReleaseFromOSRelease(tc.contents); got != tc.want {
				t.Fatalf("RedHatReleaseFromOSRelease() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRedHatReleaseFallsBackAfterUnreadableReleaseFile(t *testing.T) {
	redHatErr := fmt.Errorf("read /etc/redhat-release: %w", fs.ErrPermission)

	version, err := redHatReleaseFromUnreadableFile(
		redHatErr,
		"ID=rhel\nVERSION_ID=9.6\n",
		nil,
	)
	if err != nil || version != 9*256+6 {
		t.Fatalf("fallback returned (%d, %v), want (%d, nil)", version, err, 9*256+6)
	}

	version, err = redHatReleaseFromUnreadableFile(redHatErr, "", fs.ErrPermission)
	if version != -1 || !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("both unreadable files returned (%d, %v), want (-1, permission error)", version, err)
	}

	version, err = redHatReleaseFromUnreadableFile(redHatErr, "", fs.ErrNotExist)
	if version != -1 || !errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("permission error plus absent fallback returned (%d, %v), want only the primary permission error to match", version, err)
	}
}

func TestShouldWarnRedHatDetectionError(t *testing.T) {
	tests := map[string]struct {
		err  error
		want bool
	}{
		"not red hat":         {err: errNotRedHatFamily, want: false},
		"wrapped not red hat": {err: fmt.Errorf("release lookup: %w", errNotRedHatFamily), want: false},
		"metadata absent":     {err: fs.ErrNotExist, want: false},
		"metadata unreadable": {err: fs.ErrPermission, want: true},
		"primary unreadable fallback absent": {
			err:  fmt.Errorf("unable to read redhat-release: %w (also unable to read os-release: %v)", fs.ErrPermission, fs.ErrNotExist),
			want: true,
		},
		"no error": {err: nil, want: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := shouldWarnRedHatDetectionError(tc.err); got != tc.want {
				t.Fatalf("shouldWarnRedHatDetectionError(%v) = %v, want %v", tc.err, got, tc.want)
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
		"rhel10-612": {
			kernels: 1 << 11,
			isRHF:   10 * 256,
			kver:    396288,
			wantIdx: 11,
			wantObj: "pnetdata_ebpf_process.6.12.rhf.o",
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

func TestEffectiveMaxBaseSelector(t *testing.T) {
	tests := []struct {
		name string
		max  int
		rhf  int
		kver uint32
		want int
	}{
		{name: "generic remains capped", max: 7, rhf: -1, kver: 396288, want: 7},
		{name: "RHEL 9 remains capped", max: 7, rhf: 9 * 256, kver: 396288, want: 7},
		{name: "RHEL 10 uses generated base family", max: 7, rhf: 10 * 256, kver: 396288, want: 11},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := effectiveMaxBaseSelector(tc.max, tc.rhf, tc.kver); got != tc.want {
				t.Fatalf("effectiveMaxBaseSelector() = %d, want %d", got, tc.want)
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
			// The plan must carry the requested family, not just bake it into the
			// path: buildFallbackPlans reads primary.IsReturn to keep the fallback
			// chain in the same 'p'/'r' family.  Asserted for every case here so a
			// regression cannot hide in whichever case sets it.
			tc.want.IsReturn = tc.isReturn

			if got != tc.want {
				t.Fatalf("BuildLoadPlan() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestBuildFallbackPlansRHFGenericAlternates(t *testing.T) {
	primary := LoadPlan{
		KernelVersion: 331264,
		IsRHF:         2308,
		Selector:      7,
		Flavor:        ObjectFlavorBuffer,
		ObjectPath:    filepath.Join(defaultPluginsDir(), "ebpf.d", "pnetdata_ebpf_socket_buffer.5.14.rhf.o"),
		LoadMode:      LoadCore,
		ProgramMode:   LoadTrampoline,
	}

	plans := buildFallbackPlans(primary, defaultPluginsDir(), primary.IsRHF, "socket", socketMaxBaseSelector)

	wantPaths := []string{
		"pnetdata_ebpf_socket_buffer.5.14.rhf.o",
		"pnetdata_ebpf_socket_buffer.5.14.o",
		"pnetdata_ebpf_socket.5.14.rhf.o",
		"pnetdata_ebpf_socket.5.14.o",
	}
	if len(plans) != len(wantPaths) {
		t.Fatalf("buildFallbackPlans() returned %d plans, want %d: %#v", len(plans), len(wantPaths), plans)
	}

	for i, want := range wantPaths {
		wantPath := filepath.Join(defaultPluginsDir(), "ebpf.d", want)
		if plans[i].ObjectPath != wantPath {
			t.Fatalf("plan[%d].ObjectPath = %q, want %q", i, plans[i].ObjectPath, wantPath)
		}
	}

	if plans[0].IsRHF != primary.IsRHF || plans[1].IsRHF != -1 || plans[2].IsRHF != primary.IsRHF ||
		plans[3].IsRHF != -1 {
		t.Fatalf("unexpected RHF fallback sequence: %#v", []int{plans[0].IsRHF, plans[1].IsRHF, plans[2].IsRHF, plans[3].IsRHF})
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
