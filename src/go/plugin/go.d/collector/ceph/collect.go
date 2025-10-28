// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const precision = 1000

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.auth(); err != nil {
		return nil, err
	}

	if c.fsid == "" {
		fsid, err := c.getFsid()
		if err != nil {
			return nil, fmt.Errorf("failed to get fsid: %v", err)
		}
		c.fsid = fsid
		c.addClusterChartsOnce.Do(c.addClusterCharts)
	}

	if err := c.collectHealth(mx); err != nil {
		return nil, fmt.Errorf("failed to collect health: %v", err)
	}
	if err := c.collectOsds(mx); err != nil {
		return nil, fmt.Errorf("failed to collect osds: %v", err)
	}
	if err := c.collectPools(mx); err != nil {
		return nil, fmt.Errorf("failed to collect pools: %v", err)
	}

	return mx, nil
}

func (c *Collector) auth() error {
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

func (c *Collector) getFsid() (string, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathApiMonitor)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", hdrAcceptVersion)
	req.Header.Set("Content-Type", hdrContentTypeJson)
	req.Header.Set("Authorization", "Bearer "+c.token)

	var resp struct {
		MonStatus struct {
			MonMap struct {
				FSID string `json:"fsid"`
			} `json:"monmap"`
		} `json:"mon_status"`
	}

	if err := c.webClient().RequestJSON(req, &resp); err != nil {
		return "", err
	}

	if resp.MonStatus.MonMap.FSID == "" {
		return "", errors.New("no fsid")
	}

	return resp.MonStatus.MonMap.FSID, nil
}

func (c *Collector) webClient(statusCodes ...int) *web.Client {
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
