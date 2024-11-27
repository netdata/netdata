// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import "database/sql"

func (c *Collector) doQueryColumns() error {
	if err := c.doDBQueryColumns(c.db); err != nil {
		c.Warning(err)
	}
	for _, conn := range c.dbConns {
		if conn.db == nil {
			continue
		}
		if err := c.doDBQueryColumns(conn.db); err != nil {
			c.Warning(err)
		}
	}
	return nil
}

func (c *Collector) doDBQueryColumns(db *sql.DB) error {
	q := queryColumnsStats()

	for _, m := range c.mx.tables {
		if m.nullColumns != nil {
			m.nullColumns = newInt(0)
		}
	}

	var dbname, schema, table string
	var nullPerc int64
	return c.doDBQuery(db, q, func(column, value string, rowEnd bool) {
		switch column {
		case "datname":
			dbname = value
		case "schemaname":
			schema = value
		case "relname":
			table = value
		case "null_percent":
			nullPerc = parseInt(value)
		}
		if !rowEnd {
			return
		}
		if nullPerc == 100 && c.hasTableMetrics(table, dbname, schema) {
			v := c.getTableMetrics(table, dbname, schema)
			if v.nullColumns == nil {
				v.nullColumns = newInt(0)
			}
			*v.nullColumns++
		}
	})
}
