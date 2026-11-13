//go:build unix

package raw

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

var unixServiceCounter atomic.Uint64

func uniqueUnixService(prefix string) string {
	return fmt.Sprintf("%s_%d", prefix, unixServiceCounter.Add(1))
}

type unixRawSessionServer struct {
	doneCh chan error
}

type unixHelloAckServer struct {
	doneCh   chan error
	accepted atomic.Uint32
}

func encodeHelloAckPacketWithVersion(version uint16, status uint16, layoutVersion uint16) []byte {
	ack := protocol.HelloAck{
		LayoutVersion:                 layoutVersion,
		Flags:                         0,
		ServerSupportedProfiles:       protocol.ProfileBaseline,
		IntersectionProfiles:          protocol.ProfileBaseline,
		SelectedProfile:               protocol.ProfileBaseline,
		AgreedMaxRequestPayloadBytes:  protocol.MaxPayloadDefault,
		AgreedMaxRequestBatchItems:    1,
		AgreedMaxResponsePayloadBytes: responseBufSize,
		AgreedMaxResponseBatchItems:   1,
		AgreedPacketSize:              0,
		SessionID:                     77,
	}

	payload := make([]byte, 48)
	n := ack.Encode(payload)
	payload = payload[:n]

	hdr := protocol.Header{
		Magic:           protocol.MagicMsg,
		Version:         version,
		HeaderLen:       protocol.HeaderSize,
		Kind:            protocol.KindControl,
		Code:            protocol.CodeHelloAck,
		TransportStatus: status,
		PayloadLen:      uint32(len(payload)),
		ItemCount:       1,
		MessageID:       0,
	}

	pkt := make([]byte, protocol.HeaderSize+len(payload))
	hdr.Encode(pkt[:protocol.HeaderSize])
	copy(pkt[protocol.HeaderSize:], payload)
	return pkt
}

func startRawPosixHelloAckServer(t *testing.T, service string, packet []byte) *unixHelloAckServer {
	t.Helper()

	ensureRunDir()
	cleanupAll(service)

	path := filepath.Join(testRunDir, service+".sock")
	_ = os.Remove(path)

	fd, err := syscall.Socket(syscall.AF_UNIX, testSockType, 0)
	if err != nil {
		t.Fatalf("socket failed: %v", err)
	}

	addr := &syscall.SockaddrUnix{Name: path}
	if err := syscall.Bind(fd, addr); err != nil {
		_ = syscall.Close(fd)
		t.Fatalf("bind failed: %v", err)
	}
	if err := syscall.Listen(fd, 4); err != nil {
		_ = syscall.Close(fd)
		t.Fatalf("listen failed: %v", err)
	}

	srv := &unixHelloAckServer{doneCh: make(chan error, 1)}
	go func() {
		defer close(srv.doneCh)
		defer syscall.Close(fd)

		connFD, _, err := syscall.Accept(fd)
		if err != nil {
			srv.doneCh <- err
			return
		}
		srv.accepted.Store(1)
		defer syscall.Close(connFD)

		buf := make([]byte, protocol.HeaderSize+128)
		if _, err := syscall.Read(connFD, buf); err != nil {
			srv.doneCh <- err
			return
		}
		if _, err := syscall.Write(connFD, packet); err != nil {
			srv.doneCh <- err
			return
		}

		srv.doneCh <- nil
	}()

	return srv
}

func (s *unixHelloAckServer) wait(t *testing.T) {
	t.Helper()
	if err := <-s.doneCh; err != nil {
		t.Fatalf("raw unix hello-ack server failed: %v", err)
	}
}

func startRawPosixSessionServerN(
	t *testing.T,
	service string,
	cfg posix.ServerConfig,
	accepts int,
	handler func(*posix.Session, protocol.Header, []byte) error,
) *unixRawSessionServer {
	t.Helper()
	ensureRunDir()
	cleanupAll(service)

	listener, err := posix.Listen(testRunDir, service, cfg)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	srv := &unixRawSessionServer{doneCh: make(chan error, 1)}
	go func() {
		defer listener.Close()

		for range accepts {
			session, err := listener.Accept()
			if err != nil {
				srv.doneCh <- err
				return
			}

			recvBuf := make([]byte, protocol.HeaderSize+int(cfg.MaxRequestPayloadBytes))
			hdr, payload, err := session.Receive(recvBuf)
			if err != nil {
				session.Close()
				srv.doneCh <- err
				return
			}

			if err := handler(session, hdr, payload); err != nil {
				session.Close()
				srv.doneCh <- err
				return
			}

			session.Close()
		}

		srv.doneCh <- nil
	}()

	return srv
}

func startRawPosixSessionServer(
	t *testing.T,
	service string,
	cfg posix.ServerConfig,
	handler func(*posix.Session, protocol.Header, []byte) error,
) *unixRawSessionServer {
	t.Helper()
	return startRawPosixSessionServerN(t, service, cfg, 1, handler)
}

func (s *unixRawSessionServer) wait(t *testing.T) {
	t.Helper()
	if err := <-s.doneCh; err != nil {
		t.Fatalf("raw unix session server failed: %v", err)
	}
}

// refreshUnixClientReady refreshes once: servers in these tests are already
// listening, so a single connect must succeed.
func refreshUnixClientReady(t *testing.T, client *Client) {
	t.Helper()
	client.Refresh()
	if !client.Ready() {
		t.Fatalf("client not ready after one refresh, state=%d", client.state)
	}
}

func TestUnixClientCallTimeoutOnWedgedPeer(t *testing.T) {
	cfg := testServerConfig()
	svc := uniqueUnixService("go_unix_call_timeout")
	// The peer holds the request unanswered until the client has timed out.
	timedOut := make(chan struct{})
	srv := startRawPosixSessionServer(t, svc, cfg,
		func(session *posix.Session, hdr protocol.Header, payload []byte) error {
			_ = session
			_ = hdr
			_ = payload
			<-timedOut
			return nil
		})

	client := NewIncrementClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	refreshUnixClientReady(t, client)

	start := time.Now()
	_, err := client.CallIncrementWithTimeout(41, 30)
	elapsed := time.Since(start)
	close(timedOut)
	if !errors.Is(err, protocol.ErrTimeout) {
		t.Fatalf("CallIncrementWithTimeout error = %v, want %v", err, protocol.ErrTimeout)
	}
	if elapsed > time.Second {
		t.Fatalf("timeout took too long: %s", elapsed)
	}

	srv.wait(t)
	cleanupAll(svc)
}

