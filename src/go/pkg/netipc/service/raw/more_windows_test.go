//go:build windows

package raw

import (
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

func TestWinClientStatusFresh(t *testing.T) {
	client := NewSnapshotClient(winTestRunDir, uniqueWinService("go_win_fresh"), testWinClientConfig())
	defer client.Close()

	status := client.Status()
	if status.State != StateDisconnected {
		t.Fatalf("expected DISCONNECTED, got %d", status.State)
	}
	if status.ConnectCount != 0 || status.ReconnectCount != 0 || status.CallCount != 0 || status.ErrorCount != 0 {
		t.Fatalf("unexpected fresh counters: %+v", status)
	}
}

func TestWinClientLifecycle(t *testing.T) {
	svc := uniqueWinService("go_win_lifecycle")

	client := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()

	if client.state != StateDisconnected {
		t.Fatalf("expected DISCONNECTED, got %d", client.state)
	}
	if client.Ready() {
		t.Fatal("client should not be ready before refresh")
	}

	changed := client.Refresh()
	if !changed {
		t.Fatal("refresh without server should change state")
	}
	if client.state != StateNotFound {
		t.Fatalf("expected NOT_FOUND, got %d", client.state)
	}

	ts := startTestIncrementServerWinWithConfig(svc, testWinServerConfig())

	changed = client.Refresh()
	if !changed {
		t.Fatal("refresh with server should change state")
	}
	if client.state != StateReady {
		t.Fatalf("expected READY, got %d", client.state)
	}
	if !client.Ready() {
		t.Fatal("client should be ready")
	}

	status := client.Status()
	if status.ConnectCount != 1 {
		t.Fatalf("expected connect_count=1, got %d", status.ConnectCount)
	}
	if status.ReconnectCount != 0 {
		t.Fatalf("expected reconnect_count=0, got %d", status.ReconnectCount)
	}

	client.Close()
	if client.state != StateDisconnected {
		t.Fatalf("expected DISCONNECTED after close, got %d", client.state)
	}

	ts.stop()
}

func TestWinClientRefreshFromReadyNoop(t *testing.T) {
	svc := uniqueWinService("go_win_ready")
	ts := startTestIncrementServerWinWithConfig(svc, testWinServerConfig())
	defer ts.stop()

	client := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()

	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	changed := client.Refresh()
	if changed {
		t.Fatal("refresh from READY should be a no-op")
	}
	if client.state != StateReady {
		t.Fatalf("expected READY, got %d", client.state)
	}
}

func TestWinServerStopWhileIdle(t *testing.T) {
	svc := uniqueWinService("go_win_stop_idle")
	server := NewServer(
		winTestRunDir,
		svc,
		testWinServerConfig(),
		protocol.MethodIncrement,
		winIncrementDispatchHandler(),
	)

	done := make(chan error, 1)
	go func() {
		done <- server.Run()
	}()

	deadline := time.Now().Add(2 * time.Second)
	for server.listener == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if server.listener == nil {
		t.Fatal("server listener did not start")
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
}

func TestWinServerStopWithActiveClientAndRestart(t *testing.T) {
	svc := uniqueWinService("go_win_stop_active")
	ts1 := startTestSnapshotServerWinWithConfig(svc, testWinServerConfig())

	client := NewSnapshotClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()
	waitWinClientReady(t, client)

	if _, err := client.CallSnapshot(); err != nil {
		t.Fatalf("first snapshot failed: %v", err)
	}

	stopDone := make(chan struct{})
	go func() {
		ts1.stop()
		close(stopDone)
	}()

	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("active server stop did not exit")
	}

	ts2 := startTestSnapshotServerWinWithConfig(svc, testWinServerConfig())
	defer ts2.stop()

	view, err := client.CallSnapshot()
	if err != nil {
		t.Fatalf("snapshot after restart failed: %v", err)
	}
	if view.ItemCount != 3 {
		t.Fatalf("expected 3 items after restart, got %d", view.ItemCount)
	}

	status := client.Status()
	if status.ReconnectCount < 1 {
		t.Fatalf("expected reconnect_count >= 1, got %d", status.ReconnectCount)
	}
}

func TestWinClientRefreshFromAuthFailed(t *testing.T) {
	svc := uniqueWinService("go_win_auth")

	scfg := testWinServerConfig()
	scfg.AuthToken = 0x1111
	ts := startTestIncrementServerWinWithConfig(svc, scfg)
	defer ts.stop()

	ccfg := testWinClientConfig()
	ccfg.AuthToken = 0x2222
	client := NewIncrementClient(winTestRunDir, svc, ccfg)
	defer client.Close()

	client.Refresh()
	if client.state != StateAuthFailed {
		t.Fatalf("expected AUTH_FAILED, got %d", client.state)
	}

	changed := client.Refresh()
	if changed {
		t.Fatal("refresh from AUTH_FAILED should be a no-op")
	}
}

func TestWinClientRefreshFromIncompatible(t *testing.T) {
	svc := uniqueWinService("go_win_incompat")

	scfg := testWinServerConfig()
	scfg.SupportedProfiles = protocol.ProfileSHMFutex
	ts := startTestIncrementServerWinWithConfig(svc, scfg)
	defer ts.stop()

	ccfg := testWinClientConfig()
	ccfg.SupportedProfiles = protocol.ProfileBaseline
	client := NewIncrementClient(winTestRunDir, svc, ccfg)
	defer client.Close()

	client.Refresh()
	if client.state != StateIncompatible {
		t.Fatalf("expected INCOMPATIBLE, got %d", client.state)
	}

	changed := client.Refresh()
	if changed {
		t.Fatal("refresh from INCOMPATIBLE should be a no-op")
	}
}

func TestWinClientRefreshFromProtocolVersionMismatch(t *testing.T) {
	svc := uniqueWinService("go_win_proto_incompat")
	packet := encodeHelloAckPacketWithVersion(protocol.Version+1, protocol.StatusOK, 1)
	srv := startRawWinHelloAckServer(t, svc, packet)

	client := NewSnapshotClient(winTestRunDir, svc, testWinClientConfig())
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

	changed = client.Refresh()
	if changed {
		t.Fatal("Refresh from StateIncompatible should be a no-op after protocol mismatch")
	}
	if client.state != StateIncompatible {
		t.Fatalf("expected StateIncompatible after second refresh, got %d", client.state)
	}
}

func TestWinClientRefreshFromBroken(t *testing.T) {
	svc := uniqueWinService("go_win_broken")
	ts := startTestIncrementServerWinWithConfig(svc, testWinServerConfig())
	defer ts.stop()

	client := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()

	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

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

func TestWinCallWithRetryNotReady(t *testing.T) {
	client := NewSnapshotClient(winTestRunDir, uniqueWinService("go_win_notready"), testWinClientConfig())
	defer client.Close()

	if _, err := client.CallSnapshot(); err == nil {
		t.Fatal("expected error when client is not ready")
	}
	if client.errorCount != 1 {
		t.Fatalf("expected error_count=1, got %d", client.errorCount)
	}
}

func TestWinClientTransportWithoutSession(t *testing.T) {
	client := NewIncrementClient(winTestRunDir, uniqueWinService("go_win_transport"), testWinClientConfig())
	defer client.Close()

	hdr := protocol.Header{
		Kind:            protocol.KindRequest,
		Code:            protocol.MethodIncrement,
		ItemCount:       1,
		MessageID:       1,
		TransportStatus: protocol.StatusOK,
	}

	if err := client.transportSend(&hdr, nil); !errors.Is(err, protocol.ErrTruncated) {
		t.Fatalf("transportSend without session = %v, want ErrTruncated", err)
	}

	if _, _, err := client.transportReceive(); !errors.Is(err, protocol.ErrTruncated) {
		t.Fatalf("transportReceive without session = %v, want ErrTruncated", err)
	}
}

func TestWinClientTransportReceiveWinShmError(t *testing.T) {
	client := NewIncrementClient(winTestRunDir, uniqueWinService("go_win_transport_shm_err"), testWinShmClientConfig())
	defer client.Close()

	client.state = StateReady
	client.shm = &windows.WinShmContext{}

	if _, _, err := client.transportReceive(); !errors.Is(err, protocol.ErrTruncated) {
		t.Fatalf("transportReceive with invalid WinSHM context = %v, want %v", err, protocol.ErrTruncated)
	}
}

func TestWinClientTransportReceiveWinShmRejectsShortMessage(t *testing.T) {
	client, serverShm := newRawWinShmClient(t, windows.WinShmProfileHybrid)

	if err := serverShm.WinShmSend([]byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("WinShmSend failed: %v", err)
	}

	if _, _, err := client.transportReceive(); !errors.Is(err, protocol.ErrTruncated) {
		t.Fatalf("transportReceive short WinSHM message = %v, want %v", err, protocol.ErrTruncated)
	}
}

func TestWinClientTransportReceiveWinShmRejectsBadHeader(t *testing.T) {
	client, serverShm := newRawWinShmClient(t, windows.WinShmProfileHybrid)

	msg := make([]byte, protocol.HeaderSize)
	if err := serverShm.WinShmSend(msg); err != nil {
		t.Fatalf("WinShmSend failed: %v", err)
	}

	if _, _, err := client.transportReceive(); !errors.Is(err, protocol.ErrBadMagic) {
		t.Fatalf("transportReceive bad WinSHM header = %v, want %v", err, protocol.ErrBadMagic)
	}
}

func TestWinDoRawCallWinShmRejectsBadMessageID(t *testing.T) {
	client, serverShm := newRawWinShmClient(t, windows.WinShmProfileHybrid)

	serverDone := make(chan error, 1)
	go func() {
		reqBuf := make([]byte, protocol.HeaderSize+32)
		n, err := serverShm.WinShmReceive(reqBuf, 1000)
		if err != nil {
			serverDone <- err
			return
		}

		hdr, err := protocol.DecodeHeader(reqBuf[:n])
		if err != nil {
			serverDone <- err
			return
		}

		var respPayload [protocol.IncrementPayloadSize]byte
		if protocol.IncrementEncode(42, respPayload[:]) == 0 {
			serverDone <- fmt.Errorf("IncrementEncode failed")
			return
		}

		respHdr := protocol.Header{
			Kind:            protocol.KindResponse,
			Code:            protocol.MethodIncrement,
			ItemCount:       1,
			MessageID:       hdr.MessageID + 1,
			TransportStatus: protocol.StatusOK,
		}
		serverDone <- serverShm.WinShmSend(encodeRawWinMessage(respHdr, respPayload[:]))
	}()

	var reqPayload [protocol.IncrementPayloadSize]byte
	if protocol.IncrementEncode(41, reqPayload[:]) == 0 {
		t.Fatal("IncrementEncode failed")
	}

	_, _, err := client.doRawCall(protocol.MethodIncrement, reqPayload[:])
	if !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("doRawCall bad WinSHM message_id = %v, want %v", err, protocol.ErrBadLayout)
	}

	if err := <-serverDone; err != nil {
		t.Fatalf("raw WinSHM server failed: %v", err)
	}
}

func TestWinCallIncrementBatchWinShmRejectsBadMessageID(t *testing.T) {
	client, serverShm := newRawWinShmClient(t, windows.WinShmProfileHybrid)

	serverDone := make(chan error, 1)
	go func() {
		reqBuf := make([]byte, protocol.HeaderSize+256)
		n, err := serverShm.WinShmReceive(reqBuf, 1000)
		if err != nil {
			serverDone <- err
			return
		}

		hdr, err := protocol.DecodeHeader(reqBuf[:n])
		if err != nil {
			serverDone <- err
			return
		}

		var itemA [protocol.IncrementPayloadSize]byte
		var itemB [protocol.IncrementPayloadSize]byte
		if protocol.IncrementEncode(2, itemA[:]) == 0 || protocol.IncrementEncode(3, itemB[:]) == 0 {
			serverDone <- fmt.Errorf("IncrementEncode failed")
			return
		}

		respBuf := make([]byte, 64)
		bb := protocol.NewBatchBuilder(respBuf, 2)
		if err := bb.Add(itemA[:]); err != nil {
			serverDone <- err
			return
		}
		if err := bb.Add(itemB[:]); err != nil {
			serverDone <- err
			return
		}
		respLen, _ := bb.Finish()

		respHdr := protocol.Header{
			Kind:            protocol.KindResponse,
			Code:            protocol.MethodIncrement,
			Flags:           protocol.FlagBatch,
			ItemCount:       2,
			MessageID:       hdr.MessageID + 1,
			TransportStatus: protocol.StatusOK,
		}
		serverDone <- serverShm.WinShmSend(encodeRawWinMessage(respHdr, respBuf[:respLen]))
	}()

	_, err := client.CallIncrementBatch([]uint64{1, 2})
	if !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("CallIncrementBatch bad WinSHM message_id = %v, want %v", err, protocol.ErrBadLayout)
	}

	if err := <-serverDone; err != nil {
		t.Fatalf("raw WinSHM server failed: %v", err)
	}
}

