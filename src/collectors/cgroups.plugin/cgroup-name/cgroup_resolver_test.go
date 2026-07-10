// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMainDispatchPureBranches(t *testing.T) {
	t.Setenv("NETDATA_HOST_PREFIX", "")
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("PODMAN_HOST", "")
	t.Setenv("NETDATA_LOG_LEVEL", "emerg")

	tests := map[string]struct {
		cgroup string
		stdout string
		code   int
	}{
		"k8s root": {
			cgroup: "kubepods",
			stdout: "k8s_kubepods\n",
			code:   exitSuccess,
		},
		"k8s qos": {
			cgroup: "kubepods-burstable",
			stdout: "k8s_kubepods_burstable\n",
			code:   exitSuccess,
		},
		"systemd nspawn": {
			cgroup: "machine.slice_demo.service",
			stdout: "demo\n",
			code:   exitSuccess,
		},
		"empty systemd nspawn capture falls back": {
			cgroup: "machine.slice_.service",
			stdout: "machine.slice_.service\n",
			code:   exitSuccess,
		},
		"libvirt lxc": {
			cgroup: "machine.slice_machine-lxc_x2d969_x2dhubud0xians01.scope/libvirt_init.scope",
			stdout: "lxc/hubud0xians01_libvirt_init\n",
			code:   exitSuccess,
		},
		"libvirt qemu": {
			cgroup: "machine.slice_machine-qemu_x2d1_x2dopnsense.scope",
			stdout: "qemu_opnsense\n",
			code:   exitSuccess,
		},
		"legacy libvirt qemu": {
			cgroup: "machine_vm-one.libvirt-qemu",
			stdout: "qemu_vm_one\n",
			code:   exitSuccess,
		},
		"lxc payload": {
			cgroup: "lxc.payload.demo",
			stdout: "demo\n",
			code:   exitSuccess,
		},
		"empty lxc payload capture falls back": {
			cgroup: "lxc.payload.",
			stdout: "lxc.payload.\n",
			code:   exitSuccess,
		},
		"short hexadecimal docker id falls through enabled": {
			cgroup: "docker_abcdef",
			stdout: "docker_abcdef\n",
			code:   exitSuccess,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			code := run([]string{"cgroup-name", tt.cgroup, tt.cgroup}, &out)
			if code != tt.code {
				t.Fatalf("exit code: want %d got %d", tt.code, code)
			}
			if out.String() != tt.stdout {
				t.Fatalf("stdout:\nwant %q\n got %q", tt.stdout, out.String())
			}
		})
	}
}

func TestProxmoxBranches(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("NETDATA_HOST_PREFIX", tmp)
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("PODMAN_HOST", "")
	t.Setenv("NETDATA_LOG_LEVEL", "emerg")

	qemuDir := filepath.Join(tmp, "etc/pve/qemu-server")
	lxcDir := filepath.Join(tmp, "etc/pve/lxc")
	if err := os.MkdirAll(qemuDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(lxcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(qemuDir, "101.conf"), []byte("name: vm-one\nname: ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(qemuDir, "103.conf"), []byte("description: no name field\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lxcDir, "102.conf"), []byte("hostname: ct-one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := map[string]struct {
		cgroup string
		want   string
	}{
		"qemu name": {
			cgroup: "qemu.slice_101.scope",
			want:   "qemu_vm-one\n",
		},
		"qemu missing name": {
			cgroup: "qemu.slice_103.scope",
			want:   "qemu_\n",
		},
		"lxc hostname": {
			cgroup: "lxc_102",
			want:   "ct-one\n",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if code := run([]string{"cgroup-name", test.cgroup, test.cgroup}, &out); code != exitSuccess {
				t.Fatalf("exit code = %d, want success", code)
			}
			if got := out.String(); got != test.want {
				t.Fatalf("stdout: want %q got %q", test.want, got)
			}
		})
	}
}

func TestMachineNameMatchesShellSedSemantics(t *testing.T) {
	tests := map[string]struct {
		input  string
		marker string
		want   string
	}{
		"shell documented qemu example": {
			input:  "machine.slice_machine-qemu_x2d1_x2dopnsense.scope",
			marker: "-qemu",
			want:   "opnsense",
		},
		"digits after later markers survive": {
			input:  "machine.slice_machine-qemu_x2d2_x2dweb_x2d02.scope",
			marker: "-qemu",
			want:   "web02",
		},
		"similarly named machines stay distinct": {
			input:  "machine.slice_machine-qemu_x2d1_x2dweb_x2d01.scope",
			marker: "-qemu",
			want:   "web01",
		},
		"shell documented nested lxc example": {
			input:  "machine.slice_machine-lxc/x2d969/x2dhubud0xians01.scope/libvirt_init.scope",
			marker: "-lxc",
			want:   "hubud0xians01/libvirt_init",
		},
		"plain lxc machine": {
			input:  "machine.slice_machine-lxc/x2d969/x2dhubud0xians01.scope",
			marker: "-lxc",
			want:   "hubud0xians01",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := machineName(test.input, test.marker); got != test.want {
				t.Fatalf("machineName(%q, %q) = %q, want %q", test.input, test.marker, got, test.want)
			}
		})
	}
}

func TestHostPathKeepsAbsolutePathsWithEmptyPrefix(t *testing.T) {
	tests := map[string]struct {
		prefix string
		path   string
		want   string
	}{
		"empty prefix keeps absolute path": {
			path: "/etc/pve",
			want: "/etc/pve",
		},
		"host prefix is prepended": {
			prefix: "/host",
			path:   "/etc/pve",
			want:   "/host/etc/pve",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := hostPath(test.prefix, test.path); got != test.want {
				t.Fatalf("hostPath(%q, %q) = %q, want %q", test.prefix, test.path, got, test.want)
			}
		})
	}
}

func TestFileReadableRejectsNonRegularFiles(t *testing.T) {
	if fileReadable(os.DevNull) {
		t.Fatalf("fileReadable(%q) accepted a non-regular file", os.DevNull)
	}
}