func TestUnixRawCallRecordsLimitExceededCapacity(t *testing.T) {
	cfg := testServerConfig()
	svc := uniqueUnixService("go_unix_limit_exceeded")
	srv := startRawPosixSessionServer(t, svc, cfg,
		func(session *posix.Session, hdr protocol.Header, payload []byte) error {
			_ = payload
			respHdr := protocol.Header{
				Kind:            protocol.KindResponse,
				Code:            hdr.Code,
				MessageID:       hdr.MessageID,
				TransportStatus: protocol.StatusLimitExceeded,
			}
			return session.Send(&respHdr, nil)
		})

	client := NewIncrementClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	refreshUnixClientReady(t, client)

	var reqBuf [protocol.IncrementPayloadSize]byte
	if protocol.IncrementEncode(41, reqBuf[:]) == 0 {
		t.Fatal("IncrementEncode failed")
	}

	_, _, err := client.doRawCallWithTimeout(protocol.MethodIncrement, reqBuf[:], 1000)
	if !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("doRawCallWithTimeout error = %v, want %v", err, protocol.ErrOverflow)
	}
	if client.config.MaxResponsePayloadBytes != responseBufSize*2 {
		t.Fatalf("response capacity = %d, want %d", client.config.MaxResponsePayloadBytes, responseBufSize*2)
	}

	srv.wait(t)
	cleanupAll(svc)
}

func TestUnixRawCallRejectsUnknownTransportStatus(t *testing.T) {
	cfg := testServerConfig()
	svc := uniqueUnixService("go_unix_bad_transport_status")
	srv := startRawPosixSessionServer(t, svc, cfg,
		func(session *posix.Session, hdr protocol.Header, payload []byte) error {
			_ = payload
			respHdr := protocol.Header{
				Kind:            protocol.KindResponse,
				Code:            hdr.Code,
				MessageID:       hdr.MessageID,
				TransportStatus: 0xffff,
			}
			return session.Send(&respHdr, nil)
		})

	client := NewIncrementClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	refreshUnixClientReady(t, client)

	var reqBuf [protocol.IncrementPayloadSize]byte
	if protocol.IncrementEncode(41, reqBuf[:]) == 0 {
		t.Fatal("IncrementEncode failed")
	}

	_, _, err := client.doRawCallWithTimeout(protocol.MethodIncrement, reqBuf[:], 1000)
	if !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("doRawCallWithTimeout error = %v, want %v", err, protocol.ErrBadLayout)
	}

	srv.wait(t)
	cleanupAll(svc)
}

func TestUnixClientAbortUnblocksCall(t *testing.T) {
	cfg := testServerConfig()
	svc := uniqueUnixService("go_unix_call_abort")
	entered := make(chan struct{})
	release := make(chan struct{})
	srv := startRawPosixSessionServer(t, svc, cfg,
		func(session *posix.Session, hdr protocol.Header, payload []byte) error {
			_ = session
			_ = hdr
			_ = payload
			close(entered)
			<-release
			return nil
		})

	client := NewIncrementClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	refreshUnixClientReady(t, client)

	errCh := make(chan error, 1)
	go func() {
		_, err := client.CallIncrementWithTimeout(41, 5000)
		errCh <- err
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("server handler did not receive request")
	}

	start := time.Now()
	client.Abort()
	var err error
	select {
	case err = <-errCh:
	case <-time.After(time.Second):
		t.Fatal("Abort did not unblock the call within one second")
	}
	if !errors.Is(err, protocol.ErrAborted) {
		t.Fatalf("aborted call error = %v, want %v", err, protocol.ErrAborted)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("abort took too long: %s", elapsed)
	}

	close(release)
	srv.wait(t)
	cleanupAll(svc)
}

func newRawPosixShmClient(t *testing.T) (*Client, *posix.ShmContext) {
	t.Helper()

	runDir := t.TempDir()
	service := uniqueUnixService("go_unix_raw_shm")
	sessionID := unixServiceCounter.Add(1)
	reqCap := uint32(protocol.HeaderSize + 4096)
	respCap := uint32(protocol.HeaderSize + responseBufSize)

	server, err := posix.ShmServerCreate(runDir, service, sessionID, reqCap, respCap)
	if err != nil {
		t.Fatalf("ShmServerCreate failed: %v", err)
	}

	clientShm, err := posix.ShmClientAttach(runDir, service, sessionID)
	if err != nil {
		server.ShmDestroy()
		t.Fatalf("ShmClientAttach failed: %v", err)
	}

	client := NewIncrementClient(runDir, service, testClientConfig())
	client.state = StateReady
	client.shm = clientShm

	t.Cleanup(func() {
		client.Close()
		server.ShmDestroy()
	})

	return client, server
}

func startServerWithWorkersSocketReady(
	service string,
	expectedMethodCode uint16,
	handler DispatchHandler,
	workers int,
) *testServer {
	ensureRunDir()
	cleanupAll(service)

	s := NewServerWithWorkers(
		testRunDir,
		service,
		testServerConfig(),
		expectedMethodCode,
		handler,
		workers,
	)
	doneCh := runServerForTest(s)
	return &testServer{server: s, doneCh: doneCh}
}

func startServerWithConfigSocketReady(
	service string,
	cfg posix.ServerConfig,
	expectedMethodCode uint16,
	handler DispatchHandler,
) *testServer {
	ensureRunDir()
	cleanupAll(service)

	s := NewServer(testRunDir, service, cfg, expectedMethodCode, handler)
	doneCh := runServerForTest(s)
	return &testServer{server: s, doneCh: doneCh}
}

