// SPDX-License-Identifier: GPL-3.0-or-later

package rawfunc

import (
	"slices"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// Method describes one Function method: its declaration and its table response.
// Columns is the built column map of the method's table.
type Method struct {
	ID, Title, Help, Sort string
	UpdateEvery           int
	History               bool
	Params                []string
	Columns               map[string]any
}

// Declaration publishes the method. Viewing any DEM Function needs a paid
// Space (access 0x0017, enforced by the Agent); handlers own raw input and info.
func (m Method) Declaration(tags string) funcapi.FunctionConfig {
	return funcapi.FunctionConfig{
		ID:                m.ID,
		FunctionName:      m.ID,
		Name:              m.Title,
		UpdateEvery:       m.UpdateEvery,
		Help:              m.Help,
		Tags:              tags,
		RequireCloud:      true,
		RequireCommercial: true,
		RawRequest:        true,
		ManagedInfo:       true,
		HasHistory:        m.History,
		AcceptedParams:    slices.Clone(m.Params),
	}
}

// Table returns a new complete table response without rows. Handlers set
// "data" and any method-specific fields.
func (m Method) Table() map[string]any {
	response := map[string]any{
		"status":              200,
		"type":                "table",
		"has_history":         m.History,
		"update_every":        m.UpdateEvery,
		"help":                m.Help,
		"columns":             m.Columns,
		"data":                [][]any{},
		"default_sort_column": m.Sort,
	}
	if len(m.Params) > 0 {
		response["accepted_params"] = m.Params
	}
	return response
}

// Column pairs a column's presentation with the value it shows for a row
// source T.
type Column[T any] struct {
	funcapi.ColumnMeta
	Value func(T) any
}

// Columns is the ordered column definition of one table: a column's position
// is its index in the column map and its slot in every row.
type Columns[T any] []Column[T]

// Build returns the table's column map.
func (c Columns[T]) Build() map[string]any {
	return funcapi.Columns(c, func(col Column[T]) funcapi.ColumnMeta { return col.ColumnMeta }).BuildColumns()
}

// Row returns the values of one row source in column order.
func (c Columns[T]) Row(src T) []any {
	row := make([]any, len(c))
	for i, col := range c {
		row[i] = col.Value(src)
	}
	return row
}
