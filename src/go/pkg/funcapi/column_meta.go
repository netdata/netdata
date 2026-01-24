// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

// ColumnMeta defines UI metadata for a table column.
// This struct contains ONLY column display and visualization properties.
//
// What belongs here: How to render and display a column in the table.
// What does NOT belong here: Data access (SQL, BSON), parameters (sort options).
//
// Collectors embed this and add their own fields for:
//   - Data access (SelectExpr, DBField, Value func, etc.)
//   - Parameter-related metadata (sort options, filter options - these are method-specific)
type ColumnMeta struct {
	// Identity
	Name    string // Column name/identifier in response (e.g., "execution_time")
	Tooltip string // Hover tooltip text shown in UI (e.g., "Execution Time")

	// Type and Display
	Type      FieldType
	Units     string
	Visible   bool
	Sortable  bool // Can this column be sorted in the table UI?
	Sticky    bool
	FullWidth bool
	Wrap      bool

	// Value Rendering
	Transform     FieldTransform
	DecimalPoints int
	Sort          FieldSort // Default sort direction for this column
	Summary       FieldSummary
	Filter        FieldFilter
	Visualization FieldVisual

	// Special Flags
	UniqueKey    bool
	ExpandFilter bool

	// Chart configuration (nil = column is not a chart metric)
	// Used by BuildCharts() and BuildDefaultCharts()
	Chart *ChartOptions

	// GroupBy configuration (nil = column not available for grouping)
	// Used by BuildGroupBy() and BuildDefaultCharts()
	GroupBy *GroupByOptions
}

// ChartOptions defines how a column participates in charts.
type ChartOptions struct {
	Group     string // Which chart this column belongs to (required)
	Title     string // Chart display title (defaults to Group if empty)
	IsDefault bool   // Include this chart in the default charts view
}

// GroupByOptions defines how a column participates in data grouping.
type GroupByOptions struct {
	IsDefault bool // This is the default grouping column for charts
}
