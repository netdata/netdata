// SPDX-License-Identifier: GPL-3.0-or-later

package riakkv

import (
	"errors"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (r *RiakKv) collect() (map[string]int64, error) {
	stats, err := r.getStats()
	if err != nil {
		return nil, err
	}

	mx := stm.ToMap(stats)

	if len(mx) == 0 {
		return nil, errors.New("no stats")
	}

	r.once.Do(func() { r.adjustCharts(mx) })

	return mx, nil
}

func (r *RiakKv) getStats() (*riakStats, error) {
	req, err := web.NewHTTPRequest(r.RequestConfig)
	if err != nil {
		return nil, err
	}

	var stats riakStats
	if err := r.client().RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (r *RiakKv) client() *web.Client {
	return web.DoHTTP(r.httpClient).OnNokCode(func(resp *http.Response) (bool, error) {
		if resp.StatusCode == http.StatusNotFound {
			return false, errors.New("riak_kv_stat is not enabled)")
		}
		return false, nil
	})
}
