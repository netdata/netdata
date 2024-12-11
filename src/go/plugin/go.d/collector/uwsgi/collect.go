// SPDX-License-Identifier: GPL-3.0-or-later

package uwsgi

import (
	"encoding/json"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

type statsResponse struct {
	Workers []workerStats `json:"workers"`
}

type workerStats struct {
	ID            int    `json:"id"`
	Accepting     int64  `json:"accepting"`
	Requests      int64  `json:"requests"`
	DeltaRequests int64  `json:"delta_requests"`
	Exceptions    int64  `json:"exceptions"`
	HarakiriCount int64  `json:"harakiri_count"`
	Status        string `json:"status"`
	RSS           int64  `json:"rss"`
	VSZ           int64  `json:"vsz"`
	RespawnCount  int64  `json:"respawn_count"`
	TX            int64  `json:"tx"`
	AvgRT         int64  `json:"avg_rt"`
}

func (c *Collector) collect() (map[string]int64, error) {
	stats, err := c.conn.queryStats()
	if err != nil {
		return nil, fmt.Errorf("failed to query stats: %v", err)
	}

	mx := make(map[string]int64)

	if err := c.collectStats(mx, stats); err != nil {
		return nil, err
	}

	return mx, nil
}

func (c *Collector) collectStats(mx map[string]int64, stats []byte) error {
	var resp statsResponse
	if err := json.Unmarshal(stats, &resp); err != nil {
		return fmt.Errorf("failed to json decode stats response: %v", err)
	}

	// stats server returns an empty array if there are no workers
	if resp.Workers == nil {
		return fmt.Errorf("unexpected stats response: no workers found")
	}

	seen := make(map[int]bool)

	mx["workers_tx"] = 0
	mx["workers_requests"] = 0
	mx["workers_harakiris"] = 0
	mx["workers_exceptions"] = 0
	mx["workers_respawns"] = 0

	for _, w := range resp.Workers {
		mx["workers_tx"] += w.TX
		mx["workers_requests"] += w.Requests
		mx["workers_harakiris"] += w.HarakiriCount
		mx["workers_exceptions"] += w.Exceptions
		mx["workers_respawns"] += w.RespawnCount

		seen[w.ID] = true

		if !c.seenWorkers[w.ID] {
			c.seenWorkers[w.ID] = true
			c.addWorkerCharts(w.ID)
		}

		px := fmt.Sprintf("worker_%d_", w.ID)

		mx[px+"tx"] = w.TX
		mx[px+"requests"] = w.Requests
		mx[px+"delta_requests"] = w.DeltaRequests
		mx[px+"average_request_time"] = w.AvgRT
		mx[px+"harakiris"] = w.HarakiriCount
		mx[px+"exceptions"] = w.Exceptions
		mx[px+"respawns"] = w.RespawnCount
		mx[px+"memory_rss"] = w.RSS
		mx[px+"memory_vsz"] = w.VSZ

		for _, v := range []string{"idle", "busy", "cheap", "pause", "sig"} {
			mx[px+"status_"+v] = metrix.Bool(w.Status == v)
		}
		mx[px+"request_handling_status_accepting"] = metrix.Bool(w.Accepting == 1)
		mx[px+"request_handling_status_not_accepting"] = metrix.Bool(w.Accepting == 0)
	}

	for id := range c.seenWorkers {
		if !seen[id] {
			delete(c.seenWorkers, id)
			c.removeWorkerCharts(id)
		}
	}

	return nil
}
