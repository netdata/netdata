// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

import (
	"encoding/json"
	"fmt"
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

func newAPIClient(client *http.Client, request web.RequestConfig) *apiClient {
	return &apiClient{httpClient: client, request: request}
}

type apiClient struct {
	httpClient *http.Client
	request    web.RequestConfig
}

func (a apiClient) getPluginsInfo() (*pluginsInfo, error) {
	req, err := a.createRequest(pluginsPath)
	if err != nil {
		return nil, fmt.Errorf("error on creating request : %v", err)
	}

	var info pluginsInfo
	if err := a.doOKDecode(req, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

func (a apiClient) doOKDecode(req *http.Request, in any) error {
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on request: %v", err)
	}

	defer web.CloseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned HTTPConfig status %d", req.URL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(in); err != nil {
		return fmt.Errorf("error on decoding response from %s : %v", req.URL, err)
	}

	return nil
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
