package protocol

import "bytes"

const (
	OrchestratorUnknown uint16 = 0
	OrchestratorSystemd uint16 = 1
	OrchestratorDocker  uint16 = 2
	OrchestratorK8s     uint16 = 3
	OrchestratorKvm     uint16 = 4
	OrchestratorLxc     uint16 = 5
	OrchestratorPodman  uint16 = 6
	OrchestratorNspawn  uint16 = 7

	LookupDirEntrySize   = 8
	LookupLabelEntrySize = 16
)

// LookupLabelView represents a key-value label pair view in the lookup wire format.
type LookupLabelView struct {
	Key   CStringView
	Value CStringView
}

func invalidSourceString(data []byte, requireNonEmpty bool) bool {
	return (requireNonEmpty && len(data) == 0) || bytes.IndexByte(data, 0) >= 0
}

// maxIntValue returns the maximum value representable by int on this platform.
func maxIntValue() int {
	return int(^uint(0) >> 1)
}

func checkedU32Int(value int) (uint32, bool) {
	if value < 0 || uint64(value) > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(value), true
}

func checkedU16Int(value int) (uint16, bool) {
	if value < 0 || value > int(^uint16(0)) {
		return 0, false
	}
	return uint16(value), true
}

func checkedWireU32Int(buf []byte, off int) (int, error) {
	value, ok := checkedInt(uint64(ne.Uint32(buf[off : off+4])))
	if !ok {
		return 0, ErrOutOfBounds
	}
	return value, nil
}

func lookupDirEntry(buf []byte, base int) (int, int, error) {
	off, err := checkedWireU32Int(buf, base)
	if err != nil {
		return 0, 0, err
	}
	length, err := checkedWireU32Int(buf, base+4)
	if err != nil {
		return 0, 0, err
	}
	return off, length, nil
}

func lookupPayloadSlice(buf []byte, start int, off int, length int) ([]byte, error) {
	if start < 0 || off < 0 || length < 0 || start > maxIntValue()-off {
		return nil, ErrOutOfBounds
	}
	abs := start + off
	if abs > len(buf) || length > len(buf)-abs {
		return nil, ErrOutOfBounds
	}
	end := abs + length
	return buf[abs:end], nil
}

func lookupBuilderDataOffset(hdrSize int, maxItems uint32) (int, bool) {
	dirSize, ok := checkedInt(uint64(maxItems) * uint64(LookupDirEntrySize))
	if !ok {
		return 0, false
	}
	return checkedAddInt(hdrSize, dirSize)
}

func lookupDirOffset(hdrSize int, index uint32) (int, bool) {
	dirOff, ok := checkedInt(uint64(index) * uint64(LookupDirEntrySize))
	if !ok {
		return 0, false
	}
	return checkedAddInt(hdrSize, dirOff)
}

func checkedInt(value uint64) (int, bool) {
	maxInt := uint64(maxIntValue()) // #nosec G115 -- maxIntValue is non-negative and intentionally widened for the bounds check.
	if value > maxInt {
		return 0, false
	}
	return int(value), true // #nosec G115 -- value is bounded by maxInt above.
}

func checkedAddInt(a, b int) (int, bool) {
	if a < 0 || b < 0 {
		return 0, false
	}
	maxInt := maxIntValue()
	if a > maxInt-b {
		return 0, false
	}
	return a + b, true
}

func checkedMulInt(a, b int) (int, bool) {
	if a < 0 || b < 0 {
		return 0, false
	}
	maxInt := maxIntValue()
	if a != 0 && b > maxInt/a {
		return 0, false
	}
	return a * b, true
}

func checkedAlign8(v int) (int, bool) {
	if v < 0 {
		return 0, false
	}
	maxInt := maxIntValue()
	if v > maxInt-7 {
		return 0, false
	}
	return Align8(v), true
}

func checkedAddU32(a, b uint32) (uint32, bool) {
	if a > ^uint32(0)-b {
		return 0, false
	}
	return a + b, true
}

func checkedAlign8U32(v uint32) (uint32, bool) {
	aligned := (uint64(v) + 7) &^ uint64(7)
	if aligned > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(aligned), true
}

