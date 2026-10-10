// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"database/sql"
	"strings"
)

func (c *Collector) doQueryTablesMetrics() error {
	if err := c.doQueryStatUserTable(); err != nil {
		return err
	}
	if err := c.doQueryStatIOUserTables(); err != nil {
		return err
	}

	return nil
}

func (c *Collector) doQueryStatUserTable() error {
	return c.queryEachDatabase("table statistics", c.doDBQueryStatUserTables)
}

func (c *Collector) doQueryStatIOUserTables() error {
	return c.queryEachDatabase("table I/O statistics", c.doDBQueryStatIOUserTables)
}

func (c *Collector) doDBQueryStatUserTables(db *sql.DB) error {
	filtered := c.MaxDBTables > 0
	var args []any
	if filtered {
		ids := c.relationsFor(db).tables.oids()
		if len(ids) == 0 {
			return nil
		}
		args = []any{ids}
	}
	q := queryStatUserTables(filtered)

	var dbname, schema, name string
	var oid uint32
	var m *tableMetrics
	var staged []*tableMetrics
	err := c.doDBQuery(db, q, func(column, value string, rowEnd bool) {
		if value == "" && strings.HasPrefix(column, "last_") {
			value = "-1"
		}
		switch column {
		case "relid":
			oid = uint32(parseInt(value))
		case "datname":
			dbname = value
		case "schemaname":
			schema = value
		case "relname":
			name = value
			m = &tableMetrics{owner: db, oid: oid, db: dbname, schema: schema, name: name}
			if old, ok := c.mx.tables[name+"_"+dbname+"_"+schema]; ok && old.owner == db && old.oid == oid {
				*m = *old
			}
			m.updated = true
			if !m.ioSampled {
				m.heapBlksRead.last, m.heapBlksHit.last = -1, -1
				m.idxBlksRead.last, m.idxBlksHit.last = -1, -1
				m.toastBlksRead.last, m.toastBlksHit.last = -1, -1
				m.tidxBlksRead.last, m.tidxBlksHit.last = -1, -1
			}
		case "parent_relname":
			m.parentName = value
		case "seq_scan":
			m.seqScan = parseInt(value)
		case "seq_tup_read":
			m.seqTupRead = parseInt(value)
		case "idx_scan":
			m.idxScan = parseInt(value)
		case "idx_tup_fetch":
			m.idxTupFetch = parseInt(value)
		case "n_tup_ins":
			m.nTupIns = parseInt(value)
		case "n_tup_upd":
			m.nTupUpd.last = parseInt(value)
		case "n_tup_del":
			m.nTupDel = parseInt(value)
		case "n_tup_hot_upd":
			m.nTupHotUpd.last = parseInt(value)
		case "n_live_tup":
			m.nLiveTup = parseInt(value)
		case "n_dead_tup":
			m.nDeadTup = parseInt(value)
		case "last_vacuum":
			m.lastVacuumAgo = parseFloat(value)
		case "last_autovacuum":
			m.lastAutoVacuumAgo = parseFloat(value)
		case "last_analyze":
			m.lastAnalyzeAgo = parseFloat(value)
		case "last_autoanalyze":
			m.lastAutoAnalyzeAgo = parseFloat(value)
		case "vacuum_count":
			m.vacuumCount = parseInt(value)
		case "autovacuum_count":
			m.autovacuumCount = parseInt(value)
		case "analyze_count":
			m.analyzeCount = parseInt(value)
		case "autoanalyze_count":
			m.autoAnalyzeCount = parseInt(value)
		case "total_relation_size":
			m.totalSize = parseInt(value)
		}
		if rowEnd {
			staged = append(staged, m)
		}
	}, args...)
	if err != nil {
		for _, m := range c.mx.tables {
			if m.owner == db {
				m.sampled, m.ioSampled = false, false
			}
		}
		return err
	}
	seen := make(map[string]bool, len(staged))
	for _, m := range staged {
		key := m.name + "_" + m.db + "_" + m.schema
		if old := c.mx.tables[key]; old != nil && (old.owner != db || old.oid != m.oid) {
			c.removeTableCharts(old)
		}
		c.mx.tables[key] = m
		seen[key] = true
	}
	for key, m := range c.mx.tables {
		if m.owner == db && !seen[key] {
			c.removeTableCharts(m)
			delete(c.mx.tables, key)
		}
	}
	return nil
}

func (c *Collector) doDBQueryStatIOUserTables(db *sql.DB) error {
	filtered := c.MaxDBTables > 0
	var args []any
	if filtered {
		ids := c.relationsFor(db).tables.oids()
		if len(ids) == 0 {
			return nil
		}
		args = []any{ids}
	}
	q := queryStatIOUserTables(filtered)

	var dbname, schema, name string
	var m *tableMetrics
	var oid uint32
	staged := make(map[string]*tableMetrics)
	err := c.doDBQuery(db, q, func(column, value string, rowEnd bool) {
		if value == "" && column != "parent_relname" {
			value = "-1"
		}
		switch column {
		case "relid":
			oid = uint32(parseInt(value))
		case "datname":
			dbname = value
		case "schemaname":
			schema = value
		case "relname":
			name = value
			m = &tableMetrics{}
			if old := c.mx.tables[name+"_"+dbname+"_"+schema]; old != nil && old.owner == db && old.updated && old.oid == oid {
				*m = *old
			}
			m.ioUpdated = true
		case "parent_relname":
			m.parentName = value
		case "heap_blks_read_bytes":
			m.heapBlksRead.last = parseInt(value)
		case "heap_blks_hit_bytes":
			m.heapBlksHit.last = parseInt(value)
		case "idx_blks_read_bytes":
			m.idxBlksRead.last = parseInt(value)
		case "idx_blks_hit_bytes":
			m.idxBlksHit.last = parseInt(value)
		case "toast_blks_read_bytes":
			m.toastBlksRead.last = parseInt(value)
		case "toast_blks_hit_bytes":
			m.toastBlksHit.last = parseInt(value)
		case "tidx_blks_read_bytes":
			m.tidxBlksRead.last = parseInt(value)
		case "tidx_blks_hit_bytes":
			m.tidxBlksHit.last = parseInt(value)
		}
		if rowEnd && m.owner == db {
			staged[name+"_"+dbname+"_"+schema] = m
		}
	}, args...)
	if err != nil {
		for _, m := range c.mx.tables {
			if m.owner == db {
				m.ioSampled = false
			}
		}
		return err
	}
	for key, m := range staged {
		c.mx.tables[key] = m
	}
	for _, m := range c.mx.tables {
		if m.owner == db && !m.ioUpdated {
			m.ioSampled = false
		}
	}
	return nil
}
