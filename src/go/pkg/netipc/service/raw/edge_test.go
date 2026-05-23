//go:build unix

package raw

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

// ---------------------------------------------------------------------------
//  Client.Refresh: state transitions
// ---------------------------------------------------------------------------

func TestClientRefreshFromReady(t *testing.T) {
	svc := "go_edge_ready"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer client.Close()

	client.Refresh() // DISCONNECTED -> READY

	// Refresh from READY should be a no-op
	changed := client.Refresh()
	if changed {
		t.Fatal("Refresh from READY should not change state")
	}
	if client.state != StateReady {
		t.Fatalf("expected READY, got %d", client.state)
	}

	cleanupAll(svc)
}

func TestClientRefreshFromAuthFailed(t *testing.T) {
	svc := "go_edge_auth"
	ensureRunDir()
	cleanupAll(svc)

	// Server with token=1
	sCfg := testServerConfig()
	sCfg.AuthToken = 0x1111
	s := NewServer(
		testRunDir,
		svc,
		sCfg,
		protocol.MethodCgroupsSnapshot,
		testSnapshotDispatch(),
	)
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

	// Client with wrong token
	cCfg := testClientConfig()
	cCfg.AuthToken = 0x2222
	client := NewSnapshotClient(testRunDir, svc, cCfg)
	defer client.Close()

	client.Refresh()
	if client.state != StateAuthFailed {
		t.Fatalf("expected StateAuthFailed, got %d", client.state)
	}

	// Refresh from AuthFailed should be a no-op
	changed := client.Refresh()
	if changed {
		t.Fatal("Refresh from StateAuthFailed should not change state")
	}

	cleanupAll(svc)
}

func TestClientRefreshFromIncompatible(t *testing.T) {
	svc := "go_edge_incompat"
	ensureRunDir()
	cleanupAll(svc)

	// Server supports only SHMFutex
	sCfg := testServerConfig()
	sCfg.SupportedProfiles = protocol.ProfileSHMFutex
	s := NewServer(
		testRunDir,
		svc,
		sCfg,
		protocol.MethodCgroupsSnapshot,
		testSnapshotDispatch(),
	)
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

	// Client supports only Baseline
	cCfg := testClientConfig()
	cCfg.SupportedProfiles = protocol.ProfileBaseline
	client := NewSnapshotClient(testRunDir, svc, cCfg)
	defer client.Close()

	client.Refresh()
	if client.state != StateIncompatible {
		t.Fatalf("expected StateIncompatible, got %d", client.state)
	}

	// Refresh from Incompatible should be a no-op
	changed := client.Refresh()
	if changed {
		t.Fatal("Refresh from StateIncompatible should not change state")
	}

	cleanupAll(svc)
}

func TestClientRefreshFromProtocolVersionMismatch(t *testing.T) {
	svc := uniqueUnixService("go_edge_proto_incompat")
	packet := encodeHelloAckPacketWithVersion(protocol.Version+1, protocol.StatusOK, 1)
	srv := startRawPosixHelloAckServer(t, svc, packet)

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer client.Close()

	changed := client.Refresh()
	if !changed {
		t.Fatal("Refresh should move client into StateIncompatible")
	}
	if client.state != StateIncompatible {
		t.Fatalf("expected StateIncompatible, got %d", client.state)
	}
	if client.Ready() {
		t.Fatal("client should not be ready after protocol version mismatch")
	}

	srv.wait(t)
	if srv.accepted.Load() != 1 {
		t.Fatalf("expected exactly one raw handshake attempt, got %d", srv.accepted.Load())
	}

	changed = client.Refresh()
	if changed {
		t.Fatal("Refresh from StateIncompatible should be a no-op after protocol mismatch")
	}
	if client.state != StateIncompatible {
		t.Fatalf("expected StateIncompatible after second refresh, got %d", client.state)
	}

	cleanupAll(svc)
}

func TestClientRefreshFromBroken(t *testing.T) {
	svc := "go_edge_broken"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, testSnapshotDispatch())

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer client.Close()

	client.Refresh()
	if !client.Ready() {
		t.Fatal("expected READY")
	}

	// Kill server to make connection broken
	ts.stop()
	cleanupAll(svc)
	time.Sleep(50 * time.Millisecond)

	// Force a call to break the connection
	_, _ = client.CallSnapshot()

	// At this point, state should be BROKEN
	if client.state != StateBroken {
		t.Logf("state is %d (may not be broken if retry succeeded; skipping)", client.state)
		return
	}

	// Start a new server
	ts2 := startTestServer(svc, testSnapshotDispatch())
	defer ts2.stop()

	// Refresh from BROKEN should reconnect
	changed := client.Refresh()
	if !changed {
		t.Fatal("Refresh from BROKEN should change state")
	}
	if client.state != StateReady {
		t.Fatalf("expected READY after reconnect, got %d", client.state)
	}
	if client.reconnectCount < 1 {
		t.Fatalf("expected reconnectCount >= 1, got %d", client.reconnectCount)
	}

	cleanupAll(svc)
}

