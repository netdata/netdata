// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	precision = 1000
)

func (c *Collector) collect() (map[string]int64, error) {
	if err := c.checkAuthSession(); err != nil {
		return nil, err
	}
	if c.auth == nil {
		auth, err := c.getAuthSession()
		if err != nil {
			return nil, err
		}
		c.auth = auth
	}

	mx := make(map[string]int64)

	if err := c.collectMetrics(mx); err != nil {
		if web.IsStatusCode(err, 401) {
			c.auth = nil
		}
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectMetrics(mx map[string]int64) error {
	if c.auth == nil {
		return errors.New("no auth session")
	}

	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathAPIStatsSummary)
	if err != nil {
		return err
	}
	req.Header.Set("X-FTL-SID", c.auth.Session.Sid)
	req.Header.Set("X-FTL-CSRF", c.auth.Session.Csrf)

	var resp ftlAPIStatsSummaryResponse

	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return err
	}

	if resp.Took == nil {
		return fmt.Errorf("unexpected response from %s", req.URL)
	}

	for k, v := range stm.ToMap(resp) {
		mx[k] = v
	}

	// 0 if unknown
	if resp.Gravity.LastUpdate != 0 {
		mx["gravity_last_update_seconds_ago"] = int64(time.Since(time.Unix(resp.Gravity.LastUpdate, 0)).Seconds())
	}

	return nil
}
