// SPDX-License-Identifier: GPL-3.0-or-later

package sqlquery

import (
	"context"
	"database/sql"
	"time"
)

// Queryer is the minimal query interface required by query helpers.
type Queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// AssignFunc receives each row value as string and rowEnd=true for the last column.
type AssignFunc func(column, value string, rowEnd bool)

// QueryRows executes query and streams row values through assign.
// The returned duration measures query submission latency (QueryContext call).
func QueryRows(ctx context.Context, q Queryer, query string, assign AssignFunc, args ...any) (time.Duration, error) {
	start := time.Now()
	rows, err := q.QueryContext(ctx, query, args...)
	queryDuration := time.Since(start)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if err := readRows(rows, assign); err != nil {
		return queryDuration, err
	}
	return queryDuration, nil
}

// readRows scans all rows and invokes assign for every column value.
func readRows(rows *sql.Rows, assign AssignFunc) error {
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
		for i := range values {
			assign(columns[i], valueToString(values[i]), i == len(values)-1)
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
