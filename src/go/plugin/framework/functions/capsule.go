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
	MaximumInputLineBytes   = 64 * 1024
	MaximumCommandLineBytes = 15_487
	MaximumInputBodyBytes   = 20 * 1024 * 1024
	MaximumFunctionTimeout  = 15 * time.Minute
	initialBodyCapacity     = 64 * 1024
)

// Call is the immutable Function ingress value transferred to the active
// Agent generation.
type Call struct {
	UID             string
	Timeout         time.Duration
	Method          string
	Args            []string
	Access          string
	Source          string
	Payload         []byte
	ContentType     string
	HasPayload      bool
	InputBodyToken  uint64
	PayloadCapacity int64
}

// BodyBudget owns input-payload reservations while the process-fixed reader
// incrementally parses a payload.
type BodyBudget interface {
	GrowInputBody(context.Context, uint64, int64) (uint64, error)
	CommitInputBodyGrowth(uint64, int64) error
	ReleaseInputBody(uint64) error
}

// ReadReturnGate fences reader returns while process ingress changes Agent
// generation.
type ReadReturnGate interface {
	AcquireInputRead(context.Context, bool) (bool, error)
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
	reader        io.Reader
	budget        BodyBudget
	payload       *payloadState
	discarding    bool
	discardingUID string
}

type ContainedInputCensus struct {
	PayloadActive   bool
	PayloadBytes    int
	PayloadCapacity int
	DiscardingLine  bool
}

type payloadState struct {
	call           Call
	body           []byte
	token          uint64
	capacity       int64
	separator      [2]byte
	separatorBytes int
	hasLine        bool
	lineStarted    bool
	overflow       bool
}

func NewInputCapsule(reader io.Reader, budget BodyBudget) (*InputCapsule, error) {
	if reader == nil || budget == nil {
		return nil, errors.New("Function ingress: incomplete input capsule")
	}
	return &InputCapsule{reader: reader, budget: budget}, nil
}