// ---------------------------------------------------------------------------
//  Client.callWithRetry: not-ready fast-fail
// ---------------------------------------------------------------------------

func TestCallWithRetryNotReady(t *testing.T) {
	svc := "go_edge_notready"
	ensureRunDir()

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer client.Close()

	// Don't call Refresh - client is DISCONNECTED
	_, err := client.CallSnapshot()
	if err == nil {
		t.Fatal("expected error when client is not ready")
	}
	if client.errorCount != 1 {
		t.Fatalf("expected errorCount=1, got %d", client.errorCount)
	}
}

// ---------------------------------------------------------------------------
//  NewServerWithWorkers: workerCount < 1 defaults to 1
// ---------------------------------------------------------------------------

func TestNewServerWithWorkersMinimum(t *testing.T) {
	s := NewServerWithWorkers(
		"/tmp",
		"test",
		posix.ServerConfig{},
		protocol.MethodCgroupsSnapshot,
		testSnapshotDispatch(),
		0,
	)
	if s.workerCount != 1 {
		t.Fatalf("expected workerCount=1 for input 0, got %d", s.workerCount)
	}
	s2 := NewServerWithWorkers(
		"/tmp",
		"test",
		posix.ServerConfig{},
		protocol.MethodCgroupsSnapshot,
		testSnapshotDispatch(),
		-5,
	)
	if s2.workerCount != 1 {
		t.Fatalf("expected workerCount=1 for input -5, got %d", s2.workerCount)
	}
}

// ---------------------------------------------------------------------------
//  Cache: Lookup on empty (unpopulated) cache
// ---------------------------------------------------------------------------

func TestCacheLookupBeforeRefresh(t *testing.T) {
	cache := NewCache(testRunDir, "nonexistent", testClientConfig())
	defer cache.Close()

	if cache.Ready() {
		t.Fatal("should not be ready")
	}

	_, found := cache.Lookup(123, "anything")
	if found {
		t.Fatal("should not find items in empty cache")
	}
}

// ---------------------------------------------------------------------------
//  Cache: Refresh with no server running
// ---------------------------------------------------------------------------

func TestCacheRefreshNoServer(t *testing.T) {
	svc := "go_edge_cache_nosrv"
	ensureRunDir()
	cleanupAll(svc)

	cache := NewCache(testRunDir, svc, testClientConfig())
	defer cache.Close()

	updated := cache.Refresh()
	if updated {
		t.Fatal("refresh should fail with no server")
	}
	if cache.Ready() {
		t.Fatal("should not be ready after failed refresh")
	}

	status := cache.Status()
	if status.RefreshFailureCount != 1 {
		t.Fatalf("expected failure_count=1, got %d", status.RefreshFailureCount)
	}

	cleanupAll(svc)
}

// ---------------------------------------------------------------------------
//  Cache: Status on fresh cache
// ---------------------------------------------------------------------------

func TestCacheStatusFresh(t *testing.T) {
	cache := NewCache(testRunDir, "fresh", testClientConfig())
	defer cache.Close()

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
	if status.LastRefreshTs != 0 {
		t.Fatalf("expected last_refresh_ts=0, got %d", status.LastRefreshTs)
	}
}

// ---------------------------------------------------------------------------
//  Cache: Close resets state
// ---------------------------------------------------------------------------

func TestCacheCloseResetsState(t *testing.T) {
	svc := "go_edge_cache_close"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	cache := NewCache(testRunDir, svc, testClientConfig())
	if !cache.Refresh() {
		t.Fatal("refresh should succeed")
	}
	if !cache.Ready() {
		t.Fatal("should be ready")
	}

	cache.Close()

	if cache.Ready() {
		t.Fatal("should not be ready after close")
	}
	_, found := cache.Lookup(1001, "docker-abc123")
	if found {
		t.Fatal("should not find items after close")
	}

	cleanupAll(svc)
}

// ---------------------------------------------------------------------------
//  Cache: NewCache with custom MaxResponsePayloadBytes
// ---------------------------------------------------------------------------

func TestCacheCustomBufferSize(t *testing.T) {
	cfg := testClientConfig()
	cfg.MaxResponsePayloadBytes = 1024

	cache := NewCache(testRunDir, "custom", cfg)
	defer cache.Close()

	expectedBufSize := protocol.HeaderSize + 1024
	if got := cache.client.maxReceiveMessageBytes(); got != expectedBufSize {
		t.Fatalf("expected response buf size=%d, got %d", expectedBufSize, got)
	}
}

