// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestMssqlMethods(t *testing.T) {
	methods := mssqlMethods()

	require := assert.New(t)
	require.Len(methods, 1)
	require.Equal("top-queries", methods[0].ID)
	require.Equal("Top Queries", methods[0].Name)
	require.NotEmpty(methods[0].RequiredParams)

	// Verify at least one default sort option exists
	var sortParam *funcapi.ParamConfig
	for i := range methods[0].RequiredParams {
		if methods[0].RequiredParams[i].ID == "__sort" {
			sortParam = &methods[0].RequiredParams[i]
			break
		}
	}
	require.NotNil(sortParam, "expected __sort required param")
	require.NotEmpty(sortParam.Options)

	hasDefault := false
	for _, opt := range sortParam.Options {
		if opt.Default {
			hasDefault = true
			require.Equal("totalTime", opt.ID) // camelCase for UI
			break
		}
	}
	require.True(hasDefault, "should have a default sort option")
}

func TestMssqlAllColumns_HasRequiredColumns(t *testing.T) {
	// Verify all required base columns are defined
	requiredIDs := []string{
		"queryHash", "query", "database", "calls",
		"totalTime", "avgTime", "avgCpu",
		"avgReads", "avgWrites",
	}

	cs := mssqlColumnSet(mssqlAllColumns)
	for _, id := range requiredIDs {
		assert.True(t, cs.ContainsColumn(id), "column %s should be defined in mssqlAllColumns", id)
	}
}

func TestMssqlAllColumns_HasValidMetadata(t *testing.T) {
	for _, col := range mssqlAllColumns {
		// Every column must have an ID
		assert.NotEmpty(t, col.Name, "column %s must have ID", col.DBColumn)

		// Every column must have a display name
		assert.NotEmpty(t, col.Name, "column %s must have Name", col.Name)

		// Every column must have a data type
		assert.NotEqual(t, funcapi.FieldTypeNone, col.Type, "column %s must have Type", col.Name)

		// Duration columns must have units
		if col.Type == funcapi.FieldTypeDuration {
			assert.NotEmpty(t, col.Units, "duration column %s must have Units", col.Name)
		}

		// Sort options must have labels
		if col.IsSortOption {
			assert.NotEmpty(t, col.SortLabel, "sort option column %s must have SortLabel", col.Name)
		}
	}
}

