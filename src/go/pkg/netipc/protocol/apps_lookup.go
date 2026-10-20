package protocol

const (
	NipcUIDUnset uint32 = ^uint32(0)

	PidLookupKnown           uint16 = 0
	PidLookupUnknown         uint16 = 1
	PidLookupPayloadExceeded uint16 = 2
	PidLookupOversizedItem   uint16 = 3

	AppsCgroupKnown             uint16 = 0
	AppsCgroupUnknownRetryLater uint16 = 1
	AppsCgroupUnknownPermanent  uint16 = 2
	AppsCgroupHostRoot          uint16 = 3

	AppsLookupReqHdr  = 16
	AppsLookupRespHdr = 16
	AppsLookupItemHdr = 60
	AppsLookupKeySize = 8

	appsLookupUnknownItemSize = AppsLookupItemHdr + 3
)

type AppsLookupRequestView struct {
	ItemCount   uint32
	packedStart int
	payload     []byte
}

type AppsLookupResponseView struct {
	LayoutVersion uint16
	Flags         uint16
	ItemCount     uint32
	Generation    uint64
	payload       []byte
}

type AppsLookupItemView struct {
	Status           uint16
	Orchestrator     uint16
	CgroupStatus     uint16
	Pid              uint32
	Ppid             uint32
	Uid              uint32
	Starttime        uint64
	Comm             CStringView
	CgroupPath       CStringView
	CgroupName       CStringView
	LabelCount       uint16
	item             []byte
	labelTableOffset int
}

type appsLookupSemantics struct {
	status       uint16
	cgroupStatus uint16
	orchestrator uint16
	ppid         uint32
	uid          uint32
	starttime    uint64
	commLen      int
	pathLen      int
	nameLen      int
	labelCount   int
}

func validateAppsLookupSemantics(v appsLookupSemantics) error {
	if v.commLen > 15 {
		return ErrBadLayout
	}
	switch v.status {
	case PidLookupKnown:
		return validateAppsLookupKnown(v)
	case PidLookupUnknown, PidLookupPayloadExceeded, PidLookupOversizedItem:
		return validateAppsLookupUnknown(v)
	default:
		return ErrBadLayout
	}
}

func validateAppsLookupUnknown(v appsLookupSemantics) error {
	if v.orchestrator != 0 || v.cgroupStatus != 0 || v.ppid != 0 || v.uid != NipcUIDUnset ||
		v.starttime != 0 || v.commLen != 0 || v.pathLen != 0 || v.nameLen != 0 || v.labelCount != 0 {
		return ErrBadLayout
	}
	return nil
}

func validateAppsLookupKnown(v appsLookupSemantics) error {
	if v.commLen == 0 {
		return ErrBadLayout
	}
	switch v.cgroupStatus {
	case AppsCgroupKnown:
		if v.pathLen == 0 {
			return ErrBadLayout
		}
	case AppsCgroupUnknownRetryLater:
		if v.orchestrator != 0 || v.nameLen != 0 || v.labelCount != 0 {
			return ErrBadLayout
		}
	case AppsCgroupUnknownPermanent:
		if v.pathLen == 0 || v.orchestrator != 0 || v.nameLen != 0 || v.labelCount != 0 {
			return ErrBadLayout
		}
	case AppsCgroupHostRoot:
		if v.orchestrator != 0 || v.pathLen != 0 || v.nameLen != 0 || v.labelCount != 0 {
			return ErrBadLayout
		}
	default:
		return ErrBadLayout
	}
	return nil
}

