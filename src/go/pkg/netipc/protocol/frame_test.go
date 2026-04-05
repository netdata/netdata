package protocol

import (
	"bytes"
	"testing"
)

// ---------------------------------------------------------------------------
//  Utility tests
// ---------------------------------------------------------------------------

func TestAlign8(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, 0},
		{1, 8},
		{7, 8},
		{8, 8},
		{9, 16},
		{15, 16},
		{16, 16},
		{17, 24},
	}
	for _, tc := range cases {
		if got := Align8(tc.in); got != tc.want {
			t.Errorf("Align8(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
//  Outer message header tests
// ---------------------------------------------------------------------------

func TestHeaderRoundtrip(t *testing.T) {
	h := Header{
		Magic:           MagicMsg,
		Version:         Version,
		HeaderLen:       HeaderLen,
		Kind:            KindRequest,
		Flags:           FlagBatch,
		Code:            MethodCgroupsSnapshot,
		TransportStatus: StatusOK,
		PayloadLen:      12345,
		ItemCount:       42,
		MessageID:       0xDEADBEEFCAFEBABE,
	}

	var buf [64]byte
	n := h.Encode(buf[:])
	if n != 32 {
		t.Fatalf("Encode returned %d, want 32", n)
	}

	out, err := DecodeHeader(buf[:n])
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if out != h {
		t.Fatalf("roundtrip mismatch:\ngot:  %+v\nwant: %+v", out, h)
	}
}

func TestHeaderEncodeTooSmall(t *testing.T) {
	h := Header{}
	var buf [16]byte
	if n := h.Encode(buf[:]); n != 0 {
		t.Fatalf("Encode returned %d, want 0", n)
	}
}

func TestHeaderDecodeTruncated(t *testing.T) {
	buf := make([]byte, 31)
	_, err := DecodeHeader(buf)
	if err != ErrTruncated {
		t.Fatalf("got %v, want ErrTruncated", err)
	}
}

func TestHeaderDecodeBadMagic(t *testing.T) {
	h := Header{
		Magic:     0x12345678,
		Version:   Version,
		HeaderLen: HeaderLen,
		Kind:      KindRequest,
	}
	var buf [32]byte
	h.Encode(buf[:])
	_, err := DecodeHeader(buf[:])
	if err != ErrBadMagic {
		t.Fatalf("got %v, want ErrBadMagic", err)
	}
}

func TestHeaderDecodeBadVersion(t *testing.T) {
	h := Header{
		Magic:     MagicMsg,
		Version:   99,
		HeaderLen: HeaderLen,
		Kind:      KindRequest,
	}
	var buf [32]byte
	h.Encode(buf[:])
	_, err := DecodeHeader(buf[:])
	if err != ErrBadVersion {
		t.Fatalf("got %v, want ErrBadVersion", err)
	}
}

func TestHeaderDecodeBadHeaderLen(t *testing.T) {
	h := Header{
		Magic:     MagicMsg,
		Version:   Version,
		HeaderLen: 64,
		Kind:      KindRequest,
	}
	var buf [32]byte
	h.Encode(buf[:])
	_, err := DecodeHeader(buf[:])
	if err != ErrBadHeaderLen {
		t.Fatalf("got %v, want ErrBadHeaderLen", err)
	}
}

func TestHeaderDecodeBadKind(t *testing.T) {
	// kind = 0
	h := Header{
		Magic:     MagicMsg,
		Version:   Version,
		HeaderLen: HeaderLen,
		Kind:      0,
	}
	var buf [32]byte
	h.Encode(buf[:])
	_, err := DecodeHeader(buf[:])
	if err != ErrBadKind {
		t.Fatalf("kind=0: got %v, want ErrBadKind", err)
	}

	// kind = 4
	h.Kind = 4
	h.Encode(buf[:])
	_, err = DecodeHeader(buf[:])
	if err != ErrBadKind {
		t.Fatalf("kind=4: got %v, want ErrBadKind", err)
	}
}

func TestHeaderAllKinds(t *testing.T) {
	for k := KindRequest; k <= KindControl; k++ {
		h := Header{
			Magic:     MagicMsg,
			Version:   Version,
			HeaderLen: HeaderLen,
			Kind:      k,
		}
		var buf [32]byte
		h.Encode(buf[:])
		out, err := DecodeHeader(buf[:])
		if err != nil {
			t.Fatalf("kind=%d: %v", k, err)
		}
		if out.Kind != k {
			t.Fatalf("kind=%d: got %d", k, out.Kind)
		}
	}
}

func TestHeaderWireBytes(t *testing.T) {
	h := Header{
		Magic:           MagicMsg,
		Version:         Version,
		HeaderLen:       HeaderLen,
		Kind:            KindRequest,
		Flags:           0,
		Code:            MethodCgroupsSnapshot,
		TransportStatus: StatusOK,
		PayloadLen:      4,
		ItemCount:       1,
		MessageID:       1,
	}

	var buf [32]byte
	h.Encode(buf[:])

	// magic = 0x4e495043 LE: 43 50 49 4e
	if !bytes.Equal(buf[0:4], []byte{0x43, 0x50, 0x49, 0x4e}) {
		t.Errorf("magic bytes: %x", buf[0:4])
	}
	// version = 1 LE: 01 00
	if !bytes.Equal(buf[4:6], []byte{0x01, 0x00}) {
		t.Errorf("version bytes: %x", buf[4:6])
	}
	// header_len = 32 LE: 20 00
	if !bytes.Equal(buf[6:8], []byte{0x20, 0x00}) {
		t.Errorf("header_len bytes: %x", buf[6:8])
	}
	// kind = 1 LE: 01 00
	if !bytes.Equal(buf[8:10], []byte{0x01, 0x00}) {
		t.Errorf("kind bytes: %x", buf[8:10])
	}
	// code = 2 LE: 02 00
	if !bytes.Equal(buf[12:14], []byte{0x02, 0x00}) {
		t.Errorf("code bytes: %x", buf[12:14])
	}
}

// ---------------------------------------------------------------------------
//  Chunk continuation header tests
// ---------------------------------------------------------------------------

func TestChunkHeaderRoundtrip(t *testing.T) {
	c := ChunkHeader{
		Magic:           MagicChunk,
		Version:         Version,
		Flags:           0,
		MessageID:       0x1234567890ABCDEF,
		TotalMessageLen: 100000,
		ChunkIndex:      3,
		ChunkCount:      10,
		ChunkPayloadLen: 8192,
	}

	var buf [64]byte
	n := c.Encode(buf[:])
	if n != 32 {
		t.Fatalf("Encode returned %d, want 32", n)
	}

	out, err := DecodeChunkHeader(buf[:n])
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if out != c {
		t.Fatalf("roundtrip mismatch:\ngot:  %+v\nwant: %+v", out, c)
	}
}

func TestChunkDecodeTruncated(t *testing.T) {
	buf := make([]byte, 31)
	_, err := DecodeChunkHeader(buf)
	if err != ErrTruncated {
		t.Fatalf("got %v, want ErrTruncated", err)
	}
}

func TestChunkDecodeBadMagic(t *testing.T) {
	c := ChunkHeader{
		Magic:   MagicMsg, // wrong magic for chunk
		Version: Version,
	}
	var buf [32]byte
	c.Encode(buf[:])
	_, err := DecodeChunkHeader(buf[:])
	if err != ErrBadMagic {
		t.Fatalf("got %v, want ErrBadMagic", err)
	}
}

func TestChunkDecodeBadVersion(t *testing.T) {
	c := ChunkHeader{
		Magic:   MagicChunk,
		Version: 2,
	}
	var buf [32]byte
	c.Encode(buf[:])
	_, err := DecodeChunkHeader(buf[:])
	if err != ErrBadVersion {
		t.Fatalf("got %v, want ErrBadVersion", err)
	}
}

func TestChunkEncodeTooSmall(t *testing.T) {
	c := ChunkHeader{}
	var buf [16]byte
	if n := c.Encode(buf[:]); n != 0 {
		t.Fatalf("Encode returned %d, want 0", n)
	}
}

func TestChunkWireBytes(t *testing.T) {
	c := ChunkHeader{
		Magic:           MagicChunk,
		Version:         Version,
		Flags:           0,
		MessageID:       1,
		TotalMessageLen: 256,
		ChunkIndex:      1,
		ChunkCount:      3,
		ChunkPayloadLen: 100,
	}

	var buf [32]byte
	c.Encode(buf[:])

	// magic = 0x4e43484b LE: 4b 48 43 4e
	if !bytes.Equal(buf[0:4], []byte{0x4b, 0x48, 0x43, 0x4e}) {
		t.Errorf("magic bytes: %x", buf[0:4])
	}
}

// ---------------------------------------------------------------------------
//  Batch item directory tests
// ---------------------------------------------------------------------------

func TestBatchDirRoundtrip(t *testing.T) {
	entries := []BatchEntry{
		{Offset: 0, Length: 100},
		{Offset: 104, Length: 200},
		{Offset: 304, Length: 50},
	}

	buf := make([]byte, 24)
	n := BatchDirEncode(entries, buf)
	if n != 24 {
		t.Fatalf("BatchDirEncode returned %d, want 24", n)
	}

	out, err := BatchDirDecode(buf, 3, 1000)
	if err != nil {
		t.Fatalf("BatchDirDecode error: %v", err)
	}
	for i, e := range entries {
		if out[i] != e {
			t.Errorf("entry[%d]: got %+v, want %+v", i, out[i], e)
		}
	}
}

func TestBatchDirEncodeTooSmall(t *testing.T) {
	entries := []BatchEntry{{Offset: 0, Length: 10}}
	buf := make([]byte, 4)
	if n := BatchDirEncode(entries, buf); n != 0 {
		t.Fatalf("got %d, want 0", n)
	}
}

func TestBatchDirDecodeTruncated(t *testing.T) {
	buf := make([]byte, 12)
	_, err := BatchDirDecode(buf, 2, 1000)
	if err != ErrTruncated {
		t.Fatalf("got %v, want ErrTruncated", err)
	}
}

func TestBatchDirDecodeBadAlignment(t *testing.T) {
	buf := make([]byte, 8)
	ne.PutUint32(buf[0:4], 3) // offset not aligned to 8
	ne.PutUint32(buf[4:8], 10)
	_, err := BatchDirDecode(buf, 1, 100)
	if err != ErrBadAlignment {
		t.Fatalf("got %v, want ErrBadAlignment", err)
	}
}

func TestBatchDirDecodeOutOfBounds(t *testing.T) {
	buf := make([]byte, 8)
	ne.PutUint32(buf[0:4], 0)
	ne.PutUint32(buf[4:8], 200) // exceeds packed area
	_, err := BatchDirDecode(buf, 1, 100)
	if err != ErrOutOfBounds {
		t.Fatalf("got %v, want ErrOutOfBounds", err)
	}
}

func TestBatchItemGetBasic(t *testing.T) {
	// Build a batch with 2 items using the builder.
	buf := make([]byte, 1024)
	b := NewBatchBuilder(buf, 2)

	item0 := []byte("hello")
	item1 := []byte("world!!!")

	if err := b.Add(item0); err != nil {
		t.Fatal(err)
	}
	if err := b.Add(item1); err != nil {
		t.Fatal(err)
	}

	total, count := b.Finish()
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}

	// Extract items.
	got0, err := BatchItemGet(buf[:total], count, 0)
	if err != nil {
		t.Fatalf("item 0: %v", err)
	}
	if !bytes.Equal(got0, item0) {
		t.Fatalf("item 0: got %q, want %q", got0, item0)
	}

	got1, err := BatchItemGet(buf[:total], count, 1)
	if err != nil {
		t.Fatalf("item 1: %v", err)
	}
	if !bytes.Equal(got1, item1) {
		t.Fatalf("item 1: got %q, want %q", got1, item1)
	}
}

func TestBatchItemGetOutOfBounds(t *testing.T) {
	buf := make([]byte, 16)
	_, err := BatchItemGet(buf, 1, 1) // index >= count
	if err != ErrOutOfBounds {
		t.Fatalf("got %v, want ErrOutOfBounds", err)
	}
}

func TestBatchItemGetTruncated(t *testing.T) {
	buf := make([]byte, 4) // too small for even 1 directory entry aligned
	_, err := BatchItemGet(buf, 1, 0)
	if err != ErrTruncated {
		t.Fatalf("got %v, want ErrTruncated", err)
	}
}

// ---------------------------------------------------------------------------
//  Batch builder tests
// ---------------------------------------------------------------------------

func TestBatchBuilderOverflowMaxItems(t *testing.T) {
	buf := make([]byte, 1024)
	b := NewBatchBuilder(buf, 1)
	if err := b.Add([]byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := b.Add([]byte("two")); err != ErrOverflow {
		t.Fatalf("got %v, want ErrOverflow", err)
	}
}

func TestBatchBuilderOverflowBuffer(t *testing.T) {
	buf := make([]byte, 16) // dir = 8 bytes, 8 bytes data
	b := NewBatchBuilder(buf, 1)
	if err := b.Add(make([]byte, 100)); err != ErrOverflow {
		t.Fatalf("got %v, want ErrOverflow", err)
	}
}

func TestBatchBuilderEmpty(t *testing.T) {
	buf := make([]byte, 64)
	b := NewBatchBuilder(buf, 5)
	total, count := b.Finish()
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
	if total != 0 {
		t.Fatalf("total = %d, want 0", total)
	}
}

func TestBatchBuilderCompaction(t *testing.T) {
	// Reserve space for 10 items, only add 1.
	buf := make([]byte, 1024)
	b := NewBatchBuilder(buf, 10)
	if err := b.Add([]byte("compact")); err != nil {
		t.Fatal(err)
	}
	total, count := b.Finish()
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}

	// Verify the item can be extracted.
	got, err := BatchItemGet(buf[:total], count, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("compact")) {
		t.Fatalf("got %q, want %q", got, "compact")
	}
}

// ---------------------------------------------------------------------------
//  Hello payload tests
// ---------------------------------------------------------------------------

func TestHelloRoundtrip(t *testing.T) {
	h := Hello{
		LayoutVersion:           1,
		Flags:                   0,
		SupportedProfiles:       ProfileBaseline | ProfileSHMFutex,
		PreferredProfiles:       ProfileSHMFutex,
		MaxRequestPayloadBytes:  4096,
		MaxRequestBatchItems:    100,
		MaxResponsePayloadBytes: 1048576,
		MaxResponseBatchItems:   1,
		AuthToken:               0xAABBCCDDEEFF0011,
		PacketSize:              65536,
	}

	var buf [64]byte
	n := h.Encode(buf[:])
	if n != 44 {
		t.Fatalf("Encode returned %d, want 44", n)
	}

	out, err := DecodeHello(buf[:n])
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if out != h {
		t.Fatalf("roundtrip mismatch:\ngot:  %+v\nwant: %+v", out, h)
	}
}

func TestHelloEncodeTooSmall(t *testing.T) {
	h := Hello{LayoutVersion: 1}
	var buf [20]byte
	if n := h.Encode(buf[:]); n != 0 {
		t.Fatalf("Encode returned %d, want 0", n)
	}
}

func TestHelloDecodeTruncated(t *testing.T) {
	buf := make([]byte, 43)
	_, err := DecodeHello(buf)
	if err != ErrTruncated {
		t.Fatalf("got %v, want ErrTruncated", err)
	}
}

func TestHelloDecodeBadLayout(t *testing.T) {
	h := Hello{LayoutVersion: 2}
	var buf [44]byte
	h.Encode(buf[:])
	_, err := DecodeHello(buf[:])
	if err != ErrBadLayout {
		t.Fatalf("got %v, want ErrBadLayout", err)
	}
}

func TestHelloPaddingIsZero(t *testing.T) {
	h := Hello{
		LayoutVersion:         1,
		MaxResponseBatchItems: 0xFFFFFFFF,
		AuthToken:             0xAAAAAAAAAAAAAAAA,
	}
	var buf [44]byte
	h.Encode(buf[:])

	// Padding at offset 28..32 must be zero.
	if !bytes.Equal(buf[28:32], []byte{0, 0, 0, 0}) {
		t.Errorf("padding not zero: %x", buf[28:32])
	}
}

func TestHelloDecodeNonzeroPadding(t *testing.T) {
	h := Hello{
		LayoutVersion:           1,
		SupportedProfiles:       ProfileBaseline,
		MaxRequestPayloadBytes:  1024,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: 1024,
		MaxResponseBatchItems:   1,
		PacketSize:              65536,
	}
	var buf [44]byte
	h.Encode(buf[:])

	// Valid first
	if _, err := DecodeHello(buf[:]); err != nil {
		t.Fatalf("valid hello failed: %v", err)
	}

	// Corrupt padding
	buf[28] = 0xFF
	if _, err := DecodeHello(buf[:]); err != ErrBadLayout {
		t.Errorf("nonzero padding: got %v, want ErrBadLayout", err)
	}
}

func TestHelloWireBytes(t *testing.T) {
	h := Hello{
		LayoutVersion:           1,
		Flags:                   0,
		SupportedProfiles:       ProfileBaseline | ProfileSHMFutex,
		PreferredProfiles:       ProfileSHMFutex,
		MaxRequestPayloadBytes:  4096,
		MaxRequestBatchItems:    100,
		MaxResponsePayloadBytes: 1048576,
		MaxResponseBatchItems:   1,
		AuthToken:               0xAABBCCDDEEFF0011,
		PacketSize:              65536,
	}
	var buf [44]byte
	h.Encode(buf[:])

	// supported_profiles = 0x05 LE at offset 4
	if !bytes.Equal(buf[4:8], []byte{0x05, 0x00, 0x00, 0x00}) {
		t.Errorf("supported_profiles bytes: %x", buf[4:8])
	}
	// auth_token at offset 32
	if !bytes.Equal(buf[32:40], []byte{0x11, 0x00, 0xFF, 0xEE, 0xDD, 0xCC, 0xBB, 0xAA}) {
		t.Errorf("auth_token bytes: %x", buf[32:40])
	}
}

// ---------------------------------------------------------------------------
//  Hello-ack payload tests
// ---------------------------------------------------------------------------

func TestHelloAckRoundtrip(t *testing.T) {
	h := HelloAck{
		LayoutVersion:                 1,
		Flags:                         0,
		ServerSupportedProfiles:       0x07,
		IntersectionProfiles:          0x05,
		SelectedProfile:               ProfileSHMFutex,
		AgreedMaxRequestPayloadBytes:  2048,
		AgreedMaxRequestBatchItems:    50,
		AgreedMaxResponsePayloadBytes: 65536,
		AgreedMaxResponseBatchItems:   1,
		AgreedPacketSize:              32768,
	}

	var buf [64]byte
	n := h.Encode(buf[:])
	if n != 48 {
		t.Fatalf("Encode returned %d, want 48", n)
	}

	out, err := DecodeHelloAck(buf[:n])
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if out != h {
		t.Fatalf("roundtrip mismatch:\ngot:  %+v\nwant: %+v", out, h)
	}
}

func TestHelloAckEncodeTooSmall(t *testing.T) {
	h := HelloAck{LayoutVersion: 1}
	var buf [20]byte
	if n := h.Encode(buf[:]); n != 0 {
		t.Fatalf("Encode returned %d, want 0", n)
	}
}

func TestHelloAckDecodeTruncated(t *testing.T) {
	buf := make([]byte, 47)
	_, err := DecodeHelloAck(buf)
	if err != ErrTruncated {
		t.Fatalf("got %v, want ErrTruncated", err)
	}
}

func TestHelloAckDecodeBadLayout(t *testing.T) {
	h := HelloAck{LayoutVersion: 99}
	var buf [48]byte
	h.Encode(buf[:])
	_, err := DecodeHelloAck(buf[:])
	if err != ErrBadLayout {
		t.Fatalf("got %v, want ErrBadLayout", err)
	}
}

// ---------------------------------------------------------------------------
//  Cgroups request tests
// ---------------------------------------------------------------------------

func TestCgroupsRequestRoundtrip(t *testing.T) {
	r := CgroupsRequest{LayoutVersion: 1, Flags: 0}

	var buf [16]byte
	n := r.Encode(buf[:])
	if n != 4 {
		t.Fatalf("Encode returned %d, want 4", n)
	}

	out, err := DecodeCgroupsRequest(buf[:n])
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if out != r {
		t.Fatalf("roundtrip mismatch:\ngot:  %+v\nwant: %+v", out, r)
	}
}

func TestCgroupsRequestEncodeTooSmall(t *testing.T) {
	r := CgroupsRequest{LayoutVersion: 1}
	var buf [2]byte
	if n := r.Encode(buf[:]); n != 0 {
		t.Fatalf("Encode returned %d, want 0", n)
	}
}

func TestCgroupsRequestDecodeTruncated(t *testing.T) {
	buf := make([]byte, 3)
	_, err := DecodeCgroupsRequest(buf)
	if err != ErrTruncated {
		t.Fatalf("got %v, want ErrTruncated", err)
	}
}

func TestCgroupsRequestDecodeBadLayout(t *testing.T) {
	r := CgroupsRequest{LayoutVersion: 2}
	var buf [4]byte
	r.Encode(buf[:])
	_, err := DecodeCgroupsRequest(buf[:])
	if err != ErrBadLayout {
		t.Fatalf("got %v, want ErrBadLayout", err)
	}
}

// ---------------------------------------------------------------------------
//  CStringView tests
// ---------------------------------------------------------------------------

func TestCStringViewBasic(t *testing.T) {
	data := []byte("hello\x00")
	v := NewCStringView(data, 5)

	if v.Len() != 5 {
		t.Fatalf("Len() = %d, want 5", v.Len())
	}
	if !bytes.Equal(v.Bytes(), []byte("hello")) {
		t.Fatalf("Bytes() = %q, want %q", v.Bytes(), "hello")
	}
	if v.String() != "hello" {
		t.Fatalf("String() = %q, want %q", v.String(), "hello")
	}
}

func TestCStringViewEmpty(t *testing.T) {
	data := []byte{0}
	v := NewCStringView(data, 0)

	if v.Len() != 0 {
		t.Fatalf("Len() = %d, want 0", v.Len())
	}
	if len(v.Bytes()) != 0 {
		t.Fatalf("Bytes() len = %d, want 0", len(v.Bytes()))
	}
	if v.String() != "" {
		t.Fatalf("String() = %q, want empty", v.String())
	}
}

// ---------------------------------------------------------------------------
//  Cgroups snapshot response tests
// ---------------------------------------------------------------------------

func TestCgroupsResponseEmptyRoundtrip(t *testing.T) {
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 0, 0, 42)
	total := b.Finish()
	if total != 24 {
		t.Fatalf("Finish returned %d, want 24", total)
	}

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if view.ItemCount != 0 {
		t.Fatalf("ItemCount = %d, want 0", view.ItemCount)
	}
	if view.SystemdEnabled != 0 {
		t.Fatalf("SystemdEnabled = %d, want 0", view.SystemdEnabled)
	}
	if view.Generation != 42 {
		t.Fatalf("Generation = %d, want 42", view.Generation)
	}
}

func TestCgroupsResponseSingleItemRoundtrip(t *testing.T) {
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 1, 1, 100)

	name := []byte("init.scope")
	path := []byte("/sys/fs/cgroup/init.scope")
	if err := b.Add(42, 0x01, 1, name, path); err != nil {
		t.Fatal(err)
	}

	total := b.Finish()

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if view.ItemCount != 1 {
		t.Fatalf("ItemCount = %d, want 1", view.ItemCount)
	}
	if view.SystemdEnabled != 1 {
		t.Fatalf("SystemdEnabled = %d, want 1", view.SystemdEnabled)
	}
	if view.Generation != 100 {
		t.Fatalf("Generation = %d, want 100", view.Generation)
	}

	item, err := view.Item(0)
	if err != nil {
		t.Fatalf("Item(0) error: %v", err)
	}
	if item.Hash != 42 {
		t.Fatalf("Hash = %d, want 42", item.Hash)
	}
	if item.Options != 0x01 {
		t.Fatalf("Options = %d, want 1", item.Options)
	}
	if item.Enabled != 1 {
		t.Fatalf("Enabled = %d, want 1", item.Enabled)
	}
	if item.Name.String() != "init.scope" {
		t.Fatalf("Name = %q, want %q", item.Name.String(), "init.scope")
	}
	if item.Path.String() != "/sys/fs/cgroup/init.scope" {
		t.Fatalf("Path = %q, want %q", item.Path.String(), "/sys/fs/cgroup/init.scope")
	}
}

