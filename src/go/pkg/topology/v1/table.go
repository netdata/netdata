// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import "fmt"

func NewTable(rows int, columns []Column, values []ColumnEncoding) (Table, error) {
	table := Table{
		Rows:    rows,
		Columns: append([]Column(nil), columns...),
		Values:  append([]ColumnEncoding(nil), values...),
	}
	if err := table.Validate(); err != nil {
		return Table{}, err
	}
	return table, nil
}

func MustTable(rows int, columns []Column, values []ColumnEncoding) Table {
	table, err := NewTable(rows, columns, values)
	if err != nil {
		panic(err)
	}
	return table
}

func EmptyTable() Table {
	return Table{
		Rows:    0,
		Columns: []Column{},
		Values:  []ColumnEncoding{},
	}
}

func (table Table) Validate() error {
	if table.Rows < 0 {
		return fmt.Errorf("rows is negative: %d", table.Rows)
	}
	if len(table.Columns) != len(table.Values) {
		return fmt.Errorf("columns/values length mismatch: %d columns, %d values", len(table.Columns), len(table.Values))
	}

	for i, column := range table.Columns {
		if column.ID == "" {
			return fmt.Errorf("columns[%d].id is empty", i)
		}
		if column.Type == "" {
			return fmt.Errorf("columns[%d].type is empty", i)
		}
		if err := validateEncoding(table.Rows, i, table.Values[i]); err != nil {
			return err
		}
	}

	return nil
}

func validateEncoding(rows, columnIndex int, encoding ColumnEncoding) error {
	switch value := encoding.(type) {
	case nil:
		return fmt.Errorf("values[%d] is nil", columnIndex)
	case ConstEncoding:
		if value.Codec != "const" {
			return fmt.Errorf("values[%d] const encoding has invalid codec %q", columnIndex, value.Codec)
		}
	case *ConstEncoding:
		if value == nil {
			return fmt.Errorf("values[%d] is nil", columnIndex)
		}
		if value.Codec != "const" {
			return fmt.Errorf("values[%d] const encoding has invalid codec %q", columnIndex, value.Codec)
		}
	case ValuesEncoding:
		if value.Codec != "values" {
			return fmt.Errorf("values[%d] values encoding has invalid codec %q", columnIndex, value.Codec)
		}
		if len(value.Values) != rows {
			return fmt.Errorf("values[%d] decoded length mismatch: expected %d, got %d", columnIndex, rows, len(value.Values))
		}
	case *ValuesEncoding:
		if value == nil {
			return fmt.Errorf("values[%d] is nil", columnIndex)
		}
		if value.Codec != "values" {
			return fmt.Errorf("values[%d] values encoding has invalid codec %q", columnIndex, value.Codec)
		}
		if len(value.Values) != rows {
			return fmt.Errorf("values[%d] decoded length mismatch: expected %d, got %d", columnIndex, rows, len(value.Values))
		}
	case DictEncoding:
		if value.Codec != "dict" {
			return fmt.Errorf("values[%d] dict encoding has invalid codec %q", columnIndex, value.Codec)
		}
		if err := validateDictEncoding(rows, columnIndex, value.Values, value.Indexes); err != nil {
			return err
		}
	case *DictEncoding:
		if value == nil {
			return fmt.Errorf("values[%d] is nil", columnIndex)
		}
		if value.Codec != "dict" {
			return fmt.Errorf("values[%d] dict encoding has invalid codec %q", columnIndex, value.Codec)
		}
		if err := validateDictEncoding(rows, columnIndex, value.Values, value.Indexes); err != nil {
			return err
		}
	default:
		return fmt.Errorf("values[%d] has unsupported encoding type %T", columnIndex, encoding)
	}
	return nil
}

func validateDictEncoding(rows, columnIndex int, values []any, indexes []int) error {
	if len(indexes) != rows {
		return fmt.Errorf("values[%d] decoded length mismatch: expected %d, got %d", columnIndex, rows, len(indexes))
	}
	for i, index := range indexes {
		if index < 0 || index >= len(values) {
			return fmt.Errorf("values[%d].indexes[%d] out of bounds: %d", columnIndex, i, index)
		}
	}
	return nil
}
