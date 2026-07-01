// Package protocol implements the wire envelope and codec for the netipc
// protocol. Pure byte-layout encode/decode. No I/O, no transport, no
// allocation on decode. Localhost-only IPC — all multi-byte fields use
// host byte order.
//
// Decoded "View" types borrow the underlying buffer and are valid only while
// that buffer lives. Copy immediately if the data is needed later.
package protocol

import (
	"encoding/binary"
	"errors"
)

// ---------------------------------------------------------------------------
//  Constants
// ---------------------------------------------------------------------------

const (
	MagicMsg   uint32 = 0x4e495043 // "NIPC"
	MagicChunk uint32 = 0x4e43484b // "NCHK"
	Version    uint16 = 1
	HeaderLen  uint16 = 32
	HeaderSize        = 32

	// Message kinds.
	KindRequest  uint16 = 1
	KindResponse uint16 = 2
	KindControl  uint16 = 3

	// Flags.
	FlagBatch uint16 = 0x0001

	// Transport status.
	StatusOK            uint16 = 0
	StatusBadEnvelope   uint16 = 1
	StatusAuthFailed    uint16 = 2
	StatusIncompatible  uint16 = 3
	StatusUnsupported   uint16 = 4
	StatusLimitExceeded uint16 = 5
	StatusInternalError uint16 = 6

	// Control opcodes.
	CodeHello    uint16 = 1
	CodeHelloAck uint16 = 2

	// Method codes.
	MethodIncrement       uint16 = 1
	MethodCgroupsSnapshot uint16 = 2
	MethodStringReverse   uint16 = 3
	MethodCgroupsLookup   uint16 = 4
	MethodAppsLookup      uint16 = 5

	// Profile bits.
	ProfileBaseline    uint32 = 0x01
	ProfileSHMHybrid   uint32 = 0x02
	ProfileSHMFutex    uint32 = 0x04
	ProfileSHMWaitAddr uint32 = 0x08

	// Defaults.
	MaxPayloadDefault uint32 = 1024

	// MaxPayloadCap is the zero-config automatic growth ceiling used only when
	// callers did not configure larger payload budgets explicitly. It is not a
	// protocol hard limit; peers may negotiate larger ceilings from
	// initialization config.
	MaxPayloadCap uint32 = 1024 * 1024

	// Alignment for batch items and typed codec items.
	Alignment = 8

	// Payload sizes.
	helloSize    = 44
	helloAckSize = 48
)

var ne = binary.NativeEndian

// ---------------------------------------------------------------------------
//  Errors
// ---------------------------------------------------------------------------

var (
	ErrTruncated     = errors.New("buffer too short")
	ErrBadMagic      = errors.New("magic value mismatch")
	ErrBadVersion    = errors.New("unsupported version")
	ErrBadHeaderLen  = errors.New("header_len != 32")
	ErrBadKind       = errors.New("unknown message kind")
	ErrBadLayout     = errors.New("unknown layout_version")
	ErrOutOfBounds   = errors.New("offset+length exceeds data")
	ErrMissingNul    = errors.New("string not NUL-terminated")
	ErrBadAlignment  = errors.New("item not 8-byte aligned")
	ErrBadItemCount  = errors.New("item count inconsistent")
	ErrOverflow      = errors.New("builder out of space")
	ErrHandlerFailed = errors.New("dispatch handler failed")
	ErrTimeout       = errors.New("synchronous call deadline expired")
	ErrAborted       = errors.New("synchronous call aborted")
)

// ---------------------------------------------------------------------------
//  Utility
// ---------------------------------------------------------------------------

// Align8 rounds v up to the next multiple of 8.
func Align8(v int) int {
	return (v + 7) &^ 7
}

// ---------------------------------------------------------------------------
//  Outer message header (32 bytes)
// ---------------------------------------------------------------------------

// Header is the outer message header (32 bytes on the wire).
type Header struct {
	Magic           uint32
	Version         uint16
	HeaderLen       uint16
	Kind            uint16
	Flags           uint16
	Code            uint16
	TransportStatus uint16
	PayloadLen      uint32
	ItemCount       uint32
	MessageID       uint64
}

