// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestK8sContainerCachePath(t *testing.T) {
	tmp := t.TempDir()
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
	// Sanitized, pattern-preserving fixtures adapted from public corpora in
	// DataDog/datadog-agent, google/cadvisor, grafana/beyla, and
	// sustainable-computing-io/kepler.
	tmp := t.TempDir()
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

	tests := map[string]struct {
		cgroup        string
		wantName      string
		wantNamespace string
		wantPod       string
		wantNode      string
		wantContainer string
		wantQOS       string
	}{
		"systemd cri-containerd burstable": {
			cgroup:        "kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod" + strings.ReplaceAll(uidA, "-", "_") + ".slice/cri-containerd-" + idA + ".scope",
			wantName:      "k8s_cntr_default_api-a_app",
			wantNamespace: "default", wantPod: "api-a", wantNode: "node-a", wantContainer: "app", wantQOS: "burstable",
		},
		"cgroupfs plain container id": {
			cgroup:        "kubepods/burstable/pod" + uidB + "/" + idB,
			wantName:      "k8s_cntr_default_api-b_worker",
			wantNamespace: "default", wantPod: "api-b", wantNode: "node-b", wantContainer: "worker", wantQOS: "burstable",
		},
		"kind kubelet-prefixed besteffort": {
			cgroup:        "kubelet.slice/kubelet-kubepods.slice/kubelet-kubepods-besteffort.slice/kubelet-kubepods-besteffort-pod" + strings.ReplaceAll(uidC, "-", "_") + ".slice/cri-containerd-" + idC + ".scope",
			wantName:      "k8s_cntr_default_api-c_sidecar",
			wantNamespace: "default", wantPod: "api-c", wantNode: "node-c", wantContainer: "sidecar", wantQOS: "besteffort",
		},
		"openshift crio guaranteed": {
			cgroup:        "kubepods.slice/kubepods-pod" + strings.ReplaceAll(uidD, "-", "_") + ".slice/crio-" + idD + ".scope",
			wantName:      "k8s_cntr_openshift-monitoring_api-d_collector",
			wantNamespace: "openshift-monitoring", wantPod: "api-d", wantNode: "node-d", wantContainer: "collector", wantQOS: "guaranteed",
		},
		"docker in docker cgroupfs inner container": {
			cgroup:        "docker/" + outerDockerID + "/kubelet/kubepods/burstable/pod" + uidE + "/" + idE,
			wantName:      "k8s_cntr_default_api-e_inner",
			wantNamespace: "default", wantPod: "api-e", wantNode: "node-e", wantContainer: "inner", wantQOS: "burstable",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if code := run([]string{"cgroup-name", test.cgroup, test.cgroup}, &out); code != exitSuccess {
				t.Fatalf("exit code: want %d got %d", exitSuccess, code)
			}
			got := strings.TrimSpace(out.String())
			if !strings.HasPrefix(got, test.wantName+" ") {
				t.Fatalf("name prefix:\nwant %q\n got %q", test.wantName+" ", got)
			}
			for _, want := range []string{
				`k8s_namespace="` + test.wantNamespace + `"`,
				`k8s_pod_name="` + test.wantPod + `"`,
				`k8s_node_name="` + test.wantNode + `"`,
				`k8s_container_name="` + test.wantContainer + `"`,
				`k8s_kind="container"`,
				`k8s_qos_class="` + test.wantQOS + `"`,
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

func k8sPodFixture() (containerdID, dockerID, crioID, pods string) {
	containerdID = strings.Repeat("1", 64)
	dockerID = strings.Repeat("2", 64)
	crioID = strings.Repeat("3", 64)
	pods = `{"items":[` +
		`{"metadata":{"namespace":"default","name":"api-123","uid":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","annotations":{"checksum/config":"ignored","netdata.cloud/service":"payments"},"ownerReferences":[{"controller":true,"kind":"ReplicaSet","name":"api-123"}]},"spec":{"nodeName":"node-a"},"status":{"containerStatuses":[{"name":"app","containerID":"containerd://` + containerdID + `"},{"name":"sidecar","containerID":"docker://` + dockerID + `"}]}},` +
		`{"metadata":{"namespace":"openshift-monitoring","name":"collector-0","uid":"bbbbbbbb-cccc-dddd-eeee-ffffffffffff","ownerReferences":[{"controller":true,"kind":"DaemonSet","name":"collector"}]},"spec":{"nodeName":"node-b"},"status":{"containerStatuses":[{"name":"collector","containerID":"cri-o://` + crioID + `"}]}}` +
		`]}`
	return
}

func assertK8sBurstableContainerResolved(t *testing.T, tmp, containerdID, dockerID, crioID string, config invocationConfig) {
	t.Helper()
	cgroup := "kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podaaaaaaaa_bbbb_cccc_dddd_eeeeeeeeeeee.slice/cri-containerd-" + containerdID + ".scope"
	var out bytes.Buffer
	if code := runWithConfig([]string{"cgroup-name", cgroup, cgroup}, &out, config); code != exitSuccess {
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
		info, err := os.Stat(filepath.Join(tmp, name))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s permissions = %o, want 600", name, got)
		}
	}
}

func TestMirroredK8sPodListFixtureViaKubelet(t *testing.T) {
	tmp := t.TempDir()
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

	containerdID, dockerID, crioID, pods := k8sPodFixture()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/pods" {
			t.Errorf("unexpected kubelet path %s", request.URL.Path)
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(pods))
	}))
	defer server.Close()
	t.Setenv("KUBELET_URL", server.URL)

	assertK8sBurstableContainerResolved(t, tmp, containerdID, dockerID, crioID, prepareInvocationConfig())
}

