// SPDX-License-Identifier: GPL-3.0-or-later

package sqlquery

import (
	"database/sql"
	"fmt"
)

type ScanValueType string

const (
	ScanValueString  ScanValueType = "string"
	ScanValueInteger ScanValueType = "integer"
	ScanValueFloat   ScanValueType = "float"
	ScanValueDiscard ScanValueType = "discard"
)

// ScanColumnSpec describes how a query result column should be scanned and normalized.
type ScanColumnSpec struct {
	Type      ScanValueType
	Transform func(any) any
}

// RowScanner is the minimal interface required for typed row scanning.
type RowScanner interface {
	Next() bool
	Scan(dest ...any) error
}

// ScanTypedRows scans all rows according to specs and returns normalized [][]any rows.
// The function intentionally does not call rows.Err(); callers keep control over that.
func ScanTypedRows(rows RowScanner, specs []ScanColumnSpec) ([][]any, error) {
	data := make([][]any, 0, 500)
	holders := makeScanHolders(specs)

	for rows.Next() {
		if err := rows.Scan(holders.ptrs...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		row := make([]any, len(specs))
		for i := range holders.ptrs {
			value, ok := holders.value(i)
			if ok && specs[i].Transform != nil {
				value = specs[i].Transform(value)
			}
			row[i] = value
		}
		data = append(data, row)
	}

	return data, nil
}

type scanHolders struct {
	ptrs []any
}

func makeScanHolders(specs []ScanColumnSpec) scanHolders {
	holders := scanHolders{ptrs: make([]any, len(specs))}
	for i, spec := range specs {
		switch spec.Type {
		case ScanValueString:
			holders.ptrs[i] = &sql.NullString{}
		case ScanValueInteger:
			holders.ptrs[i] = &sql.NullInt64{}
		case ScanValueFloat:
			holders.ptrs[i] = &sql.NullFloat64{}
		case ScanValueDiscard:
			holders.ptrs[i] = new(any)
		default:
			holders.ptrs[i] = &sql.NullString{}
		}
	}
	return holders
}

func (h scanHolders) value(i int) (any, bool) {
	switch v := h.ptrs[i].(type) {
	case *sql.NullString:
		if v.Valid {
			return v.String, true
		}
		return "", false
	case *sql.NullInt64:
		if v.Valid {
			return v.Int64, true
		}
		return int64(0), false
	case *sql.NullFloat64:
		if v.Valid {
			return v.Float64, true
		}
		return float64(0), false
	case *any:
		return nil, false
	default:
		return nil, false
	}
}
