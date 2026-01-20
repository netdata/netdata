// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMysqlMethods(t *testing.T) {
	methods := mysqlMethods()

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

func TestMysqlAllColumns_HasRequiredColumns(t *testing.T) {
	// Verify all required base columns are defined
	requiredUIKeys := []string{
		"digest", "query", "schema", "calls",
		"totalTime", "avgTime", "minTime", "maxTime",
		"rowsSent", "rowsExamined", "noIndexUsed",
	}

	uiKeys := make(map[string]bool)
	for _, col := range mysqlAllColumns {
		uiKeys[col.uiKey] = true
	}

	for _, key := range requiredUIKeys {
		assert.True(t, uiKeys[key], "column %s should be defined in mysqlAllColumns", key)
	}
}

func TestMysqlAllColumns_HasValidMetadata(t *testing.T) {
	for _, col := range mysqlAllColumns {
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

func TestCollector_mapAndValidateMySQLSortColumn(t *testing.T) {
	tests := map[string]struct {
		availableCols map[string]bool
		input         string
		expected      string
	}{
		"totalTime maps to total_time alias": {
			availableCols: map[string]bool{"SUM_TIMER_WAIT": true, "COUNT_STAR": true},
			input:         "totalTime",
			expected:      "total_time",
		},
		"calls maps to calls alias": {
			availableCols: map[string]bool{"SUM_TIMER_WAIT": true, "COUNT_STAR": true},
			input:         "calls",
			expected:      "calls",
		},
		"invalid column falls back to total_time alias": {
			availableCols: map[string]bool{"SUM_TIMER_WAIT": true, "COUNT_STAR": true},
			input:         "invalid_column",
			expected:      "total_time",
		},
		"SQL injection attempt falls back to total_time alias": {
			availableCols: map[string]bool{"SUM_TIMER_WAIT": true, "COUNT_STAR": true},
			input:         "'; DROP TABLE users;--",
			expected:      "total_time",
		},
		"falls back to calls alias when SUM_TIMER_WAIT unavailable": {
			availableCols: map[string]bool{"COUNT_STAR": true},
			input:         "invalid_column",
			expected:      "calls",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{}
			result := c.mapAndValidateMySQLSortColumn(tc.input, tc.availableCols)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCollector_buildAvailableMySQLColumns(t *testing.T) {
	tests := map[string]struct {
		availableCols map[string]bool
		expectCols    []string // UI keys we expect to see
		notExpectCols []string // UI keys we don't expect
	}{
		"Basic MySQL 5.7 columns": {
			availableCols: map[string]bool{
				"DIGEST": true, "DIGEST_TEXT": true, "SCHEMA_NAME": true, "COUNT_STAR": true,
				"SUM_TIMER_WAIT": true, "MIN_TIMER_WAIT": true, "AVG_TIMER_WAIT": true, "MAX_TIMER_WAIT": true,
				"SUM_ROWS_SENT": true, "SUM_ROWS_EXAMINED": true, "SUM_NO_INDEX_USED": true,
			},
			expectCols:    []string{"digest", "query", "schema", "calls", "totalTime", "rowsSent"},
			notExpectCols: []string{"p95Time", "cpuTime", "maxTotalMemory"}, // MySQL 8.0+ only
		},
		"MySQL 8.0 with quantiles": {
			availableCols: map[string]bool{
				"DIGEST": true, "DIGEST_TEXT": true, "SCHEMA_NAME": true, "COUNT_STAR": true,
				"SUM_TIMER_WAIT": true, "QUANTILE_95": true, "QUANTILE_99": true,
				"QUERY_SAMPLE_TEXT": true,
			},
			expectCols: []string{"digest", "query", "calls", "p95Time", "p99Time", "sampleQuery"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{}
			cols := c.buildAvailableMySQLColumns(tc.availableCols)

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

func TestCollector_buildMySQLDynamicSQL(t *testing.T) {
	c := &Collector{}

	cols := []mysqlColumnMeta{
		{dbColumn: "DIGEST", alias: "digest", uiKey: "digest", dataType: "string"},
		{dbColumn: "DIGEST_TEXT", alias: "query", uiKey: "query", dataType: "string"},
		{dbColumn: "COUNT_STAR", alias: "calls", uiKey: "calls", dataType: "integer"},
		{dbColumn: "SUM_TIMER_WAIT", alias: "total_time", uiKey: "totalTime", dataType: "duration", isPicoseconds: true},
	}

	sql := c.buildMySQLDynamicSQL(cols, "total_time", 500)

	assert.Contains(t, sql, "performance_schema.events_statements_summary_by_digest")
	assert.Contains(t, sql, "ORDER BY total_time DESC")
	assert.Contains(t, sql, "LIMIT 500")
	assert.Contains(t, sql, "AS digest")
	assert.Contains(t, sql, "AS calls")
	assert.Contains(t, sql, "AS total_time")
	// Picosecond columns should have conversion
	assert.Contains(t, sql, "SUM_TIMER_WAIT/1000000000 AS total_time")
}

func TestCollector_buildMySQLDynamicColumns(t *testing.T) {
	c := &Collector{}

	cols := []mysqlColumnMeta{
		{uiKey: "digest", displayName: "Digest", dataType: "string", visible: false, isUniqueKey: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect"},
		{uiKey: "query", displayName: "Query", dataType: "string", visible: true, isSticky: true, fullWidth: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect"},
		{uiKey: "totalTime", displayName: "Total Time", dataType: "duration", units: "seconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range"},
	}

	columns := c.buildMySQLDynamicColumns(cols)

	// Verify column count
	assert.Len(t, columns, 3)

	// Verify digest column
	digestCol := columns["digest"].(map[string]any)
	assert.Equal(t, "Digest", digestCol["name"])
	assert.Equal(t, "string", digestCol["type"])
	assert.True(t, digestCol["unique_key"].(bool))
	assert.False(t, digestCol["visible"].(bool))
	assert.Equal(t, 0, digestCol["index"])

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
func TestMysqlMethods_SortOptionsHaveLabels(t *testing.T) {
	methods := mysqlMethods()

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
func TestSortColumnValidation_SQLInjection(t *testing.T) {
	c := &Collector{}
	availableCols := map[string]bool{"SUM_TIMER_WAIT": true, "COUNT_STAR": true}

	maliciousInputs := []string{
		"'; DROP TABLE performance_schema; --",
		"COUNT_STAR; DELETE FROM mysql.user",
		"1 OR 1=1",
		"SLEEP(10)",
		"BENCHMARK(10000000,SHA1('test'))",
	}

	for _, input := range maliciousInputs {
		result := c.mapAndValidateMySQLSortColumn(input, availableCols)
		// All malicious inputs should fall back to safe default (aliases)
		assert.True(t, result == "total_time" || result == "calls",
			"malicious input should fall back to safe default alias: %s -> %s", input, result)
	}
}
