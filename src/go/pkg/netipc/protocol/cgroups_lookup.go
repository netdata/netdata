package protocol

import "bytes"

const (
	CgroupLookupKnown             uint16 = 0
	CgroupLookupUnknownRetryLater uint16 = 1
	CgroupLookupUnknownPermanent  uint16 = 2
	CgroupLookupPayloadExceeded   uint16 = 3
	CgroupLookupOversizedItem     uint16 = 4

	CgroupsLookupReqHdr  = 16
	CgroupsLookupRespHdr = 16
	CgroupsLookupItemHdr = 28

	cgroupsLookupUnknownFixedBytes = CgroupsLookupItemHdr + 1
)

type CgroupsLookupRequestView struct {
	ItemCount   uint32
	packedStart int
	payload     []byte
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
	if status != CgroupLookupKnown && status != CgroupLookupUnknownRetryLater &&
		status != CgroupLookupUnknownPermanent && status != CgroupLookupPayloadExceeded &&
		status != CgroupLookupOversizedItem {
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
	count32, ok := checkedU32Int(count)
	if !ok {
		return 0, ErrOverflow
	}
	packedStart, ok := cgroupsLookupRequestPackedStartForCount(count)
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
	ne.PutUint32(buf[4:8], count32)
	ne.PutUint32(buf[8:12], 0)
	ne.PutUint32(buf[12:16], 0)
	return data, nil
}

func cgroupsLookupRequestPackedStartForCount(count int) (int, bool) {
	if count < 0 || uint64(count) > uint64(^uint32(0)) {
		return 0, false
	}
	dirSize, ok := checkedMulInt(count, LookupDirEntrySize)
	if !ok {
		return 0, false
	}
	return checkedAddInt(CgroupsLookupReqHdr, dirSize)
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
	view := &CgroupsLookupRequestView{ItemCount: itemCount, packedStart: dirEnd, payload: buf}
	for i := uint32(0); i < itemCount; i++ {
		keyWithNul, key, err := view.itemBytes(i)
		if err != nil {
			return nil, err
		}
		if keyWithNul[len(keyWithNul)-1] != 0 {
			return nil, ErrMissingNul
		}
		if bytes.IndexByte(key, 0) >= 0 {
			return nil, ErrBadLayout
		}
	}
	return view, nil
}

func (v *CgroupsLookupRequestView) Item(index uint32) (CStringView, error) {
	item, path, err := v.itemBytes(index)
	if err != nil {
		return CStringView{}, err
	}
	pathLen := len(path)
	if uint64(pathLen) > uint64(^uint32(0)) {
		return CStringView{}, ErrOutOfBounds
	}
	return NewCStringView(item, uint32(pathLen)), nil // #nosec G115 -- bounded above.
}

func (v *CgroupsLookupRequestView) itemBytes(index uint32) ([]byte, []byte, error) {
	if index >= v.ItemCount {
		return nil, nil, ErrOutOfBounds
	}
	dirEnd := v.packedStart
	if dirEnd == 0 {
		var ok bool
		dirEnd, ok = lookupBuilderDataOffset(CgroupsLookupReqHdr, v.ItemCount)
		if !ok {
			return nil, nil, ErrOverflow
		}
	}
	dirOff64 := uint64(index) * uint64(LookupDirEntrySize)
	if dirOff64 > uint64(maxIntValue()) || CgroupsLookupReqHdr > maxIntValue()-int(dirOff64) {
		return nil, nil, ErrOverflow
	}
	base := CgroupsLookupReqHdr + int(dirOff64) // #nosec G115 -- bounded by maxIntValue above.
	if base > len(v.payload)-LookupDirEntrySize {
		return nil, nil, ErrOutOfBounds
	}
	off32 := ne.Uint32(v.payload[base : base+4])
	length32 := ne.Uint32(v.payload[base+4 : base+8])
	if uint64(off32) > uint64(maxIntValue()) || uint64(length32) > uint64(maxIntValue()) {
		return nil, nil, ErrOutOfBounds
	}
	length := int(length32) // #nosec G115 -- bounded by maxIntValue above.
	off := int(off32)       // #nosec G115 -- bounded by maxIntValue above.
	item, err := lookupPayloadSlice(v.payload, dirEnd, off, length)
	if err != nil {
		return nil, nil, err
	}
	if length < 1 {
		return nil, nil, ErrOutOfBounds
	}
	return item, item[:length-1], nil
}

func (v *CgroupsLookupRequestView) payloadExceededItemSize(index uint32) (int, error) {
	if index >= v.ItemCount {
		return 0, ErrOutOfBounds
	}
	base, ok := lookupDirOffset(CgroupsLookupReqHdr, index)
	if !ok {
		return 0, ErrOverflow
	}
	_, length, err := lookupDirEntry(v.payload, base)
	if err != nil {
		return 0, err
	}
	size, ok := checkedAddInt(CgroupsLookupItemHdr, length)
	if ok {
		size, ok = checkedAddInt(size, 1)
	}
	if !ok {
		return 0, ErrOverflow
	}
	return size, nil
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
	for i := uint32(0); i < itemCount; i++ {
		base := CgroupsLookupRespHdr + int(i)*LookupDirEntrySize
		off, length, err := lookupDirEntry(buf, base)
		if err != nil {
			return nil, err
		}
		item, err := lookupPayloadSlice(buf, dirEnd, off, length)
		if err != nil {
			return nil, err
		}
		if err := validateCgroupsLookupItem(item); err != nil {
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

// RawItem returns the validated raw wire item bytes for a response item.
func (v *CgroupsLookupResponseView) RawItem(index uint32) ([]byte, error) {
	return lookupResponseRawItem(v.payload, CgroupsLookupRespHdr, v.ItemCount, index)
}

// EncodeCgroupsLookupRawResponse encodes validated raw CGROUPS_LOOKUP response items.
func EncodeCgroupsLookupRawResponse(items [][]byte, generation uint64, buf []byte) (int, error) {
	return encodeLookupRawResponse(buf, CgroupsLookupRespHdr, generation, items, CgroupsLookupItemHdr, func(item []byte) error {
		return validateCgroupsLookupItem(item)
	})
}

func validateCgroupsLookupItem(item []byte) error {
	if len(item) < CgroupsLookupItemHdr {
		return ErrTruncated
	}
	status := ne.Uint16(item[2:4])
	orchestrator := ne.Uint16(item[4:6])
	pathOff, err := checkedWireU32Int(item, 8)
	if err != nil {
		return err
	}
	pathLen, err := checkedWireU32Int(item, 12)
	if err != nil {
		return err
	}
	nameOff, err := checkedWireU32Int(item, 16)
	if err != nil {
		return err
	}
	nameLen, err := checkedWireU32Int(item, 20)
	if err != nil {
		return err
	}
	labelCount := ne.Uint16(item[24:26])
	if ne.Uint16(item[0:2]) != 1 || ne.Uint16(item[6:8]) != 0 || ne.Uint16(item[26:28]) != 0 {
		return ErrBadLayout
	}
	if err := validateCgroupsLookupSemantics(status, orchestrator, pathLen, nameLen, int(labelCount)); err != nil {
		return err
	}
	if status != CgroupLookupKnown {
		return validateCgroupsLookupUnknownItem(item, pathOff, pathLen, nameOff)
	}
	pathEnd, err := lookupStringInto(item, CgroupsLookupItemHdr, pathOff, pathLen, nil)
	if err != nil {
		return err
	}
	nameEnd, err := lookupStringInto(item, CgroupsLookupItemHdr, nameOff, nameLen, nil)
	if err != nil {
		return err
	}
	if overlap(pathOff, pathEnd, nameOff, nameEnd) {
		return ErrBadLayout
	}
	_, err = validateLabels(item, CgroupsLookupItemHdr, labelCount, max(pathEnd, nameEnd))
	return err
}

func validateCgroupsLookupUnknownItem(item []byte, pathOff, pathLen, nameOff int) error {
	pathEnd, err := lookupStringInto(item, CgroupsLookupItemHdr, pathOff, pathLen, nil)
	if err != nil {
		return err
	}
	nameEnd, err := lookupEmptyStringInto(item, CgroupsLookupItemHdr, nameOff, nil)
	if err != nil {
		return err
	}
	if overlap(pathOff, pathEnd, nameOff, nameEnd) {
		return ErrBadLayout
	}
	if max(pathEnd, nameEnd) != len(item) {
		return ErrBadLayout
	}
	return nil
}

func decodeCgroupsLookupItem(item []byte) (*CgroupsLookupItemView, error) {
	var view CgroupsLookupItemView
	if err := decodeCgroupsLookupItemInto(item, &view); err != nil {
		return nil, err
	}
	return &view, nil
}

func decodeCgroupsLookupItemInto(item []byte, out *CgroupsLookupItemView) error {
	if len(item) < CgroupsLookupItemHdr {
		return ErrTruncated
	}
	status := ne.Uint16(item[2:4])
	orchestrator := ne.Uint16(item[4:6])
	pathOff, err := checkedWireU32Int(item, 8)
	if err != nil {
		return err
	}
	pathLen, err := checkedWireU32Int(item, 12)
	if err != nil {
		return err
	}
	nameOff, err := checkedWireU32Int(item, 16)
	if err != nil {
		return err
	}
	nameLen, err := checkedWireU32Int(item, 20)
	if err != nil {
		return err
	}
	labelCount := ne.Uint16(item[24:26])
	if ne.Uint16(item[0:2]) != 1 || ne.Uint16(item[6:8]) != 0 || ne.Uint16(item[26:28]) != 0 {
		return ErrBadLayout
	}
	if err := validateCgroupsLookupSemantics(status, orchestrator, pathLen, nameLen, int(labelCount)); err != nil {
		return err
	}
	if status != CgroupLookupKnown {
		return decodeCgroupsLookupUnknownItemInto(item, status, pathOff, pathLen, nameOff, out)
	}
	var path CStringView
	pathEnd, err := lookupStringInto(item, CgroupsLookupItemHdr, pathOff, pathLen, viewOut(out, &path))
	if err != nil {
		return err
	}
	var name CStringView
	nameEnd, err := lookupStringInto(item, CgroupsLookupItemHdr, nameOff, nameLen, viewOut(out, &name))
	if err != nil {
		return err
	}
	if overlap(pathOff, pathEnd, nameOff, nameEnd) {
		return ErrBadLayout
	}
	table, err := validateLabels(item, CgroupsLookupItemHdr, labelCount, max(pathEnd, nameEnd))
	if err != nil {
		return err
	}
	if out != nil {
		*out = CgroupsLookupItemView{
			Status:           status,
			Orchestrator:     orchestrator,
			Path:             path,
			Name:             name,
			LabelCount:       labelCount,
			item:             item,
			labelTableOffset: table,
		}
	}
	return nil
}

func decodeCgroupsLookupUnknownItemInto(item []byte, status uint16, pathOff, pathLen, nameOff int, out *CgroupsLookupItemView) error {
	var path CStringView
	pathEnd, err := lookupStringInto(item, CgroupsLookupItemHdr, pathOff, pathLen, viewOut(out, &path))
	if err != nil {
		return err
	}
	var name CStringView
	nameEnd, err := lookupEmptyStringInto(item, CgroupsLookupItemHdr, nameOff, viewOut(out, &name))
	if err != nil {
		return err
	}
	if overlap(pathOff, pathEnd, nameOff, nameEnd) {
		return ErrBadLayout
	}
	labelTableOffset := max(pathEnd, nameEnd)
	if labelTableOffset != len(item) {
		return ErrBadLayout
	}
	if out != nil {
		*out = CgroupsLookupItemView{
			Status:           status,
			Path:             path,
			Name:             name,
			item:             item,
			labelTableOffset: labelTableOffset,
		}
	}
	return nil
}

func (v *CgroupsLookupItemView) Label(index uint32) (LookupLabelView, error) {
	return lookupLabelAt(v.item, CgroupsLookupItemHdr, v.LabelCount, v.labelTableOffset, index)
}

type CgroupsLookupBuilder struct {
	buf                   []byte
	generation            uint64
	itemCount             uint32
	maxItems              uint32
	dataOffset            int
	err                   error
	payloadExceededSuffix bool
	payloadExceededBytes  []uint32
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

func (b *CgroupsLookupBuilder) SetPayloadExceededItemLens(itemLens []int) {
	suffixBytes, ok := buildPayloadExceededSuffixBytes(itemLens)
	if !ok {
		b.err = ErrOverflow
		return
	}
	b.payloadExceededBytes = suffixBytes
}

func (b *CgroupsLookupBuilder) Add(status, orchestrator uint16, path, name []byte, labels []struct{ Key, Value []byte }) error {
	return b.add(status, orchestrator, path, name, labels, false)
}

// AddRequestItem appends a response item using a path from a decoded request.
// The request path has already been validated by DecodeCgroupsLookupRequest.
func (b *CgroupsLookupBuilder) AddRequestItem(req *CgroupsLookupRequestView, index uint32, status, orchestrator uint16, name []byte, labels []struct{ Key, Value []byte }) error {
	if req == nil {
		b.err = ErrBadLayout
		return ErrBadLayout
	}
	_, path, err := req.itemBytes(index)
	if err != nil {
		b.err = err
		return err
	}
	return b.add(status, orchestrator, path, name, labels, true)
}

func (b *CgroupsLookupBuilder) add(status, orchestrator uint16, path, name []byte, labels []struct{ Key, Value []byte }, pathValidated bool) error {
	if b.payloadExceededSuffix {
		return b.addUnknown(CgroupLookupPayloadExceeded, path)
	}
	if b.itemCount >= b.maxItems {
		b.err = ErrOverflow
		return ErrOverflow
	}
	if err := validateCgroupsLookupSemantics(status, orchestrator, len(path), len(name), len(labels)); err != nil {
		b.err = err
		return err
	}
	if (!pathValidated && invalidSourceString(path, true)) || invalidSourceString(name, false) {
		b.err = ErrBadLayout
		return ErrBadLayout
	}
	if status != CgroupLookupKnown {
		if err := b.addUnknown(status, path); err != nil {
			if err == ErrOverflow {
				return b.noteItemOverflow(path)
			}
			return err
		}
		return nil
	}
	labelCount, ok := checkedU16Int(len(labels))
	if !ok {
		return b.noteItemOverflow(path)
	}
	itemStart, ok := checkedAlign8(b.dataOffset)
	if !ok {
		return b.noteItemOverflow(path)
	}
	pathOff := CgroupsLookupItemHdr
	nameOff, ok := checkedAddInt(pathOff, len(path))
	if ok {
		nameOff, ok = checkedAddInt(nameOff, 1)
	}
	if !ok {
		return b.noteItemOverflow(path)
	}
	fixedEnd, ok := checkedAddInt(nameOff, len(name))
	if ok {
		fixedEnd, ok = checkedAddInt(fixedEnd, 1)
	}
	if !ok {
		return b.noteItemOverflow(path)
	}
	tableStart, tableBytes, itemSize, err := labelLayoutGo(fixedEnd, labels)
	if err != nil {
		if err == ErrOverflow {
			return b.noteItemOverflow(path)
		}
		b.err = err
		return err
	}
	itemEnd, ok := checkedAddInt(itemStart, itemSize)
	if !ok || itemEnd > len(b.buf) {
		return b.noteItemOverflow(path)
	}
	if !payloadExceededSuffixFits(len(b.buf), itemEnd, b.payloadExceededBytes, b.itemCount+1, b.maxItems) {
		return b.noteItemOverflow(path)
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
	b.err = nil
	return nil
}

func (b *CgroupsLookupBuilder) noteItemOverflow(path []byte) error {
	if b.itemCount == 0 {
		return b.addUnknown(CgroupLookupOversizedItem, path)
	}
	b.payloadExceededSuffix = true
	return b.addUnknown(CgroupLookupPayloadExceeded, path)
}

func (b *CgroupsLookupBuilder) addUnknown(status uint16, path []byte) error {
	itemStart, ok := checkedAlign8(b.dataOffset)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	nameOff, ok := checkedAddInt(CgroupsLookupItemHdr, len(path))
	if ok {
		nameOff, ok = checkedAddInt(nameOff, 1)
	}
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	itemSize, ok := checkedAddInt(nameOff, 1)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
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
	ne.PutUint16(item[4:6], 0)
	ne.PutUint16(item[6:8], 0)
	ne.PutUint32(item[8:12], CgroupsLookupItemHdr)
	ne.PutUint32(item[12:16], pathLen32)
	ne.PutUint32(item[16:20], nameOff32)
	ne.PutUint32(item[20:24], 0)
	ne.PutUint16(item[24:26], 0)
	ne.PutUint16(item[26:28], 0)
	copy(item[CgroupsLookupItemHdr:], path)
	item[CgroupsLookupItemHdr+len(path)] = 0
	item[nameOff] = 0
	dir, ok := lookupDirOffset(CgroupsLookupRespHdr, b.itemCount)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	ne.PutUint32(b.buf[dir:dir+4], itemStart32)
	ne.PutUint32(b.buf[dir+4:dir+8], itemSize32)
	b.dataOffset = itemStart + itemSize
	b.itemCount++
	b.err = nil
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
	if request.ItemCount > 0 {
		suffixBytes, ok := makePayloadExceededSuffixBytes(request.ItemCount)
		if !ok {
			return 0, ErrOverflow
		}
		for i := 0; i < len(suffixBytes)-1; i++ {
			size, err := request.payloadExceededItemSize(uint32(i)) // #nosec G115 -- i is bounded by the checked suffixBytes length derived from ItemCount.
			if err != nil {
				return 0, err
			}
			size32, ok := checkedU32Int(size)
			if !ok {
				return 0, ErrOverflow
			}
			suffixBytes[i] = size32
		}
		if !finishPayloadExceededSuffixBytes(suffixBytes) {
			return 0, ErrOverflow
		}
		builder.payloadExceededBytes = suffixBytes
	}
	if !handler(request, builder) {
		if builder.Error() != nil {
			return 0, builder.Error()
		}
		return 0, ErrHandlerFailed
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
