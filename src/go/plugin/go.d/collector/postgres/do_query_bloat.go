// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import "database/sql"

func (c *Collector) doQueryBloat() error {
	return c.queryEachDatabase("bloat estimates", c.doDBQueryBloat)
}

func (c *Collector) doDBQueryBloat(db *sql.DB) error {
	filtered := c.MaxDBTables > 0 || c.MaxDBIndexes > 0
	var args []any
	if filtered {
		r := c.relationsFor(db)
		var tables, indexes []uint32
		if c.MaxDBTables > 0 {
			tables = r.tables.oids()
		}
		if c.MaxDBIndexes > 0 {
			indexes = r.indexes.oids()
		}
		if c.MaxDBTables > 0 && c.MaxDBIndexes > 0 && len(tables) == 0 && len(indexes) == 0 {
			return nil
		}
		args = []any{tables, indexes}
	}
	q := queryBloat(filtered)
	type result struct {
		database, schema, table, index string
		tableWasted, indexWasted       int64
	}
	var results []result

	var dbname, schema, table, iname string
	var tableWasted, idxWasted int64
	err := c.doDBQuery(db, q, func(column, value string, rowEnd bool) {
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
		results = append(results, result{dbname, schema, table, iname, tableWasted, idxWasted})
	}, args...)
	if err != nil {
		for _, m := range c.mx.tables {
			if m.owner == db {
				m.bloatValid = false
			}
		}
		for _, m := range c.mx.indexes {
			if m.owner == db {
				m.bloatValid = false
			}
		}
		return err
	}
	for _, m := range c.mx.tables {
		if m.owner == db {
			// A missing detail sample cannot supply the size for this observation.
			m.bloatValid = m.updated
			if !m.updated || m.bloatSize == nil {
				continue
			}
			m.bloatSize, m.bloatSizePerc = new(int64(0)), new(int64(0))
		}
	}
	for _, m := range c.mx.indexes {
		if m.owner == db {
			m.bloatValid = m.updated
			if !m.updated || m.bloatSize == nil {
				continue
			}
			m.bloatSize, m.bloatSizePerc = new(int64(0)), new(int64(0))
		}
	}
	for _, row := range results {
		if c.hasTableMetrics(row.table, row.database, row.schema) {
			v := c.getTableMetrics(row.table, row.database, row.schema)
			if v.owner == db && v.updated {
				v.bloatSize, v.bloatSizePerc = new(row.tableWasted), new(calcPercentage(row.tableWasted, v.totalSize))
			}
		}
		if row.index != "?" && c.hasIndexMetrics(row.index, row.table, row.database, row.schema) {
			v := c.getIndexMetrics(row.index, row.table, row.database, row.schema)
			if v.owner == db && v.updated {
				v.bloatSize, v.bloatSizePerc = new(row.indexWasted), new(calcPercentage(row.indexWasted, v.size))
			}
		}
	}
	return nil
}
