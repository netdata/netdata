// SPDX-License-Identifier: GPL-3.0-or-later

package riakkv

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

func (c *Collector) collect() (map[string]int64, error) {
	stats, err := c.getStats()
	if err != nil {
		return nil, err
	}

	mx := stm.ToMap(stats)

	if len(mx) == 0 {
		return nil, errors.New("no stats")
	}

	c.once.Do(func() { c.adjustCharts(mx) })

	return mx, nil
}

func (c *Collector) getStats() (*riakStats, error) {
	req, err := web.NewHTTPRequest(c.RequestConfig)
	if err != nil {
		return nil, err
	}

	var stats riakStats
	if err := c.client().RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (c *Collector) client() *web.Client {
	return web.DoHTTP(c.httpClient).OnNokCode(func(resp *http.Response) (bool, error) {
		if resp.StatusCode == http.StatusNotFound {
			return false, errors.New("riak_kv_stat is not enabled)")
		}
		return false, nil
	})
}
