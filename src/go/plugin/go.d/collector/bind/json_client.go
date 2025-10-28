// SPDX-License-Identifier: GPL-3.0-or-later

package bind

import (
	"fmt"
	"net/http"

	"github.com/netdata/netdata/go/plugins/pkg/web"
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

func newJSONClient(client *http.Client, request web.RequestConfig) *jsonClient {
	return &jsonClient{httpClient: client, request: request}
}

type jsonClient struct {
	httpClient *http.Client
	request    web.RequestConfig
}

func (c jsonClient) serverStats() (*serverStats, error) {
	req, err := web.NewHTTPRequestWithPath(c.request, "/server")
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	var stats jsonServerStats

	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}
