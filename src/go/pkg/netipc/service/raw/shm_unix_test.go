//go:build unix

package raw

import (
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

func testUnixShmServerConfig() posix.ServerConfig {
	cfg := testServerConfig()
	cfg.SupportedProfiles = protocol.ProfileBaseline | protocol.ProfileSHMHybrid
	cfg.PreferredProfiles = protocol.ProfileSHMHybrid
	return cfg
}

func testUnixShmClientConfig() posix.ClientConfig {
	cfg := testClientConfig()
	cfg.SupportedProfiles = protocol.ProfileBaseline | protocol.ProfileSHMHybrid
	cfg.PreferredProfiles = protocol.ProfileSHMHybrid
	return cfg
}

func startTestServerUnixWithConfig(
	service string,
	cfg posix.ServerConfig,
	expectedMethodCode uint16,
	handler DispatchHandler,
) *testServer {
	ensureRunDir()
	cleanupAll(service)

	s := NewServer(testRunDir, service, cfg, expectedMethodCode, handler)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		_ = s.Run()
	}()

	waitUnixServerReady(service)
	return &testServer{server: s, doneCh: doneCh}
}

func encodeRawUnixMessage(hdr protocol.Header, payload []byte) []byte {
	hdr.Magic = protocol.MagicMsg
	hdr.Version = protocol.Version
	hdr.HeaderLen = protocol.HeaderLen
	hdr.PayloadLen = uint32(len(payload))

	msg := make([]byte, protocol.HeaderSize+len(payload))
	hdr.Encode(msg[:protocol.HeaderSize])
	copy(msg[protocol.HeaderSize:], payload)
	return msg
}

func encodeUnixIncrementBatchPayload(t *testing.T, values ...uint64) []byte {
	t.Helper()

	itemCount := uint32(len(values))
	if itemCount == 0 {
		return nil
	}

	bufSize := protocol.Align8(int(itemCount)*8) +
		int(itemCount)*protocol.IncrementPayloadSize +
		int(itemCount)*protocol.Alignment
	buf := make([]byte, bufSize)
	entries := make([]protocol.BatchEntry, len(values))

	offset := protocol.Align8(int(itemCount) * 8)
	for i, v := range values {
		itemOff := offset
		itemLen := protocol.IncrementPayloadSize
		entries[i] = protocol.BatchEntry{Offset: uint32(itemOff), Length: uint32(itemLen)}
		if protocol.IncrementEncode(v, buf[itemOff:itemOff+itemLen]) == 0 {
			t.Fatalf("IncrementEncode(%d) failed", v)
		}
		offset += protocol.Align8(itemLen)
	}

	if n := protocol.BatchDirEncode(entries, buf[:int(itemCount)*8]); n != int(itemCount)*8 {
		t.Fatalf("BatchDirEncode returned %d, want %d", n, int(itemCount)*8)
	}

	return buf[:offset]
}

func TestUnixShmRoundTrip(t *testing.T) {
	svc := uniqueUnixService("go_unix_shm_roundtrip")
	ts := startTestServerUnixWithConfig(
		svc,
		testUnixShmServerConfig(),
		protocol.MethodIncrement,
		pingPongIncrementDispatch(),
	)
	defer ts.stop()

	client := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	defer client.Close()

	refreshUnixClientReady(t, client)

	if client.session == nil {
		t.Fatal("expected negotiated session")
	}
	if client.shm == nil {
		t.Fatal("expected SHM attachment")
	}
	if client.session.SelectedProfile != protocol.ProfileSHMHybrid {
		t.Fatalf("selected profile = %d, want %d", client.session.SelectedProfile, protocol.ProfileSHMHybrid)
	}

	got, err := client.CallIncrement(41)
	if err != nil {
		t.Fatalf("CallIncrement failed: %v", err)
	}
	if got != 42 {
		t.Fatalf("CallIncrement = %d, want 42", got)
	}
}