func buildPayloadExceededSuffixBytes(itemLens []int) ([]uint32, bool) {
	suffixBytes := make([]uint32, len(itemLens)+1)
	for i, itemLen := range itemLens {
		itemLen32, ok := checkedU32Int(itemLen)
		if !ok {
			return nil, false
		}
		suffixBytes[i] = itemLen32
	}
	if !finishPayloadExceededSuffixBytes(suffixBytes) {
		return nil, false
	}
	return suffixBytes, true
}

func makePayloadExceededSuffixBytes(itemCount uint32) ([]uint32, bool) {
	count, ok := checkedInt(uint64(itemCount))
	if !ok || count == maxIntValue() {
		return nil, false
	}
	return make([]uint32, count+1), true
}

func finishPayloadExceededSuffixBytes(suffixBytes []uint32) bool {
	if len(suffixBytes) == 0 {
		return true
	}
	suffixBytes[len(suffixBytes)-1] = 0
	for i := len(suffixBytes) - 2; i >= 0; i-- {
		itemLen := suffixBytes[i]
		itemCost := itemLen
		if suffixBytes[i+1] > 0 {
			var ok bool
			itemCost, ok = checkedAlign8U32(itemLen)
			if !ok {
				return false
			}
		}
		total, ok := checkedAddU32(itemCost, suffixBytes[i+1])
		if !ok {
			return false
		}
		suffixBytes[i] = total
	}
	return true
}

func payloadExceededSuffixFits(bufLen, dataOffset int, suffixBytes []uint32, first, maxItems uint32) bool {
	maxInt := maxIntValue()
	if uint64(maxItems) > uint64(maxInt) {
		return true
	}
	maxItemsInt := int(maxItems) // #nosec G115 -- maxItems is bounded by maxInt above.
	if maxItemsInt == maxInt || len(suffixBytes) != maxItemsInt+1 {
		return true
	}
	if uint64(first) > uint64(maxInt) {
		return false
	}
	if first > maxItems {
		return true
	}
	firstInt := int(first) // #nosec G115 -- first is bounded by maxInt above.
	if dataOffset < 0 || dataOffset > maxInt-7 {
		return false
	}
	itemStart := Align8(dataOffset)
	if itemStart > bufLen {
		return false
	}
	suffixBytesNeeded := uint64(suffixBytes[firstInt])
	return suffixBytesNeeded <= uint64(bufLen-itemStart)
}

func payloadExceededFixedSuffixFits(bufLen, dataOffset, itemLen int, first, maxItems uint32) bool {
	maxInt := maxIntValue()
	if uint64(maxItems) > uint64(maxInt) {
		return true
	}
	if uint64(first) > uint64(maxInt) {
		return false
	}
	if first > maxItems {
		return true
	}
	if dataOffset < 0 || dataOffset > maxInt-7 || itemLen < 0 {
		return false
	}
	itemStart := Align8(dataOffset)
	if itemStart > bufLen {
		return false
	}
	remaining := uint64(maxItems - first)
	if remaining == 0 {
		return true
	}
	alignedItemLen, ok := checkedAlign8(itemLen)
	if !ok {
		return false
	}
	itemLenBytes := uint64(itemLen)
	alignedItemBytes := uint64(alignedItemLen)
	tailItems := remaining - 1
	if alignedItemBytes != 0 && tailItems > (^uint64(0)-itemLenBytes)/alignedItemBytes {
		return false
	}
	needed := itemLenBytes + tailItems*alignedItemBytes
	return needed <= uint64(bufLen-itemStart)
}