func unixShmSessionPath(service string, sessionID uint64) string {
	return filepath.Join(testRunDir, fmt.Sprintf("%s-%016x.ipcshm", service, sessionID))
}

func TestUnixServerStopWhileIdle(t *testing.T) {
	svc := uniqueUnixService("go_unix_stop_idle")
	cleanupAll(svc)
	server := NewServer(
		testRunDir,
		svc,
		testServerConfig(),
		protocol.MethodCgroupsSnapshot,
		testSnapshotDispatch(),
	)

	server.timings = testServerTimings
	done := make(chan error, 1)
	go func() {
		done <- server.Run()
	}()

	select {
	case <-server.Listening():
	case <-time.After(testWaitLimit):
		t.Fatal("server never started listening")
	}
	server.Stop()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run after Stop = %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after Stop")
	}

	cleanupAll(svc)
}

func TestUnixTryConnectInvalidServiceReturnsDisconnected(t *testing.T) {
	client := NewSnapshotClient(testRunDir, "bad/service", testClientConfig())
	defer client.Close()

	if state := client.tryConnect(); state != StateDisconnected {
		t.Fatalf("tryConnect invalid service = %d, want %d", state, StateDisconnected)
	}
}

func TestUnixPollFdRejectsInvalidFdAndHangup(t *testing.T) {
	if got := pollFd(-1, 0); got != -1 {
		t.Fatalf("pollFd(-1) = %d, want -1", got)
	}

	fds := []int{0, 0}
	if err := syscall.Pipe(fds); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	defer syscall.Close(fds[0])

	if err := syscall.Close(fds[1]); err != nil {
		t.Fatalf("close writer failed: %v", err)
	}

	if got := pollFd(fds[0], 50); got != -1 {
		t.Fatalf("pollFd(read end with closed writer) = %d, want -1", got)
	}

	closedFDs := []int{0, 0}
	if err := syscall.Pipe(closedFDs); err != nil {
		t.Fatalf("Pipe for POLLNVAL failed: %v", err)
	}
	if err := syscall.Close(closedFDs[0]); err != nil {
		t.Fatalf("close reader failed: %v", err)
	}
	defer syscall.Close(closedFDs[1])

	if got := pollFd(closedFDs[0], 50); got != -1 {
		t.Fatalf("pollFd(closed reader) = %d, want -1", got)
	}
}

func TestUnixRefreshFromBrokenReconnects(t *testing.T) {
	svc := uniqueUnixService("go_unix_refresh_broken")
	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer client.Close()

	refreshUnixClientReady(t, client)

	client.session.Close()
	client.state = StateBroken

	changed := client.Refresh()
	if !changed {
		t.Fatal("refresh from BROKEN should change state")
	}
	if client.state != StateReady {
		t.Fatalf("expected READY after reconnect, got %d", client.state)
	}
	if client.reconnectCount < 1 {
		t.Fatalf("expected reconnect_count >= 1, got %d", client.reconnectCount)
	}
}

func TestUnixSingleResponseOverflowRetriesAndRecovers(t *testing.T) {
	svc := uniqueUnixService("go_unix_single_overflow")

	scfg := testServerConfig()
	scfg.MaxResponsePayloadBytes = 64
	var calls atomic.Int32
	ts := startTestServerUnixWithConfig(
		svc,
		scfg,
		protocol.MethodCgroupsSnapshot,
		SnapshotDispatch(func(request *protocol.CgroupsRequest, builder *protocol.CgroupsBuilder) bool {
			if request.LayoutVersion != 1 || request.Flags != 0 {
				return false
			}
			builder.SetHeader(1, uint64(calls.Add(1)))
			if calls.Load() == 1 {
				return builder.Add(
					1001,
					0,
					1,
					[]byte("oversized-snapshot"),
					[]byte("/sys/fs/cgroup/this-path-is-intentionally-too-large-for-the-first-response"),
				) == nil
			}
			return true
		}, 1),
	)
	defer ts.stop()

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	refreshUnixClientReady(t, client)

	view, err := client.CallSnapshot()
	if err != nil {
		t.Fatalf("CallSnapshot after single-response overflow failed: %v", err)
	}
	if view.ItemCount != 0 || view.Generation != 2 {
		t.Fatalf("CallSnapshot after single-response overflow returned item_count=%d generation=%d, want 0 and 2", view.ItemCount, view.Generation)
	}
	if client.state != StateReady {
		t.Fatalf("client state after single-response overflow = %d, want READY", client.state)
	}
	if client.reconnectCount < 1 {
		t.Fatalf("expected reconnect_count >= 1 after single-response overflow, got %d", client.reconnectCount)
	}
}

func TestUnixServerRunRejectsInvalidServiceName(t *testing.T) {
	server := NewServer(
		testRunDir,
		"bad/service",
		testServerConfig(),
		protocol.MethodCgroupsSnapshot,
		testSnapshotDispatch(),
	)
	if err := server.Run(); !errors.Is(err, posix.ErrBadParam) {
		t.Fatalf("Run() invalid service name = %v, want %v", err, posix.ErrBadParam)
	}
}

