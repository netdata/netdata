// INCREMENT codec (method 1) -- 8-byte payload: { u64 value }

package protocol

const IncrementPayloadSize = 8

// IncrementEncode writes a u64 value into buf. Returns 8 on success, 0 if
// buf is too small.
func IncrementEncode(value uint64, buf []byte) int {
	if len(buf) < IncrementPayloadSize {
		return 0
	}
	ne.PutUint64(buf[:8], value)
	return IncrementPayloadSize
}

// IncrementDecode reads a u64 value from buf.
func IncrementDecode(buf []byte) (uint64, error) {
	if len(buf) < IncrementPayloadSize {
		return 0, ErrTruncated
	}
	return ne.Uint64(buf[:8]), nil
}

// DispatchIncrement decodes request, calls handler, encodes response.
func DispatchIncrement(req []byte, resp []byte, handler func(uint64) (uint64, bool)) (int, bool) {
	value, err := IncrementDecode(req)
	if err != nil {
		return 0, false
	}
	result, ok := handler(value)
	if !ok {
		return 0, false
	}
	n := IncrementEncode(result, resp)
	return n, n > 0
}