func TestWinCallIncrementBatchWinShmRejectsMalformedPayload(t *testing.T) {
	client, serverShm := newRawWinShmClient(t, windows.WinShmProfileHybrid)

	serverDone := make(chan error, 1)
	go func() {
		reqBuf := make([]byte, protocol.HeaderSize+256)
		n, err := serverShm.WinShmReceive(reqBuf, 1000)
		if err != nil {
			serverDone <- err
			return
		}

		hdr, err := protocol.DecodeHeader(reqBuf[:n])
		if err != nil {
			serverDone <- err
			return
		}

		respHdr := protocol.Header{
			Kind:            protocol.KindResponse,
			Code:            protocol.MethodIncrement,
			Flags:           protocol.FlagBatch,
			ItemCount:       2,
			MessageID:       hdr.MessageID,
			TransportStatus: protocol.StatusOK,
		}
		// ItemCount=2 requires a 16-byte aligned directory. An 8-byte payload
		// makes the client-side BatchItemGet() fail while decoding the response.
		serverDone <- serverShm.WinShmSend(encodeRawWinMessage(respHdr, make([]byte, 8)))
	}()

	_, err := client.CallIncrementBatch([]uint64{1, 2})
	if !errors.Is(err, protocol.ErrTruncated) {
		t.Fatalf("CallIncrementBatch malformed WinSHM payload = %v, want %v", err, protocol.ErrTruncated)
	}

	if err := <-serverDone; err != nil {
		t.Fatalf("raw WinSHM server failed: %v", err)
	}
}

