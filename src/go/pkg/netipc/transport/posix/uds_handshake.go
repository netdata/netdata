//go:build unix

package posix

import (
	"errors"
	"fmt"
	"syscall"
	"time"

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

// connectSliceMs is the longest single connect wait, so an abort ends a
// connect promptly.
const connectSliceMs = 100

// connectBounded connects fd to path. A blocking AF_UNIX connect waits while
// the listener's backlog is full. On Linux SO_SNDTIMEO bounds that wait and
// the connect fails with EAGAIN when it expires; waiting in slices lets an
// abort end it. Other systems refuse a full backlog at once.
func connectBounded(fd int, path string, wait receiveWait) error {
	sa := &syscall.SockaddrUnix{Name: path}
	defer func() {
		// Session sends block without a timeout.
		_ = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &syscall.Timeval{})
	}()
	for {
		sliceMs, err := wait.waitMs()
		if err != nil {
			return err
		}
		sliceMs = min(sliceMs, connectSliceMs)
		tv := syscall.NsecToTimeval(int64(sliceMs) * int64(time.Millisecond))
		_ = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &tv)

		err = syscall.Connect(fd, sa)
		switch err {
		case nil, syscall.EISCONN:
			return nil
		case syscall.EAGAIN, syscall.EINTR, syscall.EALREADY:
			continue
		default:
			return wrapErr(ErrConnect, err.Error())
		}
	}
}

func connectAndHandshake(fd int, path string, config *ClientConfig, wait receiveWait) (*Session, error) {
	if err := connectBounded(fd, path, wait); err != nil {
		return nil, err
	}

	pktSize := packetSizeLimit(fd, config.PacketSize)

	// A server that never answers must not hold the client past its
	// deadline; the wait's own error is the result, not a receive failure.
	var waitErr error

	ack, err := framing.ClientHandshake(framing.ClientHandshakeConfig{
		Hello: framing.HelloConfig{
			SupportedProfiles:       config.SupportedProfiles,
			PreferredProfiles:       config.PreferredProfiles,
			PlatformProfiles:        platformProfiles,
			MaxRequestPayloadBytes:  applyDefault(config.MaxRequestPayloadBytes, protocol.MaxPayloadDefault),
			MaxRequestBatchItems:    applyDefault(config.MaxRequestBatchItems, defaultBatchItems),
			MaxResponsePayloadBytes: applyDefault(config.MaxResponsePayloadBytes, protocol.MaxPayloadDefault),
			MaxResponseBatchItems:   applyDefault(config.MaxResponseBatchItems, defaultBatchItems),
			AuthToken:               config.AuthToken,
			PacketSize:              pktSize,
		},
		Send: func(pkt []byte) error {
			if streamTransport {
				return streamSendAll(fd, pkt, nil)
			}
			n, err := syscall.SendmsgN(fd, pkt, nil, nil, 0)
			if err != nil {
				return err
			}
			if n != len(pkt) {
				return fmt.Errorf("short write")
			}
			return nil
		},
		Recv: func(dst []byte) (int, error) {
			if streamTransport {
				// A packet cut short is a protocol error, not a receive
				// failure, as on SEQPACKET.
				n, err := streamRecvPacket(fd, dst, 0, &wait)
				if errors.Is(err, ErrTimeout) || errors.Is(err, ErrAborted) || errors.Is(err, ErrProtocol) {
					waitErr = err
				}
				return n, err
			}
			if err := pollReadableForReceive(fd, wait); err != nil {
				waitErr = err
				return 0, err
			}
			return rawRecv(fd, dst)
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
	return serverHandshakeWait(fd, config, sessionID, newReceiveWait(0, nil), nil)
}

func serverHandshakeWait(fd int, config *ServerConfig, sessionID uint64, wait receiveWait, prepare ProfilePrepare) (*Session, error) {
	serverPktSize := packetSizeLimit(fd, config.PacketSize)

	// The wait's own timeout or abort is returned as is, as in C and Rust;
	// a stream HELLO cut short is a protocol error, as on SEQPACKET.
	var waitErr error
	sRespPay := applyDefault(config.MaxResponsePayloadBytes, protocol.MaxPayloadDefault)
	sReqPay := applyDefault(config.MaxRequestPayloadBytes, protocol.MaxPayloadDefault)
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
			n, err := serverHandshakeRecv(fd, dst, &wait)
			if errors.Is(err, ErrTimeout) || errors.Is(err, ErrAborted) ||
				(streamTransport && errors.Is(err, ErrProtocol)) {
				waitErr = err
			}
			return n, err
		},
		SendAck:         func(status uint16, ack protocol.HelloAck) error { return sendHelloAck(fd, status, ack) },
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

func serverHandshakeRecv(fd int, dst []byte, wait *receiveWait) (int, error) {
	if streamTransport {
		return streamRecvPacket(fd, dst, 0, wait)
	}
	return rawRecvWithTimeout(fd, dst, *wait)
}

func sendHelloAck(fd int, status uint16, ack protocol.HelloAck) error {
	pkt := framing.BuildHelloAckPacket(status, ack)
	if streamTransport {
		return streamSendAll(fd, pkt[:], nil)
	}

	sn, err := syscall.SendmsgN(fd, pkt[:], nil, nil, 0)
	if err != nil {
		return err
	}
	if sn != len(pkt) {
		return wrapErr(ErrSend, "hello_ack short write")
	}
	return nil
}
