package protocol

import (
	"testing"
)

// ---------------------------------------------------------------------------
//  DecodeChunkHeader error paths
// ---------------------------------------------------------------------------

func TestDecodeChunkHeaderBadFlags(t *testing.T) {
	// Valid chunk header but with non-zero flags (must be 0)
	c := ChunkHeader{
		Magic:           MagicChunk,
		Version:         Version,
		Flags:           1, // invalid
		MessageID:       1,
		TotalMessageLen: 256,
		ChunkIndex:      0,
		ChunkCount:      3,
		ChunkPayloadLen: 100,
	}
	var buf [HeaderSize]byte
	c.Encode(buf[:])
	_, err := DecodeChunkHeader(buf[:])
	if err != ErrBadLayout {
		t.Fatalf("expected ErrBadLayout for non-zero flags, got %v", err)
	}
}

func TestDecodeChunkHeaderZeroPayloadLen(t *testing.T) {
	// Valid chunk header but with zero chunk_payload_len
	c := ChunkHeader{
		Magic:           MagicChunk,
		Version:         Version,
		Flags:           0,
		MessageID:       1,
		TotalMessageLen: 256,
		ChunkIndex:      0,
		ChunkCount:      3,
		ChunkPayloadLen: 0, // invalid
	}
	var buf [HeaderSize]byte
	c.Encode(buf[:])
	_, err := DecodeChunkHeader(buf[:])
	if err != ErrBadLayout {
		t.Fatalf("expected ErrBadLayout for zero chunk_payload_len, got %v", err)
	}
}

