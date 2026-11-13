//go:build windows

package windows

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

// platformProfiles are the profiles Windows can provide; unassigned bits are
// never offered or accepted, so a peer can never select a profile without an
// implementation (0x04 is WinSHM busy-wait here, not the POSIX futex profile).
const platformProfiles = protocol.ProfileBaseline | WinShmProfileHybrid | WinShmProfileBusywait

func headerVersionIncompatible(buf []byte, expectedCode uint16) bool {
	return framing.HeaderVersionIncompatible(buf, expectedCode)
}

func helloLayoutIncompatible(buf []byte) bool {
	return framing.HelloLayoutIncompatible(buf)
}

func helloAckLayoutIncompatible(buf []byte) bool {
	return framing.HelloAckLayoutIncompatible(buf)
}

func clientHandshake(handle syscall.Handle, io *pipeIO, config *ClientConfig, wait receiveWait) (*Session, error) {
	pktSize := applyDefault(config.PacketSize, defaultPacketSize)
	pending := &Session{handle: handle, role: RoleClient, io: io}

	// A server that never answers must not hold the client past its
	// deadline; the wait's own error is the result, not a receive failure.
	var waitErr error

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
			PlatformProfiles:        platformProfiles,
		},
		Send: pending.writePacket,
		Recv: func(dst []byte) (int, error) {
			if err := waitHelloAck(pending, wait); err != nil {
				waitErr = err
				return 0, err
			}
			return pending.readPacket(dst)
		},
		StatusError:     helloAckStatusError,
		ErrSend:         func(msg string) error { return wrapErr(ErrSend, msg) },
		ErrRecv:         func(msg string) error { return wrapErr(ErrRecv, msg) },
		ErrProtocol:     func(msg string) error { return wrapErr(ErrProtocol, msg) },
		ErrIncompatible: func(msg string) error { return wrapErr(ErrIncompatible, msg) },
	})
	if waitErr != nil {
		return nil, waitErr
	}
	if err != nil {
		return nil, err
	}

	return &Session{
		handle:                  handle,
		role:                    RoleClient,
		io:                      io,
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

// waitHelloAck waits until HELLO_ACK can be read. A server that rejects the
// client writes its reply and closes, so a disconnected pipe may still hold
// that reply: the read reports what is there.
func waitHelloAck(pending *Session, wait receiveWait) error {
	for {
		waitMs, err := wait.waitMs()
		if err != nil {
			return err
		}
		ready, err := pending.WaitReadable(waitMs)
		if ready || errors.Is(err, ErrDisconnected) {
			return nil
		}
		if err != nil {
			return err
		}
	}
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
	return serverHandshakeWait(handle, config, sessionID, newReceiveWait(0, nil), nil)
}

func serverHandshakeWait(handle syscall.Handle, config *ServerConfig, sessionID uint64, wait receiveWait, prepare ProfilePrepare) (*Session, error) {
	pending := &Session{handle: handle, role: RoleServer}
	serverPktSize := applyDefault(config.PacketSize, defaultPacketSize)
	sRespPay := applyDefault(config.MaxResponsePayloadBytes, protocol.MaxPayloadDefault)
	sReqPay := applyDefault(config.MaxRequestPayloadBytes, protocol.MaxPayloadDefault)

	// The wait's own timeout or abort is returned as is, as in C and Rust.
	var waitErr error
	ack, err := framing.ServerHandshake(framing.ServerHandshakeConfig{
		ServerHelloConfig: framing.ServerHelloConfig{
			PacketSize:              serverPktSize,
			MaxRequestPayloadBytes:  sReqPay,
			MaxResponsePayloadBytes: sRespPay,
			SupportedProfiles:       config.SupportedProfiles,
			PreferredProfiles:       config.PreferredProfiles,
			AuthToken:               config.AuthToken,
			PlatformProfiles:        platformProfiles,
		},
		SessionID: sessionID,
		Prepare:   prepare,
		Recv: func(dst []byte) (int, error) {
			n, err := pending.rawRecvWithTimeout(dst, wait)
			if errors.Is(err, ErrTimeout) || errors.Is(err, ErrAborted) {
				waitErr = err
			}
			return n, err
		},
		SendAck:         func(status uint16, ack protocol.HelloAck) error { return sendHelloAck(handle, status, ack) },
		StatusError:     helloAckStatusError,
		ErrRecv:         func(msg string) error { return wrapErr(ErrRecv, msg) },
		ErrSend:         func(msg string) error { return wrapErr(ErrSend, msg) },
		ErrProtocol:     func(msg string) error { return wrapErr(ErrProtocol, msg) },
		ErrIncompatible: func(msg string) error { return wrapErr(ErrIncompatible, msg) },
	})
	if waitErr != nil {
		return nil, waitErr
	}
	if err != nil {
		return nil, err
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

func sendHelloAck(handle syscall.Handle, status uint16, ack protocol.HelloAck) error {
	pkt := framing.BuildHelloAckPacket(status, ack)
	return rawWrite(handle, pkt[:])
}
