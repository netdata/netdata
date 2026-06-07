//go:build windows

package windows

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

func headerVersionIncompatible(buf []byte, expectedCode uint16) bool {
	return framing.HeaderVersionIncompatible(buf, expectedCode)
}

func helloLayoutIncompatible(buf []byte) bool {
	return framing.HelloLayoutIncompatible(buf)
}

func helloAckLayoutIncompatible(buf []byte) bool {
	return framing.HelloAckLayoutIncompatible(buf)
}

func clientHandshake(handle syscall.Handle, config *ClientConfig) (*Session, error) {
	pktSize := applyDefault(config.PacketSize, defaultPacketSize)

	pkt := framing.BuildHelloPacket(framing.HelloConfig{
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestPayloadBytes:  applyDefault(config.MaxRequestPayloadBytes, protocol.MaxPayloadDefault),
		MaxRequestBatchItems:    applyDefault(config.MaxRequestBatchItems, defaultBatchItems),
		MaxResponsePayloadBytes: applyDefault(config.MaxResponsePayloadBytes, protocol.MaxPayloadDefault),
		MaxResponseBatchItems:   applyDefault(config.MaxResponseBatchItems, defaultBatchItems),
		AuthToken:               config.AuthToken,
		PacketSize:              pktSize,
	})

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
	ack, err := framing.DecodeHelloAckPayload(ackBuf[protocol.HeaderSize:an])
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

	hello, err := framing.DecodeHelloPayload(buf[protocol.HeaderSize:n])
	if err != nil {
		if errors.Is(err, protocol.ErrBadLayout) &&
			helloLayoutIncompatible(buf[protocol.HeaderSize:n]) {
			sendRejection(handle, protocol.StatusIncompatible)
			return nil, ErrIncompatible
		}
		return nil, wrapErr(ErrProtocol, "hello payload: "+err.Error())
	}

	ack, status, ok := framing.NegotiateHello(hello, framing.ServerHelloConfig{
		PacketSize:              serverPktSize,
		MaxResponsePayloadBytes: sRespPay,
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		AuthToken:               config.AuthToken,
	})
	if !ok {
		sendRejection(handle, status)
		return nil, helloAckStatusError(status)
	}
	ack.SessionID = sessionID
	if err := sendHelloAck(handle, protocol.StatusOK, ack); err != nil {
		return nil, wrapErr(ErrSend, "hello_ack send: "+err.Error())
	}

	return &Session{
		handle:                  handle,
		role:                    RoleServer,
		MaxRequestPayloadBytes:  ack.AgreedMaxRequestPayloadBytes,
		MaxRequestBatchItems:    ack.AgreedMaxRequestBatchItems,
		MaxResponsePayloadBytes: ack.AgreedMaxResponsePayloadBytes,
		MaxResponseBatchItems:   ack.AgreedMaxResponseBatchItems,
		PacketSize:              ack.AgreedPacketSize,
		SelectedProfile:         ack.SelectedProfile,
		SessionID:               sessionID,
		inflightIDs:             make(map[uint64]struct{}),
	}, nil
}

func sendRejection(handle syscall.Handle, status uint16) {
	_ = sendHelloAck(handle, status, protocol.HelloAck{LayoutVersion: 1})
}

func sendHelloAck(handle syscall.Handle, status uint16, ack protocol.HelloAck) error {
	pkt := framing.BuildHelloAckPacket(status, ack)
	return rawWrite(handle, pkt[:])
}