// Encode writes the header into buf. Returns 32 on success, 0 if buf is
// too small.
func (h *Header) Encode(buf []byte) int {
	if len(buf) < HeaderSize {
		return 0
	}
	ne.PutUint32(buf[0:4], h.Magic)
	ne.PutUint16(buf[4:6], h.Version)
	ne.PutUint16(buf[6:8], h.HeaderLen)
	ne.PutUint16(buf[8:10], h.Kind)
	ne.PutUint16(buf[10:12], h.Flags)
	ne.PutUint16(buf[12:14], h.Code)
	ne.PutUint16(buf[14:16], h.TransportStatus)
	ne.PutUint32(buf[16:20], h.PayloadLen)
	ne.PutUint32(buf[20:24], h.ItemCount)
	ne.PutUint64(buf[24:32], h.MessageID)
	return HeaderSize
}

// DecodeHeader decodes an outer message header from buf. Validates magic,
// version, header_len, and kind.
func DecodeHeader(buf []byte) (Header, error) {
	if len(buf) < HeaderSize {
		return Header{}, ErrTruncated
	}
	h := Header{
		Magic:           ne.Uint32(buf[0:4]),
		Version:         ne.Uint16(buf[4:6]),
		HeaderLen:       ne.Uint16(buf[6:8]),
		Kind:            ne.Uint16(buf[8:10]),
		Flags:           ne.Uint16(buf[10:12]),
		Code:            ne.Uint16(buf[12:14]),
		TransportStatus: ne.Uint16(buf[14:16]),
		PayloadLen:      ne.Uint32(buf[16:20]),
		ItemCount:       ne.Uint32(buf[20:24]),
		MessageID:       ne.Uint64(buf[24:32]),
	}
	if h.Magic != MagicMsg {
		return Header{}, ErrBadMagic
	}
	if h.Version != Version {
		return Header{}, ErrBadVersion
	}
	if h.HeaderLen != HeaderLen {
		return Header{}, ErrBadHeaderLen
	}
	if h.Kind < KindRequest || h.Kind > KindControl {
		return Header{}, ErrBadKind
	}
	return h, nil
}

// ---------------------------------------------------------------------------
//  Chunk continuation header (32 bytes)
// ---------------------------------------------------------------------------

// ChunkHeader is a chunk continuation header (32 bytes on the wire).
type ChunkHeader struct {
	Magic           uint32
	Version         uint16
	Flags           uint16
	MessageID       uint64
	TotalMessageLen uint32
	ChunkIndex      uint32
	ChunkCount      uint32
	ChunkPayloadLen uint32
}

// Encode writes the chunk header into buf. Returns 32 on success, 0 if
// buf is too small.
func (c *ChunkHeader) Encode(buf []byte) int {
	if len(buf) < HeaderSize {
		return 0
	}
	ne.PutUint32(buf[0:4], c.Magic)
	ne.PutUint16(buf[4:6], c.Version)
	ne.PutUint16(buf[6:8], c.Flags)
	ne.PutUint64(buf[8:16], c.MessageID)
	ne.PutUint32(buf[16:20], c.TotalMessageLen)
	ne.PutUint32(buf[20:24], c.ChunkIndex)
	ne.PutUint32(buf[24:28], c.ChunkCount)
	ne.PutUint32(buf[28:32], c.ChunkPayloadLen)
	return HeaderSize
}

// DecodeChunkHeader decodes a chunk continuation header from buf.
// Validates magic and version.
func DecodeChunkHeader(buf []byte) (ChunkHeader, error) {
	if len(buf) < HeaderSize {
		return ChunkHeader{}, ErrTruncated
	}
	c := ChunkHeader{
		Magic:           ne.Uint32(buf[0:4]),
		Version:         ne.Uint16(buf[4:6]),
		Flags:           ne.Uint16(buf[6:8]),
		MessageID:       ne.Uint64(buf[8:16]),
		TotalMessageLen: ne.Uint32(buf[16:20]),
		ChunkIndex:      ne.Uint32(buf[20:24]),
		ChunkCount:      ne.Uint32(buf[24:28]),
		ChunkPayloadLen: ne.Uint32(buf[28:32]),
	}
	if c.Magic != MagicChunk {
		return ChunkHeader{}, ErrBadMagic
	}
	if c.Version != Version {
		return ChunkHeader{}, ErrBadVersion
	}
	if c.Flags != 0 {
		return ChunkHeader{}, ErrBadLayout
	}
	if c.ChunkPayloadLen == 0 {
		return ChunkHeader{}, ErrBadLayout
	}
	return c, nil
}

