//go:build unix

package posix

import (
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

	ack, err := framing.ClientHandshake(framing.ClientHandshakeConfig{
		Hello: framing.HelloConfig{
			SupportedProfiles:       config.SupportedProfiles,
			PreferredProfiles:       config.PreferredProfiles,
			MaxRequestPayloadBytes:  applyDefault(config.MaxRequestPayloadBytes, protocol.MaxPayloadDefault),
			MaxRequestBatchItems:    applyDefault(config.MaxRequestBatchItems, defaultBatchItems),
			MaxResponsePayloadBytes: applyDefault(config.MaxResponsePayloadBytes, protocol.MaxPayloadDefault),
			MaxResponseBatchItems:   applyDefault(config.MaxResponseBatchItems, defaultBatchItems),
			AuthToken:               config.AuthToken,
			PacketSize:              pktSize,
		},
		Send: func(pkt []byte) error {
			n, err := syscall.SendmsgN(fd, pkt, nil, nil, 0)
			if err != nil {
				return err
			}
			if n != len(pkt) {
				return fmt.Errorf("short write")
			}
			return nil
		},
		Recv:            func(dst []byte) (int, error) { return rawRecv(fd, dst) },
		StatusError:     helloAckStatusError,
		ErrSend:         func(msg string) error { return wrapErr(ErrSend, msg) },
		ErrRecv:         func(msg string) error { return wrapErr(ErrRecv, msg) },
		ErrProtocol:     func(msg string) error { return wrapErr(ErrProtocol, msg) },
		ErrIncompatible: func(msg string) error { return wrapErr(ErrIncompatible, msg) },
	})
	if err != nil {
		return nil, err
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
	ack, err := framing.ServerHandshake(framing.ServerHandshakeConfig{
		ServerHelloConfig: framing.ServerHelloConfig{
			PacketSize:              serverPktSize,
			MaxResponsePayloadBytes: sRespPay,
			SupportedProfiles:       config.SupportedProfiles,
			PreferredProfiles:       config.PreferredProfiles,
			AuthToken:               config.AuthToken,
		},
		SessionID:       sessionID,
		Recv:            func(dst []byte) (int, error) { return rawRecv(fd, dst) },
		SendAck:         func(status uint16, ack protocol.HelloAck) error { return sendHelloAck(fd, status, ack) },
		StatusError:     helloAckStatusError,
		ErrRecv:         func(msg string) error { return wrapErr(ErrRecv, msg) },
		ErrSend:         func(msg string) error { return wrapErr(ErrSend, msg) },
		ErrProtocol:     func(msg string) error { return wrapErr(ErrProtocol, msg) },
		ErrIncompatible: func(msg string) error { return wrapErr(ErrIncompatible, msg) },
	})
	if err != nil {
		return nil, err
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
