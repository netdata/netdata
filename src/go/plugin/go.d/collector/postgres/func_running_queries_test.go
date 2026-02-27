// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestRunningQueriesColumns_HasRequiredColumns(t *testing.T) {
	requiredNames := []string{
		"durationMs", "query", "datname", "usename",
		"applicationName", "clientAddr", "waitEvent", "pid",
		"state", "queryStart",
	}

	colNames := make(map[string]bool)
	for _, col := range runningQueriesColumns {
		colNames[col.Name] = true
	}

	for _, name := range requiredNames {
		assert.True(t, colNames[name], "column %s should be defined in runningQueriesColumns", name)
	}
}

func TestRunningQueriesColumns_HasValidMetadata(t *testing.T) {
	for _, col := range runningQueriesColumns {
		assert.NotEmpty(t, col.Name, "column must have Name")
		assert.NotEmpty(t, col.Tooltip, "column %s must have Tooltip", col.Name)
		assert.NotEqual(t, funcapi.FieldTypeNone, col.Type, "column %s must have Type", col.Name)
		assert.NotEmpty(t, col.DBColumn, "column %s must have DBColumn", col.Name)

		if col.Type == funcapi.FieldTypeDuration {
			assert.NotEmpty(t, col.Units, "duration column %s must have Units", col.Name)
		}

		if col.sortOpt {
			assert.NotEmpty(t, col.sortLbl, "sort option column %s must have sortLbl", col.Name)
		}
	}
}

func TestRunningQueriesColumns_HasDefaultSort(t *testing.T) {
	var defaultSortCol string
	for _, col := range runningQueriesColumns {
		if col.defaultSort {
			defaultSortCol = col.Name
			break
		}
	}
	assert.Equal(t, "durationMs", defaultSortCol, "durationMs should be the default sort column")
}

func TestFuncRunningQueries_getColumnsForVersion(t *testing.T) {
	tests := map[string]struct {
		pgVersion     int
		expectCols    []string
		notExpectCols []string
	}{
		"PG9 excludes version-gated columns": {
			pgVersion:     9_06_00,
			expectCols:    []string{"durationMs", "query", "pid", "state"},
			notExpectCols: []string{"backendType", "leaderPid", "queryId"},
		},
		"PG10 includes backendType": {
			pgVersion:     pgVersion10,
			expectCols:    []string{"durationMs", "query", "backendType"},
			notExpectCols: []string{"leaderPid", "queryId"},
		},
		"PG13 includes leaderPid": {
			pgVersion:     pgVersion13,
			expectCols:    []string{"durationMs", "backendType", "leaderPid"},
			notExpectCols: []string{"queryId"},
		},
		"PG14 includes queryId": {
			pgVersion:  pgVersion14,
			expectCols: []string{"durationMs", "backendType", "leaderPid", "queryId"},
		},
		"PG0 defaults to PG14 behavior": {
			pgVersion:  0,
			expectCols: []string{"backendType", "leaderPid", "queryId"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{pgVersion: tc.pgVersion}
			r := &funcRouter{collector: c}
			f := &funcRunningQueries{router: r}

			cols := f.getColumnsForVersion()
			colNames := make(map[string]bool)
			for _, col := range cols {
				colNames[col.Name] = true
			}

			for _, expected := range tc.expectCols {
				assert.True(t, colNames[expected], "expected column %s for PG version %d", expected, tc.pgVersion)
			}
			for _, notExpected := range tc.notExpectCols {
				assert.False(t, colNames[notExpected], "did not expect column %s for PG version %d", notExpected, tc.pgVersion)
			}
		})
	}
}

