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
			require.Equal("total_time", opt.ID)
			break
		}
	}
	require.True(hasDefault, "should have a default sort option")
}

func TestMysqlValidSortColumns(t *testing.T) {
	// Test that all expected columns are in the whitelist
	expectedColumns := []string{
		"SUM_TIMER_WAIT", "COUNT_STAR", "AVG_TIMER_WAIT",
		"SUM_ROWS_SENT", "SUM_ROWS_EXAMINED", "SUM_NO_INDEX_USED",
	}

	for _, col := range expectedColumns {
		assert.True(t, mysqlValidSortColumns[col], "column %s should be in whitelist", col)
	}

	// Test that invalid columns are not in whitelist
	invalidColumns := []string{"invalid", "drop_table", "'; DROP TABLE users;--", "total_time"}
	for _, col := range invalidColumns {
		assert.False(t, mysqlValidSortColumns[col], "column %s should NOT be in whitelist", col)
	}
}

func TestMysqlMethods_SortOptionsMatchWhitelist(t *testing.T) {
	methods := mysqlMethods()

	for _, method := range methods {
		for _, opt := range method.SortOptions {
			// The Column field should be in the whitelist
			assert.True(t, mysqlValidSortColumns[opt.Column],
				"sort option %s column %s should be in whitelist", opt.ID, opt.Column)
		}
	}
}

func TestBuildMySQLTopQueriesColumns(t *testing.T) {
	columns := buildMySQLTopQueriesColumns()

	// Verify all expected columns are present
	expectedColumns := []string{
		"digest", "query", "schema", "calls",
		"total_time", "avg_time", "min_time", "max_time",
		"rows_sent", "rows_examined", "no_index_used", "no_good_index_used",
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

	// Verify unique_key is set on digest
	digestCol := columns["digest"].(map[string]any)
	assert.True(t, digestCol["unique_key"].(bool), "digest should be unique_key")
	assert.False(t, digestCol["visible"].(bool), "digest should be hidden")
	assert.Equal(t, 0, digestCol["index"])
}

// TestSortColumnValidation_SQLInjection verifies that SQL injection attempts
// are caught by the whitelist defense-in-depth mechanism
func TestSortColumnValidation_SQLInjection(t *testing.T) {
	maliciousInputs := []string{
		"'; DROP TABLE performance_schema; --",
		"COUNT_STAR; DELETE FROM mysql.user",
		"1 OR 1=1",
		"SLEEP(10)",
		"BENCHMARK(10000000,SHA1('test'))",
	}

	for _, input := range maliciousInputs {
		assert.False(t, mysqlValidSortColumns[input],
			"malicious input should not be in whitelist: %s", input)
	}
}