func TestUnixServerRejectsSessionAtWorkerCapacity(t *testing.T) {
	svc := uniqueUnixService("go_unix_capacity")
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	released := false
	releaseOnce := func() {
		if !released {
			close(release)
			released = true
		}
	}

	var handlerCalls atomic.Int32
	ts := startServerWithWorkersSocketReady(
		svc,
		protocol.MethodIncrement,
		IncrementDispatch(func(v uint64) (uint64, bool) {
			handlerCalls.Add(1)
			if v == 41 {
				select {
				case entered <- struct{}{}:
				default:
				}
				<-release
			}
			return v + 1, true
		}),
		1,
	)
	defer ts.stop()
	// Deferred after ts.stop so it runs first: a t.Fatal must unblock the
	// handler, or stopping the server waits for it forever.
	defer releaseOnce()

	client1 := NewIncrementClient(testRunDir, svc, testClientConfig())
	defer client1.Close()
	refreshUnixClientReady(t, client1)

	type callResult struct {
		got uint64
		err error
	}
	callDone := make(chan callResult, 1)
	go func() {
		got, err := client1.CallIncrement(41)
		callDone <- callResult{got: got, err: err}
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("first client did not occupy the only worker slot")
	}

	// Over capacity the server closes before HELLO_ACK, so the connect fails.
	ccfg := testClientConfig()
	session2, err := posix.Connect(testRunDir, svc, &ccfg)
	if err == nil {
		session2.Close()
		t.Fatal("second connect should fail while the server is at worker capacity")
	}
	if handlerCalls.Load() != 1 {
		t.Fatalf("handler entered %d times, want 1 while second session is rejected", handlerCalls.Load())
	}

	releaseOnce()
	res := <-callDone
	if res.err != nil {
		t.Fatalf("first client call failed: %v", res.err)
	}
	if res.got != 42 {
		t.Fatalf("first client result = %d, want 42", res.got)
	}
	client1.Close()

	// The slot frees when the first session's worker exits.
	waitSessionExits(ts.server, 1)
	verify := NewIncrementClient(testRunDir, svc, testClientConfig())
	refreshUnixClientReady(t, verify)
	got, err := verify.CallIncrement(1)
	verify.Close()
	if err != nil {
		t.Fatalf("verification call after worker-capacity reject failed: %v", err)
	}
	if got != 2 {
		t.Fatalf("verification increment = %d, want 2", got)
	}
}

func TestUnixIdlePeerDisconnectKeepsServerHealthy(t *testing.T) {
	svc := uniqueUnixService("go_unix_idle_disconnect")
	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	ccfg := testClientConfig()
	session, err := posix.Connect(testRunDir, svc, &ccfg)
	if err != nil {
		t.Fatalf("raw connect failed: %v", err)
	}
	session.Close()
	waitSessionExits(ts.server, 1)

	verify := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer verify.Close()
	refreshUnixClientReady(t, verify)

	view, err := verify.CallSnapshot()
	if err != nil {
		t.Fatalf("normal call after idle disconnect failed: %v", err)
	}
	if view.ItemCount != 3 {
		t.Fatalf("expected 3 items after idle disconnect, got %d", view.ItemCount)
	}
}

func TestUnixClientTransportWithoutSession(t *testing.T) {
	client := NewIncrementClient(testRunDir, uniqueUnixService("go_unix_transport"), testClientConfig())
	defer client.Close()

	hdr := protocol.Header{
		Kind:            protocol.KindRequest,
		Code:            protocol.MethodIncrement,
		ItemCount:       1,
		MessageID:       1,
		TransportStatus: protocol.StatusOK,
	}

	if err := client.transportSend(&hdr, nil, 0); !errors.Is(err, protocol.ErrTransport) {
		t.Fatalf("transportSend without session = %v, want ErrTransport", err)
	}
	if _, _, err := client.transportReceive(); !errors.Is(err, protocol.ErrTransport) {
		t.Fatalf("transportReceive without session = %v, want ErrTransport", err)
	}
}

func TestUnixCallWithRetryStopsWhenOverflowCannotGrowCapacity(t *testing.T) {
	svc := uniqueUnixService("go_unix_retry_no_growth")
	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	refreshUnixClientReady(t, client)

	attempts := 0
	err := client.callWithRetry(0, func() error {
		attempts++
		return protocol.ErrOverflow
	})
	if !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("callWithRetry no-growth error = %v, want ErrOverflow", err)
	}
	if attempts != 1 {
		t.Fatalf("callWithRetry no-growth attempts = %d, want 1", attempts)
	}
	if client.state != StateBroken {
		t.Fatalf("callWithRetry no-growth state = %d, want StateBroken", client.state)
	}
}

func TestUnixCallWithRetryCapsOverflowGrowthRetries(t *testing.T) {
	svc := uniqueUnixService("go_unix_retry_growth_cap")
	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	refreshUnixClientReady(t, client)
	client.config.MaxRequestPayloadBytes = protocol.MaxPayloadDefault

	attempts := 0
	err := client.callWithRetry(0, func() error {
		attempts++
		client.config.MaxRequestPayloadBytes++
		return protocol.ErrOverflow
	})
	if !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("callWithRetry growth-cap error = %v, want ErrOverflow", err)
	}
	if attempts != 8 {
		t.Fatalf("callWithRetry growth-cap attempts = %d, want 8", attempts)
	}
	if client.state != StateBroken {
		t.Fatalf("callWithRetry growth-cap state = %d, want StateBroken", client.state)
	}
}

func TestUnixDispatchSingleUnsupportedMethods(t *testing.T) {
	server := NewServer(
		testRunDir,
		uniqueUnixService("go_unix_dispatch"),
		testServerConfig(),
		protocol.MethodCgroupsSnapshot,
		nil,
	)
	responseBuf := make([]byte, 128)

	for _, methodCode := range []uint16{
		protocol.MethodIncrement,
		protocol.MethodStringReverse,
		protocol.MethodCgroupsSnapshot,
		0xffff,
	} {
		if n, err := server.dispatchSingle(methodCode, nil, responseBuf); err == nil || n != 0 {
			t.Fatalf("dispatchSingle(%d) = (%d, %v), want (0, error)", methodCode, n, err)
		}
	}

	server = NewServer(
		testRunDir,
		uniqueUnixService("go_unix_snapshot_dispatch"),
		testServerConfig(),
		protocol.MethodCgroupsSnapshot,
		testSnapshotDispatch(),
	)
	if n, err := server.dispatchSingle(protocol.MethodCgroupsSnapshot, nil, nil); err == nil || n != 0 {
		t.Fatalf("dispatchSingle snapshot with zero response buffer = (%d, %v), want (0, error)", n, err)
	}

	server = NewServer(
		testRunDir,
		uniqueUnixService("go_unix_snapshot_dispatch_zero"),
		testServerConfig(),
		protocol.MethodCgroupsSnapshot,
		SnapshotDispatch(testCgroupsHandler, 0),
	)
	if n, err := server.dispatchSingle(protocol.MethodCgroupsSnapshot, nil, nil); err == nil || n != 0 {
		t.Fatalf("dispatchSingle snapshot with derived zero max items = (%d, %v), want (0, error)", n, err)
	}
}