func TestUnixShmAttachFailureFallsBackToBaseline(t *testing.T) {
	svc := uniqueUnixService("go_unix_shm_attach_fail")
	cfg := testUnixShmServerConfig()
	ensureRunDir()
	cleanupAll(svc)

	listener, err := posix.Listen(testRunDir, svc, cfg)
	if err != nil {
		t.Fatalf("posix.Listen failed: %v", err)
	}
	defer listener.Close()

	type attachFailureResult struct {
		firstSelected  uint32
		secondSelected uint32
		err            error
	}
	doneCh := make(chan attachFailureResult, 1)
	go func() {
		first, err := listener.Accept()
		if err != nil {
			doneCh <- attachFailureResult{err: err}
			return
		}
		defer first.Close()
		firstSelected := first.SelectedProfile

		recvBuf := make([]byte, protocol.HeaderSize+4096)
		_, _, err = first.Receive(recvBuf)
		if err == nil {
			doneCh <- attachFailureResult{err: errors.New("expected first receive to fail after SHM attach fallback disconnect")}
			return
		}

		second, err := listener.Accept()
		if err != nil {
			doneCh <- attachFailureResult{err: err}
			return
		}
		defer second.Close()
		secondSelected := second.SelectedProfile

		_, _, err = second.Receive(recvBuf)
		if err == nil {
			doneCh <- attachFailureResult{err: errors.New("expected second receive to fail after client close")}
			return
		}

		doneCh <- attachFailureResult{
			firstSelected:  firstSelected,
			secondSelected: secondSelected,
		}
	}()

	client := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())

	if changed := client.Refresh(); !changed {
		t.Fatal("refresh should transition to READY via baseline fallback after SHM attach failure")
	}
	if !client.Ready() {
		t.Fatal("client should be ready after baseline fallback")
	}
	if client.state != StateReady {
		t.Fatalf("client state = %d, want READY", client.state)
	}
	if client.shm != nil {
		t.Fatal("expected no SHM attachment after fallback")
	}
	if client.session == nil {
		t.Fatal("expected live baseline session after fallback")
	}
	if client.session.SelectedProfile != protocol.ProfileBaseline {
		t.Fatalf("selected profile after fallback = %#x, want baseline", client.session.SelectedProfile)
	}
	if client.config.SupportedProfiles&protocol.ProfileSHMHybrid != 0 {
		t.Fatalf("supported profiles should drop SHM after attach failure, got %#x", client.config.SupportedProfiles)
	}
	if client.config.PreferredProfiles&protocol.ProfileSHMHybrid != 0 {
		t.Fatalf("preferred profiles should drop SHM after attach failure, got %#x", client.config.PreferredProfiles)
	}

	client.Close()

	result := <-doneCh
	if result.err != nil {
		t.Fatalf("attach-failure fallback server failed: %v", result.err)
	}
	if result.firstSelected != protocol.ProfileSHMHybrid {
		t.Fatalf("first selected profile = %#x, want SHM hybrid", result.firstSelected)
	}
	if result.secondSelected != protocol.ProfileBaseline {
		t.Fatalf("second selected profile = %#x, want baseline", result.secondSelected)
	}
}

func TestUnixDoRawCallShmRejectsBadMessageID(t *testing.T) {
	client, serverShm := newRawPosixShmClient(t)

	serverDone := make(chan error, 1)
	go func() {
		reqBuf := make([]byte, protocol.HeaderSize+32)
		n, err := serverShm.ShmReceive(reqBuf, 1000)
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
			serverDone <- errors.New("IncrementEncode failed")
			return
		}

		respHdr := protocol.Header{
			Kind:            protocol.KindResponse,
			Code:            protocol.MethodIncrement,
			ItemCount:       1,
			MessageID:       hdr.MessageID + 1,
			TransportStatus: protocol.StatusOK,
		}
		serverDone <- serverShm.ShmSend(encodeRawUnixMessage(respHdr, respPayload[:]))
	}()

	var reqPayload [protocol.IncrementPayloadSize]byte
	if protocol.IncrementEncode(41, reqPayload[:]) == 0 {
		t.Fatal("IncrementEncode failed")
	}

	_, _, err := client.doRawCall(protocol.MethodIncrement, reqPayload[:])
	if !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("doRawCall bad SHM message_id = %v, want %v", err, protocol.ErrBadLayout)
	}

	if err := <-serverDone; err != nil {
		t.Fatalf("raw POSIX SHM server failed: %v", err)
	}
}

