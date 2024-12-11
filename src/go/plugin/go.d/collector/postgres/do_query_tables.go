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
	if err := c.doDBQueryStatUserTables(c.db); err != nil {
		c.Warning(err)
	}
	for _, conn := range c.dbConns {
		if conn.db == nil {
			continue
		}
		if err := c.doDBQueryStatUserTables(conn.db); err != nil {
			c.Warning(err)
		}
	}
	return nil
}

func (c *Collector) doQueryStatIOUserTables() error {
	if err := c.doDBQueryStatIOUserTables(c.db); err != nil {
		c.Warning(err)
	}
	for _, conn := range c.dbConns {
		if conn.db == nil {
			continue
		}
		if err := c.doDBQueryStatIOUserTables(conn.db); err != nil {
			c.Warning(err)
		}
	}
	return nil
}

func (c *Collector) doDBQueryStatUserTables(db *sql.DB) error {
	q := queryStatUserTables()

	var dbname, schema, name string
	return c.doDBQuery(db, q, func(column, value string, _ bool) {
		if value == "" && strings.HasPrefix(column, "last_") {
			value = "-1"
		}
		switch column {
		case "datname":
			dbname = value
		case "schemaname":
			schema = value
		case "relname":
			name = value
			c.getTableMetrics(name, dbname, schema).updated = true
		case "parent_relname":
			c.getTableMetrics(name, dbname, schema).parentName = value
		case "seq_scan":
			c.getTableMetrics(name, dbname, schema).seqScan = parseInt(value)
		case "seq_tup_read":
			c.getTableMetrics(name, dbname, schema).seqTupRead = parseInt(value)
		case "idx_scan":
			c.getTableMetrics(name, dbname, schema).idxScan = parseInt(value)
		case "idx_tup_fetch":
			c.getTableMetrics(name, dbname, schema).idxTupFetch = parseInt(value)
		case "n_tup_ins":
			c.getTableMetrics(name, dbname, schema).nTupIns = parseInt(value)
		case "n_tup_upd":
			c.getTableMetrics(name, dbname, schema).nTupUpd.last = parseInt(value)
		case "n_tup_del":
			c.getTableMetrics(name, dbname, schema).nTupDel = parseInt(value)
		case "n_tup_hot_upd":
			c.getTableMetrics(name, dbname, schema).nTupHotUpd.last = parseInt(value)
		case "n_live_tup":
			c.getTableMetrics(name, dbname, schema).nLiveTup = parseInt(value)
		case "n_dead_tup":
			c.getTableMetrics(name, dbname, schema).nDeadTup = parseInt(value)
		case "last_vacuum":
			c.getTableMetrics(name, dbname, schema).lastVacuumAgo = parseFloat(value)
		case "last_autovacuum":
			c.getTableMetrics(name, dbname, schema).lastAutoVacuumAgo = parseFloat(value)
		case "last_analyze":
			c.getTableMetrics(name, dbname, schema).lastAnalyzeAgo = parseFloat(value)
		case "last_autoanalyze":
			c.getTableMetrics(name, dbname, schema).lastAutoAnalyzeAgo = parseFloat(value)
		case "vacuum_count":
			c.getTableMetrics(name, dbname, schema).vacuumCount = parseInt(value)
		case "autovacuum_count":
			c.getTableMetrics(name, dbname, schema).autovacuumCount = parseInt(value)
		case "analyze_count":
			c.getTableMetrics(name, dbname, schema).analyzeCount = parseInt(value)
		case "autoanalyze_count":
			c.getTableMetrics(name, dbname, schema).autoAnalyzeCount = parseInt(value)
		case "total_relation_size":
			c.getTableMetrics(name, dbname, schema).totalSize = parseInt(value)
		}
	})
}

func (c *Collector) doDBQueryStatIOUserTables(db *sql.DB) error {
	q := queryStatIOUserTables()

	var dbname, schema, name string
	return c.doDBQuery(db, q, func(column, value string, rowEnd bool) {
		if value == "" && column != "parent_relname" {
			value = "-1"
		}
		switch column {
		case "datname":
			dbname = value
		case "schemaname":
			schema = value
		case "relname":
			name = value
			c.getTableMetrics(name, dbname, schema).updated = true
		case "parent_relname":
			c.getTableMetrics(name, dbname, schema).parentName = value
		case "heap_blks_read_bytes":
			c.getTableMetrics(name, dbname, schema).heapBlksRead.last = parseInt(value)
		case "heap_blks_hit_bytes":
			c.getTableMetrics(name, dbname, schema).heapBlksHit.last = parseInt(value)
		case "idx_blks_read_bytes":
			c.getTableMetrics(name, dbname, schema).idxBlksRead.last = parseInt(value)
		case "idx_blks_hit_bytes":
			c.getTableMetrics(name, dbname, schema).idxBlksHit.last = parseInt(value)
		case "toast_blks_read_bytes":
			c.getTableMetrics(name, dbname, schema).toastBlksRead.last = parseInt(value)
		case "toast_blks_hit_bytes":
			c.getTableMetrics(name, dbname, schema).toastBlksHit.last = parseInt(value)
		case "tidx_blks_read_bytes":
			c.getTableMetrics(name, dbname, schema).tidxBlksRead.last = parseInt(value)
		case "tidx_blks_hit_bytes":
			c.getTableMetrics(name, dbname, schema).tidxBlksHit.last = parseInt(value)
		}
	})
}
