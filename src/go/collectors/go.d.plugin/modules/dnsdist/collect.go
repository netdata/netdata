// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/netdata/go.d.plugin/pkg/stm"
	"github.com/netdata/go.d.plugin/pkg/web"
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
	req, _ := web.NewHTTPRequest(d.Request)
	req.URL.Path = urlPathJSONStat
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
		return fmt.Errorf("error on HTTP request '%s': %v", req.URL, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(in); err != nil {
		return fmt.Errorf("error on decoding response from '%s': %v", req.URL, err)
	}

	return nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