func TestCgroupsResponseMultiItemRoundtrip(t *testing.T) {
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 3, 1, 999)

	if err := b.Add(100, 0, 1,
		[]byte("init.scope"),
		[]byte("/sys/fs/cgroup/init.scope")); err != nil {
		t.Fatal(err)
	}
	if err := b.Add(200, 0x02, 0,
		[]byte("system.slice/docker-abc.scope"),
		[]byte("/sys/fs/cgroup/system.slice/docker-abc.scope")); err != nil {
		t.Fatal(err)
	}
	if err := b.Add(300, 0, 1, []byte(""), []byte("")); err != nil {
		t.Fatal(err)
	}

	total := b.Finish()

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if view.ItemCount != 3 {
		t.Fatalf("ItemCount = %d, want 3", view.ItemCount)
	}
	if view.SystemdEnabled != 1 {
		t.Fatalf("SystemdEnabled = %d, want 1", view.SystemdEnabled)
	}
	if view.Generation != 999 {
		t.Fatalf("Generation = %d, want 999", view.Generation)
	}

	// Item 0
	item, err := view.Item(0)
	if err != nil {
		t.Fatal(err)
	}
	if item.Hash != 100 {
		t.Errorf("item 0 Hash = %d, want 100", item.Hash)
	}
	if item.Options != 0 {
		t.Errorf("item 0 Options = %d, want 0", item.Options)
	}
	if item.Enabled != 1 {
		t.Errorf("item 0 Enabled = %d, want 1", item.Enabled)
	}
	if item.Name.String() != "init.scope" {
		t.Errorf("item 0 Name = %q", item.Name.String())
	}
	if item.Path.String() != "/sys/fs/cgroup/init.scope" {
		t.Errorf("item 0 Path = %q", item.Path.String())
	}

	// Item 1
	item, err = view.Item(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Hash != 200 {
		t.Errorf("item 1 Hash = %d, want 200", item.Hash)
	}
	if item.Options != 0x02 {
		t.Errorf("item 1 Options = %d, want 2", item.Options)
	}
	if item.Enabled != 0 {
		t.Errorf("item 1 Enabled = %d, want 0", item.Enabled)
	}
	if item.Name.String() != "system.slice/docker-abc.scope" {
		t.Errorf("item 1 Name = %q", item.Name.String())
	}

	// Item 2 (empty strings)
	item, err = view.Item(2)
	if err != nil {
		t.Fatal(err)
	}
	if item.Hash != 300 {
		t.Errorf("item 2 Hash = %d, want 300", item.Hash)
	}
	if item.Name.Len() != 0 {
		t.Errorf("item 2 Name.Len() = %d, want 0", item.Name.Len())
	}
	if item.Path.Len() != 0 {
		t.Errorf("item 2 Path.Len() = %d, want 0", item.Path.Len())
	}
}

