// SPDX-License-Identifier: GPL-3.0-or-later

package sqlquery

import (
	"context"
	"fmt"
)

type PlaceholderStyle int

const (
	PlaceholderQuestion PlaceholderStyle = iota
	PlaceholderDollar
)

func tableColumnsQuery(style PlaceholderStyle) (string, error) {
	switch style {
	case PlaceholderQuestion:
		return `
SELECT COLUMN_NAME
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = ?
  AND TABLE_NAME = ?`, nil
	case PlaceholderDollar:
		return `
SELECT COLUMN_NAME
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = $1
  AND TABLE_NAME = $2`, nil
	default:
		return "", fmt.Errorf("unsupported placeholder style: %d", style)
	}
}

// FetchTableColumns returns the set of column names for schema.table.
// If transform is non-nil, it is applied to each column name before insertion.
func FetchTableColumns(ctx context.Context, q Queryer, schema, table string, style PlaceholderStyle, transform func(string) string) (map[string]bool, error) {
	query, err := tableColumnsQuery(style)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, query, schema, table)
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
