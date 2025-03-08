// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	precision = 1000
)

func (c *Collector) collect() (map[string]int64, error) {
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

func (c *Collector) getAuthSession() (*ftlAPIAuthResponse, error) {
	var pass struct {
		Password string `json:"password"`
	}
	pass.Password = c.Password
	bs, _ := json.Marshal(pass)

	cfg := c.RequestConfig.Copy()
	cfg.Method = http.MethodPost
	cfg.Body = string(bs)

	req, err := web.NewHTTPRequestWithPath(cfg, urlPathAPIAuth)
	if err != nil {
		return nil, err
	}

	var resp ftlAPIAuthResponse

	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		return nil, err
	}

	if !resp.Session.Valid {
		return nil, fmt.Errorf("invalid auth session (%s)", resp.Session.Message)
	}

	return &resp, nil
}

func calcPercentage(value, total int64) (v int64) {
	if total == 0 {
		return 0
	}
	return int64(float64(value) * 100 / float64(total) * precision)
}
