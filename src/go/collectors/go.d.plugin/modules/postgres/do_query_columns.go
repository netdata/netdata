// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import "database/sql"

func (p *Postgres) doQueryColumns() error {
	if err := p.doDBQueryColumns(p.db); err != nil {
		p.Warning(err)
	}
	for _, conn := range p.dbConns {
		if conn.db == nil {
			continue
		}
		if err := p.doDBQueryColumns(conn.db); err != nil {
			p.Warning(err)
		}
	}
	return nil
}

func (p *Postgres) doDBQueryColumns(db *sql.DB) error {
	q := queryColumnsStats()

	for _, m := range p.mx.tables {
		if m.nullColumns != nil {
			m.nullColumns = newInt(0)
		}
	}

	var dbname, schema, table string
	var nullPerc int64
	return p.doDBQuery(db, q, func(column, value string, rowEnd bool) {
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
		if nullPerc == 100 && p.hasTableMetrics(table, dbname, schema) {
			v := p.getTableMetrics(table, dbname, schema)
			if v.nullColumns == nil {
				v.nullColumns = newInt(0)
			}
			*v.nullColumns++
		}
	})
}
