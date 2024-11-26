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

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	fcgiclient "github.com/kanocz/fcgi_client"
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
	req    web.RequestConfig
	dec    decoder
}

func newHTTPClient(c *http.Client, r web.RequestConfig) (*httpClient, error) {
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
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	st := &status{}

	if err := web.DoHTTP(c.client).Request(req, func(body io.Reader) error {
		return c.dec(body, st)
	}); err != nil {
		return nil, err
	}

	return st, nil
}

type socketClient struct {
	*logger.Logger

	socket  string
	timeout time.Duration
	env     map[string]string
}

func newSocketClient(log *logger.Logger, socket string, timeout time.Duration, fcgiPath string) *socketClient {
	return &socketClient{
		Logger:  log,
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

	if err := socket.SetTimeout(c.timeout); err != nil {
		return nil, fmt.Errorf("error on setting socket timeout: %v", err)
	}

	resp, err := socket.Get(c.env)
	if err != nil {
		return nil, fmt.Errorf("error on getting data from socket '%s': %v", c.socket, err)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error on reading response from socket '%s': %v", c.socket, err)
	}

	if len(content) == 0 {
		return nil, fmt.Errorf("no data returned from socket '%s'", c.socket)
	}

	st := &status{}
	if err := json.Unmarshal(content, st); err != nil {
		c.Debugf("failed to JSON decode data: %s", string(content))
		return nil, fmt.Errorf("error on decoding response from socket '%s': %v", c.socket, err)
	}

	return st, nil
}

type tcpClient struct {
	*logger.Logger

	address string
	timeout time.Duration
	env     map[string]string
}

func newTcpClient(log *logger.Logger, address string, timeout time.Duration, fcgiPath string) *tcpClient {
	return &tcpClient{
		Logger:  log,
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

	if len(content) == 0 {
		return nil, fmt.Errorf("no data returned from address '%s'", c.address)
	}

	st := &status{}
	if err := json.Unmarshal(content, st); err != nil {
		c.Debugf("failed to JSON decode data: %s", string(content))
		return nil, fmt.Errorf("error on decoding response from address '%s': %v", c.address, err)
	}

	return st, nil
}
