package protocol

import "bytes"

const (
	CgroupLookupKnown             uint16 = 0
	CgroupLookupUnknownRetryLater uint16 = 1
	CgroupLookupUnknownPermanent  uint16 = 2

	CgroupsLookupReqHdr  = 16
	CgroupsLookupRespHdr = 16
	CgroupsLookupItemHdr = 28
)

type CgroupsLookupRequestView struct {
	ItemCount uint32
	payload   []byte
}

type CgroupsLookupResponseView struct {
	LayoutVersion uint16
	Flags         uint16
	ItemCount     uint32
	Generation    uint64
	payload       []byte
}

type CgroupsLookupItemView struct {
	Status           uint16
	Orchestrator     uint16
	Path             CStringView
	Name             CStringView
	LabelCount       uint16
	item             []byte
	labelTableOffset int
}

func validateCgroupsLookupSemantics(status, orchestrator uint16, pathLen, nameLen, labelCount int) error {
	if status != CgroupLookupKnown && status != CgroupLookupUnknownRetryLater && status != CgroupLookupUnknownPermanent {
		return ErrBadLayout
	}
	if pathLen == 0 {
		return ErrBadLayout
	}
	if status != CgroupLookupKnown && (orchestrator != 0 || nameLen != 0 || labelCount != 0) {
		return ErrBadLayout
	}
	return nil
}

func EncodeCgroupsLookupRequest(paths [][]byte, buf []byte) (int, error) {
	count := len(paths)
	if uint64(count) > uint64(^uint32(0)) {
		return 0, ErrOverflow
	}
	dirSize, ok := checkedMulInt(count, LookupDirEntrySize)
	if !ok {
		return 0, ErrOverflow
	}
	packedStart, ok := checkedAddInt(CgroupsLookupReqHdr, dirSize)
	if !ok {
		return 0, ErrOverflow
	}
	if len(buf) < packedStart {
		return 0, ErrOverflow
	}
	data := packedStart
	for i, path := range paths {
		if invalidSourceString(path, true) {
			return 0, ErrBadLayout
		}
		aligned, ok := checkedAlign8(data)
		if !ok {
			return 0, ErrOverflow
		}
		keyLen, ok := checkedAddInt(len(path), 1)
		if !ok {
			return 0, ErrOverflow
		}
		end, ok := checkedAddInt(aligned, keyLen)
		if !ok {
			return 0, ErrOverflow
		}
		if end > len(buf) {
			return 0, ErrOverflow
		}
		clear(buf[data:aligned])
		offset32, ok := checkedU32Int(aligned - packedStart)
		if !ok {
			return 0, ErrOverflow
		}
		keyLen32, ok := checkedU32Int(keyLen)
		if !ok {
			return 0, ErrOverflow
		}
		base := CgroupsLookupReqHdr + i*LookupDirEntrySize
		ne.PutUint32(buf[base:base+4], offset32)
		ne.PutUint32(buf[base+4:base+8], keyLen32)
		copy(buf[aligned:], path)
		buf[aligned+len(path)] = 0
		data = end
	}
	ne.PutUint16(buf[0:2], 1)
	ne.PutUint16(buf[2:4], 0)
	ne.PutUint32(buf[4:8], uint32(count))
	ne.PutUint32(buf[8:12], 0)
	ne.PutUint32(buf[12:16], 0)
	return data, nil
}

