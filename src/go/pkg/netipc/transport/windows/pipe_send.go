//go:build windows

package windows

import (
	"errors"
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

// Send sends one logical message. Fills magic/version/header_len/payload_len.
func (s *Session) Send(hdr *protocol.Header, payload []byte) error {
	return s.SendTimeout(hdr, payload, 0, nil)
}

// SendTimeout is Send with an optional deadline and abort channel for a
// client session. timeoutMs == 0 waits without a deadline. If the deadline
// passes or abortCh is closed before a write completes, the write is
// cancelled, the session is closed (later calls fail with ErrBadParam) and
// it returns ErrTimeout or ErrAborted, since part of the message may be on
// the wire.
func (s *Session) SendTimeout(hdr *protocol.Header, payload []byte, timeoutMs uint32, abortCh <-chan struct{}) error {
	if s.handle == syscall.InvalidHandle {
		return wrapErr(ErrBadParam, "session closed")
	}
	s.sendBounded = s.io != nil && (timeoutMs != 0 || abortCh != nil)
	if s.sendBounded {
		s.sendWait = newReceiveWait(timeoutMs, abortCh)
	}

	// One request in flight (every typed call) writes without draining;
	// only a pipelining client can deadlock with a server blocked writing
	// to it.
	inflight := len(s.inflightIDs)
	if s.role == RoleClient && hdr.Kind == protocol.KindRequest {
		inflight++
	}
	s.progressBound = 0
	if s.io != nil && inflight > 1 {
		s.progressBound = pendingBound(s.MaxResponsePayloadBytes, s.PacketSize, inflight)
	}

	return framing.SessionSend(framing.SessionSendConfig{
		RoleClient:       s.role == RoleClient,
		PacketSize:       s.PacketSize,
		InflightIDs:      &s.inflightIDs,
		FailAllInflight:  s.failAllInflight,
		IsSendDisconnect: func(err error) bool { return errors.Is(err, ErrDisconnected) },
		SendFirstPacket: func(packetHdr *protocol.Header, packetPayload []byte, packetLen int) error {
			msg := ensurePipeScratchBuf(&s.sendBuf, packetLen)
			packetHdr.Encode(msg[:protocol.HeaderSize])
			copy(msg[protocol.HeaderSize:], packetPayload)
			return s.writePacket(msg[:packetLen])
		},
		SendChunk: func(chk protocol.ChunkHeader, chunkPayload []byte) error {
			pktLen := protocol.HeaderSize + len(chunkPayload)
			msg := ensurePipeScratchBuf(&s.sendBuf, pktLen)
			chk.Encode(msg[:protocol.HeaderSize])
			copy(msg[protocol.HeaderSize:], chunkPayload)
			return s.writePacket(msg[:pktLen])
		},
		ErrLimitExceeded:  func(msg string) error { return wrapErr(ErrLimitExceeded, msg) },
		ErrDuplicateMsgID: func(msg string) error { return wrapErr(ErrDuplicateMsgID, msg) },
		ErrBadParam:       func(msg string) error { return wrapErr(ErrBadParam, msg) },
	}, hdr, payload)
}
