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
	total, ok := stringReverseEncodedLen(len(s))
	if !ok {
		return 0
	}
	if len(buf) < total {
		return 0
	}
	strLen, ok := checkedU32Int(len(s))
	if !ok {
		return 0
	}
	ne.PutUint32(buf[0:4], uint32(StringReverseHdrSize)) // str_offset
	ne.PutUint32(buf[4:8], strLen)                       // str_length
	if len(s) > 0 {
		copy(buf[8:8+len(s)], s)
	}
	buf[8+len(s)] = 0 // NUL terminator
	return total
}

func stringReverseEncodedLen(strLen int) (int, bool) {
	if _, ok := checkedU32Int(strLen); !ok {
		return 0, false
	}
	total, ok := checkedAddInt(StringReverseHdrSize, strLen)
	if ok {
		total, ok = checkedAddInt(total, 1)
	}
	return total, ok
}

// StringReverseDecode decodes a STRING_REVERSE payload from buf.
func StringReverseDecode(buf []byte) (StringReverseView, error) {
	if len(buf) < StringReverseHdrSize {
		return StringReverseView{}, ErrTruncated
	}
	strOffset, err := checkedWireU32Int(buf, 0)
	if err != nil {
		return StringReverseView{}, err
	}
	if strOffset != StringReverseHdrSize {
		return StringReverseView{}, ErrBadLayout
	}
	strLength, err := checkedWireU32Int(buf, 4)
	if err != nil {
		return StringReverseView{}, err
	}
	strLength32 := ne.Uint32(buf[4:8])
	strEnd, ok := checkedAddInt(strOffset, strLength)
	if !ok {
		return StringReverseView{}, ErrOutOfBounds
	}
	strNulEnd, ok := checkedAddInt(strEnd, 1)
	if !ok || strNulEnd > len(buf) {
		return StringReverseView{}, ErrOutOfBounds
	}
	if buf[strEnd] != 0 {
		return StringReverseView{}, ErrMissingNul
	}
	return StringReverseView{
		Str:    string(buf[strOffset:strEnd]),
		StrLen: strLength32,
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