func TestUnixNonRequestTerminatesSession(t *testing.T) {
	svc := uniqueUnixService("go_unix_nonreq")
	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	ccfg := testClientConfig()
	session, err := posix.Connect(testRunDir, svc, &ccfg)
	if err != nil {
		t.Fatalf("raw connect failed: %v", err)
	}

	badHdr := &protocol.Header{
		Kind:            protocol.KindResponse,
		Code:            protocol.MethodCgroupsSnapshot,
		ItemCount:       0,
		MessageID:       1,
		TransportStatus: protocol.StatusOK,
	}
	if err := session.Send(badHdr, nil); err != nil {
		t.Fatalf("send non-request failed: %v", err)
	}

	// The server ends the session on the bad message, so the receive below
	// fails instead of returning a response.
	req := protocol.CgroupsRequest{LayoutVersion: 1, Flags: 0}
	var reqBuf [4]byte
	if req.Encode(reqBuf[:]) == 0 {
		t.Fatal("request encode failed")
	}
	goodHdr := &protocol.Header{
		Kind:            protocol.KindRequest,
		Code:            protocol.MethodCgroupsSnapshot,
		ItemCount:       1,
		MessageID:       2,
		TransportStatus: protocol.StatusOK,
	}
	_ = session.Send(goodHdr, reqBuf[:])

	recvBuf := make([]byte, 4096)
	if _, _, err := session.Receive(recvBuf); err == nil {
		t.Fatal("receive after non-request should fail")
	}
	session.Close()

	verify := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer verify.Close()
	refreshUnixClientReady(t, verify)

	view, err := verify.CallSnapshot()
	if err != nil {
		t.Fatalf("normal call after bad session failed: %v", err)
	}
	if view.ItemCount != 3 {
		t.Fatalf("expected 3 items after recovery, got %d", view.ItemCount)
	}
}

func TestUnixTruncatedRawRequestKeepsServerHealthy(t *testing.T) {
	svc := uniqueUnixService("go_unix_short_req")
	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	ccfg := testClientConfig()
	session, err := posix.Connect(testRunDir, svc, &ccfg)
	if err != nil {
		t.Fatalf("raw connect failed: %v", err)
	}
	if _, err := syscall.SendmsgN(session.Fd(), []byte{1, 2, 3, 4}, nil, nil, 0); err != nil {
		session.Close()
		t.Fatalf("send truncated raw request failed: %v", err)
	}
	session.Close()
	waitSessionExits(ts.server, 1)

	verify := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer verify.Close()
	refreshUnixClientReady(t, verify)

	view, err := verify.CallSnapshot()
	if err != nil {
		t.Fatalf("normal call after truncated raw request failed: %v", err)
	}
	if view.ItemCount != 3 {
		t.Fatalf("expected 3 items after truncated raw request, got %d", view.ItemCount)
	}
}

func TestUnixPeerDisconnectDuringResponseKeepsServerHealthy(t *testing.T) {
	svc := uniqueUnixService("go_unix_send_fail")
	// The first request is answered only after its client has gone, so the
	// response send fails.
	clientGone := make(chan struct{})
	ts := startTestServerUnixWithConfig(
		svc,
		testServerConfig(),
		protocol.MethodIncrement,
		IncrementDispatch(func(v uint64) (uint64, bool) {
			if v == 41 {
				<-clientGone
			}
			return v + 1, true
		}),
	)
	defer ts.stop()

	ccfg := testClientConfig()
	session, err := posix.Connect(testRunDir, svc, &ccfg)
	if err != nil {
		t.Fatalf("raw connect failed: %v", err)
	}

	reqHdr := &protocol.Header{
		Kind:            protocol.KindRequest,
		Code:            protocol.MethodIncrement,
		ItemCount:       1,
		MessageID:       1,
		TransportStatus: protocol.StatusOK,
	}
	var reqPayload [protocol.IncrementPayloadSize]byte
	if protocol.IncrementEncode(41, reqPayload[:]) == 0 {
		t.Fatal("IncrementEncode failed")
	}
	if err := session.Send(reqHdr, reqPayload[:]); err != nil {
		t.Fatalf("send increment request failed: %v", err)
	}
	session.Close()
	close(clientGone)
	waitSessionExits(ts.server, 1)

	verify := NewIncrementClient(testRunDir, svc, testClientConfig())
	defer verify.Close()
	refreshUnixClientReady(t, verify)

	got, err := verify.CallIncrement(9)
	if err != nil {
		t.Fatalf("normal call after peer disconnect failed: %v", err)
	}
	if got != 10 {
		t.Fatalf("increment after peer disconnect = %d, want 10", got)
	}
}

func TestUnixCallSnapshotWithMalformedTransportState(t *testing.T) {
	svc := uniqueUnixService("go_unix_malformed_state")
	ts := startTestServer(svc, testSnapshotDispatch())
	defer ts.stop()

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer client.Close()

	refreshUnixClientReady(t, client)

	client.session.Close()
	client.session = nil

	view, err := client.CallSnapshot()
	if err != nil {
		t.Fatalf("expected reconnect to recover from nil session, got %v", err)
	}
	if view.ItemCount != 3 {
		t.Fatalf("expected recovered snapshot, got %d items", view.ItemCount)
	}
}

func TestUnixCallIncrementBatchWithMalformedTransportState(t *testing.T) {
	svc := uniqueUnixService("go_unix_batch_malformed_state")

	scfg := testServerConfig()
	scfg.MaxRequestBatchItems = 16
	scfg.MaxResponseBatchItems = 16
	s := NewServer(
		testRunDir,
		svc,
		scfg,
		protocol.MethodIncrement,
		pingPongIncrementDispatch(),
	)
	doneCh := runServerForTest(s)
	defer func() {
		s.Stop()
		<-doneCh
		cleanupAll(svc)
	}()

	ccfg := testClientConfig()
	ccfg.MaxRequestBatchItems = 16
	ccfg.MaxResponseBatchItems = 16
	client := NewIncrementClient(testRunDir, svc, ccfg)
	defer client.Close()

	refreshUnixClientReady(t, client)

	client.session.Close()
	client.session = nil

	got, err := client.CallIncrementBatch([]uint64{1, 41})
	if err != nil {
		t.Fatalf("expected batch reconnect to recover from nil session, got %v", err)
	}

	want := []uint64{2, 42}
	if len(got) != len(want) {
		t.Fatalf("batch result len = %d, want %d", len(got), len(want))
	}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("batch[%d] = %d, want %d", i, got[i], v)
		}
	}
}