func TestWinClientMaxReceiveMessageBytes(t *testing.T) {
	client := NewSnapshotClient(winTestRunDir, uniqueWinService("go_win_recvmax"), testWinClientConfig())
	defer client.Close()

	if got := client.maxReceiveMessageBytes(); got != protocol.HeaderSize+cacheResponseBufSize {
		t.Fatalf("default maxReceiveMessageBytes = %d, want %d", got, protocol.HeaderSize+cacheResponseBufSize)
	}

	client.config.MaxResponsePayloadBytes = 1234
	if got := client.maxReceiveMessageBytes(); got != protocol.HeaderSize+1234 {
		t.Fatalf("config maxReceiveMessageBytes = %d, want %d", got, protocol.HeaderSize+1234)
	}

	client.session = &windows.Session{MaxResponsePayloadBytes: 4321}
	if got := client.maxReceiveMessageBytes(); got != protocol.HeaderSize+4321 {
		t.Fatalf("session maxReceiveMessageBytes = %d, want %d", got, protocol.HeaderSize+4321)
	}
}

func TestWinServerDispatchSingleMissingHandler(t *testing.T) {
	server := &Server{expectedMethodCode: protocol.MethodIncrement}
	responseBuf := make([]byte, 128)

	for _, methodCode := range []uint16{
		protocol.MethodIncrement,
		protocol.MethodStringReverse,
		protocol.MethodCgroupsSnapshot,
		0xFFFF,
	} {
		if n, err := server.dispatchSingle(methodCode, nil, responseBuf); !errors.Is(err, errHandlerFailed) || n != 0 {
			t.Fatalf("dispatchSingle(%d) = (%d, %v), want (0, %v)", methodCode, n, err, errHandlerFailed)
		}
	}
}

