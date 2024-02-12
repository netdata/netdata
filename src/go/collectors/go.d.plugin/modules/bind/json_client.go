// SPDX-License-Identifier: GPL-3.0-or-later

package bind

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/netdata/go.d.plugin/pkg/web"
)

type serverStats = jsonServerStats

type jsonServerStats struct {
	OpCodes   map[string]int64
	QTypes    map[string]int64
	NSStats   map[string]int64
	SockStats map[string]int64
	Views     map[string]jsonView
}

type jsonView struct {
	Resolver jsonViewResolver
}

type jsonViewResolver struct {
	Stats      map[string]int64
	QTypes     map[string]int64
	CacheStats map[string]int64
}

func newJSONClient(client *http.Client, request web.Request) *jsonClient {
	return &jsonClient{httpClient: client, request: request}
}

type jsonClient struct {
	httpClient *http.Client
	request    web.Request
}

func (c jsonClient) serverStats() (*serverStats, error) {
	req := c.request.Copy()
	u, err := url.Parse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("error on parsing URL: %v", err)
	}

	u.Path = path.Join(u.Path, "/server")
	req.URL = u.String()

	httpReq, err := web.NewHTTPRequest(req)
	if err != nil {
		return nil, fmt.Errorf("error on creating HTTP request: %v", err)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("error on request : %v", err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP status %d", httpReq.URL, resp.StatusCode)
	}

	stats := &jsonServerStats{}
	if err = json.NewDecoder(resp.Body).Decode(stats); err != nil {
		return nil, fmt.Errorf("error on decoding response from %s : %v", httpReq.URL, err)
	}
	return stats, nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