func DecodeCgroupsLookupRequest(buf []byte) (*CgroupsLookupRequestView, error) {
	if len(buf) < CgroupsLookupReqHdr {
		return nil, ErrTruncated
	}
	if ne.Uint16(buf[0:2]) != 1 || ne.Uint16(buf[2:4]) != 0 ||
		ne.Uint32(buf[8:12]) != 0 || ne.Uint32(buf[12:16]) != 0 {
		return nil, ErrBadLayout
	}
	itemCount := ne.Uint32(buf[4:8])
	dirSize64 := uint64(itemCount) * uint64(LookupDirEntrySize)
	dirEnd, ok := checkedInt(uint64(CgroupsLookupReqHdr) + dirSize64)
	if !ok {
		return nil, ErrBadItemCount
	}
	if dirEnd > len(buf) {
		return nil, ErrTruncated
	}
	if err := validateLookupDir(buf, CgroupsLookupReqHdr, itemCount, len(buf)-dirEnd, 2, -1); err != nil {
		return nil, err
	}
	for i := range itemCount {
		base := CgroupsLookupReqHdr + int(i)*LookupDirEntrySize
		off, length, err := lookupDirEntry(buf, base)
		if err != nil {
			return nil, err
		}
		key, err := lookupPayloadSlice(buf, dirEnd, off, length)
		if err != nil {
			return nil, err
		}
		if key[length-1] != 0 {
			return nil, ErrMissingNul
		}
		if bytes.Contains(key[:length-1], []byte{0}) {
			return nil, ErrBadLayout
		}
	}
	return &CgroupsLookupRequestView{ItemCount: itemCount, payload: buf}, nil
}

func (v *CgroupsLookupRequestView) Item(index uint32) (CStringView, error) {
	if index >= v.ItemCount {
		return CStringView{}, ErrOutOfBounds
	}
	dirEnd, ok := lookupBuilderDataOffset(CgroupsLookupReqHdr, v.ItemCount)
	if !ok {
		return CStringView{}, ErrOverflow
	}
	base, ok := lookupDirOffset(CgroupsLookupReqHdr, index)
	if !ok {
		return CStringView{}, ErrOverflow
	}
	off, length, err := lookupDirEntry(v.payload, base)
	if err != nil {
		return CStringView{}, err
	}
	item, err := lookupPayloadSlice(v.payload, dirEnd, off, length)
	if err != nil {
		return CStringView{}, err
	}
	stringLen, ok := checkedU32Int(length - 1)
	if !ok {
		return CStringView{}, ErrOutOfBounds
	}
	return NewCStringView(item, stringLen), nil
}

func DecodeCgroupsLookupResponse(buf []byte) (*CgroupsLookupResponseView, error) {
	if len(buf) < CgroupsLookupRespHdr {
		return nil, ErrTruncated
	}
	if ne.Uint16(buf[0:2]) != 1 || ne.Uint16(buf[2:4]) != 0 {
		return nil, ErrBadLayout
	}
	itemCount := ne.Uint32(buf[4:8])
	dirEnd, ok := checkedInt(uint64(CgroupsLookupRespHdr) + uint64(itemCount)*uint64(LookupDirEntrySize))
	if !ok {
		return nil, ErrBadItemCount
	}
	if dirEnd > len(buf) {
		return nil, ErrTruncated
	}
	if err := validateLookupDir(buf, CgroupsLookupRespHdr, itemCount, len(buf)-dirEnd, CgroupsLookupItemHdr, -1); err != nil {
		return nil, err
	}
	for i := range itemCount {
		base := CgroupsLookupRespHdr + int(i)*LookupDirEntrySize
		off, length, err := lookupDirEntry(buf, base)
		if err != nil {
			return nil, err
		}
		item, err := lookupPayloadSlice(buf, dirEnd, off, length)
		if err != nil {
			return nil, err
		}
		if _, err := decodeCgroupsLookupItem(item); err != nil {
			return nil, err
		}
	}
	return &CgroupsLookupResponseView{
		LayoutVersion: 1,
		Flags:         0,
		ItemCount:     itemCount,
		Generation:    ne.Uint64(buf[8:16]),
		payload:       buf,
	}, nil
}