// ---------------------------------------------------------------------------
//  Batch item directory
// ---------------------------------------------------------------------------

// BatchEntry is one entry in a batch item directory (8 bytes on wire).
type BatchEntry struct {
	Offset uint32
	Length uint32
}

// BatchDirEncode encodes entries into buf. Returns total bytes written
// (len(entries) * 8), or 0 if buf is too small.
func BatchDirEncode(entries []BatchEntry, buf []byte) int {
	need := len(entries) * 8
	if len(buf) < need {
		return 0
	}
	for i, e := range entries {
		base := i * 8
		ne.PutUint32(buf[base:base+4], e.Offset)
		ne.PutUint32(buf[base+4:base+8], e.Length)
	}
	return need
}

// BatchDirDecode decodes itemCount directory entries from buf. Validates
// alignment and that each entry falls within packedAreaLen.
func BatchDirDecode(buf []byte, itemCount uint32, packedAreaLen uint32) ([]BatchEntry, error) {
	count, ok := checkedInt(uint64(itemCount))
	if !ok {
		return nil, ErrBadItemCount
	}
	dirSize, ok := checkedMulInt(count, 8)
	if !ok {
		return nil, ErrBadItemCount
	}
	if len(buf) < dirSize {
		return nil, ErrTruncated
	}

	out := make([]BatchEntry, count)
	for i := range count {
		base := i * 8
		off := ne.Uint32(buf[base : base+4])
		length := ne.Uint32(buf[base+4 : base+8])

		if off%uint32(Alignment) != 0 {
			return nil, ErrBadAlignment
		}
		if uint64(off)+uint64(length) > uint64(packedAreaLen) {
			return nil, ErrOutOfBounds
		}
		out[i] = BatchEntry{Offset: off, Length: length}
	}
	return out, nil
}

// BatchDirValidate validates the batch directory without allocating.
// Checks alignment and that each entry falls within packedAreaLen.
func BatchDirValidate(buf []byte, itemCount uint32, packedAreaLen uint32) error {
	dirSize64 := uint64(itemCount) * 8
	if dirSize64 > maxIntUint64() {
		return ErrBadItemCount
	}
	dirSize := int(dirSize64) // #nosec G115 -- bounded by maxIntValue above.
	if len(buf) < dirSize {
		return ErrTruncated
	}
	count := int(itemCount)
	for i := range count {
		base := i * 8
		off := ne.Uint32(buf[base : base+4])
		length := ne.Uint32(buf[base+4 : base+8])
		if off%uint32(Alignment) != 0 {
			return ErrBadAlignment
		}
		if uint64(off)+uint64(length) > uint64(packedAreaLen) {
			return ErrOutOfBounds
		}
	}
	return nil
}

// BatchItemGet extracts a single batch item by index from a complete batch
// payload. Returns the item slice on success.
func BatchItemGet(payload []byte, itemCount uint32, index uint32) ([]byte, error) {
	if index >= itemCount {
		return nil, ErrOutOfBounds
	}

	dirSize64 := uint64(itemCount) * 8
	if dirSize64 > maxIntUint64() {
		return nil, ErrBadItemCount
	}
	dirSize := int(dirSize64) // #nosec G115 -- bounded by maxIntValue above.
	if len(payload) < dirSize {
		return nil, ErrTruncated
	}

	base := int(index) * 8 // #nosec G115 -- index < itemCount and dirSize is bounded above.
	off := ne.Uint32(payload[base : base+4])
	length := ne.Uint32(payload[base+4 : base+8])
	packedAreaStart := dirSize
	packedAreaLen := len(payload) - packedAreaStart

	if off%uint32(Alignment) != 0 {
		return nil, ErrBadAlignment
	}
	if !uint32RangeWithinInt(off, length, packedAreaLen) {
		return nil, ErrOutOfBounds
	}

	start := packedAreaStart + int(off) // #nosec G115 -- bounds check above proves this fits len(payload).
	end := start + int(length)          // #nosec G115 -- bounds check above proves this fits len(payload).
	return payload[start:end], nil
}

