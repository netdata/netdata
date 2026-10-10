// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import "database/sql"

func (c *Collector) doQueryColumns() error {
	return c.queryEachDatabase("column statistics", c.doDBQueryColumns)
}

func (c *Collector) doDBQueryColumns(db *sql.DB) error {
	filtered := c.MaxDBTables > 0
	var args []any
	if filtered {
		ids := c.relationsFor(db).tables.oids()
		if len(ids) == 0 {
			return nil
		}
		args = []any{ids}
	}
	q := queryColumnsStats(filtered)
	counts := make(map[string]int64)

	var dbname, schema, table string
	var nullPerc int64
	err := c.doDBQuery(db, q, func(column, value string, rowEnd bool) {
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
		if nullPerc == 100 {
			counts[table+"_"+dbname+"_"+schema]++
		}
	}, args...)
	if err != nil {
		for _, m := range c.mx.tables {
			if m.owner == db {
				m.nullValid = false
			}
		}
		return err
	}
	for key, m := range c.mx.tables {
		if m.owner != db || !m.updated {
			continue
		}
		m.nullValid = true
		count := counts[key]
		if count > 0 || m.nullColumns != nil {
			m.nullColumns = new(count)
		}
	}
	return nil
}