func (v *CgroupsLookupResponseView) Item(index uint32) (*CgroupsLookupItemView, error) {
	if index >= v.ItemCount {
		return nil, ErrOutOfBounds
	}
	dirEnd, ok := lookupBuilderDataOffset(CgroupsLookupRespHdr, v.ItemCount)
	if !ok {
		return nil, ErrOverflow
	}
	base, ok := lookupDirOffset(CgroupsLookupRespHdr, index)
	if !ok {
		return nil, ErrOverflow
	}
	off, length, err := lookupDirEntry(v.payload, base)
	if err != nil {
		return nil, err
	}
	item, err := lookupPayloadSlice(v.payload, dirEnd, off, length)
	if err != nil {
		return nil, err
	}
	return decodeCgroupsLookupItem(item)
}

func decodeCgroupsLookupItem(item []byte) (*CgroupsLookupItemView, error) {
	if len(item) < CgroupsLookupItemHdr {
		return nil, ErrTruncated
	}
	status := ne.Uint16(item[2:4])
	orchestrator := ne.Uint16(item[4:6])
	pathOff, err := checkedWireU32Int(item, 8)
	if err != nil {
		return nil, err
	}
	pathLen, err := checkedWireU32Int(item, 12)
	if err != nil {
		return nil, err
	}
	nameOff, err := checkedWireU32Int(item, 16)
	if err != nil {
		return nil, err
	}
	nameLen, err := checkedWireU32Int(item, 20)
	if err != nil {
		return nil, err
	}
	labelCount := ne.Uint16(item[24:26])
	if ne.Uint16(item[0:2]) != 1 || ne.Uint16(item[6:8]) != 0 || ne.Uint16(item[26:28]) != 0 {
		return nil, ErrBadLayout
	}
	if err := validateCgroupsLookupSemantics(status, orchestrator, pathLen, nameLen, int(labelCount)); err != nil {
		return nil, err
	}
	path, pathEnd, err := lookupString(item, CgroupsLookupItemHdr, pathOff, pathLen)
	if err != nil {
		return nil, err
	}
	name, nameEnd, err := lookupString(item, CgroupsLookupItemHdr, nameOff, nameLen)
	if err != nil {
		return nil, err
	}
	if overlap(pathOff, pathEnd, nameOff, nameEnd) {
		return nil, ErrBadLayout
	}
	table, err := validateLabels(item, CgroupsLookupItemHdr, labelCount, max(pathEnd, nameEnd))
	if err != nil {
		return nil, err
	}
	return &CgroupsLookupItemView{
		Status:           status,
		Orchestrator:     orchestrator,
		Path:             path,
		Name:             name,
		LabelCount:       labelCount,
		item:             item,
		labelTableOffset: table,
	}, nil
}

func (v *CgroupsLookupItemView) Label(index uint32) (LookupLabelView, error) {
	return lookupLabelAt(v.item, CgroupsLookupItemHdr, v.LabelCount, v.labelTableOffset, index)
}

type CgroupsLookupBuilder struct {
	buf        []byte
	generation uint64
	itemCount  uint32
	maxItems   uint32
	dataOffset int
	err        error
}

func NewCgroupsLookupBuilder(buf []byte, maxItems uint32, generation uint64) *CgroupsLookupBuilder {
	minRequired, ok := lookupBuilderDataOffset(CgroupsLookupRespHdr, maxItems)
	if !ok {
		panic("CgroupsLookupBuilder buffer too small")
	}
	if len(buf) < minRequired {
		panic("CgroupsLookupBuilder buffer too small")
	}
	return &CgroupsLookupBuilder{buf: buf, generation: generation, maxItems: maxItems, dataOffset: minRequired}
}

func (b *CgroupsLookupBuilder) SetGeneration(generation uint64) {
	b.generation = generation
}

