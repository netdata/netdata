// SPDX-License-Identifier: GPL-3.0-or-later

package hdfs

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func newClient(httpClient *http.Client, request web.Request) *client {
	return &client{
		httpClient: httpClient,
		request:    request,
	}
}

type client struct {
	httpClient *http.Client
	request    web.Request
}

func (c *client) do() (*http.Response, error) {
	req, err := web.NewHTTPRequest(c.request)
	if err != nil {
		return nil, fmt.Errorf("error on creating http request to %s : %v", c.request.URL, err)
	}

	// req.Header.Add("Accept-Encoding", "gzip")
	// req.Header.Set("User-Agent", "netdata/go.d.plugin")

	return c.httpClient.Do(req)
}

func (c *client) doOK() (*http.Response, error) {
	resp, err := c.do()
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("%s returned %d", c.request.URL, resp.StatusCode)
	}
	return resp, nil
}

func (c *client) doOKWithDecodeJSON(dst interface{}) error {
	resp, err := c.doOK()
	defer web.CloseBody(resp)
	if err != nil {
		return err
	}

	err = json.NewDecoder(resp.Body).Decode(dst)
	if err != nil {
		return fmt.Errorf("error on decoding response from %s : %v", c.request.URL, err)
	}
	return nil
}
