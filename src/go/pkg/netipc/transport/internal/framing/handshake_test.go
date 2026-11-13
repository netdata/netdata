package framing

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

var errTestProtocol = errors.New("protocol")

func testErr(err error) func(string) error {
	return func(string) error { return err }
}

// padded returns pkt with 16 more bytes, the header declaring them too when
// declare is set.
func padded(pkt []byte, declare bool) []byte {
	out := append(append([]byte(nil), pkt...), make([]byte, 16)...)
	if declare {
		binary.NativeEndian.PutUint32(out[16:20], uint32(len(pkt)-protocol.HeaderSize+16))
	}
	return out
}

// HELLO and HELLO_ACK are exactly 44 and 48 payload bytes: a packet with
// more, declared or not, is a protocol error.
func TestHandshakePayloadLengthExact(t *testing.T) {
	hello := HelloConfig{
		PacketSize:              4096,
		SupportedProfiles:       protocol.ProfileBaseline,
		MaxRequestPayloadBytes:  1024,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: 1024,
		MaxResponseBatchItems:   1,
	}
	ackPkt := BuildHelloAckPacket(protocol.StatusOK, protocol.HelloAck{
		LayoutVersion:    1,
		SelectedProfile:  protocol.ProfileBaseline,
		AgreedPacketSize: 4096,
	})
	client := func(reply []byte) error {
		_, err := ClientHandshake(ClientHandshakeConfig{
			Hello:           hello,
			Send:            func([]byte) error { return nil },
			Recv:            func(dst []byte) (int, error) { return copy(dst, reply), nil },
			StatusError:     func(uint16) error { return nil },
			ErrSend:         testErr(errors.New("send")),
			ErrRecv:         testErr(errors.New("recv")),
			ErrProtocol:     testErr(errTestProtocol),
			ErrIncompatible: testErr(errors.New("incompatible")),
		})
		return err
	}
	if err := client(ackPkt[:]); err != nil {
		t.Fatalf("exact HELLO_ACK: %v", err)
	}
	for _, declare := range []bool{false, true} {
		if err := client(padded(ackPkt[:], declare)); !errors.Is(err, errTestProtocol) {
			t.Fatalf("padded HELLO_ACK (declared %v): got %v", declare, err)
		}
	}

	helloPkt := BuildHelloPacket(hello)
	server := func(pkt []byte) error {
		_, err := ServerHandshake(ServerHandshakeConfig{
			ServerHelloConfig: ServerHelloConfig{
				PacketSize:              4096,
				MaxRequestPayloadBytes:  1024,
				MaxResponsePayloadBytes: 1024,
				SupportedProfiles:       protocol.ProfileBaseline,
			},
			SessionID:       1,
			Recv:            func(dst []byte) (int, error) { return copy(dst, pkt), nil },
			SendAck:         func(uint16, protocol.HelloAck) error { return nil },
			StatusError:     func(uint16) error { return errors.New("status") },
			ErrRecv:         testErr(errors.New("recv")),
			ErrSend:         testErr(errors.New("send")),
			ErrProtocol:     testErr(errTestProtocol),
			ErrIncompatible: testErr(errors.New("incompatible")),
		})
		return err
	}
	if err := server(helloPkt[:]); err != nil {
		t.Fatalf("exact HELLO: %v", err)
	}
	for _, declare := range []bool{false, true} {
		if err := server(padded(helloPkt[:], declare)); !errors.Is(err, errTestProtocol) {
			t.Fatalf("padded HELLO (declared %v): got %v", declare, err)
		}
	}
}

// A server may only lower the client's packet size proposal, and the agreed
// size must leave room for a payload after the header.
func TestClientRejectsAgreedPacketSizeOutOfRange(t *testing.T) {
	hello := HelloConfig{
		PacketSize:              4096,
		SupportedProfiles:       protocol.ProfileBaseline,
		MaxRequestPayloadBytes:  1024,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: 1024,
		MaxResponseBatchItems:   1,
	}
	client := func(agreed uint32) error {
		reply := BuildHelloAckPacket(protocol.StatusOK, protocol.HelloAck{
			LayoutVersion:    1,
			SelectedProfile:  protocol.ProfileBaseline,
			AgreedPacketSize: agreed,
		})
		_, err := ClientHandshake(ClientHandshakeConfig{
			Hello:           hello,
			Send:            func([]byte) error { return nil },
			Recv:            func(dst []byte) (int, error) { return copy(dst, reply[:]), nil },
			StatusError:     func(uint16) error { return nil },
			ErrSend:         testErr(errors.New("send")),
			ErrRecv:         testErr(errors.New("recv")),
			ErrProtocol:     testErr(errTestProtocol),
			ErrIncompatible: testErr(errors.New("incompatible")),
		})
		return err
	}
	for _, agreed := range []uint32{protocol.HeaderSize + 1, 4096} {
		if err := client(agreed); err != nil {
			t.Fatalf("agreed %d: %v", agreed, err)
		}
	}
	for _, agreed := range []uint32{0, protocol.HeaderSize, 4097} {
		if err := client(agreed); !errors.Is(err, errTestProtocol) {
			t.Fatalf("agreed %d: got %v, want protocol error", agreed, err)
		}
	}
}
