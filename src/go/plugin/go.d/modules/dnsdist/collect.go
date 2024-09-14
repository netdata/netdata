// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

import (
	"net/url"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathJSONStat = "/jsonstat"
)

func (d *DNSdist) collect() (map[string]int64, error) {
	statistics, err := d.scrapeStatistics()
	if err != nil {
		return nil, err
	}

	collected := make(map[string]int64)
	d.collectStatistic(collected, statistics)

	return collected, nil
}

func (d *DNSdist) collectStatistic(collected map[string]int64, statistics *statisticMetrics) {
	for metric, value := range stm.ToMap(statistics) {
		collected[metric] = value
	}
}

func (d *DNSdist) scrapeStatistics() (*statisticMetrics, error) {
	req, err := web.NewHTTPRequestWithPath(d.RequestConfig, urlPathJSONStat)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = url.Values{"command": []string{"stats"}}.Encode()

	var stats statisticMetrics
	if err := web.DoHTTP(d.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}
