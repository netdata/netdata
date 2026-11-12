//go:build linux || (freebsd && (amd64 || 386))

// SHM service tests: the POSIX SHM data plane exists on Linux only.

package raw

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/posix"
)

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

	// The listener negotiates SHM but never creates a region, so the attach
	// window runs out; the test window keeps that in milliseconds.
	client := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig()).useTestTimings()

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

	result := recvWithin(t, doneCh, "attach-failure server")
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

// serveSessionsUntilClosed accepts n sessions on listener, one after the
// other, and records each one's selected profile; each session is served
// until the client closes it.
func serveSessionsUntilClosed(listener *posix.Listener, n int) <-chan []uint32 {
	done := make(chan []uint32, 1)
	go func() {
		var selected []uint32
		recvBuf := make([]byte, protocol.HeaderSize+4096)
		for range n {
			session, err := listener.Accept()
			if err != nil {
				break
			}
			selected = append(selected, session.SelectedProfile)
			_, _, _ = session.Receive(recvBuf)
			session.Close()
		}
		done <- selected
	}()
	return done
}

// A region that never becomes attachable, with a call timeout below the SHM
// attach timeout: every attach wait ends at the call deadline. The first one
// keeps SHM; the second in a row drops it, so the third connect runs on the
// baseline instead of every call ending at its deadline.
func TestUnixShmPersistentCallBoundAttachFailureFallsBack(t *testing.T) {
	svc := uniqueUnixService("go_unix_shm_call_bound")
	ensureRunDir()
	cleanupAll(svc)

	listener, err := posix.Listen(testRunDir, svc, testUnixShmServerConfig())
	if err != nil {
		t.Fatalf("posix.Listen failed: %v", err)
	}
	defer listener.Close()
	doneCh := serveSessionsUntilClosed(listener, 3)

	client := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	client.shmAttachTimeout = 5 * time.Second
	client.SetCallTimeout(100)

	if client.Refresh(); client.Ready() {
		t.Fatal("first refresh should end at the call deadline")
	}
	if client.config.SupportedProfiles&protocol.ProfileSHMHybrid == 0 {
		t.Fatal("one call-bound attach failure should keep SHM")
	}
	if client.Refresh(); client.Ready() {
		t.Fatal("second refresh should end at the call deadline")
	}
	if client.config.SupportedProfiles&protocol.ProfileSHMHybrid != 0 {
		t.Fatal("a second call-bound attach failure in a row should drop SHM")
	}
	if !client.Refresh() || !client.Ready() {
		t.Fatal("third refresh should connect over the baseline")
	}
	if client.shm != nil || client.session.SelectedProfile != protocol.ProfileBaseline {
		t.Fatalf("selected profile = %#x, want baseline without SHM", client.session.SelectedProfile)
	}
	client.Close()

	want := []uint32{protocol.ProfileSHMHybrid, protocol.ProfileSHMHybrid, protocol.ProfileBaseline}
	if got := recvWithin(t, doneCh, "session server"); !slices.Equal(got, want) {
		t.Fatalf("server selected profiles = %#x, want %#x", got, want)
	}
}

