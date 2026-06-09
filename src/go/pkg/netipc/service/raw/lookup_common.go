package raw

import "github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"

func checkedLookupAdd(a, b int) (int, error) {
	if a < 0 || b < 0 {
		return 0, protocol.ErrOverflow
	}
	maxInt := int(^uint(0) >> 1)
	if a > maxInt-b {
		return 0, protocol.ErrOverflow
	}
	return a + b, nil
}

func checkedLookupMul(a, b int) (int, error) {
	if a < 0 || b < 0 {
		return 0, protocol.ErrOverflow
	}
	maxInt := int(^uint(0) >> 1)
	if a != 0 && b > maxInt/a {
		return 0, protocol.ErrOverflow
	}
	return a * b, nil
}

func checkedLookupAlign8(v int) (int, error) {
	if v < 0 {
		return 0, protocol.ErrOverflow
	}
	maxInt := int(^uint(0) >> 1)
	if v > maxInt-7 {
		return 0, protocol.ErrOverflow
	}
	return protocol.Align8(v), nil
}

func checkedLookupU32(value int) (uint32, error) {
	if value < 0 || uint64(value) > uint64(^uint32(0)) {
		return 0, protocol.ErrOverflow
	}
	return uint32(value), nil // #nosec G115 -- value is bounded by the uint32 maximum above.
}

func lookupMinRequired(hdrSize int, itemCount uint32) (int, error) {
	if hdrSize < 0 {
		return 0, protocol.ErrOverflow
	}
	dirSize64 := uint64(itemCount) * uint64(protocol.LookupDirEntrySize)
	min64 := uint64(hdrSize) + dirSize64
	if min64 > uint64(int(^uint(0)>>1)) {
		return 0, protocol.ErrOverflow
	}
	return int(min64), nil
}
