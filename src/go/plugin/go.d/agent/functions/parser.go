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
	if line = strings.TrimSpace(line); line == "" {
		return nil, nil
	}

	if p.readingPayload {
		return p.handlePayloadLine(line)
	}

	switch {
	case strings.HasPrefix(line, lineFunction+" "):
		return p.parseFunction(line)
	case strings.HasPrefix(line, lineFunctionPayload+" "):
		fn, err := p.parseFunction(line)
		if err != nil {
			return nil, err
		}
		p.readingPayload = true
		p.currentFn = fn
		p.payloadBuf.Reset()
		return nil, nil
	default:
		return nil, errors.New("unexpected line format")
	}
}

func (p *inputParser) handlePayloadLine(line string) (*Function, error) {
	if line == lineFunctionPayloadEnd {
		p.readingPayload = false
		p.currentFn.Payload = []byte(p.payloadBuf.String())
		fn := p.currentFn
		p.currentFn = nil
		return fn, nil
	}

	if strings.HasPrefix(line, lineFunction) {
		p.readingPayload = false
		p.currentFn = nil
		p.payloadBuf.Reset()
		return p.parse(line)
	}

	if p.payloadBuf.Len() > 0 {
		p.payloadBuf.WriteByte('\n')
	}
	p.payloadBuf.WriteString(line)

	return nil, nil
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
