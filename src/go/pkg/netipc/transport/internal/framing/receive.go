package framing

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// Receiver assembles one framed NetIPC message over a transport-specific
// packet receive function.
type Receiver struct {
	PacketSize uint32

	MaxPayload    uint32
	MaxBatchItems uint32

	TrackResponses bool
	InflightIDs    map[uint64]struct{}

	RecvBuf   *[]byte
	PacketBuf *[]byte

	Recv                func([]byte) (int, error)
	EnsurePacketScratch func(*[]byte, int) []byte
	OnRecvError         func(error)

	ErrLimitExceeded func(string) error
	ErrProtocol      func(string) error
	ErrChunk         func(string) error
	ErrUnknownMsgID  func(string) error
	ErrRecv          func(string) error
}

// Receive reads and validates one complete logical message.
func (r Receiver) Receive(buf []byte) (protocol.Header, []byte, error) {
	n, err := r.Recv(buf)
	if err != nil {
		r.noteRecvError(err)
		return protocol.Header{}, nil, err
	}

	if n < protocol.HeaderSize {
		return protocol.Header{}, nil, r.ErrProtocol("packet too short for header")
	}

	hdr, err := protocol.DecodeHeader(buf[:n])
	if err != nil {
		return protocol.Header{}, nil, r.ErrProtocol("header decode: " + err.Error())
	}

	if err := r.validateInboundLimits(hdr); err != nil {
		return protocol.Header{}, nil, err
	}
	if err := r.trackInboundResponse(hdr); err != nil {
		return protocol.Header{}, nil, err
	}

	totalMsg, err := r.totalMessageLen(hdr)
	if err != nil {
		return protocol.Header{}, nil, err
	}
	payloadLen := int(hdr.PayloadLen)
	if n >= totalMsg {
		payload := buf[protocol.HeaderSize : protocol.HeaderSize+payloadLen]
		if err := r.validateBatchPayload(hdr, payload); err != nil {
			return protocol.Header{}, nil, err
		}
		return hdr, payload, nil
	}

	return r.receiveChunked(buf, n, hdr, totalMsg)
}

func (r Receiver) totalMessageLen(hdr protocol.Header) (int, error) {
	maxInt := uint64(int(^uint(0) >> 1))
	totalMsg := uint64(protocol.HeaderSize) + uint64(hdr.PayloadLen)
	if totalMsg > maxInt {
		return 0, r.ErrLimitExceeded("total message length exceeds platform limit")
	}
	return int(totalMsg), nil
}

func (r Receiver) noteRecvError(err error) {
	if r.OnRecvError != nil {
		r.OnRecvError(err)
	}
}

func (r Receiver) validateInboundLimits(hdr protocol.Header) error {
	if hdr.PayloadLen > r.MaxPayload {
		return r.ErrLimitExceeded(
			fmt.Sprintf("payload_len %d exceeds negotiated max %d", hdr.PayloadLen, r.MaxPayload))
	}
	if hdr.ItemCount > r.MaxBatchItems {
		return r.ErrLimitExceeded(
			fmt.Sprintf("item_count %d exceeds negotiated max %d", hdr.ItemCount, r.MaxBatchItems))
	}
	return nil
}

func (r Receiver) trackInboundResponse(hdr protocol.Header) error {
	if !r.TrackResponses || hdr.Kind != protocol.KindResponse {
		return nil
	}
	if r.InflightIDs == nil {
		return r.ErrUnknownMsgID(fmt.Sprintf("message_id %d", hdr.MessageID))
	}
	if _, exists := r.InflightIDs[hdr.MessageID]; !exists {
		return r.ErrUnknownMsgID(fmt.Sprintf("message_id %d", hdr.MessageID))
	}
	delete(r.InflightIDs, hdr.MessageID)
	return nil
}

