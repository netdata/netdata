// SPDX-License-Identifier: GPL-3.0-or-later

package lighttpd

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	busyWorkers = "BusyWorkers"
	idleWorkers = "IdleWorkers"

	busyServers   = "BusyServers"
	idleServers   = "IdleServers"
	totalAccesses = "Total Accesses"
	totalkBytes   = "Total kBytes"
	uptime        = "Uptime"
	scoreBoard    = "Scoreboard"
)

type (
	serverStatus struct {
		Total struct {
			Accesses *int64 `stm:"accesses"`
			KBytes   *int64 `stm:"kBytes"`
		} `stm:"total"`
		Servers struct {
			Busy *int64 `stm:"busy_servers"`
			Idle *int64 `stm:"idle_servers"`
		} `stm:""`
		Uptime     *int64      `stm:"uptime"`
		Scoreboard *scoreboard `stm:"scoreboard"`
	}
	scoreboard struct {
		Waiting       int64 `stm:"waiting"`
		Open          int64 `stm:"open"`
		Close         int64 `stm:"close"`
		HardError     int64 `stm:"hard_error"`
		KeepAlive     int64 `stm:"keepalive"`
		Read          int64 `stm:"read"`
		ReadPost      int64 `stm:"read_post"`
		Write         int64 `stm:"write"`
		HandleRequest int64 `stm:"handle_request"`
		RequestStart  int64 `stm:"request_start"`
		RequestEnd    int64 `stm:"request_end"`
		ResponseStart int64 `stm:"response_start"`
		ResponseEnd   int64 `stm:"response_end"`
	}
)

func parseResponse(r io.Reader) (*serverStatus, error) {
	s := bufio.NewScanner(r)
	var status serverStatus

	for s.Scan() {
		parts := strings.Split(s.Text(), ":")
		if len(parts) != 2 {
			continue
		}
		key, value := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])

		switch key {
		default:
		case busyWorkers, idleWorkers:
			return nil, fmt.Errorf("found '%s', apache data", key)
		case busyServers:
			status.Servers.Busy = mustParseInt(value)
		case idleServers:
			status.Servers.Idle = mustParseInt(value)
		case totalAccesses:
			status.Total.Accesses = mustParseInt(value)
		case totalkBytes:
			status.Total.KBytes = mustParseInt(value)
		case uptime:
			status.Uptime = mustParseInt(value)
		case scoreBoard:
			status.Scoreboard = parseScoreboard(value)
		}
	}

	return &status, nil
}

func parseScoreboard(value string) *scoreboard {
	// Descriptions from https://blog.serverdensity.com/monitor-lighttpd/
	//
	// “.” = Opening the TCP connection (connect)
	// “C” = Closing the TCP connection if no other HTTP request will use it (close)
	// “E” = hard error
	// “k” = Keeping the TCP connection open for more HTTP requests from the same client to avoid the TCP handling overhead (keep-alive)
	// “r” = ReadAsMap the content of the HTTP request (read)
	// “R” = ReadAsMap the content of the HTTP request (read-POST)
	// “W” = Write the HTTP response to the socket (write)
	// “h” = Decide action to take with the request (handle-request)
	// “q” = Start of HTTP request (request-start)
	// “Q” = End of HTTP request (request-end)
	// “s” = Start of the HTTP request response (response-start)
	// “S” = End of the HTTP request response (response-end)
	// “_” Waiting for Connection (NOTE: not sure, copied the description from apache score board)

	var sb scoreboard
	for _, s := range strings.Split(value, "") {
		switch s {
		case "_":
			sb.Waiting++
		case ".":
			sb.Open++
		case "C":
			sb.Close++
		case "E":
			sb.HardError++
		case "k":
			sb.KeepAlive++
		case "r":
			sb.Read++
		case "R":
			sb.ReadPost++
		case "W":
			sb.Write++
		case "h":
			sb.HandleRequest++
		case "q":
			sb.RequestStart++
		case "Q":
			sb.RequestEnd++
		case "s":
			sb.ResponseStart++
		case "S":
			sb.ResponseEnd++
		}
	}

	return &sb
}

func mustParseInt(value string) *int64 {
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		panic(err)
	}
	return &v
}
