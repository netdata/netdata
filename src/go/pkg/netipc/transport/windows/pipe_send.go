//go:build windows

package windows

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

// Send sends one logical message. Fills magic/version/header_len/payload_len.
func (s *Session) Send(hdr *protocol.Header, payload []byte) error {
	if s.handle == syscall.InvalidHandle {
		return wrapErr(ErrBadParam, "session closed")
	}

	totalMsg, payloadLen, ok := framing.HeaderPayloadLen(len(payload))
	if !ok {
		return wrapErr(ErrLimitExceeded, "total message length exceeds protocol limit")
	}

	if s.role == RoleClient && hdr.Kind == protocol.KindRequest {
		if s.inflightIDs == nil {
			s.inflightIDs = make(map[uint64]struct{})
		}
		if _, exists := s.inflightIDs[hdr.MessageID]; exists {
			return wrapErr(ErrDuplicateMsgID, fmt.Sprintf("message_id %d", hdr.MessageID))
		}
		s.inflightIDs[hdr.MessageID] = struct{}{}
	}

	framing.FillMessageHeader(hdr, payloadLen)

	tracked := s.role == RoleClient && hdr.Kind == protocol.KindRequest
	sendErr := framing.Sender{
		PacketSize: s.PacketSize,
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
		ErrBadParam: func(msg string) error { return wrapErr(ErrBadParam, msg) },
	}.Send(hdr, payload, totalMsg)
	if sendErr != nil && tracked {
		if errors.Is(sendErr, ErrDisconnected) {
			s.failAllInflight()
		} else {
			delete(s.inflightIDs, hdr.MessageID)
		}
	}

	return sendErr
}
