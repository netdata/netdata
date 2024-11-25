// SPDX-License-Identifier: GPL-3.0-or-later

package logstash

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

const urlPathNodeStatsAPI = "/_node/stats"

func (l *Logstash) collect() (map[string]int64, error) {
	stats, err := l.queryNodeStats()
	if err != nil {
		return nil, err
	}

	l.updateCharts(stats.Pipelines)

	return stm.ToMap(stats), nil
}

func (l *Logstash) updateCharts(pipelines map[string]pipelineStats) {
	seen := make(map[string]bool)

	for id := range pipelines {
		seen[id] = true
		if !l.pipelines[id] {
			l.pipelines[id] = true
			l.addPipelineCharts(id)
		}
	}

	for id := range l.pipelines {
		if !seen[id] {
			delete(l.pipelines, id)
			l.removePipelineCharts(id)
		}
	}
}

func (l *Logstash) queryNodeStats() (*nodeStats, error) {
	req, err := web.NewHTTPRequestWithPath(l.RequestConfig, urlPathNodeStatsAPI)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	var stats nodeStats

	if err := web.DoHTTP(l.httpClient).RequestJSON(req, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}
