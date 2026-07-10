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
	"sync/atomic"
	"testing"
	"time"
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

func TestPodLabelsStopAtTypedContainerField(t *testing.T) {
	labels := labelSet{items: []label{
		{name: "namespace", value: "default"},
		{name: "netdata.cloud/note", value: "keep,container_fake=value"},
		{name: "pod_uid", value: "pod-uid"},
		{name: "container_name", value: "app"},
		{name: "container_id", value: "container-id"},
	}}

	got := labelsBeforeContainerFields(labels).String()
	want := `namespace="default",netdata.cloud/note="keep,container_fake=value",pod_uid="pod-uid"`
	if got != want {
		t.Fatalf("pod labels:\nwant %q\n got %q", want, got)
	}
}

func TestKubernetesPodAndContainerOutcomes(t *testing.T) {
	base := labelSet{items: []label{
		{name: "namespace", value: "default"},
		{name: "pod_name", value: "api"},
		{name: "pod_uid", value: "pod-uid"},
		{name: "node_name", value: "node-a"},
		{name: "container_name", value: "app"},
		{name: "container_id", value: "container-id"},
	}}
	r := newResolver([]string{"cgroup-name"}, invocationConfig{logLevel: ndlpEmerg})

	t.Run("pod success", func(t *testing.T) {
		result := r.resolveKubernetesPod("k8s_get_kubepod_name", "pod-cgroup", kubernetesCgroupIdentity{
			podUID:   "pod-uid",
			qosClass: "burstable",
		}, kubernetesMetadata{
			clusterName: "cluster-a",
			systemUID:   "system-uid",
			containers:  []labelSet{base},
		})
		if result.outcome != kubePodSuccess || result.name != "pod_default_api" {
			t.Fatalf("outcome=%d name=%q", result.outcome, result.name)
		}
		wantLabels := `k8s_namespace="default",k8s_pod_name="api",k8s_node_name="node-a",k8s_kind="pod",k8s_qos_class="burstable",k8s_cluster_id="system-uid",k8s_cluster_name="cluster-a"`
		if got := result.labels.String(); got != wantLabels {
			t.Fatalf("pod labels:\nwant %q\n got %q", wantLabels, got)
		}
	})

	t.Run("missing pod retries", func(t *testing.T) {
		result := r.resolveKubernetesPod("k8s_get_kubepod_name", "pod-cgroup", kubernetesCgroupIdentity{
			podUID: "missing",
		}, kubernetesMetadata{containers: []labelSet{base}})
		if result.outcome != kubePodRetryFallback {
			t.Fatalf("outcome=%d, want retry", result.outcome)
		}
	})

	t.Run("kubevirt helper disables", func(t *testing.T) {
		labels := base.clone()
		for index := range labels.items {
			switch labels.items[index].name {
			case "pod_name":
				labels.items[index].value = "virt-launcher-vm"
			case "container_name":
				labels.items[index].value = "guest-console-log"
			}
		}
		result := r.resolveKubernetesContainer("k8s_get_kubepod_name", "container-cgroup", kubernetesCgroupIdentity{
			containerID: "container-id",
		}, kubernetesMetadata{containerLabels: labels, hasContainerLabels: true})
		if result.outcome != kubePodDisableFallback {
			t.Fatalf("outcome=%d, want disable", result.outcome)
		}
	})

	t.Run("null name policy", func(t *testing.T) {
		labels := base.without("namespace")
		for _, test := range []struct {
			name       string
			useKubelet bool
			want       kubePodOutcome
		}{
			{name: "API enables", want: kubePodEnableFallback},
			{name: "kubelet retries", useKubelet: true, want: kubePodRetryFallback},
		} {
			t.Run(test.name, func(t *testing.T) {
				resolver := newResolver([]string{"cgroup-name"}, invocationConfig{
					logLevel: ndlpEmerg,
					kubernetes: kubernetesConfig{
						useKubelet: test.useKubelet,
					},
				})
				result := resolver.resolveKubernetesContainer("k8s_get_kubepod_name", "container-cgroup", kubernetesCgroupIdentity{
					containerID: "container-id",
				}, kubernetesMetadata{containerLabels: labels, hasContainerLabels: true})
				if result.outcome != test.want {
					t.Fatalf("outcome=%d, want %d", result.outcome, test.want)
				}
			})
		}
	})
}

func TestSingleCgroupProcessIsPause(t *testing.T) {
	readPause := func(pid string) ([]byte, error) {
		if pid != "42" {
			t.Fatalf("pid = %q, want 42", pid)
		}
		return []byte("pause\n"), nil
	}
	if !singleCgroupProcessIsPause([]byte("42\n"), readPause) {
		t.Fatal("single pause process was not detected")
	}
	if singleCgroupProcessIsPause([]byte("42 43\n"), readPause) {
		t.Fatal("multiple processes must not be classified as a pause container")
	}
}

