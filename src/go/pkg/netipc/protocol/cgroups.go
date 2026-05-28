// Cgroups snapshot codec -- request, response view, builder, dispatch.

package protocol

import (
	"fmt"
)

// ---------------------------------------------------------------------------
//  Cgroups snapshot request (4 bytes)
// ---------------------------------------------------------------------------

// CgroupsRequest is the cgroups snapshot request payload (4 bytes).
type CgroupsRequest struct {
	LayoutVersion uint16
	Flags         uint16
}

// Encode writes the request into buf. Returns 4 on success, 0 if buf is
// too small.
func (r *CgroupsRequest) Encode(buf []byte) int {
	if len(buf) < cgroupsReqSize {
		return 0
	}
	ne.PutUint16(buf[0:2], r.LayoutVersion)
	ne.PutUint16(buf[2:4], r.Flags)
	return cgroupsReqSize
}

// DecodeCgroupsRequest decodes a cgroups request from buf. Validates
// layout_version.
func DecodeCgroupsRequest(buf []byte) (CgroupsRequest, error) {
	if len(buf) < cgroupsReqSize {
		return CgroupsRequest{}, ErrTruncated
	}
	r := CgroupsRequest{
		LayoutVersion: ne.Uint16(buf[0:2]),
		Flags:         ne.Uint16(buf[2:4]),
	}
	if r.LayoutVersion != 1 {
		return CgroupsRequest{}, ErrBadLayout
	}
	// flags must be zero (reserved for future use)
	if r.Flags != 0 {
		return CgroupsRequest{}, ErrBadLayout
	}
	return r, nil
}

// ---------------------------------------------------------------------------
//  CStringView - borrowed string view into payload buffer
// ---------------------------------------------------------------------------

// CStringView is a borrowed, zero-copy string view into the payload buffer.
// It wraps a byte slice that includes the NUL terminator. The view is
// ephemeral and valid only while the underlying payload buffer lives.
// Copy immediately via String() if the data is needed later.
type CStringView struct {
	data []byte // includes trailing NUL
	len  uint32 // length excluding NUL
}

// NewCStringView creates a CStringView from a slice that includes the NUL
// terminator and the length excluding the NUL.
func NewCStringView(data []byte, length uint32) CStringView {
	return CStringView{data: data, len: length}
}

// Bytes returns the string content as a byte slice (without the NUL).
func (v CStringView) Bytes() []byte {
	return v.data[:v.len]
}

// Len returns the string length excluding the NUL terminator.
func (v CStringView) Len() uint32 {
	return v.len
}

// String returns a copy of the string content. This allocates.
func (v CStringView) String() string {
	return string(v.data[:v.len])
}

// GoString implements fmt.GoStringer for debug output.
func (v CStringView) GoString() string {
	return fmt.Sprintf("CStringView(%q)", v.data[:v.len])
}

// ---------------------------------------------------------------------------
//  Cgroups snapshot response
// ---------------------------------------------------------------------------

// CgroupsItemView is a per-item view -- ephemeral, borrows the payload
// buffer. Valid only while the payload buffer is alive.
type CgroupsItemView struct {
	LayoutVersion uint16
	Flags         uint16
	Hash          uint32
	Options       uint32
	Enabled       uint32
	Name          CStringView
	Path          CStringView
}

// CgroupsResponseView is a full snapshot view -- ephemeral, borrows the
// payload buffer. Valid only during the current library call or callback.
// Copy immediately if the data is needed later.
type CgroupsResponseView struct {
	LayoutVersion  uint16
	Flags          uint16
	ItemCount      uint32
	SystemdEnabled uint32
	Generation     uint64
	payload        []byte // full payload for item access
}

