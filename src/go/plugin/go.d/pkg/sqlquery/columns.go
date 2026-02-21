// SPDX-License-Identifier: GPL-3.0-or-later

package sqlquery

import "context"

const queryFetchTableColumns = `
SELECT COLUMN_NAME
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = ?
  AND TABLE_NAME = ?`

// FetchTableColumns returns the set of column names for schema.table.
// If transform is non-nil, it is applied to each column name before insertion.
// Note: this helper currently uses MySQL-style '?' placeholders and is intended
// for MySQL-family collectors in its current form.
func FetchTableColumns(ctx context.Context, q Queryer, schema, table string, transform func(string) string) (map[string]bool, error) {
	rows, err := q.QueryContext(ctx, queryFetchTableColumns, schema, table)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	cols := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if transform != nil {
			name = transform(name)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return cols, nil
}
