// SPDX-License-Identifier: GPL-3.0-or-later

package puppet

import (
	"fmt"
	"net/url"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

var (
	//https://puppet.com/docs/puppet/8/server/status-api/v1/services
	urlPathStatusService  = "/status/v1/services"
	urlQueryStatusService = url.Values{"level": {"debug"}}.Encode()
)

func (c *Collector) collect() (map[string]int64, error) {
	stats, err := c.queryStatsService()
	if err != nil {
		return nil, err
	}

	mx := stm.ToMap(stats)

	return mx, nil
}

func (c *Collector) queryStatsService() (*statusServiceResponse, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathStatusService)
	if err != nil {
		return nil, err
	}

	req.URL.RawQuery = urlQueryStatusService

	var stats statusServiceResponse
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	if stats.StatusService == nil {
		return nil, fmt.Errorf("unexpected response: not puppet service status data")
	}

	return &stats, nil
}