func TestCgroupsResponseCompaction(t *testing.T) {
	// Reserve space for 100 items, add only 2.
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 100, 0, 7)

	if err := b.Add(1, 0, 1, []byte("a"), []byte("b")); err != nil {
		t.Fatal(err)
	}
	if err := b.Add(2, 0, 1, []byte("c"), []byte("d")); err != nil {
		t.Fatal(err)
	}

	total := b.Finish()

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if view.ItemCount != 2 {
		t.Fatalf("ItemCount = %d, want 2", view.ItemCount)
	}

	item, err := view.Item(0)
	if err != nil {
		t.Fatal(err)
	}
	if item.Name.String() != "a" {
		t.Fatalf("item 0 Name = %q, want %q", item.Name.String(), "a")
	}

	item, err = view.Item(1)
	if err != nil {
		t.Fatal(err)
	}
	if item.Name.String() != "c" {
		t.Fatalf("item 1 Name = %q, want %q", item.Name.String(), "c")
	}
}

func TestCgroupsResponseItemOutOfBounds(t *testing.T) {
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 1, 0, 1)
	if err := b.Add(1, 0, 1, []byte("x"), []byte("y")); err != nil {
		t.Fatal(err)
	}
	total := b.Finish()

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}

	_, err = view.Item(1) // index out of bounds
	if err != ErrOutOfBounds {
		t.Fatalf("got %v, want ErrOutOfBounds", err)
	}
}