// An attach error that waiting cannot fix (a region of another version) drops
// SHM at once, even when the call deadline would end the wait first.
func TestUnixShmPermanentAttachErrorFallsBackAtOnce(t *testing.T) {
	svc := uniqueUnixService("go_unix_shm_bad_ver")
	ensureRunDir()
	cleanupAll(svc)

	listener, err := posix.Listen(testRunDir, svc, testUnixShmServerConfig())
	if err != nil {
		t.Fatalf("posix.Listen failed: %v", err)
	}
	defer listener.Close()

	// The first session gets id 1; its region carries a foreign version.
	region := make([]byte, 64)
	binary.NativeEndian.PutUint32(region[0:4], 0x4e53484d)
	binary.NativeEndian.PutUint16(region[4:6], 0xffff)
	regionPath := filepath.Join(testRunDir, fmt.Sprintf("%s-%016x.ipcshm", svc, 1))
	if err := os.WriteFile(regionPath, region, 0o600); err != nil {
		t.Fatalf("write region: %v", err)
	}
	defer os.Remove(regionPath)
	doneCh := serveSessionsUntilClosed(listener, 2)

	client := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	client.shmAttachTimeout = 5 * time.Second
	client.SetCallTimeout(2000)

	if !client.Refresh() || !client.Ready() {
		t.Fatal("refresh should fall back to the baseline in the same call")
	}
	if client.shm != nil || client.session.SelectedProfile != protocol.ProfileBaseline {
		t.Fatalf("selected profile = %#x, want baseline without SHM", client.session.SelectedProfile)
	}
	client.Close()

	want := []uint32{protocol.ProfileSHMHybrid, protocol.ProfileBaseline}
	if got := recvWithin(t, doneCh, "session server"); !slices.Equal(got, want) {
		t.Fatalf("server selected profiles = %#x, want %#x", got, want)
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

	if err := recvWithin(t, serverDone, "raw SHM server"); err != nil {
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

	if err := recvWithin(t, serverDone, "raw SHM server"); err != nil {
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

	if err := recvWithin(t, serverDone, "raw SHM server"); err != nil {
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

	// The server answers the malformed batch with an error status and keeps
	// the session.
	respBuf := make([]byte, protocol.HeaderSize+4096)
	n, err := client.shm.ShmReceive(respBuf, 5000)
	if err != nil {
		t.Fatalf("ShmReceive reply to malformed batch failed: %v", err)
	}
	respHdr, err := protocol.DecodeHeader(respBuf[:n])
	if err != nil {
		t.Fatalf("decode reply to malformed batch: %v", err)
	}
	if respHdr.MessageID != 1 || respHdr.TransportStatus == protocol.StatusOK {
		t.Fatalf("reply to malformed batch = id %d status %d, want id 1 with an error status",
			respHdr.MessageID, respHdr.TransportStatus)
	}

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

	// The server ends the session on the malformed message.
	waitSessionExits(ts.server, 1)

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

	// The server ends the session on the malformed message.
	waitSessionExits(ts.server, 1)

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

func TestUnixShmBatchResponseOverflowRespectsConfiguredCeiling(t *testing.T) {
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
	if !errors.Is(err, protocol.ErrOverflow) {
		t.Fatalf("CallIncrementBatch with capped SHM response = %v, want ErrOverflow", err)
	}
	if gotBatch != nil {
		t.Fatalf("CallIncrementBatch with capped SHM response returned %v, want nil", gotBatch)
	}
	if client.state != StateBroken {
		t.Fatalf("client state after capped batch overflow = %d, want BROKEN", client.state)
	}
	if client.reconnectCount < 1 {
		t.Fatalf("expected reconnect_count >= 1 after capped SHM batch overflow, got %d", client.reconnectCount)
	}
}

func TestUnixShmRegionOnlyForShmSessions(t *testing.T) {
	svc := uniqueUnixService("go_unix_shm_only_selected")
	ts := startServerWithConfigSocketReady(svc, testUnixShmServerConfig(), protocol.MethodIncrement,
		IncrementDispatch(func(v uint64) (uint64, bool) { return v + 1, true }))
	defer ts.stop()
	s := ts.server

	baseline := NewIncrementClient(testRunDir, svc, testClientConfig())
	baseline.config.SupportedProfiles = protocol.ProfileBaseline
	baseline.config.PreferredProfiles = protocol.ProfileBaseline
	defer baseline.Close()
	baseline.Refresh()
	if !baseline.Ready() {
		t.Fatalf("baseline client not ready, state=%d", baseline.state)
	}
	if n := shmCreatedCount(s); n != 0 {
		t.Fatalf("baseline session created %d SHM regions, want 0", n)
	}

	badAuth := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	badAuth.config.AuthToken++
	defer badAuth.Close()
	badAuth.Refresh()
	if badAuth.Ready() {
		t.Fatal("client with a wrong auth token must not become ready")
	}
	if n := shmCreatedCount(s); n != 0 {
		t.Fatalf("rejected handshake created %d SHM regions, want 0", n)
	}

	shmClient := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	defer shmClient.Close()
	shmClient.Refresh()
	if !shmClient.Ready() || shmClient.session.SelectedProfile != protocol.ProfileSHMHybrid {
		t.Fatalf("SHM client not ready over SHM, state=%d session=%+v", shmClient.state, shmClient.session)
	}
	if n := shmCreatedCount(s); n != 1 {
		t.Fatalf("SHM session created %d SHM regions, want 1", n)
	}

	s.failShmCreate.Store(true)
	fallback := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	defer fallback.Close()
	fallback.Refresh()
	if !fallback.Ready() || fallback.session.SelectedProfile != protocol.ProfileBaseline {
		t.Fatalf("client after SHM create failure not ready over baseline, state=%d session=%+v",
			fallback.state, fallback.session)
	}
	if n := shmCreatedCount(s); n != 0 {
		t.Fatalf("failed SHM create reported %d regions, want 0", n)
	}
}

func TestUnixShmPrepareFailureFallsBackToBaseline(t *testing.T) {
	svc := uniqueUnixService("go_unix_shm_create_fail")
	obstruction := unixShmSessionPath(svc, 1)

	scfg := testUnixShmServerConfig()
	ts := startServerWithConfigSocketReady(
		svc,
		scfg,
		protocol.MethodIncrement,
		IncrementDispatch(func(v uint64) (uint64, bool) {
			return v + 1, true
		}),
	)
	defer func() {
		ts.stop()
		_ = os.RemoveAll(obstruction)
	}()

	if err := os.MkdirAll(filepath.Join(obstruction, "keep"), 0700); err != nil {
		t.Fatalf("mkdir SHM obstruction failed: %v", err)
	}

	badClient := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	defer badClient.Close()

	if changed := badClient.Refresh(); !changed {
		t.Fatal("Refresh should transition to READY when baseline stays viable")
	}
	if !badClient.Ready() {
		t.Fatal("client should stay usable over baseline after server SHM prepare failure")
	}
	if badClient.state != StateReady {
		t.Fatalf("client state after SHM prepare failure = %d, want READY", badClient.state)
	}
	if badClient.shm != nil {
		t.Fatalf("fallback baseline session must not attach SHM: selected=%#x session=%+v", badClient.session.SelectedProfile, badClient.session)
	}
	if badClient.session == nil || badClient.session.SelectedProfile != protocol.ProfileBaseline {
		t.Fatalf("selected profile after SHM prepare failure = %#v, want baseline", badClient.session)
	}

	if _, err := os.Stat(obstruction); err != nil {
		t.Fatalf("SHM obstruction should still exist after failed create, stat err = %v", err)
	}
	if err := os.RemoveAll(obstruction); err != nil {
		t.Fatalf("remove SHM obstruction failed: %v", err)
	}

	goodClient := NewIncrementClient(testRunDir, svc, testUnixShmClientConfig())
	defer goodClient.Close()
	refreshUnixClientReady(t, goodClient)

	if goodClient.shm == nil {
		t.Fatal("expected SHM attachment after obstruction removal")
	}
	got, err := goodClient.CallIncrement(4)
	if err != nil {
		t.Fatalf("CallIncrement after SHM create recovery failed: %v", err)
	}
	if got != 5 {
		t.Fatalf("CallIncrement after SHM create recovery = %d, want 5", got)
	}
}

func TestUnixClientTransportReceiveShmError(t *testing.T) {
	client := NewIncrementClient(testRunDir, uniqueUnixService("go_unix_transport_shm_err"), testClientConfig())
	defer client.Close()

	client.state = StateReady
	client.shm = &posix.ShmContext{}

	if _, _, err := client.transportReceive(); !errors.Is(err, protocol.ErrTransport) {
		t.Fatalf("transportReceive with invalid SHM context = %v, want %v", err, protocol.ErrTransport)
	}
}

func TestUnixClientTransportReceiveShmRejectsShortMessage(t *testing.T) {
	client, serverShm := newRawPosixShmClient(t)

	if err := serverShm.ShmSend([]byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("ShmSend failed: %v", err)
	}

	if _, _, err := client.transportReceive(); !errors.Is(err, protocol.ErrTruncated) {
		t.Fatalf("transportReceive short SHM message = %v, want %v", err, protocol.ErrTruncated)
	}
}

func TestUnixClientTransportReceiveShmRejectsBadHeader(t *testing.T) {
	client, serverShm := newRawPosixShmClient(t)

	msg := make([]byte, protocol.HeaderSize)
	if err := serverShm.ShmSend(msg); err != nil {
		t.Fatalf("ShmSend failed: %v", err)
	}

	if _, _, err := client.transportReceive(); !errors.Is(err, protocol.ErrBadMagic) {
		t.Fatalf("transportReceive bad SHM header = %v, want %v", err, protocol.ErrBadMagic)
	}
}