func TestUnixCallIncrementRejectsMalformedResponseEnvelope(t *testing.T) {
	cases := []struct {
		name   string
		want   error
		mutate func(*protocol.Header)
	}{
		{
			name: "bad kind",
			want: protocol.ErrBadKind,
			mutate: func(h *protocol.Header) {
				h.Kind = protocol.KindRequest
			},
		},
		{
			name: "bad code",
			want: protocol.ErrBadLayout,
			mutate: func(h *protocol.Header) {
				h.Code = protocol.MethodStringReverse
			},
		},
		{
			name: "bad status",
			want: protocol.ErrBadLayout,
			mutate: func(h *protocol.Header) {
				h.TransportStatus = protocol.StatusInternalError
			},
		},
		{
			name: "bad message_id",
			want: protocol.ErrTruncated,
			mutate: func(h *protocol.Header) {
				h.MessageID++
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := uniqueUnixService("go_unix_bad_incr_resp")
			srv := startRawPosixSessionServer(t, svc, testServerConfig(),
				func(session *posix.Session, hdr protocol.Header, payload []byte) error {
					if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodIncrement {
						return fmt.Errorf("unexpected request header: %+v", hdr)
					}

					var respPayload [protocol.IncrementPayloadSize]byte
					protocol.IncrementEncode(42, respPayload[:])

					respHdr := protocol.Header{
						Kind:            protocol.KindResponse,
						Code:            protocol.MethodIncrement,
						ItemCount:       1,
						MessageID:       hdr.MessageID,
						TransportStatus: protocol.StatusOK,
					}
					tc.mutate(&respHdr)
					return session.Send(&respHdr, respPayload[:])
				})

			client := NewIncrementClient(testRunDir, svc, testClientConfig())
			defer client.Close()

			refreshUnixClientReady(t, client)

			_, err := client.CallIncrement(41)
			if !errors.Is(err, tc.want) {
				t.Fatalf("CallIncrement error = %v, want %v", err, tc.want)
			}

			srv.wait(t)
			cleanupAll(svc)
		})
	}
}

func TestUnixCallIncrementRejectsMalformedPayload(t *testing.T) {
	svc := uniqueUnixService("go_unix_bad_incr_payload")
	srv := startRawPosixSessionServer(t, svc, testServerConfig(),
		func(session *posix.Session, hdr protocol.Header, payload []byte) error {
			if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodIncrement {
				return fmt.Errorf("unexpected request header: %+v", hdr)
			}

			respHdr := protocol.Header{
				Kind:            protocol.KindResponse,
				Code:            protocol.MethodIncrement,
				ItemCount:       1,
				MessageID:       hdr.MessageID,
				TransportStatus: protocol.StatusOK,
			}
			return session.Send(&respHdr, []byte{1, 2, 3, 4})
		})

	client := NewIncrementClient(testRunDir, svc, testClientConfig())
	defer client.Close()

	refreshUnixClientReady(t, client)

	_, err := client.CallIncrement(41)
	if !errors.Is(err, protocol.ErrTruncated) {
		t.Fatalf("CallIncrement error = %v, want %v", err, protocol.ErrTruncated)
	}

	srv.wait(t)
	cleanupAll(svc)
}

func TestUnixCallStringReverseRejectsMalformedResponseEnvelope(t *testing.T) {
	cases := []struct {
		name   string
		want   error
		mutate func(*protocol.Header)
	}{
		{
			name: "bad kind",
			want: protocol.ErrBadKind,
			mutate: func(h *protocol.Header) {
				h.Kind = protocol.KindRequest
			},
		},
		{
			name: "bad code",
			want: protocol.ErrBadLayout,
			mutate: func(h *protocol.Header) {
				h.Code = protocol.MethodIncrement
			},
		},
		{
			name: "bad status",
			want: protocol.ErrBadLayout,
			mutate: func(h *protocol.Header) {
				h.TransportStatus = protocol.StatusInternalError
			},
		},
		{
			name: "bad message_id",
			want: protocol.ErrTruncated,
			mutate: func(h *protocol.Header) {
				h.MessageID++
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := uniqueUnixService("go_unix_bad_str_resp")
			srv := startRawPosixSessionServer(t, svc, testServerConfig(),
				func(session *posix.Session, hdr protocol.Header, payload []byte) error {
					if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodStringReverse {
						return fmt.Errorf("unexpected request header: %+v", hdr)
					}

					respPayload := make([]byte, protocol.StringReverseHdrSize+len("olleh")+1)
					if protocol.StringReverseEncode("olleh", respPayload) == 0 {
						return fmt.Errorf("StringReverseEncode failed")
					}

					respHdr := protocol.Header{
						Kind:            protocol.KindResponse,
						Code:            protocol.MethodStringReverse,
						ItemCount:       1,
						MessageID:       hdr.MessageID,
						TransportStatus: protocol.StatusOK,
					}
					tc.mutate(&respHdr)
					return session.Send(&respHdr, respPayload)
				})

			client := NewStringReverseClient(testRunDir, svc, testClientConfig())
			defer client.Close()

			refreshUnixClientReady(t, client)

			_, err := client.CallStringReverse("hello")
			if !errors.Is(err, tc.want) {
				t.Fatalf("CallStringReverse error = %v, want %v", err, tc.want)
			}

			srv.wait(t)
			cleanupAll(svc)
		})
	}
}

