//go:build unix

package raw

import (
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

const (
	testRunDir      = "/tmp/nipc_svc_go_test"
	authToken       = uint64(0xDEADBEEFCAFEBABE)
	responseBufSize = 65536
)

func ensureRunDir() {
	os.MkdirAll(testRunDir, 0700)
}

func cleanupAll(service string) {
	os.Remove(testRunDir + "/" + service + ".sock")
	posix.ShmCleanupStale(testRunDir, service)
}

func waitUnixSocketReady(service string) {
	path := testRunDir + "/" + service + ".sock"
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitUnixServerReady(service string) {
	waitUnixSocketReady(service)

	cfg := testClientConfig()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		session, err := posix.Connect(testRunDir, service, &cfg)
		if err == nil {
			session.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitUnixRawSessionConnect(service string, cfg posix.ClientConfig) (*posix.Session, error) {
	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		session, err := posix.Connect(testRunDir, service, &cfg)
		if err == nil {
			return session, nil
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	return nil, lastErr
}

func testServerConfig() posix.ServerConfig {
	return posix.ServerConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		MaxRequestPayloadBytes:  4096,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: responseBufSize,
		MaxResponseBatchItems:   1,
		AuthToken:               authToken,
		Backlog:                 4,
	}
}

func testClientConfig() posix.ClientConfig {
	return posix.ClientConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		MaxRequestPayloadBytes:  4096,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: responseBufSize,
		MaxResponseBatchItems:   1,
		AuthToken:               authToken,
	}
}

// testCgroupsHandler builds a snapshot with 3 test items.
func testCgroupsHandler(request *protocol.CgroupsRequest, builder *protocol.CgroupsBuilder) bool {
	if request.LayoutVersion != 1 || request.Flags != 0 {
		return false
	}

	builder.SetHeader(1, 42)

	items := []struct {
		hash, options, enabled uint32
		name, path             []byte
	}{
		{1001, 0, 1, []byte("docker-abc123"), []byte("/sys/fs/cgroup/docker/abc123")},
		{2002, 0, 1, []byte("k8s-pod-xyz"), []byte("/sys/fs/cgroup/kubepods/xyz")},
		{3003, 0, 0, []byte("systemd-user"), []byte("/sys/fs/cgroup/user.slice/user-1000")},
	}

	for _, item := range items {
		if err := builder.Add(item.hash, item.options, item.enabled, item.name, item.path); err != nil {
			return false
		}
	}

	return true
}

// failingHandler always fails.
func failingHandler(*protocol.CgroupsRequest, *protocol.CgroupsBuilder) bool {
	return false
}

func testSnapshotDispatch() DispatchHandler {
	return SnapshotDispatch(testCgroupsHandler, 3)
}

func failingSnapshotDispatch() DispatchHandler {
	return SnapshotDispatch(failingHandler, 3)
}

type testServer struct {
	server *Server
	doneCh chan struct{}
}

func startTestServer(service string, handler DispatchHandler) *testServer {
	ensureRunDir()
	cleanupAll(service)

	s := NewServer(testRunDir, service, testServerConfig(), protocol.MethodCgroupsSnapshot, handler)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		s.Run()
	}()

	// Wait until the server is actually accepting sessions.
	waitUnixServerReady(service)

	return &testServer{server: s, doneCh: doneCh}
}

func (ts *testServer) stop() {
	ts.server.Stop()
	<-ts.doneCh
}

func TestClientLifecycle(t *testing.T) {
	svc := "go_svc_lifecycle"
	ensureRunDir()
	cleanupAll(svc)

	// Init without server running
	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	if client.state != StateDisconnected {
		t.Fatal("expected DISCONNECTED")
	}
	if client.Ready() {
		t.Fatal("should not be ready")
	}

	// Refresh without server -> NOT_FOUND
	changed := client.Refresh()
	if !changed {
		t.Fatal("state should have changed")
	}
	if client.state != StateNotFound {
		t.Fatalf("expected NOT_FOUND, got %d", client.state)
	}

	// Start server
	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	// Refresh -> READY
	changed = client.Refresh()
	if !changed {
		t.Fatal("state should have changed")
	}
	if client.state != StateReady {
		t.Fatalf("expected READY, got %d", client.state)
	}
	if !client.Ready() {
		t.Fatal("should be ready")
	}

	// Status reporting
	status := client.Status()
	if status.ConnectCount != 1 {
		t.Fatalf("expected connect_count=1, got %d", status.ConnectCount)
	}
	if status.ReconnectCount != 0 {
		t.Fatalf("expected reconnect_count=0, got %d", status.ReconnectCount)
	}

	// Close
	client.Close()
	if client.state != StateDisconnected {
		t.Fatal("expected DISCONNECTED after close")
	}
	if client.Ready() {
		t.Fatal("should not be ready after close")
	}

	cleanupAll(svc)
}

func TestCgroupsCall(t *testing.T) {
	svc := "go_svc_cgroups"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	view, err := client.CallSnapshot()
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}

	if view.ItemCount != 3 {
		t.Fatalf("expected 3 items, got %d", view.ItemCount)
	}
	if view.SystemdEnabled != 1 {
		t.Fatalf("expected systemd_enabled=1, got %d", view.SystemdEnabled)
	}
	if view.Generation != 42 {
		t.Fatalf("expected generation=42, got %d", view.Generation)
	}

	// Verify first item
	item0, err := view.Item(0)
	if err != nil {
		t.Fatalf("item 0 error: %v", err)
	}
	if item0.Hash != 1001 {
		t.Fatalf("item 0 hash: got %d", item0.Hash)
	}
	if item0.Enabled != 1 {
		t.Fatalf("item 0 enabled: got %d", item0.Enabled)
	}
	if item0.Name.String() != "docker-abc123" {
		t.Fatalf("item 0 name: got %q", item0.Name.String())
	}
	if item0.Path.String() != "/sys/fs/cgroup/docker/abc123" {
		t.Fatalf("item 0 path: got %q", item0.Path.String())
	}

	// Verify third item
	item2, err := view.Item(2)
	if err != nil {
		t.Fatalf("item 2 error: %v", err)
	}
	if item2.Hash != 3003 {
		t.Fatalf("item 2 hash: got %d", item2.Hash)
	}
	if item2.Enabled != 0 {
		t.Fatalf("item 2 enabled: got %d", item2.Enabled)
	}
	if item2.Name.String() != "systemd-user" {
		t.Fatalf("item 2 name: got %q", item2.Name.String())
	}

	// Verify stats
	status := client.Status()
	if status.CallCount != 1 {
		t.Fatalf("expected call_count=1, got %d", status.CallCount)
	}
	if status.ErrorCount != 0 {
		t.Fatalf("expected error_count=0, got %d", status.ErrorCount)
	}

	client.Close()
	cleanupAll(svc)
}

func TestRetryOnFailure(t *testing.T) {
	svc := "go_svc_retry"
	ensureRunDir()
	cleanupAll(svc)

	ts1 := startTestServer(svc, testSnapshotDispatch())

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	// First call succeeds
	view, err := client.CallSnapshot()
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if view.ItemCount != 3 {
		t.Fatalf("expected 3 items, got %d", view.ItemCount)
	}

	// Kill server
	ts1.stop()
	cleanupAll(svc)
	time.Sleep(50 * time.Millisecond)

	// Restart server
	ts2 := startTestServer(svc, testSnapshotDispatch())
	defer ts2.stop()

	// Next call triggers reconnect + retry
	view2, err := client.CallSnapshot()
	if err != nil {
		t.Fatalf("retry call failed: %v", err)
	}
	if view2.ItemCount != 3 {
		t.Fatalf("expected 3 items after retry, got %d", view2.ItemCount)
	}

	// Verify reconnect happened
	status := client.Status()
	if status.ReconnectCount < 1 {
		t.Fatalf("expected reconnect_count >= 1, got %d", status.ReconnectCount)
	}

	client.Close()
	cleanupAll(svc)
}

func TestMultipleClients(t *testing.T) {
	svc := "go_svc_multi"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	// Client 1
	client1 := NewSnapshotClient(testRunDir, svc, testClientConfig())
	client1.Refresh()
	if !client1.Ready() {
		t.Fatal("client 1 not ready")
	}

	view1, err := client1.CallSnapshot()
	if err != nil {
		t.Fatalf("client 1 call failed: %v", err)
	}
	if view1.ItemCount != 3 {
		t.Fatalf("client 1: expected 3 items, got %d", view1.ItemCount)
	}

	// Now multi-client: keep client 1 open, connect client 2
	client2 := NewSnapshotClient(testRunDir, svc, testClientConfig())
	client2.Refresh()
	if !client2.Ready() {
		t.Fatal("client 2 not ready")
	}

	view2, err := client2.CallSnapshot()
	if err != nil {
		t.Fatalf("client 2 call failed: %v", err)
	}
	if view2.ItemCount != 3 {
		t.Fatalf("client 2: expected 3 items, got %d", view2.ItemCount)
	}

	client1.Close()
	client2.Close()
	cleanupAll(svc)
}

func TestConcurrentClients(t *testing.T) {
	svc := "go_svc_concurrent"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	const numClients = 5
	const requestsPerClient = 10

	type result struct {
		successes int
		failures  int
	}

	results := make(chan result, numClients)

	for i := 0; i < numClients; i++ {
		go func() {
			r := result{}
			client := NewSnapshotClient(testRunDir, svc, testClientConfig())
			defer client.Close()

			for retry := 0; retry < 100; retry++ {
				client.Refresh()
				if client.Ready() {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}

			if !client.Ready() {
				r.failures = requestsPerClient
				results <- r
				return
			}

			for j := 0; j < requestsPerClient; j++ {
				view, err := client.CallSnapshot()
				if err != nil || view.ItemCount != 3 {
					r.failures++
					continue
				}
				// Verify content
				item0, err := view.Item(0)
				if err != nil || item0.Hash != 1001 || item0.Name.String() != "docker-abc123" {
					r.failures++
					continue
				}
				r.successes++
			}
			results <- r
		}()
	}

	totalSuccess := 0
	totalFailure := 0
	for i := 0; i < numClients; i++ {
		r := <-results
		totalSuccess += r.successes
		totalFailure += r.failures
	}

	expected := numClients * requestsPerClient
	if totalSuccess != expected {
		t.Fatalf("expected %d successes, got %d (failures: %d)", expected, totalSuccess, totalFailure)
	}
	if totalFailure != 0 {
		t.Fatalf("expected 0 failures, got %d", totalFailure)
	}

	cleanupAll(svc)
}

func TestHandlerFailure(t *testing.T) {
	svc := "go_svc_hfail"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, failingSnapshotDispatch())
	defer ts.stop()

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	_, err := client.CallSnapshot()
	if err == nil {
		t.Fatal("expected error when handler fails")
	}

	status := client.Status()
	if status.ErrorCount < 1 {
		t.Fatalf("expected error_count >= 1, got %d", status.ErrorCount)
	}

	client.Close()
	cleanupAll(svc)
}

func TestStatusReporting(t *testing.T) {
	svc := "go_svc_status"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	// Initial counters
	s0 := client.Status()
	if s0.ConnectCount != 1 {
		t.Fatalf("expected connect_count=1, got %d", s0.ConnectCount)
	}
	if s0.CallCount != 0 {
		t.Fatalf("expected call_count=0, got %d", s0.CallCount)
	}
	if s0.ErrorCount != 0 {
		t.Fatalf("expected error_count=0, got %d", s0.ErrorCount)
	}

	// Make 3 successful calls
	for i := 0; i < 3; i++ {
		_, err := client.CallSnapshot()
		if err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}

	s1 := client.Status()
	if s1.CallCount != 3 {
		t.Fatalf("expected call_count=3, got %d", s1.CallCount)
	}
	if s1.ErrorCount != 0 {
		t.Fatalf("expected error_count=0, got %d", s1.ErrorCount)
	}

	// Call on disconnected client
	client.Close()
	_, err := client.CallSnapshot()
	if err == nil {
		t.Fatal("expected error on disconnected client")
	}

	s2 := client.Status()
	if s2.ErrorCount != 1 {
		t.Fatalf("expected error_count=1, got %d", s2.ErrorCount)
	}

	cleanupAll(svc)
}

func TestNonRequestTerminatesSession(t *testing.T) {
	svc := "go_svc_nonreq"
	ensureRunDir()
	cleanupAll(svc)

	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	// Connect via raw UDS session (transport level)
	session, err := waitUnixRawSessionConnect(svc, posix.ClientConfig{
		SupportedProfiles:       protocol.ProfileBaseline,
		MaxRequestPayloadBytes:  4096,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: responseBufSize,
		MaxResponseBatchItems:   1,
		AuthToken:               authToken,
	})
	if err != nil {
		t.Fatalf("raw connect failed: %v", err)
	}

	// Send a RESPONSE message (not REQUEST) - protocol violation
	badHdr := &protocol.Header{
		Kind:            protocol.KindResponse, // wrong kind!
		Code:            protocol.MethodCgroupsSnapshot,
		Flags:           0,
		ItemCount:       0,
		MessageID:       1,
		TransportStatus: protocol.StatusOK,
	}
	sendErr := session.Send(badHdr, nil)
	if sendErr != nil {
		// Send might fail immediately; that's acceptable
		t.Logf("send non-request failed immediately: %v", sendErr)
	} else {
		// Wait for server to process and terminate the session
		time.Sleep(200 * time.Millisecond)

		// Try to send a valid request and receive - should fail
		reqHdr := &protocol.Header{
			Kind:            protocol.KindRequest,
			Code:            protocol.MethodCgroupsSnapshot,
			Flags:           0,
			ItemCount:       1,
			MessageID:       2,
			TransportStatus: protocol.StatusOK,
		}
		var reqBuf [4]byte
		req := protocol.CgroupsRequest{LayoutVersion: 1, Flags: 0}
		req.Encode(reqBuf[:])

		_ = session.Send(reqHdr, reqBuf[:])

		recvBuf := make([]byte, 4096)
		_, _, recvErr := session.Receive(recvBuf)
		if recvErr == nil {
			t.Fatal("recv after non-request should fail (server terminated session)")
		}
	}
	session.Close()

	// Verify server is still alive: connect a new client and do a normal call
	verifyClient := NewSnapshotClient(testRunDir, svc, testClientConfig())
	refreshUnixClientReady(t, verifyClient)

	view, err := verifyClient.CallSnapshot()
	if err != nil {
		t.Fatalf("normal call should succeed after bad client: %v", err)
	}
	if view.ItemCount != 3 {
		t.Fatalf("expected 3 items, got %d", view.ItemCount)
	}

	verifyClient.Close()
	cleanupAll(svc)
}
