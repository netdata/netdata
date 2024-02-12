// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Function struct {
	key     string
	UID     string
	Timeout time.Duration
	Name    string
	Args    []string
	Payload []byte
}

func (f *Function) String() string {
	return fmt.Sprintf("key: %s, uid: %s, timeout: %s, function: %s, args: %v, payload: %s",
		f.key, f.UID, f.Timeout, f.Name, f.Args, string(f.Payload))
}

func parseFunction(s string) (*Function, error) {
	r := csv.NewReader(strings.NewReader(s))
	r.Comma = ' '

	parts, err := r.Read()
	if err != nil {
		return nil, err
	}
	if len(parts) != 4 {
		return nil, fmt.Errorf("unexpected number of words: want 4, got %d (%v)", len(parts), parts)
	}

	timeout, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return nil, err
	}

	cmd := strings.Split(parts[3], " ")

	fn := &Function{
		key:     parts[0],
		UID:     parts[1],
		Timeout: time.Duration(timeout) * time.Second,
		Name:    cmd[0],
		Args:    cmd[1:],
	}

	return fn, nil
}

func parseFunctionWithPayload(s string, sc *bufio.Scanner) (*Function, error) {
	fn, err := parseFunction(s)
	if err != nil {
		return nil, err
	}

	var n int
	var buf bytes.Buffer
	for sc.Scan() && sc.Text() != "FUNCTION_PAYLOAD_END" {
		if n++; n > 1 {
			buf.WriteString("\n")
		}
		buf.WriteString(sc.Text())
	}

	fn.Payload = append(fn.Payload, buf.Bytes()...)

	return fn, nil
}