func TestMirroredK8sPodListFixtureViaAPIServer(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PATH", tmp)
	t.Setenv("TMPDIR", tmp)
	t.Setenv("NETDATA_LOG_LEVEL", "emerg")
	t.Setenv("NETDATA_HOST_PREFIX", tmp)
	t.Setenv("K8S_TLS_INSECURE", "true")
	t.Setenv("USE_KUBELET_FOR_PODS_METADATA", "")
	t.Setenv("MY_NODE_NAME", "")

	if err := os.WriteFile(filepath.Join(tmp, "netdata-cgroups-k8s-cluster-name"), []byte("fixture-cluster\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tokenFile := filepath.Join(tmp, "serviceaccount-token")
	if err := os.WriteFile(tokenFile, []byte("fixture-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	containerdID, dockerID, crioID, pods := k8sPodFixture()
	kubeSystemNamespace := `{"metadata":{"name":"kube-system","uid":"fixture-system-uid"}}`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer fixture-token" {
			t.Errorf("Authorization header = %q, want %q", got, "Bearer fixture-token")
		}
		switch request.URL.Path {
		case "/api/v1/namespaces/kube-system":
			_, _ = w.Write([]byte(kubeSystemNamespace))
		case "/api/v1/pods":
			_, _ = w.Write([]byte(pods))
		default:
			t.Errorf("unexpected API-server path %s", request.URL.Path)
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	parsedURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, port, err := net.SplitHostPort(parsedURL.Host)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBERNETES_SERVICE_HOST", host)
	t.Setenv("KUBERNETES_PORT_443_TCP_PORT", port)

	config := prepareInvocationConfig()
	config.kubernetes.serviceAccountTokenFile = tokenFile
	assertK8sBurstableContainerResolved(t, tmp, containerdID, dockerID, crioID, config)

	uid, err := os.ReadFile(filepath.Join(tmp, "netdata-cgroups-kubesystem-uid"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(uid)); got != "fixture-system-uid" {
		t.Fatalf("cached kube-system uid = %q, want fixture-system-uid", got)
	}
}
