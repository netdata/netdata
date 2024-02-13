// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/netdata/go.d.plugin/pkg/web"

	fcgiclient "github.com/tomasen/fcgi_client"
)

type (
	status struct {
		Active    int64  `json:"active processes" stm:"active"`
		MaxActive int64  `json:"max active processes" stm:"maxActive"`
		Idle      int64  `json:"idle processes" stm:"idle"`
		Requests  int64  `json:"accepted conn" stm:"requests"`
		Reached   int64  `json:"max children reached" stm:"reached"`
		Slow      int64  `json:"slow requests" stm:"slow"`
		Processes []proc `json:"processes"`
	}
	proc struct {
		PID      int64           `json:"pid"`
		State    string          `json:"state"`
		Duration requestDuration `json:"request duration"`
		CPU      float64         `json:"last request cpu"`
		Memory   int64           `json:"last request memory"`
	}
	requestDuration int64
)

// UnmarshalJSON customise JSON for timestamp.
func (rd *requestDuration) UnmarshalJSON(b []byte) error {
	if rdc, err := strconv.Atoi(string(b)); err != nil {
		*rd = 0
	} else {
		*rd = requestDuration(rdc)
	}
	return nil
}

type client interface {
	getStatus() (*status, error)
}

type httpClient struct {
	client *http.Client
	req    web.Request
	dec    decoder
}

func newHTTPClient(c *http.Client, r web.Request) (*httpClient, error) {
	u, err := url.Parse(r.URL)
	if err != nil {
		return nil, err
	}

	dec := decodeText
	if _, ok := u.Query()["json"]; ok {
		dec = decodeJSON
	}
	return &httpClient{
		client: c,
		req:    r,
		dec:    dec,
	}, nil
}

func (c *httpClient) getStatus() (*status, error) {
	req, err := web.NewHTTPRequest(c.req)
	if err != nil {
		return nil, fmt.Errorf("error on creating HTTP request: %v", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error on HTTP request to '%s': %v", req.URL, err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP status %d", req.URL, resp.StatusCode)
	}

	st := &status{}
	if err := c.dec(resp.Body, st); err != nil {
		return nil, fmt.Errorf("error parsing HTTP response from '%s': %v", req.URL, err)
	}
	return st, nil
}

type socketClient struct {
	socket  string
	timeout time.Duration
	env     map[string]string
}

func newSocketClient(socket string, timeout time.Duration, fcgiPath string) *socketClient {
	return &socketClient{
		socket:  socket,
		timeout: timeout,
		env: map[string]string{
			"SCRIPT_NAME":     fcgiPath,
			"SCRIPT_FILENAME": fcgiPath,
			"SERVER_SOFTWARE": "go / fcgiclient ",
			"REMOTE_ADDR":     "127.0.0.1",
			"QUERY_STRING":    "json&full",
			"REQUEST_METHOD":  "GET",
			"CONTENT_TYPE":    "application/json",
		},
	}
}

func (c *socketClient) getStatus() (*status, error) {
	socket, err := fcgiclient.DialTimeout("unix", c.socket, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("error on connecting to socket '%s': %v", c.socket, err)
	}
	defer socket.Close()

	resp, err := socket.Get(c.env)
	if err != nil {
		return nil, fmt.Errorf("error on getting data from socket '%s': %v", c.socket, err)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error on reading response from socket '%s': %v", c.socket, err)
	}

	st := &status{}
	if err := json.Unmarshal(content, st); err != nil {
		return nil, fmt.Errorf("error on decoding response from socket '%s': %v", c.socket, err)
	}
	return st, nil
}

type tcpClient struct {
	address string
	timeout time.Duration
	env     map[string]string
}

func newTcpClient(address string, timeout time.Duration, fcgiPath string) *tcpClient {
	return &tcpClient{
		address: address,
		timeout: timeout,
		env: map[string]string{
			"SCRIPT_NAME":     fcgiPath,
			"SCRIPT_FILENAME": fcgiPath,
			"SERVER_SOFTWARE": "go / fcgiclient ",
			"REMOTE_ADDR":     "127.0.0.1",
			"QUERY_STRING":    "json&full",
			"REQUEST_METHOD":  "GET",
			"CONTENT_TYPE":    "application/json",
		},
	}
}

func (c *tcpClient) getStatus() (*status, error) {
	client, err := fcgiclient.DialTimeout("tcp", c.address, c.timeout)
	if err != nil {
		return nil, fmt.Errorf("error on connecting to address '%s': %v", c.address, err)
	}
	defer client.Close()

	resp, err := client.Get(c.env)
	if err != nil {
		return nil, fmt.Errorf("error on getting data from address '%s': %v", c.address, err)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error on reading response from address '%s': %v", c.address, err)
	}

	st := &status{}
	if err := json.Unmarshal(content, st); err != nil {
		return nil, fmt.Errorf("error on decoding response from address '%s': %v", c.address, err)
	}
	return st, nil
}