func TestUnixCallIncrementBatchShmRejectsBadMessageID(t *testing.T) {
	client, serverShm := newRawPosixShmClient(t)

	serverDone := make(chan error, 1)
	go func() {
		reqBuf := make([]byte, protocol.HeaderSize+256)
		n, err := serverShm.ShmReceive(reqBuf, 1000)
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
			serverDone <- errors.New("IncrementEncode failed")
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
		serverDone <- serverShm.ShmSend(encodeRawUnixMessage(respHdr, respBuf[:respLen]))
	}()

	_, err := client.CallIncrementBatch([]uint64{1, 2})
	if !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("CallIncrementBatch bad SHM message_id = %v, want %v", err, protocol.ErrBadLayout)
	}

	if err := <-serverDone; err != nil {
		t.Fatalf("raw POSIX SHM server failed: %v", err)
	}
}

func TestUnixCallIncrementBatchShmRejectsMalformedPayload(t *testing.T) {
	client, serverShm := newRawPosixShmClient(t)

	serverDone := make(chan error, 1)
	go func() {
		reqBuf := make([]byte, protocol.HeaderSize+256)
		n, err := serverShm.ShmReceive(reqBuf, 1000)
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
		serverDone <- serverShm.ShmSend(encodeRawUnixMessage(respHdr, make([]byte, 8)))
	}()

	_, err := client.CallIncrementBatch([]uint64{1, 2})
	if !errors.Is(err, protocol.ErrTruncated) {
		t.Fatalf("CallIncrementBatch malformed SHM payload = %v, want %v", err, protocol.ErrTruncated)
	}

	if err := <-serverDone; err != nil {
		t.Fatalf("raw POSIX SHM server failed: %v", err)
	}
}