func EncodeAppsLookupRequest(pids []uint32, buf []byte) (int, error) {
	count := len(pids)
	count32, ok := checkedU32Int(count)
	if !ok {
		return 0, ErrOverflow
	}
	packedStart, total, ok := appsLookupRequestLayoutForCount(count)
	if !ok {
		return 0, ErrOverflow
	}
	if total > len(buf) {
		return 0, ErrOverflow
	}
	for i, pid := range pids {
		offset32, ok := checkedU32Int(i * AppsLookupKeySize)
		if !ok {
			return 0, ErrOverflow
		}
		base := AppsLookupReqHdr + i*LookupDirEntrySize
		ne.PutUint32(buf[base:base+4], offset32)
		ne.PutUint32(buf[base+4:base+8], AppsLookupKeySize)
		key := packedStart + i*AppsLookupKeySize
		ne.PutUint32(buf[key:key+4], pid)
		ne.PutUint32(buf[key+4:key+8], 0)
	}
	ne.PutUint16(buf[0:2], 1)
	ne.PutUint16(buf[2:4], 0)
	ne.PutUint32(buf[4:8], count32)
	ne.PutUint32(buf[8:12], 0)
	ne.PutUint32(buf[12:16], 0)
	return total, nil
}

func appsLookupRequestLayoutForCount(count int) (packedStart, total int, ok bool) {
	if count < 0 || uint64(count) > uint64(^uint32(0)) {
		return 0, 0, false
	}
	dirSize, ok := checkedMulInt(count, LookupDirEntrySize)
	if !ok {
		return 0, 0, false
	}
	keySize, ok := checkedMulInt(count, AppsLookupKeySize)
	if !ok {
		return 0, 0, false
	}
	packedStart, ok = checkedAddInt(AppsLookupReqHdr, dirSize)
	if !ok {
		return 0, 0, false
	}
	total, ok = checkedAddInt(packedStart, keySize)
	if !ok {
		return 0, 0, false
	}
	return packedStart, total, true
}

func DecodeAppsLookupRequest(buf []byte) (*AppsLookupRequestView, error) {
	if len(buf) < AppsLookupReqHdr {
		return nil, ErrTruncated
	}
	if ne.Uint16(buf[0:2]) != 1 || ne.Uint16(buf[2:4]) != 0 ||
		ne.Uint32(buf[8:12]) != 0 || ne.Uint32(buf[12:16]) != 0 {
		return nil, ErrBadLayout
	}
	itemCount := ne.Uint32(buf[4:8])
	dirSize64 := uint64(itemCount) * uint64(LookupDirEntrySize)
	dirEnd, ok := checkedInt(uint64(AppsLookupReqHdr) + dirSize64)
	if !ok {
		return nil, ErrBadItemCount
	}
	if dirEnd > len(buf) {
		return nil, ErrTruncated
	}
	if err := validateLookupDir(buf, AppsLookupReqHdr, itemCount, len(buf)-dirEnd, 0, AppsLookupKeySize); err != nil {
		return nil, err
	}
	for i := range itemCount {
		base := AppsLookupReqHdr + int(i)*LookupDirEntrySize
		off, _, err := lookupDirEntry(buf, base)
		if err != nil {
			return nil, err
		}
		key, err := lookupPayloadSlice(buf, dirEnd, off, AppsLookupKeySize)
		if err != nil {
			return nil, err
		}
		if ne.Uint32(key[4:8]) != 0 {
			return nil, ErrBadLayout
		}
	}
	return &AppsLookupRequestView{ItemCount: itemCount, packedStart: dirEnd, payload: buf}, nil
}

func (v *AppsLookupRequestView) Item(index uint32) (uint32, error) {
	if index >= v.ItemCount {
		return 0, ErrOutOfBounds
	}
	dirEnd := v.packedStart
	if dirEnd == 0 {
		var ok bool
		dirEnd, ok = lookupBuilderDataOffset(AppsLookupReqHdr, v.ItemCount)
		if !ok {
			return 0, ErrOverflow
		}
	}
	base, ok := lookupDirOffset(AppsLookupReqHdr, index)
	if !ok {
		return 0, ErrOverflow
	}
	off, _, err := lookupDirEntry(v.payload, base)
	if err != nil {
		return 0, err
	}
	key, err := lookupPayloadSlice(v.payload, dirEnd, off, AppsLookupKeySize)
	if err != nil {
		return 0, err
	}
	return ne.Uint32(key[0:4]), nil
}