func (capsule *InputCapsule) Run(ctx context.Context, consumer Consumer) error {
	if ctx == nil || consumer == nil {
		return errors.New("Function ingress: invalid run")
	}
	reader := bufio.NewReaderSize(capsule.reader, MaximumInputLineBytes+1)
	gate, _ := capsule.budget.(ReadReturnGate)
	for {
		segment, readErr := reader.ReadSlice('\n')
		if gate != nil {
			allowed, err := gate.AcquireInputRead(ctx, errors.Is(readErr, bufio.ErrBufferFull))
			if err != nil {
				return err
			}
			if !allowed {
				if readErr != nil && !errors.Is(readErr, bufio.ErrBufferFull) {
					if errors.Is(readErr, io.EOF) {
						return nil
					}
					return fmt.Errorf("Function ingress: read after containment: %w", readErr)
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
			return false, false, errors.Join(errors.New("Function ingress: unterminated payload"), capsule.abortPayload())
		}
		quit, handleErr := capsule.handlePayloadSegment(ctx, consumer, segment, complete)
		if handleErr != nil {
			return false, false, errors.Join(handleErr, capsule.abortPayload())
		}
		if quit {
			return true, false, nil
		}
	} else if capsule.discarding {
		if complete {
			uid := capsule.discardingUID
			capsule.discarding = false
			capsule.discardingUID = ""
			if rejectErr := consumer.HandleReject(ctx, uid, 400); rejectErr != nil {
				return false, false, rejectErr
			}
		}
	} else if !complete {
		uid := rejectionUID(string(segment))
		if uid == "" {
			return false, false, errors.New("Function ingress: oversized line lacks safe UID")
		}
		capsule.discarding = true
		capsule.discardingUID = strings.Clone(uid)
	} else if len(segment) > 0 {
		line, _, terminated := splitLine(segment)
		if !terminated {
			return false, false, errors.New("Function ingress: unterminated line")
		}
		if len(line) > MaximumCommandLineBytes {
			if uid := rejectionUID(string(line)); uid != "" {
				if rejectErr := consumer.HandleReject(ctx, uid, 400); rejectErr != nil {
					return false, false, rejectErr
				}
				return false, false, nil
			}
			return false, false, errors.New("Function ingress: oversized command lacks safe UID")
		}
		quit, handleErr := capsule.handleCommandLine(ctx, consumer, string(line))
		if handleErr != nil {
			if uid := rejectionUID(string(line)); uid != "" {
				if rejectErr := consumer.HandleReject(ctx, uid, 400); rejectErr != nil {
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
		return false, false, fmt.Errorf("Function ingress: read: %w", readErr)
	}
	return false, false, nil
}

// DiscardPausedPayload removes parser ownership after the corresponding body
// reservation has been suspended or released by process composition.
func (capsule *InputCapsule) DiscardPausedPayload(expectedToken uint64) error {
	capsule.discarding = false
	capsule.discardingUID = ""
	if capsule.payload == nil {
		if expectedToken != 0 {
			return errors.New("Function ingress: contained body token has no parser payload")
		}
		return nil
	}
	state := capsule.payload
	capsule.payload = nil
	state.body = nil
	if state.token != expectedToken {
		return errors.New("Function ingress: contained parser/body token mismatch")
	}
	state.token = 0
	state.capacity = 0
	return nil
}

func (capsule *InputCapsule) ContainedCensus() ContainedInputCensus {
	census := ContainedInputCensus{DiscardingLine: capsule.discarding}
	if capsule.payload != nil {
		census.PayloadActive = true
		census.PayloadBytes = len(capsule.payload.body)
		census.PayloadCapacity = cap(capsule.payload.body)
	}
	return census
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
		capsule.payload = &payloadState{call: call}
		return false, nil
	}
	return false, consumer.HandleCall(ctx, call)
}

func rejectionUID(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 2 || (fields[0] != "FUNCTION" && fields[0] != "FUNCTION_PAYLOAD") {
		return ""
	}
	uid := fields[1]
	if uid == "" || len(uid) > 128 || strings.ContainsAny(uid, " \t\r\n\x00") {
		return ""
	}
	return uid
}

func parseCapsuleCall(fields []string) (Call, bool, error) {
	if len(fields) != 6 && len(fields) != 7 {
		return Call{}, false, errors.New("Function ingress: invalid command")
	}
	payload := fields[0] == "FUNCTION_PAYLOAD"
	if fields[0] != "FUNCTION" && !payload {
		return Call{}, false, errors.New("Function ingress: invalid command")
	}
	if payload && len(fields) != 7 {
		return Call{}, false, errors.New("Function ingress: payload content type is missing")
	}
	timeoutSeconds, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil || timeoutSeconds < 0 ||
		timeoutSeconds > int64(MaximumFunctionTimeout/time.Second) {
		return Call{}, false, errors.New("Function ingress: invalid timeout")
	}
	callFields := strings.Fields(fields[3])
	if len(callFields) == 0 {
		return Call{}, false, errors.New("Function ingress: empty call")
	}
	call := Call{
		UID: fields[1], Timeout: time.Duration(timeoutSeconds) * time.Second,
		Method: callFields[0], Args: append([]string(nil), callFields[1:]...),
		Access: fields[4], Source: fields[5], HasPayload: payload,
	}
	if len(fields) == 7 {
		call.ContentType = fields[6]
	}
	return call, payload, nil
}

func (capsule *InputCapsule) handlePayloadSegment(ctx context.Context, consumer Consumer, segment []byte, complete bool) (bool, error) {
	state := capsule.payload
	content := segment
	var ending []byte
	if complete {
		var terminated bool
		content, ending, terminated = splitLine(segment)
		if !terminated {
			return false, errors.New("Function ingress: unterminated payload line")
		}
	}
	if complete && !state.lineStarted {
		switch {
		case bytes.Equal(content, []byte("FUNCTION_PAYLOAD_END")):
			return false, capsule.completePayload(ctx, consumer)
		case bytes.Equal(content, []byte("QUIT")):
			if err := capsule.abortPayload(); err != nil {
				return false, err
			}
			return true, consumer.HandleQuit(ctx)
		case bytes.HasPrefix(content, []byte("FUNCTION_CANCEL")):
			fields := strings.Fields(string(content))
			if len(fields) != 2 || fields[0] != "FUNCTION_CANCEL" {
				return false, errors.New("Function ingress: invalid payload cancel")
			}
			if fields[1] == state.call.UID {
				uid := state.call.UID
				if err := capsule.abortPayload(); err != nil {
					return false, err
				}
				return false, consumer.HandleReject(ctx, uid, 499)
			}
			return false, consumer.HandleCancel(ctx, fields[1])
		case bytes.HasPrefix(content, []byte("FUNCTION_PROGRESS")):
			return false, nil
		case isPayloadInterruptCommand(content):
			if state.overflow {
				if err := consumer.HandleReject(ctx, state.call.UID, 413); err != nil {
					return false, err
				}
			}
			if err := capsule.abortPayload(); err != nil {
				return false, err
			}
			return capsule.handleCommandLine(ctx, consumer, string(content))
		}
	}
	if state.overflow {
		state.lineStarted = !complete
		return false, nil
	}
	if !state.lineStarted && state.hasLine {
		if err := capsule.appendPayload(ctx, state.separator[:state.separatorBytes]); err != nil {
			return false, err
		}
	}
	if err := capsule.appendPayload(ctx, content); err != nil {
		return false, err
	}
	if complete {
		copy(state.separator[:], ending)
		state.separatorBytes = len(ending)
		state.hasLine = true
		state.lineStarted = false
	} else {
		state.lineStarted = true
	}
	return false, nil
}

func isPayloadInterruptCommand(line []byte) bool {
	return bytes.Equal(line, []byte("FUNCTION")) ||
		bytes.Equal(line, []byte("FUNCTION_PAYLOAD")) ||
		bytes.HasPrefix(line, []byte("FUNCTION ")) ||
		bytes.HasPrefix(line, []byte("FUNCTION_PAYLOAD ")) ||
		bytes.HasPrefix(line, []byte("FUNCTION_"))
}

func (capsule *InputCapsule) appendPayload(ctx context.Context, value []byte) error {
	state := capsule.payload
	if state.overflow {
		return nil
	}
	if len(value) > MaximumInputBodyBytes-len(state.body) {
		if state.token != 0 {
			if err := capsule.budget.ReleaseInputBody(state.token); err != nil {
				return err
			}
		}
		state.body = nil
		state.token = 0
		state.capacity = 0
		state.overflow = true
		return nil
	}
	needed := len(state.body) + len(value)
	if int64(needed) > state.capacity {
		next := min(max(max(state.capacity*2, initialBodyCapacity), int64(needed)), MaximumInputBodyBytes)
		token, err := capsule.budget.GrowInputBody(ctx, state.token, next)
		if err != nil {
			return err
		}
		grown := make([]byte, len(state.body), int(next))
		copy(grown, state.body)
		state.body = grown
		state.token = token
		state.capacity = next
		if err := capsule.budget.CommitInputBodyGrowth(token, next); err != nil {
			_ = capsule.budget.ReleaseInputBody(token)
			state.body = nil
			state.token = 0
			state.capacity = 0
			return err
		}
	}
	state.body = append(state.body, value...)
	return nil
}

func (capsule *InputCapsule) completePayload(ctx context.Context, consumer Consumer) error {
	state := capsule.payload
	if state.overflow {
		uid := state.call.UID
		capsule.payload = nil
		return consumer.HandleReject(ctx, uid, 413)
	}
	call := state.call
	call.Payload = state.body
	call.InputBodyToken = state.token
	call.PayloadCapacity = state.capacity
	state.token = 0
	capsule.payload = nil
	return consumer.HandleCall(ctx, call)
}

func (capsule *InputCapsule) abortPayload() error {
	if capsule.payload == nil {
		return nil
	}
	state := capsule.payload
	capsule.payload = nil
	if state.token == 0 {
		return nil
	}
	return capsule.budget.ReleaseInputBody(state.token)
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
		end := offset + 1
		escaped := false
		for ; end < len(line); end++ {
			if escaped {
				escaped = false
				continue
			}
			if line[end] == '\\' {
				escaped = true
				continue
			}
			if line[end] == '"' {
				break
			}
		}
		if end == len(line) {
			return nil, errors.New("Function ingress: unterminated quote")
		}
		value, err := strconv.Unquote(line[offset : end+1])
		if err != nil {
			return nil, errors.New("Function ingress: invalid quoted field")
		}
		fields = append(fields, value)
		offset = end + 1
		if offset < len(line) && line[offset] != ' ' {
			return nil, errors.New("Function ingress: missing field separator")
		}
	}
	return fields, nil
}