func TestCgroupsResponseDecodeTruncated(t *testing.T) {
	buf := make([]byte, 23) // less than 24-byte header
	_, err := DecodeCgroupsResponse(buf)
	if err != ErrTruncated {
		t.Fatalf("got %v, want ErrTruncated", err)
	}
}

func TestCgroupsResponseDecodeBadLayout(t *testing.T) {
	buf := make([]byte, 24)
	ne.PutUint16(buf[0:2], 99) // bad layout_version
	_, err := DecodeCgroupsResponse(buf)
	if err != ErrBadLayout {
		t.Fatalf("got %v, want ErrBadLayout", err)
	}
}

func TestCgroupsResponseDecodeDirectoryTruncated(t *testing.T) {
	buf := make([]byte, 28)     // 24 header + 4 bytes, not enough for 1 dir entry (8)
	ne.PutUint16(buf[0:2], 1)   // layout_version
	ne.PutUint16(buf[2:4], 0)   // flags
	ne.PutUint32(buf[4:8], 1)   // item_count = 1
	ne.PutUint32(buf[8:12], 0)  // systemd_enabled
	ne.PutUint32(buf[12:16], 0) // reserved
	ne.PutUint64(buf[16:24], 0) // generation

	_, err := DecodeCgroupsResponse(buf)
	if err != ErrTruncated {
		t.Fatalf("got %v, want ErrTruncated", err)
	}
}

