// SPDX-License-Identifier: GPL-3.0-or-later

package logstash

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/stm"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

const urlPathNodeStatsAPI = "/_node/stats"

func (c *Collector) collect(ctx context.Context) (map[string]int64, error) {
	stats, err := c.queryNodeStats(ctx)
	if err != nil {
		return nil, err
	}

	c.updateCharts(stats.Pipelines)

	return stm.ToMap(stats), nil
}

func (c *Collector) updateCharts(pipelines map[string]pipelineStats) {
	seen := make(map[string]bool)

	for id := range pipelines {
		seen[id] = true
		if !c.pipelines[id] {
			c.pipelines[id] = true
			c.addPipelineCharts(id)
		}
	}

	for id := range c.pipelines {
		if !seen[id] {
			delete(c.pipelines, id)
			c.removePipelineCharts(id)
		}
	}
}

func (c *Collector) queryNodeStats(ctx context.Context) (*nodeStats, error) {
	req, err := web.NewHTTPRequestWithPath(ctx, c.RequestConfig, urlPathNodeStatsAPI)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	var stats nodeStats

	if err := web.DoHTTP(c.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}
