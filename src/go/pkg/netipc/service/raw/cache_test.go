//go:build unix

package raw

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// --- L3 Cache Tests ---

func TestCacheFullRoundTrip(t *testing.T) {
	svc := "go_cache_rt"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	cache := NewCache(testRunDir, svc, testClientConfig())
	if cache.Ready() {
		t.Fatal("should not be ready before refresh")
	}

	// Ensure monotonic epoch advances past 0ms before refresh
	time.Sleep(2 * time.Millisecond)

	// Refresh populates the cache
	updated := cache.Refresh()
	if !updated {
		t.Fatal("refresh should update the cache")
	}
	if !cache.Ready() {
		t.Fatal("should be ready after successful refresh")
	}

	// Lookup by hash + name
	item, found := cache.Lookup(1001, "docker-abc123")
	if !found {
		t.Fatal("item should be found")
	}
	if item.Hash != 1001 {
		t.Fatalf("expected hash=1001, got %d", item.Hash)
	}
	if item.Options != 0 {
		t.Fatalf("expected options=0, got %d", item.Options)
	}
	if item.Enabled != 1 {
		t.Fatalf("expected enabled=1, got %d", item.Enabled)
	}
	if item.Name != "docker-abc123" {
		t.Fatalf("expected name=docker-abc123, got %q", item.Name)
	}
	if item.Path != "/sys/fs/cgroup/docker/abc123" {
		t.Fatalf("expected path=/sys/fs/cgroup/docker/abc123, got %q", item.Path)
	}

	item2, found2 := cache.Lookup(3003, "systemd-user")
	if !found2 {
		t.Fatal("item 2 should be found")
	}
	if item2.Enabled != 0 {
		t.Fatalf("expected enabled=0, got %d", item2.Enabled)
	}

	// Status
	status := cache.Status()
	if !status.Populated {
		t.Fatal("status should be populated")
	}
	if status.ItemCount != 3 {
		t.Fatalf("expected item_count=3, got %d", status.ItemCount)
	}
	if status.SystemdEnabled != 1 {
		t.Fatalf("expected systemd_enabled=1, got %d", status.SystemdEnabled)
	}
	if status.Generation != 42 {
		t.Fatalf("expected generation=42, got %d", status.Generation)
	}
	if status.RefreshSuccessCount != 1 {
		t.Fatalf("expected success_count=1, got %d", status.RefreshSuccessCount)
	}
	if status.RefreshFailureCount != 0 {
		t.Fatalf("expected failure_count=0, got %d", status.RefreshFailureCount)
	}
	if status.ConnectionState != StateReady {
		t.Fatalf("expected connection_state=StateReady, got %d", status.ConnectionState)
	}
	if status.LastRefreshTs <= 0 {
		t.Fatalf("expected positive last_refresh_ts, got %d", status.LastRefreshTs)
	}

	cache.Close()
	cleanupAll(svc)
}

func TestCacheRefreshFailurePreserves(t *testing.T) {
	svc := "go_cache_preserve"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, testSnapshotDispatch())

	cache := NewCache(testRunDir, svc, testClientConfig())

	// First refresh populates cache
	if !cache.Refresh() {
		t.Fatal("first refresh should succeed")
	}
	if !cache.Ready() {
		t.Fatal("should be ready")
	}
	_, found := cache.Lookup(1001, "docker-abc123")
	if !found {
		t.Fatal("item should be found after first refresh")
	}

	// Kill server
	ts.stop()
	cleanupAll(svc)
	time.Sleep(50 * time.Millisecond)

	// Refresh fails, old cache preserved
	updated := cache.Refresh()
	if updated {
		t.Fatal("refresh should fail with no server")
	}
	if !cache.Ready() {
		t.Fatal("should still be ready (old cache preserved)")
	}
	_, found = cache.Lookup(1001, "docker-abc123")
	if !found {
		t.Fatal("item should still be found (old cache preserved)")
	}

	status := cache.Status()
	if status.RefreshSuccessCount != 1 {
		t.Fatalf("expected success_count=1, got %d", status.RefreshSuccessCount)
	}
	if status.RefreshFailureCount < 1 {
		t.Fatalf("expected failure_count >= 1, got %d", status.RefreshFailureCount)
	}

	cache.Close()
	cleanupAll(svc)
}