func TestCgroupsResponseDecodeItemBadAlignment(t *testing.T) {
	// Create a payload with a directory entry that has a non-aligned offset.
	buf := make([]byte, 128)
	ne.PutUint16(buf[0:2], 1)   // layout_version
	ne.PutUint16(buf[2:4], 0)   // flags
	ne.PutUint32(buf[4:8], 1)   // item_count = 1
	ne.PutUint32(buf[8:12], 0)  // systemd_enabled
	ne.PutUint32(buf[12:16], 0) // reserved
	ne.PutUint64(buf[16:24], 0) // generation

	// Directory entry at offset 24: offset=3 (not aligned), length=32
	ne.PutUint32(buf[24:28], 3) // bad alignment
	ne.PutUint32(buf[28:32], 32)

	_, err := DecodeCgroupsResponse(buf)
	if err != ErrBadAlignment {
		t.Fatalf("got %v, want ErrBadAlignment", err)
	}
}

func TestCgroupsResponseDecodeItemOutOfBounds(t *testing.T) {
	buf := make([]byte, 64)
	ne.PutUint16(buf[0:2], 1)
	ne.PutUint16(buf[2:4], 0)
	ne.PutUint32(buf[4:8], 1) // item_count = 1
	ne.PutUint32(buf[8:12], 0)
	ne.PutUint32(buf[12:16], 0)
	ne.PutUint64(buf[16:24], 0)

	// Dir entry: offset=0, length=1000 (exceeds buffer)
	ne.PutUint32(buf[24:28], 0)
	ne.PutUint32(buf[28:32], 1000)

	_, err := DecodeCgroupsResponse(buf)
	if err != ErrOutOfBounds {
		t.Fatalf("got %v, want ErrOutOfBounds", err)
	}
}

func TestCgroupsResponseDecodeItemTooSmall(t *testing.T) {
	// Item is present but smaller than 32-byte item header.
	buf := make([]byte, 64)
	ne.PutUint16(buf[0:2], 1)
	ne.PutUint16(buf[2:4], 0)
	ne.PutUint32(buf[4:8], 1) // item_count = 1
	ne.PutUint32(buf[8:12], 0)
	ne.PutUint32(buf[12:16], 0)
	ne.PutUint64(buf[16:24], 0)

	// Dir entry: offset=0, length=16 (< 32 item header)
	ne.PutUint32(buf[24:28], 0)
	ne.PutUint32(buf[28:32], 16)

	_, err := DecodeCgroupsResponse(buf)
	if err != ErrTruncated {
		t.Fatalf("got %v, want ErrTruncated", err)
	}
}

