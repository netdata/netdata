// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	maximumInputLineBytes   = 64 * 1024
	MaximumCommandLineBytes = 15_487
	MaximumInputBodyBytes   = 20 * 1024 * 1024
	maximumFunctionTimeout  = 2 * time.Minute
	initialBodyCapacity     = 64 * 1024
)

// Call is the immutable Function ingress value transferred to the active
// Agent generation.
type Call struct {
	UID         string
	Timeout     time.Duration
	Method      string
	Args        []string
	Access      string
	Source      string
	Payload     []byte
	ContentType string
	HasPayload  bool
}

// readReturnGate fences reader returns while process ingress changes Agent
// generation.
type readReturnGate interface {
	AcquireInputRead(context.Context) (bool, error)
	ReleaseInputRead()
}

type Consumer interface {
	HandleCall(context.Context, Call) error
	HandleCancel(context.Context, string) error
	HandleReject(context.Context, string, int) error
	HandleQuit(context.Context) error
}

// InputCapsule is the process-fixed bounded parser. It owns only the current
// parser payload and transfers completed calls to Consumer.
type InputCapsule struct {
	reader            io.Reader
	payload           *payloadState
	discarding        bool
	discardingUID     string
	discardingPayload bool
}

type payloadState struct {
	call           Call
	body           []byte
	separator      [2]byte
	separatorBytes int
	hasLine        bool
	lineStarted    bool
	rejectStatus   int
}

func NewInputCapsule(reader io.Reader) (*InputCapsule, error) {
	if reader == nil {
		return nil, errors.New("function ingress: nil input reader")
	}
	return &InputCapsule{
		reader: reader,
	}, nil
}

func (capsule *InputCapsule) Run(ctx context.Context, consumer Consumer) error {
	if ctx == nil || consumer == nil {
		return errors.New("function ingress: invalid run")
	}
	reader := bufio.NewReaderSize(capsule.reader, maximumInputLineBytes+1)
	gate, _ := consumer.(readReturnGate)
	for {
		segment, readErr := reader.ReadSlice('\n')
		if gate != nil {
			allowed, err := gate.AcquireInputRead(ctx)
			if err != nil {
				return err
			}
			if !allowed {
				if readErr != nil && !errors.Is(readErr, bufio.ErrBufferFull) {
					if errors.Is(readErr, io.EOF) {
						return nil
					}
					return fmt.Errorf("function ingress: read after containment: %w", readErr)
				}
				continue
			}
		}
		quit, done, err := capsule.consumeRead(ctx, consumer, segment, readErr)
		if gate != nil {
			gate.ReleaseInputRead()
		}
		if err != nil {
			return err
		}
		if quit || done {
			return nil
		}
	}
}

func (capsule *InputCapsule) consumeRead(ctx context.Context, consumer Consumer, segment []byte, readErr error) (quit, done bool, resultErr error) {
	complete := !errors.Is(readErr, bufio.ErrBufferFull)
	if capsule.payload != nil {
		if errors.Is(readErr, io.EOF) && (len(segment) == 0 || !hasLF(segment)) {
			capsule.abortPayload()
			return false, false, errors.New("function ingress: unterminated payload")
		}
		if handleErr := capsule.handlePayloadSegment(ctx, consumer, segment, complete); handleErr != nil {
			capsule.abortPayload()
			return false, false, handleErr
		}
	} else if capsule.discarding {
		if complete {
			if !hasLF(segment) {
				return false, false, errors.New("function ingress: unterminated line")
			}
			uid := capsule.discardingUID
			payload := capsule.discardingPayload
			capsule.discarding = false
			capsule.discardingUID = ""
			capsule.discardingPayload = false
			if rejectErr := capsule.rejectHeader(ctx, consumer, uid, payload); rejectErr != nil {
				return false, false, rejectErr
			}
		}
	} else if !complete {
		uid, payload := rejectionHeader(string(segment))
		if uid == "" {
			return false, false, errors.New("function ingress: oversized line lacks safe UID")
		}
		capsule.discarding = true
		capsule.discardingUID = strings.Clone(uid)
		capsule.discardingPayload = payload
	} else if len(segment) > 0 {
		line, _, terminated := splitLine(segment)
		if !terminated {
			return false, false, errors.New("function ingress: unterminated line")
		}
		if len(line) > MaximumCommandLineBytes {
			if uid, payload := rejectionHeader(string(line)); uid != "" {
				if rejectErr := capsule.rejectHeader(ctx, consumer, uid, payload); rejectErr != nil {
					return false, false, rejectErr
				}
				return false, false, nil
			}
			return false, false, errors.New("function ingress: oversized command lacks safe UID")
		}
		quit, handleErr := capsule.handleCommandLine(ctx, consumer, string(line))
		if handleErr != nil {
			if uid, payload := rejectionHeader(string(line)); uid != "" {
				if rejectErr := capsule.rejectHeader(ctx, consumer, uid, payload); rejectErr != nil {
					return false, false, rejectErr
				}
				return false, false, nil
			}
			return false, false, handleErr
		}
		if quit {
			return true, false, nil
		}
	}
	if readErr != nil && !errors.Is(readErr, bufio.ErrBufferFull) {
		if errors.Is(readErr, io.EOF) {
			return false, true, nil
		}
		return false, false, fmt.Errorf("function ingress: read: %w", readErr)
	}
	return false, false, nil
}

