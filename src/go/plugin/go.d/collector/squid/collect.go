// SPDX-License-Identifier: GPL-3.0-or-later

package squid

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const (
	// https://wiki.squid-cache.org/Features/CacheManager/Index#controlling-access-to-the-cache-manager
	urlPathServerStats = "/squid-internal-mgr/counters"
)

var statsCounters = map[string]bool{
	"client_http.kbytes_in":      true,
	"client_http.kbytes_out":     true,
	"server.all.errors":          true,
	"server.all.requests":        true,
	"server.all.kbytes_out":      true,
	"server.all.kbytes_in":       true,
	"client_http.errors":         true,
	"client_http.hits":           true,
	"client_http.requests":       true,
	"client_http.hit_kbytes_out": true,
}

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectCounters(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectCounters(mx map[string]int64) error {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathServerStats)
	if err != nil {
		return fmt.Errorf("failed to create '%s' request: %w", urlPathServerStats, err)
	}

	return web.DoHTTP(c.httpClient).Request(req, func(body io.Reader) error {
		sc := bufio.NewScanner(body)

		for sc.Scan() {
			key, value, ok := strings.Cut(sc.Text(), "=")
			if !ok {
				continue
			}

			key, value = strings.TrimSpace(key), strings.TrimSpace(value)

			if !statsCounters[key] {
				continue
			}

			v, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				c.Debugf("failed to parse key %s value %s: %v", key, value, err)
				continue
			}

			mx[key] = v
		}

		if len(mx) == 0 {
			return fmt.Errorf("unexpected response from '%s': no metrics found", req.URL)
		}
		return nil
	})
}