func TestCgroupsResponseItemBadLayout(t *testing.T) {
	// Build a valid response, then corrupt the item's layout_version.
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 1, 0, 1)
	if err := b.Add(1, 0, 1, []byte("x"), []byte("y")); err != nil {
		t.Fatal(err)
	}
	total := b.Finish()

	// Find the item start and corrupt layout_version.
	dirBase := cgroupsRespHdr
	itemOff := int(ne.Uint32(buf[dirBase : dirBase+4]))
	packedStart := cgroupsRespHdr + 1*cgroupsDirEntry
	itemStart := packedStart + itemOff
	ne.PutUint16(buf[itemStart:itemStart+2], 99) // corrupt layout_version

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}

	_, err = view.Item(0)
	if err != ErrBadLayout {
		t.Fatalf("got %v, want ErrBadLayout", err)
	}
}

func TestCgroupsResponseItemMissingNul(t *testing.T) {
	// Build a valid response, then overwrite the name's NUL terminator.
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 1, 0, 1)
	if err := b.Add(1, 0, 1, []byte("test"), []byte("path")); err != nil {
		t.Fatal(err)
	}
	total := b.Finish()

	// Find item and overwrite the NUL after "test".
	dirBase := cgroupsRespHdr
	itemOff := int(ne.Uint32(buf[dirBase : dirBase+4]))
	packedStart := cgroupsRespHdr + 1*cgroupsDirEntry
	itemStart := packedStart + itemOff
	// name is at offset 32 within the item, length 4, NUL at 36.
	buf[itemStart+32+4] = 'X' // overwrite NUL

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}
	_, err = view.Item(0)
	if err != ErrMissingNul {
		t.Fatalf("got %v, want ErrMissingNul", err)
	}
}

func TestCgroupsResponseItemNameOutOfBounds(t *testing.T) {
	// Build valid, then corrupt name_offset to point beyond item bounds.
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 1, 0, 1)
	if err := b.Add(1, 0, 1, []byte("x"), []byte("y")); err != nil {
		t.Fatal(err)
	}
	total := b.Finish()

	dirBase := cgroupsRespHdr
	itemOff := int(ne.Uint32(buf[dirBase : dirBase+4]))
	packedStart := cgroupsRespHdr + 1*cgroupsDirEntry
	itemStart := packedStart + itemOff
	// Corrupt name_offset to a huge value.
	ne.PutUint32(buf[itemStart+16:itemStart+20], 9999)

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}
	_, err = view.Item(0)
	if err != ErrOutOfBounds {
		t.Fatalf("got %v, want ErrOutOfBounds", err)
	}
}

func TestCgroupsResponseItemNameOffsetBelowHeader(t *testing.T) {
	// name_offset < 32 (item header size).
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 1, 0, 1)
	if err := b.Add(1, 0, 1, []byte("x"), []byte("y")); err != nil {
		t.Fatal(err)
	}
	total := b.Finish()

	dirBase := cgroupsRespHdr
	itemOff := int(ne.Uint32(buf[dirBase : dirBase+4]))
	packedStart := cgroupsRespHdr + 1*cgroupsDirEntry
	itemStart := packedStart + itemOff
	ne.PutUint32(buf[itemStart+16:itemStart+20], 4) // below 32

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}
	_, err = view.Item(0)
	if err != ErrOutOfBounds {
		t.Fatalf("got %v, want ErrOutOfBounds", err)
	}
}

func TestCgroupsBuilderOverflowMaxItems(t *testing.T) {
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 1, 0, 1)
	if err := b.Add(1, 0, 1, []byte("a"), []byte("b")); err != nil {
		t.Fatal(err)
	}
	if err := b.Add(2, 0, 1, []byte("c"), []byte("d")); err != ErrOverflow {
		t.Fatalf("got %v, want ErrOverflow", err)
	}
}

func TestCgroupsBuilderOverflowBuffer(t *testing.T) {
	buf := make([]byte, 40) // too small for header + dir + item
	b := NewCgroupsBuilder(buf, 1, 0, 1)
	err := b.Add(1, 0, 1, []byte("long-name-that-wont-fit"), []byte("long-path-too"))
	if err != ErrOverflow {
		t.Fatalf("got %v, want ErrOverflow", err)
	}
}

func TestCgroupsResponseEmptyStrings(t *testing.T) {
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 1, 0, 1)
	if err := b.Add(42, 0, 1, []byte(""), []byte("")); err != nil {
		t.Fatal(err)
	}
	total := b.Finish()

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}

	item, err := view.Item(0)
	if err != nil {
		t.Fatal(err)
	}
	if item.Name.Len() != 0 {
		t.Fatalf("Name.Len() = %d, want 0", item.Name.Len())
	}
	if item.Path.Len() != 0 {
		t.Fatalf("Path.Len() = %d, want 0", item.Path.Len())
	}
	if item.Name.String() != "" {
		t.Fatalf("Name.String() = %q, want empty", item.Name.String())
	}
	if item.Path.String() != "" {
		t.Fatalf("Path.String() = %q, want empty", item.Path.String())
	}
}

// ---------------------------------------------------------------------------
//  Interop reference: test values matching C and Rust interop binaries
// ---------------------------------------------------------------------------

func TestInteropHeaderValues(t *testing.T) {
	h := Header{
		Magic:           MagicMsg,
		Version:         Version,
		HeaderLen:       HeaderLen,
		Kind:            KindRequest,
		Flags:           FlagBatch,
		Code:            MethodCgroupsSnapshot,
		TransportStatus: StatusOK,
		PayloadLen:      12345,
		ItemCount:       42,
		MessageID:       0xDEADBEEFCAFEBABE,
	}

	var buf [32]byte
	h.Encode(buf[:])

	// Decode and verify all fields match.
	out, err := DecodeHeader(buf[:])
	if err != nil {
		t.Fatal(err)
	}
	if out.Magic != MagicMsg {
		t.Errorf("magic = %x", out.Magic)
	}
	if out.PayloadLen != 12345 {
		t.Errorf("payload_len = %d", out.PayloadLen)
	}
	if out.ItemCount != 42 {
		t.Errorf("item_count = %d", out.ItemCount)
	}
	if out.MessageID != 0xDEADBEEFCAFEBABE {
		t.Errorf("message_id = %x", out.MessageID)
	}
}

func TestInteropChunkValues(t *testing.T) {
	c := ChunkHeader{
		Magic:           MagicChunk,
		Version:         Version,
		Flags:           0,
		MessageID:       0x1234567890ABCDEF,
		TotalMessageLen: 100000,
		ChunkIndex:      3,
		ChunkCount:      10,
		ChunkPayloadLen: 8192,
	}

	var buf [32]byte
	c.Encode(buf[:])

	out, err := DecodeChunkHeader(buf[:])
	if err != nil {
		t.Fatal(err)
	}
	if out.MessageID != 0x1234567890ABCDEF {
		t.Errorf("message_id = %x", out.MessageID)
	}
	if out.TotalMessageLen != 100000 {
		t.Errorf("total_message_len = %d", out.TotalMessageLen)
	}
	if out.ChunkIndex != 3 {
		t.Errorf("chunk_index = %d", out.ChunkIndex)
	}
	if out.ChunkCount != 10 {
		t.Errorf("chunk_count = %d", out.ChunkCount)
	}
	if out.ChunkPayloadLen != 8192 {
		t.Errorf("chunk_payload_len = %d", out.ChunkPayloadLen)
	}
}

