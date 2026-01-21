// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestMysqlMethods(t *testing.T) {
	methods := mysqlMethods()

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
		assert.NotEqual(t, funcapi.FieldTypeNone, col.dataType, "column %s must have dataType", col.uiKey)

		// Duration columns must have units
		if col.dataType == ftDuration {
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
		"totalTime maps correctly": {
			availableCols: map[string]bool{"SUM_TIMER_WAIT": true, "COUNT_STAR": true},
			input:         "totalTime",
			expected:      "totalTime",
		},
		"calls maps correctly": {
			availableCols: map[string]bool{"SUM_TIMER_WAIT": true, "COUNT_STAR": true},
			input:         "calls",
			expected:      "calls",
		},
		"invalid column falls back to totalTime": {
			availableCols: map[string]bool{"SUM_TIMER_WAIT": true, "COUNT_STAR": true},
			input:         "invalid_column",
			expected:      "totalTime",
		},
		"SQL injection attempt falls back to totalTime": {
			availableCols: map[string]bool{"SUM_TIMER_WAIT": true, "COUNT_STAR": true},
			input:         "'; DROP TABLE users;--",
			expected:      "totalTime",
		},
		"falls back to calls when SUM_TIMER_WAIT unavailable": {
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
		{dbColumn: "DIGEST", uiKey: "digest", dataType: ftString},
		{dbColumn: "DIGEST_TEXT", uiKey: "query", dataType: ftString},
		{dbColumn: "COUNT_STAR", uiKey: "calls", dataType: ftInteger},
		{dbColumn: "SUM_TIMER_WAIT", uiKey: "totalTime", dataType: ftDuration, isPicoseconds: true},
	}

	sql := c.buildMySQLDynamicSQL(cols, "totalTime", 500)

	assert.Contains(t, sql, "performance_schema.events_statements_summary_by_digest")
	assert.Contains(t, sql, "ORDER BY `totalTime` DESC")
	assert.Contains(t, sql, "LIMIT 500")
	assert.Contains(t, sql, "AS `digest`")
	assert.Contains(t, sql, "AS `calls`")
	assert.Contains(t, sql, "AS `totalTime`")
	// Picosecond columns should have conversion
	assert.Contains(t, sql, "SUM_TIMER_WAIT/1000000000 AS `totalTime`")
}

func TestCollector_buildMySQLDynamicColumns(t *testing.T) {
	c := &Collector{}

	cols := []mysqlColumnMeta{
		{uiKey: "digest", displayName: "Digest", dataType: ftString, visible: false, isUniqueKey: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti},
		{uiKey: "query", displayName: "Query", dataType: ftString, visible: true, isSticky: true, fullWidth: true, transform: trNone, sortDir: sortAsc, summary: summaryCount, filter: filterMulti},
		{uiKey: "totalTime", displayName: "Total Time", dataType: ftDuration, units: "seconds", visible: true, transform: trDuration, decimalPoints: 2, sortDir: sortDesc, summary: summarySum, filter: filterRange},
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
		// All malicious inputs should fall back to safe default
		assert.True(t, result == "totalTime" || result == "calls",
			"malicious input should fall back to safe default: %s -> %s", input, result)
	}
}
