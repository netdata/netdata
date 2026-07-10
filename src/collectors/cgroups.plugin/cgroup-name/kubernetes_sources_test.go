// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestK8sGCPGetClusterName(t *testing.T) {
	responses := map[string]string{
		"/computeMetadata/v1/project/project-id":                   "project-a",
		"/computeMetadata/v1/instance/attributes/cluster-location": "region-a",
		"/computeMetadata/v1/instance/attributes/cluster-name":     "cluster-a",
	}
	var mu sync.Mutex
	seen := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Metadata-Flavor"); got != "Google" {
			t.Errorf("Metadata-Flavor = %q, want Google", got)
		}
		value, ok := responses[request.URL.Path]
		if !ok {
			http.NotFound(w, request)
			return
		}
		mu.Lock()
		seen[request.URL.Path]++
		mu.Unlock()
		_, _ = w.Write([]byte(value))
	}))
	defer server.Close()
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")

	r := newResolver([]string{"cgroup-name"}, invocationConfig{
		logLevel: ndlpEmerg,
		kubernetes: kubernetesConfig{
			gcpMetadataURL: server.URL,
		},
	})
	name, ok := r.k8sGCPGetClusterName(context.Background())
	if !ok || name != "gke_project-a_region-a_cluster-a" {
		t.Fatalf("ok=%v name=%q", ok, name)
	}
	mu.Lock()
	defer mu.Unlock()
	for path := range responses {
		if seen[path] != 1 {
			t.Fatalf("request count for %s = %d, want 1", path, seen[path])
		}
	}
}

func TestK8sFetchPodsViaKubectlPreservesKubeConfigPresence(t *testing.T) {
	for _, test := range []struct {
		name          string
		kubeConfig    string
		kubeConfigSet bool
		wantNamespace string
		wantPods      string
	}{
		{
			name:          "unset",
			wantNamespace: "--kubeconfig= get namespaces kube-system -o json",
			wantPods:      "--kubeconfig=/etc/kubernetes/admin.conf get pods --all-namespaces -o json",
		},
		{
			name:          "explicitly empty",
			kubeConfigSet: true,
			wantNamespace: "--kubeconfig= get namespaces kube-system -o json",
			wantPods:      "--kubeconfig= get pods --all-namespaces -o json",
		},
		{
			name:          "explicit path",
			kubeConfig:    "/fixture/config",
			kubeConfigSet: true,
			wantNamespace: "--kubeconfig=/fixture/config get namespaces kube-system -o json",
			wantPods:      "--kubeconfig=/fixture/config get pods --all-namespaces -o json",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			tmp := t.TempDir()
			calls := filepath.Join(tmp, "kubectl.calls")
			if err := os.WriteFile(filepath.Join(tmp, "ps"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			kubectl := `#!/bin/sh
printf '%s\n' "$*" >> "$KUBECTL_CALLS"
case "$*" in
  *"get namespaces kube-system"*) printf '%s' '{"metadata":{"uid":"system-uid"}}' ;;
  *"get pods --all-namespaces"*) printf '%s' '{"items":[]}' ;;
  *) exit 9 ;;
esac
`
			if err := os.WriteFile(filepath.Join(tmp, "kubectl"), []byte(kubectl), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", tmp)
			t.Setenv("KUBECTL_CALLS", calls)

			r := newResolver([]string{"cgroup-name"}, invocationConfig{
				logLevel: ndlpEmerg,
				kubernetes: kubernetesConfig{
					kubeConfig:    test.kubeConfig,
					kubeConfigSet: test.kubeConfigSet,
				},
			})
			result := r.k8sFetchPods(context.Background(), "k8s_get_kubepod_name", "")
			if result.outcome != kubePodSuccess || result.kubeSystemNamespace != `{"metadata":{"uid":"system-uid"}}` || result.pods != `{"items":[]}` {
				t.Fatalf("unexpected result: outcome=%d namespace=%q pods=%q", result.outcome, result.kubeSystemNamespace, result.pods)
			}
			data, err := os.ReadFile(calls)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Split(strings.TrimSpace(string(data)), "\n")
			want := []string{test.wantNamespace, test.wantPods}
			if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
				t.Fatalf("kubectl calls:\nwant %q\n got %q", want, got)
			}
		})
	}
}
