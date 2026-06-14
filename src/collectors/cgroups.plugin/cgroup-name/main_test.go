// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		`LABEL_netdata.cloud/escaped="a,b\"c\nline"`,
	}, "\n"))

	if r.name != "prod-api-worker-abc123" {
		t.Fatalf("unexpected name %q", r.name)
	}
	wantLabels := `image="registry.example.invalid/app:1",netdata.cloud/service="payments",netdata.cloud/url="https://example.invalid/path?k=v",netdata.cloud/escaped="a,b\"c line"`
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
		`LABEL_netdata.cloud/a="1"`,
		`LABEL_netdata.cloud/b="2"`,
	}, "\n")
	if got != want {
		t.Fatalf("unexpected inspect output:\nwant %q\n got %q", want, got)
	}
}

func TestLabelHelpers(t *testing.T) {
	labels := strings.Join([]string{
		labelPair("namespace", "default"),
		labelPair("pod_name", "api,primary"),
		labelPair("container_id", "docker://abc"),
		labelPair("netdata.cloud/key", "a=b"),
		labelPair("netdata.cloud/escaped", "a,b\"c\nline"),
	}, ",")
	if strings.Contains(labels, "\n") {
		t.Fatalf("label stream contains a newline: %q", labels)
	}
	if got := getLblVal(labels, "pod_name"); got != "api,primary" {
		t.Fatalf("getLblVal with embedded comma = %q", got)
	}
	if got := getLblVal(labels, "netdata.cloud/key"); got != "a=b" {
		t.Fatalf("getLblVal with embedded equals = %q", got)
	}
	if got := getLblVal(labels, "netdata.cloud/escaped"); got != "a,b\"c line" {
		t.Fatalf("getLblVal with escaped value = %q", got)
	}
	if got := getLblVal(labels, "missing"); got != "null" {
		t.Fatalf("missing label = %q", got)
	}
	if got := removeLbl(labels, "container_id"); strings.Contains(got, "container_id") {
		t.Fatalf("removeLbl left container_id in %q", got)
	}
	if got := addLblPrefix(`a="1,2",b="x\"y"`, "k8s_"); got != `k8s_a="1,2",k8s_b="x\"y"` {
		t.Fatalf("addLblPrefix = %q", got)
	}
}

func withTrustedCommandDirs(t *testing.T, dirs ...string) {
	t.Helper()
	previous := trustedCommandDirs
	trustedCommandDirs = append([]string(nil), dirs...)
	t.Cleanup(func() {
		trustedCommandDirs = previous
	})
}