func TestDecodeChunkHeaderBadVersion(t *testing.T) {
	c := ChunkHeader{
		Magic:           MagicChunk,
		Version:         99,
		Flags:           0,
		MessageID:       1,
		TotalMessageLen: 256,
		ChunkIndex:      0,
		ChunkCount:      3,
		ChunkPayloadLen: 100,
	}
	var buf [HeaderSize]byte
	c.Encode(buf[:])
	_, err := DecodeChunkHeader(buf[:])
	if err != ErrBadVersion {
		t.Fatalf("expected ErrBadVersion, got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  DecodeHelloAck error paths
// ---------------------------------------------------------------------------

func TestDecodeHelloAckBadLayout(t *testing.T) {
	h := HelloAck{
		LayoutVersion: 99, // invalid
	}
	var buf [64]byte
	h.Encode(buf[:])
	_, err := DecodeHelloAck(buf[:])
	if err != ErrBadLayout {
		t.Fatalf("expected ErrBadLayout for bad layout_version, got %v", err)
	}
}

func TestDecodeHelloAckBadFlags(t *testing.T) {
	h := HelloAck{
		LayoutVersion: 1,
		Flags:         0x0001, // non-zero flags
	}
	var buf [64]byte
	h.Encode(buf[:])
	_, err := DecodeHelloAck(buf[:])
	if err != ErrBadLayout {
		t.Fatalf("expected ErrBadLayout for non-zero flags, got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  BatchDirValidate
// ---------------------------------------------------------------------------

func TestBatchDirValidateSuccess(t *testing.T) {
	// Build a valid directory with 2 entries
	var buf [16]byte
	ne.PutUint32(buf[0:4], 0)   // offset=0, aligned
	ne.PutUint32(buf[4:8], 10)  // length=10
	ne.PutUint32(buf[8:12], 16) // offset=16, aligned
	ne.PutUint32(buf[12:16], 5) // length=5

	err := BatchDirValidate(buf[:], 2, 100)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestBatchDirValidateTruncated(t *testing.T) {
	err := BatchDirValidate(make([]byte, 4), 2, 100)
	if err != ErrTruncated {
		t.Fatalf("expected ErrTruncated, got %v", err)
	}
}

func TestBatchDirValidateBadAlignment(t *testing.T) {
	var buf [8]byte
	ne.PutUint32(buf[0:4], 3) // offset=3, not 8-byte aligned
	ne.PutUint32(buf[4:8], 5)

	err := BatchDirValidate(buf[:], 1, 100)
	if err != ErrBadAlignment {
		t.Fatalf("expected ErrBadAlignment, got %v", err)
	}
}

func TestBatchDirValidateOutOfBounds(t *testing.T) {
	var buf [8]byte
	ne.PutUint32(buf[0:4], 0)
	ne.PutUint32(buf[4:8], 200) // length=200 exceeds packedAreaLen=100

	err := BatchDirValidate(buf[:], 1, 100)
	if err != ErrOutOfBounds {
		t.Fatalf("expected ErrOutOfBounds, got %v", err)
	}
}

func TestBatchDirValidateZeroItems(t *testing.T) {
	err := BatchDirValidate(nil, 0, 0)
	if err != nil {
		t.Fatalf("expected success for zero items, got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  BatchItemGet error paths
// ---------------------------------------------------------------------------

func TestBatchItemGetBadAlignment(t *testing.T) {
	// Build a batch payload with 1 item, but set misaligned offset
	var buf [64]byte
	ne.PutUint32(buf[0:4], 3) // offset=3, not aligned
	ne.PutUint32(buf[4:8], 5) // length=5

	_, err := BatchItemGet(buf[:], 1, 0)
	if err != ErrBadAlignment {
		t.Fatalf("expected ErrBadAlignment, got %v", err)
	}
}

func TestBatchItemGetDirTruncated(t *testing.T) {
	// 2 items = 16 bytes dir, aligned to 16. But give only 8 bytes total.
	_, err := BatchItemGet(make([]byte, 8), 2, 0)
	if err != ErrTruncated {
		t.Fatalf("expected ErrTruncated, got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  DecodeCgroupsRequest error paths
// ---------------------------------------------------------------------------

func TestDecodeCgroupsRequestBadFlags(t *testing.T) {
	// Valid layout_version but non-zero flags
	var buf [4]byte
	ne.PutUint16(buf[0:2], 1)    // layout_version = 1
	ne.PutUint16(buf[2:4], 0x01) // flags = 1 (invalid)

	_, err := DecodeCgroupsRequest(buf[:])
	if err != ErrBadLayout {
		t.Fatalf("expected ErrBadLayout for non-zero flags, got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  DecodeCgroupsResponse error paths
// ---------------------------------------------------------------------------

func TestDecodeCgroupsResponseBadFlags(t *testing.T) {
	var buf [24]byte
	ne.PutUint16(buf[0:2], 1)    // layout_version = 1
	ne.PutUint16(buf[2:4], 0x01) // flags = 1 (invalid)
	ne.PutUint32(buf[4:8], 0)    // item_count = 0
	ne.PutUint32(buf[8:12], 0)   // systemd_enabled
	ne.PutUint32(buf[12:16], 0)  // reserved
	ne.PutUint64(buf[16:24], 0)  // generation

	_, err := DecodeCgroupsResponse(buf[:])
	if err != ErrBadLayout {
		t.Fatalf("expected ErrBadLayout for non-zero flags, got %v", err)
	}
}

func TestDecodeCgroupsResponseBadReserved(t *testing.T) {
	var buf [24]byte
	ne.PutUint16(buf[0:2], 1)    // layout_version = 1
	ne.PutUint16(buf[2:4], 0)    // flags = 0
	ne.PutUint32(buf[4:8], 0)    // item_count = 0
	ne.PutUint32(buf[8:12], 0)   // systemd_enabled
	ne.PutUint32(buf[12:16], 99) // reserved non-zero (invalid)
	ne.PutUint64(buf[16:24], 0)  // generation

	_, err := DecodeCgroupsResponse(buf[:])
	if err != ErrBadLayout {
		t.Fatalf("expected ErrBadLayout for non-zero reserved field, got %v", err)
	}
}

func TestDecodeCgroupsResponseDirTruncated(t *testing.T) {
	// Declare 1 item but no space for the directory entry
	var buf [24]byte
	ne.PutUint16(buf[0:2], 1)  // layout_version = 1
	ne.PutUint16(buf[2:4], 0)  // flags = 0
	ne.PutUint32(buf[4:8], 1)  // item_count = 1
	ne.PutUint32(buf[8:12], 0) // systemd_enabled
	ne.PutUint32(buf[12:16], 0)
	ne.PutUint64(buf[16:24], 0)

	_, err := DecodeCgroupsResponse(buf[:])
	if err != ErrTruncated {
		t.Fatalf("expected ErrTruncated, got %v", err)
	}
}

func TestDecodeCgroupsResponseDirBadAlignment(t *testing.T) {
	// 1 item: dir at offset 24, packed area starts at 32
	// Set item offset to 3 (misaligned)
	var buf [128]byte
	ne.PutUint16(buf[0:2], 1)  // layout_version
	ne.PutUint16(buf[2:4], 0)  // flags
	ne.PutUint32(buf[4:8], 1)  // item_count = 1
	ne.PutUint32(buf[8:12], 0) // systemd_enabled
	ne.PutUint32(buf[12:16], 0)
	ne.PutUint64(buf[16:24], 0)
	// Directory entry at offset 24
	ne.PutUint32(buf[24:28], 3)  // offset = 3 (misaligned)
	ne.PutUint32(buf[28:32], 40) // length = 40

	_, err := DecodeCgroupsResponse(buf[:])
	if err != ErrBadAlignment {
		t.Fatalf("expected ErrBadAlignment, got %v", err)
	}
}

func TestDecodeCgroupsResponseDirItemTooSmall(t *testing.T) {
	// 1 item with length < cgroupsItemHdr (32)
	var buf [128]byte
	ne.PutUint16(buf[0:2], 1)  // layout_version
	ne.PutUint16(buf[2:4], 0)  // flags
	ne.PutUint32(buf[4:8], 1)  // item_count = 1
	ne.PutUint32(buf[8:12], 0) // systemd_enabled
	ne.PutUint32(buf[12:16], 0)
	ne.PutUint64(buf[16:24], 0)
	// Directory entry at offset 24
	ne.PutUint32(buf[24:28], 0)  // offset = 0
	ne.PutUint32(buf[28:32], 16) // length = 16 (< cgroupsItemHdr=32)

	_, err := DecodeCgroupsResponse(buf[:])
	if err != ErrTruncated {
		t.Fatalf("expected ErrTruncated for item too small, got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  CgroupsResponseView.Item error paths
// ---------------------------------------------------------------------------

func TestCgroupsItemBadLayoutVersion(t *testing.T) {
	// Build a valid snapshot, then corrupt the item's layout_version
	var buf [4096]byte
	b := NewCgroupsBuilder(buf[:], 1, 0, 1)
	if err := b.Add(42, 0, 1, []byte("test"), []byte("/path")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	total := b.Finish()
	payload := buf[:total]

	// Decode successfully first
	view, err := DecodeCgroupsResponse(payload)
	if err != nil {
		t.Fatalf("DecodeCgroupsResponse: %v", err)
	}

	// Find the item in the buffer and corrupt layout_version
	dirBase := cgroupsRespHdr
	itemOff := int(ne.Uint32(payload[dirBase : dirBase+4]))
	packedStart := cgroupsRespHdr + int(view.ItemCount)*cgroupsDirEntry
	itemAbsStart := packedStart + itemOff
	ne.PutUint16(payload[itemAbsStart:itemAbsStart+2], 99) // bad layout_version

	_, err = view.Item(0)
	if err != ErrBadLayout {
		t.Fatalf("expected ErrBadLayout for bad item layout_version, got %v", err)
	}
}

func TestCgroupsItemBadFlags(t *testing.T) {
	var buf [4096]byte
	b := NewCgroupsBuilder(buf[:], 1, 0, 1)
	if err := b.Add(42, 0, 1, []byte("test"), []byte("/path")); err != nil {
		t.Fatalf("Add: %v", err)
	}
	total := b.Finish()
	payload := buf[:total]

	view, err := DecodeCgroupsResponse(payload)
	if err != nil {
		t.Fatalf("DecodeCgroupsResponse: %v", err)
	}

	// Corrupt item flags
	dirBase := cgroupsRespHdr
	itemOff := int(ne.Uint32(payload[dirBase : dirBase+4]))
	packedStart := cgroupsRespHdr + int(view.ItemCount)*cgroupsDirEntry
	itemAbsStart := packedStart + itemOff
	ne.PutUint16(payload[itemAbsStart+2:itemAbsStart+4], 0x01) // bad flags

	_, err = view.Item(0)
	if err != ErrBadLayout {
		t.Fatalf("expected ErrBadLayout for bad item flags, got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  CStringView.GoString
// ---------------------------------------------------------------------------

func TestCStringViewGoString(t *testing.T) {
	data := []byte("hello\x00")
	v := NewCStringView(data, 5)
	gs := v.GoString()
	if gs != `CStringView("hello")` {
		t.Fatalf("GoString = %q, want CStringView(\"hello\")", gs)
	}
}

// ---------------------------------------------------------------------------
//  DispatchCgroupsSnapshot
// ---------------------------------------------------------------------------

func TestDispatchCgroupsSnapshotSuccess(t *testing.T) {
	// Valid request
	var reqBuf [4]byte
	req := CgroupsRequest{LayoutVersion: 1, Flags: 0}
	req.Encode(reqBuf[:])

	resp := make([]byte, 4096)
	n, ok := DispatchCgroupsSnapshot(reqBuf[:], resp, 1,
		func(r *CgroupsRequest, b *CgroupsBuilder) bool {
			if err := b.Add(1, 0, 1, []byte("cg"), []byte("/path")); err != nil {
				return false
			}
			return true
		})
	if !ok {
		t.Fatal("expected success")
	}
	if n == 0 {
		t.Fatal("expected non-zero payload size")
	}

	// Verify the result decodes
	view, err := DecodeCgroupsResponse(resp[:n])
	if err != nil {
		t.Fatalf("DecodeCgroupsResponse: %v", err)
	}
	if view.ItemCount != 1 {
		t.Fatalf("expected 1 item, got %d", view.ItemCount)
	}
}

func TestDispatchCgroupsSnapshotBadRequest(t *testing.T) {
	// Truncated request
	_, ok := DispatchCgroupsSnapshot([]byte{0}, make([]byte, 4096), 1,
		func(r *CgroupsRequest, b *CgroupsBuilder) bool {
			return true
		})
	if ok {
		t.Fatal("expected failure for bad request")
	}
}

func TestDispatchCgroupsSnapshotHandlerFails(t *testing.T) {
	var reqBuf [4]byte
	req := CgroupsRequest{LayoutVersion: 1, Flags: 0}
	req.Encode(reqBuf[:])

	_, ok := DispatchCgroupsSnapshot(reqBuf[:], make([]byte, 4096), 1,
		func(r *CgroupsRequest, b *CgroupsBuilder) bool {
			return false
		})
	if ok {
		t.Fatal("expected failure when handler returns false")
	}
}

func TestDispatchCgroupsSnapshotEmptyResult(t *testing.T) {
	var reqBuf [4]byte
	req := CgroupsRequest{LayoutVersion: 1, Flags: 0}
	req.Encode(reqBuf[:])

	// Handler succeeds but adds no items - Finish returns cgroupsRespHdr (24) which is > 0
	n, ok := DispatchCgroupsSnapshot(reqBuf[:], make([]byte, 4096), 0,
		func(r *CgroupsRequest, b *CgroupsBuilder) bool {
			return true
		})
	if !ok {
		t.Fatal("expected success for empty snapshot")
	}
	if n != cgroupsRespHdr {
		t.Fatalf("expected %d bytes, got %d", cgroupsRespHdr, n)
	}
}

// ---------------------------------------------------------------------------
//  IncrementEncode / IncrementDecode
// ---------------------------------------------------------------------------

func TestIncrementEncodeSuccess(t *testing.T) {
	var buf [8]byte
	n := IncrementEncode(0xDEADBEEFCAFEBABE, buf[:])
	if n != 8 {
		t.Fatalf("expected 8, got %d", n)
	}
	val, err := IncrementDecode(buf[:])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if val != 0xDEADBEEFCAFEBABE {
		t.Fatalf("expected 0xDEADBEEFCAFEBABE, got 0x%x", val)
	}
}

func TestIncrementEncodeTooSmall(t *testing.T) {
	var buf [4]byte
	n := IncrementEncode(42, buf[:])
	if n != 0 {
		t.Fatalf("expected 0 for too-small buffer, got %d", n)
	}
}

func TestIncrementDecodeTruncated(t *testing.T) {
	_, err := IncrementDecode(make([]byte, 3))
	if err != ErrTruncated {
		t.Fatalf("expected ErrTruncated, got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  DispatchIncrement
// ---------------------------------------------------------------------------

func TestDispatchIncrementSuccess(t *testing.T) {
	var reqBuf [8]byte
	IncrementEncode(100, reqBuf[:])

	var respBuf [8]byte
	n, ok := DispatchIncrement(reqBuf[:], respBuf[:], func(v uint64) (uint64, bool) {
		return v + 1, true
	})
	if !ok {
		t.Fatal("expected success")
	}
	if n != 8 {
		t.Fatalf("expected 8, got %d", n)
	}
	val, err := IncrementDecode(respBuf[:])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if val != 101 {
		t.Fatalf("expected 101, got %d", val)
	}
}

func TestDispatchIncrementBadRequest(t *testing.T) {
	_, ok := DispatchIncrement([]byte{0, 1}, make([]byte, 8), func(v uint64) (uint64, bool) {
		return v, true
	})
	if ok {
		t.Fatal("expected failure for truncated request")
	}
}

func TestDispatchIncrementHandlerFails(t *testing.T) {
	var reqBuf [8]byte
	IncrementEncode(42, reqBuf[:])

	_, ok := DispatchIncrement(reqBuf[:], make([]byte, 8), func(v uint64) (uint64, bool) {
		return 0, false
	})
	if ok {
		t.Fatal("expected failure when handler returns false")
	}
}

func TestDispatchIncrementRespTooSmall(t *testing.T) {
	var reqBuf [8]byte
	IncrementEncode(42, reqBuf[:])

	n, ok := DispatchIncrement(reqBuf[:], make([]byte, 2), func(v uint64) (uint64, bool) {
		return 42, true
	})
	// IncrementEncode returns 0 for too-small buffer, so n=0, ok = (0>0) = false
	if ok {
		t.Fatal("expected failure for too-small response buffer")
	}
	if n != 0 {
		t.Fatalf("expected n=0, got %d", n)
	}
}

// ---------------------------------------------------------------------------
//  StringReverseEncode / StringReverseDecode
// ---------------------------------------------------------------------------

func TestStringReverseRoundtrip(t *testing.T) {
	s := "hello world"
	buf := make([]byte, StringReverseHdrSize+len(s)+1)
	n := StringReverseEncode(s, buf)
	if n != len(buf) {
		t.Fatalf("expected %d, got %d", len(buf), n)
	}

	view, err := StringReverseDecode(buf)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Str != s {
		t.Fatalf("expected %q, got %q", s, view.Str)
	}
	if view.StrLen != uint32(len(s)) {
		t.Fatalf("expected len=%d, got %d", len(s), view.StrLen)
	}
}

func TestStringReverseEncodeEmpty(t *testing.T) {
	buf := make([]byte, StringReverseHdrSize+1) // 8 + 0 + 1 NUL
	n := StringReverseEncode("", buf)
	if n != StringReverseHdrSize+1 {
		t.Fatalf("expected %d, got %d", StringReverseHdrSize+1, n)
	}

	view, err := StringReverseDecode(buf[:n])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Str != "" {
		t.Fatalf("expected empty, got %q", view.Str)
	}
}

func TestStringReverseEncodeTooSmall(t *testing.T) {
	n := StringReverseEncode("hello", make([]byte, 5))
	if n != 0 {
		t.Fatalf("expected 0 for too-small buffer, got %d", n)
	}
}

func TestStringReverseDecodeTruncated(t *testing.T) {
	_, err := StringReverseDecode(make([]byte, 4))
	if err != ErrTruncated {
		t.Fatalf("expected ErrTruncated, got %v", err)
	}
}

func TestStringReverseDecodeOutOfBounds(t *testing.T) {
	var buf [8]byte
	ne.PutUint32(buf[0:4], 8)  // str_offset
	ne.PutUint32(buf[4:8], 99) // str_length (overflows)

	_, err := StringReverseDecode(buf[:])
	if err != ErrOutOfBounds {
		t.Fatalf("expected ErrOutOfBounds, got %v", err)
	}
}

func TestStringReverseDecodeMissingNul(t *testing.T) {
	// Build payload where NUL terminator is wrong
	buf := make([]byte, StringReverseHdrSize+6)
	ne.PutUint32(buf[0:4], uint32(StringReverseHdrSize)) // str_offset
	ne.PutUint32(buf[4:8], 5)                            // str_length
	copy(buf[8:13], "hello")
	buf[13] = 'X' // should be 0

	_, err := StringReverseDecode(buf)
	if err != ErrMissingNul {
		t.Fatalf("expected ErrMissingNul, got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  DispatchStringReverse
// ---------------------------------------------------------------------------

func TestDispatchStringReverseSuccess(t *testing.T) {
	input := "hello"
	reqBuf := make([]byte, StringReverseHdrSize+len(input)+1)
	StringReverseEncode(input, reqBuf)

	respBuf := make([]byte, 128)
	n, ok := DispatchStringReverse(reqBuf, respBuf, func(s string) (string, bool) {
		// Reverse the string
		b := []byte(s)
		for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
			b[i], b[j] = b[j], b[i]
		}
		return string(b), true
	})
	if !ok {
		t.Fatal("expected success")
	}
	if n == 0 {
		t.Fatal("expected non-zero response size")
	}

	view, err := StringReverseDecode(respBuf[:n])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Str != "olleh" {
		t.Fatalf("expected 'olleh', got %q", view.Str)
	}
}

func TestDispatchStringReverseBadRequest(t *testing.T) {
	_, ok := DispatchStringReverse([]byte{0}, make([]byte, 128), func(s string) (string, bool) {
		return s, true
	})
	if ok {
		t.Fatal("expected failure for bad request")
	}
}

func TestDispatchStringReverseHandlerFails(t *testing.T) {
	input := "test"
	reqBuf := make([]byte, StringReverseHdrSize+len(input)+1)
	StringReverseEncode(input, reqBuf)

	_, ok := DispatchStringReverse(reqBuf, make([]byte, 128), func(s string) (string, bool) {
		return "", false
	})
	if ok {
		t.Fatal("expected failure when handler returns false")
	}
}

func TestDispatchStringReverseRespTooSmall(t *testing.T) {
	input := "hello"
	reqBuf := make([]byte, StringReverseHdrSize+len(input)+1)
	StringReverseEncode(input, reqBuf)

	// Response buffer too small for result
	n, ok := DispatchStringReverse(reqBuf, make([]byte, 2), func(s string) (string, bool) {
		return "very long response string", true
	})
	if ok {
		t.Fatal("expected failure for too-small response buffer")
	}
	if n != 0 {
		t.Fatalf("expected n=0, got %d", n)
	}
}

// ---------------------------------------------------------------------------
//  CgroupsBuilder edge cases
// ---------------------------------------------------------------------------

func TestCgroupsBuilderOverflow(t *testing.T) {
	// Buffer too small for the item
	var buf [64]byte
	b := NewCgroupsBuilder(buf[:], 1, 0, 1)
	// name + path would overflow the small buffer
	err := b.Add(1, 0, 1,
		[]byte("this-name-is-quite-long-to-overflow"),
		[]byte("/this/path/is/also/long/enough/to/overflow"))
	if err != ErrOverflow {
		t.Fatalf("expected ErrOverflow, got %v", err)
	}
}

func TestCgroupsBuilderMaxItemsExceeded(t *testing.T) {
	var buf [4096]byte
	b := NewCgroupsBuilder(buf[:], 1, 0, 1)
	// First item succeeds
	if err := b.Add(1, 0, 1, []byte("a"), []byte("/b")); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	// Second item exceeds maxItems
	err := b.Add(2, 0, 1, []byte("c"), []byte("/d"))
	if err != ErrOverflow {
		t.Fatalf("expected ErrOverflow, got %v", err)
	}
}

func TestCgroupsBuilderFinishCompaction(t *testing.T) {
	// Reserve space for 4 items but only add 2 — tests compaction path
	var buf [4096]byte
	b := NewCgroupsBuilder(buf[:], 4, 1, 42)
	if err := b.Add(1, 0, 1, []byte("name1"), []byte("/path1")); err != nil {
		t.Fatalf("Add 1: %v", err)
	}
	if err := b.Add(2, 0, 1, []byte("name2"), []byte("/path2")); err != nil {
		t.Fatalf("Add 2: %v", err)
	}
	total := b.Finish()

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.ItemCount != 2 {
		t.Fatalf("expected 2 items, got %d", view.ItemCount)
	}
	if view.SystemdEnabled != 1 {
		t.Fatalf("expected systemd_enabled=1, got %d", view.SystemdEnabled)
	}
	if view.Generation != 42 {
		t.Fatalf("expected generation=42, got %d", view.Generation)
	}

	// Verify both items decode correctly
	item0, err := view.Item(0)
	if err != nil {
		t.Fatalf("item 0: %v", err)
	}
	if item0.Name.String() != "name1" || item0.Path.String() != "/path1" {
		t.Fatalf("item 0 mismatch: name=%q path=%q", item0.Name.String(), item0.Path.String())
	}

	item1, err := view.Item(1)
	if err != nil {
		t.Fatalf("item 1: %v", err)
	}
	if item1.Name.String() != "name2" || item1.Path.String() != "/path2" {
		t.Fatalf("item 1 mismatch: name=%q path=%q", item1.Name.String(), item1.Path.String())
	}
}

// ---------------------------------------------------------------------------
//  BatchBuilder edge cases
// ---------------------------------------------------------------------------

func TestBatchBuilderCompactionWide(t *testing.T) {
	// Reserve 8 slots, use only 2 (wider gap than existing test)
	var buf [4096]byte
	bb := NewBatchBuilder(buf[:], 8)
	if err := bb.Add([]byte{1, 2, 3}); err != nil {
		t.Fatalf("Add 1: %v", err)
	}
	if err := bb.Add([]byte{4, 5}); err != nil {
		t.Fatalf("Add 2: %v", err)
	}
	total, count := bb.Finish()
	if count != 2 {
		t.Fatalf("expected count=2, got %d", count)
	}

	// Verify items are accessible
	item0, err := BatchItemGet(buf[:total], count, 0)
	if err != nil {
		t.Fatalf("get item 0: %v", err)
	}
	if len(item0) != 3 || item0[0] != 1 {
		t.Fatalf("item 0 mismatch: %v", item0)
	}

	item1, err := BatchItemGet(buf[:total], count, 1)
	if err != nil {
		t.Fatalf("get item 1: %v", err)
	}
	if len(item1) != 2 || item1[0] != 4 {
		t.Fatalf("item 1 mismatch: %v", item1)
	}
}

func TestBatchBuilderOverflowMaxItemsSingle(t *testing.T) {
	var buf [4096]byte
	bb := NewBatchBuilder(buf[:], 1)
	if err := bb.Add([]byte{1}); err != nil {
		t.Fatalf("first Add: %v", err)
	}
	err := bb.Add([]byte{2})
	if err != ErrOverflow {
		t.Fatalf("expected ErrOverflow, got %v", err)
	}
}

func TestBatchBuilderOverflowBufferFull(t *testing.T) {
	// Tiny buffer: dir for 1 item = 8 bytes aligned, so 8 bytes.
	// Total buf = 16 bytes, leaving 8 for data.
	var buf [16]byte
	bb := NewBatchBuilder(buf[:], 1)
	err := bb.Add(make([]byte, 100)) // too large
	if err != ErrOverflow {
		t.Fatalf("expected ErrOverflow, got %v", err)
	}
}

func TestBatchBuilderFinishNoCompaction(t *testing.T) {
	// Use exactly the reserved number of slots - no compaction needed
	var buf [4096]byte
	bb := NewBatchBuilder(buf[:], 2)
	if err := bb.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8}); err != nil {
		t.Fatalf("Add 1: %v", err)
	}
	if err := bb.Add([]byte{10, 20}); err != nil {
		t.Fatalf("Add 2: %v", err)
	}
	total, count := bb.Finish()
	if count != 2 {
		t.Fatalf("expected count=2, got %d", count)
	}

	// Verify access
	for i := uint32(0); i < count; i++ {
		_, err := BatchItemGet(buf[:total], count, i)
		if err != nil {
			t.Fatalf("item %d: %v", i, err)
		}
	}
}

// ---------------------------------------------------------------------------
//  DecodeHello padding validation
// ---------------------------------------------------------------------------

func TestDecodeHelloBadPadding(t *testing.T) {
	h := Hello{
		LayoutVersion:           1,
		SupportedProfiles:       ProfileBaseline,
		MaxRequestPayloadBytes:  1024,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: 1024,
		MaxResponseBatchItems:   1,
		AuthToken:               42,
		PacketSize:              65536,
	}
	var buf [64]byte
	h.Encode(buf[:])

	// Corrupt padding bytes at offset 28..32
	ne.PutUint32(buf[28:32], 0xFFFF)

	_, err := DecodeHello(buf[:44])
	if err != ErrBadLayout {
		t.Fatalf("expected ErrBadLayout for bad padding, got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  CgroupsBuilder utility functions
// ---------------------------------------------------------------------------

func TestCgroupsBuilderSetHeader(t *testing.T) {
	var buf [4096]byte
	b := NewCgroupsBuilder(buf[:], 10, 0, 0)

	// Initial values should be zero
	if b.systemdEnabled != 0 {
		t.Fatalf("initial systemdEnabled should be 0, got %d", b.systemdEnabled)
	}
	if b.generation != 0 {
		t.Fatalf("initial generation should be 0, got %d", b.generation)
	}

	// Set header values
	b.SetHeader(1, 12345)

	if b.systemdEnabled != 1 {
		t.Fatalf("SetHeader systemdEnabled should be 1, got %d", b.systemdEnabled)
	}
	if b.generation != 12345 {
		t.Fatalf("SetHeader generation should be 12345, got %d", b.generation)
	}
}

func TestEstimateCgroupsMaxItems(t *testing.T) {
	// Buffer too small - should return 0
	maxItems := EstimateCgroupsMaxItems(cgroupsRespHdr)
	if maxItems != 0 {
		t.Fatalf("EstimateCgroupsMaxItems(%d) should be 0, got %d", cgroupsRespHdr, maxItems)
	}

	// Small buffer - should return 0
	maxItems = EstimateCgroupsMaxItems(cgroupsRespHdr + 10)
	if maxItems != 0 {
		t.Fatalf("EstimateCgroupsMaxItems(small) should be 0, got %d", maxItems)
	}

	// Reasonable buffer - should return positive value
	maxItems = EstimateCgroupsMaxItems(4096)
	if maxItems == 0 {
		t.Fatalf("EstimateCgroupsMaxItems(4096) should be > 0, got 0")
	}

	// Larger buffer - should return larger value
	maxItemsLarge := EstimateCgroupsMaxItems(65536)
	if maxItemsLarge <= maxItems {
		t.Fatalf("EstimateCgroupsMaxItems(65536)=%d should be > EstimateCgroupsMaxItems(4096)=%d", maxItemsLarge, maxItems)
	}
}