func (r Receiver) validateBatchPayload(hdr protocol.Header, payload []byte) error {
	if hdr.Flags&protocol.FlagBatch == 0 || hdr.ItemCount <= 1 {
		return nil
	}

	dirBytes64 := uint64(hdr.ItemCount) * 8
	if dirBytes64 > uint64(int(^uint(0)>>1)) {
		return r.ErrProtocol("batch dir exceeds platform limit")
	}
	dirBytes := int(dirBytes64)
	dirAligned := protocol.Align8(dirBytes)
	if len(payload) < dirAligned {
		return r.ErrProtocol("batch dir exceeds payload")
	}
	packedAreaLen := uint32(len(payload) - dirAligned)
	if err := protocol.BatchDirValidate(payload[:dirBytes], hdr.ItemCount, packedAreaLen); err != nil {
		return r.ErrProtocol("batch dir: " + err.Error())
	}
	return nil
}

func (r Receiver) receiveChunked(
	buf []byte,
	firstPacketLen int,
	hdr protocol.Header,
	totalMsg int,
) (protocol.Header, []byte, error) {
	firstPayloadBytes := firstPacketLen - protocol.HeaderSize
	needed := int(hdr.PayloadLen)
	if len(*r.RecvBuf) < needed {
		*r.RecvBuf = make([]byte, needed)
	}

	copy((*r.RecvBuf)[:firstPayloadBytes],
		buf[protocol.HeaderSize:protocol.HeaderSize+firstPayloadBytes])

	assembled := firstPayloadBytes
	chunkPayloadBudget := int(r.PacketSize) - protocol.HeaderSize
	expectedChunkCount := expectedReceiveChunkCount(
		int(hdr.PayloadLen), firstPayloadBytes, chunkPayloadBudget)

	pktBuf := r.EnsurePacketScratch(r.PacketBuf, int(r.PacketSize))
	for ci := uint32(1); assembled < int(hdr.PayloadLen); ci++ {
		err := r.receiveOneChunk(pktBuf, hdr, ci, expectedChunkCount,
			totalMsg, &assembled)
		if err != nil {
			return protocol.Header{}, nil, err
		}
	}

	payload := (*r.RecvBuf)[:int(hdr.PayloadLen)]
	if err := r.validateBatchPayload(hdr, payload); err != nil {
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

func (r Receiver) receiveOneChunk(
	pktBuf []byte,
	hdr protocol.Header,
	chunkIndex uint32,
	expectedChunkCount uint32,
	totalMsg int,
	assembled *int,
) error {
	cn, err := r.Recv(pktBuf)
	if err != nil {
		r.noteRecvError(err)
		return r.ErrRecv("continuation recv: " + err.Error())
	}
	if cn < protocol.HeaderSize {
		return r.ErrChunk("continuation too short")
	}

	chk, err := protocol.DecodeChunkHeader(pktBuf[:cn])
	if err != nil {
		return r.ErrChunk("chunk header: " + err.Error())
	}
	if err := r.validateReceiveChunk(chk, hdr, chunkIndex, expectedChunkCount, totalMsg); err != nil {
		return err
	}

	chunkData := cn - protocol.HeaderSize
	if chunkData != int(chk.ChunkPayloadLen) {
		return r.ErrChunk("chunk_payload_len mismatch")
	}
	if *assembled+chunkData > int(hdr.PayloadLen) {
		return r.ErrChunk("chunk exceeds payload_len")
	}

	copy((*r.RecvBuf)[*assembled:*assembled+chunkData],
		pktBuf[protocol.HeaderSize:protocol.HeaderSize+chunkData])
	*assembled += chunkData
	return nil
}

func (r Receiver) validateReceiveChunk(
	chk protocol.ChunkHeader,
	hdr protocol.Header,
	chunkIndex uint32,
	expectedChunkCount uint32,
	totalMsg int,
) error {
	if chk.MessageID != hdr.MessageID {
		return r.ErrChunk("message_id mismatch")
	}
	if chk.ChunkIndex != chunkIndex {
		return r.ErrChunk(fmt.Sprintf(
			"chunk_index mismatch: expected %d, got %d", chunkIndex, chk.ChunkIndex))
	}
	if chk.ChunkCount != expectedChunkCount {
		return r.ErrChunk("chunk_count mismatch")
	}
	if chk.TotalMessageLen != uint32(totalMsg) {
		return r.ErrChunk("total_message_len mismatch")
	}
	return nil
}