func validateLookupDir(buf []byte, dirStart int, itemCount uint32, packedAreaLen int, minLen int, exactLen int) error {
	if dirStart < 0 {
		return ErrBadItemCount
	}
	if packedAreaLen < 0 {
		return ErrOutOfBounds
	}
	var minLen32 uint32
	var exactLen32 uint32
	if minLen >= 0 {
		converted, ok := checkedU32Int(minLen)
		if !ok {
			return ErrBadLayout
		}
		minLen32 = converted
	}
	if exactLen >= 0 {
		converted, ok := checkedU32Int(exactLen)
		if !ok {
			return ErrBadLayout
		}
		exactLen32 = converted
	}

	dirSize64 := uint64(itemCount) * uint64(LookupDirEntrySize)
	if dirSize64 > uint64(maxIntValue()) {
		return ErrBadItemCount
	}
	dirSize := int(dirSize64) // #nosec G115 -- bounded by maxIntValue above.
	if dirStart > maxIntValue()-dirSize {
		return ErrBadItemCount
	}
	dirEnd := dirStart + dirSize
	if dirEnd > len(buf) {
		return ErrTruncated
	}

	prevEnd := uint64(0)
	count := int(itemCount)
	for i := range count {
		base := dirStart + i*LookupDirEntrySize
		off := ne.Uint32(buf[base : base+4])
		length := ne.Uint32(buf[base+4 : base+8])
		if off%uint32(Alignment) != 0 {
			return ErrBadAlignment
		}
		if exactLen >= 0 {
			if length != exactLen32 {
				return ErrBadLayout
			}
		} else if length < minLen32 {
			return ErrBadLayout
		}
		end := uint64(off) + uint64(length)
		if end > uint64(packedAreaLen) {
			return ErrOutOfBounds
		}
		if i > 0 && uint64(off) < prevEnd {
			return ErrBadLayout
		}
		prevEnd = end
	}
	return nil
}

func lookupString(item []byte, hdrSize int, off int, length int) (CStringView, int, error) {
	var view CStringView
	end, err := lookupStringInto(item, hdrSize, off, length, &view)
	return view, end, err
}

func viewOut[T any](owner *T, view *CStringView) *CStringView {
	if owner == nil {
		return nil
	}
	return view
}

func lookupStringInto(item []byte, hdrSize int, off int, length int, out *CStringView) (int, error) {
	if off < hdrSize || length < 0 || off > maxIntValue()-length {
		return 0, ErrOutOfBounds
	}
	nul := off + length
	if nul >= len(item) {
		return 0, ErrOutOfBounds
	}
	if item[nul] != 0 {
		return 0, ErrMissingNul
	}
	if bytes.IndexByte(item[off:nul], 0) >= 0 {
		return 0, ErrBadLayout
	}
	if out != nil {
		if uint64(length) > uint64(^uint32(0)) {
			return 0, ErrOutOfBounds
		}
		length32 := uint32(length) // #nosec G115 -- bounded by uint32 max above.
		*out = NewCStringView(item[off:nul+1], length32)
	}
	return nul + 1, nil
}

func lookupEmptyString(item []byte, hdrSize int, off int) (CStringView, int, error) {
	var view CStringView
	end, err := lookupEmptyStringInto(item, hdrSize, off, &view)
	return view, end, err
}

func lookupEmptyStringInto(item []byte, hdrSize int, off int, out *CStringView) (int, error) {
	if off < hdrSize || off >= len(item) {
		return 0, ErrOutOfBounds
	}
	if item[off] != 0 {
		return 0, ErrMissingNul
	}
	if out != nil {
		*out = NewCStringView(item[off:off+1], 0)
	}
	return off + 1, nil
}

func overlap(aStart, aEnd, bStart, bEnd int) bool {
	return aStart < bEnd && bStart < aEnd
}

