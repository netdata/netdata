// SPDX-License-Identifier: GPL-3.0-or-later

package nginx

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
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

func newAPIClient(client *http.Client, request web.Request) *apiClient {
	return &apiClient{httpClient: client, request: request}
}

type apiClient struct {
	httpClient *http.Client
	request    web.Request
}

func (a apiClient) getStubStatus() (*stubStatus, error) {
	req, err := web.NewHTTPRequest(a.request)
	if err != nil {
		return nil, fmt.Errorf("error on creating request : %v", err)
	}

	resp, err := a.doRequestOK(req)
	defer web.CloseBody(resp)
	if err != nil {
		return nil, err
	}

	status, err := parseStubStatus(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error on parsing response : %v", err)
	}

	return status, nil
}

func (a apiClient) doRequestOK(req *http.Request) (*http.Response, error) {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return resp, fmt.Errorf("error on request : %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("%s returned HTTP status %d", req.URL, resp.StatusCode)
	}

	return resp, err
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
