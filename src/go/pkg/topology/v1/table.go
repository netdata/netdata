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

	seenColumns := make(map[string]struct{}, len(table.Columns))
	for i, column := range table.Columns {
		if column.ID == "" {
			return fmt.Errorf("columns[%d].id is empty", i)
		}
		if _, ok := seenColumns[column.ID]; ok {
			return fmt.Errorf("columns[%d].id duplicates column %q", i, column.ID)
		}
		seenColumns[column.ID] = struct{}{}
		if column.Type == "" {
			return fmt.Errorf("columns[%d].type is empty", i)
		}
		if err := validateColumnContract(i, column); err != nil {
			return err
		}
		if err := validateEncoding(table.Rows, i, column, table.Values[i]); err != nil {
			return err
		}
	}

	return nil
}

func validateColumnContract(columnIndex int, column Column) error {
	switch column.Type {
	case "string_ref", "ip_ref", "mac_ref":
		if column.Dictionary == "" {
			return fmt.Errorf("columns[%d] type %q requires dictionary", columnIndex, column.Type)
		}
	case "bool", "int", "uint", "float", "string", "timestamp", "duration", "ip", "mac", "actor_ref", "link_ref", "evidence_ref", "array", "json":
		if column.Dictionary != "" {
			return fmt.Errorf("columns[%d] uses dictionary with non-reference type %q", columnIndex, column.Type)
		}
	default:
		return fmt.Errorf("columns[%d] has unsupported type %q", columnIndex, column.Type)
	}

	return nil
}

func validateEncoding(rows, columnIndex int, column Column, encoding ColumnEncoding) error {
	values, err := decodeEncodingValues(rows, columnIndex, encoding)
	if err != nil {
		return err
	}
	for rowIndex, value := range values {
		if err := validateEncodedColumnValue(columnIndex, rowIndex, column, value); err != nil {
			return err
		}
	}
	return nil
}

func decodeEncodingValues(rows, columnIndex int, encoding ColumnEncoding) ([]any, error) {
	switch value := encoding.(type) {
	case nil:
		return nil, fmt.Errorf("values[%d] is nil", columnIndex)
	case ConstEncoding:
		if value.Codec != "const" {
			return nil, fmt.Errorf("values[%d] const encoding has invalid codec %q", columnIndex, value.Codec)
		}
		return repeatValue(rows, value.Value), nil
	case *ConstEncoding:
		if value == nil {
			return nil, fmt.Errorf("values[%d] is nil", columnIndex)
		}
		if value.Codec != "const" {
			return nil, fmt.Errorf("values[%d] const encoding has invalid codec %q", columnIndex, value.Codec)
		}
		return repeatValue(rows, value.Value), nil
	case ValuesEncoding:
		if value.Codec != "values" {
			return nil, fmt.Errorf("values[%d] values encoding has invalid codec %q", columnIndex, value.Codec)
		}
		if len(value.Values) != rows {
			return nil, fmt.Errorf("values[%d] decoded length mismatch: expected %d, got %d", columnIndex, rows, len(value.Values))
		}
		return append([]any(nil), value.Values...), nil
	case *ValuesEncoding:
		if value == nil {
			return nil, fmt.Errorf("values[%d] is nil", columnIndex)
		}
		if value.Codec != "values" {
			return nil, fmt.Errorf("values[%d] values encoding has invalid codec %q", columnIndex, value.Codec)
		}
		if len(value.Values) != rows {
			return nil, fmt.Errorf("values[%d] decoded length mismatch: expected %d, got %d", columnIndex, rows, len(value.Values))
		}
		return append([]any(nil), value.Values...), nil
	case DictEncoding:
		if value.Codec != "dict" {
			return nil, fmt.Errorf("values[%d] dict encoding has invalid codec %q", columnIndex, value.Codec)
		}
		if err := validateDictEncoding(rows, columnIndex, value.Values, value.Indexes); err != nil {
			return nil, err
		}
		return decodeDictValues(value.Values, value.Indexes), nil
	case *DictEncoding:
		if value == nil {
			return nil, fmt.Errorf("values[%d] is nil", columnIndex)
		}
		if value.Codec != "dict" {
			return nil, fmt.Errorf("values[%d] dict encoding has invalid codec %q", columnIndex, value.Codec)
		}
		if err := validateDictEncoding(rows, columnIndex, value.Values, value.Indexes); err != nil {
			return nil, err
		}
		return decodeDictValues(value.Values, value.Indexes), nil
	default:
		return nil, fmt.Errorf("values[%d] has unsupported encoding type %T", columnIndex, encoding)
	}
}

func repeatValue(rows int, value any) []any {
	values := make([]any, rows)
	for i := range values {
		values[i] = value
	}
	return values
}

func decodeDictValues(values []any, indexes []int) []any {
	decoded := make([]any, len(indexes))
	for i, index := range indexes {
		decoded[i] = values[index]
	}
	return decoded
}

func validateEncodedColumnValue(columnIndex, rowIndex int, column Column, value any) error {
	if value == nil {
		if column.Nullable {
			return nil
		}
		return fmt.Errorf("values[%d][%d] is null but column is not nullable", columnIndex, rowIndex)
	}

	switch column.Type {
	case "string_ref", "ip_ref", "mac_ref":
		if n, ok := integerValue(value); !ok || n < 0 {
			return fmt.Errorf("values[%d][%d] is not a non-negative dictionary reference", columnIndex, rowIndex)
		}
	case "actor_ref", "link_ref", "evidence_ref":
		if n, ok := integerValue(value); !ok || n < 0 {
			return fmt.Errorf("values[%d][%d] is not a non-negative %s reference", columnIndex, rowIndex, column.Type)
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("values[%d][%d] is not an array", columnIndex, rowIndex)
		}
	case "bool":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("values[%d][%d] is not a bool", columnIndex, rowIndex)
		}
	case "int":
		if _, ok := integerValue(value); !ok {
			return fmt.Errorf("values[%d][%d] is not an integer", columnIndex, rowIndex)
		}
	case "uint":
		if n, ok := integerValue(value); !ok || n < 0 {
			return fmt.Errorf("values[%d][%d] is not a non-negative integer", columnIndex, rowIndex)
		}
	case "float", "duration":
		if _, ok := numberValue(value); !ok {
			return fmt.Errorf("values[%d][%d] is not a number", columnIndex, rowIndex)
		}
	case "string", "ip", "mac", "timestamp":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("values[%d][%d] is not a string", columnIndex, rowIndex)
		}
	case "json":
		return nil
	default:
		return fmt.Errorf("columns[%d] has unsupported type %q", columnIndex, column.Type)
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
