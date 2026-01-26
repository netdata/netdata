// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testColumn is a test column type that embeds ColumnMeta.
type testColumn struct {
	ColumnMeta
	Extra string // Simulates collector-specific field
}

func testColumnSet(cols ...testColumn) ColumnSet[testColumn] {
	return Columns(cols, func(c testColumn) ColumnMeta { return c.ColumnMeta })
}

func TestColumnSet_Len(t *testing.T) {
	cs := testColumnSet(
		testColumn{ColumnMeta: ColumnMeta{Name: "a"}},
		testColumn{ColumnMeta: ColumnMeta{Name: "b"}},
	)
	assert.Equal(t, 2, cs.Len())
}

func TestColumnSet_Names(t *testing.T) {
	cs := testColumnSet(
		testColumn{ColumnMeta: ColumnMeta{Name: "col1"}},
		testColumn{ColumnMeta: ColumnMeta{Name: "col2"}},
		testColumn{ColumnMeta: ColumnMeta{Name: "col3"}},
	)
	assert.Equal(t, []string{"col1", "col2", "col3"}, cs.Names())
}

func TestColumnSet_ContainsColumn(t *testing.T) {
	cs := testColumnSet(
		testColumn{ColumnMeta: ColumnMeta{Name: "exists"}},
	)
	assert.True(t, cs.ContainsColumn("exists"))
	assert.False(t, cs.ContainsColumn("not_exists"))
}

func TestColumnSet_BuildColumns(t *testing.T) {
	cs := testColumnSet(
		testColumn{ColumnMeta: ColumnMeta{
			Name:     "query",
			Tooltip:  "Query",
			Type:     FieldTypeString,
			Visible:  true,
			Sortable: true,
			Filter:   FieldFilterMultiselect,
		}},
		testColumn{ColumnMeta: ColumnMeta{
			Name:          "duration",
			Tooltip:       "Duration",
			Type:          FieldTypeDuration,
			Units:         "ms",
			Visible:       true,
			Sortable:      true,
			Transform:     FieldTransformDuration,
			DecimalPoints: 2,
		}},
	)

	result := cs.BuildColumns()

	require.Len(t, result, 2)
	require.Contains(t, result, "query")
	require.Contains(t, result, "duration")

	// Check query column
	queryCol := result["query"].(map[string]any)
	assert.Equal(t, 0, queryCol["index"])
	assert.Equal(t, "Query", queryCol["name"])
	assert.Equal(t, "string", queryCol["type"])
	assert.Equal(t, true, queryCol["visible"])
	assert.Equal(t, true, queryCol["sortable"])

	// Check duration column - should auto-switch to bar visualization
	durationCol := result["duration"].(map[string]any)
	assert.Equal(t, 1, durationCol["index"])
	assert.Equal(t, "Duration", durationCol["name"])
	assert.Equal(t, "duration", durationCol["type"])
	assert.Equal(t, "bar", durationCol["visualization"]) // Auto-switched from value to bar
	assert.Equal(t, "ms", durationCol["units"])
}

func TestColumnSet_BuildColumns_ExplicitVisualization(t *testing.T) {
	// When visualization is explicitly set, it should NOT be overridden
	cs := testColumnSet(
		testColumn{ColumnMeta: ColumnMeta{
			Name:          "duration",
			Tooltip:       "Duration",
			Type:          FieldTypeDuration,
			Visualization: FieldVisualPill, // Explicitly set to pill
		}},
	)

	result := cs.BuildColumns()
	durationCol := result["duration"].(map[string]any)
	assert.Equal(t, "pill", durationCol["visualization"]) // Should remain pill, not bar
}

func TestColumnSet_BuildCharts(t *testing.T) {
	cs := testColumnSet(
		testColumn{ColumnMeta: ColumnMeta{
			Name:    "query",
			Tooltip: "Query",
			Chart:   nil, // Not a chart metric
		}},
		testColumn{ColumnMeta: ColumnMeta{
			Name:    "exec_time",
			Tooltip: "Execution Time",
			Chart:   &ChartOptions{Group: "Time", Title: "Query Time"},
		}},
		testColumn{ColumnMeta: ColumnMeta{
			Name:    "cpu_time",
			Tooltip: "CPU Time",
			Chart:   &ChartOptions{Group: "Time", Title: "Query Time"},
		}},
		testColumn{ColumnMeta: ColumnMeta{
			Name:    "rows",
			Tooltip: "Rows",
			Chart:   &ChartOptions{Group: "Rows", Title: "Row Count"},
		}},
	)

	result := cs.BuildCharts()

	require.Len(t, result, 2)

	// Time chart should have 2 columns
	timeChart := result["Time"]
	assert.Equal(t, "Query Time", timeChart.Name)
	assert.Equal(t, "stacked-bar", timeChart.Type)
	assert.Equal(t, []string{"exec_time", "cpu_time"}, timeChart.Columns)

	// Rows chart should have 1 column
	rowsChart := result["Rows"]
	assert.Equal(t, "Row Count", rowsChart.Name)
	assert.Equal(t, []string{"rows"}, rowsChart.Columns)
}

func TestColumnSet_BuildCharts_FallbackTitle(t *testing.T) {
	cs := testColumnSet(
		testColumn{ColumnMeta: ColumnMeta{
			Name:  "metric",
			Chart: &ChartOptions{Group: "MyGroup", Title: ""}, // Empty title
		}},
	)

	result := cs.BuildCharts()
	assert.Equal(t, "MyGroup", result["MyGroup"].Name) // Falls back to Group
}

