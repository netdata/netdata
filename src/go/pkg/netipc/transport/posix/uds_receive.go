//go:build unix

package posix

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

// Receive reads one logical message. Blocks until a complete message
// arrives. buf is a caller-provided scratch buffer for the first packet.
// On success, returns the header and a payload view valid until the next
// Receive call on this session.
func (s *Session) Receive(buf []byte) (protocol.Header, []byte, error) {
	if s.fd < 0 {
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
		Recv:                    func(dst []byte) (int, error) { return rawRecv(s.fd, dst) },
		EnsurePacketScratch:     ensureScratchBuf,
		IsRecvDisconnect:        func(err error) bool { return errors.Is(err, ErrRecv) },
		FailAllInflight:         s.failAllInflight,
		ErrLimitExceeded:        func(msg string) error { return wrapErr(ErrLimitExceeded, msg) },
		ErrProtocol:             func(msg string) error { return wrapErr(ErrProtocol, msg) },
		ErrChunk:                func(msg string) error { return wrapErr(ErrChunk, msg) },
		ErrUnknownMsgID:         func(msg string) error { return wrapErr(ErrUnknownMsgID, msg) },
		ErrRecv:                 func(msg string) error { return wrapErr(ErrRecv, msg) },
	}, buf)
}