// DecodeCgroupsResponse decodes the snapshot response header and validates
// the item directory. On success, use Item() to access individual items.
func DecodeCgroupsResponse(buf []byte) (CgroupsResponseView, error) {
	if len(buf) < cgroupsRespHdr {
		return CgroupsResponseView{}, ErrTruncated
	}

	layoutVersion := ne.Uint16(buf[0:2])
	flags := ne.Uint16(buf[2:4])
	itemCount := ne.Uint32(buf[4:8])
	systemdEnabled := ne.Uint32(buf[8:12])
	reserved := ne.Uint32(buf[12:16])
	generation := ne.Uint64(buf[16:24])

	if layoutVersion != 1 {
		return CgroupsResponseView{}, ErrBadLayout
	}

	// flags must be zero
	if flags != 0 {
		return CgroupsResponseView{}, ErrBadLayout
	}

	// reserved field must be zero
	if reserved != 0 {
		return CgroupsResponseView{}, ErrBadLayout
	}

	// Validate directory fits (use uint64 to prevent int overflow on 32-bit).
	dirSize64 := uint64(itemCount) * uint64(cgroupsDirEntry)
	dirEnd64 := uint64(cgroupsRespHdr) + dirSize64
	if dirEnd64 > uint64(len(buf)) {
		return CgroupsResponseView{}, ErrTruncated
	}
	dirEnd := int(dirEnd64)

	packedAreaLen := len(buf) - dirEnd

	// Validate each directory entry.
	dirSize := int(dirSize64)
	for i := 0; i < dirSize; i += cgroupsDirEntry {
		base := cgroupsRespHdr + i
		off := ne.Uint32(buf[base : base+4])
		length := ne.Uint32(buf[base+4 : base+8])

		if int(off)%Alignment != 0 {
			return CgroupsResponseView{}, ErrBadAlignment
		}
		if uint64(off)+uint64(length) > uint64(packedAreaLen) {
			return CgroupsResponseView{}, ErrOutOfBounds
		}
		if int(length) < cgroupsItemHdr {
			return CgroupsResponseView{}, ErrTruncated
		}
	}

	return CgroupsResponseView{
		LayoutVersion:  layoutVersion,
		Flags:          flags,
		ItemCount:      itemCount,
		SystemdEnabled: systemdEnabled,
		Generation:     generation,
		payload:        buf,
	}, nil
}

// Item accesses the item at index from a decoded snapshot view. Returns an
// ephemeral item view.
func (v *CgroupsResponseView) Item(index uint32) (CgroupsItemView, error) {
	if index >= v.ItemCount {
		return CgroupsItemView{}, ErrOutOfBounds
	}

	dirStart := cgroupsRespHdr
	dirSize := int(uint64(v.ItemCount) * uint64(cgroupsDirEntry))
	packedAreaStart := dirStart + dirSize

	dirBase := dirStart + int(index)*cgroupsDirEntry
	itemOff := int(ne.Uint32(v.payload[dirBase : dirBase+4]))
	itemLen := int(ne.Uint32(v.payload[dirBase+4 : dirBase+8]))

	itemStart := packedAreaStart + itemOff
	item := v.payload[itemStart : itemStart+itemLen]

	layoutVersion := ne.Uint16(item[0:2])
	flags := ne.Uint16(item[2:4])
	hash := ne.Uint32(item[4:8])
	options := ne.Uint32(item[8:12])
	enabled := ne.Uint32(item[12:16])

	nameOff := int(ne.Uint32(item[16:20]))
	nameLen := ne.Uint32(item[20:24])
	pathOff := int(ne.Uint32(item[24:28]))
	pathLen := ne.Uint32(item[28:32])

	if layoutVersion != 1 {
		return CgroupsItemView{}, ErrBadLayout
	}

	// item flags must be zero
	if flags != 0 {
		return CgroupsItemView{}, ErrBadLayout
	}

	// Validate name string.
	if nameOff < cgroupsItemHdr {
		return CgroupsItemView{}, ErrOutOfBounds
	}
	if uint64(nameOff)+uint64(nameLen)+1 > uint64(itemLen) {
		return CgroupsItemView{}, ErrOutOfBounds
	}
	if item[nameOff+int(nameLen)] != 0 {
		return CgroupsItemView{}, ErrMissingNul
	}

	// Validate path string.
	if pathOff < cgroupsItemHdr {
		return CgroupsItemView{}, ErrOutOfBounds
	}
	if uint64(pathOff)+uint64(pathLen)+1 > uint64(itemLen) {
		return CgroupsItemView{}, ErrOutOfBounds
	}
	if item[pathOff+int(pathLen)] != 0 {
		return CgroupsItemView{}, ErrMissingNul
	}

	// Reject overlapping name and path regions (including NUL)
	{
		nameStart := uint64(nameOff)
		nameEnd := nameStart + uint64(nameLen) + 1
		pathStart := uint64(pathOff)
		pathEnd := pathStart + uint64(pathLen) + 1
		if nameStart < pathEnd && pathStart < nameEnd {
			return CgroupsItemView{}, ErrBadLayout
		}
	}

	name := NewCStringView(item[nameOff:nameOff+int(nameLen)+1], nameLen)
	path := NewCStringView(item[pathOff:pathOff+int(pathLen)+1], pathLen)

	return CgroupsItemView{
		LayoutVersion: layoutVersion,
		Flags:         flags,
		Hash:          hash,
		Options:       options,
		Enabled:       enabled,
		Name:          name,
		Path:          path,
	}, nil
}