func (b *CgroupsLookupBuilder) Add(status, orchestrator uint16, path, name []byte, labels []struct{ Key, Value []byte }) error {
	if b.itemCount >= b.maxItems {
		b.err = ErrOverflow
		return ErrOverflow
	}
	if err := validateCgroupsLookupSemantics(status, orchestrator, len(path), len(name), len(labels)); err != nil {
		b.err = err
		return err
	}
	if invalidSourceString(path, true) || invalidSourceString(name, false) {
		b.err = ErrBadLayout
		return ErrBadLayout
	}
	labelCount, ok := checkedU16Int(len(labels))
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	itemStart, ok := checkedAlign8(b.dataOffset)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	pathOff := CgroupsLookupItemHdr
	nameOff, ok := checkedAddInt(pathOff, len(path))
	if ok {
		nameOff, ok = checkedAddInt(nameOff, 1)
	}
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	fixedEnd, ok := checkedAddInt(nameOff, len(name))
	if ok {
		fixedEnd, ok = checkedAddInt(fixedEnd, 1)
	}
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	tableStart, tableBytes, itemSize, err := labelLayoutGo(fixedEnd, labels)
	if err != nil {
		b.err = err
		return err
	}
	itemEnd, ok := checkedAddInt(itemStart, itemSize)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	if itemEnd > len(b.buf) {
		b.err = ErrOverflow
		return ErrOverflow
	}
	pathOff32, ok := checkedU32Int(pathOff)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	pathLen32, ok := checkedU32Int(len(path))
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	nameOff32, ok := checkedU32Int(nameOff)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	nameLen32, ok := checkedU32Int(len(name))
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	itemStart32, ok := checkedU32Int(itemStart)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	itemSize32, ok := checkedU32Int(itemSize)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	clear(b.buf[b.dataOffset:itemStart])
	item := b.buf[itemStart:itemEnd]
	ne.PutUint16(item[0:2], 1)
	ne.PutUint16(item[2:4], status)
	ne.PutUint16(item[4:6], orchestrator)
	ne.PutUint16(item[6:8], 0)
	ne.PutUint32(item[8:12], pathOff32)
	ne.PutUint32(item[12:16], pathLen32)
	ne.PutUint32(item[16:20], nameOff32)
	ne.PutUint32(item[20:24], nameLen32)
	ne.PutUint16(item[24:26], labelCount)
	ne.PutUint16(item[26:28], 0)
	copy(item[pathOff:], path)
	item[pathOff+len(path)] = 0
	copy(item[nameOff:], name)
	item[nameOff+len(name)] = 0
	if len(labels) > 0 {
		clear(item[fixedEnd:tableStart])
		next, err := writeLookupLabels(item, tableStart, tableBytes, labels)
		if err != nil {
			b.err = err
			return err
		}
		itemSize = next
	}
	dir, ok := lookupDirOffset(CgroupsLookupRespHdr, b.itemCount)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	ne.PutUint32(b.buf[dir:dir+4], itemStart32)
	ne.PutUint32(b.buf[dir+4:dir+8], itemSize32)
	b.dataOffset = itemStart + itemSize
	b.itemCount++
	return nil
}

func (b *CgroupsLookupBuilder) Finish() int {
	return finishLookupResponse(b.buf, CgroupsLookupRespHdr, b.itemCount, b.dataOffset, b.generation)
}

func (b *CgroupsLookupBuilder) Error() error {
	return b.err
}

func (b *CgroupsLookupBuilder) ItemCount() uint32 {
	return b.itemCount
}

func DispatchCgroupsLookup(req []byte, resp []byte, handler func(*CgroupsLookupRequestView, *CgroupsLookupBuilder) bool) (int, error) {
	request, err := DecodeCgroupsLookupRequest(req)
	if err != nil {
		return 0, err
	}
	minRequired, ok := lookupBuilderDataOffset(CgroupsLookupRespHdr, request.ItemCount)
	if !ok || len(resp) < minRequired {
		return 0, ErrOverflow
	}
	builder := NewCgroupsLookupBuilder(resp, request.ItemCount, 0)
	if !handler(request, builder) {
		if builder.Error() != nil {
			return 0, builder.Error()
		}
		return 0, ErrBadLayout
	}
	if builder.Error() != nil {
		return 0, builder.Error()
	}
	if builder.itemCount != request.ItemCount {
		return 0, ErrBadItemCount
	}
	n := builder.Finish()
	if n == 0 {
		return 0, ErrOverflow
	}
	return n, nil
}
