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
			require.Equal("total_time", opt.ID)
			break
		}
	}
	require.True(hasDefault, "should have a default sort option")
}

func TestMssqlValidSortColumns(t *testing.T) {
	// Test that all expected columns are in the whitelist
	expectedColumns := []string{
		"total_time_ms", "calls", "avg_time_ms",
		"avg_cpu_ms", "avg_reads", "avg_writes",
	}

	for _, col := range expectedColumns {
		assert.True(t, mssqlValidSortColumns[col], "column %s should be in whitelist", col)
	}

	// Test that invalid columns are not in whitelist
	invalidColumns := []string{"invalid", "drop_table", "'; DROP TABLE users;--", "total_time"}
	for _, col := range invalidColumns {
		assert.False(t, mssqlValidSortColumns[col], "column %s should NOT be in whitelist", col)
	}
}

func TestMssqlMethods_SortOptionsMatchWhitelist(t *testing.T) {
	methods := mssqlMethods()

	for _, method := range methods {
		for _, opt := range method.SortOptions {
			// The Column field should be in the whitelist
			assert.True(t, mssqlValidSortColumns[opt.Column],
				"sort option %s column %s should be in whitelist", opt.ID, opt.Column)
		}
	}
}

func TestBuildMSSQLTopQueriesColumns(t *testing.T) {
	columns := buildMSSQLTopQueriesColumns()

	// Verify all expected columns are present
	expectedColumns := []string{
		"query_hash", "query", "database", "calls",
		"total_time", "avg_time", "avg_cpu", "avg_reads", "avg_writes",
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

	// Verify unique_key is set on query_hash
	queryHashCol := columns["query_hash"].(map[string]any)
	assert.True(t, queryHashCol["unique_key"].(bool), "query_hash should be unique_key")
	assert.False(t, queryHashCol["visible"].(bool), "query_hash should be hidden")
	assert.Equal(t, 0, queryHashCol["index"])
}

func TestCollector_buildQueryStoreSQL(t *testing.T) {
	tests := map[string]struct {
		sortColumn     string
		timeWindowDays int
		checkTimeFilter bool
	}{
		"with time filter": {
			sortColumn:     "total_time_ms",
			timeWindowDays: 7,
			checkTimeFilter: true,
		},
		"without time filter": {
			sortColumn:     "calls",
			timeWindowDays: 0,
			checkTimeFilter: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{}
			sql := c.buildQueryStoreSQL(tc.sortColumn, tc.timeWindowDays)

			assert.Contains(t, sql, "sys.query_store_query")
			assert.Contains(t, sql, tc.sortColumn)
			assert.Contains(t, sql, "TOP 5000")

			if tc.checkTimeFilter {
				assert.Contains(t, sql, "DATEADD")
			}
		})
	}
}

// TestSortColumnValidation_SQLInjection verifies that SQL injection attempts
// are caught by the whitelist defense-in-depth mechanism
func TestMssqlSortColumnValidation_SQLInjection(t *testing.T) {
	maliciousInputs := []string{
		"'; DROP TABLE sys.query_store_query; --",
		"total_time_ms; DELETE FROM master.dbo.sysdatabases",
		"1 OR 1=1",
		"WAITFOR DELAY '00:00:10'",
		"xp_cmdshell 'whoami'",
	}

	for _, input := range maliciousInputs {
		assert.False(t, mssqlValidSortColumns[input],
			"malicious input should not be in whitelist: %s", input)
	}
}
