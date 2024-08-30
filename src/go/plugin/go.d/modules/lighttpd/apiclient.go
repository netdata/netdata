// SPDX-License-Identifier: GPL-3.0-or-later

package lighttpd

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
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

func newAPIClient(client *http.Client, request web.Request) *apiClient {
	return &apiClient{httpClient: client, request: request}
}

type apiClient struct {
	httpClient *http.Client
	request    web.Request
}

func (a apiClient) getServerStatus() (*serverStatus, error) {
	req, err := web.NewHTTPRequest(a.request)

	if err != nil {
		return nil, fmt.Errorf("error on creating request : %v", err)
	}

	resp, err := a.doRequestOK(req)

	defer closeBody(resp)

	if err != nil {
		return nil, err
	}

	status, err := parseResponse(resp.Body)

	if err != nil {
		return nil, fmt.Errorf("error on parsing response from %s : %v", req.URL, err)
	}

	return status, nil
}

func (a apiClient) doRequestOK(req *http.Request) (*http.Response, error) {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error on request : %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("%s returned HTTP status %d", req.URL, resp.StatusCode)
	}
	return resp, nil
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

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
