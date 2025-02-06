// SPDX-License-Identifier: GPL-3.0-or-later

package zookeeper

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

func (c *Collector) collect() (map[string]int64, error) {
	return c.collectMntr()
}

func (c *Collector) collectMntr() (map[string]int64, error) {
	const command = "mntr"

	lines, err := c.fetch("mntr")
	if err != nil {
		return nil, err
	}

	switch len(lines) {
	case 0:
		return nil, fmt.Errorf("'%s' command returned empty response", command)
	case 1:
		// mntr is not executed because it is not in the whitelist.
		return nil, fmt.Errorf("'%s' command returned bad response: %s", command, lines[0])
	}

	mx := make(map[string]int64)

	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "zk_") {
			continue
		}

		key, value := parts[0], parts[1]
		if !zkMetrics[key] {
			continue
		}
		key = strings.TrimPrefix(key, "zk_")

		switch key {
		case "server_state":
			for _, v := range zkServerStates {
				mx[key+"_"+v] = metrix.Bool(v == value)
			}
		case "min_latency", "avg_latency", "max_latency":
			writeMetric(mx, key, value, 1000, 1)
		case "uptime":
			writeMetric(mx, key, value, 1, 1000) // ms->seconds
		default:
			writeMetric(mx, key, value, 1, 1)
		}
	}

	if len(mx) == 0 {
		return nil, fmt.Errorf("'%s' command: failed to parse response", command)
	}

	return mx, nil
}

func writeMetric(mx map[string]int64, metric, value string, mul, div float64) {
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return
	}
	if mul != 0 {
		v *= mul
	}
	if div != 0 {
		v /= div
	}
	mx[metric] = int64(v)

}

var zkServerStates = []string{
	"leader",
	"follower",
	"observer",
	"standalone",
}

var zkMetrics = map[string]bool{
	"zk_server_state": true,

	"zk_outstanding_requests":   true,
	"zk_min_latency":            true,
	"zk_avg_latency":            true,
	"zk_max_latency":            true,
	"zk_stale_requests":         true,
	"zk_stale_requests_dropped": true,

	"zk_num_alive_connections": true,
	"zk_auth_failed_count":     true,
	"zk_connection_drop_count": true,
	"zk_connection_rejected":   true,
	"zk_global_sessions":       true,

	"zk_packets_received": true,
	"zk_packets_sent":     true,

	"zk_open_file_descriptor_count": true,
	"zk_max_file_descriptor_count":  true,
	"zk_znode_count":                true,
	"zk_ephemerals_count":           true,
	"zk_watch_count":                true,
	"zk_approximate_data_size":      true,

	"zk_uptime":        true,
	"zk_throttled_ops": true,
}
