// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

import (
	"context"
	"maps"
	"net/url"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	urlPathJSONStat = "/jsonstat"
)

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	statistics, err := c.scrapeStatistics(ctx)
	if err != nil {
		return nil, err
	}

	collected := make(map[string]int64)
	c.collectStatistic(collected, statistics)

	return collected, nil
}

func (c *Collector) collectStatistic(collected map[string]int64, statistics *statisticMetrics) {
	maps.Copy(collected, stm.ToMap(statistics))
}

func (c *Collector) scrapeStatistics(ctx context.Context) (*statisticMetrics, error) {
	req, err := web.NewHTTPRequestWithPath(ctx, c.RequestConfig, urlPathJSONStat)
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
