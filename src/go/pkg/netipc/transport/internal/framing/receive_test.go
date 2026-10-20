package framing

import (
	"errors"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func TestReceiverRejectsPacketLongerThanDeclaredFrame(t *testing.T) {
	packet := encodeReceiveTestPacket(1, []byte("ab"))
	receiver := receiveTestReceiver(packet)

	_, _, err := receiver.Receive(make([]byte, 64))
	if err == nil {
		t.Fatal("expected trailing packet bytes to be rejected")
	}
	if !strings.Contains(err.Error(), "exceeds declared payload_len") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReceiverAcceptsExactDeclaredFrame(t *testing.T) {
	packet := encodeReceiveTestPacket(2, []byte("ab"))
	receiver := receiveTestReceiver(packet)

	hdr, payload, err := receiver.Receive(make([]byte, 64))
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	if hdr.PayloadLen != 2 {
		t.Fatalf("payload_len = %d, want 2", hdr.PayloadLen)
	}
	if string(payload) != "ab" {
		t.Fatalf("payload = %q, want ab", payload)
	}
}

func receiveTestReceiver(packet []byte) Receiver {
	return Receiver{
		PacketSize:    64,
		MaxPayload:    64,
		MaxBatchItems: 1,
		Recv: func(buf []byte) (int, error) {
			return copy(buf, packet), nil
		},
		ErrLimitExceeded: func(msg string) error { return errors.New(msg) },
		ErrProtocol:      func(msg string) error { return errors.New(msg) },
		ErrChunk:         func(msg string) error { return errors.New(msg) },
		ErrUnknownMsgID:  func(msg string) error { return errors.New(msg) },
		ErrRecv:          func(msg string) error { return errors.New(msg) },
	}
}

func encodeReceiveTestPacket(declaredPayloadLen uint32, payload []byte) []byte {
	hdr := protocol.Header{
		Magic:           protocol.MagicMsg,
		Version:         protocol.Version,
		HeaderLen:       protocol.HeaderLen,
		Kind:            protocol.KindRequest,
		TransportStatus: protocol.StatusOK,
		PayloadLen:      declaredPayloadLen,
		ItemCount:       1,
		MessageID:       1,
	}
	packet := make([]byte, protocol.HeaderSize+len(payload))
	if hdr.Encode(packet) != protocol.HeaderSize {
		panic("header encode failed")
	}
	copy(packet[protocol.HeaderSize:], payload)
	return packet
}