// ---------------------------------------------------------------------------
//  Batch builder
// ---------------------------------------------------------------------------

// BatchBuilder builds a batch payload: [directory] [align-pad] [packed items].
type BatchBuilder struct {
	buf        []byte
	itemCount  uint32
	maxItems   uint32
	dirEnd     int // byte offset where directory reservation ends
	dataOffset int // current offset within the packed data area (relative)
}

// Reset reinitializes a batch builder against a caller-provided buffer.
// This lets hot paths reuse stack-allocated builders instead of allocating
// a fresh helper object for every request.
func (b *BatchBuilder) Reset(buf []byte, maxItems uint32) {
	b.buf = buf
	b.itemCount = 0
	b.maxItems = maxItems
	dirSize, ok := checkedInt(uint64(maxItems) * 8)
	if !ok {
		b.dirEnd = maxIntValue()
		b.dataOffset = 0
		return
	}
	dirEnd, ok := checkedAlign8(dirSize)
	if !ok {
		b.dirEnd = maxIntValue()
		b.dataOffset = 0
		return
	}
	b.dirEnd = dirEnd
	b.dataOffset = 0
}

// NewBatchBuilder creates a new batch builder. buf must be large enough for
// maxItems*8 (directory) + packed data.
func NewBatchBuilder(buf []byte, maxItems uint32) *BatchBuilder {
	b := &BatchBuilder{}
	b.Reset(buf, maxItems)
	return b
}

// Add appends an item payload. Handles alignment padding.
func (b *BatchBuilder) Add(item []byte) error {
	maxInt := maxIntValue()
	itemLen32, itemLenFitsU32 := checkedU32Int(len(item))
	// Inline the common case; addSlow preserves the precise error returns for
	// overflow and unusual bounds.
	if b.itemCount < b.maxItems &&
		b.dirEnd >= 0 &&
		b.dataOffset >= 0 &&
		b.dataOffset <= maxInt-7 &&
		itemLenFitsU32 {
		alignedOff := Align8(b.dataOffset)
		alignedOff32, alignedOffFitsU32 := checkedU32Int(alignedOff)
		if alignedOffFitsU32 &&
			alignedOff <= maxInt-b.dirEnd &&
			len(item) <= maxInt-alignedOff {
			absPos := b.dirEnd + alignedOff
			if len(item) <= maxInt-absPos {
				itemEnd := absPos + len(item)
				idx, ok := checkedInt(uint64(b.itemCount) * 8)
				if ok {
					if itemEnd <= len(b.buf) && idx <= len(b.buf)-8 {
						if alignedOff > b.dataOffset {
							clear(b.buf[b.dirEnd+b.dataOffset : b.dirEnd+alignedOff])
						}
						copy(b.buf[absPos:], item)
						ne.PutUint32(b.buf[idx:idx+4], alignedOff32)
						ne.PutUint32(b.buf[idx+4:idx+8], itemLen32)
						b.dataOffset = alignedOff + len(item)
						b.itemCount++
						return nil
					}
				}
			}
		}
	}
	return b.addSlow(item)
}

