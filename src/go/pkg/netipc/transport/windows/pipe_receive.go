//go:build windows

package windows

import (
	"errors"
	"fmt"
	"syscall"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// Receive reads one logical message. buf is a scratch buffer.
func (s *Session) Receive(buf []byte) (protocol.Header, []byte, error) {
	if s.handle == syscall.InvalidHandle {
		return protocol.Header{}, nil, wrapErr(ErrBadParam, "session closed")
	}

	n, err := rawRecv(s.handle, buf)
	if err != nil {
		if errors.Is(err, ErrDisconnected) {
			s.failAllInflight()
		}
		return protocol.Header{}, nil, err
	}

	if n < protocol.HeaderSize {
		return protocol.Header{}, nil, wrapErr(ErrProtocol, "packet too short for header")
	}

	hdr, err := protocol.DecodeHeader(buf[:n])
	if err != nil {
		return protocol.Header{}, nil, wrapErr(ErrProtocol, "header decode: "+err.Error())
	}

	if err := validateInboundLimits(s, hdr); err != nil {
		return protocol.Header{}, nil, err
	}
	if err := trackInboundResponse(s, hdr); err != nil {
		return protocol.Header{}, nil, err
	}

	totalMsg := protocol.HeaderSize + int(hdr.PayloadLen)
	if n >= totalMsg {
		payload := buf[protocol.HeaderSize : protocol.HeaderSize+int(hdr.PayloadLen)]
		if err := validateBatchPayload(hdr, payload); err != nil {
			return protocol.Header{}, nil, err
		}
		return hdr, payload, nil
	}

	return s.receiveChunked(buf, n, hdr, totalMsg)
}

func validateInboundLimits(s *Session, hdr protocol.Header) error {
	maxPayload := s.MaxResponsePayloadBytes
	if s.role == RoleServer {
		maxPayload = s.MaxRequestPayloadBytes
	}
	if hdr.PayloadLen > maxPayload {
		return wrapErr(ErrLimitExceeded,
			fmt.Sprintf("payload_len %d exceeds negotiated max %d", hdr.PayloadLen, maxPayload))
	}

	maxBatch := s.MaxResponseBatchItems
	if s.role == RoleServer {
		maxBatch = s.MaxRequestBatchItems
	}
	if hdr.ItemCount > maxBatch {
		return wrapErr(ErrLimitExceeded,
			fmt.Sprintf("item_count %d exceeds negotiated max %d", hdr.ItemCount, maxBatch))
	}

	return nil
}

func trackInboundResponse(s *Session, hdr protocol.Header) error {
	if s.role != RoleClient || hdr.Kind != protocol.KindResponse {
		return nil
	}
	if s.inflightIDs == nil {
		return wrapErr(ErrUnknownMsgID, fmt.Sprintf("message_id %d", hdr.MessageID))
	}
	if _, exists := s.inflightIDs[hdr.MessageID]; !exists {
		return wrapErr(ErrUnknownMsgID, fmt.Sprintf("message_id %d", hdr.MessageID))
	}
	delete(s.inflightIDs, hdr.MessageID)
	return nil
}

func validateBatchPayload(hdr protocol.Header, payload []byte) error {
	if hdr.Flags&protocol.FlagBatch == 0 || hdr.ItemCount <= 1 {
		return nil
	}

	dirBytes := int(hdr.ItemCount) * 8
	dirAligned := protocol.Align8(dirBytes)
	if len(payload) < dirAligned {
		return wrapErr(ErrProtocol, "batch dir exceeds payload")
	}
	packedAreaLen := uint32(len(payload) - dirAligned)
	if err := protocol.BatchDirValidate(payload[:dirBytes], hdr.ItemCount, packedAreaLen); err != nil {
		return wrapErr(ErrProtocol, "batch dir: "+err.Error())
	}
	return nil
}

func (s *Session) receiveChunked(
	buf []byte,
	firstPacketLen int,
	hdr protocol.Header,
	totalMsg int,
) (protocol.Header, []byte, error) {
	firstPayloadBytes := firstPacketLen - protocol.HeaderSize
	needed := int(hdr.PayloadLen)
	if len(s.recvBuf) < needed {
		s.recvBuf = make([]byte, needed)
	}

	copy(s.recvBuf[:firstPayloadBytes],
		buf[protocol.HeaderSize:protocol.HeaderSize+firstPayloadBytes])

	assembled := firstPayloadBytes
	chunkPayloadBudget := int(s.PacketSize) - protocol.HeaderSize
	expectedChunkCount := expectedReceiveChunkCount(
		int(hdr.PayloadLen), firstPayloadBytes, chunkPayloadBudget)

	pktBuf := ensurePipeScratchBuf(&s.pktBuf, int(s.PacketSize))
	for ci := uint32(1); assembled < int(hdr.PayloadLen); ci++ {
		err := s.receiveOneChunk(pktBuf, hdr, ci, expectedChunkCount,
			totalMsg, &assembled)
		if err != nil {
			return protocol.Header{}, nil, err
		}
	}

	payload := s.recvBuf[:hdr.PayloadLen]
	if err := validateBatchPayload(hdr, payload); err != nil {
		return protocol.Header{}, nil, err
	}
	return hdr, payload, nil
}

func expectedReceiveChunkCount(payloadLen, firstPayloadBytes, chunkPayloadBudget int) uint32 {
	remainingAfterFirst := payloadLen - firstPayloadBytes
	expectedContinuations := uint32(0)
	if remainingAfterFirst > 0 && chunkPayloadBudget > 0 {
		expectedContinuations = uint32(1 + ((remainingAfterFirst - 1) / chunkPayloadBudget))
	}
	return 1 + expectedContinuations
}

func (s *Session) receiveOneChunk(
	pktBuf []byte,
	hdr protocol.Header,
	chunkIndex uint32,
	expectedChunkCount uint32,
	totalMsg int,
	assembled *int,
) error {
	cn, err := rawRecv(s.handle, pktBuf)
	if err != nil {
		if errors.Is(err, ErrDisconnected) {
			s.failAllInflight()
		}
		return wrapErr(ErrRecv, "continuation recv: "+err.Error())
	}
	if cn < protocol.HeaderSize {
		return wrapErr(ErrChunk, "continuation too short")
	}

	chk, err := protocol.DecodeChunkHeader(pktBuf[:cn])
	if err != nil {
		return wrapErr(ErrChunk, "chunk header: "+err.Error())
	}
	if err := validateReceiveChunk(chk, hdr, chunkIndex, expectedChunkCount, totalMsg); err != nil {
		return err
	}

	chunkData := cn - protocol.HeaderSize
	if chunkData != int(chk.ChunkPayloadLen) {
		return wrapErr(ErrChunk, "chunk_payload_len mismatch")
	}
	if *assembled+chunkData > int(hdr.PayloadLen) {
		return wrapErr(ErrChunk, "chunk exceeds payload_len")
	}

	copy(s.recvBuf[*assembled:*assembled+chunkData],
		pktBuf[protocol.HeaderSize:protocol.HeaderSize+chunkData])
	*assembled += chunkData
	return nil
}

func validateReceiveChunk(
	chk protocol.ChunkHeader,
	hdr protocol.Header,
	chunkIndex uint32,
	expectedChunkCount uint32,
	totalMsg int,
) error {
	if chk.MessageID != hdr.MessageID {
		return wrapErr(ErrChunk, "message_id mismatch")
	}
	if chk.ChunkIndex != chunkIndex {
		return wrapErr(ErrChunk, fmt.Sprintf(
			"chunk_index mismatch: expected %d, got %d", chunkIndex, chk.ChunkIndex))
	}
	if chk.ChunkCount != expectedChunkCount {
		return wrapErr(ErrChunk, "chunk_count mismatch")
	}
	if chk.TotalMessageLen != uint32(totalMsg) {
		return wrapErr(ErrChunk, "total_message_len mismatch")
	}
	return nil
}
