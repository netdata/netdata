//go:build windows

package raw

import (
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

func TestWinShmRoundTrip(t *testing.T) {
	t.Run("snapshot", func(t *testing.T) {
		svc := uniqueWinService("go_win_shm_roundtrip_snapshot")
		ts := startTestSnapshotServerWinWithConfig(svc, testWinShmServerConfig())
		defer ts.stop()

		client := NewSnapshotClient(winTestRunDir, svc, testWinShmClientConfig())
		defer client.Close()

		waitWinClientReady(t, client)

		if client.session == nil {
			t.Fatal("expected negotiated session")
		}
		if client.shm == nil {
			t.Fatal("expected WinSHM attachment")
		}
		if client.session.SelectedProfile != windows.WinShmProfileHybrid {
			t.Fatalf("selected profile = %d, want %d", client.session.SelectedProfile, windows.WinShmProfileHybrid)
		}

		view, err := client.CallSnapshot()
		if err != nil {
			t.Fatalf("CallSnapshot failed: %v", err)
		}
		if view.ItemCount != 3 {
			t.Fatalf("snapshot item count = %d, want 3", view.ItemCount)
		}
		if status := client.Status(); status.CallCount != 1 || status.ErrorCount != 0 {
			t.Fatalf("unexpected client status: %+v", status)
		}
	})

	t.Run("increment", func(t *testing.T) {
		svc := uniqueWinService("go_win_shm_roundtrip_increment")
		ts := startTestIncrementServerWinWithConfig(svc, testWinShmServerConfig())
		defer ts.stop()

		client := NewIncrementClient(winTestRunDir, svc, testWinShmClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

		got, err := client.CallIncrement(41)
		if err != nil {
			t.Fatalf("CallIncrement failed: %v", err)
		}
		if got != 42 {
			t.Fatalf("increment result = %d, want 42", got)
		}

		batch, err := client.CallIncrementBatch([]uint64{1, 41, 99})
		if err != nil {
			t.Fatalf("CallIncrementBatch failed: %v", err)
		}
		wantBatch := []uint64{2, 42, 100}
		if len(batch) != len(wantBatch) {
			t.Fatalf("batch result len = %d, want %d", len(batch), len(wantBatch))
		}
		for i, want := range wantBatch {
			if batch[i] != want {
				t.Fatalf("batch[%d] = %d, want %d", i, batch[i], want)
			}
		}
	})

	t.Run("string-reverse", func(t *testing.T) {
		svc := uniqueWinService("go_win_shm_roundtrip_reverse")
		ts := startTestStringReverseServerWinWithConfig(svc, testWinShmServerConfig())
		defer ts.stop()

		client := NewStringReverseClient(winTestRunDir, svc, testWinShmClientConfig())
		defer client.Close()
		waitWinClientReady(t, client)

		reversed, err := client.CallStringReverse("hello")
		if err != nil {
			t.Fatalf("CallStringReverse failed: %v", err)
		}
		if reversed.Str != "olleh" {
			t.Fatalf("string reverse result = %q, want %q", reversed.Str, "olleh")
		}
	})
}

func TestWinShmIdleTimeoutKeepsSessionAlive(t *testing.T) {
	svc := uniqueWinService("go_win_shm_idle")
	ts := startTestIncrementServerWinWithConfig(svc, testWinShmServerConfig())
	defer ts.stop()

	client := NewIncrementClient(winTestRunDir, svc, testWinShmClientConfig())
	defer client.Close()

	waitWinClientReady(t, client)

	time.Sleep(350 * time.Millisecond)

	got, err := client.CallIncrement(9)
	if err != nil {
		t.Fatalf("CallIncrement after idle timeout failed: %v", err)
	}
	if got != 10 {
		t.Fatalf("CallIncrement after idle timeout = %d, want 10", got)
	}
}

func TestWinServerRunInvalidServiceName(t *testing.T) {
	server := NewServer(winTestRunDir, "bad/name", testWinServerConfig(), protocol.MethodIncrement, winIncrementDispatchHandler())
	if err := server.Run(); err == nil {
		t.Fatal("expected Run() to fail for invalid service name")
	}
}

func TestWinShmAttachFailureFallsBackToBaseline(t *testing.T) {
	svc := uniqueWinService("go_win_shm_attach_fail")
	cfg := testWinShmServerConfig()

	listener, err := windows.Listen(winTestRunDir, svc, cfg)
	if err != nil {
		t.Fatalf("windows.Listen failed: %v", err)
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
			doneCh <- attachFailureResult{err: errors.New("expected first receive to fail after WinSHM attach fallback disconnect")}
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

	client := NewIncrementClient(winTestRunDir, svc, testWinShmClientConfig())

	if changed := client.Refresh(); !changed {
		t.Fatal("refresh should transition to READY via baseline fallback after WinSHM attach failure")
	}
	if !client.Ready() {
		t.Fatal("client should be ready after baseline fallback")
	}
	if client.state != StateReady {
		t.Fatalf("client state = %d, want READY", client.state)
	}
	if client.shm != nil {
		t.Fatal("expected no WinSHM attachment after fallback")
	}
	if client.session == nil {
		t.Fatal("expected live baseline session after fallback")
	}
	if client.session.SelectedProfile != protocol.ProfileBaseline {
		t.Fatalf("selected profile after fallback = %#x, want baseline", client.session.SelectedProfile)
	}
	if client.config.SupportedProfiles&windows.WinShmProfileHybrid != 0 {
		t.Fatalf("supported profiles should drop WinSHM after attach failure, got %#x", client.config.SupportedProfiles)
	}
	if client.config.PreferredProfiles&windows.WinShmProfileHybrid != 0 {
		t.Fatalf("preferred profiles should drop WinSHM after attach failure, got %#x", client.config.PreferredProfiles)
	}

	client.Close()

	result := <-doneCh
	if result.err != nil {
		t.Fatalf("attach-failure fallback server failed: %v", result.err)
	}
	if result.firstSelected != windows.WinShmProfileHybrid {
		t.Fatalf("first selected profile = %#x, want WinSHM hybrid", result.firstSelected)
	}
	if result.secondSelected != protocol.ProfileBaseline {
		t.Fatalf("second selected profile = %#x, want baseline", result.secondSelected)
	}
}

func TestWinShmMalformedShortRequestRecovers(t *testing.T) {
	svc := uniqueWinService("go_win_shm_short_req")
	ts := startTestIncrementServerWinWithConfig(svc, testWinShmServerConfig())
	defer ts.stop()

	client := NewIncrementClient(winTestRunDir, svc, testWinShmClientConfig())
	defer client.Close()

	waitWinClientReady(t, client)

	if client.shm == nil {
		t.Fatal("expected WinSHM attachment")
	}
	if err := client.shm.WinShmSend([]byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("WinShmSend malformed short request failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	got, err := client.CallIncrement(9)
	if err != nil {
		t.Fatalf("CallIncrement after malformed short WinSHM request failed: %v", err)
	}
	if got != 10 {
		t.Fatalf("CallIncrement after malformed short WinSHM request = %d, want 10", got)
	}
	if client.Status().ReconnectCount < 1 {
		t.Fatalf("expected reconnect after malformed short WinSHM request, got status %+v", client.Status())
	}
}

func TestWinShmMalformedHeaderRequestRecovers(t *testing.T) {
	svc := uniqueWinService("go_win_shm_bad_hdr")
	ts := startTestIncrementServerWinWithConfig(svc, testWinShmServerConfig())
	defer ts.stop()

	client := NewIncrementClient(winTestRunDir, svc, testWinShmClientConfig())
	defer client.Close()

	waitWinClientReady(t, client)

	if client.shm == nil {
		t.Fatal("expected WinSHM attachment")
	}
	msg := make([]byte, protocol.HeaderSize)
	if err := client.shm.WinShmSend(msg); err != nil {
		t.Fatalf("WinShmSend malformed header request failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	got, err := client.CallIncrement(11)
	if err != nil {
		t.Fatalf("CallIncrement after malformed header WinSHM request failed: %v", err)
	}
	if got != 12 {
		t.Fatalf("CallIncrement after malformed header WinSHM request = %d, want 12", got)
	}
	if client.Status().ReconnectCount < 1 {
		t.Fatalf("expected reconnect after malformed header WinSHM request, got status %+v", client.Status())
	}
}

func TestWinShmUnexpectedMessageKindRecovers(t *testing.T) {
	svc := uniqueWinService("go_win_shm_bad_kind")
	ts := startTestIncrementServerWinWithConfig(svc, testWinShmServerConfig())
	defer ts.stop()

	client := NewIncrementClient(winTestRunDir, svc, testWinShmClientConfig())
	defer client.Close()

	waitWinClientReady(t, client)

	if client.shm == nil {
		t.Fatal("expected WinSHM attachment")
	}

	reqHdr := protocol.Header{
		Kind:            protocol.KindResponse,
		Code:            protocol.MethodIncrement,
		ItemCount:       1,
		MessageID:       1,
		TransportStatus: protocol.StatusOK,
	}
	var reqPayload [protocol.IncrementPayloadSize]byte
	if protocol.IncrementEncode(9, reqPayload[:]) == 0 {
		t.Fatal("IncrementEncode failed")
	}

	if err := client.shm.WinShmSend(encodeRawWinMessage(reqHdr, reqPayload[:])); err != nil {
		t.Fatalf("WinShmSend unexpected-kind request failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	got, err := client.CallIncrement(13)
	if err != nil {
		t.Fatalf("CallIncrement after unexpected-kind WinSHM request failed: %v", err)
	}
	if got != 14 {
		t.Fatalf("CallIncrement after unexpected-kind WinSHM request = %d, want 14", got)
	}
	if client.Status().ReconnectCount < 1 {
		t.Fatalf("expected reconnect after unexpected-kind WinSHM request, got status %+v", client.Status())
	}
}

func TestWinShmMalformedBatchRequestRecovers(t *testing.T) {
	svc := uniqueWinService("go_win_shm_bad_batch")
	ts := startTestIncrementServerWinWithConfig(svc, testWinShmServerConfig())
	defer ts.stop()

	client := NewIncrementClient(winTestRunDir, svc, testWinShmClientConfig())
	defer client.Close()

	waitWinClientReady(t, client)

	if client.shm == nil {
		t.Fatal("expected WinSHM attachment")
	}

	reqHdr := protocol.Header{
		Kind:            protocol.KindRequest,
		Code:            protocol.MethodIncrement,
		Flags:           protocol.FlagBatch,
		ItemCount:       2,
		MessageID:       1,
		TransportStatus: protocol.StatusOK,
	}
	// ItemCount=2 requires a 16-byte aligned directory. An 8-byte payload
	// forces BatchItemGet() to fail in the server batch loop.
	badPayload := encodeWinIncrementBatchPayload(t, 1, 2)[:8]
	if err := client.shm.WinShmSend(encodeRawWinMessage(reqHdr, badPayload)); err != nil {
		t.Fatalf("WinShmSend malformed batch request failed: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	got, err := client.CallIncrement(21)
	if err != nil {
		t.Fatalf("CallIncrement after malformed batch WinSHM request failed: %v", err)
	}
	if got != 22 {
		t.Fatalf("CallIncrement after malformed batch WinSHM request = %d, want 22", got)
	}
	if !client.Ready() {
		t.Fatalf("client should stay usable after malformed batch WinSHM request, got status %+v", client.Status())
	}
}

func TestWinShmBatchHandlerFailureNeedsRefresh(t *testing.T) {
	svc := uniqueWinService("go_win_shm_batch_fail")
	ts := startTestServerWinWithConfig(svc, testWinShmServerConfig(), protocol.MethodIncrement, IncrementDispatch(func(v uint64) (uint64, bool) {
		if v == 99 {
			return 0, false
		}
		return v + 1, true
	}))
	defer ts.stop()

	client := NewIncrementClient(winTestRunDir, svc, testWinShmClientConfig())
	defer client.Close()

	waitWinClientReady(t, client)

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

func TestWinShmBatchResponseOverflowRetriesAndRecovers(t *testing.T) {
	svc := uniqueWinService("go_win_shm_batch_overflow")

	scfg := testWinShmServerConfig()
	scfg.MaxResponsePayloadBytes = 24
	scfg.MaxResponseBatchItems = 2
	ccfg := testWinShmClientConfig()
	ccfg.MaxResponsePayloadBytes = 24
	ccfg.MaxResponseBatchItems = 2
	ccfg.MaxRequestBatchItems = 2

	ts := startTestServerWinWithConfig(svc, scfg, protocol.MethodIncrement, winIncrementDispatchHandler())
	defer ts.stop()

	client := NewIncrementClient(winTestRunDir, svc, ccfg)
	defer client.Close()

	waitWinClientReady(t, client)

	gotBatch, err := client.CallIncrementBatch([]uint64{1, 2})
	if err != nil {
		t.Fatalf("CallIncrementBatch after WinSHM overflow failed: %v", err)
	}
	if len(gotBatch) != 2 || gotBatch[0] != 2 || gotBatch[1] != 3 {
		t.Fatalf("CallIncrementBatch after WinSHM overflow = %v, want [2 3]", gotBatch)
	}
	if client.state != StateReady {
		t.Fatalf("client state after batch overflow recovery = %d, want READY", client.state)
	}
	if client.reconnectCount < 1 {
		t.Fatalf("expected reconnect_count >= 1 after WinSHM batch overflow, got %d", client.reconnectCount)
	}

	got, err := client.CallIncrement(8)
	if err != nil {
		t.Fatalf("CallIncrement after WinSHM overflow recovery failed: %v", err)
	}
	if got != 9 {
		t.Fatalf("CallIncrement after WinSHM overflow recovery = %d, want 9", got)
	}
}
