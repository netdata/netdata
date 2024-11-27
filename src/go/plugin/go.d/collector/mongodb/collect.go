// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import "fmt"

func (c *Collector) collect() (map[string]int64, error) {
	if err := c.conn.initClient(c.URI, c.Timeout.Duration()); err != nil {
		return nil, fmt.Errorf("init mongo conn: %v", err)
	}

	mx := make(map[string]int64)

	if err := c.collectServerStatus(mx); err != nil {
		return nil, fmt.Errorf("couldn't collect server status metrics: %v", err)
	}

	if err := c.collectDbStats(mx); err != nil {
		return mx, fmt.Errorf("couldn't collect dbstats metrics: %v", err)
	}

	if c.conn.isReplicaSet() {
		if err := c.collectReplSetStatus(mx); err != nil {
			return mx, fmt.Errorf("couldn't collect documentReplSetStatus metrics: %v", err)
		}
	}

	if c.conn.isMongos() {
		c.addShardingChartsOnce.Do(c.addShardingCharts)
		if err := c.collectSharding(mx); err != nil {
			return mx, fmt.Errorf("couldn't collect sharding metrics: %v", err)
		}
	}

	return mx, nil
}
