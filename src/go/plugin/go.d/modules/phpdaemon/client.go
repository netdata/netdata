// SPDX-License-Identifier: GPL-3.0-or-later

package phpdaemon

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

type decodeFunc func(dst interface{}, reader io.Reader) error

func decodeJson(dst interface{}, reader io.Reader) error { return json.NewDecoder(reader).Decode(dst) }

func newAPIClient(httpClient *http.Client, request web.Request) *client {
	return &client{
		httpClient: httpClient,
		request:    request,
	}
}

type client struct {
	httpClient *http.Client
	request    web.Request
}

func (c *client) queryFullStatus() (*FullStatus, error) {
	var status FullStatus
	err := c.doWithDecode(&status, decodeJson, c.request)
	if err != nil {
		return nil, err
	}

	return &status, nil
}

func (c *client) doWithDecode(dst interface{}, decode decodeFunc, request web.Request) error {
	req, err := web.NewHTTPRequest(request)
	if err != nil {
		return fmt.Errorf("error on creating http request to %s : %v", request.URL, err)
	}

	resp, err := c.doOK(req)
	defer closeBody(resp)
	if err != nil {
		return err
	}

	if err = decode(dst, resp.Body); err != nil {
		return fmt.Errorf("error on parsing response from %s : %v", req.URL, err)
	}

	return nil
}

func (c *client) doOK(req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return resp, fmt.Errorf("error on request : %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("%s returned HTTP status %d", req.URL, resp.StatusCode)
	}

	return resp, err
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
