// SPDX-License-Identifier: GPL-3.0-or-later

package mongo

import "fmt"

func (m *Mongo) collect() (map[string]int64, error) {
	if err := m.conn.initClient(m.URI, m.Timeout.Duration()); err != nil {
		return nil, fmt.Errorf("init mongo conn: %v", err)
	}

	mx := make(map[string]int64)

	if err := m.collectServerStatus(mx); err != nil {
		return nil, fmt.Errorf("couldn't collect server status metrics: %v", err)
	}

	if err := m.collectDbStats(mx); err != nil {
		return mx, fmt.Errorf("couldn't collect dbstats metrics: %v", err)
	}

	if m.conn.isReplicaSet() {
		if err := m.collectReplSetStatus(mx); err != nil {
			return mx, fmt.Errorf("couldn't collect documentReplSetStatus metrics: %v", err)
		}
	}

	if m.conn.isMongos() {
		m.addShardingChartsOnce.Do(m.addShardingCharts)
		if err := m.collectSharding(mx); err != nil {
			return mx, fmt.Errorf("couldn't collect sharding metrics: %v", err)
		}
	}

	return mx, nil
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
