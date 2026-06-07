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

	maxPayload := s.MaxResponsePayloadBytes
	if s.role == RoleServer {
		maxPayload = s.MaxRequestPayloadBytes
	}
	maxBatch := s.MaxResponseBatchItems
	if s.role == RoleServer {
		maxBatch = s.MaxRequestBatchItems
	}

	return framing.Receiver{
		PacketSize:     s.PacketSize,
		MaxPayload:     maxPayload,
		MaxBatchItems:  maxBatch,
		TrackResponses: s.role == RoleClient,
		InflightIDs:    s.inflightIDs,
		RecvBuf:        &s.recvBuf,
		PacketBuf:      &s.pktBuf,
		Recv: func(dst []byte) (int, error) {
			return rawRecv(s.handle, dst)
		},
		EnsurePacketScratch: ensurePipeScratchBuf,
		OnRecvError: func(err error) {
			if errors.Is(err, ErrDisconnected) {
				s.failAllInflight()
			}
		},
		ErrLimitExceeded: func(msg string) error { return wrapErr(ErrLimitExceeded, msg) },
		ErrProtocol:      func(msg string) error { return wrapErr(ErrProtocol, msg) },
		ErrChunk:         func(msg string) error { return wrapErr(ErrChunk, msg) },
		ErrUnknownMsgID:  func(msg string) error { return wrapErr(ErrUnknownMsgID, msg) },
		ErrRecv:          func(msg string) error { return wrapErr(ErrRecv, msg) },
	}.Receive(buf)
}
