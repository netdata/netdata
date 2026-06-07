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
	abs, ok := checkedAddInt(start, off)
	if !ok {
		return nil, ErrOutOfBounds
	}
	end, ok := checkedAddInt(abs, length)
	if !ok || end > len(buf) {
		return nil, ErrOutOfBounds
	}
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

func validateLookupDir(buf []byte, dirStart int, itemCount uint32, packedAreaLen int, minLen int, exactLen int) error {
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

	dirSize, ok := checkedInt(uint64(itemCount) * uint64(LookupDirEntrySize))
	if !ok {
		return ErrBadItemCount
	}
	dirEnd, ok := checkedAddInt(dirStart, dirSize)
	if !ok {
		return ErrBadItemCount
	}
	if dirEnd > len(buf) {
		return ErrTruncated
	}

	prevEnd := 0
	for i := range itemCount {
		base, ok := lookupDirOffset(dirStart, i)
		if !ok {
			return ErrBadItemCount
		}
		off, length, err := lookupDirEntry(buf, base)
		if err != nil {
			return err
		}
		if off%Alignment != 0 {
			return ErrBadAlignment
		}
		length32, ok := checkedU32Int(length)
		if !ok {
			return ErrOutOfBounds
		}
		if exactLen >= 0 {
			if length32 != exactLen32 {
				return ErrBadLayout
			}
		} else if length32 < minLen32 {
			return ErrBadLayout
		}
		end, ok := checkedAddInt(off, length)
		if !ok || end > packedAreaLen {
			return ErrOutOfBounds
		}
		if i > 0 && off < prevEnd {
			return ErrBadLayout
		}
		prevEnd = end
	}
	return nil
}

func lookupString(item []byte, hdrSize int, off int, length int) (CStringView, int, error) {
	if off < hdrSize {
		return CStringView{}, 0, ErrOutOfBounds
	}
	nul, ok := checkedAddInt(off, length)
	if !ok || nul >= len(item) {
		return CStringView{}, 0, ErrOutOfBounds
	}
	if item[nul] != 0 {
		return CStringView{}, 0, ErrMissingNul
	}
	if bytes.Contains(item[off:nul], []byte{0}) {
		return CStringView{}, 0, ErrBadLayout
	}
	length32, ok := checkedU32Int(length)
	if !ok {
		return CStringView{}, 0, ErrOutOfBounds
	}
	return NewCStringView(item[off:nul+1], length32), nul + 1, nil
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
	next, ok := checkedAddInt(tableStart, tableBytes)
	if !ok {
		return 0, ErrOverflow
	}
	for i, label := range labels {
		keyOff32, ok := checkedU32Int(next)
		if !ok {
			return 0, ErrOverflow
		}
		keyLen32, ok := checkedU32Int(len(label.Key))
		if !ok {
			return 0, ErrOverflow
		}
		valueOff, ok := checkedAddInt(next, len(label.Key))
		if ok {
			valueOff, ok = checkedAddInt(valueOff, 1)
		}
		if !ok {
			return 0, ErrOverflow
		}
		valueOff32, ok := checkedU32Int(valueOff)
		if !ok {
			return 0, ErrOverflow
		}
		valueLen32, ok := checkedU32Int(len(label.Value))
		if !ok {
			return 0, ErrOverflow
		}
		entryRel, ok := checkedMulInt(i, LookupLabelEntrySize)
		if !ok {
			return 0, ErrOverflow
		}
		entry, ok := checkedAddInt(tableStart, entryRel)
		if !ok {
			return 0, ErrOverflow
		}
		ne.PutUint32(item[entry:entry+4], keyOff32)
		ne.PutUint32(item[entry+4:entry+8], keyLen32)
		ne.PutUint32(item[entry+8:entry+12], valueOff32)
		ne.PutUint32(item[entry+12:entry+16], valueLen32)
		copy(item[next:], label.Key)
		item[next+len(label.Key)] = 0
		next = valueOff
		copy(item[next:], label.Value)
		item[next+len(label.Value)] = 0
		next, ok = checkedAddInt(next, len(label.Value))
		if ok {
			next, ok = checkedAddInt(next, 1)
		}
		if !ok {
			return 0, ErrOverflow
		}
	}
	return next, nil
}

func labelLayoutGo(fixedEnd int, labels []struct{ Key, Value []byte }) (int, int, int, error) {
	if len(labels) == 0 {
		return fixedEnd, 0, fixedEnd, nil
	}
	tableStart, ok := checkedAlign8(fixedEnd)
	if !ok {
		return 0, 0, 0, ErrOverflow
	}
	tableBytes, ok := checkedMulInt(len(labels), LookupLabelEntrySize)
	if !ok {
		return 0, 0, 0, ErrOverflow
	}
	itemSize, ok := checkedAddInt(tableStart, tableBytes)
	if !ok {
		return 0, 0, 0, ErrOverflow
	}
	for _, label := range labels {
		if invalidSourceString(label.Key, true) || invalidSourceString(label.Value, false) {
			return 0, 0, 0, ErrBadLayout
		}
		keySize, ok := checkedAddInt(len(label.Key), 1)
		if ok {
			valueSize, okValue := checkedAddInt(len(label.Value), 1)
			if okValue {
				keySize, ok = checkedAddInt(keySize, valueSize)
			} else {
				ok = false
			}
		}
		if ok {
			itemSize, ok = checkedAddInt(itemSize, keySize)
		}
		if !ok {
			return 0, 0, 0, ErrOverflow
		}
	}
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
