// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import "database/sql"

func (c *Collector) doQueryBloat() error {
	if err := c.doDBQueryBloat(c.db); err != nil {
		c.Warning(err)
	}
	for _, conn := range c.dbConns {
		if conn.db == nil {
			continue
		}
		if err := c.doDBQueryBloat(conn.db); err != nil {
			c.Warning(err)
		}
	}
	return nil
}

func (c *Collector) doDBQueryBloat(db *sql.DB) error {
	q := queryBloat()

	for _, m := range c.mx.tables {
		if m.bloatSize != nil {
			m.bloatSize = newInt(0)
		}
		if m.bloatSizePerc != nil {
			m.bloatSizePerc = newInt(0)
		}
	}
	for _, m := range c.mx.indexes {
		if m.bloatSize != nil {
			m.bloatSize = newInt(0)
		}
		if m.bloatSizePerc != nil {
			m.bloatSizePerc = newInt(0)
		}
	}

	var dbname, schema, table, iname string
	var tableWasted, idxWasted int64
	return c.doDBQuery(db, q, func(column, value string, rowEnd bool) {
		switch column {
		case "db":
			dbname = value
		case "schemaname":
			schema = value
		case "tablename":
			table = value
		case "wastedbytes":
			tableWasted = parseFloat(value)
		case "iname":
			iname = removeSpaces(value)
		case "wastedibytes":
			idxWasted = parseFloat(value)
		}
		if !rowEnd {
			return
		}
		if c.hasTableMetrics(table, dbname, schema) {
			v := c.getTableMetrics(table, dbname, schema)
			v.bloatSize = newInt(tableWasted)
			v.bloatSizePerc = newInt(calcPercentage(tableWasted, v.totalSize))
		}
		if iname != "?" && c.hasIndexMetrics(iname, table, dbname, schema) {
			v := c.getIndexMetrics(iname, table, dbname, schema)
			v.bloatSize = newInt(idxWasted)
			v.bloatSizePerc = newInt(calcPercentage(idxWasted, v.size))
		}
	})
}
