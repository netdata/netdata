// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

// ValueOptions defines per-column formatting settings.
type ValueOptions struct {
	Transform     FieldTransform
	DecimalPoints int
	DefaultValue  any
}

// Column defines a table column for function responses.
type Column struct {
	Index                 int
	Name                  string
	Type                  FieldType
	Units                 string
	Visualization         FieldVisual
	Sort                  FieldSort
	Sortable              bool
	Sticky                bool
	Summary               FieldSummary
	Filter                FieldFilter
	FullWidth             bool
	Wrap                  bool
	DefaultExpandedFilter bool
	UniqueKey             bool
	Visible               bool
	ValueOptions          ValueOptions
	Max                   *float64
	PointerTo             string
	Dummy                 bool
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