func (b *BatchBuilder) addSlow(item []byte) error {
	if b.itemCount >= b.maxItems {
		return ErrOverflow
	}
	if b.dirEnd < 0 || b.dataOffset < 0 || b.dataOffset > maxIntValue()-7 {
		return ErrOverflow
	}
	alignedOff := Align8(b.dataOffset)
	alignedOff32, ok := checkedU32Int(alignedOff)
	if !ok {
		return ErrOverflow
	}
	itemLen32, ok := checkedU32Int(len(item))
	if !ok {
		return ErrOverflow
	}
	if alignedOff > maxIntValue()-b.dirEnd {
		return ErrOverflow
	}
	absPos := b.dirEnd + alignedOff
	if len(item) > maxIntValue()-absPos {
		return ErrOverflow
	}
	itemEnd := absPos + len(item)
	if itemEnd > len(b.buf) {
		return ErrOverflow
	}
	if len(item) > maxIntValue()-alignedOff {
		return ErrOverflow
	}

	// Zero alignment padding.
	if alignedOff > b.dataOffset {
		if b.dataOffset > maxIntValue()-b.dirEnd {
			return ErrOverflow
		}
		padStart := b.dirEnd + b.dataOffset
		padEnd := b.dirEnd + alignedOff
		clear(b.buf[padStart:padEnd])
	}

	copy(b.buf[absPos:], item)

	// Write directory entry.
	idx, ok := checkedInt(uint64(b.itemCount) * 8)
	if !ok {
		return ErrOverflow
	}
	if idx > len(b.buf)-8 {
		return ErrOverflow
	}
	ne.PutUint32(b.buf[idx:idx+4], alignedOff32)
	ne.PutUint32(b.buf[idx+4:idx+8], itemLen32)

	b.dataOffset = alignedOff + len(item)
	b.itemCount++
	return nil
}

// Finish finalizes the batch. Returns (totalPayloadSize, itemCount).
// Compacts if fewer items were added than maxItems.
func (b *BatchBuilder) Finish() (int, uint32) {
	count := b.itemCount
	dirSize, ok := checkedInt(uint64(count) * 8)
	if !ok {
		return 0, count
	}
	finalDirAligned, ok := checkedAlign8(dirSize)
	if !ok {
		return 0, count
	}

	if finalDirAligned < b.dirEnd && b.dataOffset > 0 {
		dataEnd, ok := checkedAddInt(b.dirEnd, b.dataOffset)
		if !ok {
			return 0, count
		}
		// Shift packed data left.
		copy(b.buf[finalDirAligned:], b.buf[b.dirEnd:dataEnd])
	}

	alignedData, ok := checkedAlign8(b.dataOffset)
	if !ok {
		return 0, count
	}
	total, ok := checkedAddInt(finalDirAligned, alignedData)
	if !ok {
		return 0, count
	}
	return total, count
}

// ---------------------------------------------------------------------------
//  Hello payload (44 bytes)
// ---------------------------------------------------------------------------

// Hello is the client handshake payload (44 bytes on the wire).
type Hello struct {
	LayoutVersion           uint16
	Flags                   uint16
	SupportedProfiles       uint32
	PreferredProfiles       uint32
	MaxRequestPayloadBytes  uint32
	MaxRequestBatchItems    uint32
	MaxResponsePayloadBytes uint32
	MaxResponseBatchItems   uint32
	AuthToken               uint64
	PacketSize              uint32
}

// Encode writes the Hello payload into buf. Returns 44 on success, 0 if
// buf is too small.
func (h *Hello) Encode(buf []byte) int {
	if len(buf) < helloSize {
		return 0
	}
	ne.PutUint16(buf[0:2], h.LayoutVersion)
	ne.PutUint16(buf[2:4], h.Flags)
	ne.PutUint32(buf[4:8], h.SupportedProfiles)
	ne.PutUint32(buf[8:12], h.PreferredProfiles)
	ne.PutUint32(buf[12:16], h.MaxRequestPayloadBytes)
	ne.PutUint32(buf[16:20], h.MaxRequestBatchItems)
	ne.PutUint32(buf[20:24], h.MaxResponsePayloadBytes)
	ne.PutUint32(buf[24:28], h.MaxResponseBatchItems)
	ne.PutUint32(buf[28:32], 0) // padding
	ne.PutUint64(buf[32:40], h.AuthToken)
	ne.PutUint32(buf[40:44], h.PacketSize)
	return helloSize
}

