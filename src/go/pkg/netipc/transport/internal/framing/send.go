package framing

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
)

// HeaderPayloadLen validates header + payload before lengths are encoded into
// the protocol's uint32 wire fields.
func HeaderPayloadLen(payloadLen int) (int, uint32, bool) {
	if payloadLen < 0 {
		return 0, 0, false
	}
	totalMsg := uint64(protocol.HeaderSize) + uint64(payloadLen)
	if totalMsg > uint64(^uint32(0)) || totalMsg > uint64(int(^uint(0)>>1)) {
		return 0, 0, false
	}
	payloadU32, ok := checkedU32(payloadLen)
	if !ok {
		return 0, 0, false
	}
	return int(totalMsg), payloadU32, true
}

// FillMessageHeader applies the common NetIPC message header fields.
func FillMessageHeader(hdr *protocol.Header, payloadLen uint32) {
	hdr.Magic = protocol.MagicMsg
	hdr.Version = protocol.Version
	hdr.HeaderLen = protocol.HeaderLen
	hdr.PayloadLen = payloadLen
}

// Sender chunks one logical NetIPC message over a transport-specific packet
// writer.
type Sender struct {
	PacketSize uint32

	SendFirstPacket func(*protocol.Header, []byte, int) error
	SendChunk       func(protocol.ChunkHeader, []byte) error
	ErrBadParam     func(string) error
}

type SessionSendConfig struct {
	RoleClient bool
	PacketSize uint32

	InflightIDs      *map[uint64]struct{}
	FailAllInflight  func()
	IsSendDisconnect func(error) bool

	SendFirstPacket func(*protocol.Header, []byte, int) error
	SendChunk       func(protocol.ChunkHeader, []byte) error

	ErrLimitExceeded  func(string) error
	ErrDuplicateMsgID func(string) error
	ErrBadParam       func(string) error
}

func SessionSend(config SessionSendConfig, hdr *protocol.Header, payload []byte) error {
	totalMsg, payloadLen, ok := HeaderPayloadLen(len(payload))
	if !ok {
		return config.ErrLimitExceeded("total message length exceeds protocol limit")
	}

	tracked, err := trackOutboundRequest(config, hdr)
	if err != nil {
		return err
	}

	FillMessageHeader(hdr, payloadLen)
	sendErr := Sender{
		PacketSize:      config.PacketSize,
		SendFirstPacket: config.SendFirstPacket,
		SendChunk:       config.SendChunk,
		ErrBadParam:     config.ErrBadParam,
	}.Send(hdr, payload, totalMsg)

	if sendErr != nil && tracked {
		if config.IsSendDisconnect(sendErr) {
			config.FailAllInflight()
		} else {
			delete(*config.InflightIDs, hdr.MessageID)
		}
	}

	return sendErr
}

func trackOutboundRequest(config SessionSendConfig, hdr *protocol.Header) (bool, error) {
	if !config.RoleClient || hdr.Kind != protocol.KindRequest {
		return false, nil
	}

	if *config.InflightIDs == nil {
		*config.InflightIDs = make(map[uint64]struct{})
	}
	if _, exists := (*config.InflightIDs)[hdr.MessageID]; exists {
		return false, config.ErrDuplicateMsgID(fmt.Sprintf("message_id %d", hdr.MessageID))
	}
	(*config.InflightIDs)[hdr.MessageID] = struct{}{}
	return true, nil
}

// Send writes one complete logical message.
func (s Sender) Send(hdr *protocol.Header, payload []byte, totalMsg int) error {
	packetSize, err := s.packetSize()
	if err != nil {
		return err
	}
	if totalMsg <= packetSize {
		return s.SendFirstPacket(hdr, payload, totalMsg)
	}

	chunkPayloadBudget := packetSize - protocol.HeaderSize
	if chunkPayloadBudget <= 0 {
		return s.ErrBadParam("packet_size too small")
	}

	firstChunkPayload := minInt(len(payload), chunkPayloadBudget)
	remainingAfterFirst := len(payload) - firstChunkPayload
	continuationChunks := 0
	if remainingAfterFirst > 0 {
		continuationChunks = 1 + ((remainingAfterFirst - 1) / chunkPayloadBudget)
	}
	chunkCount, ok := checkedU32(1 + continuationChunks)
	if !ok {
		return s.ErrBadParam("chunk count exceeds protocol limit")
	}
	totalMsgU32, ok := checkedU32(totalMsg)
	if !ok {
		return s.ErrBadParam("total message length exceeds protocol limit")
	}

	if err := s.SendFirstPacket(hdr, payload[:firstChunkPayload],
		protocol.HeaderSize+firstChunkPayload); err != nil {
		return err
	}

	offset := firstChunkPayload
	for ci := uint32(1); ci < chunkCount; ci++ {
		remaining := len(payload) - offset
		thisChunk := minInt(remaining, chunkPayloadBudget)
		chunkLen, ok := checkedU32(thisChunk)
		if !ok {
			return s.ErrBadParam("chunk payload length exceeds protocol limit")
		}

		chk := protocol.ChunkHeader{
			Magic:           protocol.MagicChunk,
			Version:         protocol.Version,
			Flags:           0,
			MessageID:       hdr.MessageID,
			TotalMessageLen: totalMsgU32,
			ChunkIndex:      ci,
			ChunkCount:      chunkCount,
			ChunkPayloadLen: chunkLen,
		}
		if err := s.SendChunk(chk, payload[offset:offset+thisChunk]); err != nil {
			return err
		}

		offset += thisChunk
	}

	return nil
}

func (s Sender) packetSize() (int, error) {
	if uint64(s.PacketSize) > uint64(int(^uint(0)>>1)) {
		return 0, s.ErrBadParam("packet_size exceeds platform limit")
	}
	return int(s.PacketSize), nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func checkedU32(value int) (uint32, bool) {
	if value < 0 || uint64(value) > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(value), true
}