func validateLabels(item []byte, hdrSize int, labelCount uint16, fixedEnd int) (int, error) {
	if labelCount == 0 {
		if fixedEnd != len(item) {
			return 0, ErrBadLayout
		}
		return fixedEnd, nil
	}

	tableStart, ok := checkedAlign8(fixedEnd)
	if !ok {
		return 0, ErrOutOfBounds
	}
	if tableStart > len(item) {
		return 0, ErrOutOfBounds
	}
	for _, b := range item[fixedEnd:tableStart] {
		if b != 0 {
			return 0, ErrBadLayout
		}
	}

	tableBytes, ok := checkedInt(uint64(labelCount) * uint64(LookupLabelEntrySize))
	if !ok {
		return 0, ErrOutOfBounds
	}
	expected, ok := checkedAddInt(tableStart, tableBytes)
	if !ok || expected > len(item) {
		return 0, ErrOutOfBounds
	}

	for i := range labelCount {
		entryRel, ok := checkedInt(uint64(i) * uint64(LookupLabelEntrySize))
		if !ok {
			return 0, ErrOutOfBounds
		}
		base, ok := checkedAddInt(tableStart, entryRel)
		if !ok {
			return 0, ErrOutOfBounds
		}
		keyOff, err := checkedWireU32Int(item, base)
		if err != nil {
			return 0, err
		}
		keyLen, err := checkedWireU32Int(item, base+4)
		if err != nil {
			return 0, err
		}
		valueOff, err := checkedWireU32Int(item, base+8)
		if err != nil {
			return 0, err
		}
		valueLen, err := checkedWireU32Int(item, base+12)
		if err != nil {
			return 0, err
		}
		if keyLen == 0 || keyOff != expected {
			return 0, ErrBadLayout
		}
		_, keyEnd, err := lookupString(item, hdrSize, keyOff, keyLen)
		if err != nil {
			return 0, err
		}
		expected = keyEnd
		if valueOff != expected {
			return 0, ErrBadLayout
		}
		_, valueEnd, err := lookupString(item, hdrSize, valueOff, valueLen)
		if err != nil {
			return 0, err
		}
		expected = valueEnd
	}
	if expected != len(item) {
		return 0, ErrBadLayout
	}
	return tableStart, nil
}

func lookupLabelAt(item []byte, hdrSize int, labelCount uint16, tableOffset int, index uint32) (LookupLabelView, error) {
	if index >= uint32(labelCount) {
		return LookupLabelView{}, ErrOutOfBounds
	}
	entryRel, ok := checkedInt(uint64(index) * uint64(LookupLabelEntrySize))
	if !ok {
		return LookupLabelView{}, ErrOutOfBounds
	}
	base, ok := checkedAddInt(tableOffset, entryRel)
	if !ok {
		return LookupLabelView{}, ErrOutOfBounds
	}
	keyOff, err := checkedWireU32Int(item, base)
	if err != nil {
		return LookupLabelView{}, err
	}
	keyLen, err := checkedWireU32Int(item, base+4)
	if err != nil {
		return LookupLabelView{}, err
	}
	valueOff, err := checkedWireU32Int(item, base+8)
	if err != nil {
		return LookupLabelView{}, err
	}
	valueLen, err := checkedWireU32Int(item, base+12)
	if err != nil {
		return LookupLabelView{}, err
	}
	key, _, err := lookupString(item, hdrSize, keyOff, keyLen)
	if err != nil {
		return LookupLabelView{}, err
	}
	value, _, err := lookupString(item, hdrSize, valueOff, valueLen)
	if err != nil {
		return LookupLabelView{}, err
	}
	return LookupLabelView{Key: key, Value: value}, nil
}

func writeLookupLabels(item []byte, tableStart, tableBytes int, labels []struct{ Key, Value []byte }) (int, error) {
	if tableStart < 0 || tableBytes < 0 || tableStart > maxIntValue()-tableBytes {
		return 0, ErrOverflow
	}
	next := tableStart + tableBytes
	for i, label := range labels {
		keyLen := len(label.Key)
		valueLen := len(label.Value)
		if next > maxIntValue()-keyLen ||
			next+keyLen > maxIntValue()-1 {
			return 0, ErrOverflow
		}
		valueOff := next + keyLen + 1
		if valueOff > maxIntValue()-valueLen || valueOff+valueLen > maxIntValue()-1 {
			return 0, ErrOverflow
		}
		itemNext := valueOff + valueLen + 1
		if uint64(next) > uint64(^uint32(0)) ||
			uint64(keyLen) > uint64(^uint32(0)) ||
			uint64(valueOff) > uint64(^uint32(0)) ||
			uint64(valueLen) > uint64(^uint32(0)) {
			return 0, ErrOverflow
		}
		entry := tableStart + i*LookupLabelEntrySize
		keyOff32 := uint32(next)       // #nosec G115 -- bounded by uint32 max above.
		keyLen32 := uint32(keyLen)     // #nosec G115 -- bounded by uint32 max above.
		valueOff32 := uint32(valueOff) // #nosec G115 -- bounded by uint32 max above.
		valueLen32 := uint32(valueLen) // #nosec G115 -- bounded by uint32 max above.
		ne.PutUint32(item[entry:entry+4], keyOff32)
		ne.PutUint32(item[entry+4:entry+8], keyLen32)
		ne.PutUint32(item[entry+8:entry+12], valueOff32)
		ne.PutUint32(item[entry+12:entry+16], valueLen32)
		copy(item[next:], label.Key)
		item[next+len(label.Key)] = 0
		next = valueOff
		copy(item[next:], label.Value)
		item[next+len(label.Value)] = 0
		next = itemNext
	}
	return next, nil
}