// ---------------------------------------------------------------------------
//  Cgroups snapshot response builder
// ---------------------------------------------------------------------------

// CgroupsBuilder builds a cgroups snapshot response payload.
//
// Layout during building (maxItems directory slots reserved):
//
//	[24-byte header space] [maxItems*8 directory] [packed items]
//
// Layout after Finish (compacted to actual itemCount):
//
//	[24-byte header] [itemCount*8 directory] [packed items]
type CgroupsBuilder struct {
	buf            []byte
	systemdEnabled uint32
	generation     uint64
	itemCount      uint32
	maxItems       uint32
	dataOffset     int // current write position (absolute in buf)
}

// NewCgroupsBuilder initializes a cgroups response builder. buf must be
// caller-owned and large enough for the expected snapshot.
func NewCgroupsBuilder(buf []byte, maxItems uint32, systemdEnabled uint32, generation uint64) *CgroupsBuilder {
	minRequired, ok := CgroupsBuilderMinBytes(maxItems)
	if !ok || len(buf) < minRequired {
		panic(fmt.Sprintf("CgroupsBuilder buffer too small: need at least %d bytes, got %d",
			minRequired, len(buf)))
	}
	dataOffset := minRequired
	return &CgroupsBuilder{
		buf:            buf,
		systemdEnabled: systemdEnabled,
		generation:     generation,
		maxItems:       maxItems,
		dataOffset:     dataOffset,
	}
}

// CgroupsBuilderMinBytes returns the minimum response buffer required to
// reserve directory slots for maxItems before packed item data is appended.
func CgroupsBuilderMinBytes(maxItems uint32) (int, bool) {
	minRequired := uint64(cgroupsRespHdr) + uint64(maxItems)*uint64(cgroupsDirEntry)
	maxInt := uint64(int(^uint(0) >> 1))
	if minRequired > maxInt {
		return 0, false
	}
	return int(minRequired), true
}

// SetHeader updates the response header fields written by Finish().
func (b *CgroupsBuilder) SetHeader(systemdEnabled uint32, generation uint64) {
	b.systemdEnabled = systemdEnabled
	b.generation = generation
}

// EstimateCgroupsMaxItems returns a safe upper bound for the number of
// cgroup items that can fit in a response buffer of size bufSize.
//
// This is an upper bound for builder reservation, not a promise that all of
// those items will fit with arbitrary string lengths.
func EstimateCgroupsMaxItems(bufSize int) uint32 {
	if bufSize <= cgroupsRespHdr {
		return 0
	}

	minAlignedItem := Align8(cgroupsItemHdr + 2)
	return uint32((bufSize - cgroupsRespHdr) / (cgroupsDirEntry + minAlignedItem))
}

