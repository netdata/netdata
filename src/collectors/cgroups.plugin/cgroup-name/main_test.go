// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"net/http"
	"net/http/httptest"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCmdLinePreservesShellQuotingBug(t *testing.T) {
	got := buildCmdLine([]string{"cgroup-name", "/docker/a'b", "docker_a"})
	want := "'cgroup-name' '/docker/a'b' 'docker_a' "
	if got != want {
		t.Fatalf("unexpected cmd line:\nwant %q\n got %q", want, got)
	}
}

func TestDockerInspectParser(t *testing.T) {
	r := newResolver([]string{"cgroup-name"}, &bytes.Buffer{})
	r.parseDockerLikeInspectOutput(strings.Join([]string{
		"NOMAD_NAMESPACE=prod",
		"NOMAD_JOB_NAME=api",
		"NOMAD_TASK_NAME=worker",
		"NOMAD_SHORT_ALLOC_ID=abc123",
		"CONT_NAME=/ignored",
		"IMAGE_NAME=registry.example.invalid/app:1",
		"LABEL_netdata.cloud/service=payments",
		"LABEL_netdata.cloud/url=https://example.invalid/path?k=v",
	}, "\n"))

	if r.name != "prod-api-worker-abc123" {
		t.Fatalf("unexpected name %q", r.name)
	}
	wantLabels := `image="registry.example.invalid/app:1",netdata.cloud/service="payments",netdata.cloud/url="https://example.invalid/path?k=v"`
	if r.labels != wantLabels {
		t.Fatalf("unexpected labels:\nwant %q\n got %q", wantLabels, r.labels)
	}
}

func TestDockerJSONToInspectOutputPreservesLabelOrder(t *testing.T) {
	body := []byte(`{"Name":"/web","Config":{"Env":["A=B"],"Image":"img:v1","Labels":{"netdata.cloud/a":"1","netdata.cloud/b":"2"}}}`)
	got, ok := dockerJSONToInspectOutput(body)
	if !ok {
		t.Fatal("docker JSON did not parse")
	}
	want := strings.Join([]string{
		"A=B",
		"CONT_NAME=/web",
		"IMAGE_NAME=img:v1",
		"LABEL_netdata.cloud/a=1",
		"LABEL_netdata.cloud/b=2",
	}, "\n")
	if got != want {
		t.Fatalf("unexpected inspect output:\nwant %q\n got %q", want, got)
	}
}

func TestLabelHelpers(t *testing.T) {
	labels := `namespace="default",pod_name="api",container_id="docker://abc",netdata.cloud/key="a=b"`
	if got := getLblVal(labels, "netdata.cloud/key"); got != "a=b" {
		t.Fatalf("getLblVal with embedded equals = %q", got)
	}
	if got := getLblVal(labels, "missing"); got != "null" {
		t.Fatalf("missing label = %q", got)
	}
	if got := removeLbl(labels, "container_id"); strings.Contains(got, "container_id") {
		t.Fatalf("removeLbl left container_id in %q", got)
	}
	if got := addLblPrefix(`a="1",b="2"`, "k8s_"); got != `k8s_a="1",k8s_b="2"` {
		t.Fatalf("addLblPrefix = %q", got)
	}
}

