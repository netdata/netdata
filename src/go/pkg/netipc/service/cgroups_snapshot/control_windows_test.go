//go:build windows

package cgroups_snapshot

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func TestClientAndCacheControlMethodsWindows(t *testing.T) {
	service := uniqueWinService("control")

	client := NewClient(testWinRunDir, service, testWinClientConfig())
	defer client.Close()

	client.SetCallTimeout(25)
	client.Abort()
	client.ClearAbort()
	if client.Ready() {
		t.Fatal("client unexpectedly ready without a server")
	}
	client.Refresh()
	status := client.Status()
	if status.State != StateNotFound {
		t.Fatalf("client state = %v, want StateNotFound", status.State)
	}
	if _, err := client.CallSnapshotWithTimeout(1); err != protocol.ErrBadLayout {
		t.Fatalf("CallSnapshotWithTimeout without READY = %v, want ErrBadLayout", err)
	}

	cache := NewCache(testWinRunDir, service, testWinClientConfig())
	defer cache.Close()

	cache.SetCallTimeout(25)
	cache.Abort()
	cache.ClearAbort()
	if cache.Refresh() {
		t.Fatal("cache refresh unexpectedly succeeded without a server")
	}
	if cache.Ready() {
		t.Fatal("cache unexpectedly ready without a successful refresh")
	}
	guard := cache.ReadLock()
	if item := guard.Get(123, "missing"); item != nil {
		guard.Unlock()
		t.Fatalf("cache lookup without refresh = %+v, want nil", item)
	}
	guard.Unlock()
	cacheStatus := cache.Status()
	if cacheStatus.Populated || cacheStatus.RefreshFailureCount != 1 ||
		cacheStatus.ConnectionState != StateNotFound {
		t.Fatalf("cache status = %+v, want one failed refresh and StateNotFound", cacheStatus)
	}
}
