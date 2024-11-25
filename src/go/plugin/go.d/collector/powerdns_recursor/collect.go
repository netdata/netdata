// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns_recursor

import (
	"errors"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathLocalStatistics = "/api/v1/servers/localhost/statistics"
)

func (r *Recursor) collect() (map[string]int64, error) {
	statistics, err := r.scrapeStatistics()
	if err != nil {
		return nil, err
	}

	collected := make(map[string]int64)

	r.collectStatistics(collected, statistics)

	if !isPowerDNSRecursorMetrics(collected) {
		return nil, errors.New("returned metrics aren't PowerDNS Recursor metrics")
	}

	return collected, nil
}

func isPowerDNSRecursorMetrics(collected map[string]int64) bool {
	// PowerDNS Authoritative Server has same endpoint and returns data in the same format.
	_, ok1 := collected["over-capacity-drops"]
	_, ok2 := collected["tcp-questions"]
	return ok1 && ok2
}

func (r *Recursor) collectStatistics(collected map[string]int64, statistics statisticMetrics) {
	for _, s := range statistics {
		// https://doc.powerdns.com/authoritative/http-api/statistics.html#statisticitem
		if s.Type != "StatisticItem" {
			continue
		}

		value, ok := s.Value.(string)
		if !ok {
			r.Debugf("%s value (%v) unexpected type: want=string, got=%T.", s.Name, s.Value, s.Value)
			continue
		}

		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			r.Debugf("%s value (%v) parse error: %v", s.Name, s.Value, err)
			continue
		}

		collected[s.Name] = v
	}
}

func (r *Recursor) scrapeStatistics() ([]statisticMetric, error) {
	req, _ := web.NewHTTPRequestWithPath(r.RequestConfig, urlPathLocalStatistics)

	var stats statisticMetrics
	if err := web.DoHTTP(r.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	return stats, nil
}