func TestCacheReconnectRebuilds(t *testing.T) {
	svc := "go_cache_reconn"
	ensureRunDir()
	cleanupAll(svc)

	ts1 := startTestServer(svc, testSnapshotDispatch())

	cache := NewCache(testRunDir, svc, testClientConfig())
	if !cache.Refresh() {
		t.Fatal("first refresh should succeed")
	}
	if cache.Status().ItemCount != 3 {
		t.Fatal("expected 3 items")
	}

	// Kill and restart server
	ts1.stop()
	cleanupAll(svc)
	time.Sleep(50 * time.Millisecond)

	ts2 := startTestServer(svc, testSnapshotDispatch())
	defer ts2.stop()

	// Refresh should reconnect and rebuild cache
	updated := cache.Refresh()
	if !updated {
		t.Fatal("refresh after reconnect should succeed")
	}
	if !cache.Ready() {
		t.Fatal("should be ready after reconnect")
	}
	if cache.Status().ItemCount != 3 {
		t.Fatal("expected 3 items after reconnect")
	}
	if cache.Status().RefreshSuccessCount != 2 {
		t.Fatalf("expected success_count=2, got %d", cache.Status().RefreshSuccessCount)
	}

	cache.Close()
	cleanupAll(svc)
}

func TestCacheLookupNotFound(t *testing.T) {
	svc := "go_cache_notfound"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	cache := NewCache(testRunDir, svc, testClientConfig())
	if !cache.Refresh() {
		t.Fatal("refresh should succeed")
	}

	// Non-existent hash
	_, found := cache.Lookup(9999, "nonexistent")
	if found {
		t.Fatal("should not find nonexistent item")
	}

	// Correct hash, wrong name
	_, found = cache.Lookup(1001, "wrong-name")
	if found {
		t.Fatal("should not find with wrong name")
	}

	// Correct name, wrong hash
	_, found = cache.Lookup(9999, "docker-abc123")
	if found {
		t.Fatal("should not find with wrong hash")
	}

	cache.Close()
	cleanupAll(svc)
}

func TestCacheEmpty(t *testing.T) {
	svc := "go_cache_empty"
	ensureRunDir()
	cleanupAll(svc)

	cache := NewCache(testRunDir, svc, testClientConfig())

	// Not ready before any refresh
	if cache.Ready() {
		t.Fatal("should not be ready")
	}

	// Lookup on empty cache returns not-found
	_, found := cache.Lookup(1001, "docker-abc123")
	if found {
		t.Fatal("should not find in empty cache")
	}

	status := cache.Status()
	if status.Populated {
		t.Fatal("should not be populated")
	}
	if status.ItemCount != 0 {
		t.Fatalf("expected item_count=0, got %d", status.ItemCount)
	}
	if status.RefreshSuccessCount != 0 {
		t.Fatalf("expected success_count=0, got %d", status.RefreshSuccessCount)
	}
	if status.RefreshFailureCount != 0 {
		t.Fatalf("expected failure_count=0, got %d", status.RefreshFailureCount)
	}

	cleanupAll(svc)
}

func TestCacheLargeDataset(t *testing.T) {
	svc := "go_cache_large"
	ensureRunDir()
	cleanupAll(svc)

	const N = 1000

	// Handler that builds N items
	largeHandler := SnapshotDispatch(
		func(request *protocol.CgroupsRequest, builder *protocol.CgroupsBuilder) bool {
			if request.LayoutVersion != 1 || request.Flags != 0 {
				return false
			}
			builder.SetHeader(1, 100)

			for i := uint32(0); i < N; i++ {
				name := fmt.Sprintf("cgroup-%d", i)
				path := fmt.Sprintf("/sys/fs/cgroup/test/%d", i)
				enabled := uint32(1)
				if i%3 == 0 {
					enabled = 0
				}
				if err := builder.Add(i+1000, 0, enabled,
					[]byte(name), []byte(path)); err != nil {
					return false
				}
			}
			return true
		},
		N,
	)

	cfg := testServerConfig()
	cfg.MaxResponsePayloadBytes = 256 * N

	s := NewServer(testRunDir, svc, cfg, protocol.MethodCgroupsSnapshot, largeHandler)
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		s.Run()
	}()
	time.Sleep(100 * time.Millisecond)

	defer func() {
		s.Stop()
		<-doneCh
	}()

	ccfg := testClientConfig()
	ccfg.MaxResponsePayloadBytes = 256 * N

	cache := NewCache(testRunDir, svc, ccfg)
	if !cache.Refresh() {
		t.Fatal("refresh should succeed")
	}
	if cache.Status().ItemCount != N {
		t.Fatalf("expected %d items, got %d", N, cache.Status().ItemCount)
	}

	// Verify all lookups
	for i := uint32(0); i < N; i++ {
		name := fmt.Sprintf("cgroup-%d", i)
		item, found := cache.Lookup(i+1000, name)
		if !found {
			t.Fatalf("item %d not found", i)
		}
		if item.Hash != i+1000 {
			t.Fatalf("item %d: expected hash=%d, got %d", i, i+1000, item.Hash)
		}
		expectedPath := fmt.Sprintf("/sys/fs/cgroup/test/%d", i)
		if item.Path != expectedPath {
			t.Fatalf("item %d: expected path=%q, got %q", i, expectedPath, item.Path)
		}
	}

	cache.Close()
	cleanupAll(svc)
}
