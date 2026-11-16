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
	// PlatformProfiles limits the offered profiles to those the platform
	// can provide; 0 means no limit.
	PlatformProfiles uint32
}

type ServerHelloConfig struct {
	PacketSize              uint32
	MaxRequestPayloadBytes  uint32
	MaxResponsePayloadBytes uint32
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	AuthToken               uint64
	// PlatformProfiles limits the accepted profiles to those the platform
	// can provide; 0 means no limit.
	PlatformProfiles uint32
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

	// Prepare, when set, runs once a profile is selected and the payload
	// limits are agreed, before HELLO_ACK is sent. Returning false drops
	// that profile and the handshake selects again.
	Prepare func(profile, maxRequestPayloadBytes, maxResponsePayloadBytes uint32) bool

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

// profiles applies the baseline default and then the platform limit.
func profiles(supported, preferred, platform uint32) (uint32, uint32) {
	if supported == 0 {
		supported = protocol.ProfileBaseline
	}
	if platform != 0 {
		supported &= platform
		preferred &= platform
	}
	return supported, preferred
}

func BuildHelloPacket(config HelloConfig) [protocol.HeaderSize + HelloPayloadSize]byte {
	supported, preferred := profiles(config.SupportedProfiles, config.PreferredProfiles, config.PlatformProfiles)

	hello := protocol.Hello{
		LayoutVersion:           1,
		Flags:                   0,
		SupportedProfiles:       supported,
		PreferredProfiles:       preferred,
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
	// HELLO_ACK is exactly one 48-byte payload: extra bytes in the packet, or
	// a declared length a stream peer may still be sending, would otherwise be
	// read as the session's first message.
	if ackHdr.PayloadLen != HelloAckPayloadSize || n != protocol.HeaderSize+HelloAckPayloadSize {
		return protocol.HelloAck{}, config.ErrProtocol("ack payload length mismatch")
	}

	offered, _ := profiles(config.Hello.SupportedProfiles, 0, config.Hello.PlatformProfiles)
	selected := ack.SelectedProfile
	if selected == 0 || selected&(selected-1) != 0 || selected&offered == 0 {
		return protocol.HelloAck{}, config.ErrProtocol("selected profile was not offered")
	}
	// A server may only lower our proposal: this side sizes its buffers, and
	// on SEQPACKET its datagrams, from it. A packet must hold a payload byte.
	if ack.AgreedPacketSize <= protocol.HeaderSize || ack.AgreedPacketSize > config.Hello.PacketSize {
		return protocol.HelloAck{}, config.ErrProtocol("agreed packet_size out of range")
	}

	return ack, nil
}

func NegotiateHello(hello protocol.Hello, config ServerHelloConfig) (protocol.HelloAck, uint16, bool) {
	config.SupportedProfiles, config.PreferredProfiles = profiles(
		config.SupportedProfiles, config.PreferredProfiles, config.PlatformProfiles)

	intersection := hello.SupportedProfiles & config.SupportedProfiles
	if intersection == 0 {
		return protocol.HelloAck{}, protocol.StatusUnsupported, false
	}
	if hello.AuthToken != config.AuthToken {
		return protocol.HelloAck{}, protocol.StatusAuthFailed, false
	}
	if hello.MaxRequestPayloadBytes > config.MaxRequestPayloadBytes {
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

	hdr, err := DecodeControlHeader(buf[:n], protocol.CodeHello)
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
	// HELLO is exactly one 44-byte payload, as HELLO_ACK is 48.
	if hdr.PayloadLen != HelloPayloadSize || n != protocol.HeaderSize+HelloPayloadSize {
		return protocol.HelloAck{}, config.ErrProtocol("hello payload length mismatch")
	}

	ack, status, ok := NegotiateHello(hello, config.ServerHelloConfig)
	if !ok {
		sendRejection(config.SendAck, status)
		return protocol.HelloAck{}, config.StatusError(status)
	}
	ack.SessionID = config.SessionID

	// Set up the selected profile before the ACK promises it. A profile the
	// caller cannot prepare is dropped, as if the server never offered it.
	for config.Prepare != nil && !config.Prepare(ack.SelectedProfile,
		ack.AgreedMaxRequestPayloadBytes, ack.AgreedMaxResponsePayloadBytes) {
		ack.ServerSupportedProfiles &^= ack.SelectedProfile
		ack.IntersectionProfiles &^= ack.SelectedProfile
		if ack.IntersectionProfiles == 0 {
			sendRejection(config.SendAck, protocol.StatusUnsupported)
			return protocol.HelloAck{}, config.StatusError(protocol.StatusUnsupported)
		}
		preferred := ack.IntersectionProfiles & hello.PreferredProfiles & config.PreferredProfiles
		ack.SelectedProfile = highestBit(ack.IntersectionProfiles)
		if preferred != 0 {
			ack.SelectedProfile = highestBit(preferred)
		}
	}

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
