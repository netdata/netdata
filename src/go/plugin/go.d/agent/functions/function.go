// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"
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

func parseFunction(s string) (*Function, error) {
	r := csv.NewReader(strings.NewReader(s))
	r.Comma = ' '

	parts, err := r.Read()
	if err != nil {
		return nil, err
	}

	// FUNCTION UID Timeout "Name ...Parameters" 0xPermissions "SourceType" [ContentType]
	if n := len(parts); n != 6 && n != 7 {
		return nil, fmt.Errorf("unexpected number of words: want 6 or 7, got %d (%v)", n, parts)
	}

	timeout, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, err
	}

	cmd := strings.Split(parts[3], " ")

	fn := &Function{
		key:         parts[0],
		UID:         parts[1],
		Timeout:     time.Duration(timeout) * time.Second,
		Name:        cmd[0],
		Args:        cmd[1:],
		Permissions: parts[4],
		Source:      parts[5],
	}

	if len(parts) == 7 {
		fn.ContentType = parts[6]
	}

	return fn, nil
}

func parseFunctionWithPayload(ctx context.Context, s string, in input) (*Function, error) {
	fn, err := parseFunction(s)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer

	for {
		select {
		case <-ctx.Done():
			return nil, nil
		case line, ok := <-in.lines():
			if !ok {
				return nil, nil
			}
			if line == "FUNCTION_PAYLOAD_END" {
				fn.Payload = append(fn.Payload, buf.Bytes()...)
				return fn, nil
			}
			if buf.Len() > 0 {
				buf.WriteString("\n")
			}
			buf.WriteString(line)
		}
	}
}