func lookupLabelWriteLayout(next, keyLen, valueLen int) (keyOff32, keyLen32 uint32, valueOff int, valueOff32, valueLen32 uint32, itemNext int, ok bool) {
	keyOff32, ok = checkedU32Int(next)
	if !ok {
		return 0, 0, 0, 0, 0, 0, false
	}
	keyLen32, ok = checkedU32Int(keyLen)
	if !ok {
		return 0, 0, 0, 0, 0, 0, false
	}
	valueOff, ok = checkedAddInt(next, keyLen)
	if ok {
		valueOff, ok = checkedAddInt(valueOff, 1)
	}
	if !ok {
		return 0, 0, 0, 0, 0, 0, false
	}
	valueOff32, ok = checkedU32Int(valueOff)
	if !ok {
		return 0, 0, 0, 0, 0, 0, false
	}
	valueLen32, ok = checkedU32Int(valueLen)
	if !ok {
		return 0, 0, 0, 0, 0, 0, false
	}
	itemNext, ok = checkedAddInt(valueOff, valueLen)
	if ok {
		itemNext, ok = checkedAddInt(itemNext, 1)
	}
	if !ok {
		return 0, 0, 0, 0, 0, 0, false
	}
	return keyOff32, keyLen32, valueOff, valueOff32, valueLen32, itemNext, true
}

func labelLayoutGo(fixedEnd int, labels []struct{ Key, Value []byte }) (int, int, int, error) {
	tableStart, tableBytes, itemSize, err := labelLayoutPrefix(fixedEnd, len(labels))
	if err != nil {
		return 0, 0, 0, err
	}
	for _, label := range labels {
		if invalidSourceString(label.Key, true) || invalidSourceString(label.Value, false) {
			return 0, 0, 0, ErrBadLayout
		}
		keyLen := len(label.Key)
		valueLen := len(label.Value)
		if keyLen > maxIntValue()-1 ||
			valueLen > maxIntValue()-1 ||
			keyLen+1 > maxIntValue()-(valueLen+1) {
			return 0, 0, 0, ErrOverflow
		}
		labelBytes := keyLen + 1 + valueLen + 1
		if itemSize > maxIntValue()-labelBytes {
			return 0, 0, 0, ErrOverflow
		}
		itemSize += labelBytes
	}
	return tableStart, tableBytes, itemSize, nil
}

func labelLayoutPrefix(fixedEnd, labelCount int) (int, int, int, error) {
	if labelCount == 0 {
		return fixedEnd, 0, fixedEnd, nil
	}
	if fixedEnd < 0 || fixedEnd > maxIntValue()-7 {
		return 0, 0, 0, ErrOverflow
	}
	tableStart := Align8(fixedEnd)
	tableBytes64 := uint64(labelCount) * uint64(LookupLabelEntrySize)
	if tableBytes64 > uint64(maxIntValue()) {
		return 0, 0, 0, ErrOverflow
	}
	tableBytes := int(tableBytes64) // #nosec G115 -- bounded by maxIntValue above.
	if tableStart > maxIntValue()-tableBytes {
		return 0, 0, 0, ErrOverflow
	}
	itemSize := tableStart + tableBytes
	return tableStart, tableBytes, itemSize, nil
}

