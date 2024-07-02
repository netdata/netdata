// SPDX-License-Identifier: GPL-3.0-or-later

package apache

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (a *Apache) collect() (map[string]int64, error) {
	status, err := a.scrapeStatus()
	if err != nil {
		return nil, err
	}

	mx := stm.ToMap(status)
	if len(mx) == 0 {
		return nil, fmt.Errorf("nothing was collected from %s", a.URL)
	}

	a.once.Do(func() { a.charts = newCharts(status) })

	return mx, nil
}

func (a *Apache) scrapeStatus() (*serverStatus, error) {
	req, err := web.NewHTTPRequest(a.Request)
	if err != nil {
		return nil, err
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error on HTTP request '%s': %v", req.URL, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	return parseResponse(resp.Body)
}

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
		case "BusyServers", "IdleServers":
			return nil, fmt.Errorf("found '%s', Lighttpd data", key)
		case "BusyWorkers":
			status.Workers.Busy = parseInt(value)
		case "IdleWorkers":
			status.Workers.Idle = parseInt(value)
		case "ConnsTotal":
			status.Connections.Total = parseInt(value)
		case "ConnsAsyncWriting":
			status.Connections.Async.Writing = parseInt(value)
		case "ConnsAsyncKeepAlive":
			status.Connections.Async.KeepAlive = parseInt(value)
		case "ConnsAsyncClosing":
			status.Connections.Async.Closing = parseInt(value)
		case "Total Accesses":
			status.Total.Accesses = parseInt(value)
		case "Total kBytes":
			status.Total.KBytes = parseInt(value)
		case "Uptime":
			status.Uptime = parseInt(value)
		case "ReqPerSec":
			status.Averages.ReqPerSec = parseFloat(value)
		case "BytesPerSec":
			status.Averages.BytesPerSec = parseFloat(value)
		case "BytesPerReq":
			status.Averages.BytesPerReq = parseFloat(value)
		case "Scoreboard":
			status.Scoreboard = parseScoreboard(value)
		}
	}

	return &status, nil
}

func parseScoreboard(line string) *scoreboard {
	//  “_” Waiting for Connection
	// “S” Starting up
	// “R” Reading Request
	// “W” Sending Reply
	// “K” Keepalive (read)
	// “D” DNS Lookup
	// “C” Closing connection
	// “L” Logging
	// “G” Gracefully finishing
	// “I” Idle cleanup of worker
	// “.” Open slot with no current process
	var sb scoreboard
	for _, s := range strings.Split(line, "") {
		switch s {
		case "_":
			sb.Waiting++
		case "S":
			sb.Starting++
		case "R":
			sb.Reading++
		case "W":
			sb.Sending++
		case "K":
			sb.KeepAlive++
		case "D":
			sb.DNSLookup++
		case "C":
			sb.Closing++
		case "L":
			sb.Logging++
		case "G":
			sb.Finishing++
		case "I":
			sb.IdleCleanup++
		case ".":
			sb.Open++
		}
	}
	return &sb
}

func parseInt(value string) *int64 {
	v, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return nil
	}
	return &v
}

func parseFloat(value string) *float64 {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil
	}
	return &v
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
