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
			require.Equal("total_time", opt.ID)
			break
		}
	}
	require.True(hasDefault, "should have a default sort option")
}

func TestPgValidSortColumns(t *testing.T) {
	// Test that all expected columns are in the whitelist
	expectedColumns := []string{
		"total_time", "calls", "mean_time", "rows",
		"shared_blks_read", "temp_blks_written",
		// PG 13+ column names
		"total_exec_time", "mean_exec_time", "min_exec_time", "max_exec_time",
	}

	for _, col := range expectedColumns {
		assert.True(t, pgValidSortColumns[col], "column %s should be in whitelist", col)
	}

	// Test that invalid columns are not in whitelist
	invalidColumns := []string{"invalid", "drop_table", "'; DROP TABLE users;--"}
	for _, col := range invalidColumns {
		assert.False(t, pgValidSortColumns[col], "column %s should NOT be in whitelist", col)
	}
}

func TestCollector_mapSortColumn(t *testing.T) {
	tests := map[string]struct {
		pgVersion  int
		input      string
		expected   string
	}{
		"total_time on PG12": {
			pgVersion: pgVersionOld,
			input:     "total_time",
			expected:  "total_time",
		},
		"total_time on PG13 maps to exec variant": {
			pgVersion: pgVersion13,
			input:     "total_time",
			expected:  "total_exec_time",
		},
		"mean_time on PG13 maps to exec variant": {
			pgVersion: pgVersion13,
			input:     "mean_time",
			expected:  "mean_exec_time",
		},
		"calls unchanged on any version": {
			pgVersion: pgVersion13,
			input:     "calls",
			expected:  "calls",
		},
		"rows unchanged on any version": {
			pgVersion: pgVersionOld,
			input:     "rows",
			expected:  "rows",
		},
		"invalid column falls back to default (PG12)": {
			pgVersion: pgVersionOld,
			input:     "invalid_column",
			expected:  "total_time",
		},
		"invalid column falls back to default (PG13)": {
			pgVersion: pgVersion13,
			input:     "invalid_column",
			expected:  "total_exec_time",
		},
		"SQL injection attempt falls back to default": {
			pgVersion: pgVersion13,
			input:     "'; DROP TABLE users;--",
			expected:  "total_exec_time",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{pgVersion: tc.pgVersion}
			result := c.mapSortColumn(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestCollector_buildTopQueriesSQL(t *testing.T) {
	tests := map[string]struct {
		pgVersion   int
		sortColumn  string
		checkPG13   bool // true if we expect PG13+ column aliases
	}{
		"PG12 uses old column names": {
			pgVersion:   pgVersionOld,
			sortColumn:  "total_time",
			checkPG13:   false,
		},
		"PG13 uses aliased column names": {
			pgVersion:   pgVersion13,
			sortColumn:  "total_exec_time",
			checkPG13:   true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{pgVersion: tc.pgVersion}
			sql := c.buildTopQueriesSQL(tc.sortColumn)

			assert.Contains(t, sql, "pg_stat_statements")
			assert.Contains(t, sql, tc.sortColumn)
			assert.Contains(t, sql, "LIMIT 5000")

			if tc.checkPG13 {
				assert.Contains(t, sql, "total_exec_time AS total_time")
			} else {
				assert.Contains(t, sql, "total_time, mean_time")
			}
		})
	}
}

func TestCollector_buildTopQueriesColumns(t *testing.T) {
	c := &Collector{}
	columns := c.buildTopQueriesColumns()

	// Verify all expected columns are present
	expectedColumns := []string{
		"queryid", "query", "database", "user", "calls",
		"total_time", "mean_time", "min_time", "max_time",
		"rows", "shared_blks_hit", "shared_blks_read", "temp_blks_written",
	}

	for _, col := range expectedColumns {
		_, ok := columns[col]
		assert.True(t, ok, "column %s should be present", col)
	}

	// Verify column metadata follows v3 schema
	queryCol := columns["query"].(map[string]any)
	assert.Equal(t, "Query", queryCol["name"])
	assert.Equal(t, "string", queryCol["type"])
	assert.True(t, queryCol["full_width"].(bool))
	assert.True(t, queryCol["visible"].(bool))
	assert.Equal(t, 1, queryCol["index"])

	totalTimeCol := columns["total_time"].(map[string]any)
	assert.Equal(t, "duration", totalTimeCol["type"])
	assert.True(t, totalTimeCol["visible"].(bool))

	// Verify unique_key is set on queryid
	queryidCol := columns["queryid"].(map[string]any)
	assert.True(t, queryidCol["unique_key"].(bool), "queryid should be unique_key")
	assert.Equal(t, "string", queryidCol["type"], "queryid should be string for JS precision")
	assert.False(t, queryidCol["visible"].(bool), "queryid should be hidden")
	assert.Equal(t, 0, queryidCol["index"])
}

// Test that method config sort options match valid columns
func TestPgMethods_SortOptionsMatchWhitelist(t *testing.T) {
	methods := pgMethods()

	for _, method := range methods {
		for _, opt := range method.SortOptions {
			// The Column field should map to a valid column after mapping
			// For semantic IDs like "total_time", the handler maps them via mapSortColumn
			c := &Collector{pgVersion: pgVersion13}
			mappedCol := c.mapSortColumn(opt.ID)
			assert.True(t, pgValidSortColumns[mappedCol],
				"sort option %s (maps to %s) should be in whitelist", opt.ID, mappedCol)
		}
	}
}