func finishLookupResponse(buf []byte, hdrSize int, itemCount uint32, dataOffset int, generation uint64) int {
	ne.PutUint16(buf[0:2], 1)
	ne.PutUint16(buf[2:4], 0)
	ne.PutUint32(buf[4:8], itemCount)
	ne.PutUint64(buf[8:16], generation)
	if itemCount == 0 {
		return hdrSize
	}
	dirSize, ok := checkedInt(uint64(itemCount) * uint64(LookupDirEntrySize))
	if !ok {
		return 0
	}
	count := int(itemCount)
	finalPackedStart, ok := checkedAddInt(hdrSize, dirSize)
	if !ok {
		return 0
	}
	firstItemAbs, ok := checkedInt(uint64(ne.Uint32(buf[hdrSize : hdrSize+4])))
	if !ok {
		return 0
	}
	if dataOffset < firstItemAbs {
		return 0
	}
	packedDataLen := dataOffset - firstItemAbs
	if finalPackedStart < firstItemAbs {
		copy(buf[finalPackedStart:], buf[firstItemAbs:firstItemAbs+packedDataLen])
	}
	for i := range count {
		entry := hdrSize + i*LookupDirEntrySize
		abs, ok := checkedInt(uint64(ne.Uint32(buf[entry : entry+4])))
		if !ok || abs < firstItemAbs {
			return 0
		}
		rel, ok := checkedU32Int(abs - firstItemAbs)
		if !ok {
			return 0
		}
		ne.PutUint32(buf[entry:entry+4], rel)
	}
	return finalPackedStart + packedDataLen
}

func lookupResponseRawItem(payload []byte, hdrSize int, itemCount uint32, index uint32) ([]byte, error) {
	if index >= itemCount {
		return nil, ErrOutOfBounds
	}
	packedStart, ok := lookupBuilderDataOffset(hdrSize, itemCount)
	if !ok {
		return nil, ErrBadItemCount
	}
	dir, ok := lookupDirOffset(hdrSize, index)
	if !ok {
		return nil, ErrBadItemCount
	}
	off, length, err := lookupDirEntry(payload, dir)
	if err != nil {
		return nil, err
	}
	return lookupPayloadSlice(payload, packedStart, off, length)
}

func encodeLookupRawResponse(buf []byte, hdrSize int, generation uint64, items [][]byte, minItemLen int, validate func([]byte) error) (int, error) {
	itemCount, minRequired, ok := lookupRawResponseMinBytes(hdrSize, len(items))
	if !ok || len(buf) < minRequired {
		return 0, ErrOverflow
	}
	dataOffset := minRequired
	for i, item := range items {
		if len(item) < minItemLen {
			return 0, ErrBadLayout
		}
		if validate != nil {
			if err := validate(item); err != nil {
				return 0, err
			}
		}
		itemStart, itemEnd, ok := lookupRawResponseItemBounds(dataOffset, len(item))
		if !ok || itemEnd > len(buf) {
			return 0, ErrOverflow
		}
		clear(buf[dataOffset:itemStart])
		copy(buf[itemStart:itemEnd], item)
		dir := hdrSize + i*LookupDirEntrySize
		itemStart32, ok := checkedU32Int(itemStart)
		if !ok {
			return 0, ErrOverflow
		}
		itemLen32, ok := checkedU32Int(len(item))
		if !ok {
			return 0, ErrOverflow
		}
		ne.PutUint32(buf[dir:dir+4], itemStart32)
		ne.PutUint32(buf[dir+4:dir+8], itemLen32)
		dataOffset = itemEnd
	}
	n := finishLookupResponse(buf, hdrSize, itemCount, dataOffset, generation)
	if n == 0 {
		return 0, ErrOverflow
	}
	return n, nil
}

func lookupRawResponseMinBytes(hdrSize, itemCount int) (uint32, int, bool) {
	if itemCount < 0 || uint64(itemCount) > uint64(^uint32(0)) {
		return 0, 0, false
	}
	itemCount32 := uint32(itemCount)
	minRequired, ok := lookupBuilderDataOffset(hdrSize, itemCount32)
	return itemCount32, minRequired, ok
}

func lookupRawResponseItemBounds(dataOffset, itemLen int) (int, int, bool) {
	itemStart, ok := checkedAlign8(dataOffset)
	if !ok {
		return 0, 0, false
	}
	itemEnd, ok := checkedAddInt(itemStart, itemLen)
	if !ok {
		return 0, 0, false
	}
	return itemStart, itemEnd, true
}
