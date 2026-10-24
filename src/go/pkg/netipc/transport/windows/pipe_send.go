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
	if s.handle == syscall.InvalidHandle {
		return wrapErr(ErrBadParam, "session closed")
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
			return rawSendMsg(s.handle, msg[:packetLen])
		},
		SendChunk: func(chk protocol.ChunkHeader, chunkPayload []byte) error {
			pktLen := protocol.HeaderSize + len(chunkPayload)
			msg := ensurePipeScratchBuf(&s.sendBuf, pktLen)
			chk.Encode(msg[:protocol.HeaderSize])
			copy(msg[protocol.HeaderSize:], chunkPayload)
			return rawSendMsg(s.handle, msg[:pktLen])
		},
		ErrLimitExceeded:  func(msg string) error { return wrapErr(ErrLimitExceeded, msg) },
		ErrDuplicateMsgID: func(msg string) error { return wrapErr(ErrDuplicateMsgID, msg) },
		ErrBadParam:       func(msg string) error { return wrapErr(ErrBadParam, msg) },
	}, hdr, payload)
}