func TestCollector_mapAndValidateMSSQLSortColumn(t *testing.T) {
	// Build a filtered column list with common available columns
	availableCols := map[string]bool{"avg_duration": true, "count_executions": true}
	c := &Collector{}
	cols := c.buildAvailableMSSQLColumns(availableCols)

	tests := map[string]struct {
		cols     []mssqlColumn
		input    string
		expected string
	}{
		"totalTime maps correctly": {
			cols:     cols,
			input:    "totalTime",
			expected: "totalTime",
		},
		"calls maps correctly": {
			cols:     cols,
			input:    "calls",
			expected: "calls",
		},
		"invalid column falls back to first sort option": {
			cols:     cols,
			input:    "invalid_column",
			expected: "calls", // first sort option in filtered cols
		},
		"SQL injection attempt falls back to first sort option": {
			cols:     cols,
			input:    "'; DROP TABLE users;--",
			expected: "calls", // first sort option in filtered cols
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := c.mapAndValidateMSSQLSortColumn(tc.input, tc.cols)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCollector_buildAvailableMSSQLColumns(t *testing.T) {
	tests := map[string]struct {
		availableCols map[string]bool
		expectCols    []string // Column IDs we expect to see
		notExpectCols []string // Column IDs we don't expect
	}{
		"SQL Server 2016 columns": {
			availableCols: map[string]bool{
				"count_executions": true,
				"avg_duration":     true, "last_duration": true, "min_duration": true, "max_duration": true,
				"avg_cpu_time": true, "last_cpu_time": true, "min_cpu_time": true, "max_cpu_time": true,
				"avg_logical_io_reads": true, "avg_logical_io_writes": true,
			},
			expectCols:    []string{"queryHash", "query", "database", "calls", "totalTime", "avgTime", "avgCpu", "avgReads"},
			notExpectCols: []string{"avgLogBytes", "avgTempdb"}, // SQL Server 2017+ only
		},
		"SQL Server 2017 with log bytes and tempdb": {
			availableCols: map[string]bool{
				"count_executions":      true,
				"avg_duration":          true,
				"avg_cpu_time":          true,
				"avg_log_bytes_used":    true,
				"avg_tempdb_space_used": true,
			},
			expectCols: []string{"queryHash", "query", "calls", "avgLogBytes", "avgTempdb"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{}
			cols := c.buildAvailableMSSQLColumns(tc.availableCols)
			cs := mssqlColumnSet(cols)

			for _, id := range tc.expectCols {
				assert.True(t, cs.ContainsColumn(id), "expected column %s to be present", id)
			}
			for _, id := range tc.notExpectCols {
				assert.False(t, cs.ContainsColumn(id), "did not expect column %s to be present", id)
			}
		})
	}
}

func TestCollector_buildMSSQLDynamicSQL(t *testing.T) {
	c := &Collector{}

	cols := []mssqlColumn{
		{ColumnMeta: funcapi.ColumnMeta{Name: "queryHash", Type: funcapi.FieldTypeString}, DBColumn: "query_hash", IsIdentity: true},
		{ColumnMeta: funcapi.ColumnMeta{Name: "query", Type: funcapi.FieldTypeString}, DBColumn: "query_sql_text", IsIdentity: true},
		{ColumnMeta: funcapi.ColumnMeta{Name: "database", Type: funcapi.FieldTypeString}, DBColumn: "database_name", IsIdentity: true},
		{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Type: funcapi.FieldTypeInteger}, DBColumn: "count_executions"},
		{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Type: funcapi.FieldTypeDuration}, DBColumn: "avg_duration", IsMicroseconds: true},
	}

	// sortColumn is the ID which is also used as the SQL alias
	sql := c.buildMSSQLDynamicSQL(cols, "totalTime", 7, 500)

	// Basic query structure
	assert.Contains(t, sql, "sys.query_store_query")
	assert.Contains(t, sql, "AS [totalTime]")
	assert.Contains(t, sql, "ORDER BY [totalTime] DESC")
	assert.Contains(t, sql, "TOP 500")
	assert.Contains(t, sql, "DATEADD")
	assert.Contains(t, sql, "q.query_hash")

	// Cross-database aggregation features
	assert.Contains(t, sql, "QUOTENAME(name)")                              // Safe database name escaping
	assert.Contains(t, sql, "UNION ALL")                                    // Combining results from multiple databases
	assert.Contains(t, sql, "sp_executesql")                                // Executing dynamic SQL
	assert.Contains(t, sql, "sys.databases")                                // Finding databases with Query Store
	assert.Contains(t, sql, "is_query_store_on = 1")                        // Condition for Query Store enabled
	assert.Contains(t, sql, "NOT IN ('master', 'tempdb', 'model', 'msdb')") // Excluding system databases
}

func TestCollector_buildMSSQLDynamicSQL_NoTimeFilter(t *testing.T) {
	c := &Collector{}

	cols := []mssqlColumn{
		{ColumnMeta: funcapi.ColumnMeta{Name: "queryHash", Type: funcapi.FieldTypeString}, DBColumn: "query_hash", IsIdentity: true},
		{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Type: funcapi.FieldTypeInteger}, DBColumn: "count_executions"},
	}

	sql := c.buildMSSQLDynamicSQL(cols, "calls", 0, 500)

	assert.Contains(t, sql, "sys.query_store_query")
	assert.Contains(t, sql, "TOP 500")
	assert.Contains(t, sql, "ORDER BY [calls] DESC")
	assert.NotContains(t, sql, "DATEADD")
}

