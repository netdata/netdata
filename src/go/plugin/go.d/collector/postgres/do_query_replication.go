// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"fmt"
)

func (c *Collector) doQueryReplicationMetrics() error {
	if err := c.doQueryReplStandbyAppWALDelta(); err != nil {
		return fmt.Errorf("querying replication standby app wal delta error: %v", err)
	}

	if c.pgVersion >= pgVersion10 {
		if err := c.doQueryReplStandbyAppWALLag(); err != nil {
			return fmt.Errorf("querying replication standby app wal lag error: %v", err)
		}
	}

	if c.pgVersion >= pgVersion10 && c.isSuperUser() {
		if err := c.doQueryReplSlotFiles(); err != nil {
			return fmt.Errorf("querying replication slot files error: %v", err)
		}
	}

	return nil
}

func (c *Collector) doQueryReplStandbyAppWALDelta() error {
	q := queryReplicationStandbyAppDelta(c.pgVersion)

	var app string
	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "application_name":
			app = value
			c.getReplAppMetrics(app).updated = true
		default:
			// TODO: delta calculation was changed in https://github.com/netdata/netdata/go/plugins/plugin/go.d/pull/1039
			// - 'replay_delta' (probably other deltas too?) can be negative
			// - Also, WAL delta != WAL lag after that PR
			v := parseInt(value)
			if v < 0 {
				v = 0
			}
			switch column {
			case "sent_delta":
				c.getReplAppMetrics(app).walSentDelta += v
			case "write_delta":
				c.getReplAppMetrics(app).walWriteDelta += v
			case "flush_delta":
				c.getReplAppMetrics(app).walFlushDelta += v
			case "replay_delta":
				c.getReplAppMetrics(app).walReplayDelta += v
			}
		}
	})
}

func (c *Collector) doQueryReplStandbyAppWALLag() error {
	q := queryReplicationStandbyAppLag()

	var app string
	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "application_name":
			app = value
			c.getReplAppMetrics(app).updated = true
		case "write_lag":
			c.getReplAppMetrics(app).walWriteLag += parseInt(value)
		case "flush_lag":
			c.getReplAppMetrics(app).walFlushLag += parseInt(value)
		case "replay_lag":
			c.getReplAppMetrics(app).walReplayLag += parseInt(value)
		}
	})
}

func (c *Collector) doQueryReplSlotFiles() error {
	q := queryReplicationSlotFiles(c.pgVersion)

	var slot string
	return c.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "slot_name":
			slot = value
			c.getReplSlotMetrics(slot).updated = true
		case "replslot_wal_keep":
			c.getReplSlotMetrics(slot).walKeep += parseInt(value)
		case "replslot_files":
			c.getReplSlotMetrics(slot).files += parseInt(value)
		}
	})
}
