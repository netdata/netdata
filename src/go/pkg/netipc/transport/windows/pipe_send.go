//go:build windows

package windows

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// Send sends one logical message. Fills magic/version/header_len/payload_len.
func (s *Session) Send(hdr *protocol.Header, payload []byte) error {
	if s.handle == syscall.InvalidHandle {
		return wrapErr(ErrBadParam, "session closed")
	}

	totalMsg, err := headerPayloadLen(len(payload))
	if err != nil {
		return err
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

	hdr.Magic = protocol.MagicMsg
	hdr.Version = protocol.Version
	hdr.HeaderLen = protocol.HeaderLen
	hdr.PayloadLen = uint32(len(payload))

	tracked := s.role == RoleClient && hdr.Kind == protocol.KindRequest
	sendErr := s.sendInner(hdr, payload, totalMsg)
	if sendErr != nil && tracked {
		if errors.Is(sendErr, ErrDisconnected) {
			s.failAllInflight()
		} else {
			delete(s.inflightIDs, hdr.MessageID)
		}
	}

	return sendErr
}

func headerPayloadLen(payloadLen int) (int, error) {
	if payloadLen < 0 {
		return 0, wrapErr(ErrLimitExceeded, "total message length exceeds protocol limit")
	}
	totalMsg := uint64(protocol.HeaderSize) + uint64(payloadLen)
	if totalMsg > uint64(^uint32(0)) || totalMsg > uint64(int(^uint(0)>>1)) {
		return 0, wrapErr(ErrLimitExceeded, "total message length exceeds protocol limit")
	}
	return int(totalMsg), nil
}

func (s *Session) sendInner(hdr *protocol.Header, payload []byte, totalMsg int) error {

	if totalMsg <= int(s.PacketSize) {
		msg := ensurePipeScratchBuf(&s.sendBuf, totalMsg)
		hdr.Encode(msg[:protocol.HeaderSize])
		copy(msg[protocol.HeaderSize:], payload)
		return rawSendMsg(s.handle, msg[:totalMsg])
	}

	chunkPayloadBudget := int(s.PacketSize) - protocol.HeaderSize
	if chunkPayloadBudget <= 0 {
		return wrapErr(ErrBadParam, "packet_size too small")
	}

	firstChunkPayload := len(payload)
	if firstChunkPayload > chunkPayloadBudget {
		firstChunkPayload = chunkPayloadBudget
	}

	remainingAfterFirst := len(payload) - firstChunkPayload
	continuationChunks := uint32(0)
	if remainingAfterFirst > 0 {
		continuationChunks = uint32(1 + ((remainingAfterFirst - 1) / chunkPayloadBudget))
	}
	chunkCount := 1 + continuationChunks

	pktBuf := ensurePipeScratchBuf(&s.sendBuf, int(s.PacketSize))
	hdr.Encode(pktBuf[:protocol.HeaderSize])
	copy(pktBuf[protocol.HeaderSize:], payload[:firstChunkPayload])
	if err := rawSendMsg(s.handle, pktBuf[:protocol.HeaderSize+firstChunkPayload]); err != nil {
		return err
	}

	offset := firstChunkPayload
	for ci := uint32(1); ci < chunkCount; ci++ {
		remaining := len(payload) - offset
		thisChunk := remaining
		if thisChunk > chunkPayloadBudget {
			thisChunk = chunkPayloadBudget
		}

		chk := protocol.ChunkHeader{
			Magic:           protocol.MagicChunk,
			Version:         protocol.Version,
			Flags:           0,
			MessageID:       hdr.MessageID,
			TotalMessageLen: uint32(totalMsg),
			ChunkIndex:      ci,
			ChunkCount:      chunkCount,
			ChunkPayloadLen: uint32(thisChunk),
		}

		chk.Encode(pktBuf[:protocol.HeaderSize])
		copy(pktBuf[protocol.HeaderSize:], payload[offset:offset+thisChunk])
		if err := rawSendMsg(s.handle, pktBuf[:protocol.HeaderSize+thisChunk]); err != nil {
			return err
		}

		offset += thisChunk
	}

	return nil
}