func TestColumnSet_BuildGroupBy(t *testing.T) {
	cs := testColumnSet(
		testColumn{ColumnMeta: ColumnMeta{
			Name:    "query",
			Tooltip: "Query",
			GroupBy: &GroupByOptions{},
		}},
		testColumn{ColumnMeta: ColumnMeta{
			Name:    "database",
			Tooltip: "Database",
			GroupBy: &GroupByOptions{},
		}},
		testColumn{ColumnMeta: ColumnMeta{
			Name:    "duration",
			Tooltip: "Duration",
			GroupBy: nil, // Not available for grouping
		}},
	)

	result := cs.BuildGroupBy()

	require.Len(t, result, 2)
	assert.Equal(t, "Group by Query", result["query"].Name)
	assert.Equal(t, []string{"query"}, result["query"].Columns)
	assert.Equal(t, "Group by Database", result["database"].Name)
}

func TestColumnSet_FindDefaultGroupBy(t *testing.T) {
	tests := []struct {
		name     string
		cols     []testColumn
		expected string
	}{
		{
			name: "returns default groupby if exists",
			cols: []testColumn{
				{ColumnMeta: ColumnMeta{Name: "a", GroupBy: &GroupByOptions{}}},
				{ColumnMeta: ColumnMeta{Name: "b", GroupBy: &GroupByOptions{IsDefault: true}}},
				{ColumnMeta: ColumnMeta{Name: "c", GroupBy: &GroupByOptions{}}},
			},
			expected: "b",
		},
		{
			name: "falls back to first groupby if no default",
			cols: []testColumn{
				{ColumnMeta: ColumnMeta{Name: "a", GroupBy: nil}},
				{ColumnMeta: ColumnMeta{Name: "b", GroupBy: &GroupByOptions{}}},
				{ColumnMeta: ColumnMeta{Name: "c", GroupBy: &GroupByOptions{}}},
			},
			expected: "b",
		},
		{
			name: "returns empty if no groupby columns",
			cols: []testColumn{
				{ColumnMeta: ColumnMeta{Name: "a"}},
				{ColumnMeta: ColumnMeta{Name: "b"}},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := testColumnSet(tt.cols...)
			assert.Equal(t, tt.expected, cs.FindDefaultGroupBy())
		})
	}
}

func TestColumnSet_FindDefaultChartGroups(t *testing.T) {
	tests := []struct {
		name     string
		cols     []testColumn
		expected []string
	}{
		{
			name: "returns default chart groups",
			cols: []testColumn{
				{ColumnMeta: ColumnMeta{Name: "a", Chart: &ChartOptions{Group: "A", IsDefault: true}}},
				{ColumnMeta: ColumnMeta{Name: "b", Chart: &ChartOptions{Group: "B", IsDefault: false}}},
				{ColumnMeta: ColumnMeta{Name: "c", Chart: &ChartOptions{Group: "C", IsDefault: true}}},
			},
			expected: []string{"A", "C"},
		},
		{
			name: "falls back to all groups if no defaults",
			cols: []testColumn{
				{ColumnMeta: ColumnMeta{Name: "a", Chart: &ChartOptions{Group: "A", IsDefault: false}}},
				{ColumnMeta: ColumnMeta{Name: "b", Chart: &ChartOptions{Group: "B", IsDefault: false}}},
			},
			expected: []string{"A", "B"},
		},
		{
			name: "deduplicates groups",
			cols: []testColumn{
				{ColumnMeta: ColumnMeta{Name: "a", Chart: &ChartOptions{Group: "Same", IsDefault: true}}},
				{ColumnMeta: ColumnMeta{Name: "b", Chart: &ChartOptions{Group: "Same", IsDefault: true}}},
			},
			expected: []string{"Same"},
		},
		{
			name: "returns nil for no chart columns",
			cols: []testColumn{
				{ColumnMeta: ColumnMeta{Name: "a", Chart: nil}},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cs := testColumnSet(tt.cols...)
			assert.Equal(t, tt.expected, cs.FindDefaultChartGroups())
		})
	}
}

func TestColumnSet_BuildDefaultCharts(t *testing.T) {
	cs := testColumnSet(
		testColumn{ColumnMeta: ColumnMeta{Name: "query", GroupBy: &GroupByOptions{IsDefault: true}}},
		testColumn{ColumnMeta: ColumnMeta{Name: "time", Chart: &ChartOptions{Group: "Time", IsDefault: true}}},
		testColumn{ColumnMeta: ColumnMeta{Name: "rows", Chart: &ChartOptions{Group: "Rows", IsDefault: true}}},
	)

	result := cs.BuildDefaultCharts()

	expected := DefaultCharts{
		{Chart: "Time", GroupBy: "query"},
		{Chart: "Rows", GroupBy: "query"},
	}
	assert.Equal(t, expected, result)

	// Test Build() method for JSON output
	assert.Equal(t, [][]string{{"Time", "query"}, {"Rows", "query"}}, result.Build())
}

func TestColumnSet_BuildDefaultCharts_NoGroupBy(t *testing.T) {
	cs := testColumnSet(
		testColumn{ColumnMeta: ColumnMeta{Name: "metric", Chart: &ChartOptions{Group: "G"}}},
	)

	result := cs.BuildDefaultCharts()
	assert.Nil(t, result)
}

func TestColumnSet_Empty(t *testing.T) {
	cs := testColumnSet()

	assert.Equal(t, 0, cs.Len())
	assert.Empty(t, cs.Names())
	assert.Empty(t, cs.BuildColumns())
	assert.Empty(t, cs.BuildCharts())
	assert.Empty(t, cs.BuildGroupBy())
	assert.Nil(t, cs.BuildDefaultCharts())
	assert.Equal(t, "", cs.FindDefaultGroupBy())
	assert.Nil(t, cs.FindDefaultChartGroups())
	assert.False(t, cs.ContainsColumn("any"))
}
