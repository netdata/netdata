//go:build unix

package posix

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

func connectAndHandshake(fd int, path string, config *ClientConfig) (*Session, error) {
	sa := &syscall.SockaddrUnix{Name: path}
	if err := syscall.Connect(fd, sa); err != nil {
		return nil, wrapErr(ErrConnect, err.Error())
	}

	pktSize := config.PacketSize
	if pktSize == 0 {
		pktSize = detectPacketSize(fd)
	}

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

	n, err := syscall.SendmsgN(fd, pkt[:], nil, nil, 0)
	if err != nil {
		return nil, wrapErr(ErrSend, "hello send: "+err.Error())
	}
	if n != len(pkt) {
		return nil, wrapErr(ErrSend, "hello short write")
	}

	var ackBuf [128]byte
	an, _, _, _, err := syscall.Recvmsg(fd, ackBuf[:], nil, 0)
	if err != nil {
		return nil, wrapErr(ErrRecv, "hello_ack recv: "+err.Error())
	}
	if an == 0 {
		return nil, wrapErr(ErrRecv, "peer disconnected during handshake")
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
		fd:                      fd,
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

func serverHandshake(fd int, config *ServerConfig, sessionID uint64) (*Session, error) {
	serverPktSize := config.PacketSize
	if serverPktSize == 0 {
		serverPktSize = detectPacketSize(fd)
	}

	sRespPay := applyDefault(config.MaxResponsePayloadBytes, protocol.MaxPayloadDefault)

	var buf [128]byte
	n, _, _, _, err := syscall.Recvmsg(fd, buf[:], nil, 0)
	if err != nil {
		return nil, wrapErr(ErrRecv, "hello recv: "+err.Error())
	}
	if n == 0 {
		return nil, wrapErr(ErrRecv, "peer disconnected during handshake")
	}

	hdr, err := protocol.DecodeHeader(buf[:n])
	if err != nil {
		if errors.Is(err, protocol.ErrBadVersion) &&
			headerVersionIncompatible(buf[:n], protocol.CodeHello) {
			sendRejection(fd, protocol.StatusIncompatible)
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
			sendRejection(fd, protocol.StatusIncompatible)
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
		sendRejection(fd, status)
		return nil, helloAckStatusError(status)
	}
	ack.SessionID = sessionID
	if err := sendHelloAck(fd, protocol.StatusOK, ack); err != nil {
		return nil, wrapErr(ErrSend, "hello_ack send: "+err.Error())
	}

	return &Session{
		fd:                      fd,
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

func sendRejection(fd int, status uint16) {
	_ = sendHelloAck(fd, status, protocol.HelloAck{LayoutVersion: 1})
}

func sendHelloAck(fd int, status uint16, ack protocol.HelloAck) error {
	pkt := framing.BuildHelloAckPacket(status, ack)

	sn, err := syscall.SendmsgN(fd, pkt[:], nil, nil, 0)
	if err != nil {
		return err
	}
	if sn != len(pkt) {
		return wrapErr(ErrSend, "hello_ack short write")
	}
	return nil
}
