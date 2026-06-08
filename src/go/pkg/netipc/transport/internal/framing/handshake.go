package framing

import (
	"encoding/binary"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

const (
	HelloPayloadSize    = 44
	HelloAckPayloadSize = 48
)

type HelloConfig struct {
	PacketSize              uint32
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	MaxRequestPayloadBytes  uint32
	MaxRequestBatchItems    uint32
	MaxResponsePayloadBytes uint32
	MaxResponseBatchItems   uint32
	AuthToken               uint64
}

type ServerHelloConfig struct {
	PacketSize              uint32
	MaxResponsePayloadBytes uint32
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	AuthToken               uint64
}

type ClientHandshakeConfig struct {
	Hello HelloConfig

	Send        func([]byte) error
	Recv        func([]byte) (int, error)
	StatusError func(uint16) error

	ErrSend         func(string) error
	ErrRecv         func(string) error
	ErrProtocol     func(string) error
	ErrIncompatible func(string) error
}

type ServerHandshakeConfig struct {
	ServerHelloConfig
	SessionID uint64

	Recv        func([]byte) (int, error)
	SendAck     func(uint16, protocol.HelloAck) error
	StatusError func(uint16) error

	ErrRecv         func(string) error
	ErrSend         func(string) error
	ErrProtocol     func(string) error
	ErrIncompatible func(string) error
}

func HeaderVersionIncompatible(buf []byte, expectedCode uint16) bool {
	if len(buf) < protocol.HeaderSize {
		return false
	}

	return binary.NativeEndian.Uint32(buf[0:4]) == protocol.MagicMsg &&
		binary.NativeEndian.Uint16(buf[4:6]) != protocol.Version &&
		binary.NativeEndian.Uint16(buf[6:8]) == protocol.HeaderLen &&
		binary.NativeEndian.Uint16(buf[8:10]) == protocol.KindControl &&
		binary.NativeEndian.Uint16(buf[12:14]) == expectedCode
}

func HelloLayoutIncompatible(buf []byte) bool {
	return len(buf) >= 2 && binary.NativeEndian.Uint16(buf[0:2]) != 1
}

func HelloAckLayoutIncompatible(buf []byte) bool {
	return len(buf) >= 2 && binary.NativeEndian.Uint16(buf[0:2]) != 1
}

func BuildHelloPacket(config HelloConfig) [protocol.HeaderSize + HelloPayloadSize]byte {
	if config.SupportedProfiles == 0 {
		config.SupportedProfiles = protocol.ProfileBaseline
	}

	hello := protocol.Hello{
		LayoutVersion:           1,
		Flags:                   0,
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestPayloadBytes:  config.MaxRequestPayloadBytes,
		MaxRequestBatchItems:    config.MaxRequestBatchItems,
		MaxResponsePayloadBytes: config.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   config.MaxResponseBatchItems,
		AuthToken:               config.AuthToken,
		PacketSize:              config.PacketSize,
	}

	var payload [HelloPayloadSize]byte
	hello.Encode(payload[:])

	hdr := protocol.Header{
		Magic:           protocol.MagicMsg,
		Version:         protocol.Version,
		HeaderLen:       protocol.HeaderLen,
		Kind:            protocol.KindControl,
		Flags:           0,
		Code:            protocol.CodeHello,
		TransportStatus: protocol.StatusOK,
		PayloadLen:      HelloPayloadSize,
		ItemCount:       1,
		MessageID:       0,
	}

	var pkt [protocol.HeaderSize + HelloPayloadSize]byte
	hdr.Encode(pkt[:protocol.HeaderSize])
	copy(pkt[protocol.HeaderSize:], payload[:])
	return pkt
}

func DecodeControlHeader(buf []byte, expectedCode uint16) (protocol.Header, error) {
	hdr, err := protocol.DecodeHeader(buf)
	if err != nil {
		return protocol.Header{}, err
	}
	if hdr.Kind != protocol.KindControl || hdr.Code != expectedCode {
		return protocol.Header{}, protocol.ErrBadKind
	}
	return hdr, nil
}

func DecodeHelloPayload(buf []byte) (protocol.Hello, error) {
	return protocol.DecodeHello(buf)
}

func DecodeHelloAckPayload(buf []byte) (protocol.HelloAck, error) {
	return protocol.DecodeHelloAck(buf)
}

func ClientHandshake(config ClientHandshakeConfig) (protocol.HelloAck, error) {
	pkt := BuildHelloPacket(config.Hello)
	if err := config.Send(pkt[:]); err != nil {
		return protocol.HelloAck{}, config.ErrSend("hello send: " + err.Error())
	}

	var ackBuf [128]byte
	n, err := config.Recv(ackBuf[:])
	if err != nil {
		return protocol.HelloAck{}, config.ErrRecv("hello_ack recv: " + err.Error())
	}

	ackHdr, err := DecodeControlHeader(ackBuf[:n], protocol.CodeHelloAck)
	if err != nil {
		if errors.Is(err, protocol.ErrBadVersion) {
			return protocol.HelloAck{}, config.ErrIncompatible("ack header version mismatch")
		}
		if errors.Is(err, protocol.ErrBadKind) {
			return protocol.HelloAck{}, config.ErrProtocol("expected HELLO_ACK")
		}
		return protocol.HelloAck{}, config.ErrProtocol("ack header: " + err.Error())
	}

	if err := config.StatusError(ackHdr.TransportStatus); err != nil {
		return protocol.HelloAck{}, err
	}

	if n < protocol.HeaderSize+HelloAckPayloadSize {
		return protocol.HelloAck{}, config.ErrProtocol("ack payload truncated")
	}
	ack, err := DecodeHelloAckPayload(ackBuf[protocol.HeaderSize:n])
	if err != nil {
		if errors.Is(err, protocol.ErrBadLayout) &&
			HelloAckLayoutIncompatible(ackBuf[protocol.HeaderSize:n]) {
			return protocol.HelloAck{}, config.ErrIncompatible("ack payload layout version mismatch")
		}
		return protocol.HelloAck{}, config.ErrProtocol("ack payload: " + err.Error())
	}

	return ack, nil
}

func NegotiateHello(hello protocol.Hello, config ServerHelloConfig) (protocol.HelloAck, uint16, bool) {
	if config.SupportedProfiles == 0 {
		config.SupportedProfiles = protocol.ProfileBaseline
	}

	intersection := hello.SupportedProfiles & config.SupportedProfiles
	if intersection == 0 {
		return protocol.HelloAck{}, protocol.StatusUnsupported, false
	}
	if hello.AuthToken != config.AuthToken {
		return protocol.HelloAck{}, protocol.StatusAuthFailed, false
	}
	// Level 1 keeps request payload limits client-proposed: the server rejects
	// values over the wire cap and echoes accepted values unchanged.
	if hello.MaxRequestPayloadBytes > protocol.MaxPayloadCap {
		return protocol.HelloAck{}, protocol.StatusLimitExceeded, false
	}

	preferredIntersection := intersection & hello.PreferredProfiles & config.PreferredProfiles
	selected := highestBit(intersection)
	if preferredIntersection != 0 {
		selected = highestBit(preferredIntersection)
	}

	agreedPkt := minU32(hello.PacketSize, config.PacketSize)
	if agreedPkt <= protocol.HeaderSize {
		return protocol.HelloAck{}, protocol.StatusIncompatible, false
	}

	ack := protocol.HelloAck{
		LayoutVersion:                 1,
		Flags:                         0,
		ServerSupportedProfiles:       config.SupportedProfiles,
		IntersectionProfiles:          intersection,
		SelectedProfile:               selected,
		AgreedMaxRequestPayloadBytes:  hello.MaxRequestPayloadBytes,
		AgreedMaxRequestBatchItems:    hello.MaxRequestBatchItems,
		AgreedMaxResponsePayloadBytes: config.MaxResponsePayloadBytes,
		AgreedMaxResponseBatchItems:   hello.MaxRequestBatchItems,
		AgreedPacketSize:              agreedPkt,
	}
	return ack, protocol.StatusOK, true
}

func ServerHandshake(config ServerHandshakeConfig) (protocol.HelloAck, error) {
	var buf [128]byte
	n, err := config.Recv(buf[:])
	if err != nil {
		return protocol.HelloAck{}, config.ErrRecv("hello recv: " + err.Error())
	}

	_, err = DecodeControlHeader(buf[:n], protocol.CodeHello)
	if err != nil {
		if errors.Is(err, protocol.ErrBadVersion) &&
			HeaderVersionIncompatible(buf[:n], protocol.CodeHello) {
			sendRejection(config.SendAck, protocol.StatusIncompatible)
			return protocol.HelloAck{}, config.ErrIncompatible("hello header version mismatch")
		}
		if errors.Is(err, protocol.ErrBadKind) {
			return protocol.HelloAck{}, config.ErrProtocol("expected HELLO")
		}
		return protocol.HelloAck{}, config.ErrProtocol("hello header: " + err.Error())
	}

	hello, err := DecodeHelloPayload(buf[protocol.HeaderSize:n])
	if err != nil {
		if errors.Is(err, protocol.ErrBadLayout) &&
			HelloLayoutIncompatible(buf[protocol.HeaderSize:n]) {
			sendRejection(config.SendAck, protocol.StatusIncompatible)
			return protocol.HelloAck{}, config.ErrIncompatible("hello payload layout version mismatch")
		}
		return protocol.HelloAck{}, config.ErrProtocol("hello payload: " + err.Error())
	}

	ack, status, ok := NegotiateHello(hello, config.ServerHelloConfig)
	if !ok {
		sendRejection(config.SendAck, status)
		return protocol.HelloAck{}, config.StatusError(status)
	}
	ack.SessionID = config.SessionID

	if err := config.SendAck(protocol.StatusOK, ack); err != nil {
		return protocol.HelloAck{}, config.ErrSend("hello_ack send: " + err.Error())
	}

	return ack, nil
}

func BuildHelloAckPacket(status uint16, ack protocol.HelloAck) [protocol.HeaderSize + HelloAckPayloadSize]byte {
	var payload [HelloAckPayloadSize]byte
	ack.Encode(payload[:])

	hdr := protocol.Header{
		Magic:           protocol.MagicMsg,
		Version:         protocol.Version,
		HeaderLen:       protocol.HeaderLen,
		Kind:            protocol.KindControl,
		Code:            protocol.CodeHelloAck,
		TransportStatus: status,
		PayloadLen:      HelloAckPayloadSize,
		ItemCount:       1,
	}

	var pkt [protocol.HeaderSize + HelloAckPayloadSize]byte
	hdr.Encode(pkt[:protocol.HeaderSize])
	copy(pkt[protocol.HeaderSize:], payload[:])
	return pkt
}

func sendRejection(sendAck func(uint16, protocol.HelloAck) error, status uint16) {
	_ = sendAck(status, protocol.HelloAck{LayoutVersion: 1})
}

func highestBit(mask uint32) uint32 {
	if mask == 0 {
		return 0
	}
	var b uint32 = 1
	for mask>>1 != 0 {
		mask >>= 1
		b <<= 1
	}
	return b
}

func minU32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}