func TestTrustedCommandPathIgnoresAmbientPath(t *testing.T) {
	tmp := t.TempDir()
	command := "netdata-test-command"
	if err := os.WriteFile(filepath.Join(tmp, command), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", tmp)
	withTrustedCommandDirs(t, "/definitely-not-a-command-dir")
	if _, ok := trustedCommandPath(command); ok {
		t.Fatal("ambient PATH must not make a command trusted")
	}

	withTrustedCommandDirs(t, tmp)
	if path, ok := trustedCommandPath(command); !ok || path != filepath.Join(tmp, command) {
		t.Fatalf("trusted command path = %q/%v, want %q/true", path, ok, filepath.Join(tmp, command))
	}
}

func TestMainDispatchPureBranches(t *testing.T) {
	tmp := t.TempDir()
	withTrustedCommandDirs(t, tmp)
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
	withTrustedCommandDirs(t, tmp)
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
	withTrustedCommandDirs(t, tmp)
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

func TestMirroredK8sContainerPathFixturesFromCache(t *testing.T) {
	// These are sanitized, pattern-preserving fixtures adapted from public test
	// corpora in DataDog/datadog-agent, google/cadvisor, grafana/beyla, and
	// sustainable-computing-io/kepler. They intentionally avoid copying upstream
	// pod/container names while preserving the cgroup path shapes.
	tmp := t.TempDir()
	withTrustedCommandDirs(t, tmp)
	if err := os.WriteFile(filepath.Join(tmp, "jq"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)
	t.Setenv("TMPDIR", tmp)
	t.Setenv("NETDATA_LOG_LEVEL", "emerg")
	t.Setenv("NETDATA_HOST_PREFIX", tmp)
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("PODMAN_HOST", "")

	if err := os.MkdirAll(filepath.Join(tmp, "sys/fs/cgroup"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "netdata-cgroups-k8s-cluster-name"), []byte("fixture-cluster\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "netdata-cgroups-kubesystem-uid"), []byte("fixture-system-uid\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	idA := strings.Repeat("a", 64)
	idB := strings.Repeat("b", 64)
	idC := strings.Repeat("c", 64)
	idD := strings.Repeat("d", 64)
	idE := strings.Repeat("e", 64)
	outerDockerID := strings.Repeat("f", 64)
	uidA := "11111111-2222-3333-4444-555555555555"
	uidB := "22222222-3333-4444-5555-666666666666"
	uidC := "33333333-4444-5555-6666-777777777777"
	uidD := "44444444-5555-6666-7777-888888888888"
	uidE := "55555555-6666-7777-8888-999999999999"
	uidAUnderscore := strings.ReplaceAll(uidA, "-", "_")
	uidCUnderscore := strings.ReplaceAll(uidC, "-", "_")
	uidDUnderscore := strings.ReplaceAll(uidD, "-", "_")

	containerLine := func(namespace, podName, podUID, nodeName, containerName, containerID string) string {
		return `namespace="` + namespace + `",pod_name="` + podName + `",pod_uid="` + podUID + `",controller_kind="ReplicaSet",controller_name="` + podName + `-rs",node_name="` + nodeName + `",container_name="` + containerName + `",container_id="` + containerID + `"`
	}
	containers := strings.Join([]string{
		containerLine("default", "api-a", uidA, "node-a", "app", idA),
		containerLine("default", "api-b", uidB, "node-b", "worker", idB),
		containerLine("default", "api-c", uidC, "node-c", "sidecar", idC),
		containerLine("openshift-monitoring", "api-d", uidD, "node-d", "collector", idD),
		containerLine("default", "api-e", uidE, "node-e", "inner", idE),
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmp, "netdata-cgroups-containers"), []byte(containers+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		cgroup        string
		wantName      string
		wantNamespace string
		wantPod       string
		wantNode      string
		wantContainer string
		wantQOS       string
	}{
		{
			name:          "systemd cri-containerd burstable",
			cgroup:        "kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod" + uidAUnderscore + ".slice/cri-containerd-" + idA + ".scope",
			wantName:      "k8s_cntr_default_api-a_app",
			wantNamespace: "default",
			wantPod:       "api-a",
			wantNode:      "node-a",
			wantContainer: "app",
			wantQOS:       "burstable",
		},
		{
			name:          "cgroupfs plain container id",
			cgroup:        "kubepods/burstable/pod" + uidB + "/" + idB,
			wantName:      "k8s_cntr_default_api-b_worker",
			wantNamespace: "default",
			wantPod:       "api-b",
			wantNode:      "node-b",
			wantContainer: "worker",
			wantQOS:       "burstable",
		},
		{
			name:          "kind kubelet-prefixed besteffort",
			cgroup:        "kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-besteffort.slice/kubelet-kubepods-besteffort-pod" + uidCUnderscore + ".slice/cri-containerd-" + idC + ".scope",
			wantName:      "k8s_cntr_default_api-c_sidecar",
			wantNamespace: "default",
			wantPod:       "api-c",
			wantNode:      "node-c",
			wantContainer: "sidecar",
			wantQOS:       "besteffort",
		},
		{
			name:          "openshift crio guaranteed",
			cgroup:        "kubepods.slice/kubepods-pod" + uidDUnderscore + ".slice/crio-" + idD + ".scope",
			wantName:      "k8s_cntr_openshift-monitoring_api-d_collector",
			wantNamespace: "openshift-monitoring",
			wantPod:       "api-d",
			wantNode:      "node-d",
			wantContainer: "collector",
			wantQOS:       "guaranteed",
		},
		{
			name:          "docker in docker cgroupfs inner container",
			cgroup:        "docker/" + outerDockerID + "/kubelet/kubepods/burstable/pod" + uidE + "/" + idE,
			wantName:      "k8s_cntr_default_api-e_inner",
			wantNamespace: "default",
			wantPod:       "api-e",
			wantNode:      "node-e",
			wantContainer: "inner",
			wantQOS:       "burstable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			code := run([]string{"cgroup-name", tt.cgroup, tt.cgroup}, &out)
			if code != exitSuccess {
				t.Fatalf("exit code: want %d got %d", exitSuccess, code)
			}
			got := strings.TrimSpace(out.String())
			if !strings.HasPrefix(got, tt.wantName+" ") {
				t.Fatalf("name prefix:\nwant %q\n got %q", tt.wantName+" ", got)
			}
			for _, want := range []string{
				`k8s_namespace="` + tt.wantNamespace + `"`,
				`k8s_pod_name="` + tt.wantPod + `"`,
				`k8s_node_name="` + tt.wantNode + `"`,
				`k8s_container_name="` + tt.wantContainer + `"`,
				`k8s_kind="container"`,
				`k8s_qos_class="` + tt.wantQOS + `"`,
				`k8s_cluster_id="fixture-system-uid"`,
				`k8s_cluster_name="fixture-cluster"`,
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("output missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestMirroredK8sPodListFixtureViaKubelet(t *testing.T) {
	// This small pod-list payload preserves shapes found in Datadog/New Relic/
	// OpenTelemetry kubelet fixtures: owner references, annotations, node names,
	// Docker IDs, containerd IDs, and CRI-O IDs.
	tmp := t.TempDir()
	withTrustedCommandDirs(t, tmp)
	if err := os.WriteFile(filepath.Join(tmp, "jq"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)
	t.Setenv("TMPDIR", tmp)
	t.Setenv("NETDATA_LOG_LEVEL", "emerg")
	t.Setenv("NETDATA_HOST_PREFIX", tmp)
	t.Setenv("KUBERNETES_SERVICE_HOST", "127.0.0.1")
	t.Setenv("KUBERNETES_PORT_443_TCP_PORT", "443")
	t.Setenv("USE_KUBELET_FOR_PODS_METADATA", "1")

	if err := os.WriteFile(filepath.Join(tmp, "netdata-cgroups-k8s-cluster-name"), []byte("fixture-cluster\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "netdata-cgroups-kubesystem-uid"), []byte("fixture-system-uid\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	containerdID := strings.Repeat("1", 64)
	dockerID := strings.Repeat("2", 64)
	crioID := strings.Repeat("3", 64)
	pods := `{"items":[` +
		`{"metadata":{"namespace":"default","name":"api-123","uid":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","annotations":{"checksum/config":"ignored","netdata.cloud/service":"payments"},"ownerReferences":[{"controller":true,"kind":"ReplicaSet","name":"api-123"}]},"spec":{"nodeName":"node-a"},"status":{"containerStatuses":[{"name":"app","containerID":"containerd://` + containerdID + `"},{"name":"sidecar","containerID":"docker://` + dockerID + `"}]}},` +
		`{"metadata":{"namespace":"openshift-monitoring","name":"collector-0","uid":"bbbbbbbb-cccc-dddd-eeee-ffffffffffff","ownerReferences":[{"controller":true,"kind":"DaemonSet","name":"collector"}]},"spec":{"nodeName":"node-b"},"status":{"containerStatuses":[{"name":"collector","containerID":"cri-o://` + crioID + `"}]}}` +
		`]}`

	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pods" {
			t.Fatalf("unexpected kubelet path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(pods))
	}))
	defer ts.Close()
	t.Setenv("KUBELET_URL", ts.URL)

	cgroup := "kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podaaaaaaaa_bbbb_cccc_dddd_eeeeeeeeeeee.slice/cri-containerd-" + containerdID + ".scope"
	var out bytes.Buffer
	code := run([]string{"cgroup-name", cgroup, cgroup}, &out)
	if code != exitSuccess {
		t.Fatalf("exit code: want %d got %d", exitSuccess, code)
	}
	got := strings.TrimSpace(out.String())
	for _, want := range []string{
		`k8s_cntr_default_api-123_app `,
		`k8s_namespace="default"`,
		`k8s_pod_name="api-123"`,
		`k8s_netdata.cloud/service="payments"`,
		`k8s_controller_kind="ReplicaSet"`,
		`k8s_controller_name="api-123"`,
		`k8s_node_name="node-a"`,
		`k8s_container_name="app"`,
		`k8s_kind="container"`,
		`k8s_qos_class="burstable"`,
		`k8s_cluster_id="fixture-system-uid"`,
		`k8s_cluster_name="fixture-cluster"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}

	cache, err := os.ReadFile(filepath.Join(tmp, "netdata-cgroups-containers"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`container_id="` + containerdID + `"`,
		`container_id="` + dockerID + `"`,
		`container_id="` + crioID + `"`,
	} {
		if !strings.Contains(string(cache), want) {
			t.Fatalf("cache missing %q:\n%s", want, string(cache))
		}
	}
	for _, name := range []string{
		"netdata-cgroups-k8s-cluster-name",
		"netdata-cgroups-kubesystem-uid",
		"netdata-cgroups-containers",
	} {
		st, err := os.Stat(filepath.Join(tmp, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := st.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s permissions = %o, want 600", name, got)
		}
	}
}

func TestMirroredDockerAndPodmanAPIFixtures(t *testing.T) {
	tmp := t.TempDir()
	withTrustedCommandDirs(t, tmp)
	if err := os.WriteFile(filepath.Join(tmp, "jq"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "snap"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)
	t.Setenv("NETDATA_LOG_LEVEL", "emerg")

	inspectJSON := func(name, image string) string {
		return `{"Name":"/` + name + `","Config":{"Env":["PATH=/usr/bin"],"Image":"` + image + `","Labels":{"com.docker.compose.service":"ignored","netdata.cloud/service":"payments","netdata.cloud/team":"platform"}}}`
	}

	t.Run("docker http api", func(t *testing.T) {
		id := strings.Repeat("4", 64)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/containers/"+id+"/json" {
				t.Fatalf("unexpected docker API path %s", r.URL.Path)
			}
			_, _ = w.Write([]byte(inspectJSON("api-docker", "registry.example.invalid/api:v1")))
		}))
		defer ts.Close()
		t.Setenv("DOCKER_HOST", ts.URL)
		t.Setenv("PODMAN_HOST", "")

		cgroup := "system.slice/docker-" + id + ".scope"
		var out bytes.Buffer
		code := run([]string{"cgroup-name", cgroup, cgroup}, &out)
		if code != exitSuccess {
			t.Fatalf("exit code: want %d got %d", exitSuccess, code)
		}
		got := strings.TrimSpace(out.String())
		for _, want := range []string{
			`api-docker `,
			`image="registry.example.invalid/api:v1"`,
			`netdata.cloud/service="payments"`,
			`netdata.cloud/team="platform"`,
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("output missing %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, "com.docker.compose.service") {
			t.Fatalf("non-netdata label leaked into output:\n%s", got)
		}
	})

	t.Run("podman http api", func(t *testing.T) {
		id := strings.Repeat("5", 64)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/containers/"+id+"/json" {
				t.Fatalf("unexpected podman API path %s", r.URL.Path)
			}
			_, _ = w.Write([]byte(inspectJSON("api-podman", "registry.example.invalid/podman:v1")))
		}))
		defer ts.Close()
		t.Setenv("DOCKER_HOST", "")
		t.Setenv("PODMAN_HOST", ts.URL)

		cgroup := "user.slice/user-1000.slice/user@1000.service/user.slice/libpod-" + id + ".scope/container"
		var out bytes.Buffer
		code := run([]string{"cgroup-name", cgroup, cgroup}, &out)
		if code != exitSuccess {
			t.Fatalf("exit code: want %d got %d", exitSuccess, code)
		}
		got := strings.TrimSpace(out.String())
		for _, want := range []string{
			`api-podman `,
			`image="registry.example.invalid/podman:v1"`,
			`netdata.cloud/service="payments"`,
			`netdata.cloud/team="platform"`,
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("output missing %q:\n%s", want, got)
			}
		}
	})
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
	if _, err := httpGetWithContext(context.Background(), ts.URL, httpGetOptions{tlsConfig: r.k8sTLSConfig(tlsModeKubelet), noProxy: true, fail: true}); err != nil {
		t.Fatalf("kubelet mode must accept a self-signed certificate, got: %v", err)
	}
	if _, err := httpGetWithContext(context.Background(), ts.URL, httpGetOptions{tlsConfig: r.k8sTLSConfig(tlsModeAPIServer), noProxy: true, fail: true}); err == nil {
		t.Fatal("API-server mode must reject a certificate that does not chain to the service-account CA")
	}

	t.Setenv("K8S_TLS_INSECURE", "1")
	if _, err := httpGetWithContext(context.Background(), ts.URL, httpGetOptions{tlsConfig: r.k8sTLSConfig(tlsModeAPIServer), noProxy: true, fail: true}); err != nil {
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

func TestSetupDeadlineBudget(t *testing.T) {
	t.Run("unbounded when unset", func(t *testing.T) {
		t.Setenv("NETDATA_CGROUP_NAME_TIMEOUT_MS", "")
		r := newResolver([]string{"cgroup-name"}, &bytes.Buffer{})
		_, cancel := r.setupDeadline()
		defer cancel()
		if !r.expiresAt.IsZero() {
			t.Fatal("expiresAt must be zero when no timeout is configured")
		}
		if r.budgetExpired() {
			t.Fatal("an unbounded budget must never report expired")
		}
	})

	t.Run("expires after the configured budget", func(t *testing.T) {
		t.Setenv("NETDATA_CGROUP_NAME_TIMEOUT_MS", "10")
		r := newResolver([]string{"cgroup-name"}, &bytes.Buffer{})
		ctx, cancel := r.setupDeadline()
		defer cancel()
		if r.expiresAt.IsZero() {
			t.Fatal("expiresAt must be set when a timeout is configured")
		}
		if r.budgetExpired() {
			t.Fatal("budget must not be expired immediately")
		}
		time.Sleep(20 * time.Millisecond)
		if !r.budgetExpired() {
			t.Fatal("budget must report expired after the deadline passes")
		}
		if ctx.Err() == nil {
			t.Fatal("the deadline context must be done after the budget expires")
		}
	})
}

func TestLogfmtIsSingleLineAndQuoted(t *testing.T) {
	r := newResolver([]string{"cgroup-name", "weird arg"}, &bytes.Buffer{})
	r.logLevel = ndlpDebug

	old := os.Stderr
	rp, wp, _ := os.Pipe()
	os.Stderr = wp
	r.error("name has a \" quote and a\nnewline")
	_ = wp.Close()
	os.Stderr = old

	out, _ := io.ReadAll(rp)
	line := string(out)
	if strings.Count(strings.TrimRight(line, "\n"), "\n") != 0 {
		t.Fatalf("log line must be single-line, got: %q", line)
	}
	if !strings.Contains(line, "level=error") || !strings.Contains(line, "comm=cgroup-name") {
		t.Fatalf("log line missing expected fields: %q", line)
	}
	if !strings.Contains(line, `\"`) || !strings.Contains(line, `\n`) {
		t.Fatalf("quotes and newlines in the message must be escaped: %q", line)
	}
}
