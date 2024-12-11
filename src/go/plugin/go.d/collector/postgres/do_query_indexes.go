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
	if err := c.doDBQueryStatUserIndexes(c.db); err != nil {
		c.Warning(err)
	}
	for _, conn := range c.dbConns {
		if conn.db == nil {
			continue
		}
		if err := c.doDBQueryStatUserIndexes(conn.db); err != nil {
			c.Warning(err)
		}
	}
	return nil
}

func (c *Collector) doDBQueryStatUserIndexes(db *sql.DB) error {
	q := queryStatUserIndexes()

	var dbname, schema, table, name string
	return c.doDBQuery(db, q, func(column, value string, _ bool) {
		switch column {
		case "datname":
			dbname = value
		case "schemaname":
			schema = value
		case "relname":
			table = value
		case "indexrelname":
			name = removeSpaces(value)
			c.getIndexMetrics(name, table, dbname, schema).updated = true
		case "parent_relname":
			c.getIndexMetrics(name, table, dbname, schema).parentTable = value
		case "idx_scan":
			c.getIndexMetrics(name, table, dbname, schema).idxScan = parseInt(value)
		case "idx_tup_read":
			c.getIndexMetrics(name, table, dbname, schema).idxTupRead = parseInt(value)
		case "idx_tup_fetch":
			c.getIndexMetrics(name, table, dbname, schema).idxTupFetch = parseInt(value)
		case "size":
			c.getIndexMetrics(name, table, dbname, schema).size = parseInt(value)
		}
	})
}