// DiscardPausedPayload removes parser ownership after process composition has
// fenced the process-fixed reader.
func (capsule *InputCapsule) DiscardPausedPayload() {
	capsule.discarding = false
	capsule.discardingUID = ""
	capsule.discardingPayload = false
	capsule.abortPayload()
}

func hasLF(value []byte) bool {
	return len(value) > 0 && value[len(value)-1] == '\n'
}

func splitLine(line []byte) (content, ending []byte, terminated bool) {
	if !hasLF(line) {
		return line, nil, false
	}
	if len(line) >= 2 && line[len(line)-2] == '\r' {
		return line[:len(line)-2], line[len(line)-2:], true
	}
	return line[:len(line)-1], line[len(line)-1:], true
}

func (capsule *InputCapsule) handleCommandLine(ctx context.Context, consumer Consumer, line string) (bool, error) {
	if line == "" || line == "FUNCTION_PROGRESS" || strings.HasPrefix(line, "FUNCTION_PROGRESS ") {
		return false, nil
	}
	if line == "QUIT" {
		return true, consumer.HandleQuit(ctx)
	}
	fields, err := tokenize(line)
	if err != nil {
		return false, err
	}
	if len(fields) == 2 && fields[0] == "FUNCTION_CANCEL" {
		return false, consumer.HandleCancel(ctx, fields[1])
	}
	call, payload, err := parseCapsuleCall(fields)
	if err != nil {
		return false, err
	}
	if payload {
		capsule.payload = &payloadState{
			call: call,
		}
		return false, nil
	}
	return false, consumer.HandleCall(ctx, call)
}

func rejectionHeader(line string) (uid string, payload bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 || (fields[0] != "FUNCTION" && fields[0] != "FUNCTION_PAYLOAD") {
		return "", false
	}
	uid = fields[1]
	if uid == "" || len(uid) > 128 || strings.ContainsAny(uid, " \t\r\n\x00") {
		return "", false
	}
	return uid, fields[0] == "FUNCTION_PAYLOAD"
}

func (capsule *InputCapsule) rejectHeader(ctx context.Context, consumer Consumer, uid string, payload bool) error {
	if payload {
		// Even an invalid header owns its body until the payload terminator.
		capsule.payload = &payloadState{
			call: Call{
				UID: uid,
			},
			rejectStatus: 400,
		}
		return nil
	}
	return consumer.HandleReject(ctx, uid, 400)
}

