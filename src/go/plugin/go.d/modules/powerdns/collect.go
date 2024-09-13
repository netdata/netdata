// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	urlPathLocalStatistics = "/api/v1/servers/localhost/statistics"
)

func (ns *AuthoritativeNS) collect() (map[string]int64, error) {
	statistics, err := ns.scrapeStatistics()
	if err != nil {
		return nil, err
	}

	collected := make(map[string]int64)

	ns.collectStatistics(collected, statistics)

	if !isPowerDNSAuthoritativeNSMetrics(collected) {
		return nil, errors.New("returned metrics aren't PowerDNS Authoritative Server metrics")
	}

	return collected, nil
}

func isPowerDNSAuthoritativeNSMetrics(collected map[string]int64) bool {
	// PowerDNS Recursor has same endpoint and returns data in the same format.
	_, ok1 := collected["over-capacity-drops"]
	_, ok2 := collected["tcp-questions"]
	return !ok1 && !ok2
}

func (ns *AuthoritativeNS) collectStatistics(collected map[string]int64, statistics statisticMetrics) {
	for _, s := range statistics {
		// https://doc.powerdns.com/authoritative/http-api/statistics.html#statisticitem
		if s.Type != "StatisticItem" {
			continue
		}

		value, ok := s.Value.(string)
		if !ok {
			ns.Debugf("%s value (%v) unexpected type: want=string, got=%T.", s.Name, s.Value, s.Value)
			continue
		}

		v, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			ns.Debugf("%s value (%v) parse error: %v", s.Name, s.Value, err)
			continue
		}

		collected[s.Name] = v
	}
}

func (ns *AuthoritativeNS) scrapeStatistics() ([]statisticMetric, error) {
	req, _ := web.NewHTTPRequestWithPath(ns.RequestConfig, urlPathLocalStatistics)

	var statistics statisticMetrics
	if err := ns.doOKDecode(req, &statistics); err != nil {
		return nil, err
	}

	return statistics, nil
}

func (ns *AuthoritativeNS) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := ns.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTPConfig request '%s': %v", req.URL, err)
	}

	defer web.CloseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(in); err != nil {
		return fmt.Errorf("error on decoding response from '%s': %v", req.URL, err)
	}
	return nil
}