func TestUnixShmMalformedBatchRequestRecovers(t *testing.T) {
	svc := uniqueUnixService("go_unix_shm_bad_batch")
	ts := startTestServerUnixWithConfig(
		svc,
		testUnixShmServerConfig(),
		protocol.MethodIncrement,
		pingPongIncrementDispatch(),
	)
	defer ts.stop()

	client := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	defer client.Close()

	refreshUnixClientReady(t, client)

	if client.shm == nil {
		t.Fatal("expected SHM attachment")
	}

	reqHdr := protocol.Header{
		Kind:            protocol.KindRequest,
		Code:            protocol.MethodIncrement,
		Flags:           protocol.FlagBatch,
		ItemCount:       2,
		MessageID:       1,
		TransportStatus: protocol.StatusOK,
	}
	badPayload := encodeUnixIncrementBatchPayload(t, 1, 2)[:8]
	if err := client.shm.ShmSend(encodeRawUnixMessage(reqHdr, badPayload)); err != nil {
		t.Fatalf("ShmSend malformed batch request failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	got, err := client.CallIncrement(21)
	if err != nil {
		t.Fatalf("CallIncrement after malformed batch SHM request failed: %v", err)
	}
	if got != 22 {
		t.Fatalf("CallIncrement after malformed batch SHM request = %d, want 22", got)
	}
	if !client.Ready() {
		t.Fatalf("client should stay usable after malformed batch SHM request, got status %+v", client.Status())
	}
}

func TestUnixShmShortRequestKeepsServerHealthy(t *testing.T) {
	svc := uniqueUnixService("go_unix_shm_short_req")
	ts := startTestServerUnixWithConfig(
		svc,
		testUnixShmServerConfig(),
		protocol.MethodIncrement,
		pingPongIncrementDispatch(),
	)
	defer ts.stop()

	client := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	defer client.Close()
	refreshUnixClientReady(t, client)

	if client.shm == nil {
		t.Fatal("expected SHM attachment")
	}
	if err := client.shm.ShmSend([]byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("ShmSend short request failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	verify := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	defer verify.Close()
	refreshUnixClientReady(t, verify)

	got, err := verify.CallIncrement(21)
	if err != nil {
		t.Fatalf("CallIncrement after short SHM request failed: %v", err)
	}
	if got != 22 {
		t.Fatalf("CallIncrement after short SHM request = %d, want 22", got)
	}
}

func TestUnixShmBadHeaderKeepsServerHealthy(t *testing.T) {
	svc := uniqueUnixService("go_unix_shm_bad_header")
	ts := startTestServerUnixWithConfig(
		svc,
		testUnixShmServerConfig(),
		protocol.MethodIncrement,
		pingPongIncrementDispatch(),
	)
	defer ts.stop()

	client := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	defer client.Close()
	refreshUnixClientReady(t, client)

	if client.shm == nil {
		t.Fatal("expected SHM attachment")
	}
	badMsg := make([]byte, protocol.HeaderSize)
	if err := client.shm.ShmSend(badMsg); err != nil {
		t.Fatalf("ShmSend bad header request failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	verify := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	defer verify.Close()
	refreshUnixClientReady(t, verify)

	got, err := verify.CallIncrement(9)
	if err != nil {
		t.Fatalf("CallIncrement after bad SHM header failed: %v", err)
	}
	if got != 10 {
		t.Fatalf("CallIncrement after bad SHM header = %d, want 10", got)
	}
}

func TestUnixShmBatchHandlerFailureNeedsRefresh(t *testing.T) {
	svc := uniqueUnixService("go_unix_shm_batch_fail")
	handler := IncrementDispatch(func(v uint64) (uint64, bool) {
		if v == 99 {
			return 0, false
		}
		return v + 1, true
	})
	ts := startTestServerUnixWithConfig(
		svc,
		testUnixShmServerConfig(),
		protocol.MethodIncrement,
		handler,
	)
	defer ts.stop()

	client := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	defer client.Close()

	refreshUnixClientReady(t, client)

	_, err := client.CallIncrementBatch([]uint64{99})
	if !errors.Is(err, protocol.ErrBadLayout) {
		t.Fatalf("CallIncrementBatch handler failure = %v, want %v", err, protocol.ErrBadLayout)
	}
	if client.state != StateBroken {
		t.Fatalf("client state after batch handler failure = %d, want BROKEN", client.state)
	}

	changed := client.Refresh()
	if !changed || !client.Ready() {
		t.Fatalf("Refresh after batch handler failure = %v, state=%d, want READY", changed, client.state)
	}

	got, err := client.CallIncrement(5)
	if err != nil {
		t.Fatalf("CallIncrement after Refresh failed: %v", err)
	}
	if got != 6 {
		t.Fatalf("CallIncrement after Refresh = %d, want 6", got)
	}
}

func TestUnixShmBatchResponseOverflowRetriesAndRecovers(t *testing.T) {
	svc := uniqueUnixService("go_unix_shm_batch_overflow")

	scfg := testUnixShmServerConfig()
	scfg.MaxResponsePayloadBytes = 24
	scfg.MaxResponseBatchItems = 2
	ccfg := testUnixShmClientConfig()
	ccfg.MaxResponsePayloadBytes = 24
	ccfg.MaxResponseBatchItems = 2
	ccfg.MaxRequestBatchItems = 2

	ts := startTestServerUnixWithConfig(
		svc,
		scfg,
		protocol.MethodIncrement,
		IncrementDispatch(func(v uint64) (uint64, bool) {
			return v + 1, true
		}),
	)
	defer ts.stop()

	client := NewIncrementClient(testRunDir, svc, ccfg)
	defer client.Close()

	refreshUnixClientReady(t, client)

	gotBatch, err := client.CallIncrementBatch([]uint64{1, 2})
	if err != nil {
		t.Fatalf("CallIncrementBatch after SHM overflow failed: %v", err)
	}
	if len(gotBatch) != 2 || gotBatch[0] != 2 || gotBatch[1] != 3 {
		t.Fatalf("CallIncrementBatch after SHM overflow = %v, want [2 3]", gotBatch)
	}
	if client.state != StateReady {
		t.Fatalf("client state after batch overflow recovery = %d, want READY", client.state)
	}
	if client.reconnectCount < 1 {
		t.Fatalf("expected reconnect_count >= 1 after SHM batch overflow, got %d", client.reconnectCount)
	}

	got, err := client.CallIncrement(8)
	if err != nil {
		t.Fatalf("CallIncrement after SHM overflow recovery failed: %v", err)
	}
	if got != 9 {
		t.Fatalf("CallIncrement after SHM overflow recovery = %d, want 9", got)
	}
}