func parseCapsuleCall(fields []string) (Call, bool, error) {
	if len(fields) != 6 && len(fields) != 7 {
		return Call{}, false, errors.New("function ingress: invalid command")
	}
	payload := fields[0] == "FUNCTION_PAYLOAD"
	if fields[0] != "FUNCTION" && !payload {
		return Call{}, false, errors.New("function ingress: invalid command")
	}
	if payload && len(fields) != 7 {
		return Call{}, false, errors.New("function ingress: payload content type is missing")
	}
	timeoutSeconds, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || timeoutSeconds < 0 {
		return Call{}, false, errors.New("function ingress: invalid timeout")
	}
	// Callers may send an unset (0) or wrong-unit (e.g. milliseconds) timeout;
	// clamp to the maximum and proceed rather than failing the whole call.
	if maxSeconds := int64(maximumFunctionTimeout / time.Second); timeoutSeconds == 0 || timeoutSeconds > maxSeconds {
		timeoutSeconds = maxSeconds
	}
	// Match the daemon's ASCII word separators; Unicode whitespace is part of
	// the argument and must reach the owner's validation unchanged.
	callFields := strings.FieldsFunc(fields[3], func(r rune) bool {
		switch r {
		case ' ', '\t', '\r', '\n', '\f', '\v':
			return true
		default:
			return false
		}
	})
	if len(callFields) == 0 {
		return Call{}, false, errors.New("function ingress: empty call")
	}
	call := Call{
		UID:        fields[1],
		Timeout:    time.Duration(timeoutSeconds) * time.Second,
		Method:     callFields[0],
		Args:       append([]string(nil), callFields[1:]...),
		Access:     fields[4],
		Source:     fields[5],
		HasPayload: payload,
	}
	if len(fields) == 7 {
		call.ContentType = fields[6]
	}
	return call, payload, nil
}

func (capsule *InputCapsule) handlePayloadSegment(ctx context.Context, consumer Consumer, segment []byte, complete bool) error {
	state := capsule.payload
	content := segment
	var ending []byte
	if complete {
		var terminated bool
		content, ending, terminated = splitLine(segment)
		if !terminated {
			return errors.New("function ingress: unterminated payload line")
		}
	}
	// Commands cannot interleave with a payload; only its framing marker is protocol.
	if complete && !state.lineStarted && bytes.Equal(content, []byte("FUNCTION_PAYLOAD_END")) {
		return capsule.completePayload(ctx, consumer)
	}
	if state.rejectStatus != 0 {
		state.lineStarted = !complete
		return nil
	}
	if !state.lineStarted && state.hasLine {
		capsule.appendPayload(state.separator[:state.separatorBytes])
	}
	capsule.appendPayload(content)
	if complete {
		copy(state.separator[:], ending)
		state.separatorBytes = len(ending)
		state.hasLine = true
		state.lineStarted = false
	} else {
		state.lineStarted = true
	}
	return nil
}

func (capsule *InputCapsule) appendPayload(value []byte) {
	state := capsule.payload
	if state.rejectStatus != 0 {
		return
	}
	if len(value) > MaximumInputBodyBytes-len(state.body) {
		state.body = nil
		state.rejectStatus = 413
		return
	}
	needed := len(state.body) + len(value)
	if needed > cap(state.body) {
		next := min(max(max(cap(state.body)*2, initialBodyCapacity), needed), MaximumInputBodyBytes)
		grown := make([]byte, len(state.body), next)
		copy(grown, state.body)
		state.body = grown
	}
	state.body = append(state.body, value...)
}

func (capsule *InputCapsule) completePayload(ctx context.Context, consumer Consumer) error {
	state := capsule.payload
	if state.rejectStatus != 0 {
		uid := state.call.UID
		capsule.payload = nil
		return consumer.HandleReject(ctx, uid, state.rejectStatus)
	}
	call := state.call
	call.Payload = state.body
	capsule.payload = nil
	return consumer.HandleCall(ctx, call)
}

func (capsule *InputCapsule) abortPayload() {
	capsule.payload = nil
}

func tokenize(line string) ([]string, error) {
	var fields []string
	for offset := 0; offset < len(line); {
		for offset < len(line) && line[offset] == ' ' {
			offset++
		}
		if offset == len(line) {
			break
		}
		if line[offset] != '"' {
			end := strings.IndexByte(line[offset:], ' ')
			if end < 0 {
				fields = append(fields, line[offset:])
				break
			}
			fields = append(fields, line[offset:offset+end])
			offset += end
			continue
		}
		// pluginsd_functions.c quotes sanitized fields literally. Backslashes
		// are data, including immediately before the closing quote.
		end := strings.IndexByte(line[offset+1:], '"')
		if end < 0 {
			return nil, errors.New("function ingress: unterminated quote")
		}
		end += offset + 1
		fields = append(fields, line[offset+1:end])
		offset = end + 1
		if offset < len(line) && line[offset] != ' ' {
			return nil, errors.New("function ingress: missing field separator")
		}
	}
	return fields, nil
}