func DecodeAppsLookupResponse(buf []byte) (*AppsLookupResponseView, error) {
	if len(buf) < AppsLookupRespHdr {
		return nil, ErrTruncated
	}
	if ne.Uint16(buf[0:2]) != 1 || ne.Uint16(buf[2:4]) != 0 {
		return nil, ErrBadLayout
	}
	itemCount := ne.Uint32(buf[4:8])
	dirEnd, ok := checkedInt(uint64(AppsLookupRespHdr) + uint64(itemCount)*uint64(LookupDirEntrySize))
	if !ok {
		return nil, ErrBadItemCount
	}
	if dirEnd > len(buf) {
		return nil, ErrTruncated
	}
	if err := validateLookupDir(buf, AppsLookupRespHdr, itemCount, len(buf)-dirEnd, AppsLookupItemHdr, -1); err != nil {
		return nil, err
	}
	for i := range itemCount {
		base := AppsLookupRespHdr + int(i)*LookupDirEntrySize
		off, length, err := lookupDirEntry(buf, base)
		if err != nil {
			return nil, err
		}
		item, err := lookupPayloadSlice(buf, dirEnd, off, length)
		if err != nil {
			return nil, err
		}
		if err := validateAppsLookupItem(item); err != nil {
			return nil, err
		}
	}
	return &AppsLookupResponseView{
		LayoutVersion: 1,
		Flags:         0,
		ItemCount:     itemCount,
		Generation:    ne.Uint64(buf[8:16]),
		payload:       buf,
	}, nil
}

func (v *AppsLookupResponseView) Item(index uint32) (*AppsLookupItemView, error) {
	if index >= v.ItemCount {
		return nil, ErrOutOfBounds
	}
	dirEnd, ok := lookupBuilderDataOffset(AppsLookupRespHdr, v.ItemCount)
	if !ok {
		return nil, ErrOverflow
	}
	base, ok := lookupDirOffset(AppsLookupRespHdr, index)
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
	return decodeAppsLookupItem(item)
}

// RawItem returns the validated raw wire item bytes for a response item.
func (v *AppsLookupResponseView) RawItem(index uint32) ([]byte, error) {
	return lookupResponseRawItem(v.payload, AppsLookupRespHdr, v.ItemCount, index)
}

// EncodeAppsLookupRawResponse encodes validated raw APPS_LOOKUP response items.
func EncodeAppsLookupRawResponse(items [][]byte, generation uint64, buf []byte) (int, error) {
	return encodeLookupRawResponse(buf, AppsLookupRespHdr, generation, items, AppsLookupItemHdr, func(item []byte) error {
		return validateAppsLookupItem(item)
	})
}

func validateAppsLookupItem(item []byte) error {
	if len(item) < AppsLookupItemHdr {
		return ErrTruncated
	}
	status := ne.Uint16(item[2:4])
	orchestrator := ne.Uint16(item[4:6])
	cgroupStatus := ne.Uint16(item[6:8])
	ppid := ne.Uint32(item[12:16])
	uid := ne.Uint32(item[16:20])
	starttime := ne.Uint64(item[24:32])
	commOff, err := checkedWireU32Int(item, 32)
	if err != nil {
		return err
	}
	commLen, err := checkedWireU32Int(item, 36)
	if err != nil {
		return err
	}
	pathOff, err := checkedWireU32Int(item, 40)
	if err != nil {
		return err
	}
	pathLen, err := checkedWireU32Int(item, 44)
	if err != nil {
		return err
	}
	nameOff, err := checkedWireU32Int(item, 48)
	if err != nil {
		return err
	}
	nameLen, err := checkedWireU32Int(item, 52)
	if err != nil {
		return err
	}
	labelCount := ne.Uint16(item[56:58])
	if ne.Uint16(item[0:2]) != 1 || ne.Uint32(item[20:24]) != 0 || ne.Uint16(item[58:60]) != 0 {
		return ErrBadLayout
	}
	if err := validateAppsLookupSemantics(appsLookupSemantics{
		status:       status,
		cgroupStatus: cgroupStatus,
		orchestrator: orchestrator,
		ppid:         ppid,
		uid:          uid,
		starttime:    starttime,
		commLen:      commLen,
		pathLen:      pathLen,
		nameLen:      nameLen,
		labelCount:   int(labelCount),
	}); err != nil {
		return err
	}
	if status != PidLookupKnown {
		return validateAppsLookupUnknownItem(item, commOff, pathOff, nameOff)
	}
	commEnd, err := lookupStringInto(item, AppsLookupItemHdr, commOff, commLen, nil)
	if err != nil {
		return err
	}
	pathEnd, err := lookupStringInto(item, AppsLookupItemHdr, pathOff, pathLen, nil)
	if err != nil {
		return err
	}
	nameEnd, err := lookupStringInto(item, AppsLookupItemHdr, nameOff, nameLen, nil)
	if err != nil {
		return err
	}
	if overlap(commOff, commEnd, pathOff, pathEnd) || overlap(commOff, commEnd, nameOff, nameEnd) ||
		overlap(pathOff, pathEnd, nameOff, nameEnd) {
		return ErrBadLayout
	}
	_, err = validateLabels(item, AppsLookupItemHdr, labelCount, max(commEnd, max(pathEnd, nameEnd)))
	return err
}