func TestInteropHelloValues(t *testing.T) {
	h := Hello{
		LayoutVersion:           1,
		Flags:                   0,
		SupportedProfiles:       ProfileBaseline | ProfileSHMFutex,
		PreferredProfiles:       ProfileSHMFutex,
		MaxRequestPayloadBytes:  4096,
		MaxRequestBatchItems:    100,
		MaxResponsePayloadBytes: 1048576,
		MaxResponseBatchItems:   1,
		AuthToken:               0xAABBCCDDEEFF0011,
		PacketSize:              65536,
	}

	var buf [44]byte
	h.Encode(buf[:])

	out, err := DecodeHello(buf[:])
	if err != nil {
		t.Fatal(err)
	}
	if out.SupportedProfiles != 0x05 {
		t.Errorf("supported = %x", out.SupportedProfiles)
	}
	if out.AuthToken != 0xAABBCCDDEEFF0011 {
		t.Errorf("auth_token = %x", out.AuthToken)
	}
	if out.PacketSize != 65536 {
		t.Errorf("packet_size = %d", out.PacketSize)
	}
}

func TestInteropHelloAckValues(t *testing.T) {
	h := HelloAck{
		LayoutVersion:                 1,
		Flags:                         0,
		ServerSupportedProfiles:       0x07,
		IntersectionProfiles:          0x05,
		SelectedProfile:               ProfileSHMFutex,
		AgreedMaxRequestPayloadBytes:  2048,
		AgreedMaxRequestBatchItems:    50,
		AgreedMaxResponsePayloadBytes: 65536,
		AgreedMaxResponseBatchItems:   1,
		AgreedPacketSize:              32768,
	}

	var buf [48]byte
	h.Encode(buf[:])

	out, err := DecodeHelloAck(buf[:])
	if err != nil {
		t.Fatal(err)
	}
	if out.ServerSupportedProfiles != 0x07 {
		t.Errorf("server_supported = %x", out.ServerSupportedProfiles)
	}
	if out.AgreedPacketSize != 32768 {
		t.Errorf("agreed_packet_size = %d", out.AgreedPacketSize)
	}
}

func TestInteropCgroupsResponseValues(t *testing.T) {
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 3, 1, 999)

	if err := b.Add(100, 0, 1,
		[]byte("init.scope"),
		[]byte("/sys/fs/cgroup/init.scope")); err != nil {
		t.Fatal(err)
	}
	if err := b.Add(200, 0x02, 0,
		[]byte("system.slice/docker-abc.scope"),
		[]byte("/sys/fs/cgroup/system.slice/docker-abc.scope")); err != nil {
		t.Fatal(err)
	}
	if err := b.Add(300, 0, 1, []byte(""), []byte("")); err != nil {
		t.Fatal(err)
	}

	total := b.Finish()

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}
	if view.ItemCount != 3 {
		t.Fatalf("ItemCount = %d", view.ItemCount)
	}
	if view.SystemdEnabled != 1 {
		t.Fatalf("SystemdEnabled = %d", view.SystemdEnabled)
	}
	if view.Generation != 999 {
		t.Fatalf("Generation = %d", view.Generation)
	}

	item0, _ := view.Item(0)
	if item0.Hash != 100 || item0.Name.String() != "init.scope" {
		t.Errorf("item 0: hash=%d name=%q", item0.Hash, item0.Name.String())
	}

	item1, _ := view.Item(1)
	if item1.Hash != 200 || item1.Options != 0x02 || item1.Enabled != 0 {
		t.Errorf("item 1: hash=%d options=%d enabled=%d",
			item1.Hash, item1.Options, item1.Enabled)
	}
	if item1.Name.String() != "system.slice/docker-abc.scope" {
		t.Errorf("item 1 name: %q", item1.Name.String())
	}

	item2, _ := view.Item(2)
	if item2.Hash != 300 || item2.Name.Len() != 0 || item2.Path.Len() != 0 {
		t.Errorf("item 2: hash=%d namelen=%d pathlen=%d",
			item2.Hash, item2.Name.Len(), item2.Path.Len())
	}
}

func TestInteropCgroupsResponseEmptyValues(t *testing.T) {
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 0, 0, 42)
	total := b.Finish()

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}
	if view.ItemCount != 0 {
		t.Errorf("ItemCount = %d", view.ItemCount)
	}
	if view.SystemdEnabled != 0 {
		t.Errorf("SystemdEnabled = %d", view.SystemdEnabled)
	}
	if view.Generation != 42 {
		t.Errorf("Generation = %d", view.Generation)
	}
}

// ---------------------------------------------------------------------------
//  Large snapshot test
// ---------------------------------------------------------------------------

func TestCgroupsResponseLargeSnapshot(t *testing.T) {
	const n = 200
	buf := make([]byte, 1024*1024)
	b := NewCgroupsBuilder(buf, n, 1, 12345)

	for i := 0; i < n; i++ {
		name := []byte("cgroup-" + string(rune('A'+i%26)))
		path := []byte("/sys/fs/cgroup/system.slice/cgroup-" + string(rune('A'+i%26)))
		if err := b.Add(uint32(i), uint32(i%4), uint32(i%2), name, path); err != nil {
			t.Fatalf("item %d: %v", i, err)
		}
	}

	total := b.Finish()

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}
	if view.ItemCount != n {
		t.Fatalf("ItemCount = %d, want %d", view.ItemCount, n)
	}

	for i := uint32(0); i < n; i++ {
		item, err := view.Item(i)
		if err != nil {
			t.Fatalf("item %d: %v", i, err)
		}
		if item.Hash != i {
			t.Errorf("item %d: hash = %d", i, item.Hash)
		}
	}
}

// ---------------------------------------------------------------------------
//  Error string tests (ensure errors have useful messages)
// ---------------------------------------------------------------------------

func TestErrorStrings(t *testing.T) {
	errs := []error{
		ErrTruncated, ErrBadMagic, ErrBadVersion, ErrBadHeaderLen,
		ErrBadKind, ErrBadLayout, ErrOutOfBounds, ErrMissingNul,
		ErrBadAlignment, ErrBadItemCount, ErrOverflow,
	}
	for _, e := range errs {
		if e.Error() == "" {
			t.Errorf("error has empty string: %v", e)
		}
	}
}

