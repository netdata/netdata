package observation

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	functionResultBegin = []byte("FUNCTION_RESULT_BEGIN ")
	functionResultEnd   = []byte("FUNCTION_RESULT_END\n\n")
)

type FunctionResult struct {
	UID              string
	Status           int
	ContentType      string
	Expiry           int64
	Payload          []byte
	Deferred         []byte
	Raw              []byte
	RawSHA256        string
	ReadReturnMonoNS int64
}

// FunctionResultDecoder incrementally extracts Function result frames from a
// mixed plugin stdout byte stream.
type FunctionResultDecoder struct {
	buffer        []byte
	maxFrameBytes int
}

func NewFunctionResultDecoder(maxFrameBytes int) (*FunctionResultDecoder, error) {
	if maxFrameBytes < len(functionResultBegin)+len(functionResultEnd) {
		return nil, errors.New("function result decoder: frame bound is too small")
	}
	return &FunctionResultDecoder{maxFrameBytes: maxFrameBytes}, nil
}

// Feed returns complete frames and non-Function bytes made final by this chunk.
// All frames completed by one OS read carry that read-return timestamp.
func (decoder *FunctionResultDecoder) Feed(chunk []byte, readReturnMonoNS int64) ([]FunctionResult, []byte, error) {
	decoder.buffer = append(decoder.buffer, chunk...)
	var results []FunctionResult
	var skipped []byte
	for {
		start := findLinePrefix(decoder.buffer, functionResultBegin)
		if start < 0 {
			keep := partialPrefixLen(decoder.buffer, functionResultBegin)
			flush := len(decoder.buffer) - keep
			skipped = append(skipped, decoder.buffer[:flush]...)
			decoder.buffer = append(decoder.buffer[:0], decoder.buffer[flush:]...)
			return results, skipped, nil
		}
		if start > 0 {
			skipped = append(skipped, decoder.buffer[:start]...)
			decoder.buffer = decoder.buffer[start:]
		}
		end := bytes.Index(decoder.buffer, functionResultEnd)
		if end < 0 {
			if len(decoder.buffer) > decoder.maxFrameBytes {
				return nil, skipped, errors.New("function result decoder: frame exceeds bound")
			}
			return results, skipped, nil
		}
		frameLen := end + len(functionResultEnd)
		if frameLen > decoder.maxFrameBytes {
			return nil, skipped, errors.New("function result decoder: frame exceeds bound")
		}
		result, err := parseFunctionResult(decoder.buffer[:frameLen], readReturnMonoNS)
		if err != nil {
			return nil, skipped, err
		}
		results = append(results, result)
		decoder.buffer = decoder.buffer[frameLen:]
	}
}

func (decoder *FunctionResultDecoder) Finish() ([]byte, error) {
	if findLinePrefix(decoder.buffer, functionResultBegin) >= 0 {
		return nil, errors.New("function result decoder: truncated frame")
	}
	remaining := append([]byte(nil), decoder.buffer...)
	decoder.buffer = nil
	return remaining, nil
}

func (result FunctionResult) NormalizeExpiry(runWallLowerUnix, runWallUpperUnix int64) ([]byte, error) {
	if runWallLowerUnix > runWallUpperUnix {
		return nil, errors.New("function result: inverted run clock bounds")
	}
	if result.Expiry < runWallLowerUnix || result.Expiry > runWallUpperUnix {
		return nil, fmt.Errorf("function result: expiry %d outside [%d,%d]", result.Expiry, runWallLowerUnix, runWallUpperUnix)
	}
	headerEnd := bytes.IndexByte(result.Raw, '\n')
	if headerEnd < 0 {
		return nil, errors.New("function result: raw header is missing")
	}
	fields := bytes.Fields(result.Raw[:headerEnd])
	if len(fields) != 5 {
		return nil, errors.New("function result: raw header fields changed")
	}
	prefixEnd := bytes.LastIndex(result.Raw[:headerEnd], fields[4])
	if prefixEnd < 0 {
		return nil, errors.New("function result: raw expiry token is missing")
	}
	normalized := make([]byte, 0, len(result.Raw)-len(fields[4])+len("@EXPIRY@"))
	normalized = append(normalized, result.Raw[:prefixEnd]...)
	normalized = append(normalized, "@EXPIRY@"...)
	normalized = append(normalized, result.Raw[prefixEnd+len(fields[4]):]...)
	return normalized, nil
}

func parseFunctionResult(frame []byte, readReturnMonoNS int64) (FunctionResult, error) {
	headerEnd := bytes.IndexByte(frame, '\n')
	if headerEnd < 0 {
		return FunctionResult{}, errors.New("function result: missing header LF")
	}
	fields := strings.Fields(string(frame[:headerEnd]))
	if len(fields) != 5 || fields[0] != "FUNCTION_RESULT_BEGIN" {
		return FunctionResult{}, errors.New("function result: invalid begin header")
	}
	if strings.ContainsAny(fields[1], "\x00\r\n") || fields[1] == "" {
		return FunctionResult{}, errors.New("function result: invalid UID")
	}
	status, err := strconv.Atoi(fields[2])
	if err != nil || status < 100 || status > 599 {
		return FunctionResult{}, errors.New("function result: invalid status")
	}
	if fields[3] == "" || strings.ContainsAny(fields[3], "\x00\r\n") {
		return FunctionResult{}, errors.New("function result: invalid content type")
	}
	expiry, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil {
		return FunctionResult{}, errors.New("function result: invalid expiry")
	}
	bodyEnd := len(frame) - len(functionResultEnd)
	deferred := frame[headerEnd+1 : bodyEnd]
	var payload []byte
	if len(deferred) > 0 {
		if deferred[len(deferred)-1] != '\n' {
			return FunctionResult{}, errors.New("function result: nonempty payload lacks final LF")
		}
		payload = deferred[:len(deferred)-1]
	}
	raw := append([]byte(nil), frame...)
	return FunctionResult{
		UID: fields[1], Status: status, ContentType: fields[3], Expiry: expiry,
		Payload: append([]byte(nil), payload...), Deferred: append([]byte(nil), deferred...),
		Raw: raw, RawSHA256: sha256Hex(raw), ReadReturnMonoNS: readReturnMonoNS,
	}, nil
}

func findLinePrefix(buffer, prefix []byte) int {
	for offset := 0; offset < len(buffer); {
		index := bytes.Index(buffer[offset:], prefix)
		if index < 0 {
			return -1
		}
		index += offset
		if index == 0 || buffer[index-1] == '\n' {
			return index
		}
		offset = index + 1
	}
	return -1
}

func partialPrefixLen(buffer, prefix []byte) int {
	maximum := min(len(buffer), len(prefix)-1)
	for count := maximum; count > 0; count-- {
		if bytes.Equal(buffer[len(buffer)-count:], prefix[:count]) && (len(buffer) == count || buffer[len(buffer)-count-1] == '\n') {
			return count
		}
	}
	return 0
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
