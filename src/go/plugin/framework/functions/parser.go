// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	lineFunction           = "FUNCTION"
	lineFunctionPayload    = "FUNCTION_PAYLOAD"
	lineFunctionPayloadEnd = "FUNCTION_PAYLOAD_END"
	lineFunctionCancel     = "FUNCTION_CANCEL"
	lineFunctionProgress   = "FUNCTION_PROGRESS"
	lineQuit               = "QUIT"
)

type Function struct {
	key         string
	UID         string
	Timeout     time.Duration
	Name        string
	Args        []string
	Payload     []byte
	Permissions string
	Source      string
	ContentType string
}

func (f *Function) String() string {
	return fmt.Sprintf("key: '%s', uid: '%s', timeout: '%s', function: '%s', args: '%v', permissions: '%s', source: '%s',  contentType: '%s', payload: '%s'",
		f.key, f.UID, f.Timeout, f.Name, f.Args, f.Permissions, f.Source, f.ContentType, string(f.Payload))
}

func newInputParser() *inputParser {
	return &inputParser{}
}

type inputParser struct {
	currentFn      *Function
	readingPayload bool
	payloadBuf     bytes.Buffer
}

func (p *inputParser) parse(line string) (*Function, error) {
	event, err := p.parseEvent(line)
	if err != nil {
		return nil, err
	}
	if event.kind == inputEventCall {
		return event.fn, nil
	}
	return nil, nil
}

type inputEventKind uint8

const (
	inputEventNone inputEventKind = iota
	inputEventCall
	inputEventCancel
	inputEventProgress
	inputEventQuit
)

type inputEvent struct {
	kind         inputEventKind
	fn           *Function
	uid          string
	preAdmission bool
}

func (p *inputParser) parseEvent(line string) (inputEvent, error) {
	if line = strings.TrimSpace(line); line == "" {
		return inputEvent{}, nil
	}

	if p.readingPayload {
		return p.handlePayloadLine(line)
	}

	switch {
	case line == lineQuit:
		return inputEvent{kind: inputEventQuit}, nil
	case hasLinePrefix(line, lineFunctionCancel):
		return parseCancelEvent(line)
	case hasLinePrefix(line, lineFunctionProgress):
		return parseProgressEvent(line), nil
	case strings.HasPrefix(line, lineFunction+" "):
		fn, err := p.parseFunction(line)
		if err != nil {
			return inputEvent{}, err
		}
		return inputEvent{kind: inputEventCall, fn: fn}, nil
	case strings.HasPrefix(line, lineFunctionPayload+" "):
		fn, err := p.parseFunction(line)
		if err != nil {
			return inputEvent{}, err
		}
		p.readingPayload = true
		p.currentFn = fn
		p.payloadBuf.Reset()
		return inputEvent{}, nil
	default:
		return inputEvent{}, errors.New("unexpected line format")
	}
}

func (p *inputParser) handlePayloadLine(line string) (inputEvent, error) {
	if line == lineFunctionPayloadEnd {
		p.readingPayload = false
		p.currentFn.Payload = []byte(p.payloadBuf.String())
		fn := p.currentFn
		p.currentFn = nil
		p.payloadBuf.Reset()
		return inputEvent{kind: inputEventCall, fn: fn}, nil
	}

	if hasLinePrefix(line, lineFunctionCancel) {
		event, err := parseCancelEvent(line)
		if err != nil {
			// Malformed cancel must not affect payload parser state.
			return inputEvent{}, err
		}

		if p.currentFn != nil && event.uid == p.currentFn.UID {
			p.resetPayloadState()
			event.preAdmission = true
		}
		return event, nil
	}

	if hasLinePrefix(line, lineFunctionProgress) {
		return parseProgressEvent(line), nil
	}

	if line == lineQuit {
		p.resetPayloadState()
		return inputEvent{kind: inputEventQuit}, nil
	}

	if hasLinePrefix(line, lineFunction) || strings.HasPrefix(line, lineFunction+"_") {
		p.resetPayloadState()
		return p.parseEvent(line)
	}

	if p.payloadBuf.Len() > 0 {
		p.payloadBuf.WriteByte('\n')
	}
	p.payloadBuf.WriteString(line)

	return inputEvent{}, nil
}

func (p *inputParser) resetPayloadState() {
	p.readingPayload = false
	p.currentFn = nil
	p.payloadBuf.Reset()
}

func hasLinePrefix(line, keyword string) bool {
	return line == keyword || strings.HasPrefix(line, keyword+" ")
}

func parseCancelEvent(line string) (inputEvent, error) {
	parts := strings.Fields(line)
	if len(parts) != 2 || parts[0] != lineFunctionCancel || parts[1] == "" {
		return inputEvent{}, errors.New("unexpected FUNCTION_CANCEL format")
	}
	return inputEvent{kind: inputEventCancel, uid: parts[1]}, nil
}

func parseProgressEvent(line string) inputEvent {
	parts := strings.Fields(line)
	event := inputEvent{kind: inputEventProgress}
	if len(parts) >= 2 {
		event.uid = parts[1]
	}
	return event
}

func (p *inputParser) parseFunction(line string) (*Function, error) {
	r := csv.NewReader(strings.NewReader(line))
	r.Comma = ' '

	parts, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	if n := len(parts); n != 6 && n != 7 {
		return nil, fmt.Errorf("unexpected number of parts: want 6 or 7, got %d", n)
	}

	timeout, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid timeout value: %w", err)
	}

	nameAndArgs := strings.Split(parts[3], " ")
	if len(nameAndArgs) == 0 {
		return nil, fmt.Errorf("empty function name and arguments")
	}

	fn := &Function{
		key:         parts[0],
		UID:         parts[1],
		Timeout:     time.Duration(timeout) * time.Second,
		Name:        nameAndArgs[0],
		Args:        nameAndArgs[1:],
		Permissions: parts[4],
		Source:      parts[5],
	}

	if len(parts) == 7 {
		fn.ContentType = parts[6]
	}

	return fn, nil
}
