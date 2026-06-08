//go:build windows

package windows

import (
	"errors"
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

// Receive reads one logical message. buf is a scratch buffer.
func (s *Session) Receive(buf []byte) (protocol.Header, []byte, error) {
	if s.handle == syscall.InvalidHandle {
		return protocol.Header{}, nil, wrapErr(ErrBadParam, "session closed")
	}

	return framing.SessionReceive(framing.SessionReceiveConfig{
		RoleServer:              s.role == RoleServer,
		PacketSize:              s.PacketSize,
		MaxRequestPayloadBytes:  s.MaxRequestPayloadBytes,
		MaxRequestBatchItems:    s.MaxRequestBatchItems,
		MaxResponsePayloadBytes: s.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   s.MaxResponseBatchItems,
		InflightIDs:             s.inflightIDs,
		RecvBuf:                 &s.recvBuf,
		PacketBuf:               &s.pktBuf,
		Recv:                    func(dst []byte) (int, error) { return rawRecv(s.handle, dst) },
		EnsurePacketScratch:     ensurePipeScratchBuf,
		IsRecvDisconnect:        func(err error) bool { return errors.Is(err, ErrDisconnected) },
		FailAllInflight:         s.failAllInflight,
		ErrLimitExceeded:        func(msg string) error { return wrapErr(ErrLimitExceeded, msg) },
		ErrProtocol:             func(msg string) error { return wrapErr(ErrProtocol, msg) },
		ErrChunk:                func(msg string) error { return wrapErr(ErrChunk, msg) },
		ErrUnknownMsgID:         func(msg string) error { return wrapErr(ErrUnknownMsgID, msg) },
		ErrRecv:                 func(msg string) error { return wrapErr(ErrRecv, msg) },
	}, buf)
}
