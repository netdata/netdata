// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"database/sql"
)

func (c *Collector) doQueryRow(query string, v any) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	return c.db.QueryRowContext(ctx, query).Scan(v)
}

func (c *Collector) doDBQueryRow(db *sql.DB, query string, v any) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	return db.QueryRowContext(ctx, query).Scan(v)
}

func (c *Collector) doQuery(query string, assign func(column, value string, rowEnd bool)) error {
	return c.doDBQuery(c.db, query, assign)
}

func (c *Collector) doDBQuery(db *sql.DB, query string, assign func(column, value string, rowEnd bool)) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout.Duration())
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	return readRows(rows, assign)
}

func readRows(rows *sql.Rows, assign func(column, value string, rowEnd bool)) error {
	if assign == nil {
		return nil
	}

	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	values := makeValues(len(columns))

	for rows.Next() {
		if err := rows.Scan(values...); err != nil {
			return err
		}
		for i, l := 0, len(values); i < l; i++ {
			assign(columns[i], valueToString(values[i]), i == l-1)
		}
	}
	return rows.Err()
}

func valueToString(value any) string {
	v, ok := value.(*sql.NullString)
	if !ok || !v.Valid {
		return ""
	}
	return v.String
}

func makeValues(size int) []any {
	vs := make([]any, size)
	for i := range vs {
		vs[i] = &sql.NullString{}
	}
	return vs
}
