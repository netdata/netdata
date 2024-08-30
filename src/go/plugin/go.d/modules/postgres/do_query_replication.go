// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"fmt"
)

func (p *Postgres) doQueryReplicationMetrics() error {
	if err := p.doQueryReplStandbyAppWALDelta(); err != nil {
		return fmt.Errorf("querying replication standby app wal delta error: %v", err)
	}

	if p.pgVersion >= pgVersion10 {
		if err := p.doQueryReplStandbyAppWALLag(); err != nil {
			return fmt.Errorf("querying replication standby app wal lag error: %v", err)
		}
	}

	if p.pgVersion >= pgVersion10 && p.isSuperUser() {
		if err := p.doQueryReplSlotFiles(); err != nil {
			return fmt.Errorf("querying replication slot files error: %v", err)
		}
	}

	return nil
}

func (p *Postgres) doQueryReplStandbyAppWALDelta() error {
	q := queryReplicationStandbyAppDelta(p.pgVersion)

	var app string
	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "application_name":
			app = value
			p.getReplAppMetrics(app).updated = true
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
				p.getReplAppMetrics(app).walSentDelta += v
			case "write_delta":
				p.getReplAppMetrics(app).walWriteDelta += v
			case "flush_delta":
				p.getReplAppMetrics(app).walFlushDelta += v
			case "replay_delta":
				p.getReplAppMetrics(app).walReplayDelta += v
			}
		}
	})
}

func (p *Postgres) doQueryReplStandbyAppWALLag() error {
	q := queryReplicationStandbyAppLag()

	var app string
	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "application_name":
			app = value
			p.getReplAppMetrics(app).updated = true
		case "write_lag":
			p.getReplAppMetrics(app).walWriteLag += parseInt(value)
		case "flush_lag":
			p.getReplAppMetrics(app).walFlushLag += parseInt(value)
		case "replay_lag":
			p.getReplAppMetrics(app).walReplayLag += parseInt(value)
		}
	})
}

func (p *Postgres) doQueryReplSlotFiles() error {
	q := queryReplicationSlotFiles(p.pgVersion)

	var slot string
	return p.doQuery(q, func(column, value string, _ bool) {
		switch column {
		case "slot_name":
			slot = value
			p.getReplSlotMetrics(slot).updated = true
		case "replslot_wal_keep":
			p.getReplSlotMetrics(slot).walKeep += parseInt(value)
		case "replslot_files":
			p.getReplSlotMetrics(slot).files += parseInt(value)
		}
	})
}
