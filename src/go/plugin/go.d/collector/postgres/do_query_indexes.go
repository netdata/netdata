// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"database/sql"
)

func (c *Collector) doQueryIndexesMetrics() error {
	if err := c.doQueryStatUserIndexes(); err != nil {
		return err
	}

	return nil
}

func (c *Collector) doQueryStatUserIndexes() error {
	return c.queryEachDatabase("index statistics", c.doDBQueryStatUserIndexes)
}

func (c *Collector) doDBQueryStatUserIndexes(db *sql.DB) error {
	filtered := c.MaxDBIndexes > 0
	var args []any
	if filtered {
		ids := c.relationsFor(db).indexes.oids()
		if len(ids) == 0 {
			return nil
		}
		args = []any{ids}
	}
	q := queryStatUserIndexes(filtered)

	var dbname, schema, table, name string
	var oid uint32
	var m *indexMetrics
	var staged []*indexMetrics
	err := c.doDBQuery(db, q, func(column, value string, rowEnd bool) {
		switch column {
		case "indexrelid":
			oid = uint32(parseInt(value))
		case "datname":
			dbname = value
		case "schemaname":
			schema = value
		case "relname":
			table = value
		case "indexrelname":
			name = removeSpaces(value)
			m = &indexMetrics{owner: db, oid: oid, db: dbname, schema: schema, name: name, relname: value, table: table}
			if old, ok := c.mx.indexes[m.key()]; ok && old.owner == db && old.oid == oid {
				*m = *old
				m.relname = value
			}
			m.updated = true
		case "parent_relname":
			m.parentTable = value
		case "idx_scan":
			m.idxScan = parseInt(value)
		case "idx_tup_read":
			m.idxTupRead = parseInt(value)
		case "idx_tup_fetch":
			m.idxTupFetch = parseInt(value)
		case "size":
			m.size = parseInt(value)
		}
		if rowEnd {
			staged = append(staged, m)
		}
	}, args...)
	if err != nil {
		return err
	}
	// Index names that differ only in whitespace share one chart identity; the lowest OID keeps it.
	batch := make(map[indexKey]*indexMetrics, len(staged))
	for _, m := range staged {
		if prev := batch[m.key()]; prev == nil || m.oid < prev.oid {
			batch[m.key()] = m
		}
	}
	for key, m := range batch {
		if old := c.mx.indexes[key]; old != nil && (old.owner != db || old.oid != m.oid) {
			c.removeIndexCharts(old)
		}
		c.mx.indexes[key] = m
	}
	for key, m := range c.mx.indexes {
		if m.owner == db && batch[key] == nil {
			c.removeIndexCharts(m)
			delete(c.mx.indexes, key)
		}
	}
	return nil
}