func TestCacheDefaultBufferSize(t *testing.T) {
	cfg := testClientConfig()
	cfg.MaxResponsePayloadBytes = 0

	cache := NewCache(testRunDir, "default", cfg)
	defer cache.Close()

	expectedBufSize := protocol.HeaderSize + cacheResponseBufSize
	if got := cache.client.maxReceiveMessageBytes(); got != expectedBufSize {
		t.Fatalf("expected response buf size=%d, got %d", expectedBufSize, got)
	}
}

// ---------------------------------------------------------------------------
//  Cache: Lookup miss with correct hash but wrong name, and vice versa
// ---------------------------------------------------------------------------

func TestCacheLookupHashNameMismatch(t *testing.T) {
	svc := "go_edge_cache_mismatch"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	cache := NewCache(testRunDir, svc, testClientConfig())
	defer cache.Close()

	if !cache.Refresh() {
		t.Fatal("refresh should succeed")
	}

	// Correct hash (1001), wrong name
	_, found := cache.Lookup(1001, "wrong-name")
	if found {
		t.Fatal("should not find with wrong name")
	}

	// Wrong hash, correct name
	_, found = cache.Lookup(9999, "docker-abc123")
	if found {
		t.Fatal("should not find with wrong hash")
	}

	// Both correct
	item, found := cache.Lookup(1001, "docker-abc123")
	if !found {
		t.Fatal("should find with correct hash+name")
	}
	if item.Hash != 1001 {
		t.Fatalf("expected hash=1001, got %d", item.Hash)
	}

	cleanupAll(svc)
}

// ---------------------------------------------------------------------------
//  CallIncrementBatch: empty slice
// ---------------------------------------------------------------------------

func TestCallIncrementBatchEmpty(t *testing.T) {
	svc := "go_edge_batch_empty"
	ensureRunDir()
	cleanupAll(svc)

	client := NewIncrementClient(testRunDir, svc, testClientConfig())
	defer client.Close()

	results, err := client.CallIncrementBatch(nil)
	if err != nil {
		t.Fatalf("expected nil error for empty batch, got %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results for empty batch, got %v", results)
	}

	results2, err := client.CallIncrementBatch([]uint64{})
	if err != nil {
		t.Fatalf("expected nil error for empty slice, got %v", err)
	}
	if results2 != nil {
		t.Fatalf("expected nil results for empty slice, got %v", results2)
	}

	cleanupAll(svc)
}

// ---------------------------------------------------------------------------
//  Client error classification helpers
// ---------------------------------------------------------------------------

func TestIsConnectError(t *testing.T) {
	if !isConnectError(posix.ErrConnect) {
		t.Error("should match ErrConnect")
	}
	if !isConnectError(posix.ErrSocket) {
		t.Error("should match ErrSocket")
	}
	if isConnectError(posix.ErrAuthFailed) {
		t.Error("should not match ErrAuthFailed")
	}
	if isConnectError(posix.ErrNoProfile) {
		t.Error("should not match ErrNoProfile")
	}
	if isConnectError(nil) {
		t.Error("should not match nil")
	}
}

func TestIsAuthError(t *testing.T) {
	if !isAuthError(posix.ErrAuthFailed) {
		t.Error("should match ErrAuthFailed")
	}
	if isAuthError(posix.ErrConnect) {
		t.Error("should not match ErrConnect")
	}
	if isAuthError(nil) {
		t.Error("should not match nil")
	}
}

func TestIsIncompatibleError(t *testing.T) {
	if !isIncompatibleError(posix.ErrNoProfile) {
		t.Error("should match ErrNoProfile")
	}
	if !isIncompatibleError(posix.ErrIncompatible) {
		t.Error("should match ErrIncompatible")
	}
	if isIncompatibleError(posix.ErrAuthFailed) {
		t.Error("should not match ErrAuthFailed")
	}
	if isIncompatibleError(nil) {
		t.Error("should not match nil")
	}
}

// ---------------------------------------------------------------------------
//  Client.Status on fresh client
// ---------------------------------------------------------------------------

func TestClientStatusFresh(t *testing.T) {
	client := NewSnapshotClient(testRunDir, "nosvc", testClientConfig())
	defer client.Close()

	status := client.Status()
	if status.State != StateDisconnected {
		t.Fatalf("expected DISCONNECTED, got %d", status.State)
	}
	if status.ConnectCount != 0 {
		t.Fatalf("expected connect_count=0, got %d", status.ConnectCount)
	}
	if status.CallCount != 0 {
		t.Fatalf("expected call_count=0, got %d", status.CallCount)
	}
	if status.ErrorCount != 0 {
		t.Fatalf("expected error_count=0, got %d", status.ErrorCount)
	}
}