func TestUnixCallSnapshotRejectsMalformedPayload(t *testing.T) {
	svc := uniqueUnixService("go_unix_bad_snapshot_payload")
	srv := startRawPosixSessionServer(t, svc, testServerConfig(),
		func(session *posix.Session, hdr protocol.Header, payload []byte) error {
			if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodCgroupsSnapshot {
				return fmt.Errorf("unexpected request header: %+v", hdr)
			}

			respHdr := protocol.Header{
				Kind:            protocol.KindResponse,
				Code:            protocol.MethodCgroupsSnapshot,
				ItemCount:       1,
				MessageID:       hdr.MessageID,
				TransportStatus: protocol.StatusOK,
			}
			return session.Send(&respHdr, []byte{1, 2, 3, 4})
		})

	client := NewSnapshotClient(testRunDir, svc, testClientConfig())
	defer client.Close()

	refreshUnixClientReady(t, client)

	_, err := client.CallSnapshot()
	if !errors.Is(err, protocol.ErrTruncated) {
		t.Fatalf("CallSnapshot error = %v, want %v", err, protocol.ErrTruncated)
	}

	srv.wait(t)
	cleanupAll(svc)
}

func TestUnixCallStringReverseRejectsMalformedPayload(t *testing.T) {
	svc := uniqueUnixService("go_unix_bad_str_payload")
	srv := startRawPosixSessionServer(t, svc, testServerConfig(),
		func(session *posix.Session, hdr protocol.Header, payload []byte) error {
			if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodStringReverse {
				return fmt.Errorf("unexpected request header: %+v", hdr)
			}

			respHdr := protocol.Header{
				Kind:            protocol.KindResponse,
				Code:            protocol.MethodStringReverse,
				ItemCount:       1,
				MessageID:       hdr.MessageID,
				TransportStatus: protocol.StatusOK,
			}

			respPayload := make([]byte, protocol.StringReverseHdrSize+3)
			binary.NativeEndian.PutUint32(respPayload[0:4], uint32(protocol.StringReverseHdrSize))
			binary.NativeEndian.PutUint32(respPayload[4:8], 2)
			copy(respPayload[8:], []byte{'o', 'k', 'x'})
			return session.Send(&respHdr, respPayload)
		})

	client := NewStringReverseClient(testRunDir, svc, testClientConfig())
	defer client.Close()

	refreshUnixClientReady(t, client)

	_, err := client.CallStringReverse("hello")
	if !errors.Is(err, protocol.ErrMissingNul) {
		t.Fatalf("CallStringReverse error = %v, want %v", err, protocol.ErrMissingNul)
	}

	srv.wait(t)
	cleanupAll(svc)
}

func TestUnixCallIncrementBatchRejectsMalformedResponseEnvelope(t *testing.T) {
	cases := []struct {
		name   string
		want   error
		mutate func(*protocol.Header)
	}{
		{
			name: "bad kind",
			want: protocol.ErrBadKind,
			mutate: func(h *protocol.Header) {
				h.Kind = protocol.KindRequest
			},
		},
		{
			name: "bad code",
			want: protocol.ErrBadLayout,
			mutate: func(h *protocol.Header) {
				h.Code = protocol.MethodStringReverse
			},
		},
		{
			name: "bad status",
			want: protocol.ErrBadLayout,
			mutate: func(h *protocol.Header) {
				h.TransportStatus = protocol.StatusInternalError
			},
		},
		{
			name: "bad message_id",
			want: protocol.ErrTruncated,
			mutate: func(h *protocol.Header) {
				h.MessageID++
			},
		},
		{
			name: "missing batch flag",
			want: protocol.ErrBadItemCount,
			mutate: func(h *protocol.Header) {
				h.Flags = 0
			},
		},
		{
			name: "wrong item count",
			want: protocol.ErrBadItemCount,
			mutate: func(h *protocol.Header) {
				h.Flags = protocol.FlagBatch
				h.ItemCount = 1
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testServerConfig()
			cfg.MaxRequestBatchItems = 16
			cfg.MaxResponseBatchItems = 16

			svc := uniqueUnixService("go_unix_bad_batch_resp")
			srv := startRawPosixSessionServer(t, svc, cfg,
				func(session *posix.Session, hdr protocol.Header, payload []byte) error {
					if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodIncrement {
						return fmt.Errorf("unexpected request header: %+v", hdr)
					}

					respHdr := protocol.Header{
						Kind:            protocol.KindResponse,
						Code:            protocol.MethodIncrement,
						Flags:           protocol.FlagBatch,
						ItemCount:       hdr.ItemCount,
						MessageID:       hdr.MessageID,
						TransportStatus: protocol.StatusOK,
					}
					tc.mutate(&respHdr)

					var itemA [protocol.IncrementPayloadSize]byte
					var itemB [protocol.IncrementPayloadSize]byte
					if protocol.IncrementEncode(2, itemA[:]) == 0 || protocol.IncrementEncode(3, itemB[:]) == 0 {
						return fmt.Errorf("IncrementEncode failed")
					}

					respBuf := make([]byte, 64)
					bb := protocol.NewBatchBuilder(respBuf, 2)
					if err := bb.Add(itemA[:]); err != nil {
						return err
					}
					if err := bb.Add(itemB[:]); err != nil {
						return err
					}
					n, _ := bb.Finish()
					return session.Send(&respHdr, respBuf[:n])
				})

			ccfg := testClientConfig()
			ccfg.MaxRequestBatchItems = 16
			ccfg.MaxResponseBatchItems = 16

			client := NewIncrementClient(testRunDir, svc, ccfg)
			defer client.Close()

			refreshUnixClientReady(t, client)

			_, err := client.CallIncrementBatch([]uint64{1, 2})
			if !errors.Is(err, tc.want) {
				t.Fatalf("CallIncrementBatch error = %v, want %v", err, tc.want)
			}

			srv.wait(t)
			cleanupAll(svc)
		})
	}
}

