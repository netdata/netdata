// STRING_REVERSE codec (method 3) -- variable-length payload:
//
//	[0:4] u32 str_offset (from payload start, always 8)
//	[4:8] u32 str_length (excluding NUL)
//	[8:N+1] string data + NUL

package protocol

const StringReverseHdrSize = 8

// StringReverseView is the decoded result of a STRING_REVERSE payload.
type StringReverseView struct {
	Str    string
	StrLen uint32
}

// StringReverseEncode writes a STRING_REVERSE payload into buf.
// Returns total bytes written, or 0 if buf is too small.
func StringReverseEncode(s string, buf []byte) int {
	total := StringReverseHdrSize + len(s) + 1
	if len(buf) < total {
		return 0
	}
	ne.PutUint32(buf[0:4], uint32(StringReverseHdrSize)) // str_offset
	ne.PutUint32(buf[4:8], uint32(len(s)))               // str_length
	if len(s) > 0 {
		copy(buf[8:8+len(s)], s)
	}
	buf[8+len(s)] = 0 // NUL terminator
	return total
}

// StringReverseDecode decodes a STRING_REVERSE payload from buf.
func StringReverseDecode(buf []byte) (StringReverseView, error) {
	if len(buf) < StringReverseHdrSize {
		return StringReverseView{}, ErrTruncated
	}
	strOffset := int(ne.Uint32(buf[0:4]))
	strLength := int(ne.Uint32(buf[4:8]))
	if strOffset+strLength+1 > len(buf) {
		return StringReverseView{}, ErrOutOfBounds
	}
	if buf[strOffset+strLength] != 0 {
		return StringReverseView{}, ErrMissingNul
	}
	return StringReverseView{
		Str:    string(buf[strOffset : strOffset+strLength]),
		StrLen: uint32(strLength),
	}, nil
}

// DispatchStringReverse decodes request, calls handler, encodes response.
func DispatchStringReverse(req []byte, resp []byte, handler func(string) (string, bool)) (int, bool) {
	view, err := StringReverseDecode(req)
	if err != nil {
		return 0, false
	}
	result, ok := handler(view.Str)
	if !ok {
		return 0, false
	}
	n := StringReverseEncode(result, resp)
	return n, n > 0
}
