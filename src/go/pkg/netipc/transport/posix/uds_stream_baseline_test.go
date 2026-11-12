//go:build darwin || freebsd

package posix

import (
	"errors"
	"syscall"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// A receive that consumed part of a message cannot leave the session
// usable: the rest is still queued and would be read as a header.
func TestStreamPartialReceiveEndsReads(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, sockType, 0)
	if err != nil {
		t.Fatalf("socketpair: %v", err)
	}
	defer syscall.Close(fds[1])
	session := &Session{
		fd:                      fds[0],
		role:                    RoleServer,
		MaxRequestPayloadBytes:  4096,
		MaxRequestBatchItems:    16,
		MaxResponsePayloadBytes: 4096,
		MaxResponseBatchItems:   16,
		PacketSize:              1024,
		SelectedProfile:         protocol.ProfileBaseline,
		SessionID:               1,
		inflightIDs:             make(map[uint64]struct{}),
	}
	defer session.Close()
	buf := make([]byte, 2048)

	if _, _, err := session.ReceiveTimeout(buf, 1, nil); !errors.Is(err, ErrTimeout) {
		t.Fatalf("timeout before the first byte: %v, want ErrTimeout", err)
	}

	pkt := streamOuter(5, []byte("hello"))
	if err := sendPacket(fds[1], pkt, nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, payload, err := session.Receive(buf); err != nil || string(payload) != "hello" {
		t.Fatalf("intact after a clean timeout: %q, %v", payload, err)
	}

	if err := sendPacket(fds[1], pkt[:10], nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, _, err := session.ReceiveTimeout(buf, 1, nil); !errors.Is(err, ErrTimeout) {
		t.Fatalf("timeout inside a message: %v, want ErrTimeout", err)
	}
	if err := sendPacket(fds[1], pkt[10:], nil); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, _, err := session.Receive(buf); !errors.Is(err, ErrRecv) {
		t.Fatalf("later receives: %v, want ErrRecv", err)
	}

	// Sends always write whole messages, so they stay valid.
	resp := protocol.Header{Kind: protocol.KindResponse, Code: 1, ItemCount: 1, MessageID: 9}
	if err := session.Send(&resp, []byte("ok")); err != nil {
		t.Fatalf("sends still work: %v", err)
	}
	if n, err := streamRecvPacket(fds[1], buf, 1024, nil); err != nil || n != protocol.HeaderSize+2 {
		t.Fatalf("peer gets the response: %d, %v", n, err)
	}
}

// Stream baseline: a receive that fails mid-stream marks the receive side failed, and
// every later receive must fail. Bytes left in the pending queue are part of
// that abandoned stream: here a forged response for an in-flight id follows
// an over-limit header, and must not be delivered.
func TestStreamFailedReceiveDropsPending(t *testing.T) {
	client, wg := smallBufferStart(t, smallBufCloseAfterOne, 65536)
	if err := sendPipelinedRequest(client, 0); err != nil {
		t.Fatalf("request 1: %v", err)
	}

	over := protocol.Header{
		Magic:      protocol.MagicMsg,
		Version:    protocol.Version,
		HeaderLen:  protocol.HeaderSize,
		Kind:       protocol.KindResponse,
		ItemCount:  1,
		MessageID:  1,
		PayloadLen: 65537,
	}
	buf := make([]byte, 2*protocol.HeaderSize)
	over.Encode(buf[:protocol.HeaderSize])
	over.PayloadLen = 0
	over.Encode(buf[protocol.HeaderSize:])
	client.pending = pendingQueue{buf: buf}

	rbuf := make([]byte, pipeBufPkt)
	if _, _, err := client.Receive(rbuf); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("expected ErrLimitExceeded, got %v", err)
	}
	// No response can arrive any more, so the request in flight has failed.
	if n := len(client.inflightIDs); n != 0 {
		t.Fatalf("%d requests still in flight after the receive side failed", n)
	}
	if _, _, err := client.Receive(rbuf); err == nil {
		t.Fatal("later receive delivered a queued message")
	}
	client.Close()
	wg.Wait()
}
