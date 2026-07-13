// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestLoadKubernetesMetadataRefreshesUnknownClusterName(t *testing.T) {
	const containerID = "abcdef"
	tests := map[string]struct {
		cacheAge      time.Duration
		gcpValues     bool
		wantCluster   string
		wantCached    string
		minCalls      int32
		maxCalls      int32
		wantTimestamp bool
	}{
		"fresh negative cache is reused": {
			gcpValues:   true,
			wantCluster: "unknown",
			wantCached:  "unknown",
		},
		"expired negative cache is replaced on success": {
			cacheAge:      10 * time.Minute,
			gcpValues:     true,
			wantCluster:   "gke_project-a_region-a_cluster-a",
			wantCached:    "gke_project-a_region-a_cluster-a",
			minCalls:      3,
			maxCalls:      3,
			wantTimestamp: true,
		},
		"expired negative cache is refreshed on failure": {
			cacheAge:      10 * time.Minute,
			wantCluster:   "unknown",
			wantCached:    "unknown",
			minCalls:      1,
			maxCalls:      3,
			wantTimestamp: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				if !test.gcpValues {
					return
				}
				values := map[string]string{
					"/computeMetadata/v1/project/project-id":                   "project-a",
					"/computeMetadata/v1/instance/attributes/cluster-location": "region-a",
					"/computeMetadata/v1/instance/attributes/cluster-name":     "cluster-a",
				}
				_, _ = w.Write([]byte(values[request.URL.Path]))
			}))
			defer server.Close()

			tmp := t.TempDir()
			cache := mustNewKubernetesCache(t, tmp)
			if err := cache.writeClusterName("unknown"); err != nil {
				t.Fatal(err)
			}
			if err := cache.writeSystemUID("system-uid"); err != nil {
				t.Fatal(err)
			}
			if err := cache.writeContainers([]labelSet{{items: []label{
				{name: "namespace", value: "default"},
				{name: "pod_name", value: "api"},
				{name: "pod_uid", value: "pod-uid"},
				{name: "container_name", value: "app"},
				{name: "container_id", value: containerID},
			}}}); err != nil {
				t.Fatal(err)
			}
			clusterPath := filepath.Join(tmp, "netdata-cgroups-k8s-cluster-name")
			writtenAt := time.Now()
			if test.cacheAge > 0 {
				writtenAt = time.Now().Add(-test.cacheAge)
				if err := os.Chtimes(clusterPath, writtenAt, writtenAt); err != nil {
					t.Fatal(err)
				}
			}

			r := newResolver([]string{"cgroup-name"}, invocationConfig{
				kubernetesCacheDir: tmp,
				logLevel:           ndlpEmerg,
				kubernetes: kubernetesConfig{
					gcpMetadataURL: server.URL,
				},
			})
			metadata, outcome := r.loadKubernetesMetadata(context.Background(), "k8s_get_kubepod_name", containerID)
			if outcome != kubePodSuccess {
				t.Fatalf("outcome = %d, want success", outcome)
			}
			if metadata.clusterName != test.wantCluster {
				t.Fatalf("cluster name = %q, want %q", metadata.clusterName, test.wantCluster)
			}
			if got := cache.clusterName(); got != test.wantCached {
				t.Fatalf("cached cluster name = %q, want %q", got, test.wantCached)
			}
			if got := calls.Load(); got < test.minCalls || got > test.maxCalls {
				t.Fatalf("GCP calls = %d, want range [%d,%d]", got, test.minCalls, test.maxCalls)
			}
			if test.wantTimestamp {
				info, err := os.Stat(clusterPath)
				if err != nil {
					t.Fatal(err)
				}
				if !info.ModTime().After(writtenAt) {
					t.Fatalf("negative-cache timestamp was not refreshed: old=%s new=%s", writtenAt, info.ModTime())
				}
			}
		})
	}
}