func TestMainDispatchPureBranches(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PATH", tmp)
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
	if err := os.WriteFile(filepath.Join(lxcDir, "102.conf"), []byte("hostname: ct-one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{
		"qemu.slice_101.scope": "qemu_vm-one\n",
		"lxc_102":              "ct-one\n",
	}
	for cgroup, want := range tests {
		var out bytes.Buffer
		code := run([]string{"cgroup-name", cgroup, cgroup}, &out)
		if code != exitSuccess {
			t.Fatalf("%s exit code %d", cgroup, code)
		}
		if out.String() != want {
			t.Fatalf("%s stdout: want %q got %q", cgroup, want, out.String())
		}
	}
}

func TestPodsToContainerLines(t *testing.T) {
	pods := `{"items":[{"metadata":{"namespace":"default","name":"api-123","uid":"pod-uid","annotations":{"netdata.cloud/service":"payments"},"ownerReferences":[{"controller":true,"kind":"ReplicaSet","name":"api-123"}]},"spec":{"nodeName":"node-a"},"status":{"containerStatuses":[{"name":"app","containerID":"containerd://abcdef"}]}}]}`
	got, err := podsToContainerLines(pods)
	if err != nil {
		t.Fatal(err)
	}
	want := `namespace="default",pod_name="api-123",pod_uid="pod-uid",netdata.cloud/service="payments",controller_kind="ReplicaSet",controller_name="api-123",node_name="node-a",container_name="app",container_id="abcdef"`
	if got != want {
		t.Fatalf("container lines:\nwant %q\n got %q", want, got)
	}
}

func TestK8sContainerCachePath(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "jq"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)
	t.Setenv("TMPDIR", tmp)
	t.Setenv("NETDATA_LOG_LEVEL", "emerg")

	id := "abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"
	uid := "11111111-2222-3333-4444-555555555555"
	if err := os.WriteFile(filepath.Join(tmp, "netdata-cgroups-k8s-cluster-name"), []byte("gke_project_region_cluster\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "netdata-cgroups-kubesystem-uid"), []byte("system-uid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	line := `namespace="default",pod_name="api",pod_uid="` + uid + `",node_name="node-a",container_name="app",container_id="` + id + `"`
	if err := os.WriteFile(filepath.Join(tmp, "netdata-cgroups-containers"), []byte(line+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cgroup := "kubepods-burstable-pod11111111_2222_3333_4444_555555555555_docker-" + id + ".scope"
	var out bytes.Buffer
	code := run([]string{"cgroup-name", cgroup, cgroup}, &out)
	if code != exitSuccess {
		t.Fatalf("exit code %d", code)
	}
	want := `k8s_cntr_default_api_app k8s_namespace="default",k8s_pod_name="api",k8s_node_name="node-a",k8s_container_name="app",k8s_kind="container",k8s_qos_class="burstable",k8s_cluster_id="system-uid",k8s_cluster_name="gke_project_region_cluster"` + "\n"
	if out.String() != want {
		t.Fatalf("stdout:\nwant %q\n got %q", want, out.String())
	}
}

func TestMachineNameMatchesShellSedSemantics(t *testing.T) {
	// the shell stripped the libvirt id segment with a non-global sed
	// expression; digits after LATER x2d markers belong to the machine name
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := machineName(tc.input, tc.marker); got != tc.want {
				t.Fatalf("machineName(%q, %q) = %q, want %q", tc.input, tc.marker, got, tc.want)
			}
		})
	}
}

func TestHostPathKeepsAbsolutePathsWithEmptyPrefix(t *testing.T) {
	t.Setenv("NETDATA_HOST_PREFIX", "")
	if got := hostPath("/etc/pve"); got != "/etc/pve" {
		t.Fatalf("hostPath with empty prefix = %q, want /etc/pve", got)
	}

	t.Setenv("NETDATA_HOST_PREFIX", "/host")
	if got := hostPath("/etc/pve"); got != "/host/etc/pve" {
		t.Fatalf("hostPath with /host prefix = %q, want /host/etc/pve", got)
	}
}

func TestK8sTLSInsecureParsing(t *testing.T) {
	tests := map[string]struct {
		value string
		want  bool
	}{
		"unset is secure":        {value: "", want: false},
		"zero is secure":         {value: "0", want: false},
		"false is secure":        {value: "false", want: false},
		"FALSE is secure":        {value: "FALSE", want: false},
		"no is secure":           {value: "no", want: false},
		"one enables":            {value: "1", want: true},
		"true enables":           {value: "true", want: true},
		"yes enables":            {value: "yes", want: true},
		"arbitrary text enables": {value: "on", want: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Setenv("K8S_TLS_INSECURE", tc.value)
			if got := k8sTLSInsecure(); got != tc.want {
				t.Fatalf("k8sTLSInsecure() with %q = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

func TestK8sTLSModesAgainstSelfSignedServer(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	r := newResolver([]string{"cgroup-name"}, &bytes.Buffer{})

	t.Setenv("K8S_TLS_INSECURE", "")
	if _, err := httpGet(ts.URL, nil, r.k8sTLSConfig(tlsModeKubelet), true, true, 0); err != nil {
		t.Fatalf("kubelet mode must accept a self-signed certificate, got: %v", err)
	}
	if _, err := httpGet(ts.URL, nil, r.k8sTLSConfig(tlsModeAPIServer), true, true, 0); err == nil {
		t.Fatal("API-server mode must reject a certificate that does not chain to the service-account CA")
	}

	t.Setenv("K8S_TLS_INSECURE", "1")
	if _, err := httpGet(ts.URL, nil, r.k8sTLSConfig(tlsModeAPIServer), true, true, 0); err != nil {
		t.Fatalf("API-server mode with K8S_TLS_INSECURE=1 must accept any certificate, got: %v", err)
	}
}

func TestKubeletPodsURLAppendsPods(t *testing.T) {
	t.Setenv("KUBELET_URL", "")
	if got := kubeletPodsURL(); got != "https://localhost:10250/pods" {
		t.Fatalf("default kubelet url = %q", got)
	}

	t.Setenv("KUBELET_URL", "https://node-1:10250")
	if got := kubeletPodsURL(); got != "https://node-1:10250/pods" {
		t.Fatalf("configured kubelet url = %q, the base must get /pods appended like the shell did", got)
	}
}