func TestMssqlColumnSet_BuildColumns(t *testing.T) {
	cols := []mssqlColumn{
		{ColumnMeta: funcapi.ColumnMeta{Name: "queryHash", Tooltip: "Query Hash", Type: funcapi.FieldTypeString, Visible: false, UniqueKey: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect}},
		{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sticky: true, FullWidth: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect}},
		{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "seconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange}},
	}

	cs := mssqlColumnSet(cols)
	columns := cs.BuildColumns()

	// Verify column count
	assert.Len(t, columns, 3)

	// Verify queryHash column
	queryHashCol := columns["queryHash"].(map[string]any)
	assert.Equal(t, "Query Hash", queryHashCol["name"])
	assert.Equal(t, "string", queryHashCol["type"])
	assert.True(t, queryHashCol["unique_key"].(bool))
	assert.False(t, queryHashCol["visible"].(bool))
	assert.Equal(t, 0, queryHashCol["index"])

	// Verify query column
	queryCol := columns["query"].(map[string]any)
	assert.Equal(t, "Query", queryCol["name"])
	assert.True(t, queryCol["sticky"].(bool))
	assert.True(t, queryCol["full_width"].(bool))
	assert.Equal(t, 1, queryCol["index"])

	// Verify totalTime column
	totalTimeCol := columns["totalTime"].(map[string]any)
	assert.Equal(t, "Total Time", totalTimeCol["name"])
	assert.Equal(t, "duration", totalTimeCol["type"])
	assert.Equal(t, "seconds", totalTimeCol["units"])
	assert.Equal(t, "bar", totalTimeCol["visualization"]) // duration uses bar
	assert.Equal(t, 2, totalTimeCol["index"])
}

// Test that method config sort options have valid column references
func TestMssqlMethods_SortOptionsHaveLabels(t *testing.T) {
	methods := mssqlMethods()

	for _, method := range methods {
		var sortParam *funcapi.ParamConfig
		for i := range method.RequiredParams {
			if method.RequiredParams[i].ID == "__sort" {
				sortParam = &method.RequiredParams[i]
				break
			}
		}
		assert.NotNil(t, sortParam)
		for _, opt := range sortParam.Options {
			assert.NotEmpty(t, opt.ID, "sort option must have ID")
			assert.NotEmpty(t, opt.Name, "sort option %s must have Name", opt.ID)
			assert.Contains(t, opt.Name, "Top queries by", "label should have standard prefix")
		}
	}
}

// TestMapAndValidateMSSQLSortColumn_NoSortOptions verifies fallback when no sort columns exist
func TestMapAndValidateMSSQLSortColumn_NoSortOptions(t *testing.T) {
	c := &Collector{}

	// Only identity columns available (no sort options)
	identityOnlyCols := []mssqlColumn{
		{ColumnMeta: funcapi.ColumnMeta{Name: "queryHash"}, IsIdentity: true},
		{ColumnMeta: funcapi.ColumnMeta{Name: "query"}, IsIdentity: true},
		{ColumnMeta: funcapi.ColumnMeta{Name: "database"}, IsIdentity: true},
	}

	result := c.mapAndValidateMSSQLSortColumn("totalTime", identityOnlyCols)
	// Should fall back to first column since no sort options exist
	assert.Equal(t, "queryHash", result, "should fall back to first column when no sort options")

	// Empty columns list
	result = c.mapAndValidateMSSQLSortColumn("totalTime", []mssqlColumn{})
	assert.Equal(t, "", result, "should return empty string when no columns available")
}

// TestSortColumnValidation_SQLInjection verifies that SQL injection attempts
// are handled by the validation mechanism
func TestMssqlSortColumnValidation_SQLInjection(t *testing.T) {
	c := &Collector{}
	availableCols := map[string]bool{"avg_duration": true, "count_executions": true}
	cols := c.buildAvailableMSSQLColumns(availableCols)

	maliciousInputs := []string{
		"'; DROP TABLE sys.query_store_query; --",
		"total_time_ms; DELETE FROM master.dbo.sysdatabases",
		"1 OR 1=1",
		"WAITFOR DELAY '00:00:10'",
		"xp_cmdshell 'whoami'",
	}

	for _, input := range maliciousInputs {
		result := c.mapAndValidateMSSQLSortColumn(input, cols)
		// All malicious inputs should fall back to first available sort option
		assert.Equal(t, "calls", result,
			"malicious input should fall back to safe default: %s -> %s", input, result)
	}
}
