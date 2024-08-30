// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const pluginsPath = "/api/plugins.json"

type pluginsInfo struct {
	Payload []pluginData `json:"plugins"`
}

type pluginData struct {
	ID                    string `json:"plugin_id"`
	Type                  string `json:"type"`
	Category              string `json:"plugin_category"`
	RetryCount            *int64 `json:"retry_count"`
	BufferTotalQueuedSize *int64 `json:"buffer_total_queued_size"`
	BufferQueueLength     *int64 `json:"buffer_queue_length"`
}

func (p pluginData) hasCategory() bool {
	return p.RetryCount != nil
}

func (p pluginData) hasBufferQueueLength() bool {
	return p.BufferQueueLength != nil
}

func (p pluginData) hasBufferTotalQueuedSize() bool {
	return p.BufferTotalQueuedSize != nil
}

func newAPIClient(client *http.Client, request web.Request) *apiClient {
	return &apiClient{httpClient: client, request: request}
}

type apiClient struct {
	httpClient *http.Client
	request    web.Request
}

func (a apiClient) getPluginsInfo() (*pluginsInfo, error) {
	req, err := a.createRequest(pluginsPath)
	if err != nil {
		return nil, fmt.Errorf("error on creating request : %v", err)
	}

	resp, err := a.doRequestOK(req)
	defer closeBody(resp)
	if err != nil {
		return nil, err
	}

	var info pluginsInfo
	if err = json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("error on decoding response from %s : %v", req.URL, err)
	}

	return &info, nil
}

func (a apiClient) doRequestOK(req *http.Request) (*http.Response, error) {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error on request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("%s returned HTTP status %d", req.URL, resp.StatusCode)
	}
	return resp, nil
}

func (a apiClient) createRequest(urlPath string) (*http.Request, error) {
	req := a.request.Copy()
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, err
	}

	u.Path = path.Join(u.Path, urlPath)
	req.URL = u.String()
	return web.NewHTTPRequest(req)
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
