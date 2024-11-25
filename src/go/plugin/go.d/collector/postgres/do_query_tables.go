// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"database/sql"
	"strings"
)

func (p *Postgres) doQueryTablesMetrics() error {
	if err := p.doQueryStatUserTable(); err != nil {
		return err
	}
	if err := p.doQueryStatIOUserTables(); err != nil {
		return err
	}

	return nil
}

func (p *Postgres) doQueryStatUserTable() error {
	if err := p.doDBQueryStatUserTables(p.db); err != nil {
		p.Warning(err)
	}
	for _, conn := range p.dbConns {
		if conn.db == nil {
			continue
		}
		if err := p.doDBQueryStatUserTables(conn.db); err != nil {
			p.Warning(err)
		}
	}
	return nil
}

func (p *Postgres) doQueryStatIOUserTables() error {
	if err := p.doDBQueryStatIOUserTables(p.db); err != nil {
		p.Warning(err)
	}
	for _, conn := range p.dbConns {
		if conn.db == nil {
			continue
		}
		if err := p.doDBQueryStatIOUserTables(conn.db); err != nil {
			p.Warning(err)
		}
	}
	return nil
}

func (p *Postgres) doDBQueryStatUserTables(db *sql.DB) error {
	q := queryStatUserTables()

	var dbname, schema, name string
	return p.doDBQuery(db, q, func(column, value string, _ bool) {
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
			p.getTableMetrics(name, dbname, schema).updated = true
		case "parent_relname":
			p.getTableMetrics(name, dbname, schema).parentName = value
		case "seq_scan":
			p.getTableMetrics(name, dbname, schema).seqScan = parseInt(value)
		case "seq_tup_read":
			p.getTableMetrics(name, dbname, schema).seqTupRead = parseInt(value)
		case "idx_scan":
			p.getTableMetrics(name, dbname, schema).idxScan = parseInt(value)
		case "idx_tup_fetch":
			p.getTableMetrics(name, dbname, schema).idxTupFetch = parseInt(value)
		case "n_tup_ins":
			p.getTableMetrics(name, dbname, schema).nTupIns = parseInt(value)
		case "n_tup_upd":
			p.getTableMetrics(name, dbname, schema).nTupUpd.last = parseInt(value)
		case "n_tup_del":
			p.getTableMetrics(name, dbname, schema).nTupDel = parseInt(value)
		case "n_tup_hot_upd":
			p.getTableMetrics(name, dbname, schema).nTupHotUpd.last = parseInt(value)
		case "n_live_tup":
			p.getTableMetrics(name, dbname, schema).nLiveTup = parseInt(value)
		case "n_dead_tup":
			p.getTableMetrics(name, dbname, schema).nDeadTup = parseInt(value)
		case "last_vacuum":
			p.getTableMetrics(name, dbname, schema).lastVacuumAgo = parseFloat(value)
		case "last_autovacuum":
			p.getTableMetrics(name, dbname, schema).lastAutoVacuumAgo = parseFloat(value)
		case "last_analyze":
			p.getTableMetrics(name, dbname, schema).lastAnalyzeAgo = parseFloat(value)
		case "last_autoanalyze":
			p.getTableMetrics(name, dbname, schema).lastAutoAnalyzeAgo = parseFloat(value)
		case "vacuum_count":
			p.getTableMetrics(name, dbname, schema).vacuumCount = parseInt(value)
		case "autovacuum_count":
			p.getTableMetrics(name, dbname, schema).autovacuumCount = parseInt(value)
		case "analyze_count":
			p.getTableMetrics(name, dbname, schema).analyzeCount = parseInt(value)
		case "autoanalyze_count":
			p.getTableMetrics(name, dbname, schema).autoAnalyzeCount = parseInt(value)
		case "total_relation_size":
			p.getTableMetrics(name, dbname, schema).totalSize = parseInt(value)
		}
	})
}

func (p *Postgres) doDBQueryStatIOUserTables(db *sql.DB) error {
	q := queryStatIOUserTables()

	var dbname, schema, name string
	return p.doDBQuery(db, q, func(column, value string, rowEnd bool) {
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
			p.getTableMetrics(name, dbname, schema).updated = true
		case "parent_relname":
			p.getTableMetrics(name, dbname, schema).parentName = value
		case "heap_blks_read_bytes":
			p.getTableMetrics(name, dbname, schema).heapBlksRead.last = parseInt(value)
		case "heap_blks_hit_bytes":
			p.getTableMetrics(name, dbname, schema).heapBlksHit.last = parseInt(value)
		case "idx_blks_read_bytes":
			p.getTableMetrics(name, dbname, schema).idxBlksRead.last = parseInt(value)
		case "idx_blks_hit_bytes":
			p.getTableMetrics(name, dbname, schema).idxBlksHit.last = parseInt(value)
		case "toast_blks_read_bytes":
			p.getTableMetrics(name, dbname, schema).toastBlksRead.last = parseInt(value)
		case "toast_blks_hit_bytes":
			p.getTableMetrics(name, dbname, schema).toastBlksHit.last = parseInt(value)
		case "tidx_blks_read_bytes":
			p.getTableMetrics(name, dbname, schema).tidxBlksRead.last = parseInt(value)
		case "tidx_blks_hit_bytes":
			p.getTableMetrics(name, dbname, schema).tidxBlksHit.last = parseInt(value)
		}
	})
}
