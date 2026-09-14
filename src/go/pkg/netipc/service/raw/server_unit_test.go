package raw

import (
	"bytes"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func TestServerUnsupportedMethodResponse(t *testing.T) {
	server := &Server{
		expectedMethodCode: protocol.MethodIncrement,
		handler:            IncrementDispatch(func(v uint64) (uint64, bool) { return v + 1, true }),
	}

	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodStringReverse,
		MessageID: 42,
	}
	resp, responseLen, closeAfterSend := server.handleServerRequest(
		hdr,
		nil,
		make([]byte, 64),
		make([]byte, 64),
		64,
	)
	if responseLen != 0 || closeAfterSend {
		t.Fatalf("unsupported response len/close = %d/%v, want 0/false", responseLen, closeAfterSend)
	}
	if resp.Kind != protocol.KindResponse ||
		resp.Code != protocol.MethodStringReverse ||
		resp.MessageID != 42 ||
		resp.TransportStatus != protocol.StatusUnsupported ||
		resp.ItemCount != 1 {
		t.Fatalf("unsupported response header = %+v", resp)
	}
}

func TestServerResponseEncodingAndDispatchGuards(t *testing.T) {
	hdr := protocol.Header{
		Kind:      protocol.KindRequest,
		Code:      protocol.MethodIncrement,
		MessageID: 7,
	}
	respHdr := serverResponseHeader(hdr)
	payload := []byte{1, 2, 3, 4}
	msgBuf := make([]byte, 1)
	msg, err := serverEncodeSharedResponse(&respHdr, payload, &msgBuf)
	if err != nil {
		t.Fatalf("encode shared response: %v", err)
	}
	if len(msg) != protocol.HeaderSize+len(payload) {
		t.Fatalf("encoded message len = %d, want %d", len(msg), protocol.HeaderSize+len(payload))
	}
	if !bytes.Equal(msg[protocol.HeaderSize:], payload) {
		t.Fatalf("encoded payload = %x, want %x", msg[protocol.HeaderSize:], payload)
	}
	if respHdr.PayloadLen != uint32(len(payload)) {
		t.Fatalf("response payload len = %d, want %d", respHdr.PayloadLen, len(payload))
	}

	server := &Server{
		expectedMethodCode: protocol.MethodIncrement,
		handler: func([]byte, []byte) (int, error) {
			return -1, nil
		},
	}
	resp, responseLen, closeAfterSend := server.handleServerRequest(
		hdr,
		nil,
		make([]byte, 64),
		make([]byte, 64),
		64,
	)
	if responseLen != 0 || !closeAfterSend || resp.TransportStatus != protocol.StatusLimitExceeded {
		t.Fatalf("negative dispatch response = header %+v len %d close %v", resp, responseLen, closeAfterSend)
	}
}

func TestServerCommonInitDefaultsAndWorkerSlotGuards(t *testing.T) {
	var server Server
	server.initCommon("run", "svc", protocol.MethodIncrement, nil, 0, 0, 0)
	if server.workerCount != 1 {
		t.Fatalf("default worker count = %d, want 1", server.workerCount)
	}
	if server.learnedRequestPayloadBytes.Load() != protocol.MaxPayloadDefault ||
		server.learnedResponsePayloadBytes.Load() != protocol.MaxPayloadDefault {
		t.Fatalf("default learned capacities = %d/%d, want %d/%d",
			server.learnedRequestPayloadBytes.Load(),
			server.learnedResponsePayloadBytes.Load(),
			protocol.MaxPayloadDefault,
			protocol.MaxPayloadDefault)
	}
	if server.requestPayloadGrowthCeiling != protocol.MaxPayloadCap ||
		server.responsePayloadGrowthCeiling != protocol.MaxPayloadCap {
		t.Fatalf("default growth ceilings = %d/%d, want %d/%d",
			server.requestPayloadGrowthCeiling,
			server.responsePayloadGrowthCeiling,
			protocol.MaxPayloadCap,
			protocol.MaxPayloadCap)
	}

	var configured Server
	configured.initCommon("run", "svc", protocol.MethodIncrement, nil, 4, 4096, 8192)
	if configured.workerCount != 4 {
		t.Fatalf("configured worker count = %d, want 4", configured.workerCount)
	}
	if configured.learnedRequestPayloadBytes.Load() != 4096 ||
		configured.learnedResponsePayloadBytes.Load() != 8192 {
		t.Fatalf("configured learned capacities = %d/%d, want 4096/8192",
			configured.learnedRequestPayloadBytes.Load(),
			configured.learnedResponsePayloadBytes.Load())
	}
	if configured.requestPayloadGrowthCeiling != 4096 ||
		configured.responsePayloadGrowthCeiling != 8192 {
		t.Fatalf("configured growth ceilings = %d/%d, want 4096/8192",
			configured.requestPayloadGrowthCeiling,
			configured.responsePayloadGrowthCeiling)
	}

	var cleanupCalled atomic.Bool
	if server.retryAcceptAfter(func() { cleanupCalled.Store(true) }) {
		t.Fatal("retryAcceptAfter should stop when server is not running")
	}
	if !cleanupCalled.Load() {
		t.Fatal("retryAcceptAfter did not run cleanup")
	}

	sem := make(chan struct{}, 1)
	sem <- struct{}{}
	cleanupCalled.Store(false)
	var closed atomic.Bool
	if server.acquireWorkerSlot(sem, func() { cleanupCalled.Store(true) }, func() { closed.Store(true) }) {
		t.Fatal("acquireWorkerSlot should fail when the worker semaphore is full")
	}
	if !cleanupCalled.Load() || !closed.Load() {
		t.Fatalf("worker-slot cleanup/close = %v/%v, want true/true", cleanupCalled.Load(), closed.Load())
	}
}

func TestServerStartSessionWorkerRecoversPanic(t *testing.T) {
	var server Server
	sem := make(chan struct{}, 1)
	sem <- struct{}{}
	server.startSessionWorker(sem, func() {
		panic("test panic")
	})
	server.wg.Wait()
	if len(sem) != 0 {
		t.Fatalf("worker semaphore len = %d, want 0", len(sem))
	}
}