func validateAppsLookupUnknownItem(item []byte, commOff, pathOff, nameOff int) error {
	commEnd, err := lookupEmptyStringInto(item, AppsLookupItemHdr, commOff, nil)
	if err != nil {
		return err
	}
	pathEnd, err := lookupEmptyStringInto(item, AppsLookupItemHdr, pathOff, nil)
	if err != nil {
		return err
	}
	nameEnd, err := lookupEmptyStringInto(item, AppsLookupItemHdr, nameOff, nil)
	if err != nil {
		return err
	}
	if overlap(commOff, commEnd, pathOff, pathEnd) || overlap(commOff, commEnd, nameOff, nameEnd) ||
		overlap(pathOff, pathEnd, nameOff, nameEnd) {
		return ErrBadLayout
	}
	if max(commEnd, max(pathEnd, nameEnd)) != len(item) {
		return ErrBadLayout
	}
	return nil
}

func decodeAppsLookupItem(item []byte) (*AppsLookupItemView, error) {
	var view AppsLookupItemView
	if err := decodeAppsLookupItemInto(item, &view); err != nil {
		return nil, err
	}
	return &view, nil
}

func decodeAppsLookupItemInto(item []byte, out *AppsLookupItemView) error {
	if len(item) < AppsLookupItemHdr {
		return ErrTruncated
	}
	status := ne.Uint16(item[2:4])
	orchestrator := ne.Uint16(item[4:6])
	cgroupStatus := ne.Uint16(item[6:8])
	pid := ne.Uint32(item[8:12])
	ppid := ne.Uint32(item[12:16])
	uid := ne.Uint32(item[16:20])
	starttime := ne.Uint64(item[24:32])
	commOff, err := checkedWireU32Int(item, 32)
	if err != nil {
		return err
	}
	commLen, err := checkedWireU32Int(item, 36)
	if err != nil {
		return err
	}
	pathOff, err := checkedWireU32Int(item, 40)
	if err != nil {
		return err
	}
	pathLen, err := checkedWireU32Int(item, 44)
	if err != nil {
		return err
	}
	nameOff, err := checkedWireU32Int(item, 48)
	if err != nil {
		return err
	}
	nameLen, err := checkedWireU32Int(item, 52)
	if err != nil {
		return err
	}
	labelCount := ne.Uint16(item[56:58])
	if ne.Uint16(item[0:2]) != 1 || ne.Uint32(item[20:24]) != 0 || ne.Uint16(item[58:60]) != 0 {
		return ErrBadLayout
	}
	if err := validateAppsLookupSemantics(appsLookupSemantics{
		status:       status,
		cgroupStatus: cgroupStatus,
		orchestrator: orchestrator,
		ppid:         ppid,
		uid:          uid,
		starttime:    starttime,
		commLen:      commLen,
		pathLen:      pathLen,
		nameLen:      nameLen,
		labelCount:   int(labelCount),
	}); err != nil {
		return err
	}
	if status != PidLookupKnown {
		return decodeAppsLookupUnknownItemInto(item, status, pid, commOff, pathOff, nameOff, out)
	}
	var comm CStringView
	commEnd, err := lookupStringInto(item, AppsLookupItemHdr, commOff, commLen, viewOut(out, &comm))
	if err != nil {
		return err
	}
	var path CStringView
	pathEnd, err := lookupStringInto(item, AppsLookupItemHdr, pathOff, pathLen, viewOut(out, &path))
	if err != nil {
		return err
	}
	var name CStringView
	nameEnd, err := lookupStringInto(item, AppsLookupItemHdr, nameOff, nameLen, viewOut(out, &name))
	if err != nil {
		return err
	}
	if overlap(commOff, commEnd, pathOff, pathEnd) || overlap(commOff, commEnd, nameOff, nameEnd) ||
		overlap(pathOff, pathEnd, nameOff, nameEnd) {
		return ErrBadLayout
	}
	table, err := validateLabels(item, AppsLookupItemHdr, labelCount, max(commEnd, max(pathEnd, nameEnd)))
	if err != nil {
		return err
	}
	if out != nil {
		*out = AppsLookupItemView{
			Status:           status,
			Orchestrator:     orchestrator,
			CgroupStatus:     cgroupStatus,
			Pid:              pid,
			Ppid:             ppid,
			Uid:              uid,
			Starttime:        starttime,
			Comm:             comm,
			CgroupPath:       path,
			CgroupName:       name,
			LabelCount:       labelCount,
			item:             item,
			labelTableOffset: table,
		}
	}
	return nil
}