// DecodeHello decodes a Hello payload from buf. Validates layout_version.
func DecodeHello(buf []byte) (Hello, error) {
	if len(buf) < helloSize {
		return Hello{}, ErrTruncated
	}
	h := Hello{
		LayoutVersion:           ne.Uint16(buf[0:2]),
		Flags:                   ne.Uint16(buf[2:4]),
		SupportedProfiles:       ne.Uint32(buf[4:8]),
		PreferredProfiles:       ne.Uint32(buf[8:12]),
		MaxRequestPayloadBytes:  ne.Uint32(buf[12:16]),
		MaxRequestBatchItems:    ne.Uint32(buf[16:20]),
		MaxResponsePayloadBytes: ne.Uint32(buf[20:24]),
		MaxResponseBatchItems:   ne.Uint32(buf[24:28]),
		// buf[28:32] is reserved padding, must be zero.
		AuthToken:  ne.Uint64(buf[32:40]),
		PacketSize: ne.Uint32(buf[40:44]),
	}
	if h.LayoutVersion != 1 {
		return Hello{}, ErrBadLayout
	}
	// Validate padding bytes 28..32 are zero
	if ne.Uint32(buf[28:32]) != 0 {
		return Hello{}, ErrBadLayout
	}
	return h, nil
}

// ---------------------------------------------------------------------------
//  Hello-ack payload (44 bytes)
// ---------------------------------------------------------------------------

// HelloAck is the server handshake response payload (48 bytes on the wire).
type HelloAck struct {
	LayoutVersion                 uint16
	Flags                         uint16
	ServerSupportedProfiles       uint32
	IntersectionProfiles          uint32
	SelectedProfile               uint32
	AgreedMaxRequestPayloadBytes  uint32
	AgreedMaxRequestBatchItems    uint32
	AgreedMaxResponsePayloadBytes uint32
	AgreedMaxResponseBatchItems   uint32
	AgreedPacketSize              uint32
	SessionID                     uint64
}

// Encode writes the HelloAck payload into buf. Returns 48 on success, 0
// if buf is too small.
func (h *HelloAck) Encode(buf []byte) int {
	if len(buf) < helloAckSize {
		return 0
	}
	ne.PutUint16(buf[0:2], h.LayoutVersion)
	ne.PutUint16(buf[2:4], h.Flags)
	ne.PutUint32(buf[4:8], h.ServerSupportedProfiles)
	ne.PutUint32(buf[8:12], h.IntersectionProfiles)
	ne.PutUint32(buf[12:16], h.SelectedProfile)
	ne.PutUint32(buf[16:20], h.AgreedMaxRequestPayloadBytes)
	ne.PutUint32(buf[20:24], h.AgreedMaxRequestBatchItems)
	ne.PutUint32(buf[24:28], h.AgreedMaxResponsePayloadBytes)
	ne.PutUint32(buf[28:32], h.AgreedMaxResponseBatchItems)
	ne.PutUint32(buf[32:36], h.AgreedPacketSize)
	ne.PutUint32(buf[36:40], 0) // padding
	ne.PutUint64(buf[40:48], h.SessionID)
	return helloAckSize
}

// DecodeHelloAck decodes a HelloAck payload from buf. Validates
// layout_version.
func DecodeHelloAck(buf []byte) (HelloAck, error) {
	if len(buf) < helloAckSize {
		return HelloAck{}, ErrTruncated
	}
	h := HelloAck{
		LayoutVersion:                 ne.Uint16(buf[0:2]),
		Flags:                         ne.Uint16(buf[2:4]),
		ServerSupportedProfiles:       ne.Uint32(buf[4:8]),
		IntersectionProfiles:          ne.Uint32(buf[8:12]),
		SelectedProfile:               ne.Uint32(buf[12:16]),
		AgreedMaxRequestPayloadBytes:  ne.Uint32(buf[16:20]),
		AgreedMaxRequestBatchItems:    ne.Uint32(buf[20:24]),
		AgreedMaxResponsePayloadBytes: ne.Uint32(buf[24:28]),
		AgreedMaxResponseBatchItems:   ne.Uint32(buf[28:32]),
		AgreedPacketSize:              ne.Uint32(buf[32:36]),
		// skip padding at 36:40
		SessionID: ne.Uint64(buf[40:48]),
	}
	if h.LayoutVersion != 1 {
		return HelloAck{}, ErrBadLayout
	}
	if h.Flags != 0 {
		return HelloAck{}, ErrBadLayout
	}
	return h, nil
}
