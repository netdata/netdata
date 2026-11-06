// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (c *Collector) doQueryTablesMetrics() error {
	return c.queryEachDatabase("table metrics", c.doDBQueryTablesMetrics)
}

func (c *Collector) doDBQueryTablesMetrics(db *sql.DB) error {
	statsErr := c.doDBQueryStatUserTables(db)
	ioErr := c.doDBQueryStatIOUserTables(db, statsErr == nil)
	if statsErr != nil {
		statsErr = fmt.Errorf("statistics: %w", statsErr)
	}
	if ioErr != nil {
		ioErr = fmt.Errorf("I/O statistics: %w", ioErr)
	}
	return errors.Join(statsErr, ioErr)
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
			m = newTableMetrics(db, oid, name, dbname, schema)
			if old, ok := c.mx.tables[name+"_"+dbname+"_"+schema]; ok && old.owner == db && old.oid == oid {
				*m = *old
			}
			m.updated = true
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
				m.sampled = false
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

func (c *Collector) doDBQueryStatIOUserTables(db *sql.DB, statsComplete bool) error {
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
	staged := make(map[uint32]*tableMetrics)
	var accepted bool
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
			m = newTableMetrics(db, oid, name, dbname, schema)
			old := c.mx.tables[name+"_"+dbname+"_"+schema]
			matches := old != nil && old.owner == db && old.oid == oid
			// A successful stats observation owns this cycle's identity. Otherwise
			// this complete I/O observation can establish an identity independently.
			accepted = !statsComplete || (matches && old.updated)
			if matches {
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
		if rowEnd && accepted {
			staged[oid] = m
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
	for _, m := range staged {
		key := m.name + "_" + m.db + "_" + m.schema
		if old := c.mx.tables[key]; old != nil && (old.owner != db || old.oid != m.oid) {
			c.removeTableCharts(old)
		}
		c.mx.tables[key] = m
	}
	for key, m := range c.mx.tables {
		if m.owner != db {
			continue
		}
		if current := staged[m.oid]; current != nil && (current.name != m.name || current.schema != m.schema) {
			c.removeTableCharts(m)
			delete(c.mx.tables, key)
			continue
		}
		if !m.ioUpdated {
			m.ioSampled = false
		}
	}
	return nil
}
