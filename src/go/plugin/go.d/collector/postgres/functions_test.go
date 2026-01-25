// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
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

func TestPgAllColumns_HasRequiredColumns(t *testing.T) {
	// Verify all required base columns are defined
	requiredIDs := []string{
		"queryid", "query", "database", "user", "calls",
		"totalTime", "meanTime", "minTime", "maxTime",
		"rows", "sharedBlksHit", "sharedBlksRead", "tempBlksWritten",
	}

	cs := pgColumnSet(pgAllColumns)

	for _, id := range requiredIDs {
		assert.True(t, cs.ContainsColumn(id), "column %s should be defined in pgAllColumns", id)
	}
}

func TestPgAllColumns_HasValidMetadata(t *testing.T) {
	for _, col := range pgAllColumns {
		// Every column must have an ID
		assert.NotEmpty(t, col.Name, "column %s must have Name", col.DBColumn)

		// Every column must have a display name (tooltip)
		assert.NotEmpty(t, col.Tooltip, "column %s must have Tooltip", col.Name)

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

func TestFuncTopQueries_mapAndValidateSortColumn(t *testing.T) {
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
			r := &funcRouter{collector: c}
			f := &funcTopQueries{router: r}
			result := f.mapAndValidateSortColumn(tc.input, tc.availableCols)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFuncTopQueries_buildAvailableColumns(t *testing.T) {
	tests := map[string]struct {
		pgVersion     int
		availableCols map[string]bool
		expectCols    []string // Column IDs we expect to see
		notExpectCols []string // Column IDs we don't expect
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
			r := &funcRouter{collector: c}
			f := &funcTopQueries{router: r}
			cols := f.buildAvailableColumns(tc.availableCols)
			cs := pgColumnSet(cols)

			for _, id := range tc.expectCols {
				assert.True(t, cs.ContainsColumn(id), "expected column %s to be present", id)
			}
			for _, id := range tc.notExpectCols {
				assert.False(t, cs.ContainsColumn(id), "did not expect column %s to be present", id)
			}
		})
	}
}

func TestFuncTopQueries_buildDynamicSQL(t *testing.T) {
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
			r := &funcRouter{collector: c}
			f := &funcTopQueries{router: r}

			// Build minimal column set for test
			cols := []pgColumn{
				{ColumnMeta: funcapi.ColumnMeta{Name: "queryid", Type: funcapi.FieldTypeString}, DBColumn: "s.queryid"},
				{ColumnMeta: funcapi.ColumnMeta{Name: "query", Type: funcapi.FieldTypeString}, DBColumn: "s.query"},
				{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Type: funcapi.FieldTypeInteger}, DBColumn: "s.calls"},
				{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Type: funcapi.FieldTypeDuration}, DBColumn: "total_time"},
			}

			sql := f.buildDynamicSQL(cols, tc.sortColumn, 500)

			assert.Contains(t, sql, "pg_stat_statements")
			assert.Contains(t, sql, tc.sortColumn)
			assert.Contains(t, sql, "LIMIT 500")
			assert.Contains(t, sql, "s.queryid")
		})
	}
}

func TestPgColumnSet_BuildColumns(t *testing.T) {
	cols := []pgColumn{
		{ColumnMeta: funcapi.ColumnMeta{Name: "queryid", Tooltip: "Query ID", Type: funcapi.FieldTypeString, Visible: false, UniqueKey: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "s.queryid"},
		{ColumnMeta: funcapi.ColumnMeta{Name: "query", Tooltip: "Query", Type: funcapi.FieldTypeString, Visible: true, Sticky: true, FullWidth: true, Transform: funcapi.FieldTransformNone, Sort: funcapi.FieldSortAscending, Summary: funcapi.FieldSummaryCount, Filter: funcapi.FieldFilterMultiselect, Sortable: true}, DBColumn: "s.query"},
		{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Tooltip: "Total Time", Type: funcapi.FieldTypeDuration, Units: "seconds", Visible: true, Transform: funcapi.FieldTransformDuration, DecimalPoints: 2, Sort: funcapi.FieldSortDescending, Summary: funcapi.FieldSummarySum, Filter: funcapi.FieldFilterRange, Sortable: true}, DBColumn: "total_time"},
	}

	cs := pgColumnSet(cols)
	columns := cs.BuildColumns()

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
