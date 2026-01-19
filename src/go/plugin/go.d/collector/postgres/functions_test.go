// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// pgVersionOld is used only for tests (not in production code)
const pgVersionOld = 12_00_00 // PG 12 - before total_exec_time was introduced

func TestPgMethods(t *testing.T) {
	methods := pgMethods()

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

func TestPgAllColumns_HasRequiredColumns(t *testing.T) {
	// Verify all required base columns are defined
	requiredUIKeys := []string{
		"queryid", "query", "database", "user", "calls",
		"totalTime", "meanTime", "minTime", "maxTime",
		"rows", "sharedBlksHit", "sharedBlksRead", "tempBlksWritten",
	}

	uiKeys := make(map[string]bool)
	for _, col := range pgAllColumns {
		uiKeys[col.uiKey] = true
	}

	for _, key := range requiredUIKeys {
		assert.True(t, uiKeys[key], "column %s should be defined in pgAllColumns", key)
	}
}

func TestPgAllColumns_HasValidMetadata(t *testing.T) {
	for _, col := range pgAllColumns {
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

func TestCollector_mapAndValidateSortColumn(t *testing.T) {
	tests := map[string]struct {
		pgVersion     int
		availableCols map[string]bool
		input         string
		expected      string
	}{
		"totalTime on PG12 maps to total_time": {
			pgVersion:     pgVersionOld,
			availableCols: map[string]bool{"total_time": true, "calls": true},
			input:         "totalTime",
			expected:      "total_time",
		},
		"totalTime on PG13 maps to total_exec_time": {
			pgVersion:     pgVersion13,
			availableCols: map[string]bool{"total_exec_time": true, "calls": true},
			input:         "totalTime",
			expected:      "total_exec_time",
		},
		"calls unchanged on any version": {
			pgVersion:     pgVersion13,
			availableCols: map[string]bool{"total_exec_time": true, "calls": true},
			input:         "calls",
			expected:      "calls",
		},
		"invalid column falls back to default (PG12)": {
			pgVersion:     pgVersionOld,
			availableCols: map[string]bool{"total_time": true},
			input:         "invalid_column",
			expected:      "total_time",
		},
		"invalid column falls back to default (PG13)": {
			pgVersion:     pgVersion13,
			availableCols: map[string]bool{"total_exec_time": true},
			input:         "invalid_column",
			expected:      "total_exec_time",
		},
		"SQL injection attempt falls back to default": {
			pgVersion:     pgVersion13,
			availableCols: map[string]bool{"total_exec_time": true},
			input:         "'; DROP TABLE users;--",
			expected:      "total_exec_time",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{pgVersion: tc.pgVersion}
			result := c.mapAndValidateSortColumn(tc.input, tc.availableCols)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCollector_buildAvailableColumns(t *testing.T) {
	tests := map[string]struct {
		pgVersion     int
		availableCols map[string]bool
		expectCols    []string // UI keys we expect to see
		notExpectCols []string // UI keys we don't expect
	}{
		"PG12 with basic columns": {
			pgVersion: pgVersionOld,
			availableCols: map[string]bool{
				"queryid": true, "query": true, "calls": true,
				"total_time": true, "mean_time": true, "min_time": true, "max_time": true,
				"rows": true, "shared_blks_hit": true, "shared_blks_read": true,
			},
			expectCols:    []string{"queryid", "query", "calls", "totalTime", "meanTime", "rows"},
			notExpectCols: []string{"plans", "totalPlanTime", "walRecords"}, // PG13+ only
		},
		"PG13 with exec_time columns": {
			pgVersion: pgVersion13,
			availableCols: map[string]bool{
				"queryid": true, "query": true, "calls": true,
				"total_exec_time": true, "mean_exec_time": true,
				"rows": true, "plans": true, "total_plan_time": true,
				"wal_records": true,
			},
			expectCols: []string{"queryid", "query", "calls", "totalTime", "plans", "walRecords"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{pgVersion: tc.pgVersion}
			cols := c.buildAvailableColumns(tc.availableCols)

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

func TestCollector_buildDynamicSQL(t *testing.T) {
	tests := map[string]struct {
		pgVersion  int
		sortColumn string
		checkPG13  bool
	}{
		"PG12 builds valid SQL": {
			pgVersion:  pgVersionOld,
			sortColumn: "total_time",
			checkPG13:  false,
		},
		"PG13 builds valid SQL with exec_time": {
			pgVersion:  pgVersion13,
			sortColumn: "total_exec_time",
			checkPG13:  true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{pgVersion: tc.pgVersion}

			// Build minimal column set for test
			cols := []pgColumnMeta{
				{dbColumn: "s.queryid", uiKey: "queryid", dataType: "string"},
				{dbColumn: "s.query", uiKey: "query", dataType: "string"},
				{dbColumn: "s.calls", uiKey: "calls", dataType: "integer"},
				{dbColumn: "total_time", uiKey: "totalTime", dataType: "duration"},
			}

			sql := c.buildDynamicSQL(cols, tc.sortColumn, 500)

			assert.Contains(t, sql, "pg_stat_statements")
			assert.Contains(t, sql, tc.sortColumn)
			assert.Contains(t, sql, "LIMIT 500")
			assert.Contains(t, sql, "s.queryid")
		})
	}
}

func TestCollector_buildDynamicColumns(t *testing.T) {
	c := &Collector{}

	cols := []pgColumnMeta{
		{uiKey: "queryid", displayName: "Query ID", dataType: "string", visible: false, isUniqueKey: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect"},
		{uiKey: "query", displayName: "Query", dataType: "string", visible: true, isSticky: true, fullWidth: true, transform: "none", sortDir: "ascending", summary: "count", filter: "multiselect"},
		{uiKey: "totalTime", displayName: "Total Time", dataType: "duration", units: "seconds", visible: true, transform: "duration", decimalPoints: 2, sortDir: "descending", summary: "sum", filter: "range"},
	}

	columns := c.buildDynamicColumns(cols)

	// Verify column count
	assert.Len(t, columns, 3)

	// Verify queryid column
	queryidCol := columns["queryid"].(map[string]any)
	assert.Equal(t, "Query ID", queryidCol["name"])
	assert.Equal(t, "string", queryidCol["type"])
	assert.True(t, queryidCol["unique_key"].(bool))
	assert.False(t, queryidCol["visible"].(bool))
	assert.Equal(t, 0, queryidCol["index"])

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
func TestPgMethods_SortOptionsHaveLabels(t *testing.T) {
	methods := pgMethods()

	for _, method := range methods {
		for _, opt := range method.SortOptions {
			assert.NotEmpty(t, opt.ID, "sort option must have ID")
			assert.NotEmpty(t, opt.Label, "sort option %s must have Label", opt.ID)
			assert.Contains(t, opt.Label, "Top queries by", "label should have standard prefix")
		}
	}
}