func decodeAppsLookupUnknownItemInto(item []byte, status uint16, pid uint32, commOff, pathOff, nameOff int, out *AppsLookupItemView) error {
	var comm CStringView
	commEnd, err := lookupEmptyStringInto(item, AppsLookupItemHdr, commOff, viewOut(out, &comm))
	if err != nil {
		return err
	}
	var path CStringView
	pathEnd, err := lookupEmptyStringInto(item, AppsLookupItemHdr, pathOff, viewOut(out, &path))
	if err != nil {
		return err
	}
	var name CStringView
	nameEnd, err := lookupEmptyStringInto(item, AppsLookupItemHdr, nameOff, viewOut(out, &name))
	if err != nil {
		return err
	}
	if overlap(commOff, commEnd, pathOff, pathEnd) || overlap(commOff, commEnd, nameOff, nameEnd) ||
		overlap(pathOff, pathEnd, nameOff, nameEnd) {
		return ErrBadLayout
	}
	labelTableOffset := max(commEnd, max(pathEnd, nameEnd))
	if labelTableOffset != len(item) {
		return ErrBadLayout
	}
	if out != nil {
		*out = AppsLookupItemView{
			Status:           status,
			CgroupStatus:     0,
			Pid:              pid,
			Uid:              NipcUIDUnset,
			Comm:             comm,
			CgroupPath:       path,
			CgroupName:       name,
			item:             item,
			labelTableOffset: labelTableOffset,
		}
	}
	return nil
}

func (v *AppsLookupItemView) Label(index uint32) (LookupLabelView, error) {
	return lookupLabelAt(v.item, AppsLookupItemHdr, v.LabelCount, v.labelTableOffset, index)
}

type AppsLookupBuilder struct {
	buf                   []byte
	generation            uint64
	itemCount             uint32
	maxItems              uint32
	dataOffset            int
	err                   error
	payloadExceededSuffix bool
	payloadExceededBytes  []uint32
	payloadExceededFixed  int
}

func NewAppsLookupBuilder(buf []byte, maxItems uint32, generation uint64) *AppsLookupBuilder {
	minRequired, ok := lookupBuilderDataOffset(AppsLookupRespHdr, maxItems)
	if !ok {
		panic("AppsLookupBuilder buffer too small")
	}
	if len(buf) < minRequired {
		panic("AppsLookupBuilder buffer too small")
	}
	return &AppsLookupBuilder{buf: buf, generation: generation, maxItems: maxItems, dataOffset: minRequired}
}

func (b *AppsLookupBuilder) SetGeneration(generation uint64) {
	b.generation = generation
}

