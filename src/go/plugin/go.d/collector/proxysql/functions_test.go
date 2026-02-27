// SPDX-License-Identifier: GPL-3.0-or-later

package proxysql

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
)

func TestProxySQLMethods(t *testing.T) {
	methods := proxysqlMethods()

	require := assert.New(t)
	require.Len(methods, 1)
	require.Equal("top-queries", methods[0].ID)
	require.Equal("Top Queries", methods[0].Name)
	require.NotEmpty(methods[0].RequiredParams)

	var sortParam *funcapi.ParamConfig
	for i := range methods[0].RequiredParams {
		if methods[0].RequiredParams[i].ID == "__sort" {
			sortParam = &methods[0].RequiredParams[i]
			break
		}
	}
	require.NotNil(sortParam, "expected __sort required param")
	require.NotEmpty(sortParam.Options)
}

func TestProxySQLAllColumns_HasRequiredColumns(t *testing.T) {
	required := []string{"digest", "query", "calls", "totalTime"}

	cs := proxysqlColumnSet(proxysqlAllColumns)

	for _, id := range required {
		assert.True(t, cs.ContainsColumn(id), "column %s should be defined", id)
	}
}

func TestCollector_mapAndValidateProxySQLSortColumn(t *testing.T) {
	tests := map[string]struct {
		columns  []proxysqlColumn
		input    string
		expected string
	}{
		"valid totalTime": {
			columns:  []proxysqlColumn{{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime"}}, {ColumnMeta: funcapi.ColumnMeta{Name: "calls"}}},
			input:    "totalTime",
			expected: "totalTime",
		},
		"invalid falls back to totalTime": {
			columns:  []proxysqlColumn{{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime"}}, {ColumnMeta: funcapi.ColumnMeta{Name: "calls"}}},
			input:    "bad",
			expected: "totalTime",
		},
		"fallback to calls": {
			columns:  []proxysqlColumn{{ColumnMeta: funcapi.ColumnMeta{Name: "calls"}}},
			input:    "bad",
			expected: "calls",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c := &Collector{}
			cs := proxysqlColumnSet(tc.columns)
			assert.Equal(t, tc.expected, c.mapAndValidateProxySQLSortColumn(tc.input, cs))
		})
	}
}

func TestCollector_buildProxySQLDynamicSQL(t *testing.T) {
	c := &Collector{}

	cols := []proxysqlColumn{
		{ColumnMeta: funcapi.ColumnMeta{Name: "digest", Type: funcapi.FieldTypeString}, DBColumn: "digest"},
		{ColumnMeta: funcapi.ColumnMeta{Name: "query", Type: funcapi.FieldTypeString}, DBColumn: "digest_text"},
		{ColumnMeta: funcapi.ColumnMeta{Name: "calls", Type: funcapi.FieldTypeInteger}, DBColumn: "count_star"},
		{ColumnMeta: funcapi.ColumnMeta{Name: "totalTime", Type: funcapi.FieldTypeDuration}, DBColumn: "sum_time", IsMicroseconds: true},
	}

	sql := c.buildProxySQLDynamicSQL(cols, "totalTime", 500)

	assert.Contains(t, sql, "stats_mysql_query_digest")
	assert.Contains(t, sql, "ORDER BY `totalTime` DESC")
	assert.Contains(t, sql, "LIMIT 500")
	assert.Contains(t, sql, "sum_time/1000 AS `totalTime`")
}