// ---------------------------------------------------------------------------
//  Decode from zero/garbage bytes (robustness)
// ---------------------------------------------------------------------------

func TestDecodeZeroBytes(t *testing.T) {
	buf := make([]byte, 128)

	_, err := DecodeHeader(buf)
	if err == nil {
		t.Error("expected error for zero header")
	}

	_, err = DecodeChunkHeader(buf)
	if err == nil {
		t.Error("expected error for zero chunk header")
	}

	_, err = DecodeHello(buf)
	if err == nil {
		t.Error("expected error for zero hello")
	}

	_, err = DecodeHelloAck(buf)
	if err == nil {
		t.Error("expected error for zero hello_ack")
	}

	_, err = DecodeCgroupsRequest(buf)
	if err == nil {
		t.Error("expected error for zero cgroups_req")
	}

	_, err = DecodeCgroupsResponse(buf)
	if err == nil {
		t.Error("expected error for zero cgroups_resp")
	}
}

func TestDecodeGarbage(t *testing.T) {
	buf := make([]byte, 128)
	for i := range buf {
		buf[i] = 0xFF
	}

	_, err := DecodeHeader(buf)
	if err == nil {
		t.Error("expected error for garbage header")
	}

	_, err = DecodeChunkHeader(buf)
	if err == nil {
		t.Error("expected error for garbage chunk header")
	}

	_, err = DecodeHello(buf)
	if err == nil {
		t.Error("expected error for garbage hello")
	}

	_, err = DecodeHelloAck(buf)
	if err == nil {
		t.Error("expected error for garbage hello_ack")
	}

	_, err = DecodeCgroupsRequest(buf)
	if err == nil {
		t.Error("expected error for garbage cgroups_req")
	}

	_, err = DecodeCgroupsResponse(buf)
	if err == nil {
		t.Error("expected error for garbage cgroups_resp")
	}
}

func TestDecodeEmptyBuf(t *testing.T) {
	empty := []byte{}
	if _, err := DecodeHeader(empty); err != ErrTruncated {
		t.Errorf("header: got %v", err)
	}
	if _, err := DecodeChunkHeader(empty); err != ErrTruncated {
		t.Errorf("chunk: got %v", err)
	}
	if _, err := DecodeHello(empty); err != ErrTruncated {
		t.Errorf("hello: got %v", err)
	}
	if _, err := DecodeHelloAck(empty); err != ErrTruncated {
		t.Errorf("hello_ack: got %v", err)
	}
	if _, err := DecodeCgroupsRequest(empty); err != ErrTruncated {
		t.Errorf("cgroups_req: got %v", err)
	}
	if _, err := DecodeCgroupsResponse(empty); err != ErrTruncated {
		t.Errorf("cgroups_resp: got %v", err)
	}
}

// ---------------------------------------------------------------------------
//  Path-level string validation in cgroups item
// ---------------------------------------------------------------------------

func TestCgroupsResponseItemPathMissingNul(t *testing.T) {
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 1, 0, 1)
	if err := b.Add(1, 0, 1, []byte("ok"), []byte("bad")); err != nil {
		t.Fatal(err)
	}
	total := b.Finish()

	// Overwrite the NUL after "bad" (the path string).
	dirBase := cgroupsRespHdr
	itemOff := int(ne.Uint32(buf[dirBase : dirBase+4]))
	packedStart := cgroupsRespHdr + 1*cgroupsDirEntry
	itemStart := packedStart + itemOff

	// path_offset is at item+24, path_len at item+28.
	pathOff := int(ne.Uint32(buf[itemStart+24 : itemStart+28]))
	pathLen := int(ne.Uint32(buf[itemStart+28 : itemStart+32]))
	buf[itemStart+pathOff+pathLen] = 'X' // corrupt NUL

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}
	_, err = view.Item(0)
	if err != ErrMissingNul {
		t.Fatalf("got %v, want ErrMissingNul", err)
	}
}

func TestCgroupsResponseItemPathOutOfBounds(t *testing.T) {
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 1, 0, 1)
	if err := b.Add(1, 0, 1, []byte("x"), []byte("y")); err != nil {
		t.Fatal(err)
	}
	total := b.Finish()

	dirBase := cgroupsRespHdr
	itemOff := int(ne.Uint32(buf[dirBase : dirBase+4]))
	packedStart := cgroupsRespHdr + 1*cgroupsDirEntry
	itemStart := packedStart + itemOff
	ne.PutUint32(buf[itemStart+24:itemStart+28], 9999) // corrupt path_offset

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}
	_, err = view.Item(0)
	if err != ErrOutOfBounds {
		t.Fatalf("got %v, want ErrOutOfBounds", err)
	}
}

func TestCgroupsResponseItemPathOffsetBelowHeader(t *testing.T) {
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 1, 0, 1)
	if err := b.Add(1, 0, 1, []byte("x"), []byte("y")); err != nil {
		t.Fatal(err)
	}
	total := b.Finish()

	dirBase := cgroupsRespHdr
	itemOff := int(ne.Uint32(buf[dirBase : dirBase+4]))
	packedStart := cgroupsRespHdr + 1*cgroupsDirEntry
	itemStart := packedStart + itemOff
	ne.PutUint32(buf[itemStart+24:itemStart+28], 4) // below 32

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}
	_, err = view.Item(0)
	if err != ErrOutOfBounds {
		t.Fatalf("got %v, want ErrOutOfBounds", err)
	}
}

func TestCgroupsResponseItemOverlapRejected(t *testing.T) {
	buf := make([]byte, 8192)
	b := NewCgroupsBuilder(buf, 1, 0, 1)
	if err := b.Add(1, 0, 1, []byte("hello"), []byte("/path")); err != nil {
		t.Fatal(err)
	}
	total := b.Finish()

	dirBase := cgroupsRespHdr
	itemOff := int(ne.Uint32(buf[dirBase : dirBase+4]))
	packedStart := cgroupsRespHdr + 1*cgroupsDirEntry
	itemStart := packedStart + itemOff

	// name region is [32..38) (name_off=32, name_len=5, +1 for NUL)
	// Set path_off=34 (inside name), path_len=1
	ne.PutUint32(buf[itemStart+24:itemStart+28], 34)
	ne.PutUint32(buf[itemStart+28:itemStart+32], 1)
	// Ensure NUL at item[35]
	buf[itemStart+35] = 0

	view, err := DecodeCgroupsResponse(buf[:total])
	if err != nil {
		t.Fatal(err)
	}
	_, err = view.Item(0)
	if err != ErrBadLayout {
		t.Fatalf("got %v, want ErrBadLayout (overlapping fields)", err)
	}
}
