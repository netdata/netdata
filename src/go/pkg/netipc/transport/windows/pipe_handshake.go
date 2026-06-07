//go:build windows

package windows

import (
	"encoding/binary"
	"errors"
	"fmt"
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

func headerVersionIncompatible(buf []byte, expectedCode uint16) bool {
	if len(buf) < protocol.HeaderSize {
		return false
	}

	return binary.NativeEndian.Uint32(buf[0:4]) == protocol.MagicMsg &&
		binary.NativeEndian.Uint16(buf[4:6]) != protocol.Version &&
		binary.NativeEndian.Uint16(buf[6:8]) == protocol.HeaderLen &&
		binary.NativeEndian.Uint16(buf[8:10]) == protocol.KindControl &&
		binary.NativeEndian.Uint16(buf[12:14]) == expectedCode
}

func helloLayoutIncompatible(buf []byte) bool {
	return len(buf) >= 2 && binary.NativeEndian.Uint16(buf[0:2]) != 1
}

func helloAckLayoutIncompatible(buf []byte) bool {
	return len(buf) >= 2 && binary.NativeEndian.Uint16(buf[0:2]) != 1
}

func clientHandshake(handle syscall.Handle, config *ClientConfig) (*Session, error) {
	pktSize := applyDefault(config.PacketSize, defaultPacketSize)

	supported := config.SupportedProfiles
	if supported == 0 {
		supported = protocol.ProfileBaseline
	}

	hello := protocol.Hello{
		LayoutVersion:           1,
		Flags:                   0,
		SupportedProfiles:       supported,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestPayloadBytes:  applyDefault(config.MaxRequestPayloadBytes, protocol.MaxPayloadDefault),
		MaxRequestBatchItems:    applyDefault(config.MaxRequestBatchItems, defaultBatchItems),
		MaxResponsePayloadBytes: applyDefault(config.MaxResponsePayloadBytes, protocol.MaxPayloadDefault),
		MaxResponseBatchItems:   applyDefault(config.MaxResponseBatchItems, defaultBatchItems),
		AuthToken:               config.AuthToken,
		PacketSize:              pktSize,
	}

	var helloBuf [helloPayloadSize]byte
	hello.Encode(helloBuf[:])

	hdr := protocol.Header{
		Magic:           protocol.MagicMsg,
		Version:         protocol.Version,
		HeaderLen:       protocol.HeaderLen,
		Kind:            protocol.KindControl,
		Flags:           0,
		Code:            protocol.CodeHello,
		TransportStatus: protocol.StatusOK,
		PayloadLen:      helloPayloadSize,
		ItemCount:       1,
		MessageID:       0,
	}

	var pkt [protocol.HeaderSize + helloPayloadSize]byte
	hdr.Encode(pkt[:protocol.HeaderSize])
	copy(pkt[protocol.HeaderSize:], helloBuf[:])

	if err := rawWrite(handle, pkt[:]); err != nil {
		return nil, wrapErr(ErrSend, "hello send: "+err.Error())
	}

	var ackBuf [128]byte
	an, err := rawRecv(handle, ackBuf[:])
	if err != nil {
		return nil, wrapErr(ErrRecv, "hello_ack recv: "+err.Error())
	}

	ackHdr, err := protocol.DecodeHeader(ackBuf[:an])
	if err != nil {
		if errors.Is(err, protocol.ErrBadVersion) {
			return nil, wrapErr(ErrIncompatible, "ack header version mismatch")
		}
		return nil, wrapErr(ErrProtocol, "ack header: "+err.Error())
	}

	if ackHdr.Kind != protocol.KindControl || ackHdr.Code != protocol.CodeHelloAck {
		return nil, wrapErr(ErrProtocol, "expected HELLO_ACK")
	}

	if err := helloAckStatusError(ackHdr.TransportStatus); err != nil {
		return nil, err
	}

	if an < protocol.HeaderSize+helloAckPayloadSize {
		return nil, wrapErr(ErrProtocol, "ack payload truncated")
	}
	ack, err := protocol.DecodeHelloAck(ackBuf[protocol.HeaderSize:an])
	if err != nil {
		if errors.Is(err, protocol.ErrBadLayout) &&
			helloAckLayoutIncompatible(ackBuf[protocol.HeaderSize:an]) {
			return nil, wrapErr(ErrIncompatible, "ack payload layout version mismatch")
		}
		return nil, wrapErr(ErrProtocol, "ack payload: "+err.Error())
	}

	return &Session{
		handle:                  handle,
		role:                    RoleClient,
		MaxRequestPayloadBytes:  ack.AgreedMaxRequestPayloadBytes,
		MaxRequestBatchItems:    ack.AgreedMaxRequestBatchItems,
		MaxResponsePayloadBytes: ack.AgreedMaxResponsePayloadBytes,
		MaxResponseBatchItems:   ack.AgreedMaxResponseBatchItems,
		PacketSize:              ack.AgreedPacketSize,
		SelectedProfile:         ack.SelectedProfile,
		SessionID:               ack.SessionID,
		inflightIDs:             make(map[uint64]struct{}),
	}, nil
}

func helloAckStatusError(status uint16) error {
	switch status {
	case protocol.StatusOK:
		return nil
	case protocol.StatusAuthFailed:
		return ErrAuthFailed
	case protocol.StatusUnsupported:
		return ErrNoProfile
	case protocol.StatusIncompatible:
		return ErrIncompatible
	case protocol.StatusLimitExceeded:
		return ErrLimitExceeded
	default:
		return wrapErr(ErrHandshake, fmt.Sprintf("transport_status=%d", status))
	}
}

func serverHandshake(handle syscall.Handle, config *ServerConfig, sessionID uint64) (*Session, error) {
	serverPktSize := applyDefault(config.PacketSize, defaultPacketSize)
	sRespPay := applyDefault(config.MaxResponsePayloadBytes, protocol.MaxPayloadDefault)
	sProfiles := config.SupportedProfiles
	if sProfiles == 0 {
		sProfiles = protocol.ProfileBaseline
	}
	sPreferred := config.PreferredProfiles

	var buf [128]byte
	n, err := rawRecv(handle, buf[:])
	if err != nil {
		return nil, wrapErr(ErrRecv, "hello recv: "+err.Error())
	}

	hdr, err := protocol.DecodeHeader(buf[:n])
	if err != nil {
		if errors.Is(err, protocol.ErrBadVersion) &&
			headerVersionIncompatible(buf[:n], protocol.CodeHello) {
			sendRejection(handle, protocol.StatusIncompatible)
			return nil, ErrIncompatible
		}
		return nil, wrapErr(ErrProtocol, "hello header: "+err.Error())
	}

	if hdr.Kind != protocol.KindControl || hdr.Code != protocol.CodeHello {
		return nil, wrapErr(ErrProtocol, "expected HELLO")
	}

	hello, err := protocol.DecodeHello(buf[protocol.HeaderSize:n])
	if err != nil {
		if errors.Is(err, protocol.ErrBadLayout) &&
			helloLayoutIncompatible(buf[protocol.HeaderSize:n]) {
			sendRejection(handle, protocol.StatusIncompatible)
			return nil, ErrIncompatible
		}
		return nil, wrapErr(ErrProtocol, "hello payload: "+err.Error())
	}

	intersection := hello.SupportedProfiles & sProfiles
	if intersection == 0 {
		sendRejection(handle, protocol.StatusUnsupported)
		return nil, ErrNoProfile
	}

	if hello.AuthToken != config.AuthToken {
		sendRejection(handle, protocol.StatusAuthFailed)
		return nil, ErrAuthFailed
	}

	preferredIntersection := intersection & hello.PreferredProfiles & sPreferred
	selected := highestBit(intersection)
	if preferredIntersection != 0 {
		selected = highestBit(preferredIntersection)
	}

	if hello.MaxRequestPayloadBytes > protocol.MaxPayloadCap {
		sendRejection(handle, protocol.StatusLimitExceeded)
		return nil, ErrLimitExceeded
	}

	agreedReqPay := hello.MaxRequestPayloadBytes
	agreedReqBat := hello.MaxRequestBatchItems
	agreedRespPay := sRespPay
	agreedRespBat := agreedReqBat
	agreedPkt := minU32(hello.PacketSize, serverPktSize)
	if agreedPkt <= protocol.HeaderSize {
		sendRejection(handle, protocol.StatusIncompatible)
		return nil, ErrIncompatible
	}

	ack := protocol.HelloAck{
		LayoutVersion:                 1,
		Flags:                         0,
		ServerSupportedProfiles:       sProfiles,
		IntersectionProfiles:          intersection,
		SelectedProfile:               selected,
		AgreedMaxRequestPayloadBytes:  agreedReqPay,
		AgreedMaxRequestBatchItems:    agreedReqBat,
		AgreedMaxResponsePayloadBytes: agreedRespPay,
		AgreedMaxResponseBatchItems:   agreedRespBat,
		AgreedPacketSize:              agreedPkt,
		SessionID:                     sessionID,
	}
	if err := sendHelloAck(handle, protocol.StatusOK, ack); err != nil {
		return nil, wrapErr(ErrSend, "hello_ack send: "+err.Error())
	}

	return &Session{
		handle:                  handle,
		role:                    RoleServer,
		MaxRequestPayloadBytes:  agreedReqPay,
		MaxRequestBatchItems:    agreedReqBat,
		MaxResponsePayloadBytes: agreedRespPay,
		MaxResponseBatchItems:   agreedRespBat,
		PacketSize:              agreedPkt,
		SelectedProfile:         selected,
		SessionID:               sessionID,
		inflightIDs:             make(map[uint64]struct{}),
	}, nil
}

func sendRejection(handle syscall.Handle, status uint16) {
	_ = sendHelloAck(handle, status, protocol.HelloAck{LayoutVersion: 1})
}

func sendHelloAck(handle syscall.Handle, status uint16, ack protocol.HelloAck) error {
	var ackPayBuf [helloAckPayloadSize]byte
	ack.Encode(ackPayBuf[:])

	ackHdr := protocol.Header{
		Magic:           protocol.MagicMsg,
		Version:         protocol.Version,
		HeaderLen:       protocol.HeaderLen,
		Kind:            protocol.KindControl,
		Code:            protocol.CodeHelloAck,
		TransportStatus: status,
		PayloadLen:      helloAckPayloadSize,
		ItemCount:       1,
	}

	var pkt [protocol.HeaderSize + helloAckPayloadSize]byte
	ackHdr.Encode(pkt[:protocol.HeaderSize])
	copy(pkt[protocol.HeaderSize:], ackPayBuf[:])
	return rawWrite(handle, pkt[:])
}
