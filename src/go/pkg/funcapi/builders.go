// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

// DefaultMaxQueryLength is the default maximum length for query text display.
const DefaultMaxQueryLength = 4096

// ColumnSet wraps a typed column slice with its accessor function.
// This allows all builder methods to work without repeating the accessor.
//
// Usage:
//
//	cs := funcapi.Columns(cols, func(c myColumn) funcapi.ColumnMeta { return c.ColumnMeta })
//	response := &funcapi.FunctionResponse{
//	    Columns:       cs.BuildColumns(),
//	    Charts:        cs.BuildCharts(),
//	    DefaultCharts: cs.BuildDefaultCharts(),
//	    GroupBy:       cs.BuildGroupBy(),
//	}
type ColumnSet[T any] struct {
	cols    []T
	getMeta func(T) ColumnMeta
}

// Columns creates a new ColumnSet from a typed slice and accessor.
func Columns[T any](cols []T, getMeta func(T) ColumnMeta) ColumnSet[T] {
	return ColumnSet[T]{cols: cols, getMeta: getMeta}
}

// Len returns the number of columns.
func (cs ColumnSet[T]) Len() int {
	return len(cs.cols)
}

// BuildColumns builds the columns map for FunctionResponse.
func (cs ColumnSet[T]) BuildColumns() map[string]any {
	result := make(map[string]any, len(cs.cols))
	for i, col := range cs.cols {
		meta := cs.getMeta(col)
		vis := meta.Visualization
		if vis == FieldVisualValue && meta.Type == FieldTypeDuration {
			vis = FieldVisualBar
		}
		c := Column{
			Index:                 i,
			Name:                  meta.Tooltip,
			Type:                  meta.Type,
			Units:                 meta.Units,
			Visualization:         vis,
			Sort:                  meta.Sort,
			Sortable:              meta.Sortable,
			Sticky:                meta.Sticky,
			Summary:               meta.Summary,
			Filter:                meta.Filter,
			FullWidth:             meta.FullWidth,
			Wrap:                  meta.Wrap,
			DefaultExpandedFilter: meta.ExpandFilter,
			UniqueKey:             meta.UniqueKey,
			Visible:               meta.Visible,
			ValueOptions: ValueOptions{
				Transform:     meta.Transform,
				DecimalPoints: meta.DecimalPoints,
			},
		}
		result[meta.Name] = c.BuildColumn()
	}
	return result
}

// BuildCharts builds chart configuration from column metadata.
// Columns with Chart != nil are grouped by Chart.Group.
func (cs ColumnSet[T]) BuildCharts() map[string]ChartConfig {
	charts := make(map[string]ChartConfig)
	for _, col := range cs.cols {
		meta := cs.getMeta(col)
		if meta.Chart == nil || meta.Chart.Group == "" {
			continue
		}
		cfg, ok := charts[meta.Chart.Group]
		if !ok {
			title := meta.Chart.Title
			if title == "" {
				title = meta.Chart.Group
			}
			cfg = ChartConfig{Name: title, Type: "stacked-bar"}
		}
		cfg.Columns = append(cfg.Columns, meta.Name)
		charts[meta.Chart.Group] = cfg
	}
	return charts
}

// BuildGroupBy builds group-by configuration from columns with GroupBy != nil.
func (cs ColumnSet[T]) BuildGroupBy() map[string]GroupByConfig {
	result := make(map[string]GroupByConfig)
	for _, col := range cs.cols {
		meta := cs.getMeta(col)
		if meta.GroupBy == nil {
			continue
		}
		result[meta.Name] = GroupByConfig{
			Name:    "Group by " + meta.Tooltip,
			Columns: []string{meta.Name},
		}
	}
	return result
}

// BuildDefaultCharts builds default chart configurations.
// Each chart uses its own DefaultGroupBy if set, otherwise falls back to global default.
func (cs ColumnSet[T]) BuildDefaultCharts() DefaultCharts {
	globalGroupBy := cs.FindDefaultGroupBy()
	groups := cs.FindDefaultChartGroups()
	var result DefaultCharts
	for _, g := range groups {
		groupBy := cs.findChartGroupBy(g)
		if groupBy == "" {
			groupBy = globalGroupBy
		}
		if groupBy == "" {
			continue
		}
		result = append(result, DefaultChart{Chart: g, GroupBy: groupBy})
	}
	return result
}

// findChartGroupBy returns the DefaultGroupBy for a specific chart group.
func (cs ColumnSet[T]) findChartGroupBy(chartGroup string) string {
	for _, col := range cs.cols {
		meta := cs.getMeta(col)
		if meta.Chart != nil && meta.Chart.Group == chartGroup && meta.Chart.DefaultGroupBy != "" {
			return meta.Chart.DefaultGroupBy
		}
	}
	return ""
}

// BuildCharting builds the complete charting configuration.
func (cs ColumnSet[T]) BuildCharting() ChartingConfig {
	return ChartingConfig{
		Charts:        cs.BuildCharts(),
		DefaultCharts: cs.BuildDefaultCharts(),
		GroupBy:       cs.BuildGroupBy(),
	}
}

// FindDefaultGroupBy finds the default grouping column name.
// Returns the column with GroupBy.IsDefault, or falls back to any GroupBy column.
func (cs ColumnSet[T]) FindDefaultGroupBy() string {
	for _, col := range cs.cols {
		meta := cs.getMeta(col)
		if meta.GroupBy != nil && meta.GroupBy.IsDefault {
			return meta.Name
		}
	}
	for _, col := range cs.cols {
		meta := cs.getMeta(col)
		if meta.GroupBy != nil {
			return meta.Name
		}
	}
	return ""
}

// FindDefaultChartGroups returns chart groups, preferring ones with IsDefault.
func (cs ColumnSet[T]) FindDefaultChartGroups() []string {
	var groups []string
	seen := make(map[string]bool)

	// First pass: chart groups marked as default
	for _, col := range cs.cols {
		meta := cs.getMeta(col)
		if meta.Chart != nil && meta.Chart.Group != "" && meta.Chart.IsDefault && !seen[meta.Chart.Group] {
			seen[meta.Chart.Group] = true
			groups = append(groups, meta.Chart.Group)
		}
	}
	if len(groups) > 0 {
		return groups
	}

	// Fallback: all chart groups
	for _, col := range cs.cols {
		meta := cs.getMeta(col)
		if meta.Chart != nil && meta.Chart.Group != "" && !seen[meta.Chart.Group] {
			seen[meta.Chart.Group] = true
			groups = append(groups, meta.Chart.Group)
		}
	}
	return groups
}

// ContainsColumn checks if a column name exists in the set.
func (cs ColumnSet[T]) ContainsColumn(name string) bool {
	for _, col := range cs.cols {
		if cs.getMeta(col).Name == name {
			return true
		}
	}
	return false
}

// Names returns all column names.
func (cs ColumnSet[T]) Names() []string {
	names := make([]string, len(cs.cols))
	for i, col := range cs.cols {
		names[i] = cs.getMeta(col).Name
	}
	return names
}
