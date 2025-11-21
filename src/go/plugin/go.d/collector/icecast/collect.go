// SPDX-License-Identifier: GPL-3.0-or-later

package icecast

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const (
	urlPathServerStats = "/status-json.xsl" // https://icecast.org/docs/icecast-trunk/server_stats/
)

func (c *Collector) collect() (map[string]int64, error) {
	mx := make(map[string]int64)

	if err := c.collectServerStats(mx); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectServerStats(mx map[string]int64) error {
	stats, err := c.queryServerStats()
	if err != nil {
		return err
	}
	if stats.IceStats == nil {
		return fmt.Errorf("unexpected response: no icestats found")
	}
	if len(stats.IceStats.Source) == 0 {
		return fmt.Errorf("no icecast sources found")
	}

	seen := make(map[string]bool)

	for _, src := range stats.IceStats.Source {
		name := src.ServerName
		if name == "" {
			continue
		}

		seen[name] = true

		if !c.seenSources[name] {
			c.seenSources[name] = true
			c.addSourceCharts(name)
		}

		px := fmt.Sprintf("source_%s_", name)

		mx[px+"listeners"] = src.Listeners
	}

	for name := range c.seenSources {
		if !seen[name] {
			delete(c.seenSources, name)
			c.removeSourceCharts(name)
		}
	}

	return nil
}

func (c *Collector) queryServerStats() (*serverStats, error) {
	req, err := web.NewHTTPRequestWithPath(c.RequestConfig, urlPathServerStats)
	if err != nil {
		return nil, err
	}

	var stats serverStats
	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}
