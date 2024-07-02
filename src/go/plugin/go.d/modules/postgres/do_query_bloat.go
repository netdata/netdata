// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import "database/sql"

func (p *Postgres) doQueryBloat() error {
	if err := p.doDBQueryBloat(p.db); err != nil {
		p.Warning(err)
	}
	for _, conn := range p.dbConns {
		if conn.db == nil {
			continue
		}
		if err := p.doDBQueryBloat(conn.db); err != nil {
			p.Warning(err)
		}
	}
	return nil
}

func (p *Postgres) doDBQueryBloat(db *sql.DB) error {
	q := queryBloat()

	for _, m := range p.mx.tables {
		if m.bloatSize != nil {
			m.bloatSize = newInt(0)
		}
		if m.bloatSizePerc != nil {
			m.bloatSizePerc = newInt(0)
		}
	}
	for _, m := range p.mx.indexes {
		if m.bloatSize != nil {
			m.bloatSize = newInt(0)
		}
		if m.bloatSizePerc != nil {
			m.bloatSizePerc = newInt(0)
		}
	}

	var dbname, schema, table, iname string
	var tableWasted, idxWasted int64
	return p.doDBQuery(db, q, func(column, value string, rowEnd bool) {
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
		if p.hasTableMetrics(table, dbname, schema) {
			v := p.getTableMetrics(table, dbname, schema)
			v.bloatSize = newInt(tableWasted)
			v.bloatSizePerc = newInt(calcPercentage(tableWasted, v.totalSize))
		}
		if iname != "?" && p.hasIndexMetrics(iname, table, dbname, schema) {
			v := p.getIndexMetrics(iname, table, dbname, schema)
			v.bloatSize = newInt(idxWasted)
			v.bloatSizePerc = newInt(calcPercentage(idxWasted, v.size))
		}
	})
}
