package framing

import (
	"encoding/binary"

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
