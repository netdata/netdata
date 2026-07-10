// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMainDispatchPureBranches(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
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
		"invalid docker id falls through enabled": {
			cgroup: "docker_xyz",
			stdout: "docker_xyz\n",
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
	t.Setenv("PATH", tmp)
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

	tests := map[string]string{
		"qemu.slice_101.scope": "qemu_vm-one\n",
		"qemu.slice_103.scope": "qemu_\n",
		"lxc_102":              "ct-one\n",
	}
	for cgroup, want := range tests {
		var out bytes.Buffer
		if code := run([]string{"cgroup-name", cgroup, cgroup}, &out); code != exitSuccess {
			t.Fatalf("%s exit code %d", cgroup, code)
		}
		if out.String() != want {
			t.Fatalf("%s stdout: want %q got %q", cgroup, want, out.String())
		}
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
	if got := hostPath("", "/etc/pve"); got != "/etc/pve" {
		t.Fatalf("hostPath with empty prefix = %q, want /etc/pve", got)
	}
	if got := hostPath("/host", "/etc/pve"); got != "/host/etc/pve" {
		t.Fatalf("hostPath with /host prefix = %q, want /host/etc/pve", got)
	}
}
