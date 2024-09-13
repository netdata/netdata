// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

import (
	"encoding/json"
	"fmt"
	"net/http"
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

	var statistics statisticMetrics
	if err := d.doOKDecode(req, &statistics); err != nil {
		return nil, err
	}

	return &statistics, nil
}

func (d *DNSdist) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTPConfig request '%s': %v", req.URL, err)
	}

	defer web.CloseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTPConfig status code: %d", req.URL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(in); err != nil {
		return fmt.Errorf("error on decoding response from '%s': %v", req.URL, err)
	}

	return nil
}