// Add adds one cgroup item. Handles offset bookkeeping, NUL termination,
// and alignment.
func (b *CgroupsBuilder) Add(hash, options, enabled uint32, name, path []byte) error {
	if b.itemCount >= b.maxItems {
		return ErrOverflow
	}

	// Align item start to 8 bytes.
	itemStart := Align8(b.dataOffset)

	// Item payload: 32-byte header + name + NUL + path + NUL.
	itemSize := cgroupsItemHdr + len(name) + 1 + len(path) + 1

	if itemStart+itemSize > len(b.buf) {
		return ErrOverflow
	}

	// Zero alignment padding.
	if itemStart > b.dataOffset {
		clear(b.buf[b.dataOffset:itemStart])
	}

	nameOffset := uint32(cgroupsItemHdr)
	pathOffset := uint32(cgroupsItemHdr) + uint32(len(name)) + 1

	// Write item header.
	p := itemStart
	ne.PutUint16(b.buf[p:p+2], 1)   // layout_version
	ne.PutUint16(b.buf[p+2:p+4], 0) // flags
	ne.PutUint32(b.buf[p+4:p+8], hash)
	ne.PutUint32(b.buf[p+8:p+12], options)
	ne.PutUint32(b.buf[p+12:p+16], enabled)
	ne.PutUint32(b.buf[p+16:p+20], nameOffset)
	ne.PutUint32(b.buf[p+20:p+24], uint32(len(name)))
	ne.PutUint32(b.buf[p+24:p+28], pathOffset)
	ne.PutUint32(b.buf[p+28:p+32], uint32(len(path)))

	// Write strings with NUL terminators.
	ns := p + int(nameOffset)
	copy(b.buf[ns:], name)
	b.buf[ns+len(name)] = 0

	ps := p + int(pathOffset)
	copy(b.buf[ps:], path)
	b.buf[ps+len(path)] = 0

	// Write directory entry (absolute offset stored temporarily).
	dirEntry := cgroupsRespHdr + int(b.itemCount)*cgroupsDirEntry
	ne.PutUint32(b.buf[dirEntry:dirEntry+4], uint32(itemStart))
	ne.PutUint32(b.buf[dirEntry+4:dirEntry+8], uint32(itemSize))

	b.dataOffset = itemStart + itemSize
	b.itemCount++
	return nil
}

// Finish finalizes the builder. Returns the total payload size. The buffer
// now contains a complete, decodable cgroups snapshot response payload.
func (b *CgroupsBuilder) Finish() int {
	p := b.buf

	if b.itemCount == 0 {
		ne.PutUint16(p[0:2], 1)
		ne.PutUint16(p[2:4], 0)
		ne.PutUint32(p[4:8], 0)
		ne.PutUint32(p[8:12], b.systemdEnabled)
		ne.PutUint32(p[12:16], 0)
		ne.PutUint64(p[16:24], b.generation)
		return cgroupsRespHdr
	}

	// Where the decoder expects packed data to start.
	finalPackedStart := cgroupsRespHdr + int(b.itemCount)*cgroupsDirEntry

	// Read the first directory entry to find where packed data begins.
	firstItemAbs := int(ne.Uint32(p[cgroupsRespHdr : cgroupsRespHdr+4]))

	packedDataLen := b.dataOffset - firstItemAbs

	if finalPackedStart < firstItemAbs {
		// Shift packed data left.
		copy(p[finalPackedStart:], p[firstItemAbs:firstItemAbs+packedDataLen])
	}

	// Convert directory entries from absolute to relative offsets.
	dirBase := cgroupsRespHdr
	for i := 0; i < int(b.itemCount); i++ {
		entry := dirBase + i*cgroupsDirEntry
		absOff := ne.Uint32(p[entry : entry+4])
		relOff := absOff - uint32(firstItemAbs)
		ne.PutUint32(p[entry:entry+4], relOff)
		// length stays the same.
	}

	// Write snapshot header.
	ne.PutUint16(p[0:2], 1)
	ne.PutUint16(p[2:4], 0)
	ne.PutUint32(p[4:8], b.itemCount)
	ne.PutUint32(p[8:12], b.systemdEnabled)
	ne.PutUint32(p[12:16], 0)
	ne.PutUint64(p[16:24], b.generation)

	return finalPackedStart + packedDataLen
}

// DispatchCgroupsSnapshot decodes request, builds response via handler.
func DispatchCgroupsSnapshot(req []byte, resp []byte, maxItems uint32,
	handler func(*CgroupsRequest, *CgroupsBuilder) bool) (int, bool) {
	request, err := DecodeCgroupsRequest(req)
	if err != nil {
		return 0, false
	}
	minRequired, ok := CgroupsBuilderMinBytes(maxItems)
	if !ok || len(resp) < minRequired {
		return 0, false
	}
	builder := NewCgroupsBuilder(resp, maxItems, 0, 0)
	if !handler(&request, builder) {
		return 0, false
	}
	return builder.Finish(), true
}