func TestWinServerDispatchSingleSnapshotZeroCapacity(t *testing.T) {
	server := &Server{
		expectedMethodCode: protocol.MethodCgroupsSnapshot,
		handler:            winSnapshotDispatchHandler(),
	}

	req := protocol.CgroupsRequest{LayoutVersion: 1, Flags: 0}
	var reqBuf [4]byte
	if req.Encode(reqBuf[:]) == 0 {
		t.Fatal("request encode failed")
	}
	if n, err := server.dispatchSingle(protocol.MethodCgroupsSnapshot, reqBuf[:], nil); err == nil || n != 0 {
		t.Fatalf("dispatchSingle snapshot with zero response buffer = (%d, %v), want (0, error)", n, err)
	}
}

func TestWinCgroupsCall(t *testing.T) {
	svc := uniqueWinService("go_win_cgroups")
	ts := startTestSnapshotServerWinWithConfig(svc, testWinServerConfig())
	defer ts.stop()

	client := NewSnapshotClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()

	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	view, err := client.CallSnapshot()
	if err != nil {
		t.Fatalf("snapshot call failed: %v", err)
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

	item0, err := view.Item(0)
	if err != nil {
		t.Fatalf("item 0 error: %v", err)
	}
	if item0.Hash != 1001 || item0.Enabled != 1 {
		t.Fatalf("unexpected item0: %+v", item0)
	}
	if item0.Name.String() != "docker-abc123" {
		t.Fatalf("unexpected item0 name: %q", item0.Name.String())
	}
	if item0.Path.String() != "/sys/fs/cgroup/docker/abc123" {
		t.Fatalf("unexpected item0 path: %q", item0.Path.String())
	}

	status := client.Status()
	if status.CallCount != 1 || status.ErrorCount != 0 {
		t.Fatalf("unexpected status after snapshot: %+v", status)
	}
}

func TestWinCallIncrementBatch(t *testing.T) {
	svc := uniqueWinService("go_win_batch")
	cfg := testWinServerConfig()
	cfg.MaxRequestBatchItems = 16
	cfg.MaxResponseBatchItems = 16
	ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodIncrement, winIncrementDispatchHandler())
	defer ts.stop()

	ccfg := testWinClientConfig()
	ccfg.MaxRequestBatchItems = 16
	ccfg.MaxResponseBatchItems = 16
	client := NewIncrementClient(winTestRunDir, svc, ccfg)
	defer client.Close()

	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	got, err := client.CallIncrementBatch([]uint64{1, 41, 99, 1000})
	if err != nil {
		t.Fatalf("CallIncrementBatch failed: %v", err)
	}

	want := []uint64{2, 42, 100, 1001}
	if len(got) != len(want) {
		t.Fatalf("expected %d results, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("result[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestWinCallIncrementBatchEmpty(t *testing.T) {
	client := NewIncrementClient(winTestRunDir, uniqueWinService("go_win_empty_batch"), testWinClientConfig())
	defer client.Close()

	results, err := client.CallIncrementBatch(nil)
	if err != nil {
		t.Fatalf("expected nil error for nil batch, got %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results, got %v", results)
	}

	results, err = client.CallIncrementBatch([]uint64{})
	if err != nil {
		t.Fatalf("expected nil error for empty slice, got %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results, got %v", results)
	}
}

func TestWinRetryOnClosedSession(t *testing.T) {
	svc := uniqueWinService("go_win_retry")
	ts := startTestSnapshotServerWinWithConfig(svc, testWinServerConfig())
	defer ts.stop()

	client := NewSnapshotClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()

	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	if _, err := client.CallSnapshot(); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	client.session.Close()

	view, err := client.CallSnapshot()
	if err != nil {
		t.Fatalf("retry call failed: %v", err)
	}
	if view.ItemCount != 3 {
		t.Fatalf("expected 3 items after retry, got %d", view.ItemCount)
	}

	status := client.Status()
	if status.ReconnectCount < 1 {
		t.Fatalf("expected reconnect_count >= 1, got %d", status.ReconnectCount)
	}
}

func TestWinHandlerFailure(t *testing.T) {
	svc := uniqueWinService("go_win_handler_fail")
	ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodCgroupsSnapshot, winFailingSnapshotDispatchHandler())
	defer ts.stop()

	client := NewSnapshotClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()

	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	if _, err := client.CallSnapshot(); err == nil {
		t.Fatal("expected error when handler fails")
	}

	status := client.Status()
	if status.ErrorCount < 1 {
		t.Fatalf("expected error_count >= 1, got %d", status.ErrorCount)
	}
}

func TestWinStatusReporting(t *testing.T) {
	svc := uniqueWinService("go_win_status")
	ts := startTestSnapshotServerWinWithConfig(svc, testWinServerConfig())

	client := NewSnapshotClient(winTestRunDir, svc, testWinClientConfig())
	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	s0 := client.Status()
	if s0.ConnectCount != 1 || s0.CallCount != 0 || s0.ErrorCount != 0 {
		t.Fatalf("unexpected initial status: %+v", s0)
	}

	for i := 0; i < 3; i++ {
		if _, err := client.CallSnapshot(); err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}

	s1 := client.Status()
	if s1.CallCount != 3 || s1.ErrorCount != 0 {
		t.Fatalf("unexpected status after calls: %+v", s1)
	}

	client.Close()
	if _, err := client.CallSnapshot(); err == nil {
		t.Fatal("expected error on disconnected client")
	}

	s2 := client.Status()
	if s2.ErrorCount != 1 {
		t.Fatalf("expected error_count=1, got %+v", s2)
	}

	ts.stop()
}

func TestWinCacheLookupBeforeRefresh(t *testing.T) {
	cache := NewCache(winTestRunDir, uniqueWinService("go_win_cache_empty"), testWinClientConfig())
	defer cache.Close()

	if cache.Ready() {
		t.Fatal("cache should not be ready")
	}
	if _, found := cache.Lookup(123, "anything"); found {
		t.Fatal("lookup before refresh should miss")
	}
}

func TestWinCacheStatusFresh(t *testing.T) {
	cache := NewCache(winTestRunDir, uniqueWinService("go_win_cache_status"), testWinClientConfig())
	defer cache.Close()

	status := cache.Status()
	if status.Populated || status.ItemCount != 0 || status.RefreshSuccessCount != 0 || status.RefreshFailureCount != 0 || status.LastRefreshTs != 0 {
		t.Fatalf("unexpected fresh cache status: %+v", status)
	}
}

func TestWinCacheFullRoundTrip(t *testing.T) {
	svc := uniqueWinService("go_win_cache_rt")
	ts := startTestSnapshotServerWinWithConfig(svc, testWinServerConfig())

	cache := NewCache(winTestRunDir, svc, testWinClientConfig())
	if cache.Ready() {
		t.Fatal("cache should not be ready before refresh")
	}

	time.Sleep(2 * time.Millisecond)

	if !cache.Refresh() {
		t.Fatal("refresh should succeed")
	}
	if !cache.Ready() {
		t.Fatal("cache should be ready after refresh")
	}

	item, found := cache.Lookup(1001, "docker-abc123")
	if !found {
		t.Fatal("expected cached item")
	}
	if item.Path != "/sys/fs/cgroup/docker/abc123" {
		t.Fatalf("unexpected cached path: %q", item.Path)
	}

	status := cache.Status()
	if !status.Populated || status.ItemCount != 3 || status.SystemdEnabled != 1 || status.Generation != 42 || status.RefreshSuccessCount != 1 || status.RefreshFailureCount != 0 || status.ConnectionState != StateReady || status.LastRefreshTs <= 0 {
		t.Fatalf("unexpected cache status: %+v", status)
	}

	cache.Close()
	ts.stop()
}

func TestWinCacheRefreshNoServer(t *testing.T) {
	cache := NewCache(winTestRunDir, uniqueWinService("go_win_cache_noserver"), testWinClientConfig())
	defer cache.Close()

	if cache.Refresh() {
		t.Fatal("refresh should fail with no server")
	}
	if cache.Ready() {
		t.Fatal("cache should not be ready")
	}
	if cache.Status().RefreshFailureCount != 1 {
		t.Fatalf("expected failure_count=1, got %d", cache.Status().RefreshFailureCount)
	}
}

func TestWinCacheRefreshFailurePreserves(t *testing.T) {
	svc := uniqueWinService("go_win_cache_preserve")
	ts := startTestSnapshotServerWinWithConfig(svc, testWinServerConfig())

	cache := NewCache(winTestRunDir, svc, testWinClientConfig())
	if !cache.Refresh() {
		t.Fatal("first refresh should succeed")
	}
	if !cache.Ready() {
		t.Fatal("cache should be ready")
	}
	if _, found := cache.Lookup(1001, "docker-abc123"); !found {
		t.Fatal("expected first cached item")
	}

	cache.client.Close()
	ts.stop()

	if cache.Refresh() {
		t.Fatal("refresh should fail without server")
	}
	if !cache.Ready() {
		t.Fatal("cache should preserve readiness")
	}
	if _, found := cache.Lookup(1001, "docker-abc123"); !found {
		t.Fatal("old cache data should be preserved")
	}

	status := cache.Status()
	if status.RefreshSuccessCount != 1 || status.RefreshFailureCount < 1 {
		t.Fatalf("unexpected preserved cache status: %+v", status)
	}

	cache.Close()
}

func TestWinCacheReconnectRebuilds(t *testing.T) {
	svc := uniqueWinService("go_win_cache_reconnect")
	ts := startTestSnapshotServerWinWithConfig(svc, testWinServerConfig())
	defer ts.stop()

	cache := NewCache(winTestRunDir, svc, testWinClientConfig())
	defer cache.Close()

	if !cache.Refresh() {
		t.Fatal("first refresh should succeed")
	}

	cache.client.session.Close()

	if !cache.Refresh() {
		t.Fatal("refresh after reconnect should succeed")
	}
	if cache.Status().RefreshSuccessCount != 2 {
		t.Fatalf("expected success_count=2, got %d", cache.Status().RefreshSuccessCount)
	}
}

func TestWinCacheLookupHashNameMismatch(t *testing.T) {
	svc := uniqueWinService("go_win_cache_lookup")
	ts := startTestSnapshotServerWinWithConfig(svc, testWinServerConfig())
	defer ts.stop()

	cache := NewCache(winTestRunDir, svc, testWinClientConfig())
	defer cache.Close()

	if !cache.Refresh() {
		t.Fatal("refresh should succeed")
	}

	if _, found := cache.Lookup(1001, "wrong-name"); found {
		t.Fatal("lookup with wrong name should miss")
	}
	if _, found := cache.Lookup(9999, "docker-abc123"); found {
		t.Fatal("lookup with wrong hash should miss")
	}
	if item, found := cache.Lookup(1001, "docker-abc123"); !found || item.Hash != 1001 {
		t.Fatalf("expected exact hash+name match, got found=%v item=%+v", found, item)
	}
}

func TestWinCacheCloseResetsState(t *testing.T) {
	svc := uniqueWinService("go_win_cache_close")
	ts := startTestSnapshotServerWinWithConfig(svc, testWinServerConfig())

	cache := NewCache(winTestRunDir, svc, testWinClientConfig())
	if !cache.Refresh() {
		t.Fatal("refresh should succeed")
	}
	cache.Close()

	if cache.Ready() {
		t.Fatal("cache should not be ready after close")
	}
	if _, found := cache.Lookup(1001, "docker-abc123"); found {
		t.Fatal("lookup after close should miss")
	}

	ts.stop()
}

func TestWinCacheCustomAndDefaultBufferSize(t *testing.T) {
	customCfg := testWinClientConfig()
	customCfg.MaxResponsePayloadBytes = 1024
	custom := NewCache(winTestRunDir, uniqueWinService("go_win_custom_buf"), customCfg)
	defer custom.Close()

	if got, want := custom.client.maxReceiveMessageBytes(), protocol.HeaderSize+1024; got != want {
		t.Fatalf("custom buffer size = %d, want %d", got, want)
	}

	defaultCfg := testWinClientConfig()
	defaultCfg.MaxResponsePayloadBytes = 0
	def := NewCache(winTestRunDir, uniqueWinService("go_win_default_buf"), defaultCfg)
	defer def.Close()

	if got, want := def.client.maxReceiveMessageBytes(), protocol.HeaderSize+cacheResponseBufSize; got != want {
		t.Fatalf("default buffer size = %d, want %d", got, want)
	}
}

func TestWinCacheLargeDataset(t *testing.T) {
	svc := uniqueWinService("go_win_cache_large")
	const itemCount = 512

	cfg := testWinServerConfig()
	cfg.MaxResponsePayloadBytes = 256 * itemCount
	ts := startTestServerWinWithConfig(svc, cfg, protocol.MethodCgroupsSnapshot, SnapshotDispatch(func(request *protocol.CgroupsRequest, builder *protocol.CgroupsBuilder) bool {
		if request.LayoutVersion != 1 || request.Flags != 0 {
			return false
		}
		builder.SetHeader(1, 100)

		for i := uint32(0); i < itemCount; i++ {
			name := fmt.Sprintf("cgroup-%d", i)
			path := fmt.Sprintf("/sys/fs/cgroup/test/%d", i)
			enabled := uint32(1)
			if i%3 == 0 {
				enabled = 0
			}
			if err := builder.Add(i+1000, 0, enabled, []byte(name), []byte(path)); err != nil {
				return false
			}
		}
		return true
	}, itemCount))
	defer ts.stop()

	ccfg := testWinClientConfig()
	ccfg.MaxResponsePayloadBytes = 256 * itemCount

	cache := NewCache(winTestRunDir, svc, ccfg)
	defer cache.Close()

	if !cache.Refresh() {
		t.Fatal("refresh should succeed")
	}
	if got := cache.Status().ItemCount; got != itemCount {
		t.Fatalf("expected %d items, got %d", itemCount, got)
	}

	for i := uint32(0); i < itemCount; i++ {
		name := fmt.Sprintf("cgroup-%d", i)
		item, found := cache.Lookup(i+1000, name)
		if !found {
			t.Fatalf("item %d not found", i)
		}
		wantPath := fmt.Sprintf("/sys/fs/cgroup/test/%d", i)
		if item.Path != wantPath {
			t.Fatalf("item %d path = %q, want %q", i, item.Path, wantPath)
		}
	}
}

func TestWinIsErrorHelpers(t *testing.T) {
	if !isConnectError(windows.ErrConnect) || !isConnectError(windows.ErrCreatePipe) {
		t.Fatal("connect errors should match")
	}
	if isConnectError(windows.ErrAuthFailed) || isConnectError(windows.ErrNoProfile) || isConnectError(nil) {
		t.Fatal("non-connect errors should not match connect classification")
	}

	if !isAuthError(windows.ErrAuthFailed) || isAuthError(windows.ErrConnect) || isAuthError(nil) {
		t.Fatal("auth error classification mismatch")
	}

	if !isIncompatibleError(windows.ErrNoProfile) || !isIncompatibleError(windows.ErrIncompatible) ||
		isIncompatibleError(windows.ErrAuthFailed) || isIncompatibleError(nil) {
		t.Fatal("incompatible error classification mismatch")
	}
}

func TestWinNonRequestTerminatesSession(t *testing.T) {
	svc := uniqueWinService("go_win_nonreq")
	ts := startTestSnapshotServerWinWithConfig(svc, testWinServerConfig())
	defer ts.stop()

	ccfg := testWinClientConfig()
	session, err := windows.Connect(winTestRunDir, svc, &ccfg)
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

	time.Sleep(200 * time.Millisecond)

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

	verify := NewSnapshotClient(winTestRunDir, svc, testWinClientConfig())
	defer verify.Close()

	verify.Refresh()
	if !verify.Ready() {
		t.Fatal("server should still accept new clients after bad session")
	}

	view, err := verify.CallSnapshot()
	if err != nil {
		t.Fatalf("normal call after bad session failed: %v", err)
	}
	if view.ItemCount != 3 {
		t.Fatalf("expected 3 items after recovery, got %d", view.ItemCount)
	}
}

func TestWinPeerDisconnectDuringResponseKeepsServerHealthy(t *testing.T) {
	svc := uniqueWinService("go_win_send_fail")
	ts := startTestServerWinWithConfig(svc, testWinServerConfig(), protocol.MethodIncrement, IncrementDispatch(func(v uint64) (uint64, bool) {
		time.Sleep(100 * time.Millisecond)
		return v + 1, true
	}))
	defer ts.stop()

	ccfg := testWinClientConfig()
	session, err := windows.Connect(winTestRunDir, svc, &ccfg)
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

	time.Sleep(250 * time.Millisecond)

	verify := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
	defer verify.Close()

	verify.Refresh()
	if !verify.Ready() {
		t.Fatal("server should still accept new clients after peer disconnect during response")
	}

	got, err := verify.CallIncrement(9)
	if err != nil {
		t.Fatalf("normal call after peer disconnect failed: %v", err)
	}
	if got != 10 {
		t.Fatalf("increment after peer disconnect = %d, want 10", got)
	}
}

func TestWinCallSnapshotWithMalformedTransportState(t *testing.T) {
	svc := uniqueWinService("go_win_malformed_state")
	ts := startTestSnapshotServerWinWithConfig(svc, testWinServerConfig())
	defer ts.stop()

	client := NewSnapshotClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()

	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

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

func TestWinCallIncrementBatchWithMalformedTransportState(t *testing.T) {
	svc := uniqueWinService("go_win_batch_malformed_state")

	scfg := testWinServerConfig()
	scfg.MaxRequestBatchItems = 16
	scfg.MaxResponseBatchItems = 16
	ccfg := testWinClientConfig()
	ccfg.MaxRequestBatchItems = 16
	ccfg.MaxResponseBatchItems = 16

	ts := startTestServerWinWithConfig(svc, scfg, protocol.MethodIncrement, winIncrementDispatchHandler())
	defer ts.stop()

	client := NewIncrementClient(winTestRunDir, svc, ccfg)
	defer client.Close()

	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

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

func TestWinCallIncrementRejectsMalformedResponseEnvelope(t *testing.T) {
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
			svc := uniqueWinService("go_win_bad_resp")
			srv := startRawWinSessionServer(t, svc, testWinServerConfig(),
				func(session *windows.Session, hdr protocol.Header, payload []byte) error {
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

			client := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
			defer client.Close()

			client.Refresh()
			if !client.Ready() {
				t.Fatal("client not ready")
			}

			_, err := client.CallIncrement(41)
			if !errors.Is(err, tc.want) {
				t.Fatalf("CallIncrement error = %v, want %v", err, tc.want)
			}

			srv.wait(t)
		})
	}
}

func TestWinCallIncrementRejectsMalformedPayload(t *testing.T) {
	svc := uniqueWinService("go_win_bad_incr_payload")
	srv := startRawWinSessionServer(t, svc, testWinServerConfig(),
		func(session *windows.Session, hdr protocol.Header, payload []byte) error {
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

	client := NewIncrementClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()

	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	_, err := client.CallIncrement(41)
	if !errors.Is(err, protocol.ErrTruncated) {
		t.Fatalf("CallIncrement error = %v, want %v", err, protocol.ErrTruncated)
	}

	srv.wait(t)
}

func TestWinCallStringReverseRejectsMalformedResponseEnvelope(t *testing.T) {
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
			svc := uniqueWinService("go_win_bad_str_resp")
			srv := startRawWinSessionServer(t, svc, testWinServerConfig(),
				func(session *windows.Session, hdr protocol.Header, payload []byte) error {
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

			client := NewStringReverseClient(winTestRunDir, svc, testWinClientConfig())
			defer client.Close()

			client.Refresh()
			if !client.Ready() {
				t.Fatal("client not ready")
			}

			_, err := client.CallStringReverse("hello")
			if !errors.Is(err, tc.want) {
				t.Fatalf("CallStringReverse error = %v, want %v", err, tc.want)
			}

			srv.wait(t)
		})
	}
}

func TestWinCallSnapshotRejectsMalformedPayload(t *testing.T) {
	svc := uniqueWinService("go_win_bad_snapshot_payload")
	srv := startRawWinSessionServer(t, svc, testWinServerConfig(),
		func(session *windows.Session, hdr protocol.Header, payload []byte) error {
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

	client := NewSnapshotClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()

	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	_, err := client.CallSnapshot()
	if !errors.Is(err, protocol.ErrTruncated) {
		t.Fatalf("CallSnapshot error = %v, want %v", err, protocol.ErrTruncated)
	}

	srv.wait(t)
}

func TestWinCallStringReverseRejectsMalformedPayload(t *testing.T) {
	svc := uniqueWinService("go_win_bad_str_payload")
	srv := startRawWinSessionServer(t, svc, testWinServerConfig(),
		func(session *windows.Session, hdr protocol.Header, payload []byte) error {
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

			// Valid offsets/lengths, but missing the required NUL terminator.
			respPayload := make([]byte, protocol.StringReverseHdrSize+3)
			binary.NativeEndian.PutUint32(respPayload[0:4], uint32(protocol.StringReverseHdrSize))
			binary.NativeEndian.PutUint32(respPayload[4:8], 2)
			copy(respPayload[8:], []byte{'o', 'k', 'x'})
			return session.Send(&respHdr, respPayload)
		})

	client := NewStringReverseClient(winTestRunDir, svc, testWinClientConfig())
	defer client.Close()

	client.Refresh()
	if !client.Ready() {
		t.Fatal("client not ready")
	}

	_, err := client.CallStringReverse("hello")
	if !errors.Is(err, protocol.ErrMissingNul) {
		t.Fatalf("CallStringReverse error = %v, want %v", err, protocol.ErrMissingNul)
	}

	srv.wait(t)
}

func TestWinCallIncrementBatchRejectsMalformedResponseEnvelope(t *testing.T) {
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
			cfg := testWinServerConfig()
			cfg.MaxRequestBatchItems = 16
			cfg.MaxResponseBatchItems = 16

			svc := uniqueWinService("go_win_bad_batch_resp")
			srv := startRawWinSessionServer(t, svc, cfg,
				func(session *windows.Session, hdr protocol.Header, payload []byte) error {
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

			ccfg := testWinClientConfig()
			ccfg.MaxRequestBatchItems = 16
			ccfg.MaxResponseBatchItems = 16

			client := NewIncrementClient(winTestRunDir, svc, ccfg)
			defer client.Close()

			client.Refresh()
			if !client.Ready() {
				t.Fatal("client not ready")
			}

			_, err := client.CallIncrementBatch([]uint64{1, 2})
			if !errors.Is(err, tc.want) {
				t.Fatalf("CallIncrementBatch error = %v, want %v", err, tc.want)
			}

			srv.wait(t)
		})
	}
}

func TestWinCallIncrementBatchRejectsMalformedPayload(t *testing.T) {
	t.Run("bad batch directory", func(t *testing.T) {
		cfg := testWinServerConfig()
		cfg.MaxRequestBatchItems = 16
		cfg.MaxResponseBatchItems = 16

		svc := uniqueWinService("go_win_bad_batch_dir")
		srv := startRawWinSessionServerN(t, svc, cfg, 2,
			func(session *windows.Session, hdr protocol.Header, payload []byte) error {
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

		ccfg := testWinClientConfig()
		ccfg.MaxRequestBatchItems = 16
		ccfg.MaxResponseBatchItems = 16

		client := NewIncrementClient(winTestRunDir, svc, ccfg)
		defer client.Close()

		client.Refresh()
		if !client.Ready() {
			t.Fatal("client not ready")
		}

		_, err := client.CallIncrementBatch([]uint64{1, 2})
		if !errors.Is(err, protocol.ErrTruncated) {
			t.Fatalf("CallIncrementBatch error = %v, want %v", err, protocol.ErrTruncated)
		}

		srv.wait(t)
	})

	t.Run("truncated batch item", func(t *testing.T) {
		cfg := testWinServerConfig()
		cfg.MaxRequestBatchItems = 16
		cfg.MaxResponseBatchItems = 16

		svc := uniqueWinService("go_win_bad_batch_payload")
		srv := startRawWinSessionServerN(t, svc, cfg, 2,
			func(session *windows.Session, hdr protocol.Header, payload []byte) error {
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

		ccfg := testWinClientConfig()
		ccfg.MaxRequestBatchItems = 16
		ccfg.MaxResponseBatchItems = 16

		client := NewIncrementClient(winTestRunDir, svc, ccfg)
		defer client.Close()

		client.Refresh()
		if !client.Ready() {
			t.Fatal("client not ready")
		}

		_, err := client.CallIncrementBatch([]uint64{1})
		if !errors.Is(err, protocol.ErrTruncated) {
			t.Fatalf("CallIncrementBatch error = %v, want %v", err, protocol.ErrTruncated)
		}

		srv.wait(t)
	})

	t.Run("missing batch body", func(t *testing.T) {
		cfg := testWinServerConfig()
		cfg.MaxRequestBatchItems = 16
		cfg.MaxResponseBatchItems = 16

		svc := uniqueWinService("go_win_bad_batch_body")
		srv := startRawWinSessionServerN(t, svc, cfg, 2,
			func(session *windows.Session, hdr protocol.Header, payload []byte) error {
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

		ccfg := testWinClientConfig()
		ccfg.MaxRequestBatchItems = 16
		ccfg.MaxResponseBatchItems = 16

		client := NewIncrementClient(winTestRunDir, svc, ccfg)
		defer client.Close()

		client.Refresh()
		if !client.Ready() {
			t.Fatal("client not ready")
		}

		_, err := client.CallIncrementBatch([]uint64{1, 2})
		if !errors.Is(err, protocol.ErrTruncated) {
			t.Fatalf("CallIncrementBatch error = %v, want %v", err, protocol.ErrTruncated)
		}

		srv.wait(t)
	})
}
