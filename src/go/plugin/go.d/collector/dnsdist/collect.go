// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

import (
	"net/url"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	urlPathJSONStat = "/jsonstat"
)

func (c *Collector) collect() (map[string]int64, error) {
	statistics, err := c.scrapeStatistics()
	if err != nil {
		return nil, err
	}

	collected := make(map[string]int64)
	c.collectStatistic(collected, statistics)

	return collected, nil
}

func (c *Collector) collectStatistic(collected map[string]int64, statistics *statisticMetrics) {
	for metric, value := range stm.ToMap(statistics) {
		collected[metric] = value
	}
}

func (c *Collector) scrapeStatistics() (*statisticMetrics, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathJSONStat)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = url.Values{"command": []string{"stats"}}.Encode()

	var stats statisticMetrics
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}