func (b *AppsLookupBuilder) SetPayloadExceededItemLens(itemLens []int) {
	suffixBytes, ok := buildPayloadExceededSuffixBytes(itemLens)
	if !ok {
		b.err = ErrOverflow
		return
	}
	b.payloadExceededBytes = suffixBytes
	b.payloadExceededFixed = 0
}

// Add appends one APPS_LOOKUP wire item; parameters mirror the fixed protocol fields.
func (b *AppsLookupBuilder) Add(status, cgroupStatus, orchestrator uint16, pid, ppid, uid uint32, starttime uint64, comm, cgroupPath, cgroupName []byte, labels []struct{ Key, Value []byte }) error { //NOSONAR
	if b.payloadExceededSuffix {
		return b.addUnknown(PidLookupPayloadExceeded, pid)
	}
	if b.itemCount >= b.maxItems {
		b.err = ErrOverflow
		return ErrOverflow
	}
	if err := validateAppsLookupSemantics(appsLookupSemantics{
		status:       status,
		cgroupStatus: cgroupStatus,
		orchestrator: orchestrator,
		ppid:         ppid,
		uid:          uid,
		starttime:    starttime,
		commLen:      len(comm),
		pathLen:      len(cgroupPath),
		nameLen:      len(cgroupName),
		labelCount:   len(labels),
	}); err != nil {
		b.err = err
		return err
	}
	if status != PidLookupKnown {
		if err := b.addUnknown(status, pid); err != nil {
			if err == ErrOverflow {
				return b.noteItemOverflow(pid)
			}
			return err
		}
		return nil
	}
	if invalidSourceString(comm, status == PidLookupKnown) ||
		invalidSourceString(cgroupPath, false) || invalidSourceString(cgroupName, false) {
		b.err = ErrBadLayout
		return ErrBadLayout
	}
	labelCount, ok := checkedU16Int(len(labels))
	if !ok {
		return b.noteItemOverflow(pid)
	}
	itemStart, ok := checkedAlign8(b.dataOffset)
	if !ok {
		return b.noteItemOverflow(pid)
	}
	commOff := AppsLookupItemHdr
	pathOff, ok := checkedAddInt(commOff, len(comm))
	if ok {
		pathOff, ok = checkedAddInt(pathOff, 1)
	}
	if !ok {
		return b.noteItemOverflow(pid)
	}
	nameOff, ok := checkedAddInt(pathOff, len(cgroupPath))
	if ok {
		nameOff, ok = checkedAddInt(nameOff, 1)
	}
	if !ok {
		return b.noteItemOverflow(pid)
	}
	fixedEnd, ok := checkedAddInt(nameOff, len(cgroupName))
	if ok {
		fixedEnd, ok = checkedAddInt(fixedEnd, 1)
	}
	if !ok {
		return b.noteItemOverflow(pid)
	}
	tableStart, tableBytes, itemSize, err := labelLayoutGo(fixedEnd, labels)
	if err != nil {
		if err == ErrOverflow {
			return b.noteItemOverflow(pid)
		}
		b.err = err
		return err
	}
	itemEnd, ok := checkedAddInt(itemStart, itemSize)
	if !ok || itemEnd > len(b.buf) {
		return b.noteItemOverflow(pid)
	}
	if !b.payloadExceededSuffixFits(itemEnd) {
		return b.noteItemOverflow(pid)
	}
	commOff32, ok := checkedU32Int(commOff)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	commLen32, ok := checkedU32Int(len(comm))
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	pathOff32, ok := checkedU32Int(pathOff)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	pathLen32, ok := checkedU32Int(len(cgroupPath))
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	nameOff32, ok := checkedU32Int(nameOff)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	nameLen32, ok := checkedU32Int(len(cgroupName))
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
	ne.PutUint16(item[6:8], cgroupStatus)
	ne.PutUint32(item[8:12], pid)
	ne.PutUint32(item[12:16], ppid)
	ne.PutUint32(item[16:20], uid)
	ne.PutUint32(item[20:24], 0)
	ne.PutUint64(item[24:32], starttime)
	ne.PutUint32(item[32:36], commOff32)
	ne.PutUint32(item[36:40], commLen32)
	ne.PutUint32(item[40:44], pathOff32)
	ne.PutUint32(item[44:48], pathLen32)
	ne.PutUint32(item[48:52], nameOff32)
	ne.PutUint32(item[52:56], nameLen32)
	ne.PutUint16(item[56:58], labelCount)
	ne.PutUint16(item[58:60], 0)
	copy(item[commOff:], comm)
	item[commOff+len(comm)] = 0
	copy(item[pathOff:], cgroupPath)
	item[pathOff+len(cgroupPath)] = 0
	copy(item[nameOff:], cgroupName)
	item[nameOff+len(cgroupName)] = 0
	if len(labels) > 0 {
		clear(item[fixedEnd:tableStart])
		next, err := writeLookupLabels(item, tableStart, tableBytes, labels)
		if err != nil {
			b.err = err
			return err
		}
		itemSize = next
	}
	dir, ok := lookupDirOffset(AppsLookupRespHdr, b.itemCount)
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

func (b *AppsLookupBuilder) payloadExceededSuffixFits(dataOffset int) bool {
	if b.payloadExceededFixed > 0 {
		return payloadExceededFixedSuffixFits(len(b.buf), dataOffset, b.payloadExceededFixed, b.itemCount+1, b.maxItems)
	}
	return payloadExceededSuffixFits(len(b.buf), dataOffset, b.payloadExceededBytes, b.itemCount+1, b.maxItems)
}

func (b *AppsLookupBuilder) addUnknown(status uint16, pid uint32) error {
	itemStart, ok := checkedAlign8(b.dataOffset)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	itemEnd, ok := checkedAddInt(itemStart, appsLookupUnknownItemSize)
	if !ok || itemEnd > len(b.buf) {
		b.err = ErrOverflow
		return ErrOverflow
	}
	itemStart32, ok := checkedU32Int(itemStart)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	itemSize32, ok := checkedU32Int(appsLookupUnknownItemSize)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	clear(b.buf[b.dataOffset:itemStart])
	item := b.buf[itemStart:itemEnd]
	clear(item)
	ne.PutUint16(item[0:2], 1)
	ne.PutUint16(item[2:4], status)
	ne.PutUint32(item[8:12], pid)
	ne.PutUint32(item[16:20], NipcUIDUnset)
	ne.PutUint32(item[32:36], AppsLookupItemHdr)
	ne.PutUint32(item[40:44], AppsLookupItemHdr+1)
	ne.PutUint32(item[48:52], AppsLookupItemHdr+2)

	dir, ok := lookupDirOffset(AppsLookupRespHdr, b.itemCount)
	if !ok {
		b.err = ErrOverflow
		return ErrOverflow
	}
	ne.PutUint32(b.buf[dir:dir+4], itemStart32)
	ne.PutUint32(b.buf[dir+4:dir+8], itemSize32)
	b.dataOffset = itemEnd
	b.itemCount++
	b.err = nil
	return nil
}

func (b *AppsLookupBuilder) noteItemOverflow(pid uint32) error {
	if b.itemCount == 0 {
		return b.addUnknown(PidLookupOversizedItem, pid)
	}
	b.payloadExceededSuffix = true
	return b.addUnknown(PidLookupPayloadExceeded, pid)
}

func (b *AppsLookupBuilder) Finish() int {
	return finishLookupResponse(b.buf, AppsLookupRespHdr, b.itemCount, b.dataOffset, b.generation)
}

func (b *AppsLookupBuilder) Error() error {
	return b.err
}

func (b *AppsLookupBuilder) ItemCount() uint32 {
	return b.itemCount
}

func DispatchAppsLookup(req []byte, resp []byte, handler func(*AppsLookupRequestView, *AppsLookupBuilder) bool) (int, error) {
	request, err := DecodeAppsLookupRequest(req)
	if err != nil {
		return 0, err
	}
	minRequired, ok := lookupBuilderDataOffset(AppsLookupRespHdr, request.ItemCount)
	if !ok || len(resp) < minRequired {
		return 0, ErrOverflow
	}
	builder := NewAppsLookupBuilder(resp, request.ItemCount, 0)
	if request.ItemCount > 0 {
		builder.payloadExceededFixed = appsLookupUnknownItemSize
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
