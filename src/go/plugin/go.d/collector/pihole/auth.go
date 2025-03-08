// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (c *Collector) checkAuthSession() error {
	if c.auth == nil {
		return nil
	}

	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathAPIAuth)
	if err != nil {
		return err
	}
	req.Header.Set("X-FTL-SID", c.auth.Session.Sid)
	req.Header.Set("X-FTL-CSRF", c.auth.Session.Csrf)

	var resp ftlAPIAuthResponse

	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &resp); err != nil {
		if web.IsStatusCode(err, 401) {
			c.auth = nil
			return nil
		}
		return err
	}

	if !resp.Session.Valid {
		c.auth = nil
		return nil
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
