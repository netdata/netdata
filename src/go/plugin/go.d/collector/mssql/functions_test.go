// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMssqlMethods(t *testing.T) {
	methods := mssqlMethods()

	require := assert.New(t)
	require.Len(methods, 1)
	require.Equal("top-queries", methods[0].ID)
	require.Equal("Top Queries", methods[0].Name)
	require.NotEmpty(methods[0].SortOptions)

	// Verify at least one default sort option exists
	hasDefault := false
	for _, opt := range methods[0].SortOptions {
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
	requiredUIKeys := []string{
		"queryHash", "query", "database", "calls",
		"totalTime", "avgTime", "avgCpu",
		"avgReads", "avgWrites",
	}

	uiKeys := make(map[string]bool)
	for _, col := range mssqlAllColumns {
		uiKeys[col.uiKey] = true
	}

	for _, key := range requiredUIKeys {
		assert.True(t, uiKeys[key], "column %s should be defined in mssqlAllColumns", key)
	}
}

func TestMssqlAllColumns_HasValidMetadata(t *testing.T) {
	for _, col := range mssqlAllColumns {
		// Every column must have a UI key
		assert.NotEmpty(t, col.uiKey, "column %s must have uiKey", col.dbColumn)

		// Every column must have a display name
		assert.NotEmpty(t, col.displayName, "column %s must have displayName", col.uiKey)

		// Every column must have a data type
		assert.NotEmpty(t, col.dataType, "column %s must have dataType", col.uiKey)
		assert.Contains(t, []string{"string", "integer", "float", "duration"}, col.dataType,
			"column %s has invalid dataType: %s", col.uiKey, col.dataType)

		// Duration columns must have units
		if col.dataType == "duration" {
			assert.NotEmpty(t, col.units, "duration column %s must have units", col.uiKey)
		}

		// Sort options must have labels
		if col.isSortOption {
			assert.NotEmpty(t, col.sortLabel, "sort option column %s must have sortLabel", col.uiKey)
		}
	}
}

func TestCollector_mapAndValidateMSSQLSortColumn(t *testing.T) {
	tests := map[string]struct {
		availableCols map[string]bool
		input         string
		expected      string
	}{
		"totalTime maps correctly": {
			availableCols: map[string]bool{"avg_duration": true, "count_executions": true},
			input:         "totalTime",
			expected:      "totalTime",
		},
		"calls maps correctly": {
			availableCols: map[string]bool{"avg_duration": true, "count_executions": true},
			input:         "calls",
			expected:      "calls",
		},
		"invalid column falls back to totalTime": {
			availableCols: map[string]bool{"avg_duration": true, "count_executions": true},
			input:         "invalid_column",
			expected:      "totalTime",
		},
		"SQL injection attempt falls back to totalTime": {
			availableCols: map[string]bool{"avg_duration": true, "count_executions": true},
			input:         "'; DROP TABLE users;--",
			expected:      "totalTime",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{}
			result := c.mapAndValidateMSSQLSortColumn(tc.input, tc.availableCols)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCollector_buildAvailableMSSQLColumns(t *testing.T) {
	tests := map[string]struct {
		availableCols map[string]bool
		expectCols    []string // UI keys we expect to see
		notExpectCols []string // UI keys we don't expect
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

			// Build map of UI keys for easy lookup
			uiKeys := make(map[string]bool)
			for _, col := range cols {
				uiKeys[col.uiKey] = true
			}

			for _, key := range tc.expectCols {
				assert.True(t, uiKeys[key], "expected column %s to be present", key)
			}
			for _, key := range tc.notExpectCols {
				assert.False(t, uiKeys[key], "did not expect column %s to be present", key)
			}
		})
	}
}

func TestCollector_buildMSSQLDynamicSQL(t *testing.T) {
	c := &Collector{}

	cols := []mssqlColumnMeta{
		{dbColumn: "query_hash", uiKey: "queryHash", dataType: "string", isIdentity: true},
		{dbColumn: "query_sql_text", uiKey: "query", dataType: "string", isIdentity: true},
		{dbColumn: "database_name", uiKey: "database", dataType: "string", isIdentity: true},
		{dbColumn: "count_executions", uiKey: "calls", dataType: "integer"},
		{dbColumn: "avg_duration", uiKey: "totalTime", dataType: "duration", isMicroseconds: true},
	}

	sql := c.buildMSSQLDynamicSQL(cols, "totalTime", 7, 500)

	assert.Contains(t, sql, "sys.query_store_query")
	assert.Contains(t, sql, "total_time")
	assert.Contains(t, sql, "TOP 500")
	assert.Contains(t, sql, "DATEADD")
	assert.Contains(t, sql, "q.query_hash")
}

func TestCollector_buildMSSQLDynamicSQL_NoTimeFilter(t *testing.T) {
	c := &Collector{}

	cols := []mssqlColumnMeta{
		{dbColumn: "query_hash", uiKey: "queryHash", dataType: "string", isIdentity: true},
		{dbColumn: "count_executions", uiKey: "calls", dataType: "integer"},
	}

	sql := c.buildMSSQLDynamicSQL(cols, "calls", 0, 500)

	assert.Contains(t, sql, "sys.query_store_query")
	assert.Contains(t, sql, "TOP 500")
	assert.NotContains(t, sql, "DATEADD")
}

func TestCollector_buildMSSQLDynamicColumns(t *testing.T) {
	c := &Collector{}

	cols := []mssqlColumnMeta{
		{uiKey: "queryHash", displayName: "Query Hash", dataType: "string", visible: false, isUniqueKey: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect"},
		{uiKey: "query", displayName: "Query", dataType: "string", visible: true, isSticky: true, fullWidth: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect"},
		{uiKey: "totalTime", displayName: "Total Time", dataType: "duration", units: "seconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range"},
	}

	columns := c.buildMSSQLDynamicColumns(cols)

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
		for _, opt := range method.SortOptions {
			assert.NotEmpty(t, opt.ID, "sort option must have ID")
			assert.NotEmpty(t, opt.Label, "sort option %s must have Label", opt.ID)
			assert.Contains(t, opt.Label, "Top queries by", "label should have standard prefix")
		}
	}
}

// TestSortColumnValidation_SQLInjection verifies that SQL injection attempts
// are handled by the validation mechanism
func TestMssqlSortColumnValidation_SQLInjection(t *testing.T) {
	c := &Collector{}
	availableCols := map[string]bool{"avg_duration": true, "count_executions": true}

	maliciousInputs := []string{
		"'; DROP TABLE sys.query_store_query; --",
		"total_time_ms; DELETE FROM master.dbo.sysdatabases",
		"1 OR 1=1",
		"WAITFOR DELAY '00:00:10'",
		"xp_cmdshell 'whoami'",
	}

	for _, input := range maliciousInputs {
		result := c.mapAndValidateMSSQLSortColumn(input, availableCols)
		// All malicious inputs should fall back to safe default
		assert.Equal(t, "totalTime", result,
			"malicious input should fall back to safe default: %s -> %s", input, result)
	}
}