func TestFuncRunningQueries_resolveSortColumn(t *testing.T) {
	c := &Collector{pgVersion: pgVersion14}
	r := &funcRouter{collector: c}
	f := &funcRunningQueries{router: r}
	cols := f.getColumnsForVersion()

	tests := map[string]struct {
		input    string
		expected string
	}{
		"valid sort option durationMs": {
			input:    "durationMs",
			expected: "durationMs",
		},
		"valid sort option queryStart": {
			input:    "queryStart",
			expected: "queryStart",
		},
		"invalid column falls back to default": {
			input:    "invalid_column",
			expected: "durationMs",
		},
		"empty string falls back to default": {
			input:    "",
			expected: "durationMs",
		},
		"SQL injection attempt falls back to default": {
			input:    "'; DROP TABLE users;--",
			expected: "durationMs",
		},
		"non-sortable column falls back to default": {
			input:    "query", // query column has sortOpt: false
			expected: "durationMs",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := f.resolveSortColumn(tc.input, cols)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFuncRunningQueries_buildQuery(t *testing.T) {
	c := &Collector{pgVersion: pgVersion14}
	r := &funcRouter{collector: c}
	f := &funcRunningQueries{router: r}
	cols := f.getColumnsForVersion()

	query := f.buildQuery(cols, "durationMs")

	assert.Contains(t, query, "pg_stat_activity")
	assert.Contains(t, query, "WHERE state = 'active'")
	assert.Contains(t, query, "pid != pg_backend_pid()")
	assert.Contains(t, query, "LIMIT 500")
	assert.Contains(t, query, "ORDER BY")
	assert.Contains(t, query, "DESC NULLS LAST")
	// Check that durationMs expression is used for sorting
	assert.Contains(t, query, "EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - query_start)) * 1000")
}

func TestFuncRunningQueries_buildQuery_SortByQueryStart(t *testing.T) {
	c := &Collector{pgVersion: pgVersion14}
	r := &funcRouter{collector: c}
	f := &funcRunningQueries{router: r}
	cols := f.getColumnsForVersion()

	query := f.buildQuery(cols, "queryStart")

	// Should use query_start for ORDER BY
	assert.True(t, strings.Contains(query, "ORDER BY query_start DESC"),
		"expected ORDER BY query_start DESC in query")
}

func TestFuncRunningQueries_buildSelectClause(t *testing.T) {
	c := &Collector{pgVersion: pgVersion14}
	r := &funcRouter{collector: c}
	f := &funcRunningQueries{router: r}
	cols := f.getColumnsForVersion()

	selectClause := f.buildSelectClause(cols)

	// Should contain key DB columns
	assert.Contains(t, selectClause, "EXTRACT(EPOCH FROM (CURRENT_TIMESTAMP - query_start)) * 1000")
	assert.Contains(t, selectClause, "query")
	assert.Contains(t, selectClause, "datname")
	assert.Contains(t, selectClause, "pid")
	assert.Contains(t, selectClause, "backend_type")   // PG10+
	assert.Contains(t, selectClause, "query_id::text") // PG14+
}

func TestFuncRunningQueries_formatValue(t *testing.T) {
	f := &funcRunningQueries{}

	tests := map[string]struct {
		value    any
		col      runningQueriesColumn
		expected any
	}{
		"nil returns nil": {
			value:    nil,
			col:      runningQueriesColumn{ColumnMeta: funcapi.ColumnMeta{Type: funcapi.FieldTypeString}},
			expected: nil,
		},
		"string passthrough": {
			value:    "test_value",
			col:      runningQueriesColumn{ColumnMeta: funcapi.ColumnMeta{Type: funcapi.FieldTypeString}},
			expected: "test_value",
		},
		"int64 passthrough": {
			value:    int64(12345),
			col:      runningQueriesColumn{ColumnMeta: funcapi.ColumnMeta{Type: funcapi.FieldTypeInteger}},
			expected: int64(12345),
		},
		"int32 converts to int64": {
			value:    int32(123),
			col:      runningQueriesColumn{ColumnMeta: funcapi.ColumnMeta{Type: funcapi.FieldTypeInteger}},
			expected: int64(123),
		},
		"int converts to int64": {
			value:    int(456),
			col:      runningQueriesColumn{ColumnMeta: funcapi.ColumnMeta{Type: funcapi.FieldTypeInteger}},
			expected: int64(456),
		},
		"float64 duration passthrough": {
			value:    float64(123.456),
			col:      runningQueriesColumn{ColumnMeta: funcapi.ColumnMeta{Type: funcapi.FieldTypeDuration}},
			expected: float64(123.456),
		},
		"int64 duration converts to float64": {
			value:    int64(100),
			col:      runningQueriesColumn{ColumnMeta: funcapi.ColumnMeta{Type: funcapi.FieldTypeDuration}},
			expected: float64(100),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := f.formatValue(tc.value, tc.col)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFuncRunningQueries_formatValue_QueryTruncation(t *testing.T) {
	f := &funcRunningQueries{}

	// Create a string longer than runningQueriesMaxTextLength (4096)
	longQuery := strings.Repeat("SELECT * FROM very_long_table_name WHERE id = 1; ", 200)
	assert.Greater(t, len(longQuery), runningQueriesMaxTextLength)

	col := runningQueriesColumn{
		ColumnMeta: funcapi.ColumnMeta{Name: "query", Type: funcapi.FieldTypeString},
	}

	result := f.formatValue(longQuery, col)
	resultStr := result.(string)

	assert.LessOrEqual(t, len(resultStr), runningQueriesMaxTextLength+50, // Allow some buffer for truncation marker
		"query text should be truncated")
}

func TestFuncRunningQueries_formatValue_NonQueryStringNotTruncated(t *testing.T) {
	f := &funcRunningQueries{}

	// Long string but NOT the query column
	longValue := strings.Repeat("a", 5000)

	col := runningQueriesColumn{
		ColumnMeta: funcapi.ColumnMeta{Name: "datname", Type: funcapi.FieldTypeString},
	}

	result := f.formatValue(longValue, col)
	resultStr := result.(string)

	assert.Equal(t, len(longValue), len(resultStr), "non-query string columns should not be truncated")
}

func TestRunningQueriesMethodConfig(t *testing.T) {
	config := runningQueriesMethodConfig()

	assert.Equal(t, "running-queries", config.ID)
	assert.Equal(t, "Running Queries", config.Name)
	assert.Equal(t, 10, config.UpdateEvery)
	assert.True(t, config.RequireCloud)
	assert.NotEmpty(t, config.Help)
	assert.NotEmpty(t, config.RequiredParams)
}
