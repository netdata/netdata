// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

// ValueOptions defines how the UI should format values in this column.
type ValueOptions struct {
	// Transform controls value formatting (number, duration, datetime, text, xml).
	Transform FieldTransform
	// DecimalPoints sets numeric precision when using number formatting.
	DecimalPoints int
	// DefaultValue includes a default value in the UI response.
	DefaultValue any
}

// Column defines a table column for function responses.
type Column struct {
	// Index is the 0-based position in each row array and must match data order.
	Index int
	// Name is the column tooltip shown in the UI.
	// The column header is the map key used in the columns map, not this field.
	Name string
	// Type controls the base data type and default rendering.
	Type FieldType
	// Units sets the unit label shown next to values (use for numeric columns).
	Units string
	// Visualization selects the visual style (value, pill, bar, rich content).
	Visualization FieldVisual
	// Sort sets the default sort direction.
	Sort FieldSort
	// Sortable allows users to change the sort order in the UI.
	Sortable bool
	// Sticky pins the column during horizontal scroll.
	Sticky bool
	// Summary defines how values aggregate when grouping rows.
	Summary FieldSummary
	// Filter selects the filter UI type (range for numbers, multiselect for categories, facet for logs).
	Filter FieldFilter
	// FullWidth lets the column expand to fill available width.
	FullWidth bool
	// Wrap enables text wrapping for long values.
	Wrap bool
	// DefaultExpandedFilter expands this filter by default.
	DefaultExpandedFilter bool
	// UniqueKey marks the unique row identifier column.
	UniqueKey bool
	// Visible shows the column by default.
	Visible bool
	// ValueOptions controls value formatting (transform, decimals, defaults).
	ValueOptions ValueOptions
	// Max sets the upper bound for bar-with-integer visuals.
	Max *float64
	// PointerTo includes a pointer target identifier in the UI response.
	PointerTo string
	// Dummy includes a dummy flag in the UI response.
	Dummy bool
}

// BuildColumn converts a Column definition to the JSON map used by the UI.
func (c Column) BuildColumn() map[string]any {
	col := map[string]any{
		"index":                   c.Index,
		"unique_key":              c.UniqueKey,
		"name":                    c.Name,
		"visible":                 c.Visible,
		"type":                    c.Type.String(),
		"visualization":           c.Visualization.String(),
		"sort":                    c.Sort.String(),
		"sortable":                c.Sortable,
		"sticky":                  c.Sticky,
		"summary":                 c.Summary.String(),
		"filter":                  c.Filter.String(),
		"full_width":              c.FullWidth,
		"wrap":                    c.Wrap,
		"default_expanded_filter": c.DefaultExpandedFilter,
	}

	if c.Units != "" {
		col["units"] = c.Units
	}
	if c.Max != nil {
		col["max"] = *c.Max
	}
	if c.PointerTo != "" {
		col["pointer_to"] = c.PointerTo
	}
	if c.Dummy {
		col["dummy"] = true
	}

	valueOpts := map[string]any{
		"transform":      c.ValueOptions.Transform.String(),
		"decimal_points": c.ValueOptions.DecimalPoints,
		"default_value":  c.ValueOptions.DefaultValue,
	}
	if c.Units != "" {
		valueOpts["units"] = c.Units
	}
	col["value_options"] = valueOpts

	return col
}
