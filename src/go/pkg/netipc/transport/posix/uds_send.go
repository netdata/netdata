//go:build unix

package posix

import (
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

// Send sends one logical message. The caller fills Kind, Code, Flags,
// ItemCount, MessageID in hdr; this function sets Magic/Version/
// HeaderLen/PayloadLen. If the total message exceeds PacketSize,
// it is chunked transparently.
func (s *Session) Send(hdr *protocol.Header, payload []byte) error {
	if s.fd < 0 {
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
		SendFirstPacket: func(packetHdr *protocol.Header, packetPayload []byte, _ int) error {
			var hdrBuf [protocol.HeaderSize]byte
			packetHdr.Encode(hdrBuf[:])
			return rawSendIov(s.fd, hdrBuf[:], packetPayload)
		},
		SendChunk: func(chk protocol.ChunkHeader, chunkPayload []byte) error {
			var chkBuf [protocol.HeaderSize]byte
			chk.Encode(chkBuf[:])
			return rawSendIov(s.fd, chkBuf[:], chunkPayload)
		},
		ErrBadParam: func(msg string) error { return wrapErr(ErrBadParam, msg) },
	}.Send(hdr, payload, totalMsg)
	if sendErr != nil && tracked {
		if errors.Is(sendErr, ErrSend) {
			s.failAllInflight()
		} else {
			delete(s.inflightIDs, hdr.MessageID)
		}
	}

	return sendErr
}
