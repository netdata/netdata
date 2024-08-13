// SPDX-License-Identifier: GPL-3.0-or-later

package riakkv

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	req, err := web.NewHTTPRequest(r.Request)
	if err != nil {
		return nil, err
	}

	var stats riakStats
	if err := r.doOKDecode(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (r *RiakKv) doOKDecode(req *http.Request, in interface{}) error {
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error on HTTP request '%s': %v", req.URL, err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("'%s' returned HTTP status code: %d", req.URL, resp.StatusCode)
		if resp.StatusCode == http.StatusNotFound {
			msg = fmt.Sprintf("%s (riak_kv_stat is not enabled)", msg)
		}
		return errors.New(msg)
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
