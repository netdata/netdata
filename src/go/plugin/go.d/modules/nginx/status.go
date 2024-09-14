// SPDX-License-Identifier: GPL-3.0-or-later

package nginx

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

const (
	connActive  = "connActive"
	connAccepts = "connAccepts"
	connHandled = "connHandled"
	requests    = "requests"
	requestTime = "requestTime"
	connReading = "connReading"
	connWriting = "connWriting"
	connWaiting = "connWaiting"
)

var (
	nginxSeq = []string{
		connActive,
		connAccepts,
		connHandled,
		requests,
		connReading,
		connWriting,
		connWaiting,
	}
	tengineSeq = []string{
		connActive,
		connAccepts,
		connHandled,
		requests,
		requestTime,
		connReading,
		connWriting,
		connWaiting,
	}

	reStatus = regexp.MustCompile(`^Active connections: ([0-9]+)\n[^\d]+([0-9]+) ([0-9]+) ([0-9]+) ?([0-9]+)?\nReading: ([0-9]+) Writing: ([0-9]+) Waiting: ([0-9]+)`)
)

type stubStatus struct {
	Connections struct {
		// The current number of active client connections including Waiting connections.
		Active int64 `stm:"active"`

		// The total number of accepted client connections.
		Accepts int64 `stm:"accepts"`

		// The total number of handled connections.
		// Generally, the parameter value is the same as accepts unless some resource limits have been reached.
		Handled int64 `stm:"handled"`

		// The current number of connections where nginx is reading the request header.
		Reading int64 `stm:"reading"`

		// The current number of connections where nginx is writing the response back to the client.
		Writing int64 `stm:"writing"`

		// The current number of idle client connections waiting for a request.
		Waiting int64 `stm:"waiting"`
	} `stm:""`
	Requests struct {
		// The total number of client requests.
		Total int64 `stm:"requests"`

		// Note: tengine specific
		// The total requests' response time, which is in millisecond
		Time *int64 `stm:"request_time"`
	} `stm:""`
}

func parseStubStatus(r io.Reader) (*stubStatus, error) {
	sc := bufio.NewScanner(r)
	var lines []string

	for sc.Scan() {
		lines = append(lines, strings.Trim(sc.Text(), "\r\n "))
	}

	parsed := reStatus.FindStringSubmatch(strings.Join(lines, "\n"))

	if len(parsed) == 0 {
		return nil, fmt.Errorf("can't parse '%v'", lines)
	}

	parsed = parsed[1:]

	var (
		seq    []string
		status stubStatus
	)

	switch len(parsed) {
	default:
		return nil, fmt.Errorf("invalid number of fields, got %d, expect %d or %d", len(parsed), len(nginxSeq), len(tengineSeq))
	case len(nginxSeq):
		seq = nginxSeq
	case len(tengineSeq):
		seq = tengineSeq
	}

	for i, key := range seq {
		strValue := parsed[i]
		if strValue == "" {
			continue
		}
		value := mustParseInt(strValue)
		switch key {
		default:
			return nil, fmt.Errorf("unknown key in seq : %s", key)
		case connActive:
			status.Connections.Active = value
		case connAccepts:
			status.Connections.Accepts = value
		case connHandled:
			status.Connections.Handled = value
		case requests:
			status.Requests.Total = value
		case connReading:
			status.Connections.Reading = value
		case connWriting:
			status.Connections.Writing = value
		case connWaiting:
			status.Connections.Waiting = value
		case requestTime:
			status.Requests.Time = &value
		}
	}

	return &status, nil
}

func mustParseInt(value string) int64 {
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		panic(err)
	}
	return v
}
