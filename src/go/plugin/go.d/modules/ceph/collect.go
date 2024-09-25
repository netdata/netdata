// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const precision = 1000

func (c *Ceph) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.auth(); err != nil {
		return nil, err
	}

	if err := c.collectOsds(mx); err != nil {
		return nil, err
	}
	if err := c.collectPools(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Ceph) auth() error {
	if c.token != "" {
		ok, err := c.authCheck()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		c.token = ""
	}

	tok, err := c.authLogin()
	if err != nil {
		return err
	}
	c.token = tok

	return nil
}

func (c *Ceph) webClient(statusCodes ...int) *web.Client {
	return web.DoHTTP(c.httpClient).OnNokCode(func(resp *http.Response) (bool, error) {
		if slices.Contains(statusCodes, resp.StatusCode) {
			return true, nil
		}
		var msg struct {
			Detail string `json:"detail"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&msg); err == nil && msg.Detail != "" {
			return false, errors.New(msg.Detail)
		}
		return false, nil
	})
}
