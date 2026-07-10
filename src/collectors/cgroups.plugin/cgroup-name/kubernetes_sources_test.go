// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

func TestK8sGCPRequestPolicyAndCancellation(t *testing.T) {
	r := newResolver([]string{"cgroup-name"}, invocationConfig{
		logLevel: ndlpEmerg,
		kubernetes: kubernetesConfig{
			gcpMetadataURL: "http://metadata.invalid",
		},
	})
	type call struct {
		url     string
		options httpGetOptions
		err     error
	}
	var mu sync.Mutex
	var calls []call
	get := func(ctx context.Context, url string, options httpGetOptions) ([]byte, error) {
		if strings.HasSuffix(url, "/project/project-id") {
			err := errors.New("fixture failure")
			mu.Lock()
			calls = append(calls, call{url: url, options: options, err: err})
			mu.Unlock()
			return nil, err
		}
		<-ctx.Done()
		mu.Lock()
		calls = append(calls, call{url: url, options: options, err: ctx.Err()})
		mu.Unlock()
		return nil, ctx.Err()
	}
	if name, ok := r.k8sGCPGetClusterNameWith(context.Background(), get); ok || name != "" {
		t.Fatalf("name=%q ok=%v, want failed lookup", name, ok)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("calls = %d, want 3", len(calls))
	}
	for _, call := range calls {
		if !call.options.noProxy || !call.options.fail || call.options.timeout != 3*time.Second {
			t.Fatalf("request policy for %s: noProxy=%v fail=%v timeout=%s", call.url, call.options.noProxy, call.options.fail, call.options.timeout)
		}
		if got := call.options.headers["Metadata-Flavor"]; got != "Google" {
			t.Fatalf("Metadata-Flavor for %s = %q", call.url, got)
		}
		if call.err == nil {
			t.Fatalf("call %s unexpectedly succeeded", call.url)
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

func TestKubectlOutputIsCapped(t *testing.T) {
	tmp := t.TempDir()
	kubectl := filepath.Join(tmp, "kubectl")
	if err := os.WriteFile(kubectl, []byte("#!/bin/sh\nprintf 12345\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)
	r := newResolver([]string{"cgroup-name"}, invocationConfig{logLevel: ndlpEmerg})
	output, err := r.kubectlOutput(context.Background(), "", 4, "get", "pods")
	if err == nil || !strings.Contains(err.Error(), "kubectl output exceeds 4 bytes") {
		t.Fatalf("cap error = %v", err)
	}
	if got, want := string(output), "1234"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