func TestUnixCallIncrementBatchRejectsMalformedPayload(t *testing.T) {
	t.Run("bad batch directory", func(t *testing.T) {
		cfg := testServerConfig()
		cfg.MaxRequestBatchItems = 16
		cfg.MaxResponseBatchItems = 16

		svc := uniqueUnixService("go_unix_bad_batch_dir")
		srv := startRawPosixSessionServerN(t, svc, cfg, 2,
			func(session *posix.Session, hdr protocol.Header, payload []byte) error {
				if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodIncrement {
					return fmt.Errorf("unexpected request header: %+v", hdr)
				}

				respPayload := make([]byte, 24)
				binary.NativeEndian.PutUint32(respPayload[0:4], 1)
				binary.NativeEndian.PutUint32(respPayload[4:8], 4)
				binary.NativeEndian.PutUint32(respPayload[8:12], 0)
				binary.NativeEndian.PutUint32(respPayload[12:16], 4)
				copy(respPayload[16:], []byte("payload!!"))

				respHdr := protocol.Header{
					Kind:            protocol.KindResponse,
					Code:            protocol.MethodIncrement,
					Flags:           protocol.FlagBatch,
					ItemCount:       2,
					MessageID:       hdr.MessageID,
					TransportStatus: protocol.StatusOK,
				}
				return session.Send(&respHdr, respPayload)
			})

		ccfg := testClientConfig()
		ccfg.MaxRequestBatchItems = 16
		ccfg.MaxResponseBatchItems = 16
		client := NewIncrementClient(testRunDir, svc, ccfg)
		defer client.Close()

		refreshUnixClientReady(t, client)

		_, err := client.CallIncrementBatch([]uint64{1, 2})
		if !errors.Is(err, protocol.ErrTruncated) {
			t.Fatalf("CallIncrementBatch error = %v, want %v", err, protocol.ErrTruncated)
		}

		srv.wait(t)
		cleanupAll(svc)
	})

	t.Run("truncated batch item", func(t *testing.T) {
		cfg := testServerConfig()
		cfg.MaxRequestBatchItems = 16
		cfg.MaxResponseBatchItems = 16

		svc := uniqueUnixService("go_unix_bad_batch_payload")
		srv := startRawPosixSessionServerN(t, svc, cfg, 1,
			func(session *posix.Session, hdr protocol.Header, payload []byte) error {
				if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodIncrement {
					return fmt.Errorf("unexpected request header: %+v", hdr)
				}

				respBuf := make([]byte, 64)
				bb := protocol.NewBatchBuilder(respBuf, 1)
				if err := bb.Add([]byte{1, 2, 3, 4}); err != nil {
					return err
				}
				n, _ := bb.Finish()

				respHdr := protocol.Header{
					Kind:            protocol.KindResponse,
					Code:            protocol.MethodIncrement,
					Flags:           protocol.FlagBatch,
					ItemCount:       1,
					MessageID:       hdr.MessageID,
					TransportStatus: protocol.StatusOK,
				}
				return session.Send(&respHdr, respBuf[:n])
			})

		ccfg := testClientConfig()
		ccfg.MaxRequestBatchItems = 16
		ccfg.MaxResponseBatchItems = 16
		client := NewIncrementClient(testRunDir, svc, ccfg)
		defer client.Close()

		refreshUnixClientReady(t, client)

		_, err := client.CallIncrementBatch([]uint64{1})
		if !errors.Is(err, protocol.ErrTruncated) {
			t.Fatalf("CallIncrementBatch error = %v, want %v", err, protocol.ErrTruncated)
		}

		srv.wait(t)
		cleanupAll(svc)
	})

	t.Run("missing batch body", func(t *testing.T) {
		cfg := testServerConfig()
		cfg.MaxRequestBatchItems = 16
		cfg.MaxResponseBatchItems = 16

		svc := uniqueUnixService("go_unix_bad_batch_body")
		srv := startRawPosixSessionServerN(t, svc, cfg, 1,
			func(session *posix.Session, hdr protocol.Header, payload []byte) error {
				if hdr.Kind != protocol.KindRequest || hdr.Code != protocol.MethodIncrement {
					return fmt.Errorf("unexpected request header: %+v", hdr)
				}

				respHdr := protocol.Header{
					Kind:            protocol.KindResponse,
					Code:            protocol.MethodIncrement,
					Flags:           protocol.FlagBatch,
					ItemCount:       hdr.ItemCount,
					MessageID:       hdr.MessageID,
					TransportStatus: protocol.StatusOK,
				}
				return session.Send(&respHdr, nil)
			})

		ccfg := testClientConfig()
		ccfg.MaxRequestBatchItems = 16
		ccfg.MaxResponseBatchItems = 16
		client := NewIncrementClient(testRunDir, svc, ccfg)
		defer client.Close()

		refreshUnixClientReady(t, client)

		_, err := client.CallIncrementBatch([]uint64{1, 2})
		if !errors.Is(err, protocol.ErrTruncated) {
			t.Fatalf("CallIncrementBatch error = %v, want %v", err, protocol.ErrTruncated)
		}

		srv.wait(t)
		cleanupAll(svc)
	})
}

func TestUnixSendFailureWithLivePeerIsTransport(t *testing.T) {
	ensureRunDir()
	svc := uniqueUnixService("go_unix_send_live_peer")
	cleanupAll(svc)
	defer cleanupAll(svc)

	// The server accepts one session, then removes the endpoint so the client
	// cannot reconnect, and keeps the session open until released.
	listener, err := posix.Listen(testRunDir, svc, testServerConfig())
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	accepted := make(chan error, 1)
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		session, err := listener.Accept()
		listener.Close()
		accepted <- err
		if err != nil {
			return
		}
		<-release
		session.Close()
	}()

	client := NewIncrementClient(testRunDir, svc, testClientConfig())
	defer client.Close()
	client.Refresh()
	if client.state != StateReady {
		t.Fatalf("client state = %d, want StateReady", client.state)
	}
	if err := <-accepted; err != nil {
		t.Fatalf("Accept failed: %v", err)
	}

	// Our own write shutdown makes send fail with EPIPE while the server still
	// holds the session open.
	if err := syscall.Shutdown(client.session.Fd(), syscall.SHUT_WR); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if _, err := client.CallIncrement(1); !errors.Is(err, protocol.ErrTransport) {
		t.Fatalf("CallIncrement with live peer = %v, want ErrTransport", err)
	}

	close(release)
	<-done
}