func TestKubernetesFallbackExitMapping(t *testing.T) {
	id := strings.Repeat("c", 64)
	cgroup := "kubepods-burstable-pod11111111_2222_3333_4444_555555555555_docker-" + id + ".scope"

	for _, test := range []struct {
		name       string
		useKubelet bool
		wantCode   int
	}{
		{name: "API null name enables", wantCode: exitSuccess},
		{name: "kubelet null name retries", useKubelet: true, wantCode: exitRetry},
	} {
		t.Run(test.name, func(t *testing.T) {
			tmp := t.TempDir()
			cache := newKubernetesCache(tmp)
			cache.writeClusterName("cluster-a")
			cache.writeSystemUID("system-uid")
			cache.writeContainers([]labelSet{{items: []label{
				{name: "pod_name", value: "api"},
				{name: "container_name", value: "app"},
				{name: "container_id", value: id},
			}}})

			var out bytes.Buffer
			code := runWithConfig([]string{"cgroup-name", cgroup, cgroup}, &out, invocationConfig{
				tmpDir:   tmp,
				logLevel: ndlpEmerg,
				kubernetes: kubernetesConfig{
					useKubelet: test.useKubelet,
				},
			})
			if code != test.wantCode {
				t.Fatalf("exit code = %d, want %d", code, test.wantCode)
			}
			if got, want := out.String(), "k8s_"+cgroup+"\n"; got != want {
				t.Fatalf("stdout = %q, want %q", got, want)
			}
		})
	}

	t.Run("unrecognized kubepod disables", func(t *testing.T) {
		var out bytes.Buffer
		code := runWithConfig([]string{"cgroup-name", "kubepods-invalid", "kubepods-invalid"}, &out, invocationConfig{
			tmpDir:   t.TempDir(),
			logLevel: ndlpEmerg,
		})
		if code != exitDisable {
			t.Fatalf("exit code = %d, want disable", code)
		}
		if got, want := out.String(), "k8s_kubepods-invalid\n"; got != want {
			t.Fatalf("stdout = %q, want %q", got, want)
		}
	})
}

func TestKubernetesAPIServerDeadlinePolicy(t *testing.T) {
	id := strings.Repeat("e", 64)
	cgroup := "kubepods-burstable-pod11111111_2222_3333_4444_555555555555_docker-" + id + ".scope"

	for _, test := range []struct {
		name       string
		block      bool
		timeout    time.Duration
		wantCode   int
		wantOutput string
	}{
		{name: "deadline retries without output", block: true, timeout: 25 * time.Millisecond, wantCode: exitRetry},
		{name: "fast failure keeps shell enable fallback", timeout: 2 * time.Second, wantCode: exitSuccess, wantOutput: "k8s_" + cgroup + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if request.URL.Path != "/api/v1/pods" {
					t.Errorf("path = %q, want /api/v1/pods", request.URL.Path)
				}
				if test.block {
					<-request.Context().Done()
					return
				}
				http.Error(w, "fixture failure", http.StatusServiceUnavailable)
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
			tmp := t.TempDir()
			cache := newKubernetesCache(tmp)
			cache.writeClusterName("cluster-a")
			cache.writeSystemUID("system-uid")

			var out bytes.Buffer
			code := runWithConfig([]string{"cgroup-name", cgroup, cgroup}, &out, invocationConfig{
				tmpDir:   tmp,
				logLevel: ndlpEmerg,
				timeout:  test.timeout,
				kubernetes: kubernetesConfig{
					serviceHost: host,
					servicePort: port,
					tlsInsecure: true,
				},
			})
			if code != test.wantCode {
				t.Fatalf("exit code = %d, want %d", code, test.wantCode)
			}
			if got := out.String(); got != test.wantOutput {
				t.Fatalf("stdout = %q, want %q", got, test.wantOutput)
			}
			if got := calls.Load(); got != 1 {
				t.Fatalf("API calls = %d, want 1", got)
			}
		})
	}
}

func TestKubernetesPodEndToEndOutput(t *testing.T) {
	uid := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	pods := `{"items":[{"metadata":{"namespace":"default","name":"api-123","uid":"` + uid + `","annotations":{"netdata.cloud/service":"payments"},"ownerReferences":[{"controller":true,"kind":"ReplicaSet","name":"api-123"}]},"spec":{"nodeName":"node-a"},"status":{"containerStatuses":[{"name":"app","containerID":"containerd://` + strings.Repeat("f", 64) + `"}]}}]}`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/v1/pods" {
			t.Errorf("path = %q, want /api/v1/pods", request.URL.Path)
		}
		_, _ = w.Write([]byte(pods))
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

	tmp := t.TempDir()
	cache := newKubernetesCache(tmp)
	cache.writeClusterName("cluster-a")
	cache.writeSystemUID("system-uid")
	cgroup := "kubepods-burstable-pod" + strings.ReplaceAll(uid, "-", "_") + ".slice"
	var out bytes.Buffer
	code := runWithConfig([]string{"cgroup-name", cgroup, cgroup}, &out, invocationConfig{
		tmpDir:   tmp,
		logLevel: ndlpEmerg,
		kubernetes: kubernetesConfig{
			serviceHost: host,
			servicePort: port,
			tlsInsecure: true,
		},
	})
	if code != exitSuccess {
		t.Fatalf("exit code = %d, want success", code)
	}
	want := `k8s_pod_default_api-123 k8s_namespace="default",k8s_pod_name="api-123",k8s_netdata.cloud/service="payments",k8s_controller_kind="ReplicaSet",k8s_controller_name="api-123",k8s_node_name="node-a",k8s_kind="pod",k8s_qos_class="burstable",k8s_cluster_id="system-uid",k8s_cluster_name="cluster-a"` + "\n"
	if got := out.String(); got != want {
		t.Fatalf("stdout:\nwant %q\n got %q", want, got)
	}
}
